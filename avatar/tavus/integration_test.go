//go:build integration

package tavus

import (
	"context"
	"os"
	"testing"
	"time"
)

// Integration tests require TAVUS_API_KEY environment variable.
// Run with: go test -tags=integration -v ./avatar/tavus/...

func TestIntegrationCreateConversation(t *testing.T) {
	apiKey := os.Getenv("TAVUS_API_KEY")
	if apiKey == "" {
		t.Skip("TAVUS_API_KEY not set, skipping integration test")
	}

	client, err := NewClient(ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a conversation with minimal config
	// Note: This will fail without valid LiveKit credentials,
	// but we can verify the API accepts the request
	resp, err := client.CreateConversation(ctx, CreateConversationRequest{
		PalID: DefaultPalID,
		Properties: map[string]string{
			"livekit_ws_url":     "wss://test.livekit.cloud",
			"livekit_room_token": "test-token",
		},
		ConversationName: "integration-test-conv",
	})

	// The API should accept the request even if LiveKit connection fails
	if err != nil {
		// Check if it's an auth error (invalid key) vs other error
		t.Logf("CreateConversation error (may be expected): %v", err)

		// Auth errors should fail the test
		if provErr, ok := err.(*ProviderError); ok {
			if provErr.Err == ErrProviderAuthFailed {
				t.Fatalf("authentication failed - check TAVUS_API_KEY")
			}
		}
		// Other errors might be expected (invalid LiveKit credentials)
		return
	}

	t.Logf("Created conversation: %s", resp.ConversationID)

	// Clean up - end the conversation
	if resp.ConversationID != "" {
		err = client.EndConversation(ctx, resp.ConversationID)
		if err != nil {
			t.Logf("EndConversation error (may be expected): %v", err)
		} else {
			t.Logf("Ended conversation: %s", resp.ConversationID)
		}
	}
}

func TestIntegrationAPIKeyValidation(t *testing.T) {
	apiKey := os.Getenv("TAVUS_API_KEY")
	if apiKey == "" {
		t.Skip("TAVUS_API_KEY not set, skipping integration test")
	}

	// Test with invalid API key
	client, err := NewClient(ClientConfig{
		APIKey: "invalid-api-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = client.CreateConversation(ctx, CreateConversationRequest{
		PalID: DefaultPalID,
	})

	if err == nil {
		t.Fatal("expected error with invalid API key")
	}

	t.Logf("Got expected error with invalid key: %v", err)
}

func TestIntegrationNewSession(t *testing.T) {
	apiKey := os.Getenv("TAVUS_API_KEY")
	if apiKey == "" {
		t.Skip("TAVUS_API_KEY not set, skipping integration test")
	}

	session, err := NewSession(SessionConfig{
		APIKey: apiKey,
		PalID:  DefaultPalID,
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	t.Logf("Created session with avatar identity: %s", session.AvatarIdentity())
	t.Logf("Provider: %s", session.Provider())

	// Verify session is properly configured
	if session.Provider() != "tavus" {
		t.Errorf("expected provider 'tavus', got '%s'", session.Provider())
	}

	if session.palID != DefaultPalID {
		t.Errorf("expected PAL ID '%s', got '%s'", DefaultPalID, session.palID)
	}
}
