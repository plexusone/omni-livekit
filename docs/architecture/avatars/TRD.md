# Lip-Sync Avatar Feature - Technical Requirements Document

**Feature**: Real-Time Lip-Sync Avatars for Voice Agents
**Status**: Implemented (v0.2.0)
**Author**: PlexusOne
**Created**: 2025-07-06
**Last Updated**: 2026-07-06

## Overview

This document specifies the technical architecture for implementing lip-sync avatar support in omni-livekit, based on analysis of LiveKit's Python and JavaScript SDK implementations.

## Architecture

### High-Level Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           LiveKit Room                                   │
│                                                                         │
│  ┌───────────────────┐                    ┌───────────────────────────┐ │
│  │    Go Agent       │                    │   Avatar Worker           │ │
│  │    (omni-livekit) │                    │   (Tavus/Anam/Simli)      │ │
│  │                   │                    │                           │ │
│  │  ┌─────────────┐  │   ByteStream       │  ┌─────────────────────┐  │ │
│  │  │ TTS Engine  │──┼───(lk.audio)──────►│  │ Lip-Sync Generator  │  │ │
│  │  └─────────────┘  │                    │  └─────────────────────┘  │ │
│  │                   │                    │            │              │ │
│  │  ┌─────────────┐  │   RPC              │            ▼              │ │
│  │  │ Playback    │◄─┼───(lk.playback)────│  ┌─────────────────────┐  │ │
│  │  │ Controller  │  │                    │  │ Video + Audio Track │  │ │
│  │  └─────────────┘  │                    │  └─────────────────────┘  │ │
│  │                   │                    │            │              │ │
│  │  identity:        │                    │  identity: agent-avatar   │ │
│  │  "meeting-pm"     │                    │  publishes on behalf of   │ │
│  │                   │                    │  "meeting-pm"             │ │
│  └───────────────────┘                    └───────────────────────────┘ │
│                                                        │                │
│                                                        ▼                │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │                      Human Participant                            │  │
│  │                      Sees: Avatar video + hears audio             │  │
│  └───────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
```

### Component Diagram

```
omni-livekit/
├── agent/                    # Existing agent code
│   ├── agent.go
│   ├── audio.go
│   └── ...
│
├── avatar/                   # ✅ IMPLEMENTED: Avatar core
│   ├── session.go           # AvatarSession interface
│   ├── datastream.go        # DataStreamAudioOutput
│   ├── queue.go             # QueueAudioOutput (testing)
│   ├── rpc.go               # RPC handlers
│   ├── metrics.go           # Avatar metrics
│   └── doc.go
│
├── avatar/tavus/            # ✅ IMPLEMENTED: Tavus provider (via tavus-go SDK)
│   ├── client.go            # Wraps tavus-go SDK
│   ├── session.go           # TavusAvatarSession
│   └── doc.go
│
├── avatar/anam/             # FUTURE: Anam provider
│   ├── client.go
│   ├── session.go
│   └── doc.go
│
├── avatar/simli/            # FUTURE: Simli provider
│   ├── client.go
│   ├── session.go
│   └── doc.go
│
└── avatar/did/              # FUTURE: D-ID provider
    ├── client.go
    ├── session.go
    └── doc.go
```

## Core Interfaces

### AvatarSession Interface

```go
// avatar/session.go
package avatar

import (
    "context"
    "time"

    lksdk "github.com/livekit/server-sdk-go/v2"
)

// Session manages a lip-sync avatar that publishes video to the room.
type Session interface {
    // AvatarIdentity returns the participant identity of the avatar worker.
    // This is the identity that will appear in the room and publish video.
    AvatarIdentity() string

    // Provider returns the provider name (e.g., "tavus", "anam", "simli").
    Provider() string

    // Start initializes the avatar session.
    // It should:
    // 1. Create a session with the avatar provider
    // 2. Generate a LiveKit token for the avatar to join
    // 3. Configure the agent's audio output to stream to the avatar
    Start(ctx context.Context, opts StartOptions) error

    // WaitForJoin blocks until the avatar participant joins and publishes video.
    // Returns an error if the timeout is exceeded.
    WaitForJoin(ctx context.Context, timeout time.Duration) error

    // Close disconnects the avatar and cleans up resources.
    // It should remove the avatar participant from the room.
    Close(ctx context.Context) error
}

// StartOptions configures avatar session startup.
type StartOptions struct {
    // Room is the LiveKit room the agent has joined.
    Room *lksdk.Room

    // AgentIdentity is the identity of the agent participant.
    // The avatar will publish on behalf of this identity.
    AgentIdentity string

    // LiveKitURL is the LiveKit server URL for the avatar to connect to.
    LiveKitURL string

    // LiveKitAPIKey is used to generate tokens for the avatar.
    LiveKitAPIKey string

    // LiveKitAPISecret is used to generate tokens for the avatar.
    LiveKitAPISecret string
}

// SessionCallbacks defines event callbacks for avatar sessions.
type SessionCallbacks struct {
    // OnMetricsCollected is called when avatar metrics are available.
    OnMetricsCollected func(metrics Metrics)

    // OnPlaybackStarted is called when the avatar starts speaking.
    OnPlaybackStarted func()

    // OnPlaybackFinished is called when the avatar finishes speaking.
    OnPlaybackFinished func(position float64, interrupted bool)

    // OnError is called when an error occurs.
    OnError func(err error)
}

// Metrics contains avatar performance metrics.
type Metrics struct {
    // AvatarJoinLatency is the time from Start() to avatar joining the room.
    AvatarJoinLatency time.Duration

    // PlaybackLatency is the time from audio send to avatar speech start.
    PlaybackLatency time.Duration

    // Provider is the avatar provider name.
    Provider string
}
```

### AudioDestination Interface

```go
// avatar/datastream.go
package avatar

import (
    "context"
)

// AudioDestination receives audio frames and forwards them to an avatar.
// Implementations include DataStreamAudioOutput (remote) and QueueAudioOutput (local).
type AudioDestination interface {
    // CaptureFrame sends a PCM16 audio frame to the avatar.
    // The frame should be little-endian PCM16 at the configured sample rate.
    CaptureFrame(ctx context.Context, frame []byte) error

    // Flush marks the end of an audio segment.
    // The avatar should speak all buffered audio before accepting more.
    Flush(ctx context.Context) error

    // ClearBuffer interrupts current playback.
    // Use this when the user interrupts the agent.
    ClearBuffer(ctx context.Context) error

    // SampleRate returns the expected input sample rate.
    SampleRate() int

    // Close releases resources.
    Close() error
}

// PlaybackCallback is called when playback events occur.
type PlaybackCallback func(event PlaybackEvent)

// PlaybackEvent represents a playback state change.
type PlaybackEvent struct {
    Type        PlaybackEventType
    Position    float64 // Playback position in seconds
    Interrupted bool    // True if playback was interrupted
}

// PlaybackEventType identifies the type of playback event.
type PlaybackEventType string

const (
    PlaybackStarted  PlaybackEventType = "started"
    PlaybackFinished PlaybackEventType = "finished"
)
```

### DataStreamAudioOutput

```go
// avatar/datastream.go

// DataStreamAudioOutput streams audio to a remote avatar via LiveKit ByteStream.
type DataStreamAudioOutput struct {
    room                *lksdk.Room
    destinationIdentity string
    sampleRate          int

    // Internal state
    streamWriter *lksdk.ByteStreamWriter
    callbacks    []PlaybackCallback
    mu           sync.Mutex
}

// DataStreamConfig configures the DataStreamAudioOutput.
type DataStreamConfig struct {
    // Room is the LiveKit room.
    Room *lksdk.Room

    // DestinationIdentity is the avatar participant identity.
    DestinationIdentity string

    // SampleRate is the audio sample rate (default: 24000).
    SampleRate int

    // WaitRemoteTrack specifies which track to wait for before sending audio.
    // Use "video" to wait for the avatar to publish video.
    WaitRemoteTrack string

    // WaitPlaybackStart waits for RPC notification before marking playout started.
    WaitPlaybackStart bool
}

// NewDataStreamAudioOutput creates a new DataStreamAudioOutput.
func NewDataStreamAudioOutput(cfg DataStreamConfig) (*DataStreamAudioOutput, error)

// RPC method names for playback control.
const (
    RPCPlaybackStarted  = "lk.playback_started"
    RPCPlaybackFinished = "lk.playback_finished"
    RPCClearBuffer      = "lk.clear_buffer"
)

// AudioStreamTopic is the ByteStream topic for audio data.
const AudioStreamTopic = "lk.audio_stream"
```

## LiveKit SDK Requirements

### ByteStream Support

The avatar system requires ByteStream support for streaming audio:

```go
// Required from LiveKit Go SDK (may need implementation or contribution)

// ByteStreamWriter writes data to a remote participant.
type ByteStreamWriter interface {
    // Write sends data to the stream.
    Write(data []byte) (int, error)

    // SetAttributes sets metadata on the stream.
    SetAttributes(attrs map[string]string) error

    // Close closes the stream, signaling end of data.
    Close() error
}

// LocalParticipant.StreamBytes creates a ByteStream to a remote participant.
func (p *LocalParticipant) StreamBytes(
    ctx context.Context,
    topic string,
    destinationIdentity string,
) (ByteStreamWriter, error)

// Room.OnByteStreamReceived registers a handler for incoming ByteStreams.
func (r *Room) OnByteStreamReceived(handler func(reader ByteStreamReader, topic string, from string))
```

**Status**: Check if LiveKit Go SDK supports this. If not, options:

1. Contribute upstream to livekit/server-sdk-go
2. Implement using DataChannel primitives
3. Use WebSocket fallback

### RPC Support

The avatar system requires RPC for playback control:

```go
// Required from LiveKit Go SDK (may need implementation or contribution)

// LocalParticipant.RegisterRPCMethod registers an RPC method handler.
func (p *LocalParticipant) RegisterRPCMethod(
    method string,
    handler func(requestID string, callerIdentity string, payload string) (string, error),
) error

// LocalParticipant.PerformRPC calls an RPC method on a remote participant.
func (p *LocalParticipant) PerformRPC(
    ctx context.Context,
    destinationIdentity string,
    method string,
    payload string,
) (string, error)
```

**Status**: Check LiveKit Go SDK for RPC support.

## Provider Implementation Pattern

Each avatar provider follows this pattern:

```go
// avatar/tavus/session.go
package tavus

import (
    "context"
    "time"

    "github.com/plexusone/omni-livekit/avatar"
    lksdk "github.com/livekit/server-sdk-go/v2"
)

// Config configures the Tavus avatar session.
type Config struct {
    // APIKey is the Tavus API key.
    APIKey string

    // FaceID is the Tavus face/persona ID.
    FaceID string

    // ReplicaID is the Tavus replica ID (optional).
    ReplicaID string

    // AvatarParticipantName is the display name for the avatar.
    AvatarParticipantName string
}

// AvatarSession implements avatar.Session for Tavus.
type AvatarSession struct {
    config         Config
    avatarIdentity string
    conversationID string
    audioOutput    *avatar.DataStreamAudioOutput
    client         *Client
}

// New creates a new Tavus avatar session.
func New(cfg Config) (*AvatarSession, error)

// AvatarIdentity returns the avatar participant identity.
func (s *AvatarSession) AvatarIdentity() string

// Provider returns "tavus".
func (s *AvatarSession) Provider() string

// Start initializes the Tavus session.
func (s *AvatarSession) Start(ctx context.Context, opts avatar.StartOptions) error {
    // 1. Generate JWT token with lk.publish_on_behalf attribute
    token := generateToken(opts, s.avatarIdentity)

    // 2. Create Tavus conversation via API
    s.conversationID, err = s.client.CreateConversation(ctx, CreateConversationRequest{
        FaceID:        s.config.FaceID,
        LiveKitURL:    opts.LiveKitURL,
        LiveKitToken:  token,
    })

    // 3. Configure audio output to stream to avatar
    s.audioOutput = avatar.NewDataStreamAudioOutput(avatar.DataStreamConfig{
        Room:                opts.Room,
        DestinationIdentity: s.avatarIdentity,
        SampleRate:          24000, // Tavus uses 24kHz
        WaitRemoteTrack:     "video",
    })

    return nil
}

// WaitForJoin waits for the avatar to join and publish video.
func (s *AvatarSession) WaitForJoin(ctx context.Context, timeout time.Duration) error

// Close ends the Tavus conversation and removes the avatar.
func (s *AvatarSession) Close(ctx context.Context) error
```

## Token Generation

Avatars join rooms with special attributes:

```go
// avatar/token.go
package avatar

import (
    "time"

    "github.com/livekit/protocol/auth"
)

// GenerateAvatarToken creates a JWT token for an avatar to join a room.
func GenerateAvatarToken(opts TokenOptions) (string, error) {
    at := auth.NewAccessToken(opts.APIKey, opts.APISecret)

    grant := &auth.VideoGrant{
        RoomJoin: true,
        Room:     opts.RoomName,
    }

    at.AddGrant(grant).
        SetIdentity(opts.AvatarIdentity).
        SetName(opts.AvatarName).
        SetValidFor(opts.TTL).
        SetMetadata(`{"kind":"agent"}`).
        SetAttributes(map[string]string{
            // Critical: allows avatar to publish tracks that appear
            // as if they're from the agent
            "lk.publish_on_behalf": opts.PublishOnBehalf,
        })

    return at.ToJWT()
}

// TokenOptions configures avatar token generation.
type TokenOptions struct {
    APIKey          string
    APISecret       string
    RoomName        string
    AvatarIdentity  string
    AvatarName      string
    PublishOnBehalf string // Agent identity
    TTL             time.Duration
}
```

## Integration with Existing Agent

```go
// Example usage in voice agent
package main

import (
    "context"

    "github.com/plexusone/omni-livekit/agent"
    "github.com/plexusone/omni-livekit/avatar"
    "github.com/plexusone/omni-livekit/avatar/tavus"
)

func main() {
    // Create agent (existing code)
    ag, _ := agent.New(agent.Options{
        APIKey:    os.Getenv("LIVEKIT_API_KEY"),
        APISecret: os.Getenv("LIVEKIT_API_SECRET"),
        ServerURL: os.Getenv("LIVEKIT_URL"),
        Identity:  "meeting-pm",
        Name:      "Meeting PM",
        MediaMode: agent.AudioOnly, // Avatar handles video
    })

    // Join room
    ag.Join(ctx, "my-room")

    // Create and start avatar
    avatarSession, _ := tavus.New(tavus.Config{
        APIKey: os.Getenv("TAVUS_API_KEY"),
        FaceID: "my-face-id",
    })

    avatarSession.Start(ctx, avatar.StartOptions{
        Room:             ag.Room(),
        AgentIdentity:    ag.LocalParticipant().Identity,
        LiveKitURL:       os.Getenv("LIVEKIT_URL"),
        LiveKitAPIKey:    os.Getenv("LIVEKIT_API_KEY"),
        LiveKitAPISecret: os.Getenv("LIVEKIT_API_SECRET"),
    })

    // Wait for avatar to be ready
    avatarSession.WaitForJoin(ctx, 10*time.Second)

    // Stream TTS audio to avatar instead of publishing directly
    audioOut := avatarSession.AudioOutput()

    // In your TTS loop:
    for frame := range ttsFrames {
        audioOut.CaptureFrame(ctx, frame)
    }
    audioOut.Flush(ctx)

    // Handle interruption
    audioOut.ClearBuffer(ctx)
}
```

## Error Handling

```go
// avatar/errors.go
package avatar

import "errors"

var (
    // ErrProviderUnavailable indicates the avatar provider is unreachable.
    ErrProviderUnavailable = errors.New("avatar provider unavailable")

    // ErrSessionNotStarted indicates Start() was not called.
    ErrSessionNotStarted = errors.New("avatar session not started")

    // ErrAvatarJoinTimeout indicates the avatar didn't join in time.
    ErrAvatarJoinTimeout = errors.New("avatar join timeout")

    // ErrRPCTimeout indicates an RPC call timed out.
    ErrRPCTimeout = errors.New("RPC timeout")

    // ErrInvalidConfig indicates invalid configuration.
    ErrInvalidConfig = errors.New("invalid avatar configuration")
)
```

## Testing Strategy

### Unit Tests

```go
// avatar/datastream_test.go
func TestDataStreamAudioOutput_CaptureFrame(t *testing.T)
func TestDataStreamAudioOutput_Flush(t *testing.T)
func TestDataStreamAudioOutput_ClearBuffer(t *testing.T)

// avatar/tavus/session_test.go
func TestAvatarSession_Start(t *testing.T)
func TestAvatarSession_WaitForJoin(t *testing.T)
func TestAvatarSession_Close(t *testing.T)
```

### Integration Tests

```go
// avatar/integration_test.go
func TestAvatarSession_EndToEnd(t *testing.T) {
    // Requires: LIVEKIT_URL, TAVUS_API_KEY
    // 1. Create agent and join room
    // 2. Start avatar session
    // 3. Stream test audio
    // 4. Verify avatar joins and publishes video
    // 5. Verify playback callbacks
}
```

### Mock Provider

```go
// avatar/mock/session.go
package mock

// MockAvatarSession implements avatar.Session for testing.
type MockAvatarSession struct {
    // Configuration
    JoinDelay time.Duration
    FailStart bool
    FailJoin  bool

    // State tracking
    Started bool
    Closed  bool
    AudioFrames [][]byte
}
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `AVATAR_PROVIDER` | Avatar provider (tavus, anam, simli) | none (disabled) |
| `AVATAR_API_KEY` | Provider API key | required if provider set |
| `AVATAR_FACE_ID` | Face/persona ID | provider default |
| `AVATAR_SAMPLE_RATE` | Audio sample rate | 24000 |
| `AVATAR_JOIN_TIMEOUT` | Max wait for avatar join | 10s |

### Provider-Specific Variables

**Tavus**:

| Variable | Description |
|----------|-------------|
| `TAVUS_API_KEY` | Tavus API key |
| `TAVUS_FACE_ID` | Tavus face ID |
| `TAVUS_REPLICA_ID` | Tavus replica ID (optional) |

**Anam**:

| Variable | Description |
|----------|-------------|
| `ANAM_API_KEY` | Anam API key |
| `ANAM_PERSONA_ID` | Anam persona ID |

**Simli**:

| Variable | Description |
|----------|-------------|
| `SIMLI_API_KEY` | Simli API key |
| `SIMLI_FACE_ID` | Simli face ID |

## Dependencies

### Go Dependencies

```go
// go.mod additions
require (
    github.com/livekit/server-sdk-go/v2 v2.x.x  // Existing
    github.com/livekit/protocol v1.x.x          // Existing
)
```

### External APIs

| Provider | API Base URL | Auth |
|----------|--------------|------|
| Tavus | `https://api.tavus.io` | Bearer token |
| Anam | `https://api.anam.ai` | API key header |
| Simli | `https://api.simli.ai` | API key header |

## Technical Questions (Resolved)

1. **ByteStream in Go SDK**: ✅ Resolved - Used `DataStreamAudioOutput` abstraction that can use LiveKit's data stream APIs when available, with fallback options.

2. **RPC in Go SDK**: ✅ Resolved - Playback callbacks handled via session events rather than RPC for initial implementation.

3. **Audio chain replacement**: ✅ Resolved - Created `AudioDestination` interface with multiple implementations:
   - `DataStreamAudioOutput` for remote avatar streaming
   - `QueueAudioOutput` for local testing

4. **Metrics collection**: ✅ Resolved - Event callbacks via `SessionCallbacks` for initial implementation. Prometheus/OTel can be added later.

5. **Tavus API Integration**: ✅ Resolved - Used [tavus-go](https://github.com/plexusone/tavus-go) SDK v0.2.0 for type-safe API access instead of raw HTTP client.

## Related Documents

- [PRD.md](PRD.md) - Product Requirements Document
- [PLAN.md](PLAN.md) - Implementation Plan
- [ROADMAP.md](ROADMAP.md) - Feature Roadmap
- [Static Image Avatar](../../architecture/avatar.md) - Existing static avatar docs
