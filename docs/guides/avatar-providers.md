# Avatar Provider Comparison

This guide compares avatar providers that integrate with LiveKit for real-time lip-sync video.

## Quick Comparison

| Provider | Type | LiveKit Plugin | Latency | Price | Best For |
|----------|------|----------------|---------|-------|----------|
| [Tavus](#tavus) | Realistic humans | ✅ Official | ~300ms | ~$0.05-0.10/min | Video-trained clones |
| [HeyGen LiveAvatar](#heygen-liveavatar) | Realistic humans | ✅ Official | Sub-second | Varies | Highest quality realism |
| [D-ID](#d-id) | Photo-to-avatar | ✅ Official | Low | ~$0.10/min | Quick setup from photo |
| [bitHuman](#bithuman) | Animated + realistic | ✅ Official | <100ms | Low | Animated characters, edge deployment |
| [Simli](#simli) | Realistic + animated | ✅ Official | <300ms | <$0.01/min | Budget-friendly |

## Architecture

All providers follow the same LiveKit integration pattern:

```
┌─────────────────┐     ByteStream      ┌──────────────────┐
│   Your Agent    │ ──────────────────► │  Avatar Provider │
│  (audio output) │                     │   (lip-sync)     │
└─────────────────┘                     └────────┬─────────┘
                                                 │
                                          WebRTC video
                                                 │
┌─────────────────┐                     ┌────────▼─────────┐
│  Human User     │ ◄────────────────── │   LiveKit Room   │
│ (sees avatar)   │      video stream   │                  │
└─────────────────┘                     └──────────────────┘
```

## Provider Details

### Tavus

**Best for**: Photorealistic digital twins trained from video.

**Features**:

- Train custom faces from 5+ minute video recordings
- High-fidelity lip-sync and facial expressions
- PAL system for personality configuration
- Native LiveKit transport support

**Requirements**:

- Tavus API key
- PAL ID (or use default stock PAL: `pb87e71797da`)
- Video footage for custom face training

**omni-livekit support**: ✅ Fully integrated

```go
import "github.com/plexusone/omni-livekit/avatar/tavus"

session, _ := tavus.NewSession(tavus.SessionConfig{
    APIKey: os.Getenv("TAVUS_API_KEY"),
    PalID:  "your-pal-id",
})
```

**Resources**:

- [Tavus Documentation](https://docs.tavus.io/)
- [LiveKit Tavus Plugin](https://docs.livekit.io/agents/models/avatar/)
- [omni-livekit Tavus Guide](./tavus-avatars.md)

---

### HeyGen LiveAvatar

**Best for**: Highest quality realistic avatars with natural expressions.

**Features**:

- Avatar IV models with superior lip sync and micro-expressions
- Most natural idle behavior (blinks, micro-movements)
- 600+ avatar library
- 175+ language support
- FULL mode (HeyGen handles everything) or LITE mode (you handle LLM/TTS)

**Quality**: ⭐⭐⭐⭐⭐ - Best-in-class realism according to independent reviews.

**Requirements**:

- LiveAvatar API key from [app.liveavatar.com](https://app.liveavatar.com)

**omni-livekit support**: 🔜 Planned

**Resources**:

- [LiveAvatar Documentation](https://docs.liveavatar.com/)
- [LiveKit LiveAvatar Plugin](https://docs.livekit.io/agents/models/avatar/plugins/liveavatar/)
- [HeyGen Help Center](https://help.heygen.com/en/articles/12758516-introducing-liveavatar)

---

### D-ID

**Best for**: Quick setup - turn any photo into a talking avatar.

**Features**:

- Create avatar from any photo (no video training needed)
- Expressive v4 avatars with emotional responses
- Fast generation (~40 seconds from photo)
- Real-time focus with low latency

**Limitations**:

- Portrait framing only (no body, gestures, or scene composition)
- Lip sync can drift on longer segments
- Less realistic than video-trained alternatives

**Requirements**:

- D-ID API key

**omni-livekit support**: 🔜 Planned

**Resources**:

- [D-ID Documentation](https://docs.d-id.com/)
- [LiveKit D-ID Plugin](https://docs.livekit.io/agents/models/avatar/plugins/did/)
- [D-ID LiveKit Blog Post](https://www.d-id.com/blog/d-ids-livekit-plug-in/)

---

### bitHuman

**Best for**: Animated characters, edge deployment, CPU-only environments.

**Features**:

- Runs on-device (CPU only, no GPU required)
- Supports both realistic humans AND animated characters
- Animals, mythical creatures, playful characters
- <100ms latency
- Privacy-focused (can run air-gapped)
- Works on Raspberry Pi, Chromebooks, Mac Mini

**Use cases**:

- Trade show kiosks
- Educational apps
- NPCs in games
- AI companions
- Voice agents with custom character faces

**Requirements**:

- bitHuman API key or self-hosted SDK

**omni-livekit support**: 🔜 Planned

**Resources**:

- [bitHuman Documentation](https://docs.bithuman.ai/introduction)
- [LiveKit bitHuman Plugin](https://docs.livekit.io/agents/models/avatar/plugins/bithuman/)
- [bitHuman Website](https://www.bithuman.ai/)

---

### Simli

**Best for**: Budget-friendly real-time avatars.

**Features**:

- Lowest cost (<$0.01/min with Trinity-1 API)
- Face library (no training required)
- Configurable emotions
- Trinity (25 FPS) and Legacy (30 FPS) avatar types
- Gaussian splatting 3D technology

**Limitations**:

- Quality concerns noted in reviews (idle behavior artifacts, mechanical movements)
- Transitions between speaking/idle not as smooth

**API Endpoints**:

```
POST https://api.simli.ai/startAudioToVideoSession
POST https://api.simli.ai/getIceServer
POST /createE2ESessionToken
POST /startE2ESession
POST /textToVideoStream
POST /audioToVideoStream
```

**Requirements**:

- Simli API key
- Face ID (from library or custom)

**omni-livekit support**: 🔜 Planned

**Resources**:

- [Simli Documentation](https://docs.simli.com/overview)
- [LiveKit Simli Plugin](https://docs.livekit.io/agents/models/avatar/plugins/simli/)
- [Simli GitHub](https://github.com/simliai/)

---

## Choosing a Provider

### For Realistic Human Avatars

| Priority | Recommended Provider |
|----------|---------------------|
| Highest quality | HeyGen LiveAvatar |
| Custom digital twin | Tavus |
| Quick from photo | D-ID |
| Lowest cost | Simli |

### For Animated / 3D Characters

| Priority | Recommended Provider |
|----------|---------------------|
| Best quality | **bitHuman** |
| Edge/CPU deployment | **bitHuman** |
| Budget option | Simli |

### For Edge Deployment

| Environment | Recommended Provider |
|-------------|---------------------|
| No GPU available | bitHuman |
| Air-gapped | bitHuman (self-hosted) |
| Raspberry Pi / IoT | bitHuman |

## Implementation Status

| Provider | omni-livekit Status | Package |
|----------|---------------------|---------|
| Tavus | ✅ Implemented | `avatar/tavus` |
| HeyGen | 🔜 Planned | `avatar/heygen` |
| D-ID | 🔜 Planned | `avatar/did` |
| bitHuman | 🔜 Planned | `avatar/bithuman` |
| Simli | 🔜 Planned | `avatar/simli` |

## Adding a New Provider

All avatar providers implement the `avatar.Session` interface:

```go
type Session interface {
    Start(ctx context.Context, opts StartOptions) error
    WaitForJoin(ctx context.Context, timeout time.Duration) error
    Close(ctx context.Context) error
    AudioOutput() AudioDestination
}
```

To add a new provider:

1. Create `avatar/<provider>/` directory
2. Implement `Session` interface
3. Create `register.go` with `init()` to auto-register
4. Add config to `avatar.Config` struct

See [avatar/tavus/](../../avatar/tavus/) for reference implementation.

## See Also

- [Tavus Setup Guide](./tavus-avatars.md)
- [Avatar API Reference](../api/avatar.md)
- [Voice Pipeline Architecture](../architecture/voice-pipeline.md)
