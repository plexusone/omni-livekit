package avatar

import (
	"testing"
	"time"
)

func TestGenerateAvatarToken(t *testing.T) {
	opts := TokenOptions{
		APIKey:          "test-api-key",
		APISecret:       "test-api-secret",
		RoomName:        "test-room",
		AvatarIdentity:  "avatar-123",
		PublishOnBehalf: "agent-456",
	}

	token, err := GenerateAvatarToken(opts)
	if err != nil {
		t.Fatalf("GenerateAvatarToken failed: %v", err)
	}

	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestGenerateAvatarTokenWithName(t *testing.T) {
	opts := TokenOptions{
		APIKey:          "test-api-key",
		APISecret:       "test-api-secret",
		RoomName:        "test-room",
		AvatarIdentity:  "avatar-123",
		AvatarName:      "Friendly Avatar",
		PublishOnBehalf: "agent-456",
	}

	token, err := GenerateAvatarToken(opts)
	if err != nil {
		t.Fatalf("GenerateAvatarToken failed: %v", err)
	}

	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestGenerateAvatarTokenWithTTL(t *testing.T) {
	opts := TokenOptions{
		APIKey:          "test-api-key",
		APISecret:       "test-api-secret",
		RoomName:        "test-room",
		AvatarIdentity:  "avatar-123",
		PublishOnBehalf: "agent-456",
		TTL:             10 * time.Minute,
	}

	token, err := GenerateAvatarToken(opts)
	if err != nil {
		t.Fatalf("GenerateAvatarToken failed: %v", err)
	}

	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestGenerateAvatarTokenWithMetadata(t *testing.T) {
	opts := TokenOptions{
		APIKey:          "test-api-key",
		APISecret:       "test-api-secret",
		RoomName:        "test-room",
		AvatarIdentity:  "avatar-123",
		PublishOnBehalf: "agent-456",
		Metadata:        `{"provider":"tavus"}`,
	}

	token, err := GenerateAvatarToken(opts)
	if err != nil {
		t.Fatalf("GenerateAvatarToken failed: %v", err)
	}

	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestGenerateAvatarTokenValidation(t *testing.T) {
	tests := []struct {
		name    string
		opts    TokenOptions
		wantErr bool
	}{
		{
			name: "missing APIKey",
			opts: TokenOptions{
				APISecret:       "secret",
				RoomName:        "room",
				AvatarIdentity:  "avatar",
				PublishOnBehalf: "agent",
			},
			wantErr: true,
		},
		{
			name: "missing APISecret",
			opts: TokenOptions{
				APIKey:          "key",
				RoomName:        "room",
				AvatarIdentity:  "avatar",
				PublishOnBehalf: "agent",
			},
			wantErr: true,
		},
		{
			name: "missing RoomName",
			opts: TokenOptions{
				APIKey:          "key",
				APISecret:       "secret",
				AvatarIdentity:  "avatar",
				PublishOnBehalf: "agent",
			},
			wantErr: true,
		},
		{
			name: "missing AvatarIdentity",
			opts: TokenOptions{
				APIKey:          "key",
				APISecret:       "secret",
				RoomName:        "room",
				PublishOnBehalf: "agent",
			},
			wantErr: true,
		},
		{
			name: "missing PublishOnBehalf",
			opts: TokenOptions{
				APIKey:         "key",
				APISecret:      "secret",
				RoomName:       "room",
				AvatarIdentity: "avatar",
			},
			wantErr: true,
		},
		{
			name: "all required fields present",
			opts: TokenOptions{
				APIKey:          "key",
				APISecret:       "secret",
				RoomName:        "room",
				AvatarIdentity:  "avatar",
				PublishOnBehalf: "agent",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GenerateAvatarToken(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateAvatarToken() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultAvatarMetadata(t *testing.T) {
	meta := DefaultAvatarMetadata("tavus", "agent-123")

	if meta.Kind != "avatar" {
		t.Errorf("expected Kind 'avatar', got %s", meta.Kind)
	}
	if meta.Provider != "tavus" {
		t.Errorf("expected Provider 'tavus', got %s", meta.Provider)
	}
	if meta.AgentIdentity != "agent-123" {
		t.Errorf("expected AgentIdentity 'agent-123', got %s", meta.AgentIdentity)
	}
}
