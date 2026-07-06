package avatar

import (
	"context"
	"time"

	lksdk "github.com/livekit/server-sdk-go/v2"
)

// Session manages a lip-sync avatar that publishes video to the room.
//
// Implementations of this interface handle provider-specific API calls
// to create avatar sessions and manage their lifecycle.
//
// The typical flow is:
//  1. Create a Session with provider-specific configuration
//  2. Call Start() to initialize the avatar and connect it to the room
//  3. Call WaitForJoin() to wait for the avatar to be ready
//  4. Use AudioOutput() to stream TTS audio to the avatar
//  5. Call Close() when done to clean up resources
type Session interface {
	// AvatarIdentity returns the participant identity of the avatar worker.
	// This is the identity that appears in the room and publishes video.
	AvatarIdentity() string

	// Provider returns the provider name (e.g., "tavus", "anam", "simli").
	Provider() string

	// Start initializes the avatar session.
	//
	// This method should:
	//  1. Create a session with the avatar provider's API
	//  2. Generate a LiveKit token for the avatar to join the room
	//  3. Configure the audio output to stream to the avatar
	//
	// The avatar will join the room asynchronously. Use WaitForJoin()
	// to wait for the avatar to be ready.
	Start(ctx context.Context, opts StartOptions) error

	// WaitForJoin blocks until the avatar participant joins the room
	// and publishes the expected tracks (typically video).
	//
	// Returns an error if the timeout is exceeded or the context is cancelled.
	WaitForJoin(ctx context.Context, timeout time.Duration) error

	// AudioOutput returns the audio destination for streaming TTS audio
	// to the avatar. Returns nil if the session is not started.
	AudioOutput() AudioDestination

	// Close disconnects the avatar and cleans up resources.
	//
	// This method should:
	//  1. End the session with the avatar provider
	//  2. Remove the avatar participant from the room
	//  3. Clean up any registered RPC handlers
	Close(ctx context.Context) error
}

// StartOptions configures avatar session startup.
type StartOptions struct {
	// Room is the LiveKit room the agent has joined.
	// Required.
	Room *lksdk.Room

	// AgentIdentity is the identity of the agent participant.
	// The avatar will publish tracks on behalf of this identity
	// using the lk.publish_on_behalf attribute.
	// Required.
	AgentIdentity string

	// LiveKitURL is the LiveKit server URL for the avatar to connect to.
	// This should match the URL the agent connected to.
	// Required.
	LiveKitURL string

	// LiveKitAPIKey is used to generate tokens for the avatar.
	// Required.
	LiveKitAPIKey string

	// LiveKitAPISecret is used to generate tokens for the avatar.
	// Required.
	LiveKitAPISecret string

	// Callbacks configures optional event callbacks.
	Callbacks *SessionCallbacks
}

// Validate checks that all required fields are set.
func (o *StartOptions) Validate() error {
	if o.Room == nil {
		return ErrInvalidConfig
	}
	if o.AgentIdentity == "" {
		return ErrInvalidConfig
	}
	if o.LiveKitURL == "" {
		return ErrInvalidConfig
	}
	if o.LiveKitAPIKey == "" {
		return ErrInvalidConfig
	}
	if o.LiveKitAPISecret == "" {
		return ErrInvalidConfig
	}
	return nil
}

// SessionCallbacks defines optional event callbacks for avatar sessions.
type SessionCallbacks struct {
	// OnMetricsCollected is called when avatar metrics are available.
	OnMetricsCollected func(metrics Metrics)

	// OnPlaybackStarted is called when the avatar starts speaking.
	OnPlaybackStarted func()

	// OnPlaybackFinished is called when the avatar finishes speaking.
	// The position is the playback position in seconds when stopped.
	// The interrupted flag indicates if playback was interrupted by
	// a ClearBuffer() call.
	OnPlaybackFinished func(position float64, interrupted bool)

	// OnAvatarJoined is called when the avatar participant joins the room.
	OnAvatarJoined func(identity string)

	// OnAvatarLeft is called when the avatar participant leaves the room.
	OnAvatarLeft func(identity string)

	// OnError is called when an error occurs.
	// This is for non-fatal errors that don't cause the session to fail.
	OnError func(err error)
}

// Metrics contains avatar performance metrics.
type Metrics struct {
	// AvatarJoinLatency is the time from Start() to avatar joining the room.
	AvatarJoinLatency time.Duration

	// PlaybackLatency is the time from audio send to avatar speech start.
	// This is measured per utterance.
	PlaybackLatency time.Duration

	// Provider is the avatar provider name.
	Provider string

	// Timestamp is when the metrics were collected.
	Timestamp time.Time
}

// BaseSession provides common functionality for avatar session implementations.
// Provider-specific sessions can embed this struct.
type BaseSession struct {
	avatarIdentity string
	provider       string
	room           *lksdk.Room
	audioOutput    AudioDestination
	callbacks      *SessionCallbacks
	started        bool
	startTime      time.Time
}

// NewBaseSession creates a new BaseSession.
func NewBaseSession(provider, avatarIdentity string) *BaseSession {
	return &BaseSession{
		provider:       provider,
		avatarIdentity: avatarIdentity,
	}
}

// AvatarIdentity returns the avatar participant identity.
func (s *BaseSession) AvatarIdentity() string {
	return s.avatarIdentity
}

// Provider returns the provider name.
func (s *BaseSession) Provider() string {
	return s.provider
}

// AudioOutput returns the audio destination.
func (s *BaseSession) AudioOutput() AudioDestination {
	return s.audioOutput
}

// SetAudioOutput sets the audio destination.
func (s *BaseSession) SetAudioOutput(out AudioDestination) {
	s.audioOutput = out
}

// SetRoom sets the room reference.
func (s *BaseSession) SetRoom(room *lksdk.Room) {
	s.room = room
}

// Room returns the room reference.
func (s *BaseSession) Room() *lksdk.Room {
	return s.room
}

// SetCallbacks sets the session callbacks.
func (s *BaseSession) SetCallbacks(callbacks *SessionCallbacks) {
	s.callbacks = callbacks
}

// Callbacks returns the session callbacks.
func (s *BaseSession) Callbacks() *SessionCallbacks {
	return s.callbacks
}

// MarkStarted marks the session as started.
func (s *BaseSession) MarkStarted() {
	s.started = true
	s.startTime = time.Now()
}

// IsStarted returns true if the session has been started.
func (s *BaseSession) IsStarted() bool {
	return s.started
}

// StartTime returns when the session was started.
func (s *BaseSession) StartTime() time.Time {
	return s.startTime
}

// EmitMetrics emits metrics if a callback is registered.
func (s *BaseSession) EmitMetrics(metrics Metrics) {
	if s.callbacks != nil && s.callbacks.OnMetricsCollected != nil {
		s.callbacks.OnMetricsCollected(metrics)
	}
}

// EmitPlaybackStarted emits a playback started event if a callback is registered.
func (s *BaseSession) EmitPlaybackStarted() {
	if s.callbacks != nil && s.callbacks.OnPlaybackStarted != nil {
		s.callbacks.OnPlaybackStarted()
	}
}

// EmitPlaybackFinished emits a playback finished event if a callback is registered.
func (s *BaseSession) EmitPlaybackFinished(position float64, interrupted bool) {
	if s.callbacks != nil && s.callbacks.OnPlaybackFinished != nil {
		s.callbacks.OnPlaybackFinished(position, interrupted)
	}
}

// EmitError emits an error event if a callback is registered.
func (s *BaseSession) EmitError(err error) {
	if s.callbacks != nil && s.callbacks.OnError != nil {
		s.callbacks.OnError(err)
	}
}
