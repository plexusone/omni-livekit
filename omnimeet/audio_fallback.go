//go:build !cgo || !opus
// +build !cgo !opus

// Package omnimeet provides a LiveKit implementation of the OmniMeet provider interface.
package omnimeet

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"github.com/plexusone/omnimeet-core/provider"
)

const (
	// Default sample rate for audio processing
	defaultSampleRate = 48000
	// Default frame duration in milliseconds
	defaultFrameDuration = 20
)

// audioReader reads audio from a WebRTC track.
// This is a fallback implementation that passes through raw audio data
// without Opus decoding. For proper Opus support, build with CGO and opus tags.
type audioReader struct {
	ctx             context.Context
	cancel          context.CancelFunc
	track           *webrtc.TrackRemote
	participantID   string
	participantName string
	outputCh        chan<- provider.AudioFrame

	mu     sync.Mutex
	closed bool
}

// newAudioReader creates a new audio reader for a remote track.
// This fallback version does not decode Opus - it passes raw RTP payloads.
func newAudioReader(
	ctx context.Context,
	track *webrtc.TrackRemote,
	participantID, participantName string,
	outputCh chan<- provider.AudioFrame,
	_ interface{}, // logger (ignored in fallback)
) (*audioReader, error) {
	ctx, cancel := context.WithCancel(ctx)

	reader := &audioReader{
		ctx:             ctx,
		cancel:          cancel,
		track:           track,
		participantID:   participantID,
		participantName: participantName,
		outputCh:        outputCh,
	}

	// Start reading from the track
	go reader.readLoop()

	return reader, nil
}

// readLoop reads RTP packets from the track.
// In this fallback version, raw audio data is passed through without decoding.
func (r *audioReader) readLoop() {
	defer func() {
		r.mu.Lock()
		r.closed = true
		r.mu.Unlock()
	}()

	buf := make([]byte, 1500)
	for {
		select {
		case <-r.ctx.Done():
			return
		default:
		}

		// Read RTP packet from the track
		n, _, err := r.track.Read(buf)
		if err != nil {
			if err == io.EOF || r.ctx.Err() != nil {
				return
			}
			continue
		}

		if n == 0 {
			continue
		}

		// In fallback mode, we pass raw data (not decoded)
		// This won't be usable for most STT services which expect PCM
		data := make([]byte, n)
		copy(data, buf[:n])

		frame := provider.AudioFrame{
			ParticipantID:   r.participantID,
			ParticipantName: r.participantName,
			Data:            data,
			SampleRate:      defaultSampleRate,
			Channels:        1,
			Timestamp:       time.Now(),
		}

		select {
		case r.outputCh <- frame:
		default:
			// Channel full, drop frame
		}
	}
}

// Close stops the audio reader.
func (r *audioReader) Close() error {
	r.cancel()
	return nil
}

// audioWriter writes audio to a LiveKit track.
// This is a fallback implementation that does not encode to Opus.
type audioWriter struct {
	track      *lksdk.LocalSampleTrack
	sampleRate int
	channels   int
	frameDur   time.Duration

	mu     sync.Mutex
	closed bool
}

// newAudioWriter creates a new audio writer for publishing.
// This fallback version does not encode to Opus.
func newAudioWriter(track *lksdk.LocalSampleTrack, sampleRate, channels int, _ interface{}) (*audioWriter, error) {
	return &audioWriter{
		track:      track,
		sampleRate: sampleRate,
		channels:   channels,
		frameDur:   time.Duration(defaultFrameDuration) * time.Millisecond,
	}, nil
}

// Write writes PCM16 audio bytes to the track.
// In fallback mode, this writes raw data without Opus encoding.
// This may not work correctly with most WebRTC clients.
func (w *audioWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return 0, fmt.Errorf("writer closed")
	}
	w.mu.Unlock()

	// In fallback mode, write raw data
	// Note: This won't work properly without Opus encoding
	err := w.track.WriteSample(
		media.Sample{Data: data, Duration: w.frameDur},
		&lksdk.SampleWriteOptions{},
	)
	if err != nil {
		return 0, err
	}

	return len(data), nil
}

// Close closes the audio writer.
func (w *audioWriter) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	w.mu.Unlock()
	return nil
}
