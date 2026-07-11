package avatar

import (
	"context"
	"fmt"
	"time"

	lksdk "github.com/livekit/server-sdk-go/v2"
)

// SetupConfig contains all configuration for avatar setup.
// This is the unified configuration that commands should use.
type SetupConfig struct {
	// Provider selects the avatar type:
	//   - "" or "none": Audio-only, no avatar
	//   - "static": Static image (configure StaticImage field)
	//   - "tavus": Live Tavus avatar (configure Tavus field)
	//   - "anam", "simli": Other live avatar providers
	Provider string

	// StaticImage configures a static image avatar.
	// Used when Provider is "static".
	StaticImage StaticImageConfig

	// Tavus configures a Tavus live avatar.
	// Used when Provider is "tavus".
	Tavus TavusConfig

	// Anam configures an Anam live avatar.
	// Used when Provider is "anam".
	Anam AnamConfig

	// Simli configures a Simli live avatar.
	// Used when Provider is "simli".
	Simli SimliConfig

	// LiveKit connection settings (required for live avatars)
	LiveKitURL       string
	LiveKitAPIKey    string
	LiveKitAPISecret string
}

// StaticImageConfig configures a static image avatar.
type StaticImageConfig struct {
	// H264Path is the path to a pre-encoded H.264 keyframe file.
	// This is the recommended approach - no CGO required at runtime.
	H264Path string

	// H264Data is pre-encoded H.264 keyframe data (alternative to H264Path).
	H264Data []byte

	// UseDefault uses the embedded default OmniAgent avatar.
	// If true, H264Path and H264Data are ignored.
	UseDefault bool
}

// SetupResult contains the result of avatar setup.
type SetupResult struct {
	// Mode indicates the avatar mode to use.
	Mode SetupMode

	// Session is the live avatar session (nil for static or none).
	// The caller is responsible for calling Session.Close() when done.
	Session Session

	// StaticImage contains static image configuration for the agent.
	// Only populated when Mode is SetupModeStatic.
	StaticImage *StaticImageResult
}

// SetupMode indicates the type of avatar setup.
type SetupMode string

const (
	// SetupModeNone means no avatar (audio-only).
	SetupModeNone SetupMode = "none"

	// SetupModeStatic means a static image avatar.
	// The caller should configure the agent's Image options.
	SetupModeStatic SetupMode = "static"

	// SetupModeLive means a live lip-sync avatar.
	// The caller should use the returned Session for audio streaming.
	SetupModeLive SetupMode = "live"
)

// StaticImageResult contains static image configuration to apply to agent options.
type StaticImageResult struct {
	// H264Path is the path to the H.264 file (if using file).
	H264Path string

	// H264Data is the H.264 data (if using embedded data).
	H264Data []byte
}

// Setup creates the appropriate avatar based on the configuration.
//
// For static avatars, it returns configuration to apply to agent options.
// For live avatars, it creates a Session that must be started and closed.
//
// Example usage:
//
//	result, err := avatar.Setup(avatar.SetupConfig{
//	    Provider: "tavus",
//	    Tavus: avatar.TavusConfig{
//	        APIKey: os.Getenv("TAVUS_API_KEY"),
//	    },
//	    LiveKitURL:       os.Getenv("LIVEKIT_URL"),
//	    LiveKitAPIKey:    os.Getenv("LIVEKIT_API_KEY"),
//	    LiveKitAPISecret: os.Getenv("LIVEKIT_API_SECRET"),
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	switch result.Mode {
//	case avatar.SetupModeNone:
//	    // Audio-only, no avatar configuration needed
//	case avatar.SetupModeStatic:
//	    // Apply static image config to agent
//	    agentOpts.MediaMode = agent.AudioWithImage
//	    agentOpts.Image.H264Path = result.StaticImage.H264Path
//	case avatar.SetupModeLive:
//	    // Use live avatar session
//	    defer result.Session.Close(ctx)
//	    err := result.Session.Start(ctx, avatar.StartOptions{...})
//	}
func Setup(cfg SetupConfig) (*SetupResult, error) {
	// Normalize provider name
	provider := cfg.Provider
	if provider == "none" {
		provider = ""
	}

	// No avatar - audio only
	if provider == "" {
		return &SetupResult{Mode: SetupModeNone}, nil
	}

	// Static image avatar
	if provider == ProviderStatic {
		return setupStaticAvatar(cfg)
	}

	// Live avatar - use the factory
	session, err := NewSession(Config{
		Provider: provider,
		Tavus:    cfg.Tavus,
		Anam:     cfg.Anam,
		Simli:    cfg.Simli,
	})
	if err != nil {
		return nil, fmt.Errorf("create avatar session: %w", err)
	}

	return &SetupResult{
		Mode:    SetupModeLive,
		Session: session,
	}, nil
}

// setupStaticAvatar configures a static image avatar.
func setupStaticAvatar(cfg SetupConfig) (*SetupResult, error) {
	result := &SetupResult{
		Mode:        SetupModeStatic,
		StaticImage: &StaticImageResult{},
	}

	if cfg.StaticImage.UseDefault {
		// Use embedded default avatar (handled by agent package)
		// Return empty config - agent will use its embedded default
		return result, nil
	}

	if cfg.StaticImage.H264Path != "" {
		result.StaticImage.H264Path = cfg.StaticImage.H264Path
		return result, nil
	}

	if len(cfg.StaticImage.H264Data) > 0 {
		result.StaticImage.H264Data = cfg.StaticImage.H264Data
		return result, nil
	}

	// No specific config - use default
	return result, nil
}

// LiveAvatarHelper manages a live avatar session lifecycle.
// This is a convenience wrapper for common live avatar operations.
type LiveAvatarHelper struct {
	session Session
	config  SetupConfig
	started bool
}

// NewLiveAvatarHelper creates a helper for managing a live avatar session.
func NewLiveAvatarHelper(cfg SetupConfig) (*LiveAvatarHelper, error) {
	result, err := Setup(cfg)
	if err != nil {
		return nil, err
	}

	if result.Mode != SetupModeLive {
		return nil, fmt.Errorf("configuration does not create a live avatar (mode: %s)", result.Mode)
	}

	return &LiveAvatarHelper{
		session: result.Session,
		config:  cfg,
	}, nil
}

// Start initializes the live avatar session.
// Call this after the agent has joined the room.
func (h *LiveAvatarHelper) Start(ctx context.Context, room *lksdk.Room, agentIdentity string) error {
	if h.started {
		return ErrSessionAlreadyStarted
	}

	err := h.session.Start(ctx, StartOptions{
		Room:             room,
		AgentIdentity:    agentIdentity,
		LiveKitURL:       h.config.LiveKitURL,
		LiveKitAPIKey:    h.config.LiveKitAPIKey,
		LiveKitAPISecret: h.config.LiveKitAPISecret,
	})
	if err != nil {
		return err
	}

	h.started = true
	return nil
}

// WaitForJoin waits for the avatar to join the room.
func (h *LiveAvatarHelper) WaitForJoin(ctx context.Context, timeout time.Duration) error {
	if !h.started {
		return ErrSessionNotStarted
	}
	return h.session.WaitForJoin(ctx, timeout)
}

// AudioOutput returns the audio destination for streaming TTS audio.
func (h *LiveAvatarHelper) AudioOutput() AudioDestination {
	return h.session.AudioOutput()
}

// Session returns the underlying avatar session.
func (h *LiveAvatarHelper) Session() Session {
	return h.session
}

// Close cleans up the avatar session.
func (h *LiveAvatarHelper) Close(ctx context.Context) error {
	if h.session != nil {
		return h.session.Close(ctx)
	}
	return nil
}
