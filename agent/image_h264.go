//go:build cgo

package agent

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"os"
	"sync"
	"time"

	"github.com/gen2brain/x264-go"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	// Import image formats for decoding
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// h264ImageWriter publishes a static image as an H.264-encoded video track.
// It encodes the image once and repeatedly sends the keyframe.
type h264ImageWriter struct {
	track     *lksdk.LocalSampleTrack
	width     int
	height    int
	frameRate int
	frameDur  time.Duration

	// Cached encoded keyframe
	keyframe   []byte
	keyframeMu sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	closed bool
}

// newH264ImageWriter creates a new H.264-based image writer.
func newH264ImageWriter(track *lksdk.LocalSampleTrack, width, height, frameRate int) (*h264ImageWriter, error) {
	ctx, cancel := context.WithCancel(context.Background())

	w := &h264ImageWriter{
		track:     track,
		width:     width,
		height:    height,
		frameRate: frameRate,
		frameDur:  time.Second / time.Duration(frameRate),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Start the frame loop
	go w.frameLoop()

	return w, nil
}

// UpdateImage encodes a new image as H.264 and updates the cached keyframe.
func (w *h264ImageWriter) UpdateImage(data []byte) error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return fmt.Errorf("writer closed")
	}
	w.mu.Unlock()

	// Decode the image
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// Encode to H.264
	keyframe, err := encodeImageToH264(img, w.width, w.height)
	if err != nil {
		return fmt.Errorf("failed to encode H.264: %w", err)
	}

	w.keyframeMu.Lock()
	w.keyframe = keyframe
	w.keyframeMu.Unlock()

	return nil
}

// encodeImageToH264 encodes an image as a single H.264 keyframe.
func encodeImageToH264(img image.Image, targetWidth, targetHeight int) ([]byte, error) {
	// Get image bounds
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Use target dimensions if specified
	if targetWidth > 0 {
		width = targetWidth
	}
	if targetHeight > 0 {
		height = targetHeight
	}

	// Ensure dimensions are even (H.264 requirement)
	width = width &^ 1
	height = height &^ 1

	// Create encoder buffer
	buf := bytes.NewBuffer(make([]byte, 0, 64*1024))

	// Configure x264 for a single keyframe
	opts := &x264.Options{
		Width:     width,
		Height:    height,
		FrameRate: 1,
		KeyInt:    1, // Every frame is a keyframe
		Tune:      "stillimage",
		Preset:    "medium",
		Profile:   "baseline", // Baseline for maximum compatibility
		LogLevel:  x264.LogNone,
	}

	enc, err := x264.NewEncoder(buf, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create encoder: %w", err)
	}
	defer enc.Close()

	// Encode the image
	if err := enc.Encode(img); err != nil {
		return nil, fmt.Errorf("failed to encode frame: %w", err)
	}

	// Flush to get all data
	if err := enc.Flush(); err != nil {
		return nil, fmt.Errorf("failed to flush encoder: %w", err)
	}

	return buf.Bytes(), nil
}

// frameLoop sends the cached keyframe at the configured frame rate.
func (w *h264ImageWriter) frameLoop() {
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
				// Send the cached H.264 keyframe
				w.track.WriteSample(
					media.Sample{Data: keyframe, Duration: w.frameDur},
					&lksdk.SampleWriteOptions{},
				)
			}
		}
	}
}

// Close stops the image writer.
func (w *h264ImageWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	w.closed = true
	w.cancel()
	return nil
}

// startImageVideoLocked starts H.264-encoded image video publishing.
// This replaces the fallback implementation when CGO is available.
func (a *Agent) startImageVideoLocked(ctx context.Context) error {
	// Load image data
	var imageData []byte
	var err error

	if len(a.opts.Image.Data) > 0 {
		imageData = a.opts.Image.Data
	} else if a.opts.Image.Path != "" {
		imageData, err = os.ReadFile(a.opts.Image.Path)
		if err != nil {
			return fmt.Errorf("failed to read image file: %w", err)
		}
	}

	// Determine dimensions
	width := a.opts.Image.Width
	height := a.opts.Image.Height
	if width == 0 {
		width = 640
	}
	if height == 0 {
		height = 480
	}

	frameRate := a.opts.Image.FrameRate
	if frameRate == 0 {
		frameRate = 1 // 1 fps for static images saves bandwidth
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

	// Create H.264 image writer
	writer, err := newH264ImageWriter(track, width, height, frameRate)
	if err != nil {
		return fmt.Errorf("failed to create H.264 image writer: %w", err)
	}

	// Set initial image
	if len(imageData) > 0 {
		if err := writer.UpdateImage(imageData); err != nil {
			return fmt.Errorf("failed to set initial image: %w", err)
		}
	}

	a.videoTrack = track
	a.imageWriter = writer // h264ImageWriter implements ImageWriter

	return nil
}

// Verify h264ImageWriter implements ImageWriter
var _ ImageWriter = (*h264ImageWriter)(nil)
