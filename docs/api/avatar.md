# Avatar API Reference

The `avatar` package provides infrastructure for integrating lip-sync avatars with voice agents.

## Package Structure

```
avatar/
├── session.go      # Session interface
├── audio.go        # AudioDestination interface
├── datastream.go   # DataStreamAudioOutput
├── queue.go        # QueueAudioOutput (testing)
├── token.go        # Token generation
├── errors.go       # Error types
└── tavus/          # Tavus provider
    ├── client.go   # API client
    └── session.go  # Session implementation
```

## Core Interfaces

### Session

```go
// Session manages a lip-sync avatar lifecycle.
type Session interface {
    // Start initializes the avatar session.
    Start(ctx context.Context, opts StartOptions) error

    // Stop ends the avatar session.
    Stop() error

    // AudioDestination returns the audio output for streaming to the avatar.
    AudioDestination() AudioDestination
}
```

### AudioDestination

```go
// AudioDestination receives audio frames for avatar lip-sync.
type AudioDestination interface {
    // Write sends PCM16 audio samples to the avatar.
    Write(samples []int16) error

    // Flush signals end of current speech segment.
    Flush() error

    // ClearBuffer interrupts current playback (for user interruption).
    ClearBuffer() error

    // Close releases resources.
    Close() error
}
```

## Token Generation

### GenerateAvatarToken

Generates a LiveKit JWT token for avatar participants.

```go
func GenerateAvatarToken(cfg TokenConfig) (string, error)
```

**TokenConfig:**

| Field | Type | Description |
|-------|------|-------------|
| `APIKey` | `string` | LiveKit API key |
| `APISecret` | `string` | LiveKit API secret |
| `RoomName` | `string` | Room to join |
| `AvatarID` | `string` | Avatar participant identity |
| `AvatarName` | `string` | Avatar display name |
| `OnBehalfOf` | `string` | Agent identity (sets `lk.publish_on_behalf`) |
| `TTL` | `time.Duration` | Token validity duration |

**Example:**

```go
token, err := avatar.GenerateAvatarToken(avatar.TokenConfig{
    APIKey:     os.Getenv("LIVEKIT_API_KEY"),
    APISecret:  os.Getenv("LIVEKIT_API_SECRET"),
    RoomName:   "my-room",
    AvatarID:   "tavus-avatar",
    AvatarName: "AI Assistant",
    OnBehalfOf: "ai-agent",
    TTL:        time.Hour,
})
```

## Audio Outputs

### DataStreamAudioOutput

Streams audio to a remote avatar via LiveKit data streams.

```go
func NewDataStreamAudioOutput(cfg DataStreamConfig) *DataStreamAudioOutput
```

**DataStreamConfig:**

| Field | Type | Description |
|-------|------|-------------|
| `Room` | `*lksdk.Room` | LiveKit room |
| `DestinationIdentity` | `string` | Avatar participant identity |
| `SampleRate` | `int` | Audio sample rate (default: 24000) |

**Example:**

```go
output := avatar.NewDataStreamAudioOutput(avatar.DataStreamConfig{
    Room:                room,
    DestinationIdentity: "tavus-avatar",
    SampleRate:          24000,
})

// Stream audio
output.Write(samples)
output.Flush()

// Handle interruption
output.ClearBuffer()
```

### QueueAudioOutput

In-memory audio queue for testing without network.

```go
func NewQueueAudioOutput() *QueueAudioOutput
```

**Methods:**

| Method | Description |
|--------|-------------|
| `Write(samples)` | Enqueue audio samples |
| `Flush()` | Mark end of segment |
| `ClearBuffer()` | Clear queued audio |
| `Read() []int16` | Dequeue audio (for testing) |
| `Len() int` | Number of queued samples |

**Example:**

```go
output := avatar.NewQueueAudioOutput()

// Simulate TTS
output.Write([]int16{1, 2, 3})
output.Flush()

// Verify in tests
assert.Equal(t, 3, output.Len())
```

## Error Types

### Sentinel Errors

| Error | Description |
|-------|-------------|
| `ErrInvalidConfig` | Missing required configuration |
| `ErrSessionNotStarted` | Operation requires started session |
| `ErrAvatarJoinTimeout` | Avatar didn't join in time |

### ProviderError

Wraps errors from avatar providers (Tavus, etc.).

```go
type ProviderError struct {
    Provider  string  // e.g., "tavus"
    Operation string  // e.g., "create_conversation"
    Err       error   // Underlying error
}
```

**Example:**

```go
var providerErr *avatar.ProviderError
if errors.As(err, &providerErr) {
    log.Printf("Provider %s failed on %s: %v",
        providerErr.Provider,
        providerErr.Operation,
        providerErr.Unwrap())
}
```

## Tavus Provider

### tavus.Client

API client for Tavus CVI (Conversational Video Interface).

```go
func NewClient(cfg ClientConfig) (*Client, error)
```

**ClientConfig:**

| Field | Type | Description |
|-------|------|-------------|
| `APIKey` | `string` | Tavus API key (required) |
| `BaseURL` | `string` | API base URL (default: https://tavusapi.com) |
| `HTTPClient` | `*http.Client` | Custom HTTP client |

### CreateConversation

Creates a new avatar conversation.

```go
func (c *Client) CreateConversation(ctx context.Context, req CreateConversationRequest) (*CreateConversationResponse, error)
```

**CreateConversationRequest:**

| Field | Type | Description |
|-------|------|-------------|
| `PalID` | `string` | PAL to use (default: `DefaultPalID`) |
| `FaceID` | `string` | Optional face override |
| `LiveKitURL` | `string` | LiveKit WebSocket URL |
| `LiveKitToken` | `string` | JWT token for avatar |
| `ConversationName` | `string` | Optional name |

**CreateConversationResponse:**

| Field | Type | Description |
|-------|------|-------------|
| `ConversationID` | `string` | Unique conversation ID |
| `ConversationURL` | `string` | Join URL (if available) |

### CreatePal

Creates a new PAL (Personality AI Likeness).

```go
func (c *Client) CreatePal(ctx context.Context, req CreatePalRequest) (*CreatePalResponse, error)
```

**CreatePalRequest:**

| Field | Type | Description |
|-------|------|-------------|
| `PalName` | `string` | Display name |
| `DefaultFaceID` | `string` | Face ID (required) |
| `PipelineMode` | `string` | Processing mode (default: "echo") |
| `TransportType` | `string` | Transport type (default: "livekit") |

### EndConversation

Ends an active conversation.

```go
func (c *Client) EndConversation(ctx context.Context, conversationID string) error
```

### SDK

Returns the underlying tavus-go SDK client for advanced usage.

```go
func (c *Client) SDK() *tavussdk.Client
```

## Constants

| Constant | Value | Description |
|----------|-------|-------------|
| `tavus.DefaultPalID` | `"pb87e71797da"` | Stock Tavus PAL for testing |

## See Also

- [Tavus Setup Guide](../guides/tavus-avatars.md)
- [Technical Design](../architecture/avatars/TRD.md)
- [Voice Pipeline](../architecture/voice-pipeline.md)
