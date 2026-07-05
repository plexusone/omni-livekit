package agent

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	// Import image formats
	_ "image/gif"
)

// imageWriter publishes a static image as a video track.
type imageWriter struct {
	track       *lksdk.LocalSampleTrack
	width       int
	height      int
	frameRate   int
	frameDur    time.Duration
	currentData []byte

	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	closed bool
}

// newImageWriter creates a new image writer for publishing static images.
func newImageWriter(track *lksdk.LocalSampleTrack, width, height, frameRate int) (*imageWriter, error) {
	ctx, cancel := context.WithCancel(context.Background())

	w := &imageWriter{
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

// UpdateImage updates the static image being published.
func (w *imageWriter) UpdateImage(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return fmt.Errorf("writer closed")
	}

	w.currentData = data
	return nil
}

// frameLoop continuously sends frames at the configured frame rate.
func (w *imageWriter) frameLoop() {
	ticker := time.NewTicker(w.frameDur)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.mu.Lock()
			data := w.currentData
			closed := w.closed
			w.mu.Unlock()

			if closed {
				return
			}

			if len(data) > 0 {
				// Write the frame (already encoded as JPEG for simplicity)
				w.track.WriteSample(
					media.Sample{Data: data, Duration: w.frameDur},
					&lksdk.SampleWriteOptions{},
				)
			}
		}
	}
}

// Close stops the image writer.
func (w *imageWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	w.closed = true
	w.cancel()
	return nil
}

// startImageVideoLocked starts image-based video publishing (must hold lock).
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

	// Process image if needed (resize, convert format)
	if len(imageData) > 0 {
		imageData, err = processImage(imageData, a.opts.Image.Width, a.opts.Image.Height)
		if err != nil {
			return fmt.Errorf("failed to process image: %w", err)
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

	// Create video track for the image
	// Using VP8 as it's widely supported
	track, err := lksdk.NewLocalSampleTrack(webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypeVP8,
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

	// Create image writer
	writer, err := newImageWriter(track, width, height, a.opts.Image.FrameRate)
	if err != nil {
		return fmt.Errorf("failed to create image writer: %w", err)
	}

	// Set initial image
	if len(imageData) > 0 {
		writer.UpdateImage(imageData)
	}

	a.videoTrack = track
	a.imageWriter = writer

	return nil
}

// processImage decodes, optionally resizes, and re-encodes an image.
func processImage(data []byte, targetWidth, targetHeight int) ([]byte, error) {
	// Decode the image
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Get original dimensions
	bounds := img.Bounds()
	origWidth := bounds.Dx()
	origHeight := bounds.Dy()

	// If no resize needed and format is already JPEG, return as-is
	if (targetWidth == 0 || targetWidth == origWidth) &&
		(targetHeight == 0 || targetHeight == origHeight) &&
		strings.EqualFold(format, "jpeg") {
		return data, nil
	}

	// For now, we don't do actual resizing (would require additional dependency)
	// Just re-encode as JPEG if needed
	if !strings.EqualFold(format, "jpeg") {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
			return nil, fmt.Errorf("failed to encode image as JPEG: %w", err)
		}
		return buf.Bytes(), nil
	}

	return data, nil
}

// encodeImageToJPEG converts an image to JPEG format.
func encodeImageToJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// encodeImageToPNG converts an image to PNG format.
func encodeImageToPNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
