# Voice Agent Guide

This guide covers building a complete voice AI agent with speech-to-text, LLM processing, and text-to-speech.

## Architecture

```
Human (Browser)
     │
     ▼ WebRTC Audio
     │
┌────────────────────────────────────────────────────────┐
│                    AI Agent                            │
│                                                        │
│  ┌──────────┐    ┌──────────┐    ┌──────────┐         │
│  │   STT    │───►│   LLM    │───►│   TTS    │         │
│  │(Deepgram)│    │ (Claude) │    │ (OpenAI) │         │
│  └──────────┘    └──────────┘    └──────────┘         │
│       ▲                               │               │
│       │                               ▼               │
│  ┌──────────────────────────────────────────┐         │
│  │          LiveKit Audio Track             │         │
│  └──────────────────────────────────────────┘         │
│                                                        │
└────────────────────────────────────────────────────────┘
     │
     ▼ WebRTC Audio
     │
Human (Browser)
```

## Building

The voice agent requires Opus codec support for encoding/decoding WebRTC audio. Build with the `opus` tag to enable native Opus encoding.

### Prerequisites

Install the required native libraries:

=== "macOS (Apple Silicon)"

    ```bash
    # Install ARM64 libraries via Homebrew
    arch -arm64 /opt/homebrew/bin/brew install opus libsoxr opusfile
    ```

=== "macOS (Intel)"

    ```bash
    brew install opus libsoxr opusfile
    ```

=== "Ubuntu/Debian"

    ```bash
    sudo apt-get install libopus-dev libsoxr-dev libopusfile-dev
    ```

### Build Commands

=== "macOS (Apple Silicon)"

    ```bash
    # Set CGO flags for ARM64 Homebrew
    export CGO_CFLAGS="-I/opt/homebrew/include"
    export CGO_LDFLAGS="-L/opt/homebrew/lib"
    export PKG_CONFIG_PATH="/opt/homebrew/lib/pkgconfig"

    # Build with opus support
    go build -tags opus ./cmd/voice-agent
    ```

    Or as a one-liner:

    ```bash
    CGO_CFLAGS="-I/opt/homebrew/include" \
    CGO_LDFLAGS="-L/opt/homebrew/lib" \
    PKG_CONFIG_PATH="/opt/homebrew/lib/pkgconfig" \
    go build -tags opus ./cmd/voice-agent
    ```

=== "macOS (Intel)"

    ```bash
    go build -tags opus ./cmd/voice-agent
    ```

=== "Linux"

    ```bash
    go build -tags opus ./cmd/voice-agent
    ```

### Runtime Library Path

On macOS, ensure the dynamic libraries can be found at runtime:

```bash
export DYLD_LIBRARY_PATH="/opt/homebrew/lib:$DYLD_LIBRARY_PATH"
./voice-agent
```

!!! warning "Without Opus"
    Building without the `opus` tag uses a fallback that passes raw PCM data. This will cause codec errors since WebRTC expects Opus-encoded audio. Always build with `-tags opus` for production use.

## Complete Example

### Using cmd/voice-agent

The repository includes a complete voice agent:

```bash
# Set all required credentials
export LIVEKIT_URL="wss://your-project.livekit.cloud"
export LIVEKIT_API_KEY="your-api-key"
export LIVEKIT_API_SECRET="your-api-secret"
export ANTHROPIC_API_KEY="your-anthropic-key"
export STT_PROVIDER="deepgram"
export STT_API_KEY="your-deepgram-key"
export TTS_PROVIDER="openai"
export TTS_API_KEY="your-openai-key"

# Optional: Enable avatar (displays image in video tile)
export AGENT_AVATAR="true"  # Use default OmniAgent icon
# Or use a custom pre-encoded avatar:
# export AGENT_AVATAR="/path/to/avatar.h264"

# Build with opus support (see Building section above)
go build -tags opus ./cmd/voice-agent

# Run
./voice-agent
```

### Custom Voice Agent

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/plexusone/omni-livekit/agent"
    "github.com/plexusone/omni-livekit/room"
    "github.com/plexusone/omnivoice"
    "github.com/plexusone/omnivoice-core/stt"
    "github.com/plexusone/omnivoice-core/tts"
    _ "github.com/plexusone/omnivoice/providers/all"
)

type VoiceAgent struct {
    agent       *agent.Agent
    sttProvider stt.Provider
    ttsProvider tts.Provider
    llmClient   LLMClient
}

func NewVoiceAgent() (*VoiceAgent, error) {
    // Create LiveKit agent
    ag, err := agent.New(agent.Options{
        APIKey:    os.Getenv("LIVEKIT_API_KEY"),
        APISecret: os.Getenv("LIVEKIT_API_SECRET"),
        ServerURL: os.Getenv("LIVEKIT_URL"),
        Identity:  "voice-agent",
        Name:      "AI Assistant",
    })
    if err != nil {
        return nil, err
    }

    // Get STT provider
    sttProv, err := omnivoice.GetSTTProvider(
        os.Getenv("STT_PROVIDER"),
        omnivoice.WithAPIKey(os.Getenv("STT_API_KEY")),
    )
    if err != nil {
        return nil, err
    }

    // Get TTS provider
    ttsProv, err := omnivoice.GetTTSProvider(
        os.Getenv("TTS_PROVIDER"),
        omnivoice.WithAPIKey(os.Getenv("TTS_API_KEY")),
    )
    if err != nil {
        return nil, err
    }

    return &VoiceAgent{
        agent:       ag,
        sttProvider: sttProv,
        ttsProvider: ttsProv,
        llmClient:   NewLLMClient(), // Your LLM client
    }, nil
}

func (v *VoiceAgent) Start(ctx context.Context, roomName string) error {
    // Set up audio handler
    v.agent.OnAudioFrame(func(frame agent.AudioFrame) {
        v.processAudio(ctx, frame)
    })

    // Join room
    return v.agent.Join(ctx, roomName)
}

func (v *VoiceAgent) processAudio(ctx context.Context, frame agent.AudioFrame) {
    // Transcribe
    result, err := v.sttProvider.Transcribe(ctx, frame.Data, stt.TranscriptionConfig{
        Language:   "en",
        SampleRate: frame.SampleRate,
    })
    if err != nil || result.Text == "" {
        return
    }

    log.Printf("User: %s", result.Text)

    // Process with LLM
    response, err := v.llmClient.Complete(ctx, result.Text)
    if err != nil {
        log.Printf("LLM error: %v", err)
        return
    }

    log.Printf("Agent: %s", response)

    // Synthesize speech
    audio, err := v.ttsProvider.Synthesize(ctx, response, tts.SynthesisConfig{
        VoiceID:      "alloy",
        SampleRate:   24000,
        OutputFormat: "pcm",
    })
    if err != nil {
        log.Printf("TTS error: %v", err)
        return
    }

    // Send audio to participant
    v.agent.SendAudio(ctx, audio.Audio, audio.SampleRate)
}
```

## Provider Selection

### STT Providers

| Provider | Strengths | Best For |
|----------|-----------|----------|
| Deepgram | Fast, accurate, streaming | Real-time conversations |
| OpenAI Whisper | High accuracy | Batch processing |
| Google | Multi-language | Enterprise |

### TTS Providers

| Provider | Strengths | Best For |
|----------|-----------|----------|
| OpenAI | Fast, good quality | General use |
| ElevenLabs | Best voice quality | Premium experiences |
| Google | Multi-language | Enterprise |

### LLM Providers

| Provider | Strengths | Best For |
|----------|-----------|----------|
| Claude (Anthropic) | Nuanced, safe | Complex conversations |
| GPT-4 (OpenAI) | Versatile | General use |
| Gemini (Google) | Fast | Quick responses |

## Audio Buffering

For smooth conversation flow, buffer audio before sending to STT:

```go
type AudioBuffer struct {
    samples   []int16
    threshold int // e.g., 16000 samples = 1 second at 16kHz
}

func (b *AudioBuffer) Add(frame agent.AudioFrame) []byte {
    // Convert bytes to samples and add
    samples := bytesToSamples(frame.Data)
    b.samples = append(b.samples, samples...)

    // Check if we have enough for transcription
    if len(b.samples) >= b.threshold {
        audio := samplesToBytes(b.samples)
        b.samples = nil
        return audio
    }
    return nil
}
```

## Voice Activity Detection (VAD)

Detect when the user is speaking. For detailed internals on how VAD works (RMS energy calculation, threshold tuning, silence detection), see the [Voice Pipeline Architecture](../architecture/voice-pipeline.md#voice-activity-detection-vad).

```go
func (v *VoiceAgent) processWithVAD(ctx context.Context, frame agent.AudioFrame) {
    // Simple energy-based VAD
    energy := calculateEnergy(frame.Data)

    if energy > v.vadThreshold {
        v.isSpeaking = true
        v.buffer.Add(frame)
    } else if v.isSpeaking {
        // User stopped speaking
        v.isSpeaking = false
        audio := v.buffer.Flush()
        go v.transcribeAndRespond(ctx, audio)
    }
}

func calculateEnergy(data []byte) float64 {
    var sum float64
    samples := bytesToSamples(data)
    for _, s := range samples {
        sum += float64(s) * float64(s)
    }
    return sum / float64(len(samples))
}
```

## Interruption Handling

Allow users to interrupt the agent:

```go
func (v *VoiceAgent) handleInterruption(ctx context.Context) {
    v.agent.OnAudioFrame(func(frame agent.AudioFrame) {
        energy := calculateEnergy(frame.Data)

        if v.isSpeaking && energy > v.interruptThreshold {
            // User interrupted
            v.stopSpeaking()
            log.Println("User interrupted agent")
        }
    })
}

func (v *VoiceAgent) stopSpeaking() {
    v.cancelTTS()
    v.agent.StopAudio()
}
```

## Conversation Context

Maintain conversation history for context-aware responses:

```go
type Conversation struct {
    history []Message
    maxLen  int
}

func (c *Conversation) AddUserMessage(text string) {
    c.history = append(c.history, Message{Role: "user", Content: text})
    c.trim()
}

func (c *Conversation) AddAssistantMessage(text string) {
    c.history = append(c.history, Message{Role: "assistant", Content: text})
    c.trim()
}

func (c *Conversation) trim() {
    if len(c.history) > c.maxLen {
        c.history = c.history[len(c.history)-c.maxLen:]
    }
}
```

## Performance Tips

1. **Use streaming STT** - Get partial results faster
2. **Stream TTS audio** - Start playing before full synthesis
3. **Pre-warm providers** - Make initial calls during setup
4. **Use connection pooling** - Reuse HTTP clients
5. **Buffer appropriately** - Balance latency vs accuracy

## Troubleshooting

### Agent speaks but doesn't hear/respond to me

**Symptom**: The agent's TTS output works (you hear the greeting), but it doesn't respond to your speech.

**Cause**: Built without `-tags opus`. The fallback code passes raw RTP packets instead of decoded PCM to STT.

**Solution**: Always build with the opus tag:

```bash
# For go run
go run -tags opus ./cmd/voice-agent

# For go build
go build -tags opus ./cmd/voice-agent
```

**Verify**: Check the build output - you should see debug messages like `[DEBUG] Receiving audio frames...` when you speak. Without opus, you may see frames but STT returns empty transcriptions.

### No audio output (can't hear agent)

**Symptom**: Agent joins but you don't hear anything.

**Possible causes**:

1. **Wrong Opus codec config**: WebRTC requires `ClockRate: 48000` and `Channels: 2` in SDP negotiation
2. **TTS returning wrong format**: Ensure TTS outputs PCM (`linear16`) not encoded Opus
3. **Sample rate mismatch**: Agent expects 48kHz; resample if TTS returns 24kHz

### Audio choppy or cutting off

**Symptom**: Agent speech is choppy, has jitter, or cuts off mid-sentence.

**Possible causes**:

1. **Concurrent speak calls**: Use a mutex to serialize TTS output
2. **Missing frame pacing**: Sleep 20ms between 20ms audio frames
3. **Network jitter**: LiveKit handles this, but ensure stable connection

```go
// Serialize speak calls
var speakLock sync.Mutex

func speak(text string) {
    speakLock.Lock()
    defer speakLock.Unlock()
    // ... TTS and audio output
}
```

### STT returns empty transcriptions

**Symptom**: Debug shows audio received but STT returns `""`.

**Possible causes**:

1. **Audio not decoded** (see first issue)
2. **Wrong audio format to STT**: Must be PCM16 little-endian, wrapped in WAV
3. **Audio too short**: Minimum ~100ms of audio needed
4. **Wrong sample rate**: Match STT provider requirements (usually 16kHz or 48kHz)

### Library not found at runtime

**Symptom**: `dyld: Library not loaded: libopus.dylib`

**Solution** (macOS):

```bash
export DYLD_LIBRARY_PATH="/opt/homebrew/lib:$DYLD_LIBRARY_PATH"
./voice-agent
```

Or link statically by using the appropriate CGO flags during build.

## Avatar

Display a static image instead of a blank video tile, making your agent more visually present in meetings.

### Enable via Environment Variable

The simplest way to enable avatar is with the `AGENT_AVATAR` environment variable:

```bash
# Use the default OmniAgent icon
export AGENT_AVATAR="true"

# Or use a custom pre-encoded avatar
export AGENT_AVATAR="/path/to/avatar.h264"
```

Then run your agent normally:

```bash
go run -tags opus ./cmd/voice-facilitator
```

| Value | Behavior |
|-------|----------|
| `true` or `1` | Uses embedded default OmniAgent icon |
| `/path/to/file.h264` | Uses custom pre-encoded avatar |
| *(not set)* | Audio only, no video tile |

### Enable via Go Code

For programmatic control, set `MediaMode` to `AudioWithImage`:

```go
agent, err := livekitagent.New(livekitagent.Options{
    APIKey:    apiKey,
    APISecret: apiSecret,
    ServerURL: serverURL,
    MediaMode: livekitagent.AudioWithImage,  // Uses default avatar
})
```

To use a custom avatar:

```go
agent, err := livekitagent.New(livekitagent.Options{
    MediaMode: livekitagent.AudioWithImage,
    Image: livekitagent.ImageConfig{
        H264Path: "avatar.h264",  // Custom pre-encoded avatar
    },
})
```

### Creating Custom Avatars (Pre-encoded)

The recommended approach uses a pre-encoded H.264 file. No CGO is required at runtime.

**Step 1: Install x264 (one-time)**

=== "macOS"

    ```bash
    brew install x264
    ```

=== "Ubuntu/Debian"

    ```bash
    sudo apt-get install libx264-dev
    ```

**Step 2: Pre-encode your avatar**

```bash
# Build the encode-avatar tool
go build ./cmd/encode-avatar

# Encode your avatar image
./encode-avatar -input avatar.png -output avatar.h264

# Optional: resize during encoding
./encode-avatar -input avatar.png -output avatar.h264 -width 640 -height 480
```

**Step 3: Commit the `.h264` file**

```bash
git add avatar.h264
git commit -m "feat: add agent avatar"
```

**Step 4: Use it**

Via environment variable:

```bash
export AGENT_AVATAR="./avatar.h264"
```

Or via Go code (see "Enable via Go Code" above).

Your production binary now works without CGO or x264 installed.

### Runtime Encoding (Alternative)

If you need dynamic avatars that change per-user, you can encode at runtime. This requires CGO and x264 on the production system.

```go
agent, err := livekitagent.New(livekitagent.Options{
    APIKey:    apiKey,
    APISecret: apiSecret,
    ServerURL: serverURL,
    MediaMode: livekitagent.AudioWithImage,
    Image: livekitagent.ImageConfig{
        Path:   "/path/to/avatar.png",  // Encode at runtime
        Width:  640,                     // Optional resize
        Height: 480,
    },
})
```

Build with CGO enabled (default):

```bash
go build ./cmd/voice-agent
```

### Embedding H.264 Data

For single-binary deployments, embed the pre-encoded data:

```go
import _ "embed"

//go:embed assets/avatar.h264
var avatarH264 []byte

func main() {
    agent, err := livekitagent.New(livekitagent.Options{
        MediaMode: livekitagent.AudioWithImage,
        Image: livekitagent.ImageConfig{
            H264Data: avatarH264,  // Embedded bytes
        },
    })
}
```

### Avatar Best Practices

| Recommendation | Why |
|----------------|-----|
| Use 640x480 or smaller | Larger resolutions waste bandwidth for static content |
| Keep frame rate at 1 FPS | Default is optimal for static images |
| Use PNG for quality | JPEG artifacts are visible at low frame rates |
| Pre-encode for production | No CGO dependency, faster startup |

For technical details on how avatar encoding works, see [Avatar Architecture](../architecture/avatar.md).

## See Also

- [Avatar Architecture](../architecture/avatar.md) - How avatar encoding works internally
- [Voice Pipeline Architecture](../architecture/voice-pipeline.md) - How the audio pipeline works internally
- [Human Participation](human-participation.md) - Frontend options
- [OmniMeet Integration](omnimeet-integration.md) - Meeting abstraction
- [OmniVoice Documentation](https://github.com/plexusone/omnivoice) - Provider details
