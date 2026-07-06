package avatar

import (
	"context"
	"testing"
)

func TestNewQueueAudioOutput(t *testing.T) {
	// Test with default config
	q := NewQueueAudioOutput(AudioConfig{})
	if q.SampleRate() != 24000 {
		t.Errorf("expected default SampleRate 24000, got %d", q.SampleRate())
	}
	if q.Channels() != 1 {
		t.Errorf("expected default Channels 1, got %d", q.Channels())
	}

	// Test with custom config
	cfg := AudioConfig{
		SampleRate: 48000,
		Channels:   2,
		Encoding:   "linear16",
	}
	q2 := NewQueueAudioOutput(cfg)
	if q2.SampleRate() != 48000 {
		t.Errorf("expected SampleRate 48000, got %d", q2.SampleRate())
	}
	if q2.Channels() != 2 {
		t.Errorf("expected Channels 2, got %d", q2.Channels())
	}
}

func TestQueueAudioOutputCaptureFrame(t *testing.T) {
	ctx := context.Background()
	q := NewQueueAudioOutput(DefaultAudioConfig())

	// Capture some frames
	frame1 := []byte{0x01, 0x02, 0x03, 0x04}
	frame2 := []byte{0x05, 0x06, 0x07, 0x08}

	if err := q.CaptureFrame(ctx, frame1); err != nil {
		t.Fatalf("CaptureFrame failed: %v", err)
	}
	if err := q.CaptureFrame(ctx, frame2); err != nil {
		t.Fatalf("CaptureFrame failed: %v", err)
	}

	// Verify
	if q.FrameCount() != 2 {
		t.Errorf("expected 2 frames, got %d", q.FrameCount())
	}
	if q.TotalBytes() != 8 {
		t.Errorf("expected 8 bytes, got %d", q.TotalBytes())
	}

	frames := q.Frames()
	if len(frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(frames))
	}
	for i, b := range frame1 {
		if frames[0][i] != b {
			t.Errorf("frame1[%d]: expected %d, got %d", i, b, frames[0][i])
		}
	}
}

func TestQueueAudioOutputEmptyFrame(t *testing.T) {
	ctx := context.Background()
	q := NewQueueAudioOutput(DefaultAudioConfig())

	// Empty frames should be ignored
	if err := q.CaptureFrame(ctx, []byte{}); err != nil {
		t.Fatalf("CaptureFrame failed: %v", err)
	}
	if err := q.CaptureFrame(ctx, nil); err != nil {
		t.Fatalf("CaptureFrame failed: %v", err)
	}

	if q.FrameCount() != 0 {
		t.Errorf("expected 0 frames, got %d", q.FrameCount())
	}
}

func TestQueueAudioOutputFlush(t *testing.T) {
	ctx := context.Background()
	q := NewQueueAudioOutput(DefaultAudioConfig())

	if q.WasFlushed() {
		t.Error("expected WasFlushed false initially")
	}

	if err := q.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	if !q.WasFlushed() {
		t.Error("expected WasFlushed true after Flush()")
	}
}

func TestQueueAudioOutputClearBuffer(t *testing.T) {
	ctx := context.Background()
	q := NewQueueAudioOutput(DefaultAudioConfig())

	// Add some frames
	_ = q.CaptureFrame(ctx, []byte{0x01, 0x02})
	_ = q.CaptureFrame(ctx, []byte{0x03, 0x04})

	if q.WasCleared() {
		t.Error("expected WasCleared false initially")
	}

	if err := q.ClearBuffer(ctx); err != nil {
		t.Fatalf("ClearBuffer failed: %v", err)
	}

	if !q.WasCleared() {
		t.Error("expected WasCleared true after ClearBuffer()")
	}

	// Frames should be cleared
	if q.FrameCount() != 0 {
		t.Errorf("expected 0 frames after ClearBuffer, got %d", q.FrameCount())
	}
}

func TestQueueAudioOutputClose(t *testing.T) {
	ctx := context.Background()
	q := NewQueueAudioOutput(DefaultAudioConfig())

	if err := q.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Operations should fail after close
	if err := q.CaptureFrame(ctx, []byte{0x01}); err != ErrStreamClosed {
		t.Errorf("expected ErrStreamClosed, got %v", err)
	}
	if err := q.Flush(ctx); err != ErrStreamClosed {
		t.Errorf("expected ErrStreamClosed, got %v", err)
	}
	if err := q.ClearBuffer(ctx); err != ErrStreamClosed {
		t.Errorf("expected ErrStreamClosed, got %v", err)
	}
}

func TestQueueAudioOutputReset(t *testing.T) {
	ctx := context.Background()
	q := NewQueueAudioOutput(DefaultAudioConfig())

	// Add frames and flush
	_ = q.CaptureFrame(ctx, []byte{0x01, 0x02})
	_ = q.Flush(ctx)
	_ = q.ClearBuffer(ctx)

	// Reset
	q.Reset()

	if q.FrameCount() != 0 {
		t.Errorf("expected 0 frames after Reset, got %d", q.FrameCount())
	}
	if q.WasFlushed() {
		t.Error("expected WasFlushed false after Reset")
	}
	if q.WasCleared() {
		t.Error("expected WasCleared false after Reset")
	}
}

func TestQueueAudioOutputPlaybackCallbacks(t *testing.T) {
	q := NewQueueAudioOutput(DefaultAudioConfig())

	startedCount := 0
	finishedCount := 0
	var lastPosition float64
	var lastInterrupted bool

	q.OnPlayback(func(event PlaybackEvent) {
		switch event.Type {
		case PlaybackStarted:
			startedCount++
		case PlaybackFinished:
			finishedCount++
			lastPosition = event.Position
			lastInterrupted = event.Interrupted
		}
	})

	// Simulate events
	q.SimulatePlaybackStarted()
	if startedCount != 1 {
		t.Errorf("expected startedCount 1, got %d", startedCount)
	}

	q.SimulatePlaybackFinished(2.5, true)
	if finishedCount != 1 {
		t.Errorf("expected finishedCount 1, got %d", finishedCount)
	}
	if lastPosition != 2.5 {
		t.Errorf("expected position 2.5, got %f", lastPosition)
	}
	if !lastInterrupted {
		t.Error("expected interrupted true")
	}
}

func TestQueueAudioOutputFrameCopy(t *testing.T) {
	ctx := context.Background()
	q := NewQueueAudioOutput(DefaultAudioConfig())

	// Original frame
	frame := []byte{0x01, 0x02, 0x03, 0x04}
	_ = q.CaptureFrame(ctx, frame)

	// Modify original frame
	frame[0] = 0xFF

	// Captured frame should be unchanged
	frames := q.Frames()
	if frames[0][0] != 0x01 {
		t.Error("captured frame was mutated by external change")
	}

	// Modify returned frame
	frames[0][1] = 0xFF

	// Internal frame should be unchanged
	frames2 := q.Frames()
	if frames2[0][1] != 0x02 {
		t.Error("internal frame was mutated by external change to returned slice")
	}
}
