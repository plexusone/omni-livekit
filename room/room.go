// Package room provides utilities for working with LiveKit rooms.
package room

import (
	"context"
	"fmt"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

// Config configures room creation and token generation.
type Config struct {
	APIKey    string
	APISecret string
	URL       string
}

// Client provides room management operations.
type Client struct {
	config Config
	room   *lksdk.RoomServiceClient
}

// NewClient creates a new room client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" || cfg.APISecret == "" || cfg.URL == "" {
		return nil, fmt.Errorf("APIKey, APISecret, and URL are required")
	}

	roomClient := lksdk.NewRoomServiceClient(cfg.URL, cfg.APIKey, cfg.APISecret)

	return &Client{
		config: cfg,
		room:   roomClient,
	}, nil
}

// CreateRoom creates a new room.
func (c *Client) CreateRoom(ctx context.Context, name string) (*livekit.Room, error) {
	return c.room.CreateRoom(ctx, &livekit.CreateRoomRequest{
		Name:            name,
		EmptyTimeout:    300, // 5 minutes
		MaxParticipants: 10,
	})
}

// DeleteRoom deletes a room.
func (c *Client) DeleteRoom(ctx context.Context, name string) error {
	_, err := c.room.DeleteRoom(ctx, &livekit.DeleteRoomRequest{
		Room: name,
	})
	return err
}

// ListRooms lists all rooms.
func (c *Client) ListRooms(ctx context.Context) ([]*livekit.Room, error) {
	res, err := c.room.ListRooms(ctx, &livekit.ListRoomsRequest{})
	if err != nil {
		return nil, err
	}
	return res.Rooms, nil
}

// ListParticipants lists participants in a room.
func (c *Client) ListParticipants(ctx context.Context, roomName string) ([]*livekit.ParticipantInfo, error) {
	res, err := c.room.ListParticipants(ctx, &livekit.ListParticipantsRequest{
		Room: roomName,
	})
	if err != nil {
		return nil, err
	}
	return res.Participants, nil
}

// TokenOptions configures token generation.
type TokenOptions struct {
	Identity   string
	Name       string
	RoomName   string
	CanPublish bool
	TTL        time.Duration
	Metadata   string
}

// GenerateToken creates a JWT token for joining a room.
func (c *Client) GenerateToken(opts TokenOptions) (string, error) {
	if opts.Identity == "" {
		return "", fmt.Errorf("identity is required")
	}
	if opts.RoomName == "" {
		return "", fmt.Errorf("room name is required")
	}
	if opts.TTL == 0 {
		opts.TTL = 6 * time.Hour
	}

	at := auth.NewAccessToken(c.config.APIKey, c.config.APISecret)

	grant := &auth.VideoGrant{
		RoomJoin: true,
		Room:     opts.RoomName,
	}
	if opts.CanPublish {
		grant.CanPublish = &opts.CanPublish
	}

	at.SetVideoGrant(grant).
		SetIdentity(opts.Identity).
		SetName(opts.Name).
		SetMetadata(opts.Metadata).
		SetValidFor(opts.TTL)

	return at.ToJWT()
}

// GenerateAgentToken creates a token for the AI agent to join a room.
func (c *Client) GenerateAgentToken(roomName, identity, name string) (string, error) {
	canPublish := true
	return c.GenerateToken(TokenOptions{
		Identity:   identity,
		Name:       name,
		RoomName:   roomName,
		CanPublish: canPublish,
		TTL:        24 * time.Hour,
		Metadata:   `{"type":"ai-agent"}`,
	})
}

// GenerateClientToken creates a token for a client to join a room.
func (c *Client) GenerateClientToken(roomName, identity, name string) (string, error) {
	canPublish := true
	return c.GenerateToken(TokenOptions{
		Identity:   identity,
		Name:       name,
		RoomName:   roomName,
		CanPublish: canPublish,
		TTL:        6 * time.Hour,
	})
}
