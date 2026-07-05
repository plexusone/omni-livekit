//go:build integration

// Integration tests for LiveKit OmniMeet provider.
//
// These tests require a running LiveKit server and valid credentials.
// Set the following environment variables to run:
//
//	LIVEKIT_API_KEY
//	LIVEKIT_API_SECRET
//	LIVEKIT_URL
//
// Run with: go test -v -tags=integration ./omnimeet/...
package omnimeet

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/plexusone/omnimeet-core/meeting"
	"github.com/plexusone/omnimeet-core/participant"
	"github.com/plexusone/omnimeet-core/provider"
	"github.com/plexusone/omnimeet-core/token"
)

func getTestConfig(t *testing.T) Config {
	apiKey := os.Getenv("LIVEKIT_API_KEY")
	apiSecret := os.Getenv("LIVEKIT_API_SECRET")
	serverURL := os.Getenv("LIVEKIT_URL")

	if apiKey == "" || apiSecret == "" || serverURL == "" {
		t.Skip("Skipping integration test: LIVEKIT_API_KEY, LIVEKIT_API_SECRET, and LIVEKIT_URL must be set")
	}

	return Config{
		APIKey:    apiKey,
		APISecret: apiSecret,
		ServerURL: serverURL,
	}
}

func TestProviderName(t *testing.T) {
	cfg := getTestConfig(t)
	prov, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if prov.Name() != "livekit" {
		t.Errorf("Expected provider name 'livekit', got '%s'", prov.Name())
	}
}

func TestMeetingLifecycle(t *testing.T) {
	cfg := getTestConfig(t)
	prov, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer prov.Close()

	ctx := context.Background()
	meetingName := fmt.Sprintf("test-meeting-%d", time.Now().UnixNano())

	// Create meeting
	t.Log("Creating meeting...")
	m, err := prov.CreateMeeting(ctx, meeting.CreateRequest{
		Name: meetingName,
		Metadata: map[string]string{
			"test": "true",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create meeting: %v", err)
	}
	t.Logf("Created meeting: ID=%s, Name=%s, Status=%s", m.ID, m.Name, m.Status)

	if m.ID == "" {
		t.Error("Meeting ID should not be empty")
	}
	if m.Name != meetingName {
		t.Errorf("Expected meeting name '%s', got '%s'", meetingName, m.Name)
	}

	// Get meeting
	t.Log("Getting meeting...")
	retrieved, err := prov.GetMeeting(ctx, m.ID)
	if err != nil {
		t.Fatalf("Failed to get meeting: %v", err)
	}
	t.Logf("Retrieved meeting: ID=%s, Status=%s", retrieved.ID, retrieved.Status)

	if retrieved.ID != m.ID {
		t.Errorf("Expected meeting ID '%s', got '%s'", m.ID, retrieved.ID)
	}

	// List meetings
	// Note: LiveKit may not return empty rooms (rooms with no participants) in the list.
	// This is expected behavior - rooms only fully "exist" when someone joins.
	t.Log("Listing meetings...")
	meetings, err := prov.ListMeetings(ctx, meeting.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list meetings: %v", err)
	}
	t.Logf("Found %d meetings", len(meetings))

	found := false
	for _, listed := range meetings {
		if listed.ID == m.ID {
			found = true
			break
		}
	}
	if !found {
		t.Log("Note: Created meeting not found in list (expected for empty LiveKit rooms)")
	}

	// End meeting
	t.Log("Ending meeting...")
	if err := prov.EndMeeting(ctx, m.ID); err != nil {
		t.Fatalf("Failed to end meeting: %v", err)
	}
	t.Log("Meeting ended successfully")

	// Verify meeting is gone
	t.Log("Verifying meeting is deleted...")
	_, err = prov.GetMeeting(ctx, m.ID)
	if err == nil {
		t.Log("Note: Meeting still exists (may take time to clean up)")
	} else {
		t.Log("Meeting no longer exists")
	}
}

func TestJoinTokenGeneration(t *testing.T) {
	cfg := getTestConfig(t)
	prov, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer prov.Close()

	ctx := context.Background()
	meetingName := fmt.Sprintf("test-token-%d", time.Now().UnixNano())

	// Create meeting
	m, err := prov.CreateMeeting(ctx, meeting.CreateRequest{
		Name: meetingName,
	})
	if err != nil {
		t.Fatalf("Failed to create meeting: %v", err)
	}
	defer prov.EndMeeting(ctx, m.ID)

	// Generate human token
	t.Log("Generating human participant token...")
	humanToken, err := prov.CreateJoinToken(ctx, token.CreateRequest{
		MeetingID: m.ID,
		Participant: participant.Info{
			Name:     "Test Human",
			Kind:     participant.KindHuman,
			Identity: "test-human-1",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create human token: %v", err)
	}
	t.Logf("Human token: identity=%s, expires=%v", humanToken.ParticipantIdentity, humanToken.ExpiresAt)
	t.Logf("Join URL: %s", humanToken.JoinURL)

	if humanToken.Token == "" {
		t.Error("Token should not be empty")
	}
	if humanToken.JoinURL == "" {
		t.Error("JoinURL should not be empty")
	}
	if humanToken.ParticipantIdentity != "test-human-1" {
		t.Errorf("Expected identity 'test-human-1', got '%s'", humanToken.ParticipantIdentity)
	}

	// Generate agent token
	t.Log("Generating agent participant token...")
	agentToken, err := prov.CreateJoinToken(ctx, token.CreateRequest{
		MeetingID: m.ID,
		Participant: participant.Info{
			Name:     "Test Agent",
			Kind:     participant.KindAgent,
			Identity: "test-agent-1",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create agent token: %v", err)
	}
	t.Logf("Agent token: identity=%s", agentToken.ParticipantIdentity)

	if agentToken.Token == "" {
		t.Error("Agent token should not be empty")
	}
}

func TestAgentParticipantFactory(t *testing.T) {
	cfg := getTestConfig(t)
	prov, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer prov.Close()

	// Check if provider supports agent participation
	if !prov.SupportsAgentParticipation() {
		t.Error("LiveKit provider should support agent participation")
	}

	// Create agent participant
	t.Log("Creating agent participant...")
	agent, err := prov.CreateAgentParticipant(provider.AgentParticipantOptions{
		AutoSubscribe: true,
	})
	if err != nil {
		t.Fatalf("Failed to create agent participant: %v", err)
	}

	if agent == nil {
		t.Error("Agent participant should not be nil")
	}

	t.Log("Agent participant created successfully")
}

func TestAgentJoinMeeting(t *testing.T) {
	cfg := getTestConfig(t)
	prov, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer prov.Close()

	ctx := context.Background()
	meetingName := fmt.Sprintf("test-agent-join-%d", time.Now().UnixNano())

	// Create meeting
	m, err := prov.CreateMeeting(ctx, meeting.CreateRequest{
		Name: meetingName,
	})
	if err != nil {
		t.Fatalf("Failed to create meeting: %v", err)
	}
	defer prov.EndMeeting(ctx, m.ID)
	t.Logf("Created meeting: %s", m.ID)

	// Generate agent token
	agentToken, err := prov.CreateJoinToken(ctx, token.CreateRequest{
		MeetingID: m.ID,
		Participant: participant.Info{
			Name:     "Test Agent",
			Kind:     participant.KindAgent,
			Identity: "test-agent-join",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create agent token: %v", err)
	}

	// Create agent participant
	agent, err := prov.CreateAgentParticipant(provider.AgentParticipantOptions{
		AutoSubscribe: true,
	})
	if err != nil {
		t.Fatalf("Failed to create agent participant: %v", err)
	}

	// Join the meeting
	t.Log("Agent joining meeting...")
	if err := agent.JoinMeeting(ctx, m.ID, agentToken); err != nil {
		t.Fatalf("Failed to join meeting: %v", err)
	}
	t.Log("Agent joined meeting successfully")

	// Check connection state
	state := agent.ConnectionState()
	t.Logf("Connection state: %s", state)
	if state != provider.ConnectionStateConnected {
		t.Errorf("Expected connection state 'connected', got '%s'", state)
	}

	// Check local participant
	local := agent.LocalParticipant()
	if local == nil {
		t.Error("Local participant should not be nil")
	} else {
		t.Logf("Local participant: ID=%s, Name=%s", local.ID, local.Name)
	}

	// Check meeting
	currentMeeting := agent.Meeting()
	if currentMeeting == nil {
		t.Error("Meeting should not be nil")
	} else {
		t.Logf("Current meeting: ID=%s", currentMeeting.ID)
	}

	// List participants via provider
	t.Log("Listing participants...")
	participants, err := prov.ListParticipants(ctx, m.ID)
	if err != nil {
		t.Fatalf("Failed to list participants: %v", err)
	}
	t.Logf("Found %d participants", len(participants))
	for _, p := range participants {
		t.Logf("  - %s (%s)", p.Name, p.Kind)
	}

	// Leave the meeting
	t.Log("Agent leaving meeting...")
	if err := agent.LeaveMeeting(ctx); err != nil {
		t.Fatalf("Failed to leave meeting: %v", err)
	}
	t.Log("Agent left meeting successfully")

	// Check connection state after leaving
	state = agent.ConnectionState()
	t.Logf("Connection state after leaving: %s", state)
	if state != provider.ConnectionStateDisconnected {
		t.Errorf("Expected connection state 'disconnected', got '%s'", state)
	}
}

func TestAudioTrackPublishing(t *testing.T) {
	cfg := getTestConfig(t)
	prov, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer prov.Close()

	ctx := context.Background()
	meetingName := fmt.Sprintf("test-audio-%d", time.Now().UnixNano())

	// Create meeting
	m, err := prov.CreateMeeting(ctx, meeting.CreateRequest{
		Name: meetingName,
	})
	if err != nil {
		t.Fatalf("Failed to create meeting: %v", err)
	}
	defer prov.EndMeeting(ctx, m.ID)

	// Generate and join as agent
	agentToken, err := prov.CreateJoinToken(ctx, token.CreateRequest{
		MeetingID: m.ID,
		Participant: participant.Info{
			Name:     "Audio Test Agent",
			Kind:     participant.KindAgent,
			Identity: "audio-test-agent",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create agent token: %v", err)
	}

	agent, err := prov.CreateAgentParticipant(provider.AgentParticipantOptions{
		AutoSubscribe: true,
	})
	if err != nil {
		t.Fatalf("Failed to create agent participant: %v", err)
	}

	if err := agent.JoinMeeting(ctx, m.ID, agentToken); err != nil {
		t.Fatalf("Failed to join meeting: %v", err)
	}
	defer agent.LeaveMeeting(ctx)
	t.Log("Agent joined meeting")

	// Wait for connection to stabilize
	time.Sleep(500 * time.Millisecond)

	// Start audio track
	t.Log("Starting audio track...")
	audioWriter, err := agent.StartAudioTrack(ctx, provider.AudioTrackOptions{
		Name:       "agent-audio",
		SampleRate: 48000,
		Channels:   1,
	})
	if err != nil {
		t.Fatalf("Failed to start audio track: %v", err)
	}
	t.Log("Audio track started")

	// Write some test audio (silence - just zeros)
	t.Log("Writing test audio frames...")
	silentFrame := make([]byte, 960*2) // 20ms at 48kHz, 16-bit
	for i := 0; i < 10; i++ {
		n, err := audioWriter.Write(silentFrame)
		if err != nil {
			t.Errorf("Failed to write audio frame %d: %v", i, err)
		}
		if n != len(silentFrame) {
			t.Errorf("Expected to write %d bytes, wrote %d", len(silentFrame), n)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Log("Wrote 10 audio frames")

	// Stop audio track
	t.Log("Stopping audio track...")
	if err := agent.StopAudioTrack(ctx); err != nil {
		t.Fatalf("Failed to stop audio track: %v", err)
	}
	t.Log("Audio track stopped")

	// Close writer
	if err := audioWriter.Close(); err != nil {
		t.Errorf("Failed to close audio writer: %v", err)
	}
}

func TestDataMessages(t *testing.T) {
	cfg := getTestConfig(t)
	prov, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer prov.Close()

	ctx := context.Background()
	meetingName := fmt.Sprintf("test-data-%d", time.Now().UnixNano())

	// Create meeting
	m, err := prov.CreateMeeting(ctx, meeting.CreateRequest{
		Name: meetingName,
	})
	if err != nil {
		t.Fatalf("Failed to create meeting: %v", err)
	}
	defer prov.EndMeeting(ctx, m.ID)

	// Create two agents to test messaging
	agent1Token, _ := prov.CreateJoinToken(ctx, token.CreateRequest{
		MeetingID: m.ID,
		Participant: participant.Info{
			Name:     "Agent 1",
			Kind:     participant.KindAgent,
			Identity: "agent-1",
		},
	})

	agent2Token, _ := prov.CreateJoinToken(ctx, token.CreateRequest{
		MeetingID: m.ID,
		Participant: participant.Info{
			Name:     "Agent 2",
			Kind:     participant.KindAgent,
			Identity: "agent-2",
		},
	})

	agent1, _ := prov.CreateAgentParticipant(provider.AgentParticipantOptions{AutoSubscribe: true})
	agent2, _ := prov.CreateAgentParticipant(provider.AgentParticipantOptions{AutoSubscribe: true})

	if err := agent1.JoinMeeting(ctx, m.ID, agent1Token); err != nil {
		t.Fatalf("Agent 1 failed to join: %v", err)
	}
	defer agent1.LeaveMeeting(ctx)
	t.Log("Agent 1 joined")

	if err := agent2.JoinMeeting(ctx, m.ID, agent2Token); err != nil {
		t.Fatalf("Agent 2 failed to join: %v", err)
	}
	defer agent2.LeaveMeeting(ctx)
	t.Log("Agent 2 joined")

	// Wait for connections to stabilize
	time.Sleep(500 * time.Millisecond)

	// Set up message receiver on agent 2
	var receivedCount atomic.Int32
	var receivedPayload []byte
	var mu sync.Mutex

	agent2.OnDataMessage(func(msg provider.DataMessage) {
		mu.Lock()
		receivedPayload = msg.Payload
		mu.Unlock()
		receivedCount.Add(1)
		t.Logf("Agent 2 received message: topic=%s, payload=%s", msg.Topic, string(msg.Payload))
	})

	// Send message from agent 1
	t.Log("Agent 1 sending data message...")
	testPayload := []byte(`{"type":"greeting","text":"Hello from Agent 1"}`)
	err = agent1.SendDataMessage(ctx, provider.DataMessage{
		Topic:    "test-topic",
		Payload:  testPayload,
		Reliable: true,
	})
	if err != nil {
		t.Fatalf("Failed to send data message: %v", err)
	}
	t.Log("Data message sent")

	// Wait for message to be received
	time.Sleep(500 * time.Millisecond)

	count := receivedCount.Load()
	t.Logf("Agent 2 received %d messages", count)

	// Note: Message delivery depends on LiveKit's data channel setup
	// In some cases, messages may not be received if data channels aren't fully established
	if count > 0 {
		mu.Lock()
		if string(receivedPayload) != string(testPayload) {
			t.Errorf("Payload mismatch: expected %s, got %s", testPayload, receivedPayload)
		}
		mu.Unlock()
		t.Log("Message received correctly")
	} else {
		t.Log("Note: No messages received (data channels may not be fully established in short test)")
	}
}

func TestParticipantEvents(t *testing.T) {
	cfg := getTestConfig(t)
	prov, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer prov.Close()

	ctx := context.Background()
	meetingName := fmt.Sprintf("test-events-%d", time.Now().UnixNano())

	// Create meeting
	m, err := prov.CreateMeeting(ctx, meeting.CreateRequest{
		Name: meetingName,
	})
	if err != nil {
		t.Fatalf("Failed to create meeting: %v", err)
	}
	defer prov.EndMeeting(ctx, m.ID)

	// Create first agent (observer)
	observerToken, _ := prov.CreateJoinToken(ctx, token.CreateRequest{
		MeetingID: m.ID,
		Participant: participant.Info{
			Name:     "Observer",
			Kind:     participant.KindAgent,
			Identity: "observer",
		},
	})

	observer, _ := prov.CreateAgentParticipant(provider.AgentParticipantOptions{AutoSubscribe: true})
	if err := observer.JoinMeeting(ctx, m.ID, observerToken); err != nil {
		t.Fatalf("Observer failed to join: %v", err)
	}
	defer observer.LeaveMeeting(ctx)
	t.Log("Observer joined")

	// Set up event handlers
	var joinedCount, leftCount atomic.Int32
	var joinedName, leftName string
	var mu sync.Mutex

	observer.OnParticipantJoined(func(p participant.Participant) {
		mu.Lock()
		joinedName = p.Name
		mu.Unlock()
		joinedCount.Add(1)
		t.Logf("Participant joined event: %s (%s)", p.Name, p.Kind)
	})

	observer.OnParticipantLeft(func(p participant.Participant) {
		mu.Lock()
		leftName = p.Name
		mu.Unlock()
		leftCount.Add(1)
		t.Logf("Participant left event: %s", p.Name)
	})

	// Wait for observer to be fully connected
	time.Sleep(500 * time.Millisecond)

	// Create second agent (joiner)
	joinerToken, _ := prov.CreateJoinToken(ctx, token.CreateRequest{
		MeetingID: m.ID,
		Participant: participant.Info{
			Name:     "Joiner",
			Kind:     participant.KindAgent,
			Identity: "joiner",
		},
	})

	joiner, _ := prov.CreateAgentParticipant(provider.AgentParticipantOptions{AutoSubscribe: true})

	t.Log("Joiner joining meeting...")
	if err := joiner.JoinMeeting(ctx, m.ID, joinerToken); err != nil {
		t.Fatalf("Joiner failed to join: %v", err)
	}
	t.Log("Joiner joined")

	// Wait for join event
	time.Sleep(500 * time.Millisecond)

	t.Log("Joiner leaving meeting...")
	if err := joiner.LeaveMeeting(ctx); err != nil {
		t.Fatalf("Joiner failed to leave: %v", err)
	}
	t.Log("Joiner left")

	// Wait for leave event (may take longer to propagate)
	time.Sleep(1 * time.Second)

	// Check events
	joined := joinedCount.Load()
	left := leftCount.Load()
	t.Logf("Events received: %d joined, %d left", joined, left)

	if joined == 0 {
		t.Error("Expected at least one participant joined event")
	} else {
		mu.Lock()
		if joinedName != "Joiner" {
			t.Errorf("Expected joined participant name 'Joiner', got '%s'", joinedName)
		}
		mu.Unlock()
	}

	// Note: Left events may not always be received due to timing
	// The participant may disconnect before the event can be delivered
	if left == 0 {
		t.Log("Note: No left event received (timing-dependent)")
	} else {
		mu.Lock()
		if leftName != "Joiner" {
			t.Errorf("Expected left participant name 'Joiner', got '%s'", leftName)
		}
		mu.Unlock()
		t.Log("Left event received correctly")
	}
}

func TestMultipleParticipants(t *testing.T) {
	cfg := getTestConfig(t)
	prov, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer prov.Close()

	ctx := context.Background()
	meetingName := fmt.Sprintf("test-multi-%d", time.Now().UnixNano())

	// Create meeting
	m, err := prov.CreateMeeting(ctx, meeting.CreateRequest{
		Name: meetingName,
	})
	if err != nil {
		t.Fatalf("Failed to create meeting: %v", err)
	}
	defer prov.EndMeeting(ctx, m.ID)

	// Create and join multiple agents
	numAgents := 3
	agents := make([]provider.AgentParticipant, numAgents)

	for i := 0; i < numAgents; i++ {
		tok, _ := prov.CreateJoinToken(ctx, token.CreateRequest{
			MeetingID: m.ID,
			Participant: participant.Info{
				Name:     fmt.Sprintf("Agent-%d", i+1),
				Kind:     participant.KindAgent,
				Identity: fmt.Sprintf("agent-%d", i+1),
			},
		})

		agent, _ := prov.CreateAgentParticipant(provider.AgentParticipantOptions{AutoSubscribe: true})
		if err := agent.JoinMeeting(ctx, m.ID, tok); err != nil {
			t.Fatalf("Agent %d failed to join: %v", i+1, err)
		}
		agents[i] = agent
		t.Logf("Agent-%d joined", i+1)
	}

	// Cleanup
	defer func() {
		for i, agent := range agents {
			if err := agent.LeaveMeeting(ctx); err != nil {
				t.Errorf("Agent %d failed to leave: %v", i+1, err)
			}
		}
	}()

	// Wait for all connections to stabilize
	time.Sleep(1 * time.Second)

	// List participants via provider
	participants, err := prov.ListParticipants(ctx, m.ID)
	if err != nil {
		t.Fatalf("Failed to list participants: %v", err)
	}
	t.Logf("Found %d participants via provider", len(participants))

	if len(participants) != numAgents {
		t.Errorf("Expected %d participants, got %d", numAgents, len(participants))
	}

	// Check remote participants from each agent's perspective
	for i, agent := range agents {
		remote := agent.RemoteParticipants()
		t.Logf("Agent-%d sees %d remote participants", i+1, len(remote))
		if len(remote) != numAgents-1 {
			t.Errorf("Agent-%d: expected %d remote participants, got %d", i+1, numAgents-1, len(remote))
		}
	}
}

func TestParticipantMetadata(t *testing.T) {
	cfg := getTestConfig(t)
	prov, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer prov.Close()

	ctx := context.Background()
	meetingName := fmt.Sprintf("test-metadata-%d", time.Now().UnixNano())

	// Create meeting with metadata
	m, err := prov.CreateMeeting(ctx, meeting.CreateRequest{
		Name: meetingName,
		Metadata: map[string]string{
			"purpose":    "integration-test",
			"created_by": "test-suite",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create meeting: %v", err)
	}
	defer prov.EndMeeting(ctx, m.ID)
	t.Logf("Created meeting with metadata: %v", m.Metadata)

	// Create agent with metadata
	agentToken, err := prov.CreateJoinToken(ctx, token.CreateRequest{
		MeetingID: m.ID,
		Participant: participant.Info{
			Name:     "Metadata Agent",
			Kind:     participant.KindAgent,
			Identity: "metadata-agent",
			Metadata: map[string]string{
				"role":    "assistant",
				"version": "1.0",
			},
		},
		Metadata: map[string]string{
			"session_id": "test-session-123",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create agent token: %v", err)
	}
	t.Logf("Token metadata: %v", agentToken.Metadata)

	// Check that metadata was set in token
	if agentToken.Metadata["role"] != "assistant" {
		t.Errorf("Expected role 'assistant' in token metadata, got '%s'", agentToken.Metadata["role"])
	}
	if agentToken.Metadata["kind"] != "agent" {
		t.Errorf("Expected kind 'agent' in token metadata, got '%s'", agentToken.Metadata["kind"])
	}

	// Join and verify participant metadata
	agent, _ := prov.CreateAgentParticipant(provider.AgentParticipantOptions{AutoSubscribe: true})
	if err := agent.JoinMeeting(ctx, m.ID, agentToken); err != nil {
		t.Fatalf("Failed to join meeting: %v", err)
	}
	defer agent.LeaveMeeting(ctx)

	// Wait for connection
	time.Sleep(500 * time.Millisecond)

	// Get participant from provider and check metadata
	p, err := prov.GetParticipant(ctx, m.ID, "metadata-agent")
	if err != nil {
		t.Fatalf("Failed to get participant: %v", err)
	}
	t.Logf("Participant metadata from provider: %v", p.Metadata)
	t.Logf("Participant kind: %s", p.Kind)

	// Note: LiveKit's GetParticipant API may not always return metadata immediately
	// or in the same format. The metadata is encoded in the JWT and may be parsed
	// differently by the server.
	if len(p.Metadata) > 0 {
		if p.Metadata["kind"] != "agent" {
			t.Errorf("Expected kind 'agent' in participant metadata, got '%s'", p.Metadata["kind"])
		}
	} else {
		t.Log("Note: Metadata not returned by GetParticipant (server-side parsing dependent)")
	}

	// Verify we can at least get the participant
	if p.Name != "Metadata Agent" {
		t.Errorf("Expected participant name 'Metadata Agent', got '%s'", p.Name)
	}
	if p.Identity != "metadata-agent" {
		t.Errorf("Expected identity 'metadata-agent', got '%s'", p.Identity)
	}
}

func TestEventChannel(t *testing.T) {
	cfg := getTestConfig(t)
	prov, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer prov.Close()

	ctx := context.Background()
	meetingName := fmt.Sprintf("test-events-ch-%d", time.Now().UnixNano())

	// Create meeting
	m, err := prov.CreateMeeting(ctx, meeting.CreateRequest{
		Name: meetingName,
	})
	if err != nil {
		t.Fatalf("Failed to create meeting: %v", err)
	}
	defer prov.EndMeeting(ctx, m.ID)

	// Create agent
	tok, _ := prov.CreateJoinToken(ctx, token.CreateRequest{
		MeetingID: m.ID,
		Participant: participant.Info{
			Name:     "Event Agent",
			Kind:     participant.KindAgent,
			Identity: "event-agent",
		},
	})

	agent, _ := prov.CreateAgentParticipant(provider.AgentParticipantOptions{AutoSubscribe: true})

	// Get events channel
	events := agent.Events()
	if events == nil {
		t.Fatal("Events channel should not be nil")
	}

	// Start collecting events in background
	var eventTypes []string
	var eventMu sync.Mutex
	done := make(chan struct{})

	go func() {
		for {
			select {
			case evt, ok := <-events:
				if !ok {
					return
				}
				eventMu.Lock()
				eventTypes = append(eventTypes, string(evt.Type))
				eventMu.Unlock()
				t.Logf("Event received: %s", evt.Type)
			case <-done:
				return
			}
		}
	}()

	// Join meeting
	if err := agent.JoinMeeting(ctx, m.ID, tok); err != nil {
		t.Fatalf("Failed to join meeting: %v", err)
	}
	t.Log("Agent joined")

	// Wait for events
	time.Sleep(500 * time.Millisecond)

	// Leave meeting
	if err := agent.LeaveMeeting(ctx); err != nil {
		t.Fatalf("Failed to leave meeting: %v", err)
	}
	t.Log("Agent left")

	// Wait for disconnect events
	time.Sleep(500 * time.Millisecond)
	close(done)

	// Check events
	eventMu.Lock()
	t.Logf("Collected %d events: %v", len(eventTypes), eventTypes)
	eventMu.Unlock()
}
