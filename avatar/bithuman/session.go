package bithuman

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/plexusone/omni-livekit/avatar"
)

// SessionConfig configures a bitHuman avatar session.
type SessionConfig struct {
	// APIKey is the bitHuman API key.
	// Required.
	APIKey string

	// BaseURL is the bitHuman API base URL.
	// Default: https://api.bithuman.ai
	BaseURL string

	// AgentID is the bitHuman agent to use for this session.
	// Required.
	AgentID string

	// AudioConfig configures the audio format.
	// Default: 24kHz mono PCM16
	AudioConfig avatar.AudioConfig
}

// Session implements avatar.Session for bitHuman avatars.
type Session struct {
	*avatar.BaseSession

	client      *Client
	agentID     string
	audioConfig avatar.AudioConfig
	sessionID   string

	// Participant tracking
	participantJoined chan struct{}
	participantLeft   chan struct{}

	mu     sync.Mutex
	closed bool
}

// NewSession creates a new bitHuman avatar session.
func NewSession(cfg SessionConfig) (*Session, error) {
	if cfg.APIKey == "" {
		return nil, avatar.ErrInvalidConfig
	}
	if cfg.AgentID == "" {
		return nil, avatar.ErrInvalidConfig
	}

	client, err := NewClient(ClientConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
	})
	if err != nil {
		return nil, err
	}

	audioConfig := cfg.AudioConfig
	if audioConfig.SampleRate == 0 {
		// bitHuman expects 24kHz audio
		audioConfig = avatar.AudioConfig{
			SampleRate: 24000,
			Channels:   1,
			Encoding:   "linear16",
		}
	}

	// Generate a unique avatar identity
	avatarIdentity := fmt.Sprintf("bithuman-avatar-%s", uuid.New().String()[:8])

	return &Session{
		BaseSession:       avatar.NewBaseSession("bithuman", avatarIdentity),
		client:            client,
		agentID:           cfg.AgentID,
		audioConfig:       audioConfig,
		participantJoined: make(chan struct{}),
		participantLeft:   make(chan struct{}),
	}, nil
}

// Start initializes the bitHuman avatar session.
//
// This method:
//  1. Validates the start options
//  2. Generates a LiveKit token for the avatar
//  3. Creates a session with the bitHuman API
//  4. Sets up the audio output for streaming
//  5. Registers room callbacks to track the avatar participant
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
		AvatarName:      "bitHuman Avatar",
		PublishOnBehalf: opts.AgentIdentity,
		TTL:             10 * time.Minute,
		Metadata:        s.buildMetadata(opts.AgentIdentity),
	})
	if err != nil {
		return fmt.Errorf("failed to generate avatar token: %w", err)
	}

	// Create session with bitHuman
	sessionResp, err := s.client.CreateSession(ctx, CreateSessionRequest{
		AgentID:      s.agentID,
		LiveKitURL:   opts.LiveKitURL,
		LiveKitToken: token,
	})
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	s.mu.Lock()
	s.sessionID = sessionResp.SessionID
	s.mu.Unlock()

	// Create audio output
	audioOut, err := avatar.NewDataStreamAudioOutput(avatar.DataStreamConfig{
		Room:                opts.Room,
		DestinationIdentity: s.AvatarIdentity(),
		Audio:               s.audioConfig,
	})
	if err != nil {
		// Clean up session on failure
		_ = s.client.EndSession(ctx, sessionResp.SessionID)
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
	s.mu.Unlock()

	var errs []error

	// Close audio output
	if audioOut := s.AudioOutput(); audioOut != nil {
		if err := audioOut.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close audio output: %w", err))
		}
	}

	// End session with bitHuman
	if sessionID != "" {
		if err := s.client.EndSession(ctx, sessionID); err != nil {
			errs = append(errs, fmt.Errorf("failed to end session: %w", err))
		}
	}

	// Close channels
	close(s.participantLeft)

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// buildMetadata creates the avatar participant metadata.
func (s *Session) buildMetadata(agentIdentity string) string {
	meta := avatar.DefaultAvatarMetadata("bithuman", agentIdentity)
	data, _ := json.Marshal(meta)
	return string(data)
}

// emitJoinMetrics emits metrics about avatar join latency.
func (s *Session) emitJoinMetrics() {
	s.EmitMetrics(avatar.Metrics{
		Provider:          "bithuman",
		AvatarJoinLatency: time.Since(s.StartTime()),
		Timestamp:         time.Now(),
	})
}

// SessionID returns the bitHuman session ID.
func (s *Session) SessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessionID
}

// AgentID returns the bitHuman agent ID.
func (s *Session) AgentID() string {
	return s.agentID
}

// Verify interface compliance at compile time.
var _ avatar.Session = (*Session)(nil)
