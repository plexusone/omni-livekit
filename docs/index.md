# omni-livekit

**LiveKit provider for OmniMeet and OmniVoice**

omni-livekit enables voice AI agents to participate in LiveKit rooms via WebRTC, providing real-time audio communication with web and mobile applications.

## Overview

```
┌───────────────┐        ┌─────────────────┐        ┌───────────────────┐
│  Browser/App  │◄──────►│  LiveKit Cloud  │◄──────►│   AI Agent        │
│   (WebRTC)    │ WebRTC │    or Server    │ WebRTC │   (omni-livekit)  │
└───────────────┘        └─────────────────┘        └───────────────────┘
```

## Features

- **WebRTC Audio**: Low-latency voice communication (<200ms)
- **OmniMeet Provider**: Full meeting abstraction support
- **OmniVoice Gateway**: Voice AI pipeline integration
- **Multi-Platform**: Web, mobile, and desktop clients
- **Flexible Participation**: Humans join via browser, agents join via Go SDK

## Quick Example

```go
package main

import (
    "context"
    "os"

    "github.com/plexusone/omni-livekit/omnimeet"
    "github.com/plexusone/omnimeet-core/meeting"
)

func main() {
    ctx := context.Background()

    // Create LiveKit provider
    provider, _ := omnimeet.NewProvider(omnimeet.Config{
        APIKey:    os.Getenv("LIVEKIT_API_KEY"),
        APISecret: os.Getenv("LIVEKIT_API_SECRET"),
        ServerURL: os.Getenv("LIVEKIT_URL"),
    })

    // Create a meeting
    m, _ := provider.CreateMeeting(ctx, meeting.CreateRequest{
        Name: "AI Demo",
    })

    // Agent joins the meeting...
    // Human joins via LiveKit Meet: https://meet.livekit.io
}
```

## Use Cases

| Use Case | Description |
|----------|-------------|
| Voice Assistants | AI assistants in web/mobile apps |
| Meeting Bots | AI participants in video meetings |
| Customer Support | Real-time voice support agents |
| Education | AI tutors and language practice |
| Gaming | Voice-enabled NPCs |

## Architecture

omni-livekit serves two purposes:

1. **OmniMeet Provider** - Implements the OmniMeet meeting abstraction for LiveKit
2. **OmniVoice Gateway** - Implements the WebRTC gateway interface for voice AI

```
OmniAgent
    │
    ├── OmniMeet (meetings)
    │       └── omni-livekit/omnimeet
    │
    └── OmniVoice (voice AI)
            └── omni-livekit/omnivoice/gateway
```

## Next Steps

- [Installation](getting-started.md) - Set up omni-livekit
- [Quick Start](quickstart.md) - Create your first voice agent
- [Human Participation](guides/human-participation.md) - How humans join meetings
