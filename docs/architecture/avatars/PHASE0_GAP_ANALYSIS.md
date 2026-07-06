# Phase 0: SDK Gap Analysis - Complete

**Date**: 2025-07-06
**Status**: COMPLETE
**Result**: All required features are available in LiveKit Go SDK v2.17.0

## Executive Summary

**All required features are available in the LiveKit Go SDK.** We can proceed with a **pure Go implementation** of lip-sync avatars without any upstream contributions or workarounds.

## Feature Analysis

### 1. RPC Support

**Status**: FULLY SUPPORTED

**Location**: `/Users/johnwang/go/src/github.com/livekit/server-sdk-go/rpc.go` and `room.go`

**Available APIs**:

```go
// Register an RPC handler (avatar receives clear_buffer, etc.)
func (r *Room) RegisterRpcMethod(method string, handler RpcHandlerFunc) error

// Call an RPC method on a remote participant (agent calls avatar)
func (p *LocalParticipant) PerformRpc(params PerformRpcParams) (*string, error)

// Handler function type
type RpcHandlerFunc func(data RpcInvocationData) (string, error)

// RPC invocation data
type RpcInvocationData struct {
    RequestID       string
    CallerIdentity  string
    Payload         string
    ResponseTimeout time.Duration
}

// Error handling
type RpcError struct {
    Code    RpcErrorCode
    Message string
    Data    *string
}
```

**Example** (from `/examples/rpc/main.go`):

```go
// Register handler
room.RegisterRpcMethod("lk.clear_buffer", func(data lksdk.RpcInvocationData) (string, error) {
    // Handle clear buffer request
    return `{"status": "ok"}`, nil
})

// Perform RPC
res, err := room.LocalParticipant.PerformRpc(lksdk.PerformRpcParams{
    DestinationIdentity: "avatar-worker",
    Method:              "lk.playback_finished",
    Payload:             `{"position": 2.5, "interrupted": false}`,
})
```

### 2. ByteStream Support

**Status**: FULLY SUPPORTED

**Location**: `/Users/johnwang/go/src/github.com/livekit/server-sdk-go/streams.go` and `localparticipant.go`

**Available APIs**:

```go
// Create a byte stream writer (agent streams audio to avatar)
func (p *LocalParticipant) StreamBytes(options StreamBytesOptions) *ByteStreamWriter

// Register a byte stream handler (avatar receives audio)
func (r *Room) RegisterByteStreamHandler(topic string, handler ByteStreamHandler) error

// Stream options
type StreamBytesOptions struct {
    Topic                 string              // e.g., "lk.audio_stream"
    MimeType              string              // e.g., "audio/pcm"
    DestinationIdentities []string            // Target specific participants
    StreamId              *string             // Optional custom ID
    TotalSize             uint64              // Optional size hint
    Attributes            map[string]string   // Custom metadata
    OnProgress            func(progress float64)
    FileName              *string
}

// Writer interface
type ByteStreamWriter struct {
    Info ByteStreamInfo
    // Methods:
    // Write(data []byte, onDone *func())
    // Close()
}

// Handler type
type ByteStreamHandler func(reader *ByteStreamReader, participantIdentity string)

// Reader interface
type ByteStreamReader struct {
    Info ByteStreamInfo
    // Methods:
    // Read(bytes []byte) (int, error)
    // ReadByte() (byte, error)
    // ReadBytes(delim byte) ([]byte, error)
    // ReadAll() []byte
}
```

**Example** (from `/examples/datastreams/main.go`):

```go
// Send bytes
writer := room.LocalParticipant.StreamBytes(lksdk.StreamBytesOptions{
    Topic:                 "lk.audio_stream",
    MimeType:              "audio/pcm",
    DestinationIdentities: []string{"avatar-worker"},
    Attributes: map[string]string{
        "sample_rate": "24000",
        "channels":    "1",
    },
})

// Write audio frames
for frame := range audioFrames {
    writer.Write(frame, nil)
}
writer.Close()

// Receive bytes
room.RegisterByteStreamHandler("lk.audio_stream", func(reader *lksdk.ByteStreamReader, from string) {
    sampleRate := reader.Info.Attributes["sample_rate"]
    for {
        data := make([]byte, 1920)
        n, err := reader.Read(data)
        if err == io.EOF {
            break
        }
        processAudio(data[:n])
    }
})
```

### 3. Participant Attributes (lk.publish_on_behalf)

**Status**: FULLY SUPPORTED

**Location**: `/Users/johnwang/go/src/github.com/livekit/server-sdk-go/room.go` and `localparticipant.go`

**Available APIs**:

```go
// Set attributes when connecting
lksdk.ConnectToRoom(url, lksdk.ConnectInfo{
    APIKey:              apiKey,
    APISecret:           apiSecret,
    RoomName:            roomName,
    ParticipantIdentity: "avatar-worker",
    ParticipantAttributes: map[string]string{
        "lk.publish_on_behalf": "meeting-pm",  // Avatar publishes for agent
    },
}, callback)

// Or use the connection option
lksdk.ConnectToRoom(url, info, callback,
    lksdk.WithExtraAttributes(map[string]string{
        "lk.publish_on_behalf": agentIdentity,
    }),
)

// Update attributes after joining
room.LocalParticipant.SetAttributes(map[string]string{
    "status": "speaking",
})

// Read participant attributes
attrs := participant.Attributes()
```

**Token Generation**:

```go
// Token generation includes attributes via SetAttributes()
at := auth.NewAccessToken(apiKey, apiSecret)
at.SetVideoGrant(grant).
    SetIdentity("avatar-worker").
    SetAttributes(map[string]string{
        "lk.publish_on_behalf": "meeting-pm",
    })
token, _ := at.ToJWT()
```

### 4. CGO Requirements

**Status**: PURE GO - NO CGO REQUIRED

| Feature | CGO Required |
|---------|--------------|
| RPC | No |
| ByteStream | No |
| Participant Attributes | No |
| Token Generation | No |
| Room Connection | No |

The only CGO requirement in omni-livekit is for **Opus encoding** (audio codec), which is already handled and unrelated to avatar support.

## SDK Version

**Current**: `github.com/livekit/server-sdk-go/v2 v2.17.0`

All features verified against this version. No upgrade required.

## Mapping to Avatar Requirements

| Avatar Requirement | Go SDK Feature | Status |
|--------------------|----------------|--------|
| Stream audio to avatar | `LocalParticipant.StreamBytes()` | Ready |
| Receive audio stream | `Room.RegisterByteStreamHandler()` | Ready |
| Playback control RPC | `Room.RegisterRpcMethod()` | Ready |
| Call avatar RPC | `LocalParticipant.PerformRpc()` | Ready |
| Avatar publishes for agent | `lk.publish_on_behalf` attribute | Ready |
| Stream metadata | `StreamBytesOptions.Attributes` | Ready |

## Implementation Approach

### DataStreamAudioOutput

```go
package avatar

import (
    lksdk "github.com/livekit/server-sdk-go/v2"
)

const (
    AudioStreamTopic    = "lk.audio_stream"
    RPCClearBuffer      = "lk.clear_buffer"
    RPCPlaybackStarted  = "lk.playback_started"
    RPCPlaybackFinished = "lk.playback_finished"
)

type DataStreamAudioOutput struct {
    room                *lksdk.Room
    destinationIdentity string
    sampleRate          int

    writer     *lksdk.ByteStreamWriter
    onPlayback PlaybackCallback
}

func NewDataStreamAudioOutput(room *lksdk.Room, destIdentity string, sampleRate int) *DataStreamAudioOutput {
    out := &DataStreamAudioOutput{
        room:                room,
        destinationIdentity: destIdentity,
        sampleRate:          sampleRate,
    }

    // Register RPC handlers for playback events
    room.RegisterRpcMethod(RPCPlaybackStarted, out.handlePlaybackStarted)
    room.RegisterRpcMethod(RPCPlaybackFinished, out.handlePlaybackFinished)

    return out
}

func (o *DataStreamAudioOutput) CaptureFrame(ctx context.Context, frame []byte) error {
    if o.writer == nil {
        // Create new stream on first frame
        o.writer = o.room.LocalParticipant.StreamBytes(lksdk.StreamBytesOptions{
            Topic:                 AudioStreamTopic,
            MimeType:              "audio/pcm",
            DestinationIdentities: []string{o.destinationIdentity},
            Attributes: map[string]string{
                "sample_rate": fmt.Sprintf("%d", o.sampleRate),
                "channels":    "1",
                "encoding":    "linear16",
            },
        })
    }

    o.writer.Write(frame, nil)
    return nil
}

func (o *DataStreamAudioOutput) Flush(ctx context.Context) error {
    if o.writer != nil {
        o.writer.Close()
        o.writer = nil
    }
    return nil
}

func (o *DataStreamAudioOutput) ClearBuffer(ctx context.Context) error {
    // Call avatar's clear buffer RPC
    _, err := o.room.LocalParticipant.PerformRpc(lksdk.PerformRpcParams{
        DestinationIdentity: o.destinationIdentity,
        Method:              RPCClearBuffer,
        Payload:             "",
    })
    return err
}

func (o *DataStreamAudioOutput) handlePlaybackStarted(data lksdk.RpcInvocationData) (string, error) {
    if o.onPlayback != nil {
        o.onPlayback(PlaybackEvent{Type: PlaybackStarted})
    }
    return "", nil
}

func (o *DataStreamAudioOutput) handlePlaybackFinished(data lksdk.RpcInvocationData) (string, error) {
    var payload struct {
        Position    float64 `json:"playback_position"`
        Interrupted bool    `json:"interrupted"`
    }
    json.Unmarshal([]byte(data.Payload), &payload)

    if o.onPlayback != nil {
        o.onPlayback(PlaybackEvent{
            Type:        PlaybackFinished,
            Position:    payload.Position,
            Interrupted: payload.Interrupted,
        })
    }
    return "", nil
}
```

### Token Generation for Avatar

```go
package avatar

import (
    "time"

    "github.com/livekit/protocol/auth"
)

func GenerateAvatarToken(opts TokenOptions) (string, error) {
    at := auth.NewAccessToken(opts.APIKey, opts.APISecret)

    grant := &auth.VideoGrant{
        RoomJoin: true,
        Room:     opts.RoomName,
    }

    at.SetVideoGrant(grant).
        SetIdentity(opts.AvatarIdentity).
        SetName(opts.AvatarName).
        SetValidFor(opts.TTL).
        SetAttributes(map[string]string{
            "lk.publish_on_behalf": opts.PublishOnBehalf,
        })

    return at.ToJWT()
}

type TokenOptions struct {
    APIKey          string
    APISecret       string
    RoomName        string
    AvatarIdentity  string
    AvatarName      string
    PublishOnBehalf string
    TTL             time.Duration
}
```

## Conclusion

**No gaps identified.** The LiveKit Go SDK v2.17.0 provides all necessary features for implementing lip-sync avatar support:

1. **RPC** - Full bidirectional RPC with error handling
2. **ByteStream** - Efficient binary streaming with metadata
3. **Attributes** - `lk.publish_on_behalf` for avatar track delegation
4. **Pure Go** - No CGO dependencies for avatar features

## Next Steps

Proceed to **Phase 1: Core Avatar Infrastructure** with confidence. All required SDK features are available.

## References

- LiveKit Go SDK: https://github.com/livekit/server-sdk-go
- RPC Example: `/examples/rpc/main.go`
- DataStream Example: `/examples/datastreams/main.go`
- SDK Version: v2.17.0
