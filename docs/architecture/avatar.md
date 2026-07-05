# Static Image Avatar

This document describes the static image avatar feature for voice agents.

## Overview

Voice agents can display a static image (avatar) instead of a blank video tile. This makes the agent more visually present in meetings.

```
┌─────────────────────────────────────┐
│           Meeting View              │
├─────────────┬───────────────────────┤
│             │                       │
│   Avatar    │   Human Participant   │
│   Image     │   (Camera Feed)       │
│             │                       │
└─────────────┴───────────────────────┘
```

## Quick Start

Enable avatar via environment variable:

```bash
# Use the default embedded OmniAgent icon
export AGENT_AVATAR="true"

# Or use a custom pre-encoded avatar
export AGENT_AVATAR="/path/to/avatar.h264"
```

For usage details, see the [Voice Agent Guide](../guides/voice-agent.md#avatar).

## Two Approaches

There are two ways to use avatars, with different CGO requirements:

| Approach | CGO Required | When |
|----------|--------------|------|
| **Pre-encoded** (Recommended) | Build time only | Use `encode-avatar` tool once, then run without CGO |
| **Runtime encoding** | Runtime | Encode PNG/JPEG to H.264 on every startup |

### Pre-encoded Approach (Recommended)

The pre-encoded approach separates encoding from runtime:

1. **Build time**: Run `encode-avatar` tool (requires CGO + x264)
2. **Runtime**: Load the `.h264` file (no CGO required)

This is ideal for:

- Production deployments where minimizing dependencies matters
- Container images that shouldn't include x264/CGO
- Cross-compilation scenarios
- Consistent, repeatable builds

### Runtime Encoding Approach

The runtime approach encodes the image each time the agent starts:

1. **Runtime**: Load PNG/JPEG → encode to H.264 (requires CGO + x264)

This is useful for:

- Development where you're iterating on avatar images
- Dynamic avatars that change per-user

## Design Rationale

### Why H.264?

We chose H.264 over VP8 for static images:

| Factor | H.264 | VP8 |
|--------|-------|-----|
| Browser support | Universal (baseline profile) | Broad |
| Go library stability | x264-go (mature) | pion/mediadevices (build issues) |
| Encoder quality | Excellent for static images | Good |
| CGO dependency | Yes (x264) | Yes (libvpx) |

The `x264-go` library with `tune=stillimage` and `keyint=1` produces efficient keyframes optimized for static content.

### Why Pre-encode?

The key insight is that a static avatar doesn't change at runtime. Encoding it every time the agent starts wastes resources and adds a CGO dependency to production binaries.

By pre-encoding:

- **No CGO at runtime**: Production binary links only to standard Go libraries
- **Smaller containers**: No x264 libraries needed in the image
- **Faster startup**: No encoding delay
- **Reproducible**: Same bytes every time

### Interface Design

The `ImageWriter` interface abstracts over different implementations:

```go
type ImageWriter interface {
    UpdateImage(data []byte) error
    Close() error
}
```

Two implementations exist:

- `preencodedImageWriter` - Reads pre-encoded H.264, no CGO at runtime
- `h264ImageWriter` - Encodes at runtime, requires CGO (build tag: `cgo`)

### Build Tags

The codebase uses Go build tags to select implementations:

```
image_preencoded.go  - Always built (no tag), uses pre-encoded H.264
image_h264.go        - //go:build cgo - runtime H.264 encoding
image.go             - //go:build !cgo - fallback (limited browser support)
```

## Implementation Details

### Pre-encoded Image Pipeline

```
┌──────────────────────────────────────────────────────────────┐
│                    Build Time (CGO required)                  │
│                                                              │
│  avatar.png ──► decode ──► x264 encode ──► avatar.h264      │
│                   │           │                              │
│               image.RGBA   H.264 keyframe                    │
│                          (baseline profile)                   │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼ commit to repo
┌──────────────────────────────────────────────────────────────┐
│                    Runtime (NO CGO)                          │
│                                                              │
│  avatar.h264 ──► os.ReadFile ──► preencodedImageWriter      │
│                     │                    │                   │
│               raw bytes              write every 1s          │
│                                     (repeating keyframe)     │
│                                          │                   │
│                                          ▼                   │
│                                    LiveKit Track             │
└──────────────────────────────────────────────────────────────┘
```

### Runtime Encoding Pipeline (CGO)

```
┌──────────────────────────────────────────────────────────────┐
│                      Runtime (CGO required)                   │
│                                                              │
│  avatar.png ──► decode ──► x264 encode ──► h264ImageWriter  │
│                   │           │                 │            │
│               image.RGBA   H.264 keyframe   write every 1s   │
│                          (baseline profile)      │           │
│                                                  ▼           │
│                                            LiveKit Track     │
└──────────────────────────────────────────────────────────────┘
```

### H.264 Encoding Settings

The encoder is configured for static images:

```go
opts := &x264.Options{
    Width:     640,
    Height:    480,
    FrameRate: 1,
    KeyInt:    1,       // Every frame is a keyframe
    Tune:      "stillimage",
    Preset:    "medium",
    Profile:   "baseline", // Maximum browser compatibility
}
```

Key settings:

- **KeyInt=1**: Forces every frame to be a keyframe (I-frame). Essential for static content where there's no motion to predict.
- **Tune=stillimage**: Optimizes for content that doesn't change.
- **Profile=baseline**: Maximum browser compatibility. Avoids advanced features that some decoders don't support.
- **FrameRate=1**: One frame per second is sufficient for static content, minimizing bandwidth.

### Frame Rate

For static images, use **1 FPS** (default):

- Saves bandwidth (no motion = no need for high FPS)
- Reduces CPU usage
- Keyframes are re-sent periodically to handle packet loss

## File Sizes

### Input Images

Store avatar images as PNG/JPEG in your repository:

| Format | Typical Size | Git-Friendly |
|--------|--------------|--------------|
| PNG | 10-100 KB | ✓ Yes |
| JPEG | 5-50 KB | ✓ Yes |
| Raw RGBA | 1.2 MB | ✗ No |

### Encoded H.264

The pre-encoded `.h264` file is typically:

| Resolution | Typical Size |
|------------|--------------|
| 320x240 | 5-15 KB |
| 640x480 | 15-40 KB |
| 1280x720 | 40-100 KB |

These files are small enough to commit to Git alongside your source code.

## API Reference

### MediaMode

```go
type MediaMode string

const (
    AudioOnly      MediaMode = "audio_only"       // Audio track only (default)
    AudioWithImage MediaMode = "audio_with_image" // Audio + static image
    AudioWithVideo MediaMode = "audio_with_video" // Audio + video frames
)
```

### ImageConfig

```go
type ImageConfig struct {
    // Pre-encoded H.264 (recommended - no CGO at runtime)
    H264Path string // Path to pre-encoded .h264 file
    H264Data []byte // Pre-encoded H.264 bytes (alternative to H264Path)

    // Runtime encoding (requires CGO)
    Path string // Path to image file (PNG, JPEG, GIF)
    Data []byte // Raw image bytes (alternative to Path)

    // Resize options (only for runtime encoding)
    Width  int // Target width (0 = original)
    Height int // Target height (0 = original)

    // Track options
    FrameRate int    // Video track frame rate (default: 1)
    TrackName string // Track name (default: "video")
}
```

### Selection Priority

The agent selects the image source in this order:

1. `H264Data` - Pre-encoded bytes (fastest, no I/O)
2. `H264Path` - Pre-encoded file (no CGO at runtime)
3. `Data` - Runtime encoding from bytes (requires CGO)
4. `Path` - Runtime encoding from file (requires CGO)
5. **Default** - Embedded OmniAgent icon (no CGO, no configuration)

### Default Avatar

When `AudioWithImage` mode is enabled but no image is configured, the agent uses an embedded default avatar (the OmniAgent icon). This is stored in `agent/assets/default_avatar.h264` and embedded at compile time via `//go:embed`.

The default avatar is:

- 640x640 pixels
- ~47 KB (H.264 encoded)
- No runtime dependencies

## Alternative Approaches

### Option 1: Client-Side Placeholder

The web client can display a placeholder image when no video is available:

```javascript
// In your LiveKit client
participant.on('trackSubscribed', (track) => {
  if (track.kind === 'video') {
    // Show video
  } else {
    // Show avatar placeholder from participant metadata
  }
});
```

This avoids video encoding entirely but requires client-side changes.

### Option 2: Pre-Encoded Video Loop

For animated avatars, use a short looping video:

```bash
# Convert image to 1-second H.264 video
ffmpeg -loop 1 -i avatar.png -c:v libx264 -t 1 -r 1 -pix_fmt yuv420p avatar.mp4
```

## Dependencies

### For Pre-encoding (encode-avatar tool)

The `encode-avatar` tool requires CGO and x264:

```bash
# macOS
brew install x264

# Ubuntu/Debian
sudo apt-get install libx264-dev
```

Build the tool:

```bash
go build ./cmd/encode-avatar
```

### For Runtime Encoding

Same dependencies as above, plus your production binary must link against x264.

### For Pre-encoded Runtime (Recommended)

**No additional dependencies.** The production binary reads raw bytes from the `.h264` file.

## Related

- [Voice Agent Guide](../guides/voice-agent.md#avatar) - Usage guide with examples
- [Voice Pipeline Architecture](voice-pipeline.md) - Audio processing
