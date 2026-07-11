# Tavus Lip-Sync Avatars

This guide explains how to integrate Tavus lip-sync avatars with your voice agent.

## Overview

Tavus provides real-time lip-sync avatars that synchronize with your agent's speech. The avatar joins your LiveKit room as a participant and publishes video that matches the audio your agent produces.

```
┌─────────────────────────────────────────────────────────────┐
│                      LiveKit Room                           │
│                                                             │
│  ┌─────────────────┐              ┌─────────────────────┐   │
│  │   Your Agent    │   audio      │   Tavus Avatar      │   │
│  │  (omni-livekit) │──────────────►  (lip-sync video)   │   │
│  └─────────────────┘              └─────────────────────┘   │
│                                            │                │
│                                            ▼                │
│  ┌─────────────────────────────────────────────────────┐   │
│  │              Human Participant                       │   │
│  │         Sees avatar video + hears audio              │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## Prerequisites

1. **Tavus Account** - Sign up at [tavus.io](https://tavus.io)
2. **Tavus API Key** - Get from your Tavus dashboard
3. **PAL ID** - Create a PAL (Personality AI Likeness) or use the default
4. **LiveKit Setup** - Working omni-livekit installation

## Installation

The Tavus integration uses the [tavus-go](https://github.com/plexusone/tavus-go) SDK:

```bash
go get github.com/plexusone/omni-livekit@latest
```

## Quick Start (Recommended)

Use the unified avatar factory for the simplest integration:

```go
package main

import (
    "context"
    "os"
    "time"

    "github.com/plexusone/omni-livekit/avatar"
    _ "github.com/plexusone/omni-livekit/avatar/tavus" // Register Tavus provider
)

func main() {
    ctx := context.Background()

    // Set up Tavus avatar using the factory
    result, err := avatar.Setup(avatar.SetupConfig{
        Provider: avatar.ProviderTavus,
        Tavus: avatar.TavusConfig{
            APIKey: os.Getenv("TAVUS_API_KEY"),
            PalID:  os.Getenv("TAVUS_PAL_ID"), // Optional, uses default
        },
        LiveKitURL:       os.Getenv("LIVEKIT_URL"),
        LiveKitAPIKey:    os.Getenv("LIVEKIT_API_KEY"),
        LiveKitAPISecret: os.Getenv("LIVEKIT_API_SECRET"),
    })
    if err != nil {
        panic(err)
    }

    // Start the avatar session (after agent joins room)
    err = result.Session.Start(ctx, avatar.StartOptions{
        Room:             agentRoom,  // Your LiveKit room
        AgentIdentity:    "my-agent",
        LiveKitURL:       os.Getenv("LIVEKIT_URL"),
        LiveKitAPIKey:    os.Getenv("LIVEKIT_API_KEY"),
        LiveKitAPISecret: os.Getenv("LIVEKIT_API_SECRET"),
    })
    if err != nil {
        panic(err)
    }
    defer result.Session.Close(ctx)

    // Wait for avatar to join
    if err := result.Session.WaitForJoin(ctx, 30*time.Second); err != nil {
        panic(err)
    }

    // Stream TTS audio to avatar for lip-sync
    audioOut := result.Session.AudioOutput()
    // ... your TTS pipeline sends audio here
}
```

## Direct Client Usage

For more control, use the Tavus client directly:

```go
import "github.com/plexusone/omni-livekit/avatar/tavus"

// Create Tavus client
client, err := tavus.NewClient(tavus.ClientConfig{
    APIKey: os.Getenv("TAVUS_API_KEY"),
})

// Create a conversation (avatar session)
resp, err := client.CreateConversation(ctx, tavus.CreateConversationRequest{
    PalID:        "your-pal-id",  // Or use tavus.DefaultPalID for testing
    LiveKitURL:   os.Getenv("LIVEKIT_URL"),
    LiveKitToken: avatarToken,    // Token with publish permissions
})

// Avatar joins the room and starts publishing video
fmt.Printf("Conversation started: %s\n", resp.ConversationID)
```

## Configuration

### Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `TAVUS_API_KEY` | Your Tavus API key | Yes |
| `LIVEKIT_URL` | LiveKit server URL | Yes |
| `LIVEKIT_API_KEY` | LiveKit API key (for token generation) | Yes |
| `LIVEKIT_API_SECRET` | LiveKit API secret (for token generation) | Yes |

### Client Options

```go
client, _ := tavus.NewClient(tavus.ClientConfig{
    // Required
    APIKey: "your-api-key",

    // Optional: Custom API endpoint
    BaseURL: "https://api.tavus.io",

    // Optional: Custom HTTP client
    HTTPClient: &http.Client{
        Timeout: 60 * time.Second,
    },
})
```

## Token Generation

The avatar needs a LiveKit token with special permissions to publish video on behalf of your agent:

```go
import "github.com/plexusone/omni-livekit/avatar"

token, err := avatar.GenerateAvatarToken(avatar.TokenConfig{
    APIKey:     os.Getenv("LIVEKIT_API_KEY"),
    APISecret:  os.Getenv("LIVEKIT_API_SECRET"),
    RoomName:   "my-room",
    AvatarID:   "tavus-avatar",
    OnBehalfOf: "ai-agent",  // Your agent's identity
})
```

The `OnBehalfOf` field sets the `lk.publish_on_behalf` attribute, allowing the avatar's video to appear as if published by your agent.

## Audio Streaming

Stream your TTS audio to the avatar using `AudioDestination`:

```go
import "github.com/plexusone/omni-livekit/avatar"

// Create audio output to avatar
output := avatar.NewDataStreamAudioOutput(avatar.DataStreamConfig{
    Room:                room,
    DestinationIdentity: "tavus-avatar",
    SampleRate:          24000,
})

// Stream TTS audio frames
for frame := range ttsFrames {
    output.Write(frame)
}

// Signal end of speech
output.Flush()
```

### Handling Interruptions

When the user interrupts the agent, clear the avatar's audio buffer:

```go
// User started speaking - interrupt avatar
output.ClearBuffer()
```

## Session Management

### Creating a Session

```go
session := tavus.NewSession(tavus.SessionConfig{
    Client:   client,
    PalID:    "your-pal-id",
    FaceID:   "optional-face-override",
})

err := session.Start(ctx, avatar.StartOptions{
    Room:             room,
    AgentIdentity:    "ai-agent",
    LiveKitURL:       os.Getenv("LIVEKIT_URL"),
    LiveKitAPIKey:    os.Getenv("LIVEKIT_API_KEY"),
    LiveKitAPISecret: os.Getenv("LIVEKIT_API_SECRET"),
})
```

### Waiting for Avatar to Join

```go
// Wait up to 10 seconds for avatar to join and publish video
err := session.WaitForJoin(ctx, 10*time.Second)
if err != nil {
    log.Printf("Avatar failed to join: %v", err)
}
```

### Ending a Session

```go
// Clean up when done
err := session.Close(ctx)
```

Or end a conversation directly:

```go
err := client.EndConversation(ctx, conversationID)
```

## PAL Configuration

A PAL (Personality AI Likeness) defines your avatar's appearance and behavior.

### Using the Default PAL

For testing, use the stock Tavus PAL:

```go
resp, _ := client.CreateConversation(ctx, tavus.CreateConversationRequest{
    PalID: tavus.DefaultPalID,  // "pb87e71797da"
    // ...
})
```

### Creating a Custom PAL

```go
pal, err := client.CreatePal(ctx, tavus.CreatePalRequest{
    PalName:       "My Assistant",
    DefaultFaceID: "your-face-id",
    PipelineMode:  "echo",      // Use "echo" for LiveKit integration
    TransportType: "livekit",   // Required for LiveKit
})
```

## Error Handling

```go
import "github.com/plexusone/omni-livekit/avatar"

resp, err := client.CreateConversation(ctx, req)
if err != nil {
    if errors.Is(err, avatar.ErrInvalidConfig) {
        // Missing required configuration
    }

    var providerErr *avatar.ProviderError
    if errors.As(err, &providerErr) {
        // Tavus API error
        log.Printf("Provider: %s, Operation: %s, Cause: %v",
            providerErr.Provider,
            providerErr.Operation,
            providerErr.Unwrap())
    }
}
```

## Best Practices

1. **Reuse clients** - Create one `tavus.Client` and reuse it across requests
2. **Handle timeouts** - Set reasonable timeouts for avatar join operations
3. **Clean up sessions** - Always call `Close()` or `EndConversation()` when done
4. **Test locally first** - Use `QueueAudioOutput` for local testing without Tavus

## Troubleshooting

### Avatar doesn't join

- Verify `TAVUS_API_KEY` is correct
- Check LiveKit token has correct room permissions
- Ensure LiveKit server is reachable from Tavus

### No video appears

- Confirm `lk.publish_on_behalf` attribute is set in token
- Check that the room exists before creating conversation
- Verify PAL ID is valid

### Audio out of sync

- Ensure sample rate matches (24kHz default for Tavus)
- Check network latency between your agent and Tavus
- Consider buffering strategies for high-latency scenarios

## Next Steps

- [Technical Design](../architecture/avatars/TRD.md) - Deep dive into avatar architecture
- [Voice Pipeline](../architecture/voice-pipeline.md) - How audio flows through the system
- [API Reference](../api/avatar.md) - Full API documentation
