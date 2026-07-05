# Installation

## Requirements

- Go 1.21 or later
- LiveKit account (Cloud or self-hosted)
- Native audio libraries for Opus encoding/decoding (required for voice agents)

!!! warning "Voice Agent Build Requirement"
    Voice agents **must** be built with `-tags opus` to enable Opus codec support for WebRTC audio. Without this tag, incoming audio from browsers won't be decoded properly and STT will fail silently.

    ```bash
    # Correct
    go run -tags opus ./cmd/voice-agent
    go build -tags opus ./cmd/voice-agent

    # Incorrect - will use fallback without Opus decoding
    go run ./cmd/voice-agent
    ```

## Install Package

```bash
go get github.com/plexusone/omni-livekit
```

## Native Dependencies

This package requires native audio libraries for Opus codec and resampling.

### macOS (Apple Silicon / ARM64)

On Apple Silicon Macs, use the ARM64 Homebrew at `/opt/homebrew`:

```bash
# Install ARM64 libraries
arch -arm64 /opt/homebrew/bin/brew install opus opusfile libsoxr
```

When building, set the CGO flags to use ARM64 libraries:

```bash
export CGO_CFLAGS="-I/opt/homebrew/include"
export CGO_LDFLAGS="-L/opt/homebrew/lib"
export PKG_CONFIG_PATH="/opt/homebrew/lib/pkgconfig"

go build -tags opus ./cmd/voice-agent
```

At runtime, ensure libraries can be found:

```bash
export DYLD_LIBRARY_PATH="/opt/homebrew/lib:$DYLD_LIBRARY_PATH"
```

### macOS (Intel)

```bash
brew install opus opusfile libsoxr
```

### Ubuntu/Debian

```bash
sudo apt-get install libopus-dev libopusfile-dev libsoxr-dev
```

### Fedora

```bash
sudo dnf install opus-devel opusfile-devel soxr-devel
```

### Windows

Use vcpkg or download prebuilt binaries:

```bash
vcpkg install opus libsoxr
```

## LiveKit Setup

### Option 1: LiveKit Cloud (Recommended)

1. Sign up at [LiveKit Cloud](https://cloud.livekit.io)
2. Create a new project
3. Copy your credentials from the dashboard

### Option 2: Self-Hosted

```bash
# Using Docker
docker run -d \
  -p 7880:7880 \
  -p 7881:7881 \
  -p 7882:7882/udp \
  livekit/livekit-server \
  --dev

# Generate API keys
docker exec <container> ./livekit-server generate-keys
```

## Environment Variables

Set these environment variables:

```bash
export LIVEKIT_URL="wss://your-project.livekit.cloud"
export LIVEKIT_API_KEY="your-api-key"
export LIVEKIT_API_SECRET="your-api-secret"
```

Or create a `.envrc` file:

```bash
# .envrc
export LIVEKIT_URL="wss://your-project.livekit.cloud"
export LIVEKIT_API_KEY="APIxxxxxxxx"
export LIVEKIT_API_SECRET="your-secret-here"
```

## Verify Installation

```go
package main

import (
    "fmt"
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
        panic(err)
    }

    fmt.Println("Connected to LiveKit successfully!")
    _ = client
}
```

## Next Steps

- [Quick Start](quickstart.md) - Create your first voice agent
- [Human Participation](guides/human-participation.md) - How humans join meetings
