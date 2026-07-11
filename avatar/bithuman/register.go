package bithuman

import "github.com/plexusone/omni-livekit/avatar"

func init() {
	avatar.RegisterProvider(avatar.ProviderBitHuman, func(cfg avatar.Config) (avatar.Session, error) {
		return NewSession(SessionConfig{
			APIKey:      cfg.BitHuman.APIKey,
			BaseURL:     cfg.BitHuman.BaseURL,
			AgentID:     cfg.BitHuman.AgentID,
			AudioConfig: cfg.BitHuman.AudioConfig,
		})
	})
}
