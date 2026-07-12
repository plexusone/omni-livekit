package heygen

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/plexusone/heygen-go/liveavatar"
	"github.com/plexusone/omni-livekit/avatar"
)

// SessionConfig configures a HeyGen LiveAvatar session.
type SessionConfig struct {
	// APIKey is the LiveAvatar API key.
	// Required.
	APIKey string

	// BaseURL is the LiveAvatar API base URL.
	// Default: https://api.liveavatar.com
	BaseURL string

	// AvatarID is the UUID of the avatar to use.
	// Use liveavatar.SandboxAvatarID for testing.
	// Required.
	AvatarID string

	// Sandbox enables sandbox mode (60s limit, no credits).
	Sandbox bool

	// VideoQuality sets the avatar video quality.
	// Default: high
	VideoQuality liveavatar.VideoQuality

	// AudioConfig configures the audio format.
	// Default: 24kHz mono PCM16
	AudioConfig avatar.AudioConfig
}

// Session implements avatar.Session for HeyGen LiveAvatar.
type Session struct {
	*avatar.BaseSession

	client       *liveavatar.Client
	avatarID     string
	sandbox      bool
	videoQuality liveavatar.VideoQuality
	audioConfig  avatar.AudioConfig

	// Session state
	sessionID    string
	sessionToken string

	// Participant tracking
	participantJoined chan struct{}
	participantLeft   chan struct{}

	mu     sync.Mutex
	closed bool
}

// NewSession creates a new HeyGen LiveAvatar session.
func NewSession(cfg SessionConfig) (*Session, error) {
	if cfg.APIKey == "" {
		return nil, avatar.ErrInvalidConfig
	}
	if cfg.AvatarID == "" {
		return nil, avatar.ErrInvalidConfig
	}

	clientCfg := &liveavatar.Config{
		APIKey: cfg.APIKey,
	}
	if cfg.BaseURL != "" {
		clientCfg.BaseURL = cfg.BaseURL
	}

	client, err := liveavatar.NewClient(clientCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create LiveAvatar client: %w", err)
	}

	audioConfig := cfg.AudioConfig
	if audioConfig.SampleRate == 0 {
		// LiveAvatar works well with 24kHz audio
		audioConfig = avatar.AudioConfig{
			SampleRate: 24000,
			Channels:   1,
			Encoding:   "linear16",
		}
	}

	videoQuality := cfg.VideoQuality
	if videoQuality == "" {
		videoQuality = liveavatar.QualityHigh
	}

	// Generate a unique avatar identity
	avatarIdentity := fmt.Sprintf("heygen-avatar-%s", uuid.New().String()[:8])

	return &Session{
		BaseSession:       avatar.NewBaseSession("heygen", avatarIdentity),
		client:            client,
		avatarID:          cfg.AvatarID,
		sandbox:           cfg.Sandbox,
		videoQuality:      videoQuality,
		audioConfig:       audioConfig,
		participantJoined: make(chan struct{}),
		participantLeft:   make(chan struct{}),
	}, nil
}

// Start initializes the HeyGen LiveAvatar session.
//
// This method:
//  1. Validates the start options
//  2. Generates a LiveKit token for the avatar
//  3. Creates a session with the LiveAvatar API (LITE mode)
//  4. Starts the session
//  5. Sets up the audio output for streaming
func (s *Session) Start(ctx context.Context, opts avatar.StartOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	if s.IsStarted() {
		s.mu.Unlock()
		return avatar.ErrSessionAlreadyStarted
	}
	s.mu.Unlock()

	// Store room reference
	s.SetRoom(opts.Room)
	s.SetCallbacks(opts.Callbacks)

	// Generate token for avatar to join
	token, err := avatar.GenerateAvatarToken(avatar.TokenOptions{
		APIKey:          opts.LiveKitAPIKey,
		APISecret:       opts.LiveKitAPISecret,
		RoomName:        opts.Room.Name(),
		AvatarIdentity:  s.AvatarIdentity(),
		AvatarName:      "HeyGen Avatar",
		PublishOnBehalf: opts.AgentIdentity,
		TTL:             10 * time.Minute,
		Metadata:        s.buildMetadata(opts.AgentIdentity),
	})
	if err != nil {
		return fmt.Errorf("failed to generate avatar token: %w", err)
	}

	// Create session with LiveAvatar (LITE mode)
	sessionResp, err := s.client.NewSession(ctx, &liveavatar.NewSessionRequest{
		Mode:         "LITE",
		AvatarID:     s.avatarID,
		IsSandbox:    s.sandbox,
		VideoQuality: s.videoQuality,
		LiveKitConfig: &liveavatar.LiveKitConfig{
			LiveKitURL:         opts.LiveKitURL,
			LiveKitRoom:        opts.Room.Name(),
			LiveKitClientToken: token,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create LiveAvatar session: %w", err)
	}

	s.mu.Lock()
	s.sessionID = sessionResp.SessionID
	s.sessionToken = sessionResp.SessionToken
	s.mu.Unlock()

	// Start the session
	_, err = s.client.StartSession(ctx, sessionResp.SessionID, sessionResp.SessionToken)
	if err != nil {
		return fmt.Errorf("failed to start LiveAvatar session: %w", err)
	}

	// Create audio output via DataStream
	audioOut, err := avatar.NewDataStreamAudioOutput(avatar.DataStreamConfig{
		Room:                opts.Room,
		DestinationIdentity: s.AvatarIdentity(),
		Audio:               s.audioConfig,
	})
	if err != nil {
		// Clean up session on failure
		_ = s.client.StopSession(ctx, s.sessionID, s.sessionToken, liveavatar.StopReasonSessionEnded)
		return fmt.Errorf("failed to create audio output: %w", err)
	}

	// Wire up playback callbacks
	audioOut.OnPlayback(func(event avatar.PlaybackEvent) {
		switch event.Type {
		case avatar.PlaybackStarted:
			s.EmitPlaybackStarted()
		case avatar.PlaybackFinished:
			s.EmitPlaybackFinished(event.Position, event.Interrupted)
		}
	})

	s.SetAudioOutput(audioOut)
	s.MarkStarted()

	return nil
}

// WaitForJoin blocks until the avatar participant joins the room.
func (s *Session) WaitForJoin(ctx context.Context, timeout time.Duration) error {
	if !s.IsStarted() {
		return avatar.ErrSessionNotStarted
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	room := s.Room()
	if room == nil {
		return avatar.ErrSessionNotStarted
	}

	// Poll for participant join
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		// Check if avatar has joined
		if p := room.GetParticipantByIdentity(s.AvatarIdentity()); p != nil {
			s.emitJoinMetrics()

			// Notify via callback if set
			if callbacks := s.Callbacks(); callbacks != nil && callbacks.OnAvatarJoined != nil {
				callbacks.OnAvatarJoined(p.Identity())
			}

			// Signal join for any waiters
			select {
			case <-s.participantJoined:
				// Already closed
			default:
				close(s.participantJoined)
			}

			return nil
		}

		select {
		case <-ticker.C:
			// Continue polling
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return avatar.ErrAvatarJoinTimeout
			}
			return ctx.Err()
		}
	}
}

// Close disconnects the avatar and cleans up resources.
func (s *Session) Close(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	sessionID := s.sessionID
	sessionToken := s.sessionToken
	s.mu.Unlock()

	var errs []error

	// Close audio output
	if audioOut := s.AudioOutput(); audioOut != nil {
		if err := audioOut.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close audio output: %w", err))
		}
	}

	// Stop session with LiveAvatar
	if sessionID != "" && sessionToken != "" {
		if err := s.client.StopSession(ctx, sessionID, sessionToken, liveavatar.StopReasonSessionEnded); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop LiveAvatar session: %w", err))
		}
	}

	// Close channels
	select {
	case <-s.participantLeft:
		// Already closed
	default:
		close(s.participantLeft)
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// buildMetadata creates the avatar participant metadata.
func (s *Session) buildMetadata(agentIdentity string) string {
	meta := avatar.DefaultAvatarMetadata("heygen", agentIdentity)
	data, _ := json.Marshal(meta)
	return string(data)
}

// emitJoinMetrics emits metrics about avatar join latency.
func (s *Session) emitJoinMetrics() {
	s.EmitMetrics(avatar.Metrics{
		Provider:          "heygen",
		AvatarJoinLatency: time.Since(s.StartTime()),
		Timestamp:         time.Now(),
	})
}

// SessionID returns the LiveAvatar session ID.
func (s *Session) SessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessionID
}

// Verify interface compliance at compile time.
var _ avatar.Session = (*Session)(nil)
