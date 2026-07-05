# Voice Pipeline Architecture

This document describes the internal architecture of the voice agent's audio processing pipeline.

## Overview

The voice agent processes audio through a pipeline that transforms human speech into agent responses:

```
Browser Mic → WebRTC/Opus → RTP Packets → Opus Decode → PCM16
                                                          ↓
                                                        VAD
                                                          ↓
                                              Speech Buffer (accumulate)
                                                          ↓
                                              Silence Detection (800ms)
                                                          ↓
                                                    STT Provider
                                                          ↓
                                                   LLM (OmniAgent)
                                                          ↓
                                                    TTS Provider
                                                          ↓
PCM16 → Opus Encode → RTP Packets → WebRTC/Opus → Browser Speaker
```

## Audio Capture

### WebRTC Track Subscription

When a participant joins and publishes an audio track, the agent:

1. Detects the track via `OnTrackPublished` callback
2. Subscribes to the audio track
3. Waits for the WebRTC track to become available
4. Creates an audio reader that decodes Opus to PCM16

```go
// Track subscription flow
lkAgent.OnTrackPublished(func(p participant.Participant, t track.Track) {
    if t.Kind == "audio" {
        audioCh, _ := lkAgent.SubscribeToAudio(ctx, p.ID)
        go processAudio(ctx, audioCh)
    }
})
```

### Opus Decoding

LiveKit transmits audio using the Opus codec at 48kHz. The agent decodes incoming RTP packets:

1. Read RTP packet from WebRTC track
2. Parse RTP header to extract Opus payload
3. Decode Opus to PCM16 samples (16-bit signed, little-endian)
4. Send PCM frames to the processing channel

The Opus decoder requires the `opus` build tag:

```bash
go build -tags opus ./cmd/voice-agent
```

## Voice Activity Detection (VAD)

VAD distinguishes speech from background noise to determine when the user has finished speaking.

### Energy-Based Detection

The agent uses RMS (Root Mean Square) energy to detect speech:

```go
func calculateRMSEnergy(data []byte) int {
    var sumSquares int64
    numSamples := len(data) / 2

    for i := 0; i < numSamples; i++ {
        sample := int16(binary.LittleEndian.Uint16(data[i*2:]))
        sumSquares += int64(sample) * int64(sample)
    }

    meanSquare := sumSquares / int64(numSamples)
    return int(math.Sqrt(float64(meanSquare)))
}
```

### How It Works

1. **Calculate frame energy**: Each 20ms audio frame (~960 samples at 48kHz) has its RMS energy calculated
2. **Compare to threshold**: Frames with energy > 500 are classified as speech
3. **Accumulate speech**: Only speech frames are added to the buffer
4. **Detect silence**: When 800ms passes without speech frames, the utterance is complete

```
Audio Stream:  [noise][noise][SPEECH][SPEECH][SPEECH][noise][noise][noise]...
Energy:           120    80    2500    3200    1800    150    90    100
VAD Decision:     skip   skip   add     add     add    skip   skip  skip
                                                              ↑
                                                        800ms silence
                                                              ↓
                                                    Process utterance
```

### Tuning Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `energyThreshold` | 500 | RMS energy threshold for speech detection |
| `silenceThreshold` | 800ms | Time after last speech to trigger processing |

**Adjusting the energy threshold:**

- **Too low** (e.g., 200): Background noise triggers false speech detection
- **Too high** (e.g., 1000): Quiet speech may be missed
- **Recommended**: 400-600 for typical environments

**Adjusting the silence threshold:**

- **Too short** (e.g., 300ms): Cuts off speech mid-sentence
- **Too long** (e.g., 2000ms): Slow response, unnatural conversation
- **Recommended**: 600-1000ms for natural turn-taking

## Echo Cancellation

When the agent speaks, incoming audio is discarded to prevent feedback loops:

```go
// Set flag when speaking
va.speaking = true

// In processAudio, skip frames while speaking
if speaking {
    skippedWhileSpeaking++
    continue
}
```

This simple approach works because:

- Agent speech comes from TTS (not the mic)
- WebRTC may still send the agent's own voice back via the user's mic
- Discarding during TTS playback prevents the agent from "hearing itself"

## Speech-to-Text (STT)

After VAD detects an utterance, the accumulated PCM is sent to an STT provider:

1. Convert PCM16 buffer to WAV format (adds header)
2. Send to STT provider (Deepgram, OpenAI Whisper, etc.)
3. Receive transcription text

```go
wavData := pcmToWav(audio, 48000, 1)
result, _ := sttProvider.Transcribe(ctx, wavData, stt.TranscriptionConfig{
    Language:   "en",
    SampleRate: 48000,
    Encoding:   "linear16",
})
```

## LLM Processing

The transcribed text is processed by OmniAgent with the role's system prompt:

```go
response, _ := omniAgent.Process(ctx, sessionID, transcribedText)
```

The Meeting PM role provides context for:

- Tracking action items and decisions
- Maintaining meeting notes
- Generating summaries

## Text-to-Speech (TTS)

The LLM response is converted to audio:

1. Send text to TTS provider (Deepgram, OpenAI, ElevenLabs)
2. Receive PCM16 audio at 48kHz
3. Resample if needed (e.g., 24kHz → 48kHz)
4. Write PCM frames to the audio writer

```go
result, _ := ttsProvider.Synthesize(ctx, text, tts.SynthesisConfig{
    VoiceID:      "aura-asteria-en",
    SampleRate:   48000,
    OutputFormat: "linear16",
})
```

## Audio Output

PCM audio is encoded to Opus and sent via WebRTC:

1. Split PCM into 20ms frames (1920 bytes at 48kHz mono)
2. Encode each frame to Opus
3. Write to LiveKit track as RTP packets
4. Pace writes with 20ms sleep between frames

```go
frameSize := 1920  // 960 samples * 2 bytes
frameDuration := 20 * time.Millisecond

for i := 0; i < len(audioData); i += frameSize {
    frame := audioData[i : i+frameSize]
    audioWriter.Write(frame)
    time.Sleep(frameDuration)
}
```

## Timing Diagram

```
User speaks          Agent processes           Agent responds
    |                      |                        |
    v                      v                        v
[Speech]───────────>[Silence 800ms]──>[STT]──>[LLM]──>[TTS]──>[Playback]
    |                      |            |       |       |         |
    |<── ~2-5 seconds ─────|<── ~1s ───>|<─1s──>|<─1s──>|<──Xs───>|
```

Typical end-to-end latency: 2-4 seconds from user stops speaking to agent starts responding.

## Debug Output

Enable verbose logging to trace the pipeline:

```
[VAD] Speech started (energy=2500)
[DEBUG] Received 100 frames, speech=45, buffer=86400 bytes, energy=1200
[VAD] Silence detected after speech, processing 172800 bytes (90 speech frames)
[STT] Transcribing... "Hello, can you hear me?"
[Meeting PM] Processing...
[TTS] Speaking...
[SPEAK] Wrote 150 PCM frames to LiveKit
```

## Related Documentation

- [Voice Agent Guide](../guides/voice-agent.md) - How to run the voice agent
- [Agent API Reference](../api/agent.md) - Audio and track methods
