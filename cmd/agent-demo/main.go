// Command agent-demo demonstrates a LiveKit AI agent that joins a room
// and interacts with human participants.
//
// Usage:
//
//	export LIVEKIT_URL="wss://your-project.livekit.cloud"
//	export LIVEKIT_API_KEY="your-api-key"
//	export LIVEKIT_API_SECRET="your-api-secret"
//	go run ./cmd/agent-demo
//
// Then open the printed URL in your browser to join as a human participant.
package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/plexusone/omnimeet-core/participant"

	"github.com/plexusone/omni-livekit/agent"
	"github.com/plexusone/omni-livekit/room"
)

func main() {
	// Load config from environment
	serverURL := os.Getenv("LIVEKIT_URL")
	apiKey := os.Getenv("LIVEKIT_API_KEY")
	apiSecret := os.Getenv("LIVEKIT_API_SECRET")

	if serverURL == "" || apiKey == "" || apiSecret == "" {
		log.Fatal("Required environment variables: LIVEKIT_URL, LIVEKIT_API_KEY, LIVEKIT_API_SECRET")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Create room client
	roomClient, err := room.NewClient(room.Config{
		APIKey:    apiKey,
		APISecret: apiSecret,
		URL:       serverURL,
	})
	if err != nil {
		log.Fatalf("Failed to create room client: %v", err)
	}

	// Create a unique room name
	roomName := fmt.Sprintf("demo-%d", time.Now().Unix())

	// Create the room
	fmt.Printf("Creating room: %s\n", roomName)
	_, err = roomClient.CreateRoom(ctx, roomName)
	if err != nil {
		log.Fatalf("Failed to create room: %v", err)
	}

	// Generate token for human participant
	humanToken, err := roomClient.GenerateClientToken(roomName, "human-user", "You")
	if err != nil {
		log.Fatalf("Failed to generate human token: %v", err)
	}

	// Build LiveKit Meet URL
	meetURL := buildMeetURL(serverURL, humanToken)

	fmt.Println()
	fmt.Println("===========================================")
	fmt.Println("  LiveKit Agent Demo")
	fmt.Println("===========================================")
	fmt.Println()
	fmt.Printf("Room: %s\n", roomName)
	fmt.Println()
	fmt.Println("Join as human participant:")
	fmt.Println()
	fmt.Printf("  %s\n", meetURL)
	fmt.Println()
	fmt.Println("Or use LiveKit Playground with this token:")
	fmt.Printf("  Token: %s...\n", humanToken[:50])
	fmt.Println()
	fmt.Println("Starting agent...")
	fmt.Println()

	// Create and start agent
	ag, err := agent.New(agent.Options{
		APIKey:        apiKey,
		APISecret:     apiSecret,
		ServerURL:     serverURL,
		Identity:      "ai-agent",
		Name:          "AI Assistant",
		AutoSubscribe: true,
		Audio: agent.AudioConfig{
			SampleRate: 48000,
			Channels:   1,
			TrackName:  "agent-audio",
		},
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Set up event handlers
	ag.OnParticipantJoined(func(p participant.Participant) {
		fmt.Printf("[Event] Participant joined: %s\n", p.Name)
	})

	ag.OnParticipantLeft(func(p participant.Participant) {
		fmt.Printf("[Event] Participant left: %s\n", p.Name)
	})

	ag.OnAudioFrame(func(frame agent.AudioFrame) {
		// Audio frame received from participant
		// In a real implementation, this would go to STT
		fmt.Printf("[Audio] Received %d bytes from %s\n", len(frame.Data), frame.ParticipantName)
	})

	// Join the room
	if err := ag.Join(ctx, roomName); err != nil {
		log.Fatalf("Failed to join room: %v", err)
	}
	fmt.Println("Agent joined room successfully!")
	fmt.Println()
	fmt.Println("Waiting for participants... (Ctrl+C to exit)")

	// Subscribe to all audio
	audioCh, err := ag.SubscribeToAllAudio(ctx)
	if err != nil {
		log.Printf("Warning: Could not subscribe to audio: %v", err)
	} else {
		go func() {
			for frame := range audioCh {
				fmt.Printf("[Audio] %s: %d bytes @ %dHz\n",
					frame.ParticipantName, len(frame.Data), frame.SampleRate)
			}
		}()
	}

	// Process events
	go func() {
		for evt := range ag.Events() {
			fmt.Printf("[Event] %s: %v\n", evt.Type, evt.Data)
		}
	}()

	// Wait for shutdown
	<-ctx.Done()

	// Leave room
	if err := ag.Leave(ctx); err != nil {
		log.Printf("Error leaving room: %v", err)
	}

	// Clean up room
	if err := roomClient.DeleteRoom(context.Background(), roomName); err != nil {
		log.Printf("Error deleting room: %v", err)
	}

	fmt.Println("Goodbye!")
}

// buildMeetURL creates a URL to join via LiveKit Meet
func buildMeetURL(serverURL, token string) string {
	// LiveKit Meet expects the server URL without wss:// prefix
	// Format: https://meet.livekit.io/custom?liveKitUrl=<server>&token=<token>
	u, _ := url.Parse("https://meet.livekit.io/custom")
	q := u.Query()
	q.Set("liveKitUrl", serverURL)
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String()
}
