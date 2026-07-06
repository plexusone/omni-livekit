package avatar

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	lksdk "github.com/livekit/server-sdk-go/v2"
)

// RPC method names for playback control.
// These must match the methods implemented by avatar workers.
const (
	// RPCPlaybackStarted is sent by the avatar when it starts speaking.
	RPCPlaybackStarted = "lk.playback_started"

	// RPCPlaybackFinished is sent by the avatar when it finishes speaking.
	RPCPlaybackFinished = "lk.playback_finished"

	// RPCClearBuffer is sent by the agent to interrupt playback.
	RPCClearBuffer = "lk.clear_buffer"
)

// AudioStreamTopic is the ByteStream topic for audio data.
const AudioStreamTopic = "lk.audio_stream"

// DataStreamAudioOutput streams audio to a remote avatar via LiveKit ByteStream.
//
// It handles:
//   - Creating ByteStream writers for each utterance
//   - Registering RPC handlers for playback events
//   - Sending clear buffer requests
type DataStreamAudioOutput struct {
	room                *lksdk.Room
	destinationIdentity string
	config              AudioConfig

	// Stream state
	writer    *lksdk.ByteStreamWriter
	streamID  int
	callbacks []PlaybackCallback

	// RPC timeout
	rpcTimeout time.Duration

	// Synchronization
	mu     sync.Mutex
	closed bool
}

// DataStreamConfig configures the DataStreamAudioOutput.
type DataStreamConfig struct {
	// Room is the LiveKit room.
	// Required.
	Room *lksdk.Room

	// DestinationIdentity is the avatar participant identity.
	// Required.
	DestinationIdentity string

	// Audio is the audio configuration.
	// Optional, defaults to DefaultAudioConfig().
	Audio AudioConfig

	// RPCTimeout is the timeout for RPC calls.
	// Default: 5 seconds
	RPCTimeout time.Duration
}

// NewDataStreamAudioOutput creates a new DataStreamAudioOutput.
func NewDataStreamAudioOutput(cfg DataStreamConfig) (*DataStreamAudioOutput, error) {
	if cfg.Room == nil {
		return nil, ErrInvalidConfig
	}
	if cfg.DestinationIdentity == "" {
		return nil, ErrInvalidConfig
	}

	audio := cfg.Audio
	if audio.SampleRate == 0 {
		audio = DefaultAudioConfig()
	}

	rpcTimeout := cfg.RPCTimeout
	if rpcTimeout == 0 {
		rpcTimeout = 5 * time.Second
	}

	out := &DataStreamAudioOutput{
		room:                cfg.Room,
		destinationIdentity: cfg.DestinationIdentity,
		config:              audio,
		rpcTimeout:          rpcTimeout,
		callbacks:           make([]PlaybackCallback, 0),
	}

	// Register RPC handlers for playback events
	if err := out.registerRPCHandlers(); err != nil {
		return nil, err
	}

	return out, nil
}

// registerRPCHandlers registers RPC handlers for playback events from the avatar.
func (o *DataStreamAudioOutput) registerRPCHandlers() error {
	// Handle playback started
	if err := o.room.RegisterRpcMethod(RPCPlaybackStarted, o.handlePlaybackStarted); err != nil {
		return fmt.Errorf("failed to register %s: %w", RPCPlaybackStarted, err)
	}

	// Handle playback finished
	if err := o.room.RegisterRpcMethod(RPCPlaybackFinished, o.handlePlaybackFinished); err != nil {
		return fmt.Errorf("failed to register %s: %w", RPCPlaybackFinished, err)
	}

	return nil
}

// handlePlaybackStarted handles the playback started RPC from the avatar.
func (o *DataStreamAudioOutput) handlePlaybackStarted(data lksdk.RpcInvocationData) (string, error) {
	// Only process events from our avatar
	if data.CallerIdentity != o.destinationIdentity {
		return "", nil
	}

	o.mu.Lock()
	callbacks := make([]PlaybackCallback, len(o.callbacks))
	copy(callbacks, o.callbacks)
	o.mu.Unlock()

	event := PlaybackEvent{Type: PlaybackStarted}
	for _, cb := range callbacks {
		cb(event)
	}

	return "", nil
}

// handlePlaybackFinished handles the playback finished RPC from the avatar.
func (o *DataStreamAudioOutput) handlePlaybackFinished(data lksdk.RpcInvocationData) (string, error) {
	// Only process events from our avatar
	if data.CallerIdentity != o.destinationIdentity {
		return "", nil
	}

	// Parse payload
	var payload struct {
		PlaybackPosition float64 `json:"playback_position"`
		Interrupted      bool    `json:"interrupted"`
	}
	if err := json.Unmarshal([]byte(data.Payload), &payload); err != nil {
		// Non-fatal, use defaults
		payload.PlaybackPosition = 0
		payload.Interrupted = false
	}

	o.mu.Lock()
	callbacks := make([]PlaybackCallback, len(o.callbacks))
	copy(callbacks, o.callbacks)
	o.mu.Unlock()

	event := PlaybackEvent{
		Type:        PlaybackFinished,
		Position:    payload.PlaybackPosition,
		Interrupted: payload.Interrupted,
	}
	for _, cb := range callbacks {
		cb(event)
	}

	return "", nil
}

// CaptureFrame sends a PCM16 audio frame to the avatar.
func (o *DataStreamAudioOutput) CaptureFrame(ctx context.Context, frame []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.closed {
		return ErrStreamClosed
	}

	if len(frame) == 0 {
		return nil
	}

	// Create writer on first frame of a new utterance
	if o.writer == nil {
		o.streamID++
		streamID := fmt.Sprintf("audio-%d", o.streamID)

		o.writer = o.room.LocalParticipant.StreamBytes(lksdk.StreamBytesOptions{
			Topic:                 AudioStreamTopic,
			MimeType:              "audio/pcm",
			DestinationIdentities: []string{o.destinationIdentity},
			StreamId:              &streamID,
			Attributes: map[string]string{
				"sample_rate": fmt.Sprintf("%d", o.config.SampleRate),
				"channels":    fmt.Sprintf("%d", o.config.Channels),
				"encoding":    o.config.Encoding,
			},
		})
	}

	// Write frame to stream
	o.writer.Write(frame, nil)

	return nil
}

// Flush marks the end of an audio segment.
func (o *DataStreamAudioOutput) Flush(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.writer != nil {
		o.writer.Close()
		o.writer = nil
	}

	return nil
}

// ClearBuffer interrupts current playback.
func (o *DataStreamAudioOutput) ClearBuffer(ctx context.Context) error {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return ErrStreamClosed
	}

	// Close any active stream
	if o.writer != nil {
		o.writer.Close()
		o.writer = nil
	}
	o.mu.Unlock()

	// Send clear buffer RPC to avatar
	timeout := o.rpcTimeout
	_, err := o.room.LocalParticipant.PerformRpc(lksdk.PerformRpcParams{
		DestinationIdentity: o.destinationIdentity,
		Method:              RPCClearBuffer,
		Payload:             "",
		ResponseTimeout:     &timeout,
	})

	if err != nil {
		// Check if it's a timeout
		if rpcErr, ok := err.(*lksdk.RpcError); ok {
			if rpcErr.Code == lksdk.RpcResponseTimeout {
				return ErrRPCTimeout
			}
		}
		return fmt.Errorf("%w: %v", ErrRPCFailed, err)
	}

	return nil
}

// SampleRate returns the expected input sample rate.
func (o *DataStreamAudioOutput) SampleRate() int {
	return o.config.SampleRate
}

// Channels returns the expected number of audio channels.
func (o *DataStreamAudioOutput) Channels() int {
	return o.config.Channels
}

// OnPlayback registers a callback for playback events.
func (o *DataStreamAudioOutput) OnPlayback(callback PlaybackCallback) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.callbacks = append(o.callbacks, callback)
}

// Close releases resources.
func (o *DataStreamAudioOutput) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.closed {
		return nil
	}
	o.closed = true

	// Close any active stream
	if o.writer != nil {
		o.writer.Close()
		o.writer = nil
	}

	// Unregister RPC handlers
	// Note: The SDK doesn't have UnregisterRpcMethod, but the handlers
	// will be cleaned up when the room is disconnected.

	return nil
}

// Verify interface compliance at compile time.
var _ AudioDestination = (*DataStreamAudioOutput)(nil)
