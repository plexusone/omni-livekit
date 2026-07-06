package avatar

import (
	"time"

	"github.com/livekit/protocol/auth"
)

// TokenOptions configures avatar token generation.
type TokenOptions struct {
	// APIKey is the LiveKit API key.
	// Required.
	APIKey string

	// APISecret is the LiveKit API secret.
	// Required.
	APISecret string

	// RoomName is the room the avatar will join.
	// Required.
	RoomName string

	// AvatarIdentity is the participant identity for the avatar.
	// Required.
	AvatarIdentity string

	// AvatarName is the display name for the avatar participant.
	// Optional, defaults to AvatarIdentity.
	AvatarName string

	// PublishOnBehalf is the identity of the agent participant.
	// The avatar will publish tracks that appear as if they're from this participant.
	// Required.
	PublishOnBehalf string

	// TTL is the token validity duration.
	// Default: 5 minutes
	TTL time.Duration

	// Metadata is optional participant metadata.
	Metadata string
}

// GenerateAvatarToken creates a JWT token for an avatar to join a room.
//
// The token includes the special "lk.publish_on_behalf" attribute that allows
// the avatar participant to publish tracks that appear in the UI as if they
// came from the agent participant.
func GenerateAvatarToken(opts TokenOptions) (string, error) {
	if opts.APIKey == "" || opts.APISecret == "" {
		return "", ErrInvalidConfig
	}
	if opts.RoomName == "" || opts.AvatarIdentity == "" {
		return "", ErrInvalidConfig
	}
	if opts.PublishOnBehalf == "" {
		return "", ErrInvalidConfig
	}

	ttl := opts.TTL
	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	name := opts.AvatarName
	if name == "" {
		name = opts.AvatarIdentity
	}

	at := auth.NewAccessToken(opts.APIKey, opts.APISecret)

	grant := &auth.VideoGrant{
		RoomJoin: true,
		Room:     opts.RoomName,
	}

	at.SetVideoGrant(grant).
		SetIdentity(opts.AvatarIdentity).
		SetName(name).
		SetValidFor(ttl).
		SetAttributes(map[string]string{
			// This attribute allows the avatar to publish tracks
			// that appear as if they're from the agent
			"lk.publish_on_behalf": opts.PublishOnBehalf,
		})

	if opts.Metadata != "" {
		at.SetMetadata(opts.Metadata)
	}

	return at.ToJWT()
}

// AvatarMetadata is the standard metadata structure for avatar participants.
type AvatarMetadata struct {
	// Kind identifies this as an avatar participant.
	Kind string `json:"kind"`

	// Provider is the avatar provider name.
	Provider string `json:"provider,omitempty"`

	// AgentIdentity is the identity of the agent this avatar represents.
	AgentIdentity string `json:"agent_identity,omitempty"`
}

// DefaultAvatarMetadata returns the default metadata for avatar participants.
func DefaultAvatarMetadata(provider, agentIdentity string) AvatarMetadata {
	return AvatarMetadata{
		Kind:          "avatar",
		Provider:      provider,
		AgentIdentity: agentIdentity,
	}
}
