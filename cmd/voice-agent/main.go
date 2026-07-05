// Command voice-agent demonstrates a full voice AI agent using LiveKit.
//
// The agent joins a LiveKit room and responds to human participants using
// configurable STT, LLM, and TTS providers via omnivoice.
//
// Usage:
//
//	export LIVEKIT_URL="wss://your-project.livekit.cloud"
//	export LIVEKIT_API_KEY="your-api-key"
//	export LIVEKIT_API_SECRET="your-api-secret"
//
//	# STT provider (deepgram, openai, elevenlabs)
//	export STT_PROVIDER="deepgram"
//	export DEEPGRAM_API_KEY="your-key"  # or OPENAI_API_KEY, ELEVENLABS_API_KEY
//
//	# TTS provider (openai, elevenlabs, deepgram)
//	export TTS_PROVIDER="openai"
//	export OPENAI_API_KEY="your-key"    # or ELEVENLABS_API_KEY, DEEPGRAM_API_KEY
//
//	# LLM provider
//	export ANTHROPIC_API_KEY="your-key"
//
//	go run ./cmd/voice-agent
//
// Then open the printed URL in your browser to join and talk to the agent.
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/plexusone/omni-livekit/agent"
	"github.com/plexusone/omni-livekit/room"
	"github.com/plexusone/omnimeet-core/participant"
	"github.com/plexusone/omnivoice"
	"github.com/plexusone/omnivoice-core/stt"
	"github.com/plexusone/omnivoice-core/tts"

	// Import all omnivoice providers
	_ "github.com/plexusone/omnivoice/providers/all"
)

func main() {
	// Load config from environment
	serverURL := os.Getenv("LIVEKIT_URL")
	apiKey := os.Getenv("LIVEKIT_API_KEY")
	apiSecret := os.Getenv("LIVEKIT_API_SECRET")

	if serverURL == "" || apiKey == "" || apiSecret == "" {
		log.Fatal("Required: LIVEKIT_URL, LIVEKIT_API_KEY, LIVEKIT_API_SECRET")
	}

	// Get provider configuration
	sttProvider := getEnvOrDefault("STT_PROVIDER", "deepgram")
	ttsProvider := getEnvOrDefault("TTS_PROVIDER", "openai")

	// Resolve API keys for providers
	sttAPIKey := resolveAPIKey(sttProvider)
	ttsAPIKey := resolveAPIKey(ttsProvider)
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")

	if sttAPIKey == "" {
		log.Fatalf("No API key found for STT provider '%s'. Set %s_API_KEY", sttProvider, strings.ToUpper(sttProvider))
	}
	if ttsAPIKey == "" {
		log.Fatalf("No API key found for TTS provider '%s'. Set %s_API_KEY", ttsProvider, strings.ToUpper(ttsProvider))
	}
	if anthropicKey == "" {
		log.Fatal("Required: ANTHROPIC_API_KEY for LLM")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Create STT provider via omnivoice registry
	fmt.Printf("Initializing STT provider: %s\n", sttProvider)
	sttProv, err := omnivoice.GetSTTProvider(sttProvider, omnivoice.WithAPIKey(sttAPIKey))
	if err != nil {
		log.Fatalf("Failed to create STT provider: %v", err)
	}

	// Create TTS provider via omnivoice registry
	fmt.Printf("Initializing TTS provider: %s\n", ttsProvider)
	ttsProv, err := omnivoice.GetTTSProvider(ttsProvider, omnivoice.WithAPIKey(ttsAPIKey))
	if err != nil {
		log.Fatalf("Failed to create TTS provider: %v", err)
	}

	// Create room client
	roomClient, err := room.NewClient(room.Config{
		APIKey:    apiKey,
		APISecret: apiSecret,
		URL:       serverURL,
	})
	if err != nil {
		log.Fatalf("Failed to create room client: %v", err)
	}

	// Create a unique room name
	roomName := fmt.Sprintf("voice-%d", time.Now().Unix())

	// Create the room
	fmt.Printf("Creating room: %s\n", roomName)
	if _, err := roomClient.CreateRoom(ctx, roomName); err != nil {
		log.Fatalf("Failed to create room: %v", err)
	}

	// Generate token for human participant
	humanToken, err := roomClient.GenerateClientToken(roomName, "human-user", "You")
	if err != nil {
		log.Fatalf("Failed to generate human token: %v", err)
	}

	// Build LiveKit Meet URL
	meetURL := buildMeetURL(serverURL, humanToken)

	fmt.Println()
	fmt.Println("===========================================")
	fmt.Println("  LiveKit Voice Agent")
	fmt.Println("===========================================")
	fmt.Println()
	fmt.Printf("Room:     %s\n", roomName)
	fmt.Printf("STT:      %s\n", sttProvider)
	fmt.Printf("TTS:      %s\n", ttsProvider)
	fmt.Printf("LLM:      claude-sonnet-4\n")
	fmt.Println()
	fmt.Println("Join the meeting to talk to the AI agent:")
	fmt.Println()
	fmt.Printf("  %s\n", meetURL)
	fmt.Println()
	fmt.Println("Starting agent...")

	// Create voice agent
	va := &VoiceAgent{
		sttProvider:  sttProv,
		ttsProvider:  ttsProv,
		anthropicKey: anthropicKey,
		history:      []Message{},
		sampleRate:   48000,
	}

	// Create and start agent
	ag, err := agent.New(agent.Options{
		APIKey:        apiKey,
		APISecret:     apiSecret,
		ServerURL:     serverURL,
		Identity:      "ai-agent",
		Name:          "AI Assistant",
		AutoSubscribe: true,
		Audio: agent.AudioConfig{
			SampleRate: 48000,
			Channels:   1,
			TrackName:  "agent-audio",
		},
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Set up event handlers
	ag.OnParticipantJoined(func(p participant.Participant) {
		fmt.Printf("[+] %s joined\n", p.Name)
		// Greet the participant
		go func() {
			if err := va.speak(ctx, ag, fmt.Sprintf("Hello %s! I'm your AI assistant. How can I help you today?", p.Name)); err != nil {
				log.Printf("Error greeting participant: %v", err)
			}
		}()
	})

	ag.OnParticipantLeft(func(p participant.Participant) {
		fmt.Printf("[-] %s left\n", p.Name)
	})

	// Join the room
	if err := ag.Join(ctx, roomName); err != nil {
		log.Fatalf("Failed to join room: %v", err)
	}
	fmt.Println("Agent joined room!")

	// Start audio track for TTS output
	audioWriter, err := ag.StartAudio(ctx)
	if err != nil {
		log.Fatalf("Failed to start audio: %v", err)
	}
	va.audioWriter = audioWriter

	// Subscribe to audio from participants
	audioCh, err := ag.SubscribeToAllAudio(ctx)
	if err != nil {
		log.Printf("Warning: Could not subscribe to audio: %v", err)
	} else {
		// Process incoming audio through STT → LLM → TTS pipeline
		go va.processAudio(ctx, ag, audioCh)
	}

	fmt.Println()
	fmt.Println("Ready! Waiting for participants... (Ctrl+C to exit)")
	fmt.Println()

	// Wait for shutdown
	<-ctx.Done()

	// Cleanup
	if err := ag.Leave(ctx); err != nil {
		log.Printf("Error leaving room: %v", err)
	}
	if err := roomClient.DeleteRoom(context.Background(), roomName); err != nil {
		log.Printf("Error deleting room: %v", err)
	}

	fmt.Println("Goodbye!")
}

// VoiceAgent handles the voice processing pipeline
type VoiceAgent struct {
	sttProvider  stt.Provider
	ttsProvider  tts.Provider
	anthropicKey string
	audioWriter  agent.AudioWriter
	history      []Message
	historyMu    sync.Mutex
	speaking     bool
	speakingMu   sync.Mutex
	sampleRate   int
}

// Message represents a conversation turn
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// processAudio handles incoming audio from participants
func (va *VoiceAgent) processAudio(ctx context.Context, ag *agent.Agent, audioCh <-chan agent.AudioFrame) {
	// Buffer for collecting audio before sending to STT
	var audioBuffer []byte
	var lastAudioTime time.Time
	silenceThreshold := 500 * time.Millisecond

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case frame, ok := <-audioCh:
			if !ok {
				return
			}

			// Skip if we're currently speaking (avoid echo)
			va.speakingMu.Lock()
			speaking := va.speaking
			va.speakingMu.Unlock()
			if speaking {
				continue
			}

			audioBuffer = append(audioBuffer, frame.Data...)
			lastAudioTime = time.Now()

		case <-ticker.C:
			// Check for silence (end of speech)
			if len(audioBuffer) > 0 && time.Since(lastAudioTime) > silenceThreshold {
				// We have audio and user stopped speaking
				audioCopy := make([]byte, len(audioBuffer))
				copy(audioCopy, audioBuffer)
				audioBuffer = nil

				// Process in background
				go func(audio []byte) {
					if err := va.processUtterance(ctx, ag, audio); err != nil {
						log.Printf("Error processing utterance: %v", err)
					}
				}(audioCopy)
			}
		}
	}
}

// processUtterance handles a complete utterance: STT → LLM → TTS
func (va *VoiceAgent) processUtterance(ctx context.Context, ag *agent.Agent, audio []byte) error {
	// Minimum audio length check (avoid processing noise)
	if len(audio) < 4800 { // Less than 100ms at 48kHz
		return nil
	}

	// 1. Speech-to-Text using omnivoice provider
	fmt.Print("[STT] Transcribing... ")
	text, err := va.transcribe(ctx, audio)
	if err != nil {
		return fmt.Errorf("STT: %w", err)
	}
	if strings.TrimSpace(text) == "" {
		fmt.Println("(empty)")
		return nil
	}
	fmt.Printf("\"%s\"\n", text)

	// 2. LLM Response
	fmt.Print("[LLM] Thinking... ")
	response, err := va.generateResponse(ctx, text)
	if err != nil {
		return fmt.Errorf("LLM: %w", err)
	}
	fmt.Printf("\"%s\"\n", response)

	// 3. Text-to-Speech using omnivoice provider
	fmt.Print("[TTS] Speaking... ")
	if err := va.speak(ctx, ag, response); err != nil {
		return fmt.Errorf("TTS: %w", err)
	}
	fmt.Println("done")

	return nil
}

// transcribe converts audio to text using the configured STT provider
func (va *VoiceAgent) transcribe(ctx context.Context, audio []byte) (string, error) {
	// Convert PCM16 to WAV format
	wavData := pcmToWav(audio, va.sampleRate, 1)

	result, err := va.sttProvider.Transcribe(ctx, wavData, stt.TranscriptionConfig{
		Language:   "en",
		SampleRate: va.sampleRate,
		Encoding:   "linear16",
	})
	if err != nil {
		return "", err
	}

	return result.Text, nil
}

// generateResponse uses Claude to generate a response
func (va *VoiceAgent) generateResponse(ctx context.Context, userText string) (string, error) {
	va.historyMu.Lock()
	va.history = append(va.history, Message{Role: "user", Content: userText})
	messages := make([]Message, len(va.history))
	copy(messages, va.history)
	va.historyMu.Unlock()

	payload := map[string]any{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": 150,
		"system":     "You are a helpful voice assistant. Keep responses brief and conversational - 1-2 sentences max. Speak naturally as if having a phone conversation.",
		"messages":   messages,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.anthropic.com/v1/messages",
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", va.anthropicKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("anthropic error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("no response from Claude")
	}

	response := result.Content[0].Text

	va.historyMu.Lock()
	va.history = append(va.history, Message{Role: "assistant", Content: response})
	// Keep history manageable
	if len(va.history) > 20 {
		va.history = va.history[len(va.history)-20:]
	}
	va.historyMu.Unlock()

	return response, nil
}

// speak converts text to speech using the configured TTS provider and sends to LiveKit
func (va *VoiceAgent) speak(ctx context.Context, ag *agent.Agent, text string) error {
	va.speakingMu.Lock()
	va.speaking = true
	va.speakingMu.Unlock()
	defer func() {
		va.speakingMu.Lock()
		va.speaking = false
		va.speakingMu.Unlock()
	}()

	// Synthesize speech using omnivoice TTS provider
	result, err := va.ttsProvider.Synthesize(ctx, text, tts.SynthesisConfig{
		VoiceID:      "alloy", // Works for OpenAI; other providers will use their defaults
		SampleRate:   24000,   // Most providers output 24kHz
		OutputFormat: "pcm",   // Raw PCM audio
	})
	if err != nil {
		return fmt.Errorf("TTS synthesis: %w", err)
	}

	// Resample from provider's sample rate (usually 24kHz) to 48kHz
	audioData := result.Audio
	if result.SampleRate == 24000 {
		audioData = resample24to48(audioData)
	}

	// Write to LiveKit in chunks (20ms frames at 48kHz = 960 samples = 1920 bytes)
	frameSize := 1920
	for i := 0; i < len(audioData); i += frameSize {
		end := i + frameSize
		if end > len(audioData) {
			end = len(audioData)
		}
		frame := audioData[i:end]

		if _, err := va.audioWriter.Write(frame); err != nil {
			return err
		}

		// Pace the audio output
		time.Sleep(20 * time.Millisecond)
	}

	return nil
}

// Helper functions

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func resolveAPIKey(provider string) string {
	// Try provider-specific key first
	key := os.Getenv(strings.ToUpper(provider) + "_API_KEY")
	if key != "" {
		return key
	}
	// Some providers share keys (e.g., OpenAI for both STT and TTS)
	switch provider {
	case "openai":
		return os.Getenv("OPENAI_API_KEY")
	case "deepgram":
		return os.Getenv("DEEPGRAM_API_KEY")
	case "elevenlabs":
		return os.Getenv("ELEVENLABS_API_KEY")
	}
	return ""
}

func buildMeetURL(serverURL, token string) string {
	u, _ := url.Parse("https://meet.livekit.io/custom")
	q := u.Query()
	q.Set("liveKitUrl", serverURL)
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String()
}

func pcmToWav(pcm []byte, sampleRate, channels int) []byte {
	var buf bytes.Buffer
	w := func(data any) { _ = binary.Write(&buf, binary.LittleEndian, data) }

	// WAV header
	buf.WriteString("RIFF")
	w(uint32(36 + len(pcm)))
	buf.WriteString("WAVE")

	// fmt chunk
	buf.WriteString("fmt ")
	w(uint32(16))                        // chunk size
	w(uint16(1))                         // audio format (PCM)
	w(uint16(channels))                  // channels
	w(uint32(sampleRate))                // sample rate
	w(uint32(sampleRate * channels * 2)) // byte rate
	w(uint16(channels * 2))              // block align
	w(uint16(16))                        // bits per sample

	// data chunk
	buf.WriteString("data")
	w(uint32(len(pcm)))
	buf.Write(pcm)

	return buf.Bytes()
}

func resample24to48(audio []byte) []byte {
	if len(audio) < 2 {
		return audio
	}

	// Parse as int16 samples
	numSamples := len(audio) / 2
	samples := make([]int16, numSamples)
	for i := 0; i < numSamples; i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(audio[i*2:]))
	}

	// 2x upsample with linear interpolation
	resampled := make([]int16, numSamples*2)
	for i := 0; i < numSamples-1; i++ {
		resampled[i*2] = samples[i]
		resampled[i*2+1] = int16((int32(samples[i]) + int32(samples[i+1])) / 2)
	}
	resampled[(numSamples-1)*2] = samples[numSamples-1]
	resampled[(numSamples-1)*2+1] = samples[numSamples-1]

	// Convert back to bytes
	result := make([]byte, len(resampled)*2)
	for i, s := range resampled {
		binary.LittleEndian.PutUint16(result[i*2:], uint16(s))
	}
	return result
}
