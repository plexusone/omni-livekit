// Simple audio test - generates a tone to verify audio output works
package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/plexusone/omni-livekit/agent"
	"github.com/plexusone/omni-livekit/room"
)

func main() {
	serverURL := os.Getenv("LIVEKIT_URL")
	apiKey := os.Getenv("LIVEKIT_API_KEY")
	apiSecret := os.Getenv("LIVEKIT_API_SECRET")

	if serverURL == "" || apiKey == "" || apiSecret == "" {
		log.Fatal("Required: LIVEKIT_URL, LIVEKIT_API_KEY, LIVEKIT_API_SECRET")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Create room
	roomClient, err := room.NewClient(room.Config{
		APIKey:    apiKey,
		APISecret: apiSecret,
		URL:       serverURL,
	})
	if err != nil {
		log.Fatalf("Failed to create room client: %v", err)
	}

	roomName := fmt.Sprintf("audio-test-%d", time.Now().Unix())
	fmt.Printf("Creating room: %s\n", roomName)
	if _, err := roomClient.CreateRoom(ctx, roomName); err != nil {
		log.Fatalf("Failed to create room: %v", err)
	}

	// Generate token for human
	humanToken, err := roomClient.GenerateClientToken(roomName, "human", "Human")
	if err != nil {
		log.Fatalf("Failed to generate token: %v", err)
	}

	meetURL := buildMeetURL(serverURL, humanToken)
	fmt.Printf("\nJoin: %s\n\n", meetURL)

	// Create agent
	ag, err := agent.New(agent.Options{
		APIKey:    apiKey,
		APISecret: apiSecret,
		ServerURL: serverURL,
		Identity:  "audio-test",
		Name:      "Audio Test",
		Audio: agent.AudioConfig{
			SampleRate: 48000,
			Channels:   1,
			TrackName:  "test-audio",
		},
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Join room
	if err := ag.Join(ctx, roomName); err != nil {
		log.Fatalf("Failed to join: %v", err)
	}
	fmt.Println("Agent joined room")

	// Start audio
	audioWriter, err := ag.StartAudio(ctx)
	if err != nil {
		log.Fatalf("Failed to start audio: %v", err)
	}
	fmt.Println("Audio started")

	// Wait for human to join
	fmt.Println("Waiting for you to join... (open the URL above)")
	time.Sleep(5 * time.Second)

	// Generate and play a 440Hz tone for 2 seconds
	fmt.Println("Playing 440Hz tone for 2 seconds...")
	playTone(audioWriter, 440, 2*time.Second, 48000)
	fmt.Println("Tone finished")

	// Wait a bit then play another tone
	time.Sleep(1 * time.Second)
	fmt.Println("Playing 880Hz tone for 1 second...")
	playTone(audioWriter, 880, 1*time.Second, 48000)
	fmt.Println("Tone finished")

	fmt.Println("\nDid you hear the tones? Press Ctrl+C to exit...")
	<-ctx.Done()

	ag.Leave(ctx)
	roomClient.DeleteRoom(context.Background(), roomName)
}

func playTone(w agent.AudioWriter, freq float64, duration time.Duration, sampleRate int) {
	// Generate 20ms frames
	samplesPerFrame := sampleRate / 50 // 960 samples for 20ms at 48kHz
	bytesPerFrame := samplesPerFrame * 2
	totalFrames := int(duration.Seconds() * 50)
	frameDuration := 20 * time.Millisecond

	// Track timing to maintain consistent pacing
	startTime := time.Now()

	for frame := 0; frame < totalFrames; frame++ {
		pcm := make([]byte, bytesPerFrame)
		for i := 0; i < samplesPerFrame; i++ {
			sampleIndex := frame*samplesPerFrame + i
			t := float64(sampleIndex) / float64(sampleRate)
			sample := int16(math.Sin(2*math.Pi*freq*t) * 16000)
			binary.LittleEndian.PutUint16(pcm[i*2:], uint16(sample))
		}

		if _, err := w.Write(pcm); err != nil {
			log.Printf("Write error at frame %d: %v", frame, err)
			return
		}

		// Calculate when the next frame should be written
		nextFrameTime := startTime.Add(time.Duration(frame+1) * frameDuration)
		sleepDuration := time.Until(nextFrameTime)
		if sleepDuration > 0 {
			time.Sleep(sleepDuration)
		}
	}
}

func buildMeetURL(serverURL, token string) string {
	u, _ := url.Parse("https://meet.livekit.io/custom")
	q := u.Query()
	q.Set("liveKitUrl", serverURL)
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String()
}
