# Room Client API

Package `room` provides a client for LiveKit room management.

## Client

```go
type Client struct {
    // Internal fields
}

func NewClient(cfg Config) (*Client, error)
```

### Config

```go
type Config struct {
    // APIKey is the LiveKit API key.
    APIKey string

    // APISecret is the LiveKit API secret.
    APISecret string

    // URL is the LiveKit server URL (e.g., "wss://your-project.livekit.cloud").
    URL string
}
```

### Room Management

```go
// CreateRoom creates a new room.
func (c *Client) CreateRoom(ctx context.Context, name string) (*livekit.Room, error)

// DeleteRoom deletes a room.
func (c *Client) DeleteRoom(ctx context.Context, name string) error

// ListRooms lists all rooms.
func (c *Client) ListRooms(ctx context.Context) ([]*livekit.Room, error)

// GetRoom gets room info.
func (c *Client) GetRoom(ctx context.Context, name string) (*livekit.Room, error)
```

### Token Generation

```go
// GenerateClientToken generates a token for a participant to join.
func (c *Client) GenerateClientToken(roomName, identity, displayName string) (string, error)

// GenerateAgentToken generates a token for an agent with additional permissions.
func (c *Client) GenerateAgentToken(roomName, identity, displayName string) (string, error)
```

### Participant Management

```go
// ListParticipants lists participants in a room.
func (c *Client) ListParticipants(ctx context.Context, roomName string) ([]*livekit.ParticipantInfo, error)

// GetParticipant gets a specific participant.
func (c *Client) GetParticipant(ctx context.Context, roomName, identity string) (*livekit.ParticipantInfo, error)

// RemoveParticipant removes a participant from a room.
func (c *Client) RemoveParticipant(ctx context.Context, roomName, identity string) error
```

### Recording

Package `room` also provides `RecordingClient` for recording rooms via LiveKit Egress.

```go
func NewRecordingClient(serverURL, apiKey, apiSecret string) *RecordingClient

// StartRecording starts recording a room.
func (c *RecordingClient) StartRecording(ctx context.Context, cfg RecordingConfig) (*RecordingInfo, error)

// StopRecording stops an active recording.
func (c *RecordingClient) StopRecording(ctx context.Context, egressID string) error

// ListRecordings lists active recordings for a room.
func (c *RecordingClient) ListRecordings(ctx context.Context, roomName string) ([]*RecordingInfo, error)
```

`RecordingConfig` selects the output format and destination:

```go
type RecordingConfig struct {
    RoomName  string          // Room to record
    Layout    RecordingLayout // LayoutGrid, LayoutSpeaker (default), LayoutSingleSpeaker
    Format    RecordingFormat // FormatMP4 (default) or FormatOGG
    FilePath  string          // Local file output (use this or S3)
    S3        *S3Config       // S3 upload (use this or FilePath)
    Width     int             // Video width (default: 1920)
    Height    int             // Video height (default: 1080)
    AudioOnly bool            // Record audio only, no video
}
```

Example:

```go
recorder := room.NewRecordingClient(
    os.Getenv("LIVEKIT_URL"),
    os.Getenv("LIVEKIT_API_KEY"),
    os.Getenv("LIVEKIT_API_SECRET"),
)

info, err := recorder.StartRecording(ctx, room.RecordingConfig{
    RoomName: "my-meeting",
    Layout:   room.LayoutGrid,
    Format:   room.FormatMP4,
    FilePath: "/recordings/my-meeting.mp4",
})
if err != nil {
    log.Fatal(err)
}

// ... later
err = recorder.StopRecording(ctx, info.EgressID)
```

To upload directly to S3 instead of a local path, set `S3` instead of `FilePath`:

```go
info, err := recorder.StartRecording(ctx, room.RecordingConfig{
    RoomName: "my-meeting",
    S3: &room.S3Config{
        AccessKey: os.Getenv("AWS_ACCESS_KEY_ID"),
        Secret:    os.Getenv("AWS_SECRET_ACCESS_KEY"),
        Region:    "us-east-1",
        Bucket:    "my-recordings-bucket",
    },
})
```

## Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/plexusone/omni-livekit/room"
)

func main() {
    client, err := room.NewClient(room.Config{
        APIKey:    os.Getenv("LIVEKIT_API_KEY"),
        APISecret: os.Getenv("LIVEKIT_API_SECRET"),
        URL:       os.Getenv("LIVEKIT_URL"),
    })
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()

    // Create a room
    r, err := client.CreateRoom(ctx, "my-meeting")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Created room: %s\n", r.Name)

    // Generate token for human participant
    humanToken, err := client.GenerateClientToken("my-meeting", "user-123", "Alice")
    if err != nil {
        log.Fatal(err)
    }

    // Build join URL
    joinURL := fmt.Sprintf("https://meet.livekit.io/custom?liveKitUrl=%s&token=%s",
        os.Getenv("LIVEKIT_URL"), humanToken)
    fmt.Printf("Join URL: %s\n", joinURL)

    // Generate token for agent
    agentToken, err := client.GenerateAgentToken("my-meeting", "ai-agent", "AI Assistant")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Agent token: %s...\n", agentToken[:50])

    // List participants
    participants, _ := client.ListParticipants(ctx, "my-meeting")
    for _, p := range participants {
        fmt.Printf("Participant: %s (%s)\n", p.Name, p.Identity)
    }

    // Clean up
    client.DeleteRoom(ctx, "my-meeting")
}
```

## Token Permissions

Tokens generated by the client include appropriate permissions:

### Client Token (Human)

- Can subscribe to tracks
- Can publish audio/video
- Cannot manage room

### Agent Token

- Can subscribe to tracks
- Can publish audio/video
- Can get participant info
- Extended TTL

## See Also

- [Agent](agent.md) - High-level agent API
- [Gateway](gateway.md) - Voice gateway API
