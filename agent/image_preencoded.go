package agent

// Pre-encoded H.264 Avatar Support
//
// This file provides avatar support using pre-encoded H.264 keyframes.
// No CGO is required at runtime - the encoding is done ahead of time
// using the encode-avatar tool.
//
// Workflow:
//   1. Pre-encode: go run ./cmd/encode-avatar -input avatar.png -output avatar.h264
//   2. Commit avatar.h264 to your repository
//   3. At runtime: load and send the pre-encoded bytes
//
// This is the recommended approach for production deployments.

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// preencodedImageWriter publishes a pre-encoded H.264 keyframe as a video track.
// No encoding is performed at runtime - the H.264 data is read directly from a file.
type preencodedImageWriter struct {
	track     *lksdk.LocalSampleTrack
	frameRate int
	frameDur  time.Duration

	// Cached H.264 keyframe data
	keyframe   []byte
	keyframeMu sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	closed bool
}

// newPreencodedImageWriter creates a new pre-encoded image writer.
func newPreencodedImageWriter(track *lksdk.LocalSampleTrack, frameRate int) *preencodedImageWriter {
	ctx, cancel := context.WithCancel(context.Background())

	w := &preencodedImageWriter{
		track:     track,
		frameRate: frameRate,
		frameDur:  time.Second / time.Duration(frameRate),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Start the frame loop
	go w.frameLoop()

	return w
}

// SetH264Data sets the pre-encoded H.264 keyframe data.
func (w *preencodedImageWriter) SetH264Data(data []byte) {
	w.keyframeMu.Lock()
	w.keyframe = data
	w.keyframeMu.Unlock()
}

// UpdateImage is not supported for pre-encoded images.
// Use SetH264Data instead, or re-encode the image using encode-avatar.
func (w *preencodedImageWriter) UpdateImage(data []byte) error {
	return fmt.Errorf("UpdateImage not supported for pre-encoded avatars; use encode-avatar tool to create new H.264 file")
}

// frameLoop sends the cached keyframe at the configured frame rate.
func (w *preencodedImageWriter) frameLoop() {
	ticker := time.NewTicker(w.frameDur)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.mu.Lock()
			closed := w.closed
			w.mu.Unlock()
			if closed {
				return
			}

			w.keyframeMu.RLock()
			keyframe := w.keyframe
			w.keyframeMu.RUnlock()

			if len(keyframe) > 0 {
				// WriteSample errors are non-fatal for streaming video
				_ = w.track.WriteSample(
					media.Sample{Data: keyframe, Duration: w.frameDur},
					&lksdk.SampleWriteOptions{},
				)
			}
		}
	}
}

// Close stops the image writer.
func (w *preencodedImageWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	w.closed = true
	w.cancel()
	return nil
}

// startPreencodedImageVideo starts pre-encoded H.264 image video publishing.
// This does not require CGO - the H.264 data is read from a file.
func (a *Agent) startPreencodedImageVideo(_ context.Context) error {
	// Load pre-encoded H.264 data
	var h264Data []byte
	var err error

	if len(a.opts.Image.H264Data) > 0 {
		h264Data = a.opts.Image.H264Data
	} else if a.opts.Image.H264Path != "" {
		h264Data, err = os.ReadFile(a.opts.Image.H264Path)
		if err != nil {
			return fmt.Errorf("failed to read H.264 file: %w", err)
		}
	} else {
		return fmt.Errorf("no H.264 data provided; use encode-avatar to pre-encode your image")
	}

	frameRate := a.opts.Image.FrameRate
	if frameRate == 0 {
		frameRate = 1
	}

	// Create H.264 video track
	track, err := lksdk.NewLocalSampleTrack(webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypeH264,
		ClockRate: 90000,
	})
	if err != nil {
		return fmt.Errorf("failed to create video track: %w", err)
	}

	// Publish the track
	_, err = a.room.LocalParticipant.PublishTrack(track, &lksdk.TrackPublicationOptions{
		Name:   a.opts.Image.TrackName,
		Source: livekit.TrackSource_CAMERA,
	})
	if err != nil {
		return fmt.Errorf("failed to publish video track: %w", err)
	}

	// Create pre-encoded image writer
	writer := newPreencodedImageWriter(track, frameRate)
	writer.SetH264Data(h264Data)

	a.videoTrack = track
	a.imageWriter = writer

	return nil
}

// Verify preencodedImageWriter implements ImageWriter
var _ ImageWriter = (*preencodedImageWriter)(nil)
