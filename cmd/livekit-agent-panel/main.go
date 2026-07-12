// Command livekit-agent-panel demonstrates a multi-agent panel discussion.
//
// The panel consists of a human moderator who asks questions, and 2-4 AI panelists
// who take turns responding. Each panelist has a unique voice and personality.
//
// Usage:
//
//	export LIVEKIT_URL="wss://your-project.livekit.cloud"
//	export LIVEKIT_API_KEY="your-api-key"
//	export LIVEKIT_API_SECRET="your-api-secret"
//	export ANTHROPIC_API_KEY="your-key"
//	export OPENAI_API_KEY="your-key"  # For TTS
//
//	# Panel configuration
//	export PANEL_TOPIC="The future of AI agents"
//	export PANEL_SIZE=3  # 2-4 panelists (default: 3)
//
//	go run -tags opus ./cmd/livekit-agent-panel
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/plexusone/omni-livekit/agent"
	"github.com/plexusone/omni-livekit/room"
	"github.com/plexusone/omnimeet-core/participant"
	"github.com/plexusone/omnivoice"
	"github.com/plexusone/omnivoice-core/stt"

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

	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	if anthropicKey == "" {
		log.Fatal("Required: ANTHROPIC_API_KEY")
	}

	ttsAPIKey := os.Getenv("OPENAI_API_KEY")
	if ttsAPIKey == "" {
		log.Fatal("Required: OPENAI_API_KEY (for TTS)")
	}

	sttAPIKey := os.Getenv("DEEPGRAM_API_KEY")
	if sttAPIKey == "" {
		sttAPIKey = os.Getenv("OPENAI_API_KEY") // Fallback to OpenAI for STT
	}

	// Panel configuration
	topic := getEnvOrDefault("PANEL_TOPIC", "The future of AI agents")
	panelSize := getEnvInt("PANEL_SIZE", 3)
	if panelSize < 2 {
		panelSize = 2
	}
	if panelSize > 4 {
		panelSize = 4
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

	// Create TTS provider
	ttsProv, err := omnivoice.GetTTSProvider("openai", omnivoice.WithAPIKey(ttsAPIKey))
	if err != nil {
		log.Fatalf("Failed to create TTS provider: %v", err)
	}

	// Create STT provider
	sttProvider := "deepgram"
	if os.Getenv("DEEPGRAM_API_KEY") == "" {
		sttProvider = "openai"
	}
	sttProv, err := omnivoice.GetSTTProvider(sttProvider, omnivoice.WithAPIKey(sttAPIKey))
	if err != nil {
		log.Fatalf("Failed to create STT provider: %v", err)
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
	roomName := fmt.Sprintf("panel-%d", time.Now().Unix())

	// Create the room
	fmt.Printf("Creating room: %s\n", roomName)
	if _, err := roomClient.CreateRoom(ctx, roomName); err != nil {
		log.Fatalf("Failed to create room: %v", err)
	}

	// Generate token for human moderator
	moderatorToken, err := roomClient.GenerateClientToken(roomName, "moderator", "Moderator")
	if err != nil {
		log.Fatalf("Failed to generate moderator token: %v", err)
	}

	// Build meeting URL
	meetURL := buildMeetURL(serverURL, moderatorToken)

	// Get panelist configurations
	panelistConfigs := buildPanelistConfigs(panelSize)

	fmt.Println()
	fmt.Println("===========================================")
	fmt.Println("  Panel Discussion Agent")
	fmt.Println("===========================================")
	fmt.Println()
	fmt.Printf("Topic:      %s\n", topic)
	fmt.Printf("Room:       %s\n", roomName)
	fmt.Printf("Panelists:  %d\n", panelSize)
	fmt.Println()
	fmt.Println("Panelists:")
	for _, cfg := range panelistConfigs {
		fmt.Printf("  - %s (voice: %s)\n", cfg.Name, cfg.Voice)
	}
	fmt.Println()
	fmt.Println("Join as moderator to start the discussion:")
	fmt.Println()
	fmt.Printf("  %s\n", meetURL)
	fmt.Println()

	// Create panelists
	panelists := make([]*Panelist, len(panelistConfigs))
	for i, cfg := range panelistConfigs {
		// Create agent for this panelist
		agentOpts := agent.Options{
			APIKey:        apiKey,
			APISecret:     apiSecret,
			ServerURL:     serverURL,
			Identity:      fmt.Sprintf("panelist-%s", strings.ToLower(cfg.Name)),
			Name:          cfg.Name,
			AutoSubscribe: false, // Panelists don't need to receive audio
			Audio: agent.AudioConfig{
				SampleRate: 48000,
				Channels:   1,
				TrackName:  fmt.Sprintf("%s-audio", strings.ToLower(cfg.Name)),
			},
		}

		ag, err := agent.New(agentOpts)
		if err != nil {
			log.Fatalf("Failed to create agent for %s: %v", cfg.Name, err)
		}

		panelists[i] = NewPanelist(cfg, ag, ttsProv, anthropicKey)
	}

	// Create coordinator
	coord := NewCoordinator(panelists, topic)

	// Join all panelists to the room
	fmt.Println("Panelists joining room...")
	for _, p := range panelists {
		if err := p.Agent.Join(ctx, roomName); err != nil {
			log.Fatalf("Failed to join room for %s: %v", p.Config.Name, err)
		}
		if err := p.StartAudio(ctx); err != nil {
			log.Fatalf("Failed to start audio for %s: %v", p.Config.Name, err)
		}
		fmt.Printf("  %s joined\n", p.Config.Name)
	}

	// Create a listener agent to receive moderator audio
	listenerOpts := agent.Options{
		APIKey:        apiKey,
		APISecret:     apiSecret,
		ServerURL:     serverURL,
		Identity:      "panel-listener",
		Name:          "Panel Listener",
		AutoSubscribe: true,
		Audio: agent.AudioConfig{
			SampleRate: 48000,
			Channels:   1,
		},
	}
	listener, err := agent.New(listenerOpts)
	if err != nil {
		log.Fatalf("Failed to create listener agent: %v", err)
	}

	// Set up event handler for when moderator joins
	moderatorJoined := make(chan struct{}, 1)
	listener.OnParticipantJoined(func(p participant.Participant) {
		if p.Identity == "moderator" {
			fmt.Printf("\n[+] Moderator (%s) joined!\n", p.Name)
			select {
			case moderatorJoined <- struct{}{}:
			default:
			}
		}
	})

	listener.OnParticipantLeft(func(p participant.Participant) {
		if p.Identity == "moderator" {
			fmt.Println("\n[-] Moderator left")
		}
	})

	// Join listener to room
	if err := listener.Join(ctx, roomName); err != nil {
		log.Fatalf("Failed to join room as listener: %v", err)
	}

	// Subscribe to audio
	audioCh, err := listener.SubscribeToAllAudio(ctx)
	if err != nil {
		log.Fatalf("Failed to subscribe to audio: %v", err)
	}

	fmt.Println("\nWaiting for moderator to join... (Ctrl+C to exit)")

	// Wait for moderator
	select {
	case <-ctx.Done():
		cleanup(ctx, panelists, listener, roomClient, roomName)
		return
	case <-moderatorJoined:
	}

	// Give a moment for audio track to be ready
	time.Sleep(2 * time.Second)

	// Run introductions
	fmt.Println("\nStarting panel introductions...")
	if err := coord.RunIntroductions(ctx); err != nil {
		log.Printf("Error during introductions: %v", err)
	}

	// Process moderator audio
	go processModeratorAudio(ctx, audioCh, sttProv, coord)

	fmt.Println("\nPanel is ready! Ask questions to start the discussion.")
	fmt.Println("(Speaking detected from moderator will trigger panelist responses)")
	fmt.Println()

	// Wait for shutdown
	<-ctx.Done()

	cleanup(ctx, panelists, listener, roomClient, roomName)
	fmt.Println("Goodbye!")
}

func cleanup(_ context.Context, panelists []*Panelist, listener *agent.Agent, roomClient *room.Client, roomName string) {
	cleanupCtx := context.Background()

	for _, p := range panelists {
		if err := p.Agent.Leave(cleanupCtx); err != nil {
			log.Printf("Error leaving room for %s: %v", p.Config.Name, err)
		}
	}
	if err := listener.Leave(cleanupCtx); err != nil {
		log.Printf("Error leaving room for listener: %v", err)
	}
	if err := roomClient.DeleteRoom(cleanupCtx, roomName); err != nil {
		log.Printf("Error deleting room: %v", err)
	}
}

func processModeratorAudio(ctx context.Context, audioCh <-chan agent.AudioFrame, sttProv stt.Provider, coord *Coordinator) {
	var audioBuffer []byte
	var lastAudioTime time.Time
	silenceThreshold := 800 * time.Millisecond // Slightly longer for panel discussions

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
			audioBuffer = append(audioBuffer, frame.Data...)
			lastAudioTime = time.Now()

		case <-ticker.C:
			// Check for silence (end of speech)
			if len(audioBuffer) > 0 && time.Since(lastAudioTime) > silenceThreshold {
				audioCopy := make([]byte, len(audioBuffer))
				copy(audioCopy, audioBuffer)
				audioBuffer = nil

				// Process in background
				go func(audio []byte) {
					// Minimum audio length check
					if len(audio) < 9600 { // Less than 200ms at 48kHz
						return
					}

					// Transcribe
					text, err := transcribe(ctx, sttProv, audio)
					if err != nil {
						log.Printf("STT error: %v", err)
						return
					}
					if strings.TrimSpace(text) == "" {
						return
					}

					// Trigger panel response
					if err := coord.OnModeratorSpeech(ctx, text); err != nil {
						log.Printf("Error processing moderator speech: %v", err)
					}
				}(audioCopy)
			}
		}
	}
}

func transcribe(ctx context.Context, sttProv stt.Provider, audio []byte) (string, error) {
	wavData := pcmToWav(audio, 48000, 1)

	result, err := sttProv.Transcribe(ctx, wavData, stt.TranscriptionConfig{
		Language:   "en",
		SampleRate: 48000,
		Encoding:   "linear16",
	})
	if err != nil {
		return "", err
	}

	return result.Text, nil
}

func buildPanelistConfigs(size int) []PanelistConfig {
	defaults := DefaultPanelists()
	configs := make([]PanelistConfig, size)

	for i := 0; i < size; i++ {
		// Check for environment variable overrides
		nameKey := fmt.Sprintf("PANELIST_%d_NAME", i+1)
		voiceKey := fmt.Sprintf("PANELIST_%d_VOICE", i+1)
		personalityKey := fmt.Sprintf("PANELIST_%d_PERSONALITY", i+1)

		if name := os.Getenv(nameKey); name != "" {
			configs[i] = PanelistConfig{
				Name:        name,
				Voice:       getEnvOrDefault(voiceKey, defaults[i].Voice),
				Personality: getEnvOrDefault(personalityKey, defaults[i].Personality),
			}
		} else {
			configs[i] = defaults[i]
		}
	}

	return configs
}

// Helper functions

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
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
	w := func(data any) {
		if err := binary.Write(&buf, binary.LittleEndian, data); err != nil {
			panic(fmt.Sprintf("binary.Write failed: %v", err))
		}
	}

	// WAV header
	buf.WriteString("RIFF")
	w(uint32(36 + len(pcm))) //nolint:gosec // G115: WAV size fits in uint32
	buf.WriteString("WAVE")

	// fmt chunk
	buf.WriteString("fmt ")
	w(uint32(16))                        // chunk size
	w(uint16(1))                         // audio format (PCM)
	w(uint16(channels))                  //nolint:gosec // G115: channels is 1 or 2
	w(uint32(sampleRate))                //nolint:gosec // G115: sampleRate is 8000-48000
	w(uint32(sampleRate * channels * 2)) //nolint:gosec // G115: byte rate is bounded
	w(uint16(channels * 2))              //nolint:gosec // G115: block align is 2 or 4
	w(uint16(16))                        // bits per sample

	// data chunk
	buf.WriteString("data")
	w(uint32(len(pcm))) //nolint:gosec // G115: WAV size fits in uint32
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
		samples[i] = int16(binary.LittleEndian.Uint16(audio[i*2:])) //nolint:gosec // G115: PCM16 uses uint16->int16 bit-cast
	}

	// 2x upsample with linear interpolation
	resampled := make([]int16, numSamples*2)
	for i := 0; i < numSamples-1; i++ {
		resampled[i*2] = samples[i]
		resampled[i*2+1] = int16((int32(samples[i]) + int32(samples[i+1])) / 2) //nolint:gosec // G115: average of two int16 fits in int16
	}
	resampled[(numSamples-1)*2] = samples[numSamples-1]
	resampled[(numSamples-1)*2+1] = samples[numSamples-1]

	// Convert back to bytes
	result := make([]byte, len(resampled)*2)
	for i, s := range resampled {
		binary.LittleEndian.PutUint16(result[i*2:], uint16(s)) //nolint:gosec // G115: PCM16 uses int16->uint16 bit-cast
	}
	return result
}
