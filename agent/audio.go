//go:build cgo && opus

package agent

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/livekit/media-sdk"
	"github.com/livekit/media-sdk/opus"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/pion/webrtc/v4"
	pionmedia "github.com/pion/webrtc/v4/pkg/media"
)

const (
	defaultSampleRate    = 48000
	defaultFrameDuration = 20 // ms
)

// AudioWriter writes PCM16 audio data to the meeting.
type AudioWriter interface {
	// Write writes PCM16 little-endian audio data.
	Write(data []byte) (int, error)
	// Close stops the audio writer.
	Close() error
}

// audioWriter encodes PCM to Opus and writes to a LiveKit track.
type audioWriter struct {
	track      *lksdk.LocalSampleTrack
	encoder    media.PCM16Writer
	sampleRate int
	channels   int
	frameDur   time.Duration
	opusWriter *opusSampleWriter

	mu     sync.Mutex
	closed bool
}

// newAudioWriter creates a new audio writer for publishing.
func newAudioWriter(track *lksdk.LocalSampleTrack, sampleRate, channels int) (*audioWriter, error) {
	frameDur := time.Duration(defaultFrameDuration) * time.Millisecond

	// Create opus writer that writes to the LiveKit track
	opusWriter := &opusSampleWriter{
		track:      track,
		sampleRate: sampleRate,
		frameDur:   frameDur,
	}

	// Create Opus encoder
	encoder, err := opus.Encode(opusWriter, channels, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create opus encoder: %w", err)
	}

	return &audioWriter{
		track:      track,
		encoder:    encoder,
		sampleRate: sampleRate,
		channels:   channels,
		frameDur:   frameDur,
		opusWriter: opusWriter,
	}, nil
}

// Write writes PCM16 audio bytes (little-endian) to the track.
func (w *audioWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return 0, fmt.Errorf("writer closed")
	}
	w.mu.Unlock()

	// Convert bytes to PCM16 samples
	samples := make(media.PCM16Sample, len(data)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}

	// Encode and write
	if err := w.encoder.WriteSample(samples); err != nil {
		return 0, fmt.Errorf("failed to encode audio: %w", err)
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

	return w.encoder.Close()
}

// opusSampleWriter writes Opus samples to a LiveKit track.
type opusSampleWriter struct {
	track      *lksdk.LocalSampleTrack
	sampleRate int
	frameDur   time.Duration
}

func (w *opusSampleWriter) String() string {
	return fmt.Sprintf("OpusSampleWriter(%d)", w.sampleRate)
}

func (w *opusSampleWriter) SampleRate() int {
	return w.sampleRate
}

func (w *opusSampleWriter) Close() error {
	return nil
}

// WriteSample writes an Opus sample to the track.
func (w *opusSampleWriter) WriteSample(data opus.Sample) error {
	return w.track.WriteSample(
		pionmedia.Sample{Data: data, Duration: w.frameDur},
		&lksdk.SampleWriteOptions{},
	)
}

// audioReader reads audio from a WebRTC track and decodes Opus to PCM.
type audioReader struct {
	ctx             context.Context
	cancel          context.CancelFunc
	track           *webrtc.TrackRemote
	participantID   string
	participantName string
	outputCh        chan<- AudioFrame
	decoder         media.WriteCloser[opus.Sample]
	pcmWriter       *pcmFrameWriter

	mu     sync.Mutex
	closed bool
}

// newAudioReader creates a new audio reader for a remote track.
func newAudioReader(
	ctx context.Context,
	track *webrtc.TrackRemote,
	participantID, participantName string,
	outputCh chan<- AudioFrame,
) (*audioReader, error) {
	ctx, cancel := context.WithCancel(ctx)

	// Create PCM writer that forwards to the output channel
	pcmWriter := &pcmFrameWriter{
		participantID:   participantID,
		participantName: participantName,
		outputCh:        outputCh,
		sampleRate:      defaultSampleRate,
		channels:        1,
	}

	// Create Opus decoder that writes PCM to our writer
	decoder, err := opus.Decode(pcmWriter, 1, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create opus decoder: %w", err)
	}

	reader := &audioReader{
		ctx:             ctx,
		cancel:          cancel,
		track:           track,
		participantID:   participantID,
		participantName: participantName,
		outputCh:        outputCh,
		decoder:         decoder,
		pcmWriter:       pcmWriter,
	}

	// Start reading from the track
	go reader.readLoop()

	return reader, nil
}

// readLoop reads RTP packets from the track and decodes them.
func (r *audioReader) readLoop() {
	defer func() {
		r.decoder.Close()
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

		// Extract Opus payload from RTP packet
		// RTP header is at least 12 bytes
		if n <= 12 {
			continue
		}

		// Parse RTP header to get payload offset
		headerLen := 12
		// Check for extensions
		if buf[0]&0x10 != 0 {
			if n < headerLen+4 {
				continue
			}
			extLen := int(binary.BigEndian.Uint16(buf[headerLen+2:headerLen+4])) * 4
			headerLen += 4 + extLen
		}
		// Check for padding
		if buf[0]&0x20 != 0 && n > headerLen {
			paddingLen := int(buf[n-1])
			if paddingLen < n-headerLen {
				n -= paddingLen
			}
		}
		// Check for CSRC count
		csrcCount := int(buf[0] & 0x0F)
		headerLen += csrcCount * 4

		if n <= headerLen {
			continue
		}

		// Extract Opus payload
		opusPayload := make([]byte, n-headerLen)
		copy(opusPayload, buf[headerLen:n])

		// Decode Opus to PCM
		r.decoder.WriteSample(opus.Sample(opusPayload))
	}
}

// Close stops the audio reader.
func (r *audioReader) Close() error {
	r.cancel()
	return nil
}

// pcmFrameWriter receives decoded PCM samples and sends them as AudioFrames.
type pcmFrameWriter struct {
	participantID   string
	participantName string
	outputCh        chan<- AudioFrame
	sampleRate      int
	channels        int
	seqNum          uint64
}

func (w *pcmFrameWriter) String() string {
	return fmt.Sprintf("PCMFrameWriter(%s)", w.participantID)
}

func (w *pcmFrameWriter) SampleRate() int {
	return w.sampleRate
}

func (w *pcmFrameWriter) Close() error {
	return nil
}

// WriteSample receives PCM16 samples and sends them as AudioFrames.
func (w *pcmFrameWriter) WriteSample(data media.PCM16Sample) error {
	// Convert PCM16 samples to bytes (little-endian)
	pcmBytes := make([]byte, len(data)*2)
	for i, sample := range data {
		binary.LittleEndian.PutUint16(pcmBytes[i*2:], uint16(sample))
	}

	w.seqNum++
	frame := AudioFrame{
		ParticipantID:   w.participantID,
		ParticipantName: w.participantName,
		Data:            pcmBytes,
		SampleRate:      w.sampleRate,
		Channels:        w.channels,
		Timestamp:       time.Now(),
		SequenceNumber:  w.seqNum,
	}

	select {
	case w.outputCh <- frame:
	default:
		// Channel full, drop frame
	}

	return nil
}

// readAudioTrack reads from a remote audio track and sends frames to the channel.
func (a *Agent) readAudioTrack(ctx context.Context, pub *lksdk.RemoteTrackPublication, participantID, participantName string, audioCh chan<- AudioFrame) {
	defer close(audioCh)

	// Wait for the track to be subscribed
	track := pub.TrackRemote()
	if track == nil {
		// Track not yet subscribed, wait for it
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
	// Create audio reader with Opus decoding
	reader, err := newAudioReader(ctx, track, participantID, participantName, audioCh)
	if err != nil {
		return
	}
	defer reader.Close()

	// Wait for context cancellation
	<-ctx.Done()
}

// Verify interface compliance
var _ media.WriteCloser[opus.Sample] = (*opusSampleWriter)(nil)
var _ AudioWriter = (*audioWriter)(nil)
