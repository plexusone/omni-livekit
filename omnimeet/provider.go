// Package omnimeet provides a LiveKit implementation of the OmniMeet provider interface.
package omnimeet

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"

	"github.com/plexusone/omnimeet-core/event"
	"github.com/plexusone/omnimeet-core/meeting"
	"github.com/plexusone/omnimeet-core/participant"
	"github.com/plexusone/omnimeet-core/provider"
	"github.com/plexusone/omnimeet-core/token"
	"github.com/plexusone/omnimeet-core/track"
)

const providerName = "livekit"

// Config configures the LiveKit provider.
type Config struct {
	// APIKey is the LiveKit API key.
	APIKey string
	// APISecret is the LiveKit API secret.
	APISecret string
	// ServerURL is the LiveKit server URL (e.g., "wss://your-server.livekit.cloud").
	ServerURL string
	// WebhookSecret is the secret for validating webhooks.
	WebhookSecret string
}

// Provider implements the OmniMeet MeetingProvider interface for LiveKit.
type Provider struct {
	config        Config
	roomClient    *lksdk.RoomServiceClient
	egressClient  *lksdk.EgressClient
	eventHandlers []event.Handler
	mu            sync.RWMutex
}

// NewProvider creates a new LiveKit provider.
func NewProvider(cfg Config) (*Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("APIKey is required")
	}
	if cfg.APISecret == "" {
		return nil, fmt.Errorf("APISecret is required")
	}
	if cfg.ServerURL == "" {
		return nil, fmt.Errorf("ServerURL is required")
	}

	return &Provider{
		config:       cfg,
		roomClient:   lksdk.NewRoomServiceClient(cfg.ServerURL, cfg.APIKey, cfg.APISecret),
		egressClient: lksdk.NewEgressClient(cfg.ServerURL, cfg.APIKey, cfg.APISecret),
	}, nil
}

// NewProviderFromConfig creates a provider from a generic config map.
func NewProviderFromConfig(config map[string]any) (provider.MeetingProvider, error) {
	cfg := Config{}

	if v, ok := config["api_key"].(string); ok {
		cfg.APIKey = v
	}
	if v, ok := config["api_secret"].(string); ok {
		cfg.APISecret = v
	}
	if v, ok := config["server_url"].(string); ok {
		cfg.ServerURL = v
	}
	if v, ok := config["webhook_secret"].(string); ok {
		cfg.WebhookSecret = v
	}

	return NewProvider(cfg)
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return providerName
}

// CreateMeeting creates a new meeting (LiveKit room).
func (p *Provider) CreateMeeting(ctx context.Context, req meeting.CreateRequest) (*meeting.Meeting, error) {
	// Map meeting request to LiveKit room request
	lkReq := &livekit.CreateRoomRequest{
		Name: req.Name,
	}

	if req.MaxParticipants > 0 {
		lkReq.MaxParticipants = uint32(req.MaxParticipants)
	}

	if req.EmptyTimeout > 0 {
		lkReq.EmptyTimeout = uint32(req.EmptyTimeout.Seconds())
	} else {
		lkReq.EmptyTimeout = 300 // 5 minutes default
	}

	// Note: MaxDuration is not available in all LiveKit versions
	// If needed, set via room metadata or other mechanisms

	if req.Metadata != nil {
		metadataJSON, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		lkReq.Metadata = string(metadataJSON)
	}

	room, err := p.roomClient.CreateRoom(ctx, lkReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create room: %w", err)
	}

	return p.mapRoomToMeeting(room), nil
}

// GetMeeting retrieves a meeting by ID (room name in LiveKit).
func (p *Provider) GetMeeting(ctx context.Context, meetingID string) (*meeting.Meeting, error) {
	rooms, err := p.roomClient.ListRooms(ctx, &livekit.ListRoomsRequest{
		Names: []string{meetingID},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get room: %w", err)
	}

	if len(rooms.Rooms) == 0 {
		return nil, fmt.Errorf("meeting not found: %s", meetingID)
	}

	return p.mapRoomToMeeting(rooms.Rooms[0]), nil
}

// ListMeetings returns a list of active meetings.
func (p *Provider) ListMeetings(ctx context.Context, opts meeting.ListOptions) ([]meeting.Meeting, error) {
	rooms, err := p.roomClient.ListRooms(ctx, &livekit.ListRoomsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list rooms: %w", err)
	}

	meetings := make([]meeting.Meeting, 0, len(rooms.Rooms))
	for _, room := range rooms.Rooms {
		m := p.mapRoomToMeeting(room)

		// Apply status filter
		if opts.Status != nil && m.Status != *opts.Status {
			continue
		}

		meetings = append(meetings, *m)

		// Apply limit
		if opts.Limit > 0 && len(meetings) >= opts.Limit {
			break
		}
	}

	return meetings, nil
}

// EndMeeting ends a meeting.
func (p *Provider) EndMeeting(ctx context.Context, meetingID string) error {
	_, err := p.roomClient.DeleteRoom(ctx, &livekit.DeleteRoomRequest{
		Room: meetingID,
	})
	if err != nil {
		return fmt.Errorf("failed to end meeting: %w", err)
	}
	return nil
}

// DeleteMeeting deletes a meeting (same as EndMeeting for LiveKit).
func (p *Provider) DeleteMeeting(ctx context.Context, meetingID string) error {
	return p.EndMeeting(ctx, meetingID)
}

// CreateJoinToken generates a token for a participant to join a meeting.
func (p *Provider) CreateJoinToken(ctx context.Context, req token.CreateRequest) (*token.JoinToken, error) {
	ttl := req.TTL
	if ttl == 0 {
		ttl = token.DefaultTTL
	}

	at := auth.NewAccessToken(p.config.APIKey, p.config.APISecret)

	// Set up video grant based on participant kind and permissions
	grant := &auth.VideoGrant{
		RoomJoin: true,
		Room:     req.MeetingID,
	}

	perms := req.Participant.Permissions
	if perms == nil {
		// Use default permissions based on kind
		switch req.Participant.Kind {
		case participant.KindHuman:
			perms = participant.DefaultHumanPermissions()
		case participant.KindAgent:
			perms = participant.DefaultAgentPermissions()
		case participant.KindObserver:
			perms = participant.DefaultObserverPermissions()
		case participant.KindRecorder:
			perms = participant.DefaultRecorderPermissions()
		default:
			perms = participant.DefaultHumanPermissions()
		}
	}

	grant.CanPublish = &perms.CanPublish
	grant.CanSubscribe = &perms.CanSubscribe
	grant.CanPublishData = &perms.CanPublishData
	grant.CanUpdateOwnMetadata = &perms.CanUpdateMetadata
	grant.Hidden = perms.Hidden
	grant.Recorder = perms.Recorder

	// Build identity
	identity := req.Participant.Identity
	if identity == "" {
		identity = fmt.Sprintf("%s-%d", req.Participant.Kind, time.Now().UnixNano())
	}

	// Build metadata
	metadata := make(map[string]string)
	metadata["kind"] = string(req.Participant.Kind)
	for k, v := range req.Participant.Metadata {
		metadata[k] = v
	}
	for k, v := range req.Metadata {
		metadata[k] = v
	}
	metadataJSON, _ := json.Marshal(metadata)

	at.SetVideoGrant(grant).
		SetIdentity(identity).
		SetName(req.Participant.Name).
		SetMetadata(string(metadataJSON)).
		SetValidFor(ttl)

	tokenStr, err := at.ToJWT()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	expiresAt := time.Now().Add(ttl)

	return &token.JoinToken{
		Token:               tokenStr,
		MeetingID:           req.MeetingID,
		ParticipantIdentity: identity,
		ParticipantName:     req.Participant.Name,
		ExpiresAt:           expiresAt,
		JoinURL:             fmt.Sprintf("%s?room=%s&token=%s", p.config.ServerURL, req.MeetingID, tokenStr),
		Metadata:            metadata,
	}, nil
}

// ListParticipants returns the current participants in a meeting.
func (p *Provider) ListParticipants(ctx context.Context, meetingID string) ([]participant.Participant, error) {
	res, err := p.roomClient.ListParticipants(ctx, &livekit.ListParticipantsRequest{
		Room: meetingID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list participants: %w", err)
	}

	participants := make([]participant.Participant, 0, len(res.Participants))
	for _, lkp := range res.Participants {
		participants = append(participants, *p.mapParticipant(meetingID, lkp))
	}

	return participants, nil
}

// GetParticipant retrieves a specific participant.
func (p *Provider) GetParticipant(ctx context.Context, meetingID, participantID string) (*participant.Participant, error) {
	lkp, err := p.roomClient.GetParticipant(ctx, &livekit.RoomParticipantIdentity{
		Room:     meetingID,
		Identity: participantID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get participant: %w", err)
	}

	return p.mapParticipant(meetingID, lkp), nil
}

// RemoveParticipant removes a participant from a meeting.
func (p *Provider) RemoveParticipant(ctx context.Context, meetingID, participantID string) error {
	_, err := p.roomClient.RemoveParticipant(ctx, &livekit.RoomParticipantIdentity{
		Room:     meetingID,
		Identity: participantID,
	})
	if err != nil {
		return fmt.Errorf("failed to remove participant: %w", err)
	}
	return nil
}

// UpdateParticipant updates participant metadata or permissions.
func (p *Provider) UpdateParticipant(ctx context.Context, meetingID, participantID string, update provider.ParticipantUpdate) error {
	req := &livekit.UpdateParticipantRequest{
		Room:     meetingID,
		Identity: participantID,
	}

	if update.Name != nil {
		req.Name = *update.Name
	}

	if update.Metadata != nil {
		metadataJSON, _ := json.Marshal(update.Metadata)
		req.Metadata = string(metadataJSON)
	}

	if update.Permissions != nil {
		req.Permission = &livekit.ParticipantPermission{
			CanPublish:        update.Permissions.CanPublish,
			CanSubscribe:      update.Permissions.CanSubscribe,
			CanPublishData:    update.Permissions.CanPublishData,
			CanUpdateMetadata: update.Permissions.CanUpdateMetadata,
			Hidden:            update.Permissions.Hidden,
			Recorder:          update.Permissions.Recorder,
		}
	}

	_, err := p.roomClient.UpdateParticipant(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to update participant: %w", err)
	}
	return nil
}

// OnEvent registers a handler for meeting events.
func (p *Provider) OnEvent(handler event.Handler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.eventHandlers = append(p.eventHandlers, handler)
}

// Close releases resources held by the provider.
func (p *Provider) Close() error {
	return nil
}

// SupportsAgentParticipation returns true (LiveKit supports agent participation).
func (p *Provider) SupportsAgentParticipation() bool {
	return true
}

// CreateAgentParticipant creates a new AgentParticipant.
func (p *Provider) CreateAgentParticipant(opts provider.AgentParticipantOptions) (provider.AgentParticipant, error) {
	return NewAgentParticipant(p.config, opts)
}

// mapRoomToMeeting converts a LiveKit room to an OmniMeet meeting.
func (p *Provider) mapRoomToMeeting(room *livekit.Room) *meeting.Meeting {
	m := &meeting.Meeting{
		ID:               room.Name,
		Name:             room.Name,
		Provider:         providerName,
		ParticipantCount: int(room.NumParticipants),
		MaxParticipants:  int(room.MaxParticipants),
		CreatedAt:        time.Unix(room.CreationTime, 0),
	}

	// Determine status
	if room.NumParticipants > 0 {
		m.Status = meeting.StatusActive
		startedAt := time.Unix(room.CreationTime, 0)
		m.StartedAt = &startedAt
	} else {
		m.Status = meeting.StatusPending
	}

	// Parse metadata
	if room.Metadata != "" {
		var metadata map[string]string
		if err := json.Unmarshal([]byte(room.Metadata), &metadata); err == nil {
			m.Metadata = metadata
		}
	}

	return m
}

// mapParticipant converts a LiveKit participant to an OmniMeet participant.
func (p *Provider) mapParticipant(meetingID string, lkp *livekit.ParticipantInfo) *participant.Participant {
	part := &participant.Participant{
		ID:        lkp.Identity,
		MeetingID: meetingID,
		Name:      lkp.Name,
		Identity:  lkp.Identity,
		JoinedAt:  time.Unix(lkp.JoinedAt, 0),
	}

	// Determine kind from metadata
	part.Kind = participant.KindHuman
	if lkp.Metadata != "" {
		var metadata map[string]string
		if err := json.Unmarshal([]byte(lkp.Metadata), &metadata); err == nil {
			part.Metadata = metadata
			if kind, ok := metadata["kind"]; ok {
				part.Kind = participant.Kind(kind)
			}
		}
	}

	// Map tracks
	for _, pub := range lkp.Tracks {
		part.Tracks = append(part.Tracks, *p.mapTrack(lkp.Identity, pub))
	}

	// Map permissions
	if lkp.Permission != nil {
		part.Permissions = &participant.Permissions{
			CanPublish:        lkp.Permission.CanPublish,
			CanSubscribe:      lkp.Permission.CanSubscribe,
			CanPublishData:    lkp.Permission.CanPublishData,
			CanUpdateMetadata: lkp.Permission.CanUpdateMetadata,
			Hidden:            lkp.Permission.Hidden,
			Recorder:          lkp.Permission.Recorder,
		}
	}

	// Map connection quality
	// part.IsSpeaking = lkp.IsSpeaking // Not available in ParticipantInfo

	return part
}

// mapTrack converts a LiveKit track to an OmniMeet track.
func (p *Provider) mapTrack(participantID string, pub *livekit.TrackInfo) *track.Track {
	t := &track.Track{
		ID:            pub.Sid,
		ParticipantID: participantID,
		Name:          pub.Name,
		Muted:         pub.Muted,
		Simulcast:     pub.Simulcast,
	}

	// Map track type
	switch pub.Type {
	case livekit.TrackType_AUDIO:
		t.Kind = track.KindAudio
		t.Source = track.SourceMicrophone
	case livekit.TrackType_VIDEO:
		if pub.Source == livekit.TrackSource_SCREEN_SHARE {
			t.Kind = track.KindScreenShare
			t.Source = track.SourceScreen
		} else {
			t.Kind = track.KindVideo
			t.Source = track.SourceCamera
		}
		t.Width = int(pub.Width)
		t.Height = int(pub.Height)
	default:
		t.Kind = track.KindData
		t.Source = track.SourceUnknown
	}

	return t
}

// Verify interface compliance
var (
	_ provider.MeetingProvider       = (*Provider)(nil)
	_ provider.AgentParticipantFactory = (*Provider)(nil)
)
