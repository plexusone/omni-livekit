package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/plexusone/omni-livekit/agent"
	"github.com/plexusone/omni-livekit/avatar"
	"github.com/plexusone/omnivoice-core/tts"
)

// PanelistConfig holds configuration for a single panelist.
type PanelistConfig struct {
	Name        string // Display name (e.g., "Alex")
	Voice       string // TTS voice ID (e.g., "alloy", "echo")
	Personality string // Short personality description
}

// DefaultPanelists returns the predefined panelist configurations.
func DefaultPanelists() []PanelistConfig {
	return []PanelistConfig{
		{
			Name:        "Alex",
			Voice:       "alloy",
			Personality: "An optimistic tech enthusiast who cites benefits and opportunities. Always looks for the positive angle while remaining grounded.",
		},
		{
			Name:        "Jordan",
			Voice:       "echo",
			Personality: "A pragmatic skeptic who asks hard questions and challenges assumptions. Values evidence over hype.",
		},
		{
			Name:        "Morgan",
			Voice:       "onyx",
			Personality: "An academic expert who provides depth and context. Cites research and offers historical perspective.",
		},
		{
			Name:        "Casey",
			Voice:       "nova",
			Personality: "A creative thinker who offers novel perspectives. Makes unexpected connections and thinks outside the box.",
		},
	}
}

// Panelist represents an AI panelist in the discussion.
type Panelist struct {
	Config       PanelistConfig
	Agent        *agent.Agent
	AudioWriter  agent.AudioWriter
	ttsProvider  tts.Provider
	anthropicKey string
	sampleRate   int

	// Avatar support (optional)
	avatarSession avatar.Session
	avatarConfig  *avatar.Config
	lkConfig      *LiveKitConfig // LiveKit credentials for avatar
}

// LiveKitConfig holds LiveKit credentials for avatar sessions.
type LiveKitConfig struct {
	URL       string
	APIKey    string
	APISecret string
}

// NewPanelist creates a new panelist with the given configuration.
func NewPanelist(cfg PanelistConfig, ag *agent.Agent, ttsProv tts.Provider, anthropicKey string) *Panelist {
	return &Panelist{
		Config:       cfg,
		Agent:        ag,
		ttsProvider:  ttsProv,
		anthropicKey: anthropicKey,
		sampleRate:   48000,
	}
}

// SetAvatarConfig configures the panelist to use an avatar.
// Call this before StartAudio to enable avatar mode.
func (p *Panelist) SetAvatarConfig(cfg *avatar.Config, lkCfg *LiveKitConfig) {
	p.avatarConfig = cfg
	p.lkConfig = lkCfg
}

// GenerateResponse generates a response from this panelist given the transcript and topic.
func (p *Panelist) GenerateResponse(ctx context.Context, topic, transcript, question string) (string, error) {
	systemPrompt := fmt.Sprintf(`You are %s, a panelist in a discussion about "%s".

Your personality: %s

Guidelines:
- Keep responses to 2-4 sentences (panel format, not lectures)
- Build on what other panelists said - agree, disagree, extend, or offer a different angle
- Don't repeat points already made verbatim; add new perspective
- Address the moderator or reference other panelists by name naturally
- Stay in character with your personality throughout
- Speak conversationally as this is a voice discussion
- Do NOT use markdown formatting, asterisks, or special characters

Current discussion transcript:
%s

The moderator just said: "%s"

Respond as %s:`, p.Config.Name, topic, p.Config.Personality, transcript, question, p.Config.Name)

	payload := map[string]any{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": 200,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": question},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.anthropic.com/v1/messages",
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", p.anthropicKey)
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

	return result.Content[0].Text, nil
}

// Speak converts text to speech and sends it via the panelist's audio track or avatar.
func (p *Panelist) Speak(ctx context.Context, text string) error {
	// Synthesize speech using TTS provider with panelist's voice
	result, err := p.ttsProvider.Synthesize(ctx, text, tts.SynthesisConfig{
		VoiceID:      p.Config.Voice,
		SampleRate:   24000, // Most providers output 24kHz
		OutputFormat: "pcm",
	})
	if err != nil {
		return fmt.Errorf("TTS synthesis: %w", err)
	}

	// If using avatar, send audio to avatar session
	if p.avatarSession != nil {
		audioOut := p.avatarSession.AudioOutput()
		if audioOut == nil {
			return fmt.Errorf("avatar audio output not initialized")
		}

		// Send audio in frames (20ms at 24kHz = 480 samples = 960 bytes)
		frameSize := 960
		for i := 0; i < len(result.Audio); i += frameSize {
			end := i + frameSize
			if end > len(result.Audio) {
				end = len(result.Audio)
			}
			frame := result.Audio[i:end]

			if err := audioOut.CaptureFrame(ctx, frame); err != nil {
				return err
			}
		}

		// Flush to signal end of utterance
		return audioOut.Flush(ctx)
	}

	// Fallback to direct audio output
	if p.AudioWriter == nil {
		return fmt.Errorf("audio writer not initialized")
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

		if _, err := p.AudioWriter.Write(frame); err != nil {
			return err
		}

		// Pace the audio output
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Continue writing
		}
	}

	return nil
}

// StartAudio initializes the panelist's audio track or avatar session.
func (p *Panelist) StartAudio(ctx context.Context) error {
	// If avatar is configured, start avatar session
	if p.avatarConfig != nil && p.lkConfig != nil {
		session, err := avatar.NewSession(*p.avatarConfig)
		if err != nil {
			return fmt.Errorf("failed to create avatar session: %w", err)
		}

		// Start the avatar session
		room := p.Agent.Room()
		if room == nil {
			return fmt.Errorf("agent not joined to room")
		}

		err = session.Start(ctx, avatar.StartOptions{
			Room:             room,
			AgentIdentity:    p.Agent.LocalParticipant().Identity,
			LiveKitURL:       p.lkConfig.URL,
			LiveKitAPIKey:    p.lkConfig.APIKey,
			LiveKitAPISecret: p.lkConfig.APISecret,
		})
		if err != nil {
			return fmt.Errorf("failed to start avatar session: %w", err)
		}

		// Wait for avatar to join
		if err := session.WaitForJoin(ctx, 30*time.Second); err != nil {
			_ = session.Close(ctx)
			return fmt.Errorf("avatar failed to join: %w", err)
		}

		p.avatarSession = session
		return nil
	}

	// Fallback to direct audio output
	writer, err := p.Agent.StartAudio(ctx)
	if err != nil {
		return err
	}
	p.AudioWriter = writer
	return nil
}

// Close cleans up the panelist's resources.
func (p *Panelist) Close(ctx context.Context) error {
	if p.avatarSession != nil {
		return p.avatarSession.Close(ctx)
	}
	return nil
}

// HasAvatar returns true if the panelist has an active avatar session.
func (p *Panelist) HasAvatar() bool {
	return p.avatarSession != nil
}
