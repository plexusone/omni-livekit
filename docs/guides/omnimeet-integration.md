# OmniMeet Integration

omni-livekit provides the LiveKit implementation for [OmniMeet](https://github.com/plexusone/omnimeet-core), the PlexusOne meeting abstraction.

## Overview

OmniMeet provides a unified API for real-time meetings across platforms (LiveKit, Daily, Zoom, etc.). Using OmniMeet allows you to:

- Write provider-agnostic code
- Switch between LiveKit and other providers
- Use the OmniAgent meeting skill

## Basic Usage

### Import the Provider

```go
import (
    "github.com/plexusone/omni-livekit/omnimeet"
    omnimeetcore "github.com/plexusone/omnimeet-core"
    "github.com/plexusone/omnimeet-core/meeting"
)
```

### Create Provider

```go
provider, err := omnimeet.NewProvider(omnimeet.Config{
    APIKey:    os.Getenv("LIVEKIT_API_KEY"),
    APISecret: os.Getenv("LIVEKIT_API_SECRET"),
    ServerURL: os.Getenv("LIVEKIT_URL"),
})
if err != nil {
    log.Fatal(err)
}
defer provider.Close()
```

### Create Meeting

```go
m, err := provider.CreateMeeting(ctx, meeting.CreateRequest{
    Name: "Team Standup",
    Metadata: map[string]string{
        "team": "engineering",
    },
})
if err != nil {
    log.Fatal(err)
}
log.Printf("Meeting ID: %s", m.ID)
```

### Generate Join Tokens

```go
import (
    "github.com/plexusone/omnimeet-core/token"
    "github.com/plexusone/omnimeet-core/participant"
)

// Human participant
humanToken, _ := provider.CreateJoinToken(ctx, token.CreateRequest{
    MeetingID: m.ID,
    Participant: participant.Info{
        Name:     "Alice",
        Kind:     participant.KindHuman,
        Identity: "alice@example.com",
    },
})
log.Printf("Human join URL: %s", humanToken.JoinURL)

// Agent participant
agentToken, _ := provider.CreateJoinToken(ctx, token.CreateRequest{
    MeetingID: m.ID,
    Participant: participant.Info{
        Name:     "AI Assistant",
        Kind:     participant.KindAgent,
        Identity: "ai-assistant",
    },
})
```

### Join as Agent

```go
import "github.com/plexusone/omnimeet-core/provider"

// Get agent factory
factory := provider.(omnimeetcore.AgentParticipantFactory)

// Create agent participant
agent, err := factory.CreateAgentParticipant(provider.AgentParticipantOptions{
    AutoSubscribe: true,
})
if err != nil {
    log.Fatal(err)
}

// Set up event handlers
agent.OnParticipantJoined(func(p participant.Participant) {
    log.Printf("Participant joined: %s (%s)", p.Name, p.Kind)
})

agent.OnParticipantLeft(func(p participant.Participant) {
    log.Printf("Participant left: %s", p.Name)
})

// Join the meeting
if err := agent.JoinMeeting(ctx, m.ID, agentToken); err != nil {
    log.Fatal(err)
}
defer agent.LeaveMeeting(ctx)
```

## With Voice Integration

Use OmniMeet's voice package with OmniVoice providers:

```go
import (
    "github.com/plexusone/omnimeet-core/voice"
    "github.com/plexusone/omnivoice"
    "github.com/plexusone/omnivoice-core/stt"
    "github.com/plexusone/omnivoice-core/tts"
    _ "github.com/plexusone/omnivoice/providers/all"
)

// Get providers
sttProv, _ := omnivoice.GetSTTProvider("deepgram",
    omnivoice.WithAPIKey(os.Getenv("DEEPGRAM_API_KEY")),
)
ttsProv, _ := omnivoice.GetTTSProvider("openai",
    omnivoice.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
)

// Wrap agent with voice capabilities
voiceAgent := voice.NewVoiceAgentParticipant(agent, voice.Config{
    STTProvider: sttProv,
    TTSProvider: ttsProv,
    STTConfig: stt.TranscriptionConfig{
        Language:   "en",
        SampleRate: 16000,
    },
    TTSConfig: tts.SynthesisConfig{
        VoiceID:    "alloy",
        SampleRate: 48000,
    },
    OnTranscript: func(participantID, participantName, text string, isFinal bool) {
        if isFinal {
            log.Printf("%s: %s", participantName, text)

            // Process and respond
            response := processWithLLM(text)
            voiceAgent.Speak(ctx, response)
        }
    },
})

// Start transcribing all human participants
voiceAgent.StartTranscribingAll(ctx)
```

## Using MeetingSkill (OmniAgent)

For OmniAgent integration, use the meeting skill:

```go
import "github.com/plexusone/omnimeet-core/agent"

// Create voice-enabled meeting skill
skill, err := agent.NewVoiceMeetingSkill(
    provider,
    agent.SkillConfig{
        DefaultMeetingName:   "AI Meeting",
        DefaultAgentName:     "Assistant",
        AutoJoinAsAgent:      true,
        TranscriptionEnabled: true,
    },
    agent.VoiceSkillConfig{
        STTProvider: sttProv,
        TTSProvider: ttsProv,
        STTConfig: stt.TranscriptionConfig{
            Language:   "en",
            SampleRate: 16000,
        },
        TTSConfig: tts.SynthesisConfig{
            VoiceID:    "rachel",
            SampleRate: 48000,
        },
        OnTranscript: func(meetingID, participantID, participantName, text string, isFinal bool) {
            // Handle transcripts
        },
    },
)
if err != nil {
    log.Fatal(err)
}
defer skill.Close()

// Initialize
skill.Init(ctx)

// Available tools
for _, tool := range skill.Tools() {
    log.Printf("Tool: %s - %s", tool.Name(), tool.Description())
}

// Tools include:
// - create_meeting
// - list_meetings
// - get_meeting
// - join_meeting
// - leave_meeting
// - get_participants
// - speak_in_meeting
// - get_transcript
// - end_meeting
```

## Provider Registration

You can also use the OmniMeet registry:

```go
import (
    omnimeet "github.com/plexusone/omnimeet-core"
    _ "github.com/plexusone/omni-livekit/omnimeet" // Register provider
)

// Get provider from registry
provider, err := omnimeet.GetMeetingProvider("livekit",
    omnimeet.WithAPIKey(os.Getenv("LIVEKIT_API_KEY")),
    omnimeet.WithAPISecret(os.Getenv("LIVEKIT_API_SECRET")),
    omnimeet.WithServerURL(os.Getenv("LIVEKIT_URL")),
)
```

## Complete Example

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/plexusone/omni-livekit/omnimeet"
    "github.com/plexusone/omnimeet-core/meeting"
    "github.com/plexusone/omnimeet-core/participant"
    "github.com/plexusone/omnimeet-core/provider"
    "github.com/plexusone/omnimeet-core/token"
    "github.com/plexusone/omnimeet-core/voice"
    "github.com/plexusone/omnivoice"
    "github.com/plexusone/omnivoice-core/stt"
    "github.com/plexusone/omnivoice-core/tts"
    _ "github.com/plexusone/omnivoice/providers/all"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Handle shutdown
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    go func() { <-sigCh; cancel() }()

    // Create provider
    prov, _ := omnimeet.NewProvider(omnimeet.Config{
        APIKey:    os.Getenv("LIVEKIT_API_KEY"),
        APISecret: os.Getenv("LIVEKIT_API_SECRET"),
        ServerURL: os.Getenv("LIVEKIT_URL"),
    })
    defer prov.Close()

    // Create meeting
    m, _ := prov.CreateMeeting(ctx, meeting.CreateRequest{
        Name: "Demo Meeting",
    })
    log.Printf("Meeting: %s", m.ID)

    // Generate human join URL
    humanTok, _ := prov.CreateJoinToken(ctx, token.CreateRequest{
        MeetingID:   m.ID,
        Participant: participant.Info{Name: "Human", Kind: participant.KindHuman},
    })
    log.Printf("Human join: %s", humanTok.JoinURL)

    // Create agent
    factory := prov.(provider.AgentParticipantFactory)
    agent, _ := factory.CreateAgentParticipant(provider.AgentParticipantOptions{
        AutoSubscribe: true,
    })

    // Get voice providers
    sttProv, _ := omnivoice.GetSTTProvider("deepgram",
        omnivoice.WithAPIKey(os.Getenv("DEEPGRAM_API_KEY")))
    ttsProv, _ := omnivoice.GetTTSProvider("openai",
        omnivoice.WithAPIKey(os.Getenv("OPENAI_API_KEY")))

    // Wrap with voice
    voiceAgent := voice.NewVoiceAgentParticipant(agent, voice.Config{
        STTProvider: sttProv,
        TTSProvider: ttsProv,
        STTConfig:   stt.TranscriptionConfig{SampleRate: 16000},
        TTSConfig:   tts.SynthesisConfig{VoiceID: "alloy", SampleRate: 48000},
        OnTranscript: func(_, name, text string, isFinal bool) {
            if isFinal {
                log.Printf("%s: %s", name, text)
                voiceAgent.Speak(ctx, "I heard: "+text)
            }
        },
    })

    // Generate agent token and join
    agentTok, _ := prov.CreateJoinToken(ctx, token.CreateRequest{
        MeetingID:   m.ID,
        Participant: participant.Info{Name: "AI", Kind: participant.KindAgent},
    })
    agent.JoinMeeting(ctx, m.ID, agentTok)
    defer agent.LeaveMeeting(ctx)

    // Start transcribing
    voiceAgent.StartTranscribingAll(ctx)

    log.Println("Running... Ctrl+C to exit")
    <-ctx.Done()
}
```

## See Also

- [OmniMeet Documentation](https://github.com/plexusone/omnimeet-core)
- [OmniMeet Voice Integration Guide](https://github.com/plexusone/omnimeet-core/blob/main/docs/guides/voice-integration.md)
- [Human Participation](human-participation.md)
