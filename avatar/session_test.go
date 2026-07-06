package avatar

import (
	"testing"
	"time"
)

func TestStartOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		opts    StartOptions
		wantErr bool
	}{
		{
			name:    "all nil",
			opts:    StartOptions{},
			wantErr: true,
		},
		{
			name: "missing AgentIdentity",
			opts: StartOptions{
				LiveKitURL:       "wss://example.livekit.cloud",
				LiveKitAPIKey:    "key",
				LiveKitAPISecret: "secret",
			},
			wantErr: true,
		},
		{
			name: "missing LiveKitURL",
			opts: StartOptions{
				AgentIdentity:    "agent",
				LiveKitAPIKey:    "key",
				LiveKitAPISecret: "secret",
			},
			wantErr: true,
		},
		{
			name: "missing LiveKitAPIKey",
			opts: StartOptions{
				AgentIdentity:    "agent",
				LiveKitURL:       "wss://example.livekit.cloud",
				LiveKitAPISecret: "secret",
			},
			wantErr: true,
		},
		{
			name: "missing LiveKitAPISecret",
			opts: StartOptions{
				AgentIdentity: "agent",
				LiveKitURL:    "wss://example.livekit.cloud",
				LiveKitAPIKey: "key",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewBaseSession(t *testing.T) {
	bs := NewBaseSession("tavus", "avatar-123")

	if bs.Provider() != "tavus" {
		t.Errorf("expected Provider 'tavus', got %s", bs.Provider())
	}
	if bs.AvatarIdentity() != "avatar-123" {
		t.Errorf("expected AvatarIdentity 'avatar-123', got %s", bs.AvatarIdentity())
	}
	if bs.IsStarted() {
		t.Error("expected IsStarted false initially")
	}
	if bs.AudioOutput() != nil {
		t.Error("expected AudioOutput nil initially")
	}
	if bs.Room() != nil {
		t.Error("expected Room nil initially")
	}
	if bs.Callbacks() != nil {
		t.Error("expected Callbacks nil initially")
	}
}

func TestBaseSessionSetters(t *testing.T) {
	bs := NewBaseSession("anam", "avatar-456")

	// Set audio output
	queue := NewQueueAudioOutput(DefaultAudioConfig())
	bs.SetAudioOutput(queue)
	if bs.AudioOutput() != queue {
		t.Error("SetAudioOutput did not work correctly")
	}

	// Set callbacks
	callbacks := &SessionCallbacks{
		OnPlaybackStarted: func() {},
	}
	bs.SetCallbacks(callbacks)
	if bs.Callbacks() != callbacks {
		t.Error("SetCallbacks did not work correctly")
	}
}

func TestBaseSessionMarkStarted(t *testing.T) {
	bs := NewBaseSession("simli", "avatar-789")

	before := time.Now()
	bs.MarkStarted()
	after := time.Now()

	if !bs.IsStarted() {
		t.Error("expected IsStarted true after MarkStarted")
	}

	startTime := bs.StartTime()
	if startTime.Before(before) || startTime.After(after) {
		t.Errorf("StartTime %v not within expected range [%v, %v]", startTime, before, after)
	}
}

func TestBaseSessionEmitMetrics(t *testing.T) {
	bs := NewBaseSession("tavus", "avatar-123")

	// Without callbacks, should not panic
	bs.EmitMetrics(Metrics{Provider: "tavus"})

	// With callback
	var receivedMetrics Metrics
	bs.SetCallbacks(&SessionCallbacks{
		OnMetricsCollected: func(m Metrics) {
			receivedMetrics = m
		},
	})

	bs.EmitMetrics(Metrics{
		Provider:          "tavus",
		AvatarJoinLatency: 500 * time.Millisecond,
	})

	if receivedMetrics.Provider != "tavus" {
		t.Errorf("expected Provider 'tavus', got %s", receivedMetrics.Provider)
	}
	if receivedMetrics.AvatarJoinLatency != 500*time.Millisecond {
		t.Errorf("expected AvatarJoinLatency 500ms, got %v", receivedMetrics.AvatarJoinLatency)
	}
}

func TestBaseSessionEmitPlaybackStarted(t *testing.T) {
	bs := NewBaseSession("anam", "avatar-456")

	// Without callbacks, should not panic
	bs.EmitPlaybackStarted()

	// With callback
	called := false
	bs.SetCallbacks(&SessionCallbacks{
		OnPlaybackStarted: func() {
			called = true
		},
	})

	bs.EmitPlaybackStarted()

	if !called {
		t.Error("expected OnPlaybackStarted to be called")
	}
}

func TestBaseSessionEmitPlaybackFinished(t *testing.T) {
	bs := NewBaseSession("simli", "avatar-789")

	// Without callbacks, should not panic
	bs.EmitPlaybackFinished(1.5, true)

	// With callback
	var gotPosition float64
	var gotInterrupted bool
	bs.SetCallbacks(&SessionCallbacks{
		OnPlaybackFinished: func(pos float64, interrupted bool) {
			gotPosition = pos
			gotInterrupted = interrupted
		},
	})

	bs.EmitPlaybackFinished(2.5, false)

	if gotPosition != 2.5 {
		t.Errorf("expected position 2.5, got %f", gotPosition)
	}
	if gotInterrupted {
		t.Error("expected interrupted false")
	}
}

func TestBaseSessionEmitError(t *testing.T) {
	bs := NewBaseSession("tavus", "avatar-123")

	// Without callbacks, should not panic
	bs.EmitError(ErrProviderUnavailable)

	// With callback
	var gotErr error
	bs.SetCallbacks(&SessionCallbacks{
		OnError: func(err error) {
			gotErr = err
		},
	})

	bs.EmitError(ErrRPCTimeout)

	if gotErr != ErrRPCTimeout {
		t.Errorf("expected ErrRPCTimeout, got %v", gotErr)
	}
}
