package tavus

import (
	"testing"

	"github.com/plexusone/omni-livekit/avatar"
)

func TestNewClient(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		client, err := NewClient(ClientConfig{
			APIKey: "test-api-key",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client == nil {
			t.Fatal("expected non-nil client")
		}
		if client.sdk == nil {
			t.Fatal("expected non-nil SDK client")
		}
	})

	t.Run("missing API key", func(t *testing.T) {
		_, err := NewClient(ClientConfig{})
		if err != avatar.ErrInvalidConfig {
			t.Errorf("expected ErrInvalidConfig, got %v", err)
		}
	})

	t.Run("custom base URL", func(t *testing.T) {
		client, err := NewClient(ClientConfig{
			APIKey:  "test-api-key",
			BaseURL: "https://custom.api.com",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client == nil {
			t.Fatal("expected non-nil client")
		}
		// Can't easily verify the base URL since it's encapsulated in the SDK
	})

	t.Run("returns SDK accessor", func(t *testing.T) {
		client, err := NewClient(ClientConfig{
			APIKey: "test-api-key",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client.SDK() == nil {
			t.Fatal("expected non-nil SDK from accessor")
		}
	})
}

func TestCreateConversationRequest(t *testing.T) {
	t.Run("struct fields", func(t *testing.T) {
		req := CreateConversationRequest{
			PalID:            "pal-123",
			FaceID:           "face-456",
			LiveKitURL:       "wss://lk.example.com",
			LiveKitToken:     "token-xyz",
			ConversationName: "test-conv",
		}

		if req.PalID != "pal-123" {
			t.Errorf("expected PalID pal-123, got %s", req.PalID)
		}
		if req.FaceID != "face-456" {
			t.Errorf("expected FaceID face-456, got %s", req.FaceID)
		}
		if req.LiveKitURL != "wss://lk.example.com" {
			t.Errorf("expected LiveKitURL wss://lk.example.com, got %s", req.LiveKitURL)
		}
		if req.LiveKitToken != "token-xyz" {
			t.Errorf("expected LiveKitToken token-xyz, got %s", req.LiveKitToken)
		}
		if req.ConversationName != "test-conv" {
			t.Errorf("expected ConversationName test-conv, got %s", req.ConversationName)
		}
	})
}

func TestCreatePalRequest(t *testing.T) {
	t.Run("struct fields", func(t *testing.T) {
		req := CreatePalRequest{
			PalName:       "Test PAL",
			DefaultFaceID: "face-789",
			PipelineMode:  "echo",
			TransportType: "livekit",
		}

		if req.PalName != "Test PAL" {
			t.Errorf("expected PalName Test PAL, got %s", req.PalName)
		}
		if req.DefaultFaceID != "face-789" {
			t.Errorf("expected DefaultFaceID face-789, got %s", req.DefaultFaceID)
		}
		if req.PipelineMode != "echo" {
			t.Errorf("expected PipelineMode echo, got %s", req.PipelineMode)
		}
		if req.TransportType != "livekit" {
			t.Errorf("expected TransportType livekit, got %s", req.TransportType)
		}
	})
}

func TestClientCreatePalValidation(t *testing.T) {
	t.Run("missing face ID", func(t *testing.T) {
		client, _ := NewClient(ClientConfig{
			APIKey: "test-api-key",
		})

		_, err := client.CreatePal(nil, CreatePalRequest{})
		if err != avatar.ErrInvalidConfig {
			t.Errorf("expected ErrInvalidConfig, got %v", err)
		}
	})
}

func TestClientEndConversationValidation(t *testing.T) {
	t.Run("missing conversation ID", func(t *testing.T) {
		client, _ := NewClient(ClientConfig{
			APIKey: "test-api-key",
		})

		err := client.EndConversation(nil, "")
		if err != avatar.ErrInvalidConfig {
			t.Errorf("expected ErrInvalidConfig, got %v", err)
		}
	})
}

func TestDefaultPalID(t *testing.T) {
	if DefaultPalID == "" {
		t.Error("expected non-empty DefaultPalID")
	}
}
