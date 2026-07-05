package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/pion/webrtc/v4"
	pionmedia "github.com/pion/webrtc/v4/pkg/media"

	"github.com/plexusone/omnimeet-core/event"
	"github.com/plexusone/omnimeet-core/meeting"
	"github.com/plexusone/omnimeet-core/participant"
	"github.com/plexusone/omnimeet-core/track"

	"github.com/plexusone/omni-livekit/agent/assets"
)

// ConnectionState represents the agent's connection state.
type ConnectionState string

const (
	StateDisconnected ConnectionState = "disconnected"
	StateConnecting   ConnectionState = "connecting"
	StateConnected    ConnectionState = "connected"
	StateReconnecting ConnectionState = "reconnecting"
	StateFailed       ConnectionState = "failed"
)

// Agent is a full-featured LiveKit agent with audio and video capabilities.
type Agent struct {
	opts    Options
	room    *lksdk.Room
	meeting *meeting.Meeting
	local   *participant.Participant
	state   ConnectionState
	eventCh chan event.Event

	// Media tracks
	audioTrack  *lksdk.LocalSampleTrack
	videoTrack  *lksdk.LocalSampleTrack
	audioWriter *audioWriter
	videoWriter *videoWriter
	imageWriter ImageWriter // interface for static image publishing

	// Event handlers
	onParticipantJoined func(participant.Participant)
	onParticipantLeft   func(participant.Participant)
	onTrackPublished    func(participant.Participant, track.Track)
	onTrackUnpublished  func(participant.Participant, track.Track)
	onActiveSpeaker     func([]participant.Participant)
	onDataMessage       func(DataMessage)
	onAudioFrame        func(AudioFrame)

	mu sync.RWMutex
}

// DataMessage represents a data channel message.
type DataMessage struct {
	Topic          string
	Payload        []byte
	From           *participant.Participant
	DestinationIDs []string
	Reliable       bool
	Timestamp      time.Time
}

// AudioFrame represents a received audio frame.
type AudioFrame struct {
	ParticipantID   string
	ParticipantName string
	Data            []byte // PCM16 little-endian
	SampleRate      int
	Channels        int
	Timestamp       time.Time
	SequenceNumber  uint64
}

// New creates a new LiveKit agent with the given options.
func New(opts Options) (*Agent, error) {
	opts.applyDefaults()

	if opts.APIKey == "" {
		return nil, fmt.Errorf("APIKey is required")
	}
	if opts.APISecret == "" {
		return nil, fmt.Errorf("APISecret is required")
	}
	if opts.ServerURL == "" {
		return nil, fmt.Errorf("ServerURL is required")
	}

	if opts.Identity == "" {
		opts.Identity = uuid.New().String()
	}

	return &Agent{
		opts:    opts,
		state:   StateDisconnected,
		eventCh: make(chan event.Event, 100),
	}, nil
}

// Join connects the agent to a meeting room.
func (a *Agent) Join(ctx context.Context, roomName string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.state == StateConnected {
		return fmt.Errorf("already connected")
	}

	a.state = StateConnecting

	// Build metadata
	metadata := map[string]string{"kind": "agent"}
	for k, v := range a.opts.Metadata {
		metadata[k] = v
	}
	metadataJSON, _ := json.Marshal(metadata)

	// Create room callbacks
	roomCallback := &lksdk.RoomCallback{
		ParticipantCallback: lksdk.ParticipantCallback{
			OnTrackPublished:    a.handleTrackPublished,
			OnTrackUnpublished:  a.handleTrackUnpublished,
			OnTrackSubscribed:   a.handleTrackSubscribed,
			OnTrackUnsubscribed: a.handleTrackUnsubscribed,
			OnDataPacket:        a.handleDataPacket,
		},
		OnParticipantConnected:    a.handleParticipantConnected,
		OnParticipantDisconnected: a.handleParticipantDisconnected,
		OnActiveSpeakersChanged:   a.handleActiveSpeakersChanged,
		OnReconnecting:            a.handleReconnecting,
		OnReconnected:             a.handleReconnected,
		OnDisconnected:            a.handleDisconnected,
	}

	// Connect to room
	room, err := lksdk.ConnectToRoom(a.opts.ServerURL, lksdk.ConnectInfo{
		APIKey:              a.opts.APIKey,
		APISecret:           a.opts.APISecret,
		RoomName:            roomName,
		ParticipantIdentity: a.opts.Identity,
		ParticipantName:     a.opts.Name,
		ParticipantMetadata: string(metadataJSON),
	}, roomCallback)
	if err != nil {
		a.state = StateFailed
		return fmt.Errorf("failed to connect to room: %w", err)
	}

	a.room = room
	a.state = StateConnected

	// Build meeting object
	a.meeting = &meeting.Meeting{
		ID:       roomName,
		Name:     room.Name(),
		Status:   meeting.StatusActive,
		Provider: "livekit",
	}

	// Build local participant
	lp := room.LocalParticipant
	a.local = &participant.Participant{
		ID:        lp.Identity(),
		MeetingID: roomName,
		Name:      lp.Name(),
		Identity:  lp.Identity(),
		Kind:      participant.KindAgent,
		JoinedAt:  time.Now(),
	}

	// Auto-subscribe if enabled
	if a.opts.AutoSubscribe {
		for _, rp := range room.GetRemoteParticipants() {
			for _, pub := range rp.TrackPublications() {
				if remotePub, ok := pub.(*lksdk.RemoteTrackPublication); ok {
					if err := remotePub.SetSubscribed(true); err != nil {
						slog.Default().Warn("failed to auto-subscribe to track", "error", err)
					}
				}
			}
		}
	}

	// Start video based on media mode
	if a.opts.MediaMode == AudioWithImage {
		var err error
		// Check for pre-encoded H.264 data first (no CGO required at runtime)
		if len(a.opts.Image.H264Data) > 0 || a.opts.Image.H264Path != "" {
			err = a.startPreencodedImageVideo(ctx)
		} else if len(a.opts.Image.Data) > 0 || a.opts.Image.Path != "" {
			// Runtime encoding from provided image (requires CGO)
			err = a.startImageVideoLocked(ctx)
		} else {
			// No image configured - use embedded default avatar
			a.opts.Image.H264Data = assets.DefaultAvatarH264
			err = a.startPreencodedImageVideo(ctx)
		}
		if err != nil {
			// Log but don't fail join - image video is optional
			// Error is silently ignored; caller can check if video is available
			_ = err
		}
	}

	return nil
}

// Leave disconnects the agent from the meeting.
func (a *Agent) Leave(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.room == nil {
		return nil
	}

	// Stop media tracks
	if a.audioWriter != nil {
		a.audioWriter.Close()
		a.audioWriter = nil
	}
	if a.videoWriter != nil {
		a.videoWriter.Close()
		a.videoWriter = nil
	}
	if a.imageWriter != nil {
		a.imageWriter.Close()
		a.imageWriter = nil
	}

	a.room.Disconnect()
	a.room = nil
	a.state = StateDisconnected

	return nil
}

// StartAudio starts publishing audio and returns a writer for PCM16 data.
func (a *Agent) StartAudio(ctx context.Context) (AudioWriter, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.room == nil {
		return nil, fmt.Errorf("not connected")
	}

	if a.audioWriter != nil {
		return nil, fmt.Errorf("audio already started")
	}

	// Create audio track
	// Note: Opus in WebRTC always uses 48kHz clock rate and stereo (2 channels) signaling,
	// regardless of the actual audio sample rate or channel count. The browser expects
	// exactly these values in SDP negotiation.
	track, err := lksdk.NewLocalSampleTrack(webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypeOpus,
		ClockRate: 48000,
		Channels:  2,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create audio track: %w", err)
	}

	// Publish the track
	_, err = a.room.LocalParticipant.PublishTrack(track, &lksdk.TrackPublicationOptions{
		Name: a.opts.Audio.TrackName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to publish audio track: %w", err)
	}

	// Create audio writer with Opus encoding
	writer, err := newAudioWriter(track, a.opts.Audio.SampleRate, a.opts.Audio.Channels)
	if err != nil {
		return nil, fmt.Errorf("failed to create audio writer: %w", err)
	}

	a.audioTrack = track
	a.audioWriter = writer

	return writer, nil
}

// StopAudio stops publishing audio.
func (a *Agent) StopAudio(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.audioWriter != nil {
		a.audioWriter.Close()
		a.audioWriter = nil
	}
	a.audioTrack = nil

	return nil
}

// WriteOpusDirect writes pre-encoded Opus data directly to the audio track.
// This bypasses local Opus encoding - use when TTS returns Opus format.
// Each call should contain one Opus frame (typically 20ms of audio).
func (a *Agent) WriteOpusDirect(data []byte, duration time.Duration) error {
	a.mu.RLock()
	track := a.audioTrack
	a.mu.RUnlock()

	if track == nil {
		return fmt.Errorf("audio track not started")
	}

	return track.WriteSample(
		pionmedia.Sample{Data: data, Duration: duration},
		nil,
	)
}

// StartVideo starts publishing video and returns a writer for video frames.
// Only available in AudioWithVideo mode.
func (a *Agent) StartVideo(ctx context.Context) (VideoWriter, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.room == nil {
		return nil, fmt.Errorf("not connected")
	}

	if a.opts.MediaMode != AudioWithVideo {
		return nil, fmt.Errorf("video only available in AudioWithVideo mode")
	}

	if a.videoWriter != nil {
		return nil, fmt.Errorf("video already started")
	}

	writer, err := a.startVideoLocked(ctx)
	if err != nil {
		return nil, err
	}

	return writer, nil
}

// StopVideo stops publishing video.
func (a *Agent) StopVideo(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.videoWriter != nil {
		a.videoWriter.Close()
		a.videoWriter = nil
	}
	a.videoTrack = nil

	return nil
}

// UpdateImage updates the static image being published (AudioWithImage mode).
func (a *Agent) UpdateImage(ctx context.Context, imageData []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.opts.MediaMode != AudioWithImage {
		return fmt.Errorf("image only available in AudioWithImage mode")
	}

	if a.imageWriter == nil {
		return fmt.Errorf("image video not started")
	}

	return a.imageWriter.UpdateImage(imageData)
}

// State returns the current connection state.
func (a *Agent) State() ConnectionState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}

// Meeting returns the current meeting info.
func (a *Agent) Meeting() *meeting.Meeting {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.meeting
}

// LocalParticipant returns the agent's participant info.
func (a *Agent) LocalParticipant() *participant.Participant {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.local
}

// RemoteParticipants returns all other participants in the meeting.
func (a *Agent) RemoteParticipants() []participant.Participant {
	a.mu.RLock()
	room := a.room
	meetingID := ""
	if a.meeting != nil {
		meetingID = a.meeting.ID
	}
	a.mu.RUnlock()

	if room == nil {
		return nil
	}

	var participants []participant.Participant
	for _, rp := range room.GetRemoteParticipants() {
		participants = append(participants, *mapRemoteParticipant(meetingID, rp))
	}
	return participants
}

// GetParticipant returns a specific participant by identity.
func (a *Agent) GetParticipant(identity string) *participant.Participant {
	a.mu.RLock()
	room := a.room
	meetingID := ""
	if a.meeting != nil {
		meetingID = a.meeting.ID
	}
	local := a.local
	a.mu.RUnlock()

	if room == nil {
		return nil
	}

	// Check if it's the local participant
	if local != nil && local.Identity == identity {
		return local
	}

	// Check remote participants
	for _, p := range room.GetRemoteParticipants() {
		if p.Identity() == identity {
			return mapRemoteParticipant(meetingID, p)
		}
	}
	return nil
}

// Events returns a channel of real-time events.
func (a *Agent) Events() <-chan event.Event {
	return a.eventCh
}

// SendData sends a data channel message.
func (a *Agent) SendData(ctx context.Context, msg DataMessage) error {
	a.mu.RLock()
	room := a.room
	a.mu.RUnlock()

	if room == nil {
		return fmt.Errorf("not connected")
	}

	var dataOpts []lksdk.DataPublishOption
	if msg.Reliable {
		dataOpts = append(dataOpts, lksdk.WithDataPublishReliable(true))
	}
	if msg.Topic != "" {
		dataOpts = append(dataOpts, lksdk.WithDataPublishTopic(msg.Topic))
	}
	if len(msg.DestinationIDs) > 0 {
		dataOpts = append(dataOpts, lksdk.WithDataPublishDestination(msg.DestinationIDs))
	}

	return room.LocalParticipant.PublishDataPacket(lksdk.UserData(msg.Payload), dataOpts...)
}

// SubscribeToAudio subscribes to a participant's audio and returns a channel of frames.
func (a *Agent) SubscribeToAudio(ctx context.Context, participantID string) (<-chan AudioFrame, error) {
	a.mu.RLock()
	room := a.room
	a.mu.RUnlock()

	if room == nil {
		return nil, fmt.Errorf("not connected")
	}

	fmt.Printf("[AUDIO-DEBUG] SubscribeToAudio looking for participant: %s\n", participantID)
	participants := room.GetRemoteParticipants()
	fmt.Printf("[AUDIO-DEBUG] Found %d remote participants\n", len(participants))
	for _, p := range participants {
		fmt.Printf("[AUDIO-DEBUG]   - %s (identity: %s)\n", p.Name(), p.Identity())
	}

	var rp *lksdk.RemoteParticipant
	for _, p := range participants {
		if p.Identity() == participantID {
			rp = p
			break
		}
	}
	if rp == nil {
		return nil, fmt.Errorf("participant not found: %s", participantID)
	}

	audioCh := make(chan AudioFrame, 100)

	// Find and subscribe to audio track
	pubs := rp.TrackPublications()
	fmt.Printf("[AUDIO-DEBUG] Participant %s has %d track publications\n", rp.Name(), len(pubs))
	for _, pub := range pubs {
		fmt.Printf("[AUDIO-DEBUG]   - Track %s kind=%s muted=%v\n", pub.SID(), pub.Kind(), pub.IsMuted())
		if pub.Kind() == lksdk.TrackKindAudio {
			remotePub, ok := pub.(*lksdk.RemoteTrackPublication)
			if !ok {
				fmt.Printf("[AUDIO-DEBUG]   - Not a RemoteTrackPublication, skipping\n")
				continue
			}
			fmt.Printf("[AUDIO-DEBUG] Subscribing to audio track %s from %s\n", pub.SID(), rp.Name())
			if err := remotePub.SetSubscribed(true); err != nil {
				return nil, fmt.Errorf("failed to subscribe to audio: %w", err)
			}

			fmt.Printf("[AUDIO-DEBUG] Starting readAudioTrack goroutine for %s\n", rp.Name())
			go a.readAudioTrack(ctx, remotePub, participantID, rp.Name(), audioCh)
			return audioCh, nil
		}
	}

	return nil, fmt.Errorf("no audio track found for participant: %s", participantID)
}

// SubscribeToAllAudio subscribes to all participants' audio.
func (a *Agent) SubscribeToAllAudio(ctx context.Context) (<-chan AudioFrame, error) {
	a.mu.RLock()
	room := a.room
	a.mu.RUnlock()

	if room == nil {
		return nil, fmt.Errorf("not connected")
	}

	audioCh := make(chan AudioFrame, 100)

	// Subscribe to all current participants
	for _, rp := range room.GetRemoteParticipants() {
		for _, pub := range rp.TrackPublications() {
			if pub.Kind() == lksdk.TrackKindAudio {
				remotePub, ok := pub.(*lksdk.RemoteTrackPublication)
				if !ok {
					continue
				}
				if err := remotePub.SetSubscribed(true); err != nil {
					continue
				}
				go a.readAudioTrack(ctx, remotePub, rp.Identity(), rp.Name(), audioCh)
			}
		}
	}

	return audioCh, nil
}

// Event handler setters

func (a *Agent) OnParticipantJoined(handler func(participant.Participant)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onParticipantJoined = handler
}

func (a *Agent) OnParticipantLeft(handler func(participant.Participant)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onParticipantLeft = handler
}

func (a *Agent) OnTrackPublished(handler func(participant.Participant, track.Track)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onTrackPublished = handler
}

func (a *Agent) OnTrackUnpublished(handler func(participant.Participant, track.Track)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onTrackUnpublished = handler
}

func (a *Agent) OnActiveSpeakerChanged(handler func([]participant.Participant)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onActiveSpeaker = handler
}

func (a *Agent) OnDataMessage(handler func(DataMessage)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onDataMessage = handler
}

func (a *Agent) OnAudioFrame(handler func(AudioFrame)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onAudioFrame = handler
}

// Internal methods

func (a *Agent) emitEvent(eventType event.Type, data any) {
	evt := event.Event{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
	}

	if a.meeting != nil {
		evt.MeetingID = a.meeting.ID
	}

	select {
	case a.eventCh <- evt:
	default:
		// Channel full, drop event
	}
}

func (a *Agent) handleParticipantConnected(rp *lksdk.RemoteParticipant) {
	a.mu.RLock()
	handler := a.onParticipantJoined
	meetingID := ""
	if a.meeting != nil {
		meetingID = a.meeting.ID
	}
	autoSub := a.opts.AutoSubscribe
	a.mu.RUnlock()

	part := mapRemoteParticipant(meetingID, rp)

	if handler != nil {
		handler(*part)
	}

	a.emitEvent(event.TypeParticipantJoined, event.ParticipantData{Participant: *part})

	// Auto-subscribe to new participant's tracks
	if autoSub {
		for _, pub := range rp.TrackPublications() {
			if remotePub, ok := pub.(*lksdk.RemoteTrackPublication); ok {
				if err := remotePub.SetSubscribed(true); err != nil {
					slog.Default().Warn("failed to auto-subscribe to track", "error", err)
				}
			}
		}
	}
}

func (a *Agent) handleParticipantDisconnected(rp *lksdk.RemoteParticipant) {
	a.mu.RLock()
	handler := a.onParticipantLeft
	meetingID := ""
	if a.meeting != nil {
		meetingID = a.meeting.ID
	}
	a.mu.RUnlock()

	part := mapRemoteParticipant(meetingID, rp)
	now := time.Now()
	part.LeftAt = &now

	if handler != nil {
		handler(*part)
	}

	a.emitEvent(event.TypeParticipantLeft, event.ParticipantData{Participant: *part})
}

func (a *Agent) handleTrackPublished(pub *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
	a.mu.RLock()
	handler := a.onTrackPublished
	meetingID := ""
	if a.meeting != nil {
		meetingID = a.meeting.ID
	}
	autoSub := a.opts.AutoSubscribe
	a.mu.RUnlock()

	part := mapRemoteParticipant(meetingID, rp)
	t := mapPublication(rp.Identity(), pub)

	if handler != nil {
		handler(*part, *t)
	}

	a.emitEvent(event.TypeTrackPublished, event.TrackData{Participant: *part, Track: *t})

	// Auto-subscribe if enabled
	if autoSub {
		if err := pub.SetSubscribed(true); err != nil {
			slog.Default().Warn("failed to auto-subscribe to track", "error", err)
		}
	}
}

func (a *Agent) handleTrackUnpublished(pub *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
	a.mu.RLock()
	handler := a.onTrackUnpublished
	meetingID := ""
	if a.meeting != nil {
		meetingID = a.meeting.ID
	}
	a.mu.RUnlock()

	part := mapRemoteParticipant(meetingID, rp)
	t := mapPublication(rp.Identity(), pub)

	if handler != nil {
		handler(*part, *t)
	}

	a.emitEvent(event.TypeTrackUnpublished, event.TrackData{Participant: *part, Track: *t})
}

func (a *Agent) handleTrackSubscribed(remoteTrack *webrtc.TrackRemote, pub *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
	a.emitEvent(event.TypeTrackSubscribed, nil)
}

func (a *Agent) handleTrackUnsubscribed(remoteTrack *webrtc.TrackRemote, pub *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
	a.emitEvent(event.TypeTrackUnsubscribed, nil)
}

func (a *Agent) handleActiveSpeakersChanged(speakers []lksdk.Participant) {
	a.mu.RLock()
	handler := a.onActiveSpeaker
	meetingID := ""
	if a.meeting != nil {
		meetingID = a.meeting.ID
	}
	a.mu.RUnlock()

	var participants []participant.Participant
	for _, sp := range speakers {
		rp, ok := sp.(*lksdk.RemoteParticipant)
		if ok {
			participants = append(participants, *mapRemoteParticipant(meetingID, rp))
		}
	}

	if handler != nil {
		handler(participants)
	}

	a.emitEvent(event.TypeActiveSpeakerChanged, event.ActiveSpeakerData{Speakers: participants})
}

func (a *Agent) handleDataPacket(data lksdk.DataPacket, params lksdk.DataReceiveParams) {
	a.mu.RLock()
	handler := a.onDataMessage
	meetingID := ""
	if a.meeting != nil {
		meetingID = a.meeting.ID
	}
	a.mu.RUnlock()

	userData, ok := data.(*lksdk.UserDataPacket)
	if !ok {
		return
	}

	var from *participant.Participant
	if params.SenderIdentity != "" {
		from = &participant.Participant{
			ID:        params.SenderIdentity,
			MeetingID: meetingID,
			Identity:  params.SenderIdentity,
		}
	}

	msg := DataMessage{
		Topic:     params.Topic,
		Payload:   userData.Payload,
		From:      from,
		Timestamp: time.Now(),
	}

	if handler != nil {
		handler(msg)
	}

	a.emitEvent(event.TypeDataMessageReceived, event.DataMessageData{
		From:      *from,
		Topic:     params.Topic,
		Payload:   userData.Payload,
		Timestamp: time.Now(),
	})
}

func (a *Agent) handleReconnecting() {
	a.mu.Lock()
	a.state = StateReconnecting
	a.mu.Unlock()
	a.emitEvent(event.TypeReconnecting, nil)
}

func (a *Agent) handleReconnected() {
	a.mu.Lock()
	a.state = StateConnected
	a.mu.Unlock()
	a.emitEvent(event.TypeReconnected, nil)
}

func (a *Agent) handleDisconnected() {
	a.mu.Lock()
	a.state = StateDisconnected
	a.mu.Unlock()
	a.emitEvent(event.TypeDisconnected, nil)
}

// Helper functions

func mapRemoteParticipant(meetingID string, rp *lksdk.RemoteParticipant) *participant.Participant {
	part := &participant.Participant{
		ID:         rp.Identity(),
		MeetingID:  meetingID,
		Name:       rp.Name(),
		Identity:   rp.Identity(),
		Kind:       participant.KindHuman,
		IsSpeaking: rp.IsSpeaking(),
	}

	// Parse metadata for kind
	if rp.Metadata() != "" {
		var metadata map[string]string
		if err := json.Unmarshal([]byte(rp.Metadata()), &metadata); err == nil {
			part.Metadata = metadata
			if kind, ok := metadata["kind"]; ok {
				part.Kind = participant.Kind(kind)
			}
		}
	}

	// Map tracks
	for _, pub := range rp.TrackPublications() {
		part.Tracks = append(part.Tracks, *mapPublication(rp.Identity(), pub))
	}

	return part
}

func mapPublication(participantID string, pub lksdk.TrackPublication) *track.Track {
	t := &track.Track{
		ID:            pub.SID(),
		ParticipantID: participantID,
		Name:          pub.Name(),
		Muted:         pub.IsMuted(),
	}

	switch pub.Kind() {
	case lksdk.TrackKindAudio:
		t.Kind = track.KindAudio
		t.Source = track.SourceMicrophone
	case lksdk.TrackKindVideo:
		if pub.Source() == livekit.TrackSource_SCREEN_SHARE {
			t.Kind = track.KindScreenShare
			t.Source = track.SourceScreen
		} else {
			t.Kind = track.KindVideo
			t.Source = track.SourceCamera
		}
	}

	return t
}
