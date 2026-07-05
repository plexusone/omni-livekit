# Quick Start

This guide walks you through creating a voice AI agent that humans can talk to via their browser.

## Prerequisites

- [Installation](getting-started.md) completed
- Environment variables set

## Step 1: Run the Demo Agent

The simplest way to get started is with the built-in demo:

```bash
cd /path/to/omni-livekit

# Run the demo
go run ./cmd/agent-demo
```

Output:

```
Creating room: demo-1720000000

===========================================
  LiveKit Agent Demo
===========================================

Room: demo-1720000000

Join as human participant:

  https://meet.livekit.io/custom?liveKitUrl=wss%3A%2F%2F...&token=eyJ...

Starting agent...
Agent joined room successfully!

Waiting for participants... (Ctrl+C to exit)
```

## Step 2: Join as Human

1. Copy the join URL from the output
2. Open it in your browser
3. Grant microphone permission when prompted
4. Start talking!

The demo agent will:

- Log when participants join/leave
- Log received audio frames
- (Does not respond - see voice-agent for full STT/TTS)

## Step 3: Run the Voice Agent (Full Demo)

For a complete voice agent with speech-to-text and text-to-speech:

```bash
# Set STT/TTS provider credentials
export STT_PROVIDER="deepgram"
export STT_API_KEY="your-deepgram-key"
export TTS_PROVIDER="openai"
export TTS_API_KEY="your-openai-key"
export ANTHROPIC_API_KEY="your-anthropic-key"

# Optional: Show avatar image in video tile
export AGENT_AVATAR="true"

# Run the voice agent
go run -tags opus ./cmd/voice-agent
```

This agent:

1. Listens to human speech (via Deepgram STT)
2. Processes with Claude (Anthropic)
3. Responds with voice (via OpenAI TTS)

## Step 4: Build Your Own Agent

### Basic Agent Structure

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/plexusone/omni-livekit/agent"
    "github.com/plexusone/omni-livekit/room"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Handle shutdown
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-sigCh
        cancel()
    }()

    // Create room client
    roomClient, _ := room.NewClient(room.Config{
        APIKey:    os.Getenv("LIVEKIT_API_KEY"),
        APISecret: os.Getenv("LIVEKIT_API_SECRET"),
        URL:       os.Getenv("LIVEKIT_URL"),
    })

    // Create room
    roomName := "my-agent-room"
    roomClient.CreateRoom(ctx, roomName)

    // Generate human join URL
    humanToken, _ := roomClient.GenerateClientToken(roomName, "human", "Human")
    fmt.Printf("Human join URL: https://meet.livekit.io/custom?liveKitUrl=%s&token=%s\n",
        os.Getenv("LIVEKIT_URL"), humanToken)

    // Create agent
    ag, _ := agent.New(agent.Options{
        APIKey:    os.Getenv("LIVEKIT_API_KEY"),
        APISecret: os.Getenv("LIVEKIT_API_SECRET"),
        ServerURL: os.Getenv("LIVEKIT_URL"),
        Identity:  "my-agent",
        Name:      "My AI Agent",
    })

    // Handle events
    ag.OnParticipantJoined(func(p participant.Participant) {
        log.Printf("Participant joined: %s", p.Name)
    })

    ag.OnAudioFrame(func(frame agent.AudioFrame) {
        // Process audio here
        // Send to STT, then LLM, then TTS
    })

    // Join room
    ag.Join(ctx, roomName)

    // Wait for shutdown
    <-ctx.Done()
    ag.Leave(ctx)
}
```

### With OmniVoice Integration

```go
import (
    "github.com/plexusone/omnivoice"
    _ "github.com/plexusone/omnivoice/providers/all"
)

// Get providers
sttProv, _ := omnivoice.GetSTTProvider("deepgram",
    omnivoice.WithAPIKey(os.Getenv("DEEPGRAM_API_KEY")),
)
ttsProv, _ := omnivoice.GetTTSProvider("elevenlabs",
    omnivoice.WithAPIKey(os.Getenv("ELEVENLABS_API_KEY")),
)

// Use in your audio processing loop
ag.OnAudioFrame(func(frame agent.AudioFrame) {
    // Transcribe
    result, _ := sttProv.Transcribe(ctx, frame.Data, stt.TranscriptionConfig{
        SampleRate: frame.SampleRate,
    })

    // Process with LLM...
    response := processWithLLM(result.Text)

    // Synthesize speech
    audio, _ := ttsProv.Synthesize(ctx, response, tts.SynthesisConfig{
        VoiceID: "rachel",
    })

    // Send audio back
    ag.SendAudio(ctx, audio.Audio)
})
```

## Next Steps

- [Human Participation](guides/human-participation.md) - Frontend options
- [Voice Agent Guide](guides/voice-agent.md) - Full voice pipeline
- [OmniMeet Integration](guides/omnimeet-integration.md) - Meeting abstraction
