//go:build !cgo || !opus

package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/pion/webrtc/v4/pkg/media"
)

// AudioWriter writes PCM16 audio data to the meeting.
type AudioWriter interface {
	// Write writes PCM16 little-endian audio data.
	Write(data []byte) (int, error)
	// Close stops the audio writer.
	Close() error
}

// audioWriter is a fallback implementation without Opus encoding.
// Audio data is passed through without encoding.
type audioWriter struct {
	track      *lksdk.LocalSampleTrack
	sampleRate int
	channels   int
	frameDur   time.Duration

	mu     sync.Mutex
	closed bool
}

// newAudioWriter creates a new audio writer (fallback without Opus).
func newAudioWriter(track *lksdk.LocalSampleTrack, sampleRate, channels int) (*audioWriter, error) {
	return &audioWriter{
		track:      track,
		sampleRate: sampleRate,
		channels:   channels,
		frameDur:   20 * time.Millisecond,
	}, nil
}

// Write writes audio data (fallback: no encoding).
func (w *audioWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return 0, fmt.Errorf("writer closed")
	}
	w.mu.Unlock()

	// Fallback: write raw data directly
	// This won't work correctly without Opus encoding
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
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

// readAudioTrack is a fallback that reads raw RTP data without decoding.
func (a *Agent) readAudioTrack(ctx context.Context, pub *lksdk.RemoteTrackPublication, participantID, participantName string, audioCh chan<- AudioFrame) {
	defer close(audioCh)

	// Wait for the track to be subscribed
	track := pub.TrackRemote()
	if track == nil {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				track = pub.TrackRemote()
				if track != nil {
					goto startReading
				}
			}
		}
	}

startReading:
	buf := make([]byte, 1500)
	var seqNum uint64
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, _, err := track.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}

		if n == 0 {
			continue
		}

		// Without Opus decoding, we pass through raw RTP payload
		// This is not ideal but allows basic functionality
		seqNum++
		frame := AudioFrame{
			ParticipantID:   participantID,
			ParticipantName: participantName,
			Data:            append([]byte{}, buf[:n]...),
			SampleRate:      48000,
			Channels:        1,
			Timestamp:       time.Now(),
			SequenceNumber:  seqNum,
		}

		select {
		case audioCh <- frame:
		default:
		}
	}
}

var _ AudioWriter = (*audioWriter)(nil)
