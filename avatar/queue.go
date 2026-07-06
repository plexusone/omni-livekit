package avatar

import (
	"context"
	"sync"
)

// QueueAudioOutput is an in-memory AudioDestination for testing.
//
// It captures all audio frames sent to it, allowing tests to verify
// that the correct audio data is being produced.
type QueueAudioOutput struct {
	config    AudioConfig
	frames    [][]byte
	flushed   bool
	cleared   bool
	callbacks []PlaybackCallback

	mu     sync.Mutex
	closed bool
}

// NewQueueAudioOutput creates a new QueueAudioOutput with the given config.
func NewQueueAudioOutput(config AudioConfig) *QueueAudioOutput {
	if config.SampleRate == 0 {
		config = DefaultAudioConfig()
	}
	return &QueueAudioOutput{
		config:    config,
		frames:    make([][]byte, 0),
		callbacks: make([]PlaybackCallback, 0),
	}
}

// CaptureFrame stores a PCM16 audio frame in the queue.
func (q *QueueAudioOutput) CaptureFrame(ctx context.Context, frame []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return ErrStreamClosed
	}

	if len(frame) == 0 {
		return nil
	}

	// Copy frame to avoid external mutation
	frameCopy := make([]byte, len(frame))
	copy(frameCopy, frame)
	q.frames = append(q.frames, frameCopy)

	return nil
}

// Flush marks the end of an audio segment.
func (q *QueueAudioOutput) Flush(ctx context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return ErrStreamClosed
	}

	q.flushed = true
	return nil
}

// ClearBuffer simulates interrupting playback.
func (q *QueueAudioOutput) ClearBuffer(ctx context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return ErrStreamClosed
	}

	q.cleared = true
	q.frames = make([][]byte, 0)
	return nil
}

// SampleRate returns the configured sample rate.
func (q *QueueAudioOutput) SampleRate() int {
	return q.config.SampleRate
}

// Channels returns the configured number of channels.
func (q *QueueAudioOutput) Channels() int {
	return q.config.Channels
}

// OnPlayback registers a callback for playback events.
func (q *QueueAudioOutput) OnPlayback(callback PlaybackCallback) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.callbacks = append(q.callbacks, callback)
}

// Close marks the queue as closed.
func (q *QueueAudioOutput) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true
	return nil
}

// Frames returns all captured frames.
func (q *QueueAudioOutput) Frames() [][]byte {
	q.mu.Lock()
	defer q.mu.Unlock()

	result := make([][]byte, len(q.frames))
	for i, f := range q.frames {
		frameCopy := make([]byte, len(f))
		copy(frameCopy, f)
		result[i] = frameCopy
	}
	return result
}

// TotalBytes returns the total number of bytes captured.
func (q *QueueAudioOutput) TotalBytes() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	total := 0
	for _, f := range q.frames {
		total += len(f)
	}
	return total
}

// FrameCount returns the number of frames captured.
func (q *QueueAudioOutput) FrameCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.frames)
}

// WasFlushed returns true if Flush() was called.
func (q *QueueAudioOutput) WasFlushed() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.flushed
}

// WasCleared returns true if ClearBuffer() was called.
func (q *QueueAudioOutput) WasCleared() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.cleared
}

// Reset clears all captured data and resets state.
func (q *QueueAudioOutput) Reset() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.frames = make([][]byte, 0)
	q.flushed = false
	q.cleared = false
}

// SimulatePlaybackStarted simulates a playback started event.
func (q *QueueAudioOutput) SimulatePlaybackStarted() {
	q.mu.Lock()
	callbacks := make([]PlaybackCallback, len(q.callbacks))
	copy(callbacks, q.callbacks)
	q.mu.Unlock()

	event := PlaybackEvent{Type: PlaybackStarted}
	for _, cb := range callbacks {
		cb(event)
	}
}

// SimulatePlaybackFinished simulates a playback finished event.
func (q *QueueAudioOutput) SimulatePlaybackFinished(position float64, interrupted bool) {
	q.mu.Lock()
	callbacks := make([]PlaybackCallback, len(q.callbacks))
	copy(callbacks, q.callbacks)
	q.mu.Unlock()

	event := PlaybackEvent{
		Type:        PlaybackFinished,
		Position:    position,
		Interrupted: interrupted,
	}
	for _, cb := range callbacks {
		cb(event)
	}
}

// Verify interface compliance at compile time.
var _ AudioDestination = (*QueueAudioOutput)(nil)
