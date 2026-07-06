package avatar

import (
	"testing"
)

func TestDefaultAudioConfig(t *testing.T) {
	cfg := DefaultAudioConfig()

	if cfg.SampleRate != 24000 {
		t.Errorf("expected SampleRate 24000, got %d", cfg.SampleRate)
	}
	if cfg.Channels != 1 {
		t.Errorf("expected Channels 1, got %d", cfg.Channels)
	}
	if cfg.Encoding != "linear16" {
		t.Errorf("expected Encoding linear16, got %s", cfg.Encoding)
	}
}

func TestAudioConfigFrameSize(t *testing.T) {
	tests := []struct {
		name       string
		sampleRate int
		channels   int
		durationMs int
		expected   int
	}{
		{
			name:       "24kHz mono 20ms",
			sampleRate: 24000,
			channels:   1,
			durationMs: 20,
			expected:   960, // 24000 * 20 / 1000 * 2 * 1
		},
		{
			name:       "48kHz mono 20ms",
			sampleRate: 48000,
			channels:   1,
			durationMs: 20,
			expected:   1920, // 48000 * 20 / 1000 * 2 * 1
		},
		{
			name:       "16kHz mono 20ms",
			sampleRate: 16000,
			channels:   1,
			durationMs: 20,
			expected:   640, // 16000 * 20 / 1000 * 2 * 1
		},
		{
			name:       "24kHz stereo 20ms",
			sampleRate: 24000,
			channels:   2,
			durationMs: 20,
			expected:   1920, // 24000 * 20 / 1000 * 2 * 2
		},
		{
			name:       "24kHz mono 10ms",
			sampleRate: 24000,
			channels:   1,
			durationMs: 10,
			expected:   480, // 24000 * 10 / 1000 * 2 * 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := AudioConfig{
				SampleRate: tt.sampleRate,
				Channels:   tt.channels,
				Encoding:   "linear16",
			}
			got := cfg.FrameSize(tt.durationMs)
			if got != tt.expected {
				t.Errorf("expected %d bytes, got %d", tt.expected, got)
			}
		})
	}
}

func TestAudioConfigFrameDuration(t *testing.T) {
	tests := []struct {
		name       string
		sampleRate int
		channels   int
		frameSize  int
		expected   int
	}{
		{
			name:       "24kHz mono 960 bytes",
			sampleRate: 24000,
			channels:   1,
			frameSize:  960,
			expected:   20,
		},
		{
			name:       "48kHz mono 1920 bytes",
			sampleRate: 48000,
			channels:   1,
			frameSize:  1920,
			expected:   20,
		},
		{
			name:       "16kHz mono 640 bytes",
			sampleRate: 16000,
			channels:   1,
			frameSize:  640,
			expected:   20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := AudioConfig{
				SampleRate: tt.sampleRate,
				Channels:   tt.channels,
				Encoding:   "linear16",
			}
			got := cfg.FrameDuration(tt.frameSize)
			if got != tt.expected {
				t.Errorf("expected %d ms, got %d", tt.expected, got)
			}
		})
	}
}
