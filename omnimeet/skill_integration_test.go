//go:build integration

package omnimeet_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/plexusone/omni-livekit/omnimeet"
	"github.com/plexusone/omnimeet-core/skill"
)

// TestSkillCreateMeeting tests the create_meeting tool with real LiveKit.
func TestSkillCreateMeeting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prov, err := omnimeet.NewProvider(omnimeet.Config{
		APIKey:    os.Getenv("LIVEKIT_API_KEY"),
		APISecret: os.Getenv("LIVEKIT_API_SECRET"),
		ServerURL: os.Getenv("LIVEKIT_URL"),
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	meetingSkill := skill.New(prov, skill.Config{
		DefaultAgentName:   "Test Agent",
		DefaultMeetingName: "Test Meeting",
	})

	if err := meetingSkill.Init(ctx); err != nil {
		t.Fatalf("failed to init skill: %v", err)
	}
	defer func() {
		if err := meetingSkill.Close(); err != nil {
			t.Errorf("failed to close skill: %v", err)
		}
	}()

	// Find create_meeting tool
	var createTool interface {
		Call(context.Context, map[string]any) (any, error)
	}
	for _, tool := range meetingSkill.Tools() {
		if tool.Name() == "create_meeting" {
			createTool = tool
			break
		}
	}
	if createTool == nil {
		t.Fatal("create_meeting tool not found")
	}

	// Create meeting
	result, err := createTool.Call(ctx, map[string]any{
		"name": "Skill Integration Test",
	})
	if err != nil {
		t.Fatalf("create_meeting failed: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}

	meetingID, ok := resultMap["meeting_id"].(string)
	if !ok || meetingID == "" {
		t.Fatal("expected meeting_id in result")
	}

	if resultMap["join_url"] == nil {
		t.Error("expected join_url in result")
	}

	// Cleanup
	if err := prov.DeleteMeeting(ctx, meetingID); err != nil {
		t.Errorf("failed to delete meeting: %v", err)
	}
}

// TestSkillMeetingLifecycle tests create, get, list, end flow.
func TestSkillMeetingLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prov, err := omnimeet.NewProvider(omnimeet.Config{
		APIKey:    os.Getenv("LIVEKIT_API_KEY"),
		APISecret: os.Getenv("LIVEKIT_API_SECRET"),
		ServerURL: os.Getenv("LIVEKIT_URL"),
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	meetingSkill := skill.New(prov, skill.Config{})
	if err := meetingSkill.Init(ctx); err != nil {
		t.Fatalf("failed to init skill: %v", err)
	}
	defer func() {
		if err := meetingSkill.Close(); err != nil {
			t.Errorf("failed to close skill: %v", err)
		}
	}()

	tools := make(map[string]interface {
		Call(context.Context, map[string]any) (any, error)
	})
	for _, tool := range meetingSkill.Tools() {
		tools[tool.Name()] = tool
	}

	// 1. Create meeting
	result, err := tools["create_meeting"].Call(ctx, map[string]any{
		"name": "Lifecycle Test",
	})
	if err != nil {
		t.Fatalf("create_meeting failed: %v", err)
	}

	meetingID := result.(map[string]any)["meeting_id"].(string)
	defer func() {
		if err := prov.DeleteMeeting(context.Background(), meetingID); err != nil {
			// Ignore error - meeting may already be ended
		}
	}()

	// 2. Get meeting
	result, err = tools["get_meeting"].Call(ctx, map[string]any{
		"meeting_id": meetingID,
	})
	if err != nil {
		t.Fatalf("get_meeting failed: %v", err)
	}

	// 3. Get join link
	result, err = tools["get_join_link"].Call(ctx, map[string]any{
		"meeting_id":       meetingID,
		"participant_name": "Test User",
	})
	if err != nil {
		t.Fatalf("get_join_link failed: %v", err)
	}

	linkResult := result.(map[string]any)
	if linkResult["join_url"] == nil {
		t.Error("expected join_url")
	}
	if linkResult["token"] == nil {
		t.Error("expected token")
	}

	// 4. End meeting
	result, err = tools["end_meeting"].Call(ctx, map[string]any{
		"meeting_id": meetingID,
	})
	if err != nil {
		t.Fatalf("end_meeting failed: %v", err)
	}

	endResult := result.(map[string]any)
	if endResult["success"] != true {
		t.Error("expected success: true")
	}
}

// TestSkillAgentJoin tests joining a meeting as an agent.
func TestSkillAgentJoin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prov, err := omnimeet.NewProvider(omnimeet.Config{
		APIKey:    os.Getenv("LIVEKIT_API_KEY"),
		APISecret: os.Getenv("LIVEKIT_API_SECRET"),
		ServerURL: os.Getenv("LIVEKIT_URL"),
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	meetingSkill := skill.New(prov, skill.Config{
		DefaultAgentName: "Test Agent",
	})
	if err := meetingSkill.Init(ctx); err != nil {
		t.Fatalf("failed to init skill: %v", err)
	}
	defer func() {
		if err := meetingSkill.Close(); err != nil {
			t.Errorf("failed to close skill: %v", err)
		}
	}()

	tools := make(map[string]interface {
		Call(context.Context, map[string]any) (any, error)
	})
	for _, tool := range meetingSkill.Tools() {
		tools[tool.Name()] = tool
	}

	// Create meeting
	result, err := tools["create_meeting"].Call(ctx, map[string]any{
		"name": "Agent Join Test",
	})
	if err != nil {
		t.Fatalf("create_meeting failed: %v", err)
	}

	meetingID := result.(map[string]any)["meeting_id"].(string)
	defer func() {
		if err := prov.DeleteMeeting(context.Background(), meetingID); err != nil {
			// Ignore
		}
	}()

	// Join as agent
	result, err = tools["join_meeting"].Call(ctx, map[string]any{
		"meeting_id": meetingID,
	})
	if err != nil {
		t.Fatalf("join_meeting failed: %v", err)
	}

	joinResult := result.(map[string]any)
	if joinResult["success"] != true {
		t.Error("expected success: true")
	}

	// Verify session exists
	session := meetingSkill.GetSession(meetingID)
	if session == nil {
		t.Fatal("expected session to exist")
	}
	if session.Agent == nil {
		t.Error("expected agent in session")
	}

	// Wait for connection
	time.Sleep(2 * time.Second)

	// List participants (should include agent)
	result, err = tools["list_participants"].Call(ctx, map[string]any{
		"meeting_id": meetingID,
	})
	if err != nil {
		t.Fatalf("list_participants failed: %v", err)
	}

	// Leave meeting
	result, err = tools["leave_meeting"].Call(ctx, map[string]any{
		"meeting_id": meetingID,
	})
	if err != nil {
		t.Fatalf("leave_meeting failed: %v", err)
	}

	leaveResult := result.(map[string]any)
	if leaveResult["success"] != true {
		t.Error("expected success: true")
	}

	// Verify session is gone
	session = meetingSkill.GetSession(meetingID)
	if session != nil {
		t.Error("expected session to be removed")
	}
}

// TestSkillEvents tests event emission.
func TestSkillEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prov, err := omnimeet.NewProvider(omnimeet.Config{
		APIKey:    os.Getenv("LIVEKIT_API_KEY"),
		APISecret: os.Getenv("LIVEKIT_API_SECRET"),
		ServerURL: os.Getenv("LIVEKIT_URL"),
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	meetingSkill := skill.New(prov, skill.Config{})
	if err := meetingSkill.Init(ctx); err != nil {
		t.Fatalf("failed to init skill: %v", err)
	}
	defer func() {
		if err := meetingSkill.Close(); err != nil {
			t.Errorf("failed to close skill: %v", err)
		}
	}()

	// Track events
	var events []skill.Event
	meetingSkill.OnEvent(func(e skill.Event) {
		events = append(events, e)
	})

	tools := make(map[string]interface {
		Call(context.Context, map[string]any) (any, error)
	})
	for _, tool := range meetingSkill.Tools() {
		tools[tool.Name()] = tool
	}

	// Create meeting - should emit meeting_created
	result, err := tools["create_meeting"].Call(ctx, map[string]any{
		"name": "Event Test",
	})
	if err != nil {
		t.Fatalf("create_meeting failed: %v", err)
	}

	meetingID := result.(map[string]any)["meeting_id"].(string)
	defer func() {
		if err := prov.DeleteMeeting(context.Background(), meetingID); err != nil {
			// Ignore
		}
	}()

	// Check for meeting_created event
	foundCreated := false
	for _, e := range events {
		if e.Type == "meeting_created" && e.MeetingID == meetingID {
			foundCreated = true
			break
		}
	}
	if !foundCreated {
		t.Error("expected meeting_created event")
	}

	// Join meeting - should emit agent_joined
	_, err = tools["join_meeting"].Call(ctx, map[string]any{
		"meeting_id": meetingID,
	})
	if err != nil {
		t.Fatalf("join_meeting failed: %v", err)
	}

	foundJoined := false
	for _, e := range events {
		if e.Type == "agent_joined" && e.MeetingID == meetingID {
			foundJoined = true
			break
		}
	}
	if !foundJoined {
		t.Error("expected agent_joined event")
	}

	// Leave meeting - should emit agent_left
	_, err = tools["leave_meeting"].Call(ctx, map[string]any{
		"meeting_id": meetingID,
	})
	if err != nil {
		t.Fatalf("leave_meeting failed: %v", err)
	}

	foundLeft := false
	for _, e := range events {
		if e.Type == "agent_left" && e.MeetingID == meetingID {
			foundLeft = true
			break
		}
	}
	if !foundLeft {
		t.Error("expected agent_left event")
	}
}
