# Gateway API

Package `omnivoice/gateway` provides the LiveKit implementation of the OmniVoice WebRTC gateway interface.

## Gateway

```go
type Gateway struct {
    // Internal fields
}

func New(cfg Config) (*Gateway, error)
```

### Config

```go
type Config struct {
    // LiveKitURL is the WebSocket URL for LiveKit server.
    LiveKitURL string

    // LiveKitAPIKey is the API key for authentication.
    LiveKitAPIKey string

    // LiveKitSecret is the API secret for authentication.
    LiveKitSecret string

    // RoomName is the default room to join.
    RoomName string

    // AgentIdentity is the identity for the agent participant.
    AgentIdentity string

    // AgentName is the display name for the agent.
    AgentName string

    // SampleRate is the audio sample rate (default: 24000).
    SampleRate int

    // Channels is the number of audio channels (default: 1).
    Channels int
}
```

### Methods

```go
// Name returns the gateway name ("livekit").
func (g *Gateway) Name() coregateway.ProviderName

// Start starts the gateway and connects to LiveKit.
func (g *Gateway) Start(ctx context.Context) error

// Stop disconnects from LiveKit.
func (g *Gateway) Stop() error

// OnParticipantJoined sets the handler for participant join events.
func (g *Gateway) OnParticipantJoined(handler coregateway.ParticipantHandler)

// JoinRoom joins a specific room.
func (g *Gateway) JoinRoom(ctx context.Context, roomName string) error

// LeaveRoom leaves the current room.
func (g *Gateway) LeaveRoom() error

// CurrentRoom returns the current room name.
func (g *Gateway) CurrentRoom() string

// GetSession returns a session by participant ID.
func (g *Gateway) GetSession(participantID string) (coregateway.WebRTCSession, bool)

// ListSessions returns all active sessions.
func (g *Gateway) ListSessions() []coregateway.WebRTCSession

// GenerateClientToken generates a token for a client to join.
func (g *Gateway) GenerateClientToken(roomName, identity, displayName string) (string, error)
```

## Session

Implements `coregateway.WebRTCSession`:

```go
type Session interface {
    // ID returns the session ID.
    ID() string

    // RoomName returns the room name.
    RoomName() string

    // Participant returns participant info.
    Participant() *coregateway.ParticipantInfo

    // SendAudio sends audio samples to the participant.
    SendAudio(samples []int16) error

    // ReceiveAudio returns a channel of received audio.
    ReceiveAudio() <-chan []int16

    // Events returns the event channel.
    Events() <-chan coregateway.Event

    // Close closes the session.
    Close() error
}
```

## Example

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/plexusone/omni-livekit/omnivoice/gateway"
    coregateway "github.com/plexusone/omnivoice-core/gateway"
)

func main() {
    gw, err := gateway.New(gateway.Config{
        LiveKitURL:    os.Getenv("LIVEKIT_URL"),
        LiveKitAPIKey: os.Getenv("LIVEKIT_API_KEY"),
        LiveKitSecret: os.Getenv("LIVEKIT_API_SECRET"),
        RoomName:      "my-room",
        AgentIdentity: "ai-agent",
        SampleRate:    24000,
    })
    if err != nil {
        log.Fatal(err)
    }

    gw.OnParticipantJoined(func(p *coregateway.ParticipantInfo) error {
        log.Printf("Participant joined: %s", p.DisplayName)

        // Get session
        session, ok := gw.GetSession(p.ID)
        if !ok {
            return nil
        }

        // Handle audio
        go func() {
            for samples := range session.ReceiveAudio() {
                // Process audio...
            }
        }()

        return nil
    })

    ctx := context.Background()
    if err := gw.Start(ctx); err != nil {
        log.Fatal(err)
    }
}
```

## See Also

- [Room Client](room.md) - Room management
- [Agent](agent.md) - High-level agent API
