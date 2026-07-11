package avatar

import (
	"fmt"
	"sync"
)

// Provider constants for avatar providers.
const (
	ProviderNone     = ""         // No avatar (audio-only mode)
	ProviderTavus    = "tavus"    // Tavus conversational video
	ProviderBitHuman = "bithuman" // bitHuman real-time avatars
	ProviderAnam     = "anam"     // Anam avatar (future)
	ProviderSimli    = "simli"    // Simli avatar (future)
	ProviderStatic   = "static"   // Static image (not a Session, handled separately)
)

// Config is the unified configuration for creating avatar sessions.
// The Provider field determines which provider-specific config is used.
type Config struct {
	// Provider selects the avatar provider.
	// Supported values: "tavus", "bithuman", "anam", "simli", "" (none/audio-only)
	// Note: "static" is not a Session provider; handle static images separately.
	Provider string

	// Tavus contains Tavus-specific configuration.
	// Used when Provider is "tavus".
	Tavus TavusConfig

	// BitHuman contains bitHuman-specific configuration.
	// Used when Provider is "bithuman".
	BitHuman BitHumanConfig

	// Anam contains Anam-specific configuration.
	// Used when Provider is "anam".
	Anam AnamConfig

	// Simli contains Simli-specific configuration.
	// Used when Provider is "simli".
	Simli SimliConfig
}

// TavusConfig contains configuration for the Tavus avatar provider.
type TavusConfig struct {
	// APIKey is the Tavus API key.
	// Required.
	APIKey string

	// BaseURL is the Tavus API base URL.
	// Default: https://tavusapi.com
	BaseURL string

	// PalID is the PAL (Personalized AI Likeness) to use.
	// Default: stock avatar (provider default)
	PalID string

	// FaceID is an optional face override.
	FaceID string

	// AudioConfig configures the audio format.
	// Default: 24kHz mono PCM16 (Tavus requirement)
	AudioConfig AudioConfig
}

// AnamConfig contains configuration for the Anam avatar provider.
// Placeholder for future implementation.
type AnamConfig struct {
	// APIKey is the Anam API key.
	APIKey string

	// PersonaID is the Anam persona to use.
	PersonaID string

	// AudioConfig configures the audio format.
	AudioConfig AudioConfig
}

// SimliConfig contains configuration for the Simli avatar provider.
// Placeholder for future implementation.
type SimliConfig struct {
	// APIKey is the Simli API key.
	APIKey string

	// AvatarID is the Simli avatar to use.
	AvatarID string

	// AudioConfig configures the audio format.
	AudioConfig AudioConfig
}

// BitHumanConfig contains configuration for the bitHuman avatar provider.
type BitHumanConfig struct {
	// APIKey is the bitHuman API key.
	// Required.
	APIKey string

	// BaseURL is the bitHuman API base URL.
	// Default: https://api.bithuman.ai
	BaseURL string

	// AgentID is the bitHuman agent to use for the session.
	// Required.
	AgentID string

	// AudioConfig configures the audio format.
	// Default: 24kHz mono PCM16
	AudioConfig AudioConfig
}

// SessionConstructor is a function that creates a Session from a Config.
type SessionConstructor func(cfg Config) (Session, error)

// registry holds registered provider constructors.
var (
	registry   = make(map[string]SessionConstructor)
	registryMu sync.RWMutex
)

// RegisterProvider registers a session constructor for a provider.
// This allows providers to be registered from their sub-packages
// without creating import cycles.
//
// Example usage in avatar/tavus/init.go:
//
//	func init() {
//	    avatar.RegisterProvider("tavus", func(cfg avatar.Config) (avatar.Session, error) {
//	        return NewSession(SessionConfig{
//	            APIKey:      cfg.Tavus.APIKey,
//	            BaseURL:     cfg.Tavus.BaseURL,
//	            PalID:       cfg.Tavus.PalID,
//	            FaceID:      cfg.Tavus.FaceID,
//	            AudioConfig: cfg.Tavus.AudioConfig,
//	        })
//	    })
//	}
func RegisterProvider(name string, constructor SessionConstructor) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = constructor
}

// NewSession creates an avatar session based on the provided configuration.
//
// Returns nil, nil if Provider is empty (audio-only mode).
// Returns an error if the provider is not registered or configuration is invalid.
//
// The provider must be registered via RegisterProvider before calling this function.
// The tavus package registers itself automatically when imported.
//
// Example:
//
//	import _ "github.com/plexusone/omni-livekit/avatar/tavus" // Register tavus provider
//
//	session, err := avatar.NewSession(avatar.Config{
//	    Provider: avatar.ProviderTavus,
//	    Tavus: avatar.TavusConfig{
//	        APIKey: os.Getenv("TAVUS_API_KEY"),
//	        PalID:  os.Getenv("TAVUS_PAL_ID"),
//	    },
//	})
func NewSession(cfg Config) (Session, error) {
	// No avatar requested - audio-only mode
	if cfg.Provider == "" || cfg.Provider == ProviderNone {
		return nil, nil
	}

	// Static images are not Session-based avatars
	if cfg.Provider == ProviderStatic {
		return nil, fmt.Errorf("static avatars are not Session-based; use agent image options instead")
	}

	registryMu.RLock()
	constructor, ok := registry[cfg.Provider]
	registryMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("avatar provider %q not registered; import the provider package", cfg.Provider)
	}

	return constructor(cfg)
}

// RegisteredProviders returns a list of registered provider names.
func RegisteredProviders() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	providers := make([]string, 0, len(registry))
	for name := range registry {
		providers = append(providers, name)
	}
	return providers
}

// IsProviderRegistered returns true if the provider is registered.
func IsProviderRegistered(name string) bool {
	registryMu.RLock()
	defer registryMu.RUnlock()
	_, ok := registry[name]
	return ok
}
