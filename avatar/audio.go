package avatar

import "context"

// AudioDestination receives audio frames and forwards them to an avatar.
//
// Implementations include:
//   - DataStreamAudioOutput: Streams audio to a remote avatar via LiveKit ByteStream
//   - QueueAudioOutput: In-memory queue for testing
//
// The typical usage flow is:
//  1. Call CaptureFrame() for each PCM audio frame from TTS
//  2. Call Flush() when the utterance is complete
//  3. Call ClearBuffer() to interrupt playback (e.g., user interruption)
type AudioDestination interface {
	// CaptureFrame sends a PCM16 audio frame to the avatar.
	//
	// The frame should be little-endian PCM16 at the configured sample rate.
	// Frames are typically 20ms of audio (e.g., 960 samples at 48kHz).
	//
	// This method may block if the underlying transport is congested.
	CaptureFrame(ctx context.Context, frame []byte) error

	// Flush marks the end of an audio segment (utterance).
	//
	// The avatar should speak all buffered audio before accepting more.
	// This closes the current ByteStream and prepares for the next utterance.
	Flush(ctx context.Context) error

	// ClearBuffer interrupts current playback.
	//
	// Use this when the user interrupts the agent. The avatar should
	// stop speaking immediately and discard any buffered audio.
	//
	// This sends an RPC to the avatar worker and waits for acknowledgment.
	ClearBuffer(ctx context.Context) error

	// SampleRate returns the expected input sample rate in Hz.
	// Common values: 16000, 24000, 48000
	SampleRate() int

	// Channels returns the expected number of audio channels.
	// Typically 1 (mono).
	Channels() int

	// OnPlayback registers a callback for playback events.
	// Multiple callbacks can be registered.
	OnPlayback(callback PlaybackCallback)

	// Close releases resources associated with this audio destination.
	// After Close(), no more frames should be sent.
	Close() error
}

// PlaybackCallback is called when playback events occur.
type PlaybackCallback func(event PlaybackEvent)

// PlaybackEvent represents a playback state change from the avatar.
type PlaybackEvent struct {
	// Type is the type of playback event.
	Type PlaybackEventType

	// Position is the playback position in seconds when the event occurred.
	// Only meaningful for PlaybackFinished events.
	Position float64

	// Interrupted indicates if playback was interrupted by ClearBuffer().
	// Only meaningful for PlaybackFinished events.
	Interrupted bool
}

// PlaybackEventType identifies the type of playback event.
type PlaybackEventType string

const (
	// PlaybackStarted indicates the avatar started speaking.
	PlaybackStarted PlaybackEventType = "started"

	// PlaybackFinished indicates the avatar finished speaking.
	// Check Position and Interrupted for details.
	PlaybackFinished PlaybackEventType = "finished"
)

// AudioConfig holds audio configuration for avatar sessions.
type AudioConfig struct {
	// SampleRate is the audio sample rate in Hz.
	// Default: 24000 (common for avatar providers)
	SampleRate int

	// Channels is the number of audio channels.
	// Default: 1 (mono)
	Channels int

	// Encoding is the audio encoding format.
	// Default: "linear16" (PCM16 little-endian)
	Encoding string
}

// DefaultAudioConfig returns the default audio configuration.
func DefaultAudioConfig() AudioConfig {
	return AudioConfig{
		SampleRate: 24000,
		Channels:   1,
		Encoding:   "linear16",
	}
}

// FrameSize returns the number of bytes per frame for the given duration in ms.
func (c AudioConfig) FrameSize(durationMs int) int {
	samplesPerFrame := c.SampleRate * durationMs / 1000
	bytesPerSample := 2 // PCM16
	return samplesPerFrame * bytesPerSample * c.Channels
}

// FrameDuration returns the duration in milliseconds for the given frame size.
func (c AudioConfig) FrameDuration(frameSize int) int {
	bytesPerSample := 2 // PCM16
	samples := frameSize / (bytesPerSample * c.Channels)
	return samples * 1000 / c.SampleRate
}
