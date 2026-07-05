# Agent API

Package `agent` provides a high-level API for creating LiveKit voice agents.

## Agent

```go
type Agent struct {
    // Internal fields
}

func New(opts Options) (*Agent, error)
```

### Options

```go
type Options struct {
    // APIKey is the LiveKit API key.
    APIKey string

    // APISecret is the LiveKit API secret.
    APISecret string

    // ServerURL is the LiveKit server URL.
    ServerURL string

    // Identity is the agent's identity in the room.
    Identity string

    // Name is the agent's display name.
    Name string

    // AutoSubscribe automatically subscribes to all tracks.
    AutoSubscribe bool

    // Audio configures audio settings.
    Audio AudioConfig
}

type AudioConfig struct {
    // SampleRate is the audio sample rate (default: 48000).
    SampleRate int

    // Channels is the number of audio channels (default: 1).
    Channels int

    // TrackName is the name of the audio track.
    TrackName string
}
```

### Room Methods

```go
// Join joins a LiveKit room.
func (a *Agent) Join(ctx context.Context, roomName string) error

// Leave leaves the current room.
func (a *Agent) Leave(ctx context.Context) error

// Room returns the current room name.
func (a *Agent) Room() string
```

### Audio Methods

```go
// SubscribeToAudio subscribes to a specific participant's audio.
func (a *Agent) SubscribeToAudio(ctx context.Context, participantID string) (<-chan AudioFrame, error)

// SubscribeToAllAudio subscribes to all participants' audio.
func (a *Agent) SubscribeToAllAudio(ctx context.Context) (<-chan AudioFrame, error)

// SendAudio sends audio to the room.
func (a *Agent) SendAudio(ctx context.Context, data []byte, sampleRate int) error

// StartAudioTrack starts publishing an audio track.
func (a *Agent) StartAudioTrack(ctx context.Context) error

// StopAudioTrack stops publishing the audio track.
func (a *Agent) StopAudioTrack(ctx context.Context) error
```

### Event Handlers

```go
// OnParticipantJoined sets the handler for participant join events.
func (a *Agent) OnParticipantJoined(handler func(participant.Participant))

// OnParticipantLeft sets the handler for participant leave events.
func (a *Agent) OnParticipantLeft(handler func(participant.Participant))

// OnAudioFrame sets the handler for incoming audio frames.
func (a *Agent) OnAudioFrame(handler func(AudioFrame))

// Events returns a channel of all events.
func (a *Agent) Events() <-chan Event
```

### Participant Methods

```go
// RemoteParticipants returns all remote participants.
func (a *Agent) RemoteParticipants() []participant.Participant

// GetParticipant returns a specific participant by ID.
func (a *Agent) GetParticipant(participantID string) *participant.Participant
```

## AudioFrame

```go
type AudioFrame struct {
    // ParticipantID is the source participant.
    ParticipantID string

    // ParticipantName is the source participant's name.
    ParticipantName string

    // Data is the PCM audio data.
    Data []byte

    // SampleRate is the audio sample rate.
    SampleRate int

    // Channels is the number of audio channels.
    Channels int

    // Timestamp is when the frame was received.
    Timestamp time.Time
}
```

## Event

```go
type Event struct {
    // Type is the event type.
    Type string

    // Data contains event-specific data.
    Data interface{}
}
```

## Example

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/plexusone/omni-livekit/agent"
    "github.com/plexusone/omnimeet-core/participant"
)

func main() {
    ctx := context.Background()

    ag, err := agent.New(agent.Options{
        APIKey:        os.Getenv("LIVEKIT_API_KEY"),
        APISecret:     os.Getenv("LIVEKIT_API_SECRET"),
        ServerURL:     os.Getenv("LIVEKIT_URL"),
        Identity:      "my-agent",
        Name:          "AI Assistant",
        AutoSubscribe: true,
        Audio: agent.AudioConfig{
            SampleRate: 48000,
            Channels:   1,
            TrackName:  "agent-audio",
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    // Handle participants
    ag.OnParticipantJoined(func(p participant.Participant) {
        log.Printf("Joined: %s", p.Name)
    })

    ag.OnParticipantLeft(func(p participant.Participant) {
        log.Printf("Left: %s", p.Name)
    })

    // Handle audio
    ag.OnAudioFrame(func(frame agent.AudioFrame) {
        log.Printf("Audio from %s: %d bytes", frame.ParticipantName, len(frame.Data))

        // Process audio...
        // Send to STT, then LLM, then TTS
    })

    // Join room
    if err := ag.Join(ctx, "my-room"); err != nil {
        log.Fatal(err)
    }
    defer ag.Leave(ctx)

    // Process events
    for evt := range ag.Events() {
        log.Printf("Event: %s", evt.Type)
    }
}
```

## Audio Processing

The agent handles audio encoding/decoding automatically:

- Incoming: Opus → PCM16
- Outgoing: PCM16 → Opus

Sample rates are automatically resampled as needed.

## See Also

- [Room Client](room.md) - Low-level room management
- [Gateway](gateway.md) - Voice gateway API
- [Voice Agent Guide](../guides/voice-agent.md) - Building voice agents
