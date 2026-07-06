package tavus

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/plexusone/omni-livekit/avatar"
)

func TestNewSession(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		session, err := NewSession(SessionConfig{
			APIKey: "test-api-key",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if session == nil {
			t.Fatal("expected non-nil session")
		}
		if session.Provider() != "tavus" {
			t.Errorf("expected provider tavus, got %s", session.Provider())
		}
		if !strings.HasPrefix(session.AvatarIdentity(), "tavus-avatar-") {
			t.Errorf("expected avatar identity to start with tavus-avatar-, got %s", session.AvatarIdentity())
		}
	})

	t.Run("missing API key", func(t *testing.T) {
		_, err := NewSession(SessionConfig{})
		if err != avatar.ErrInvalidConfig {
			t.Errorf("expected ErrInvalidConfig, got %v", err)
		}
	})

	t.Run("custom PAL ID", func(t *testing.T) {
		session, err := NewSession(SessionConfig{
			APIKey: "test-api-key",
			PalID:  "custom-pal-id",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if session.palID != "custom-pal-id" {
			t.Errorf("expected custom-pal-id, got %s", session.palID)
		}
	})

	t.Run("default PAL ID", func(t *testing.T) {
		session, err := NewSession(SessionConfig{
			APIKey: "test-api-key",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if session.palID != DefaultPalID {
			t.Errorf("expected default PAL ID %s, got %s", DefaultPalID, session.palID)
		}
	})

	t.Run("default audio config", func(t *testing.T) {
		session, err := NewSession(SessionConfig{
			APIKey: "test-api-key",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if session.audioConfig.SampleRate != 24000 {
			t.Errorf("expected 24000 sample rate, got %d", session.audioConfig.SampleRate)
		}
		if session.audioConfig.Channels != 1 {
			t.Errorf("expected 1 channel, got %d", session.audioConfig.Channels)
		}
	})

	t.Run("custom audio config", func(t *testing.T) {
		session, err := NewSession(SessionConfig{
			APIKey: "test-api-key",
			AudioConfig: avatar.AudioConfig{
				SampleRate: 48000,
				Channels:   2,
				Encoding:   "linear16",
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if session.audioConfig.SampleRate != 48000 {
			t.Errorf("expected 48000 sample rate, got %d", session.audioConfig.SampleRate)
		}
	})
}

func TestSessionStartValidation(t *testing.T) {
	session, _ := NewSession(SessionConfig{
		APIKey: "test-api-key",
	})

	// Start without room should fail
	err := session.Start(context.Background(), avatar.StartOptions{})
	if err != avatar.ErrInvalidConfig {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestSessionNotStarted(t *testing.T) {
	session, _ := NewSession(SessionConfig{
		APIKey: "test-api-key",
	})

	// WaitForJoin without Start should fail
	err := session.WaitForJoin(context.Background(), time.Second)
	if err != avatar.ErrSessionNotStarted {
		t.Errorf("expected ErrSessionNotStarted, got %v", err)
	}
}

func TestSessionClose(t *testing.T) {
	session, _ := NewSession(SessionConfig{
		APIKey: "test-api-key",
	})

	// Close without conversation ID should succeed (nothing to cleanup)
	err := session.Close(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Double close should be safe
	err = session.Close(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on double close: %v", err)
	}
}

func TestSessionConversationID(t *testing.T) {
	session, _ := NewSession(SessionConfig{
		APIKey: "test-api-key",
	})

	// Initially empty
	if session.ConversationID() != "" {
		t.Errorf("expected empty conversation ID, got %s", session.ConversationID())
	}

	// Set and verify
	session.conversationID = "test-conv-id"
	if session.ConversationID() != "test-conv-id" {
		t.Errorf("expected test-conv-id, got %s", session.ConversationID())
	}
}

func TestSessionBuildMetadata(t *testing.T) {
	session, _ := NewSession(SessionConfig{
		APIKey: "test-api-key",
	})

	metadata := session.buildMetadata("agent-123")

	var meta avatar.AvatarMetadata
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}

	if meta.Kind != "avatar" {
		t.Errorf("expected kind avatar, got %s", meta.Kind)
	}
	if meta.Provider != "tavus" {
		t.Errorf("expected provider tavus, got %s", meta.Provider)
	}
	if meta.AgentIdentity != "agent-123" {
		t.Errorf("expected agent identity agent-123, got %s", meta.AgentIdentity)
	}
}

func TestSessionCallbacks(t *testing.T) {
	session, _ := NewSession(SessionConfig{
		APIKey: "test-api-key",
	})

	playbackStarted := false
	playbackFinished := false
	var finishedPosition float64
	var finishedInterrupted bool

	session.SetCallbacks(&avatar.SessionCallbacks{
		OnPlaybackStarted: func() {
			playbackStarted = true
		},
		OnPlaybackFinished: func(pos float64, interrupted bool) {
			playbackFinished = true
			finishedPosition = pos
			finishedInterrupted = interrupted
		},
	})

	// Emit playback events
	session.EmitPlaybackStarted()
	if !playbackStarted {
		t.Error("expected OnPlaybackStarted to be called")
	}

	session.EmitPlaybackFinished(1.5, true)
	if !playbackFinished {
		t.Error("expected OnPlaybackFinished to be called")
	}
	if finishedPosition != 1.5 {
		t.Errorf("expected position 1.5, got %f", finishedPosition)
	}
	if !finishedInterrupted {
		t.Error("expected interrupted true")
	}
}

func TestSessionMetrics(t *testing.T) {
	session, _ := NewSession(SessionConfig{
		APIKey: "test-api-key",
	})

	var receivedMetrics avatar.Metrics
	session.SetCallbacks(&avatar.SessionCallbacks{
		OnMetricsCollected: func(m avatar.Metrics) {
			receivedMetrics = m
		},
	})

	session.EmitMetrics(avatar.Metrics{
		Provider:          "tavus",
		AvatarJoinLatency: 500 * time.Millisecond,
	})

	if receivedMetrics.Provider != "tavus" {
		t.Errorf("expected provider tavus, got %s", receivedMetrics.Provider)
	}
	if receivedMetrics.AvatarJoinLatency != 500*time.Millisecond {
		t.Errorf("expected 500ms latency, got %v", receivedMetrics.AvatarJoinLatency)
	}
}

func TestSessionInterface(t *testing.T) {
	// Verify Session implements avatar.Session interface
	var _ avatar.Session = (*Session)(nil)
}
