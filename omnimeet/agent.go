// Package omnimeet provides a LiveKit implementation of the OmniMeet provider interface.
package omnimeet

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/pion/webrtc/v4"

	"github.com/plexusone/omnimeet-core/event"
	"github.com/plexusone/omnimeet-core/meeting"
	"github.com/plexusone/omnimeet-core/participant"
	"github.com/plexusone/omnimeet-core/provider"
	"github.com/plexusone/omnimeet-core/token"
	"github.com/plexusone/omnimeet-core/track"
)

// AgentParticipant implements the OmniMeet AgentParticipant interface for LiveKit.
type AgentParticipant struct {
	config      Config
	opts        provider.AgentParticipantOptions
	room        *lksdk.Room
	meeting     *meeting.Meeting
	localPart   *participant.Participant
	state       provider.ConnectionState
	eventCh     chan event.Event
	audioWriter *legacyAudioTrackWriter

	// Event handlers
	onParticipantJoined  func(participant.Participant)
	onParticipantLeft    func(participant.Participant)
	onTrackPublished     func(participant.Participant, track.Track)
	onTrackUnpublished   func(participant.Participant, track.Track)
	onActiveSpeaker      func([]participant.Participant)
	onDataMessage        func(provider.DataMessage)

	mu sync.RWMutex
}

// NewAgentParticipant creates a new AgentParticipant.
func NewAgentParticipant(cfg Config, opts provider.AgentParticipantOptions) (*AgentParticipant, error) {
	return &AgentParticipant{
		config:  cfg,
		opts:    opts,
		state:   provider.ConnectionStateDisconnected,
		eventCh: make(chan event.Event, 100),
	}, nil
}

// JoinMeeting joins the meeting as an agent participant.
func (a *AgentParticipant) JoinMeeting(ctx context.Context, meetingID string, tok *token.JoinToken) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.state == provider.ConnectionStateConnected {
		return fmt.Errorf("already connected")
	}

	a.state = provider.ConnectionStateConnecting

	// Create room callbacks
	roomCallback := &lksdk.RoomCallback{
		ParticipantCallback: lksdk.ParticipantCallback{
			OnTrackPublished: func(pub *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
				a.handleTrackPublished(pub, rp)
			},
			OnTrackUnpublished: func(pub *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
				a.handleTrackUnpublished(pub, rp)
			},
			OnTrackSubscribed: func(remoteTrack *webrtc.TrackRemote, pub *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
				a.handleTrackSubscribed(remoteTrack, pub, rp)
			},
			OnTrackUnsubscribed: func(remoteTrack *webrtc.TrackRemote, pub *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
				a.handleTrackUnsubscribed(remoteTrack, pub, rp)
			},
			OnDataPacket: func(data lksdk.DataPacket, params lksdk.DataReceiveParams) {
				a.handleDataPacket(data, params)
			},
		},
		OnParticipantConnected: func(rp *lksdk.RemoteParticipant) {
			a.handleParticipantConnected(rp)
		},
		OnParticipantDisconnected: func(rp *lksdk.RemoteParticipant) {
			a.handleParticipantDisconnected(rp)
		},
		OnActiveSpeakersChanged: func(speakers []lksdk.Participant) {
			a.handleActiveSpeakersChanged(speakers)
		},
		OnReconnecting: func() {
			a.mu.Lock()
			a.state = provider.ConnectionStateReconnecting
			a.mu.Unlock()
			a.emitEvent(event.TypeReconnecting, nil)
		},
		OnReconnected: func() {
			a.mu.Lock()
			a.state = provider.ConnectionStateConnected
			a.mu.Unlock()
			a.emitEvent(event.TypeReconnected, nil)
		},
		OnDisconnected: func() {
			a.mu.Lock()
			a.state = provider.ConnectionStateDisconnected
			a.mu.Unlock()
			a.emitEvent(event.TypeDisconnected, nil)
		},
	}

	// Connect to room
	room, err := lksdk.ConnectToRoom(a.config.ServerURL, lksdk.ConnectInfo{
		APIKey:              a.config.APIKey,
		APISecret:           a.config.APISecret,
		RoomName:            meetingID,
		ParticipantIdentity: tok.ParticipantIdentity,
		ParticipantName:     tok.ParticipantName,
	}, roomCallback)
	if err != nil {
		a.state = provider.ConnectionStateFailed
		return fmt.Errorf("failed to connect to room: %w", err)
	}

	a.room = room
	a.state = provider.ConnectionStateConnected

	// Build meeting object
	a.meeting = &meeting.Meeting{
		ID:       meetingID,
		Name:     room.Name(),
		Status:   meeting.StatusActive,
		Provider: providerName,
	}

	// Build local participant
	lp := room.LocalParticipant
	a.localPart = &participant.Participant{
		ID:        lp.Identity(),
		MeetingID: meetingID,
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
						// Log but don't fail
					}
				}
			}
		}
	}

	return nil
}

// LeaveMeeting leaves the current meeting.
func (a *AgentParticipant) LeaveMeeting(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.room == nil {
		return nil
	}

	a.room.Disconnect()
	a.room = nil
	a.state = provider.ConnectionStateDisconnected

	return nil
}

// SubscribeToAudio subscribes to a specific participant's audio.
func (a *AgentParticipant) SubscribeToAudio(ctx context.Context, participantID string) (<-chan provider.AudioFrame, error) {
	a.mu.RLock()
	room := a.room
	a.mu.RUnlock()

	if room == nil {
		return nil, fmt.Errorf("not connected")
	}

	var rp *lksdk.RemoteParticipant
	for _, p := range room.GetRemoteParticipants() {
		if p.Identity() == participantID {
			rp = p
			break
		}
	}
	if rp == nil {
		return nil, fmt.Errorf("participant not found: %s", participantID)
	}

	audioCh := make(chan provider.AudioFrame, 100)

	// Find and subscribe to audio track
	for _, pub := range rp.TrackPublications() {
		if pub.Kind() == lksdk.TrackKindAudio {
			remotePub, ok := pub.(*lksdk.RemoteTrackPublication)
			if !ok {
				continue
			}
			if err := remotePub.SetSubscribed(true); err != nil {
				return nil, fmt.Errorf("failed to subscribe to audio: %w", err)
			}

			// Set up audio sink
			// Note: In a real implementation, you'd use LiveKit's audio pipeline
			// to receive PCM frames. This is a simplified version.
			go a.readAudioTrack(ctx, remotePub, participantID, rp.Name(), audioCh)
			return audioCh, nil
		}
	}

	return nil, fmt.Errorf("no audio track found for participant: %s", participantID)
}

// SubscribeToAllAudio subscribes to all participants' audio.
func (a *AgentParticipant) SubscribeToAllAudio(ctx context.Context) (<-chan provider.AudioFrame, error) {
	a.mu.RLock()
	room := a.room
	a.mu.RUnlock()

	if room == nil {
		return nil, fmt.Errorf("not connected")
	}

	audioCh := make(chan provider.AudioFrame, 100)

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

// PublishAudio publishes an audio frame to the meeting.
func (a *AgentParticipant) PublishAudio(ctx context.Context, frame provider.AudioFrame) error {
	a.mu.RLock()
	writer := a.audioWriter
	a.mu.RUnlock()

	if writer == nil {
		return fmt.Errorf("audio track not started")
	}

	_, err := writer.Write(frame.Data)
	return err
}

// StartAudioTrack starts publishing audio and returns a writer.
func (a *AgentParticipant) StartAudioTrack(ctx context.Context, opts provider.AudioTrackOptions) (provider.AudioWriter, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.room == nil {
		return nil, fmt.Errorf("not connected")
	}

	// Create a new audio track
	sampleRate := opts.SampleRate
	if sampleRate == 0 {
		sampleRate = 48000
	}
	channels := opts.Channels
	if channels == 0 {
		channels = 1
	}

	// Use LiveKit's sample provider for audio publishing
	track, err := lksdk.NewLocalSampleTrack(webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypeOpus,
		ClockRate: uint32(sampleRate),
		Channels:  uint16(channels),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create audio track: %w", err)
	}

	// Publish the track
	_, err = a.room.LocalParticipant.PublishTrack(track, &lksdk.TrackPublicationOptions{
		Name: opts.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to publish audio track: %w", err)
	}

	// Create audio writer with Opus encoding
	audioWriterImpl, err := newAudioWriter(track, sampleRate, channels, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create audio writer: %w", err)
	}

	// Create wrapper for the provider interface
	writer := &legacyAudioTrackWriter{
		track:      track,
		sampleRate: sampleRate,
		channels:   channels,
		writer:     audioWriterImpl,
	}
	a.audioWriter = writer

	return writer, nil
}

// StopAudioTrack stops publishing audio.
func (a *AgentParticipant) StopAudioTrack(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.audioWriter != nil {
		a.audioWriter.Close()
		a.audioWriter = nil
	}

	return nil
}

// SubscribeToTrack subscribes to a specific track.
func (a *AgentParticipant) SubscribeToTrack(ctx context.Context, trackID string, opts track.SubscribeOptions) error {
	a.mu.RLock()
	room := a.room
	a.mu.RUnlock()

	if room == nil {
		return fmt.Errorf("not connected")
	}

	// Find the track across all participants
	for _, rp := range room.GetRemoteParticipants() {
		for _, pub := range rp.TrackPublications() {
			if pub.SID() == trackID {
				remotePub, ok := pub.(*lksdk.RemoteTrackPublication)
				if !ok {
					return fmt.Errorf("not a remote track")
				}
				return remotePub.SetSubscribed(true)
			}
		}
	}

	return fmt.Errorf("track not found: %s", trackID)
}

// UnsubscribeFromTrack unsubscribes from a track.
func (a *AgentParticipant) UnsubscribeFromTrack(ctx context.Context, trackID string) error {
	a.mu.RLock()
	room := a.room
	a.mu.RUnlock()

	if room == nil {
		return fmt.Errorf("not connected")
	}

	for _, rp := range room.GetRemoteParticipants() {
		for _, pub := range rp.TrackPublications() {
			if pub.SID() == trackID {
				remotePub, ok := pub.(*lksdk.RemoteTrackPublication)
				if !ok {
					return fmt.Errorf("not a remote track")
				}
				return remotePub.SetSubscribed(false)
			}
		}
	}

	return fmt.Errorf("track not found: %s", trackID)
}

// SendDataMessage sends a data message.
func (a *AgentParticipant) SendDataMessage(ctx context.Context, msg provider.DataMessage) error {
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

// OnDataMessage registers a handler for incoming data messages.
func (a *AgentParticipant) OnDataMessage(handler func(provider.DataMessage)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onDataMessage = handler
}

// Events returns a channel of real-time events.
func (a *AgentParticipant) Events() <-chan event.Event {
	return a.eventCh
}

// OnParticipantJoined registers a handler for participant join events.
func (a *AgentParticipant) OnParticipantJoined(handler func(participant.Participant)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onParticipantJoined = handler
}

// OnParticipantLeft registers a handler for participant leave events.
func (a *AgentParticipant) OnParticipantLeft(handler func(participant.Participant)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onParticipantLeft = handler
}

// OnTrackPublished registers a handler for track publish events.
func (a *AgentParticipant) OnTrackPublished(handler func(participant.Participant, track.Track)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onTrackPublished = handler
}

// OnTrackUnpublished registers a handler for track unpublish events.
func (a *AgentParticipant) OnTrackUnpublished(handler func(participant.Participant, track.Track)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onTrackUnpublished = handler
}

// OnActiveSpeakerChanged registers a handler for active speaker changes.
func (a *AgentParticipant) OnActiveSpeakerChanged(handler func([]participant.Participant)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onActiveSpeaker = handler
}

// Meeting returns the current meeting.
func (a *AgentParticipant) Meeting() *meeting.Meeting {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.meeting
}

// LocalParticipant returns the local (agent) participant.
func (a *AgentParticipant) LocalParticipant() *participant.Participant {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.localPart
}

// RemoteParticipants returns all remote participants.
func (a *AgentParticipant) RemoteParticipants() []participant.Participant {
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
		participants = append(participants, *a.mapRemoteParticipant(meetingID, rp))
	}
	return participants
}

// GetParticipant returns a specific participant by ID.
func (a *AgentParticipant) GetParticipant(participantID string) *participant.Participant {
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

	// Check if it's the local participant
	if room.LocalParticipant.Identity() == participantID {
		return a.LocalParticipant()
	}

	// Check remote participants
	var rp *lksdk.RemoteParticipant
	for _, p := range room.GetRemoteParticipants() {
		if p.Identity() == participantID {
			rp = p
			break
		}
	}
	if rp == nil {
		return nil
	}

	return a.mapRemoteParticipant(meetingID, rp)
}

// ConnectionState returns the current connection state.
func (a *AgentParticipant) ConnectionState() provider.ConnectionState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}

// Internal helpers

func (a *AgentParticipant) emitEvent(eventType event.Type, data any) {
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

func (a *AgentParticipant) handleParticipantConnected(rp *lksdk.RemoteParticipant) {
	a.mu.RLock()
	handler := a.onParticipantJoined
	meetingID := ""
	if a.meeting != nil {
		meetingID = a.meeting.ID
	}
	a.mu.RUnlock()

	part := a.mapRemoteParticipant(meetingID, rp)

	if handler != nil {
		handler(*part)
	}

	a.emitEvent(event.TypeParticipantJoined, event.ParticipantData{Participant: *part})
}

func (a *AgentParticipant) handleParticipantDisconnected(rp *lksdk.RemoteParticipant) {
	a.mu.RLock()
	handler := a.onParticipantLeft
	meetingID := ""
	if a.meeting != nil {
		meetingID = a.meeting.ID
	}
	a.mu.RUnlock()

	part := a.mapRemoteParticipant(meetingID, rp)
	now := time.Now()
	part.LeftAt = &now

	if handler != nil {
		handler(*part)
	}

	a.emitEvent(event.TypeParticipantLeft, event.ParticipantData{Participant: *part})
}

func (a *AgentParticipant) handleTrackPublished(pub *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
	a.mu.RLock()
	handler := a.onTrackPublished
	meetingID := ""
	if a.meeting != nil {
		meetingID = a.meeting.ID
	}
	a.mu.RUnlock()

	part := a.mapRemoteParticipant(meetingID, rp)
	t := a.mapPublication(rp.Identity(), pub)

	if handler != nil {
		handler(*part, *t)
	}

	a.emitEvent(event.TypeTrackPublished, event.TrackData{Participant: *part, Track: *t})

	// Auto-subscribe if enabled
	if a.opts.AutoSubscribe {
		pub.SetSubscribed(true)
	}
}

func (a *AgentParticipant) handleTrackUnpublished(pub *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
	a.mu.RLock()
	handler := a.onTrackUnpublished
	meetingID := ""
	if a.meeting != nil {
		meetingID = a.meeting.ID
	}
	a.mu.RUnlock()

	part := a.mapRemoteParticipant(meetingID, rp)
	t := a.mapPublication(rp.Identity(), pub)

	if handler != nil {
		handler(*part, *t)
	}

	a.emitEvent(event.TypeTrackUnpublished, event.TrackData{Participant: *part, Track: *t})
}

func (a *AgentParticipant) handleTrackSubscribed(remoteTrack *webrtc.TrackRemote, pub *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
	a.emitEvent(event.TypeTrackSubscribed, nil)
}

func (a *AgentParticipant) handleTrackUnsubscribed(remoteTrack *webrtc.TrackRemote, pub *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
	a.emitEvent(event.TypeTrackUnsubscribed, nil)
}

func (a *AgentParticipant) handleActiveSpeakersChanged(speakers []lksdk.Participant) {
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
			participants = append(participants, *a.mapRemoteParticipant(meetingID, rp))
		}
	}

	if handler != nil {
		handler(participants)
	}

	a.emitEvent(event.TypeActiveSpeakerChanged, event.ActiveSpeakerData{Speakers: participants})
}

func (a *AgentParticipant) handleDataPacket(data lksdk.DataPacket, params lksdk.DataReceiveParams) {
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

	// Build sender participant
	var from *participant.Participant
	if params.SenderIdentity != "" {
		from = &participant.Participant{
			ID:        params.SenderIdentity,
			MeetingID: meetingID,
			Identity:  params.SenderIdentity,
		}
	}

	msg := provider.DataMessage{
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

func (a *AgentParticipant) mapRemoteParticipant(meetingID string, rp *lksdk.RemoteParticipant) *participant.Participant {
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
		part.Tracks = append(part.Tracks, *a.mapPublication(rp.Identity(), pub))
	}

	return part
}

func (a *AgentParticipant) mapPublication(participantID string, pub lksdk.TrackPublication) *track.Track {
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

func (a *AgentParticipant) readAudioTrack(ctx context.Context, pub *lksdk.RemoteTrackPublication, participantID, participantName string, audioCh chan<- provider.AudioFrame) {
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
	reader, err := newAudioReader(ctx, track, participantID, participantName, audioCh, nil)
	if err != nil {
		// Log error but don't fail
		return
	}
	defer reader.Close()

	// Wait for context cancellation
	<-ctx.Done()
}

// legacyAudioTrackWriter is kept for backward compatibility.
// Use audioTrackWriterWithEncoding for actual Opus encoding.
type legacyAudioTrackWriter struct {
	track      *lksdk.LocalSampleTrack
	sampleRate int
	channels   int
	writer     *audioWriter
}

func (w *legacyAudioTrackWriter) Write(data []byte) (int, error) {
	if w.writer != nil {
		return w.writer.Write(data)
	}
	// Fallback: write raw data (not recommended)
	return len(data), nil
}

func (w *legacyAudioTrackWriter) Close() error {
	if w.writer != nil {
		return w.writer.Close()
	}
	return nil
}

// Verify interface compliance
var _ provider.AgentParticipant = (*AgentParticipant)(nil)
