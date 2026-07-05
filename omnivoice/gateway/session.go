package gateway

import (
	"errors"
	"sync"
	"time"

	"github.com/livekit/media-sdk"
	protoLogger "github.com/livekit/protocol/logger"
	lksdk "github.com/livekit/server-sdk-go/v2"
	lkmedia "github.com/livekit/server-sdk-go/v2/pkg/media"
	"github.com/pion/webrtc/v4"

	coregateway "github.com/plexusone/omnivoice-core/gateway"
)

// Verify interface compliance at compile time.
var _ coregateway.WebRTCSession = (*Session)(nil)

// Session represents an active voice conversation with a LiveKit participant.
type Session struct {
	id              string
	gateway         *Gateway
	participant     *lksdk.RemoteParticipant
	participantInfo *coregateway.ParticipantInfo
	roomName        string
	agentIdentity   string
	startTime       time.Time

	events chan coregateway.Event
	done   chan struct{}

	// Audio handling
	remoteTrack *lkmedia.PCMRemoteTrack
	audioWriter *AudioWriter

	// Conversation state
	mu         sync.RWMutex
	transcript []coregateway.Turn
	metrics    sessionMetrics
	closed     bool

	logger protoLogger.Logger
}

type sessionMetrics struct {
	userSpeechDurationMs  int
	agentSpeechDurationMs int
	sttLatencies          []int
	llmLatencies          []int
	ttsLatencies          []int
	interruptionCount     int
	toolCallCount         int
	errorCount            int
	audioBytesReceived    int64
	audioBytesSent        int64
}

// AudioWriter receives PCM16 samples from a remote participant.
type AudioWriter struct {
	session *Session
	mu      sync.Mutex
}

// WriteSample implements the PCMRemoteTrackWriter interface.
func (w *AudioWriter) WriteSample(sample media.PCM16Sample) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.session == nil {
		return errors.New("session closed")
	}

	// Track audio bytes received
	w.session.mu.Lock()
	w.session.metrics.audioBytesReceived += int64(len(sample) * 2) // 2 bytes per sample
	w.session.mu.Unlock()

	// Send audio received event
	w.session.sendEvent(coregateway.Event{
		Type:      coregateway.EventAudioReceived,
		Timestamp: time.Now(),
		Data: map[string]any{
			"samples": len(sample),
		},
	})

	// TODO: Process audio through STT → LLM → TTS pipeline
	// This is where the voice AI processing would happen

	return nil
}

// Close implements the PCMRemoteTrackWriter interface.
func (w *AudioWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.session = nil
	return nil
}

// ID returns the session identifier (participant identity).
func (s *Session) ID() string {
	return s.id
}

// Participant returns information about the remote participant.
func (s *Session) Participant() *coregateway.ParticipantInfo {
	return s.participantInfo
}

// RoomName returns the room this session is in.
func (s *Session) RoomName() string {
	return s.roomName
}

// AgentIdentity returns the identity of the AI agent.
func (s *Session) AgentIdentity() string {
	return s.agentIdentity
}

// StartTime returns when the session started.
func (s *Session) StartTime() time.Time {
	return s.startTime
}

// Duration returns the session duration.
func (s *Session) Duration() time.Duration {
	return time.Since(s.startTime)
}

// Events returns a channel for session events.
func (s *Session) Events() <-chan coregateway.Event {
	return s.events
}

// Transcript returns the conversation transcript.
func (s *Session) Transcript() []coregateway.Turn {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]coregateway.Turn, len(s.transcript))
	copy(result, s.transcript)
	return result
}

// Metrics returns session performance metrics.
func (s *Session) Metrics() coregateway.Metrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	avgSTT := average(s.metrics.sttLatencies)
	avgLLM := average(s.metrics.llmLatencies)
	avgTTS := average(s.metrics.ttsLatencies)

	return coregateway.Metrics{
		SessionDurationMs:     int(s.Duration().Milliseconds()),
		TurnCount:             len(s.transcript),
		UserSpeechDurationMs:  s.metrics.userSpeechDurationMs,
		AgentSpeechDurationMs: s.metrics.agentSpeechDurationMs,
		AvgSTTLatencyMs:       avgSTT,
		AvgLLMLatencyMs:       avgLLM,
		AvgTTSLatencyMs:       avgTTS,
		AvgTotalLatencyMs:     avgSTT + avgLLM + avgTTS,
		InterruptionCount:     s.metrics.interruptionCount,
		ToolCallCount:         s.metrics.toolCallCount,
		ErrorCount:            s.metrics.errorCount,
		AudioBytesReceived:    s.metrics.audioBytesReceived,
		AudioBytesSent:        s.metrics.audioBytesSent,
	}
}

// SendText sends text input to the agent (bypasses STT).
func (s *Session) SendText(text string) error {
	s.mu.RLock()
	closed := s.closed
	s.mu.RUnlock()

	if closed {
		return errors.New("session closed")
	}

	// Add to transcript
	s.mu.Lock()
	s.transcript = append(s.transcript, coregateway.Turn{
		Role:      "user",
		Text:      text,
		Timestamp: time.Now(),
	})
	s.mu.Unlock()

	// Send user transcript event
	s.sendEvent(coregateway.Event{
		Type:      coregateway.EventUserTranscript,
		Timestamp: time.Now(),
		Data:      text,
	})

	// TODO: Process through LLM → TTS pipeline

	return nil
}

// SendAudio sends PCM16 audio samples to the participant.
func (s *Session) SendAudio(samples []int16) error {
	if s.gateway.localTrack == nil {
		return errors.New("no local track available")
	}

	s.mu.Lock()
	s.metrics.audioBytesSent += int64(len(samples) * 2) // 2 bytes per sample
	s.mu.Unlock()

	return s.gateway.localTrack.WriteSample(media.PCM16Sample(samples))
}

// Interrupt stops the current agent speech.
func (s *Session) Interrupt() {
	s.mu.Lock()
	s.metrics.interruptionCount++
	s.mu.Unlock()

	// Clear the audio queue
	if s.gateway.localTrack != nil {
		s.gateway.localTrack.ClearQueue()
	}

	s.sendEvent(coregateway.Event{
		Type:      coregateway.EventInterruption,
		Timestamp: time.Now(),
	})
}

// Close ends the session.
func (s *Session) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	// Close remote track
	if s.remoteTrack != nil {
		s.remoteTrack.Close()
	}

	// Close audio writer
	if s.audioWriter != nil {
		_ = s.audioWriter.Close()
	}

	// Close event channel
	close(s.done)

	// Remove from gateway
	s.gateway.removeSession(s.id)

	s.logger.Infow("session closed",
		"duration", s.Duration(),
		"turns", len(s.transcript))

	return nil
}

// handleTrackSubscribed sets up audio processing for the participant's track.
func (s *Session) handleTrackSubscribed(track *webrtc.TrackRemote) {
	if track.Codec().MimeType != webrtc.MimeTypeOpus {
		s.logger.Warnw("unsupported codec", nil, "codec", track.Codec().MimeType)
		return
	}

	s.audioWriter = &AudioWriter{session: s}

	remoteTrack, err := lkmedia.NewPCMRemoteTrack(
		track,
		s.audioWriter,
		lkmedia.WithTargetSampleRate(s.gateway.config.SampleRate),
		lkmedia.WithTargetChannels(s.gateway.config.Channels),
		lkmedia.WithLogger(s.logger),
	)
	if err != nil {
		s.logger.Errorw("failed to create remote track", err)
		return
	}

	s.remoteTrack = remoteTrack
	s.logger.Infow("audio track subscribed",
		"sample_rate", s.gateway.config.SampleRate)
}

// handleTrackUnsubscribed cleans up when the participant's track is unsubscribed.
func (s *Session) handleTrackUnsubscribed(track *webrtc.TrackRemote) {
	if s.remoteTrack != nil {
		s.remoteTrack.Close()
		s.remoteTrack = nil
	}

	s.logger.Infow("audio track unsubscribed")
}

// sendEvent sends an event to the session's event channel.
func (s *Session) sendEvent(event coregateway.Event) {
	select {
	case s.events <- event:
	case <-s.done:
	default:
		// Channel full, drop event
		s.logger.Warnw("event channel full, dropping event", nil, "type", event.Type)
	}
}

// average calculates the average of a slice of integers.
func average(values []int) int {
	if len(values) == 0 {
		return 0
	}
	sum := 0
	for _, v := range values {
		sum += v
	}
	return sum / len(values)
}
