package avatar

import (
	"context"
	"time"

	lksdk "github.com/livekit/server-sdk-go/v2"

	"github.com/plexusone/omniavatar"
	avcore "github.com/plexusone/omniavatar-core/avatar"
)

// LiveKitSession wraps an omniavatar Session with LiveKit-specific functionality.
//
// This adapter:
//   - Creates DataStreamAudioOutput for audio streaming
//   - Wires up playback callbacks to the underlying session
//   - Provides a unified interface for LiveKit agent integration
type LiveKitSession struct {
	// Underlying session from omniavatar
	session avcore.Session

	// LiveKit-specific state
	room        *lksdk.Room
	audioOutput *DataStreamAudioOutput

	// Callbacks
	callbacks *SessionCallbacks
}

// WrapSession wraps an omniavatar Session with LiveKit-specific functionality.
//
// The session's Start() method expects *omniavatar.LiveKitStartOptions.
// The adapter handles DataStreamAudioOutput creation and callback wiring.
func WrapSession(session avcore.Session) *LiveKitSession {
	return &LiveKitSession{
		session: session,
	}
}

// Identity returns the avatar participant identity.
func (s *LiveKitSession) Identity() string {
	return s.session.Identity()
}

// AvatarIdentity returns the avatar participant identity.
// Alias for Identity() for backwards compatibility.
func (s *LiveKitSession) AvatarIdentity() string {
	return s.session.Identity()
}

// Provider returns the provider name.
func (s *LiveKitSession) Provider() string {
	return s.session.Provider()
}

// Start initializes the session with LiveKit-specific setup.
//
// This method:
//  1. Creates DataStreamAudioOutput for audio streaming
//  2. Wires up playback callbacks
//  3. Calls the underlying session's Start()
func (s *LiveKitSession) Start(ctx context.Context, opts StartOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	s.room = opts.Room
	s.callbacks = opts.Callbacks

	// Create DataStream audio output
	audioOut, err := NewDataStreamAudioOutput(DataStreamConfig{
		Room:                opts.Room,
		DestinationIdentity: s.session.Identity(),
		Audio:               DefaultAudioConfig(),
	})
	if err != nil {
		return err
	}
	s.audioOutput = audioOut

	// Wire up playback callbacks to emit through our callbacks
	// and the underlying session's emit methods
	audioOut.OnPlayback(func(event PlaybackEvent) {
		switch event.Type {
		case PlaybackStarted:
			if s.callbacks != nil && s.callbacks.OnPlaybackStarted != nil {
				s.callbacks.OnPlaybackStarted()
			}
			// Also emit through the underlying session for its internal callbacks
			if emitter, ok := s.session.(interface{ EmitPlaybackStarted() }); ok {
				emitter.EmitPlaybackStarted()
			}
		case PlaybackFinished:
			if s.callbacks != nil && s.callbacks.OnPlaybackFinished != nil {
				s.callbacks.OnPlaybackFinished(event.Position, event.Interrupted)
			}
			if emitter, ok := s.session.(interface {
				EmitPlaybackFinished(float64, bool)
			}); ok {
				emitter.EmitPlaybackFinished(event.Position, event.Interrupted)
			}
		}
	})

	// Convert to omniavatar.LiveKitStartOptions
	lkOpts := &omniavatar.LiveKitStartOptions{
		Room:             opts.Room,
		AgentIdentity:    opts.AgentIdentity,
		LiveKitURL:       opts.LiveKitURL,
		LiveKitAPIKey:    opts.LiveKitAPIKey,
		LiveKitAPISecret: opts.LiveKitAPISecret,
		AudioDestination: audioOut,
		Callbacks: &avcore.SessionCallbacks{
			OnAvatarJoined: func(identity string) {
				if s.callbacks != nil && s.callbacks.OnAvatarJoined != nil {
					s.callbacks.OnAvatarJoined(identity)
				}
			},
			OnAvatarLeft: func(identity string) {
				if s.callbacks != nil && s.callbacks.OnAvatarLeft != nil {
					s.callbacks.OnAvatarLeft(identity)
				}
			},
			OnPlaybackStarted: func() {
				// Already handled via audio output callbacks
			},
			OnPlaybackFinished: func(position float64, interrupted bool) {
				// Already handled via audio output callbacks
			},
			OnError: func(err error) {
				if s.callbacks != nil && s.callbacks.OnError != nil {
					s.callbacks.OnError(err)
				}
			},
		},
	}

	// Start underlying session
	return s.session.Start(ctx, lkOpts)
}

// WaitForJoin blocks until the avatar participant joins the room.
func (s *LiveKitSession) WaitForJoin(ctx context.Context, timeout time.Duration) error {
	return s.session.WaitForJoin(ctx, timeout)
}

// AudioOutput returns the audio destination for streaming TTS audio.
func (s *LiveKitSession) AudioOutput() AudioDestination {
	return s.audioOutput
}

// Close disconnects the avatar and cleans up resources.
func (s *LiveKitSession) Close(ctx context.Context) error {
	// Close audio output first
	if s.audioOutput != nil {
		_ = s.audioOutput.Close()
	}
	// Then close the underlying session
	return s.session.Close(ctx)
}

// Session returns the underlying omniavatar session.
func (s *LiveKitSession) Session() avcore.Session {
	return s.session
}

// Room returns the LiveKit room reference.
func (s *LiveKitSession) Room() *lksdk.Room {
	return s.room
}

// Verify interface compliance at compile time.
var _ Session = (*LiveKitSession)(nil)
