package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// VideoWriter writes video frames to the meeting.
type VideoWriter interface {
	// WriteFrame writes a video frame.
	WriteFrame(frame VideoFrame) error
	// Close stops the video writer.
	Close() error
}

// VideoFrame represents a video frame to publish.
type VideoFrame struct {
	// Data contains encoded video data (VP8 or H264 depending on codec).
	Data []byte
	// Width of the frame in pixels.
	Width int
	// Height of the frame in pixels.
	Height int
	// Timestamp of the frame.
	Timestamp time.Time
	// Keyframe indicates if this is a keyframe.
	Keyframe bool
}

// videoWriter encodes and writes video frames to a LiveKit track.
type videoWriter struct {
	track     *lksdk.LocalSampleTrack
	width     int
	height    int
	frameRate int
	codec     string
	frameDur  time.Duration

	mu     sync.Mutex
	closed bool
}

// newVideoWriter creates a new video writer for publishing.
func newVideoWriter(track *lksdk.LocalSampleTrack, width, height, frameRate int, codec string) (*videoWriter, error) {
	frameDur := time.Second / time.Duration(frameRate)

	return &videoWriter{
		track:     track,
		width:     width,
		height:    height,
		frameRate: frameRate,
		codec:     codec,
		frameDur:  frameDur,
	}, nil
}

// WriteFrame writes a video frame to the track.
func (w *videoWriter) WriteFrame(frame VideoFrame) error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return fmt.Errorf("writer closed")
	}
	w.mu.Unlock()

	return w.track.WriteSample(
		media.Sample{Data: frame.Data, Duration: w.frameDur},
		&lksdk.SampleWriteOptions{},
	)
}

// Close closes the video writer.
func (w *videoWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

// startVideoLocked starts video publishing (must hold lock).
func (a *Agent) startVideoLocked(ctx context.Context) (*videoWriter, error) {
	// Determine codec
	var mimeType string
	switch a.opts.Video.Codec {
	case "h264":
		mimeType = webrtc.MimeTypeH264
	default:
		mimeType = webrtc.MimeTypeVP8
	}

	// Create video track
	track, err := lksdk.NewLocalSampleTrack(webrtc.RTPCodecCapability{
		MimeType:  mimeType,
		ClockRate: 90000, // Video uses 90kHz clock
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create video track: %w", err)
	}

	// Publish the track
	_, err = a.room.LocalParticipant.PublishTrack(track, &lksdk.TrackPublicationOptions{
		Name:   a.opts.Video.TrackName,
		Source: livekit.TrackSource_CAMERA,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to publish video track: %w", err)
	}

	// Create video writer
	writer, err := newVideoWriter(
		track,
		a.opts.Video.Width,
		a.opts.Video.Height,
		a.opts.Video.FrameRate,
		a.opts.Video.Codec,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create video writer: %w", err)
	}

	a.videoTrack = track
	a.videoWriter = writer

	return writer, nil
}

var _ VideoWriter = (*videoWriter)(nil)
