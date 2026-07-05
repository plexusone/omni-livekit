//go:build cgo && opus
// +build cgo,opus

// Package omnimeet provides a LiveKit implementation of the OmniMeet provider interface.
package omnimeet

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/livekit/media-sdk"
	"github.com/livekit/media-sdk/opus"
	"github.com/livekit/media-sdk/rtp"
	"github.com/livekit/protocol/logger"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/pion/webrtc/v4"

	"github.com/plexusone/omnimeet-core/provider"
)

const (
	// Default sample rate for audio processing
	defaultSampleRate = 48000
	// Default frame duration in milliseconds
	defaultFrameDuration = 20
	// Number of samples per frame at 48kHz with 20ms frames
	samplesPerFrame = defaultSampleRate * defaultFrameDuration / 1000
)

// audioReader reads audio from a WebRTC track and decodes Opus to PCM.
type audioReader struct {
	ctx             context.Context
	cancel          context.CancelFunc
	track           *webrtc.TrackRemote
	participantID   string
	participantName string
	outputCh        chan<- provider.AudioFrame
	decoder         media.WriteCloser[opus.Sample]
	pcmWriter       *pcmFrameWriter
	logger          logger.Logger

	mu     sync.Mutex
	closed bool
}

// newAudioReader creates a new audio reader for a remote track.
func newAudioReader(
	ctx context.Context,
	track *webrtc.TrackRemote,
	participantID, participantName string,
	outputCh chan<- provider.AudioFrame,
	log logger.Logger,
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
	decoder, err := opus.Decode(pcmWriter, 1, log)
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
		logger:          log,
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
			r.logger.Warnw("failed to read from track", err)
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
		if err := r.decoder.WriteSample(opus.Sample(opusPayload)); err != nil {
			r.logger.Warnw("failed to decode opus sample", err)
		}
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
	outputCh        chan<- provider.AudioFrame
	sampleRate      int
	channels        int
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

	frame := provider.AudioFrame{
		ParticipantID:   w.participantID,
		ParticipantName: w.participantName,
		Data:            pcmBytes,
		SampleRate:      w.sampleRate,
		Channels:        w.channels,
		Timestamp:       time.Now(),
	}

	select {
	case w.outputCh <- frame:
	default:
		// Channel full, drop frame
	}

	return nil
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
func newAudioWriter(track *lksdk.LocalSampleTrack, sampleRate, channels int, log logger.Logger) (*audioWriter, error) {
	frameDur := time.Duration(defaultFrameDuration) * time.Millisecond

	// Create opus writer that writes to the LiveKit track
	opusWriter := &opusSampleWriter{
		track:      track,
		sampleRate: sampleRate,
		frameDur:   frameDur,
	}

	// Create Opus encoder
	encoder, err := opus.Encode(opusWriter, channels, log)
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
		webrtc.Sample{Data: data, Duration: w.frameDur},
		&lksdk.SampleWriteOptions{},
	)
}

// audioTrackWriterWithEncoding implements provider.AudioWriter with actual Opus encoding.
type audioTrackWriterWithEncoding struct {
	writer     *audioWriter
	sampleRate int
	channels   int
}

func (w *audioTrackWriterWithEncoding) Write(data []byte) (int, error) {
	return w.writer.Write(data)
}

func (w *audioTrackWriterWithEncoding) Close() error {
	return w.writer.Close()
}

// opusSampleWriter implements media.WriteCloser[opus.Sample] for the encoder output.
var _ media.WriteCloser[opus.Sample] = (*opusSampleWriter)(nil)

// Ensure opusSampleWriter implements the necessary interface
var _ interface {
	SampleRate() int
} = (*opusSampleWriter)(nil)

// rtp package constant
var _ = rtp.DefFramesPerSec
