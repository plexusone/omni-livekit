# Omni-LiveKit

[![Go CI][go-ci-svg]][go-ci-url]
[![Go Lint][go-lint-svg]][go-lint-url]
[![Go SAST][go-sast-svg]][go-sast-url]
[![Docs][docs-godoc-svg]][docs-godoc-url]
[![Docs][docs-mkdoc-svg]][docs-mkdoc-url]
[![Visualization][viz-svg]][viz-url]
[![License][license-svg]][license-url]

 [go-ci-svg]: https://github.com/plexusone/omni-livekit/actions/workflows/go-ci.yaml/badge.svg?branch=main
 [go-ci-url]: https://github.com/plexusone/omni-livekit/actions/workflows/go-ci.yaml
 [go-lint-svg]: https://github.com/plexusone/omni-livekit/actions/workflows/go-lint.yaml/badge.svg?branch=main
 [go-lint-url]: https://github.com/plexusone/omni-livekit/actions/workflows/go-lint.yaml
 [go-sast-svg]: https://github.com/plexusone/omni-livekit/actions/workflows/go-sast-codeql.yaml/badge.svg?branch=main
 [go-sast-url]: https://github.com/plexusone/omni-livekit/actions/workflows/go-sast-codeql.yaml
 [docs-godoc-svg]: https://pkg.go.dev/badge/github.com/plexusone/omni-livekit
 [docs-godoc-url]: https://pkg.go.dev/github.com/plexusone/omni-livekit
 [docs-mkdoc-svg]: https://img.shields.io/badge/Go-dev%20guide-blue.svg
 [docs-mkdoc-url]: https://plexusone.dev/omni-livekit
 [viz-svg]: https://img.shields.io/badge/visualizaton-Go-blue.svg
 [viz-url]: https://mango-dune-07a8b7110.1.azurestaticapps.net/?repo=plexusone%2Fomnillm
 [loc-svg]: https://tokei.rs/b1/github/plexusone/omnillm
 [repo-url]: https://github.com/plexusone/omni-livekit
 [license-svg]: https://img.shields.io/badge/license-MIT-blue.svg
 [license-url]: https://github.com/plexusone/omni-livekit/blob/main/LICENSE

LiveKit voice gateway for WebRTC-based voice AI applications.

## Overview

LiveKit provides pure WebRTC voice without phone network involvement. While PSTN gateways (Twilio, Telnyx, Vonage, Plivo) also offer WebRTC SDKs, they primarily bridge browser/app connections to the phone network. LiveKit is designed for direct browser-to-browser and app-to-app communication.

```
┌───────────────┐        ┌─────────────────┐        ┌───────────────────┐
│  Browser/App  │◄──────►│  LiveKit Cloud  │◄──────►│   OmniVoice       │
│   (WebRTC)    │ WebRTC │    or Server    │ WebRTC │   Voice Gateway   │
└───────────────┘        └─────────────────┘        └───────────────────┘
```

## Use Cases

| Use Case | PSTN Gateways | LiveKit |
|----------|---------------|---------|
| Phone calls (PSTN) | Yes | No |
| Browser WebRTC | Yes (bridges to PSTN) | Yes (native) |
| Mobile apps | WebRTC SDK available | WebRTC SDK available |
| Browser-to-browser | Via PSTN bridge | Direct WebRTC |
| Latency | 300-500ms (PSTN hop) | <200ms |
| Cost model | Per-minute telephony | Infrastructure only |

## Installation

```bash
go get github.com/plexusone/omni-livekit
```

### Native Dependencies

This package requires native audio libraries for Opus encoding/decoding and resampling:

**macOS:**
```bash
brew install opus opusfile libsoxr
```

**Ubuntu/Debian:**
```bash
apt-get install libopus-dev libopusfile-dev libsoxr-dev
```

**Fedora:**
```bash
dnf install opus-devel opusfile-devel soxr-devel
```

### Build Tags

Voice agents **must** be built with `-tags opus` to enable proper Opus codec support:

```bash
# Required for voice agents
go run -tags opus ./cmd/voice-agent
go build -tags opus ./cmd/voice-agent

# Without -tags opus, audio won't be decoded properly and STT will fail
```

## Quick Start

### Go Backend (AI Agent)

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
    // Create gateway implementing coregateway.WebRTCGateway interface
    gw, err := gateway.New(gateway.Config{
        LiveKitURL:    os.Getenv("LIVEKIT_URL"),
        LiveKitAPIKey: os.Getenv("LIVEKIT_API_KEY"),
        LiveKitSecret: os.Getenv("LIVEKIT_API_SECRET"),
        RoomName:      "voice-agent",
        AgentIdentity: "ai-agent",
        SampleRate:    24000,
    })
    if err != nil {
        log.Fatal(err)
    }

    // Handle participant joins (WebRTC-specific handler)
    gw.OnParticipantJoined(func(p *coregateway.ParticipantInfo) error {
        log.Printf("Participant joined: %s (%s)", p.DisplayName, p.Identity)
        return nil // Accept participant
    })

    ctx := context.Background()
    if err := gw.Start(ctx); err != nil {
        log.Fatal(err)
    }
}
```

### Generate Client Token

```go
import "github.com/plexusone/omni-livekit/room"

client, _ := room.NewClient(room.Config{
    APIKey:    os.Getenv("LIVEKIT_API_KEY"),
    APISecret: os.Getenv("LIVEKIT_API_SECRET"),
    URL:       os.Getenv("LIVEKIT_URL"),
})

// Generate token for web/mobile client
token, _ := client.GenerateClientToken("voice-agent", "user-123", "John")
```

### React Frontend

```tsx
import { LiveKitRoom, useVoiceAssistant } from '@livekit/components-react';

function VoiceAgent() {
    const token = await fetchTokenFromBackend();

    return (
        <LiveKitRoom
            serverUrl={process.env.LIVEKIT_URL}
            token={token}
            connect={true}
            audio={true}
        >
            <VoiceUI />
        </LiveKitRoom>
    );
}
```

## Interface

omni-livekit implements the `coregateway.WebRTCGateway` interface from omnivoice-core:

```go
type WebRTCGateway interface {
    Name() ProviderName
    Start(ctx context.Context) error
    Stop() error
    OnParticipantJoined(handler ParticipantHandler)
    JoinRoom(ctx context.Context, roomName string) error
    LeaveRoom() error
    CurrentRoom() string
    GetSession(participantID string) (WebRTCSession, bool)
    ListSessions() []WebRTCSession
    GenerateClientToken(roomName, identity, displayName string) (string, error)
}
```

This is different from the `Gateway` interface used by PSTN providers (Twilio, Telnyx, etc.) which uses phone numbers and `MakeCall()`.

## Architecture

### Audio Flow

```
Client (Browser/Mobile)
    │
    ▼ WebRTC Audio Track (Opus)
    │
┌───────────────────────────────────────┐
│           LiveKit Server              │
└───────────────────────────────────────┘
    │
    ▼ WebRTC Audio Track (Opus)
    │
┌───────────────────────────────────────┐
│       Go Backend (omni-livekit)       │
│                                       │
│  ┌─────────────────────────────────┐  │
│  │  PCMRemoteTrack                 │  │
│  │  (Opus → PCM16 decode)          │  │
│  └─────────────┬───────────────────┘  │
│                ▼                      │
│  ┌─────────────────────────────────┐  │
│  │  Voice Pipeline                 │  │
│  │  STT → LLM → TTS                │  │
│  │  (or Voice-to-Voice)            │  │
│  └─────────────┬───────────────────┘  │
│                ▼                      │
│  ┌─────────────────────────────────┐  │
│  │  PCMLocalTrack                  │  │
│  │  (PCM16 → Opus encode)          │  │
│  └─────────────────────────────────┘  │
└───────────────────────────────────────┘
```

### Key Differences from PSTN Gateways

| Aspect | PSTN Gateways | LiveKit |
|--------|---------------|---------|
| Primary use | Phone network access | Browser/app-to-app |
| Connection model | Phone number dialing | Room-based participation |
| Signaling | HTTP webhooks + WebRTC | WebRTC/WebSocket |
| Audio format | mulaw 8kHz (PSTN), Opus (WebRTC) | Opus (configurable rate) |
| Identity | Phone number or SIP URI | Participant ID |
| Session model | Call legs (inbound/outbound) | Room participants |

## Human Participation

When an AI agent joins a LiveKit room, humans can join via several methods:

### Option 1: LiveKit Meet (No Code Required)

LiveKit provides a free hosted web UI. Generate a join URL with token:

```go
// Generate token and URL
token, _ := client.GenerateClientToken("my-room", "user-123", "Alice")
joinURL := fmt.Sprintf("https://meet.livekit.io/custom?liveKitUrl=%s&token=%s",
    url.QueryEscape(serverURL), token)
// Share joinURL with the human participant
```

The human opens the link in their browser, grants mic/camera permissions, and joins.

### Option 2: LiveKit Cloud Dashboard

If using LiveKit Cloud, create rooms and generate join links from the dashboard UI.

### Option 3: Custom Frontend

Build your own UI with LiveKit's client SDKs:

| Platform | SDK | Install |
|----------|-----|---------|
| Web (JS/TS) | `livekit-client` | `npm install livekit-client` |
| React | `@livekit/components-react` | `npm install @livekit/components-react` |
| iOS | LiveKit Swift SDK | Swift Package Manager |
| Android | LiveKit Android SDK | Maven |
| Flutter | `livekit_client` | pub.dev |

**React Example:**

```tsx
import { LiveKitRoom, VideoConference } from '@livekit/components-react';
import '@livekit/components-styles';

function MeetingRoom({ token, serverUrl }) {
  return (
    <LiveKitRoom token={token} serverUrl={serverUrl} connect={true}>
      <VideoConference />
    </LiveKitRoom>
  );
}
```

**Minimal HTML/JS:**

```html
<script src="https://unpkg.com/livekit-client/dist/livekit-client.umd.js"></script>
<script>
  const room = new LivekitClient.Room();
  await room.connect('wss://your-server.livekit.cloud', token);

  // Enable microphone
  await room.localParticipant.setMicrophoneEnabled(true);
</script>
```

### Recommendation

| Use Case | Recommended Approach |
|----------|---------------------|
| Testing/Demos | LiveKit Meet |
| Quick prototype | LiveKit React components |
| Production app | Custom frontend with full UX control |

## Environment Variables

```bash
# LiveKit Cloud or self-hosted server
export LIVEKIT_URL="wss://your-app.livekit.cloud"
export LIVEKIT_API_KEY="your-api-key"
export LIVEKIT_API_SECRET="your-api-secret"
```

## Audio Configuration

The gateway supports configurable sample rates for optimal voice AI performance:

```go
gateway.Config{
    SampleRate: 24000,  // 24kHz for high quality (OpenAI Realtime API)
    // SampleRate: 16000, // 16kHz for most STT/TTS
    Channels: 1,        // Mono audio
}
```

## Session Events

```go
session, ok := gw.GetSession(participantID)
if !ok {
    return
}

// WebRTCSession provides participant info
fmt.Printf("Room: %s\n", session.RoomName())
fmt.Printf("Participant: %s\n", session.Participant().DisplayName)

// Handle events
for event := range session.Events() {
    switch event.Type {
    case coregateway.EventUserTranscript:
        fmt.Printf("User said: %s\n", event.Data)
    case coregateway.EventAgentTranscript:
        fmt.Printf("Agent said: %s\n", event.Data)
    case coregateway.EventInterruption:
        fmt.Println("User interrupted agent")
    }
}

// Send audio to participant
samples := make([]int16, 480) // 20ms at 24kHz
session.SendAudio(samples)
```

## Integration with OmniVoice

```go
import (
    "github.com/plexusone/omni-livekit/omnivoice/gateway"
    "github.com/plexusone/omnivoice"
    _ "github.com/plexusone/omnivoice/providers/all"
)

// Use OmniVoice for STT/TTS
stt, _ := omnivoice.GetSTTProvider("deepgram", omnivoice.WithAPIKey(apiKey))
tts, _ := omnivoice.GetTTSProvider("elevenlabs", omnivoice.WithAPIKey(apiKey))

// LiveKit gateway receives audio → STT → LLM → TTS → sends audio
```

### Voice Pipeline Modes

**Standard Pipeline (STT → LLM → TTS):**

```
Audio In → STT → Text → LLM → Text → TTS → Audio Out
```

This mode uses separate providers for speech-to-text, language model, and text-to-speech.

**Voice-to-Voice (Lower Latency):**

```
Audio In → Voice Model → Audio Out
```

OmniVoice supports direct voice-to-voice with:

- **Deepgram** - Nova-3 Voice Agent
- **Google** - Gemini Live
- **OpenAI** - Realtime API

Voice-to-voice eliminates the text intermediate step, reducing latency for real-time conversations.

## Lip-Sync Avatars

For realistic talking head video, integrate with [Tavus](https://tavus.io/) using the [tavus-go](https://github.com/plexusone/tavus-go) SDK:

```go
import "github.com/plexusone/omni-livekit/avatar/tavus"

client, _ := tavus.NewClient(tavus.ClientConfig{
    APIKey: os.Getenv("TAVUS_API_KEY"),
})

// Create conversation with LiveKit transport
resp, _ := client.CreateConversation(ctx, tavus.CreateConversationRequest{
    PalID:        "your-pal-id",
    LiveKitURL:   os.Getenv("LIVEKIT_URL"),
    LiveKitToken: avatarToken,
})
```

The avatar joins the LiveKit room as a participant with synchronized lip movements.

## OmniMeet Integration

This package also provides the LiveKit provider for [OmniMeet](https://github.com/plexusone/omnimeet-core), the PlexusOne meeting abstraction.

```go
import "github.com/plexusone/omni-livekit/omnimeet"

provider, _ := omnimeet.NewProvider(omnimeet.Config{
    APIKey:    os.Getenv("LIVEKIT_API_KEY"),
    APISecret: os.Getenv("LIVEKIT_API_SECRET"),
    ServerURL: os.Getenv("LIVEKIT_URL"),
})

// Create meeting
m, _ := provider.CreateMeeting(ctx, meeting.CreateRequest{
    Name: "Team Standup",
})

// Join as agent
factory := provider.(provider.AgentParticipantFactory)
agent, _ := factory.CreateAgentParticipant(provider.AgentParticipantOptions{
    AutoSubscribe: true,
})
```

See the [OmniMeet LiveKit Provider documentation](https://github.com/plexusone/omnimeet-core/blob/main/docs/providers/livekit.md) for details.

## Related Packages

- [omnimeet-core](https://github.com/plexusone/omnimeet-core) - Meeting abstraction core
- [omnivoice-core](https://github.com/plexusone/omnivoice-core) - Voice interfaces
- [omnivoice](https://github.com/plexusone/omnivoice) - STT/TTS providers
- [livekit/server-sdk-go](https://github.com/livekit/server-sdk-go) - LiveKit Go SDK

## License

MIT License
