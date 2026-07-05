package gateway

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	protoLogger "github.com/livekit/protocol/logger"
	lksdk "github.com/livekit/server-sdk-go/v2"
	lkmedia "github.com/livekit/server-sdk-go/v2/pkg/media"
	"github.com/pion/webrtc/v4"

	coregateway "github.com/plexusone/omnivoice-core/gateway"
)

// ProviderLiveKit identifies the LiveKit voice gateway provider.
const ProviderLiveKit coregateway.ProviderName = "livekit"

// Verify interface compliance at compile time.
var _ coregateway.WebRTCGateway = (*Gateway)(nil)

// Config configures the LiveKit voice gateway.
type Config struct {
	// LiveKit server configuration
	LiveKitURL    string // e.g., "wss://your-app.livekit.cloud"
	LiveKitAPIKey string
	LiveKitSecret string

	// Room configuration
	RoomName      string // Room to join on Start()
	AgentIdentity string // Identity of the AI agent participant
	AgentName     string // Display name for the agent

	// Audio configuration
	SampleRate int // 16000 or 24000 (default: 24000)
	Channels   int // 1 for mono (default: 1)

	// Voice pipeline configuration (passed to voice processing)
	STTProvider string
	STTAPIKey   string
	STTModel    string
	STTLanguage string

	TTSProvider string
	TTSAPIKey   string
	TTSVoiceID  string
	TTSModel    string

	LLMProvider     string
	LLMAPIKey       string
	LLMModel        string
	LLMSystemPrompt string

	// Session configuration
	MaxSessionDuration time.Duration
	InterruptionMode   string // "immediate", "after_sentence", "disabled"

	// Logging
	Logger protoLogger.Logger
}

// Gateway handles LiveKit WebRTC voice sessions.
type Gateway struct {
	config             Config
	room               *lksdk.Room
	currentRoom        string
	logger             protoLogger.Logger
	participantHandler coregateway.ParticipantHandler

	// Audio tracks
	localTrack *lkmedia.PCMLocalTrack

	mu       sync.RWMutex
	sessions map[string]*Session // keyed by participant identity
	started  bool
	cancel   context.CancelFunc
}

// New creates a new LiveKit voice gateway.
func New(cfg Config) (*Gateway, error) {
	if cfg.LiveKitURL == "" {
		return nil, errors.New("LiveKitURL is required")
	}
	if cfg.LiveKitAPIKey == "" {
		return nil, errors.New("LiveKitAPIKey is required")
	}
	if cfg.LiveKitSecret == "" {
		return nil, errors.New("LiveKitSecret is required")
	}
	if cfg.AgentIdentity == "" {
		cfg.AgentIdentity = "ai-agent"
	}
	if cfg.AgentName == "" {
		cfg.AgentName = "AI Agent"
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 24000
	}
	if cfg.Channels == 0 {
		cfg.Channels = 1
	}
	if cfg.Logger == nil {
		cfg.Logger = protoLogger.GetLogger()
	}
	if cfg.MaxSessionDuration == 0 {
		cfg.MaxSessionDuration = 30 * time.Minute
	}
	if cfg.InterruptionMode == "" {
		cfg.InterruptionMode = "immediate"
	}

	return &Gateway{
		config:   cfg,
		logger:   cfg.Logger,
		sessions: make(map[string]*Session),
	}, nil
}

// Name returns the provider name.
func (g *Gateway) Name() coregateway.ProviderName {
	return ProviderLiveKit
}

// OnParticipantJoined sets the handler for when participants join.
func (g *Gateway) OnParticipantJoined(handler coregateway.ParticipantHandler) {
	g.mu.Lock()
	g.participantHandler = handler
	g.mu.Unlock()
}

// Start connects to the configured room and starts handling participants.
func (g *Gateway) Start(ctx context.Context) error {
	if g.config.RoomName == "" {
		return errors.New("RoomName is required; set in config or use JoinRoom()")
	}
	return g.JoinRoom(ctx, g.config.RoomName)
}

// JoinRoom joins a specific LiveKit room.
func (g *Gateway) JoinRoom(ctx context.Context, roomName string) error {
	g.mu.Lock()
	if g.started {
		g.mu.Unlock()
		return errors.New("gateway already started; call LeaveRoom() first")
	}
	g.started = true
	g.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	g.cancel = cancel

	// Create room callbacks
	roomCallback := &lksdk.RoomCallback{
		ParticipantCallback: lksdk.ParticipantCallback{
			OnTrackSubscribed:   g.handleTrackSubscribed,
			OnTrackUnsubscribed: g.handleTrackUnsubscribed,
		},
		OnParticipantConnected:    g.handleParticipantConnected,
		OnParticipantDisconnected: g.handleParticipantDisconnected,
		OnDisconnected:            g.handleDisconnected,
	}

	// Connect to room
	room, err := lksdk.ConnectToRoom(
		g.config.LiveKitURL,
		lksdk.ConnectInfo{
			APIKey:              g.config.LiveKitAPIKey,
			APISecret:           g.config.LiveKitSecret,
			RoomName:            roomName,
			ParticipantIdentity: g.config.AgentIdentity,
			ParticipantName:     g.config.AgentName,
		},
		roomCallback,
	)
	if err != nil {
		g.mu.Lock()
		g.started = false
		g.mu.Unlock()
		return fmt.Errorf("connect to room: %w", err)
	}
	g.room = room
	g.currentRoom = roomName

	g.logger.Infow("connected to livekit room",
		"room", roomName,
		"identity", g.config.AgentIdentity,
		"url", g.config.LiveKitURL)

	// Create and publish audio track for agent output
	localTrack, err := lkmedia.NewPCMLocalTrack(
		g.config.SampleRate,
		g.config.Channels,
		g.logger,
	)
	if err != nil {
		room.Disconnect()
		g.mu.Lock()
		g.started = false
		g.mu.Unlock()
		return fmt.Errorf("create local track: %w", err)
	}
	g.localTrack = localTrack

	_, err = room.LocalParticipant.PublishTrack(localTrack, &lksdk.TrackPublicationOptions{
		Name:   "ai-voice",
		Source: livekit.TrackSource_MICROPHONE,
	})
	if err != nil {
		_ = localTrack.Close()
		room.Disconnect()
		g.mu.Lock()
		g.started = false
		g.mu.Unlock()
		return fmt.Errorf("publish track: %w", err)
	}

	g.logger.Infow("published agent audio track")

	// Handle existing participants
	for _, p := range room.GetRemoteParticipants() {
		g.handleParticipantConnected(p)
	}

	// Wait for context cancellation
	<-ctx.Done()
	return g.LeaveRoom()
}

// LeaveRoom leaves the current room.
func (g *Gateway) LeaveRoom() error {
	g.logger.Infow("leaving livekit room", "room", g.currentRoom)

	if g.cancel != nil {
		g.cancel()
	}

	// Close all sessions
	g.mu.Lock()
	for _, session := range g.sessions {
		_ = session.Close()
	}
	g.sessions = make(map[string]*Session)
	g.started = false
	g.currentRoom = ""
	g.mu.Unlock()

	// Close local track
	if g.localTrack != nil {
		_ = g.localTrack.Close()
		g.localTrack = nil
	}

	// Disconnect from room
	if g.room != nil {
		g.room.Disconnect()
		g.room = nil
	}

	return nil
}

// Stop is an alias for LeaveRoom.
func (g *Gateway) Stop() error {
	return g.LeaveRoom()
}

// CurrentRoom returns the name of the currently joined room.
func (g *Gateway) CurrentRoom() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.currentRoom
}

// GetSession retrieves an active session by participant identity.
func (g *Gateway) GetSession(participantID string) (coregateway.WebRTCSession, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	session, ok := g.sessions[participantID]
	if !ok {
		return nil, false
	}
	return session, true
}

// ListSessions returns all active sessions.
func (g *Gateway) ListSessions() []coregateway.WebRTCSession {
	g.mu.RLock()
	defer g.mu.RUnlock()

	sessions := make([]coregateway.WebRTCSession, 0, len(g.sessions))
	for _, s := range g.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

// GenerateClientToken creates a JWT token for a client to join a room.
func (g *Gateway) GenerateClientToken(roomName, identity, displayName string) (string, error) {
	if identity == "" {
		return "", errors.New("identity is required")
	}
	if roomName == "" {
		return "", errors.New("room name is required")
	}

	at := auth.NewAccessToken(g.config.LiveKitAPIKey, g.config.LiveKitSecret)

	canPublish := true
	grant := &auth.VideoGrant{
		RoomJoin:   true,
		Room:       roomName,
		CanPublish: &canPublish,
	}

	at.SetVideoGrant(grant).
		SetIdentity(identity).
		SetName(displayName).
		SetValidFor(6 * time.Hour)

	return at.ToJWT()
}

// handleParticipantConnected is called when a participant joins the room.
func (g *Gateway) handleParticipantConnected(p *lksdk.RemoteParticipant) {
	identity := p.Identity()

	g.logger.Infow("participant connected",
		"identity", identity,
		"name", p.Name())

	// Create participant info
	participantInfo := &coregateway.ParticipantInfo{
		Identity:    identity,
		DisplayName: p.Name(),
		RoomName:    g.currentRoom,
		Metadata:    p.Metadata(),
		JoinedAt:    time.Now(),
	}

	// Create session for this participant
	session := g.createSession(identity, p, participantInfo)

	// Notify participant handler
	g.mu.RLock()
	handler := g.participantHandler
	g.mu.RUnlock()

	if handler != nil {
		if err := handler(participantInfo); err != nil {
			g.logger.Warnw("participant handler rejected participant", err,
				"identity", identity)
			_ = session.Close()
			return
		}
	}

	// Send session started event
	session.sendEvent(coregateway.Event{
		Type:      coregateway.EventSessionStarted,
		Timestamp: time.Now(),
		Data: map[string]string{
			"participant": identity,
			"room":        g.currentRoom,
		},
	})
}

// handleParticipantDisconnected is called when a participant leaves the room.
func (g *Gateway) handleParticipantDisconnected(p *lksdk.RemoteParticipant) {
	identity := p.Identity()

	g.logger.Infow("participant disconnected", "identity", identity)

	g.mu.Lock()
	session, ok := g.sessions[identity]
	if ok {
		delete(g.sessions, identity)
	}
	g.mu.Unlock()

	if session != nil {
		session.sendEvent(coregateway.Event{
			Type:      coregateway.EventSessionEnded,
			Timestamp: time.Now(),
		})
		_ = session.Close()
	}
}

// handleTrackSubscribed is called when we subscribe to a participant's track.
func (g *Gateway) handleTrackSubscribed(track *webrtc.TrackRemote, pub *lksdk.RemoteTrackPublication, p *lksdk.RemoteParticipant) {
	identity := p.Identity()

	g.logger.Infow("track subscribed",
		"identity", identity,
		"track_kind", track.Kind().String(),
		"codec", track.Codec().MimeType)

	// Only handle audio tracks
	if track.Kind() != webrtc.RTPCodecTypeAudio {
		return
	}

	// Get or create session
	g.mu.RLock()
	session, ok := g.sessions[identity]
	g.mu.RUnlock()

	if !ok {
		g.logger.Warnw("received track for unknown participant", nil, "identity", identity)
		return
	}

	// Create remote track writer to receive audio
	session.handleTrackSubscribed(track)
}

// handleTrackUnsubscribed is called when a track is unsubscribed.
func (g *Gateway) handleTrackUnsubscribed(track *webrtc.TrackRemote, pub *lksdk.RemoteTrackPublication, p *lksdk.RemoteParticipant) {
	identity := p.Identity()

	g.logger.Infow("track unsubscribed",
		"identity", identity,
		"track_kind", track.Kind().String())

	g.mu.RLock()
	session, ok := g.sessions[identity]
	g.mu.RUnlock()

	if ok {
		session.handleTrackUnsubscribed(track)
	}
}

// handleDisconnected is called when the gateway disconnects from the room.
func (g *Gateway) handleDisconnected() {
	g.logger.Warnw("disconnected from livekit room", nil)
}

// createSession creates a new session for a participant.
func (g *Gateway) createSession(identity string, participant *lksdk.RemoteParticipant, info *coregateway.ParticipantInfo) *Session {
	session := &Session{
		id:              identity,
		gateway:         g,
		participant:     participant,
		participantInfo: info,
		roomName:        g.currentRoom,
		agentIdentity:   g.config.AgentIdentity,
		startTime:       time.Now(),
		events:          make(chan coregateway.Event, 100),
		done:            make(chan struct{}),
		logger:          g.logger.WithValues("participant", identity),
	}

	g.mu.Lock()
	g.sessions[identity] = session
	g.mu.Unlock()

	return session
}

// removeSession removes a session from the gateway.
func (g *Gateway) removeSession(identity string) {
	g.mu.Lock()
	delete(g.sessions, identity)
	g.mu.Unlock()
}

// GetLocalTrack returns the local audio track for writing agent audio.
func (g *Gateway) GetLocalTrack() *lkmedia.PCMLocalTrack {
	return g.localTrack
}

// Room returns the underlying LiveKit room.
func (g *Gateway) Room() *lksdk.Room {
	return g.room
}
