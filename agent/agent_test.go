package agent

import (
	"testing"
	"time"
)

func TestOptionsDefaults(t *testing.T) {
	opts := Options{}
	opts.applyDefaults()

	if opts.MediaMode != AudioOnly {
		t.Errorf("MediaMode = %q, want %q", opts.MediaMode, AudioOnly)
	}

	if opts.Audio.SampleRate != 48000 {
		t.Errorf("Audio.SampleRate = %d, want 48000", opts.Audio.SampleRate)
	}

	if opts.Audio.Channels != 1 {
		t.Errorf("Audio.Channels = %d, want 1", opts.Audio.Channels)
	}

	if opts.Audio.FrameDuration != 20*time.Millisecond {
		t.Errorf("Audio.FrameDuration = %v, want 20ms", opts.Audio.FrameDuration)
	}

	if opts.Video.Width != 640 {
		t.Errorf("Video.Width = %d, want 640", opts.Video.Width)
	}

	if opts.Video.Height != 480 {
		t.Errorf("Video.Height = %d, want 480", opts.Video.Height)
	}

	if opts.Video.FrameRate != 30 {
		t.Errorf("Video.FrameRate = %d, want 30", opts.Video.FrameRate)
	}

	if opts.Video.Codec != "vp8" {
		t.Errorf("Video.Codec = %q, want vp8", opts.Video.Codec)
	}

	if opts.Image.FrameRate != 1 {
		t.Errorf("Image.FrameRate = %d, want 1", opts.Image.FrameRate)
	}
}

func TestOptionsCustomValues(t *testing.T) {
	opts := Options{
		MediaMode: AudioWithVideo,
		Audio: AudioConfig{
			SampleRate:    16000,
			Channels:      2,
			FrameDuration: 10 * time.Millisecond,
		},
		Video: VideoConfig{
			Width:     1920,
			Height:    1080,
			FrameRate: 60,
			Codec:     "h264",
		},
	}
	opts.applyDefaults()

	// Custom values should be preserved
	if opts.MediaMode != AudioWithVideo {
		t.Errorf("MediaMode = %q, want %q", opts.MediaMode, AudioWithVideo)
	}

	if opts.Audio.SampleRate != 16000 {
		t.Errorf("Audio.SampleRate = %d, want 16000", opts.Audio.SampleRate)
	}

	if opts.Audio.Channels != 2 {
		t.Errorf("Audio.Channels = %d, want 2", opts.Audio.Channels)
	}

	if opts.Video.Width != 1920 {
		t.Errorf("Video.Width = %d, want 1920", opts.Video.Width)
	}

	if opts.Video.Codec != "h264" {
		t.Errorf("Video.Codec = %q, want h264", opts.Video.Codec)
	}
}

func TestNewAgentValidation(t *testing.T) {
	// Missing APIKey
	_, err := New(Options{
		APISecret: "secret",
		ServerURL: "wss://example.com",
	})
	if err == nil {
		t.Error("expected error for missing APIKey")
	}

	// Missing APISecret
	_, err = New(Options{
		APIKey:    "key",
		ServerURL: "wss://example.com",
	})
	if err == nil {
		t.Error("expected error for missing APISecret")
	}

	// Missing ServerURL
	_, err = New(Options{
		APIKey:    "key",
		APISecret: "secret",
	})
	if err == nil {
		t.Error("expected error for missing ServerURL")
	}

	// Valid options
	agent, err := New(Options{
		APIKey:    "key",
		APISecret: "secret",
		ServerURL: "wss://example.com",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if agent.State() != StateDisconnected {
		t.Errorf("State() = %q, want %q", agent.State(), StateDisconnected)
	}
}

func TestAgentIdentityGeneration(t *testing.T) {
	agent, err := New(Options{
		APIKey:    "key",
		APISecret: "secret",
		ServerURL: "wss://example.com",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Identity should be auto-generated
	if agent.opts.Identity == "" {
		t.Error("Identity should be auto-generated")
	}

	// Custom identity should be preserved
	agent2, err := New(Options{
		APIKey:    "key",
		APISecret: "secret",
		ServerURL: "wss://example.com",
		Identity:  "custom-id",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if agent2.opts.Identity != "custom-id" {
		t.Errorf("Identity = %q, want custom-id", agent2.opts.Identity)
	}
}

func TestMediaModes(t *testing.T) {
	if AudioOnly != "audio_only" {
		t.Errorf("AudioOnly = %q, want audio_only", AudioOnly)
	}

	if AudioWithImage != "audio_with_image" {
		t.Errorf("AudioWithImage = %q, want audio_with_image", AudioWithImage)
	}

	if AudioWithVideo != "audio_with_video" {
		t.Errorf("AudioWithVideo = %q, want audio_with_video", AudioWithVideo)
	}
}

func TestConnectionStates(t *testing.T) {
	if StateDisconnected != "disconnected" {
		t.Errorf("StateDisconnected = %q, want disconnected", StateDisconnected)
	}

	if StateConnecting != "connecting" {
		t.Errorf("StateConnecting = %q, want connecting", StateConnecting)
	}

	if StateConnected != "connected" {
		t.Errorf("StateConnected = %q, want connected", StateConnected)
	}

	if StateReconnecting != "reconnecting" {
		t.Errorf("StateReconnecting = %q, want reconnecting", StateReconnecting)
	}

	if StateFailed != "failed" {
		t.Errorf("StateFailed = %q, want failed", StateFailed)
	}
}
