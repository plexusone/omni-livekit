package tavus

import "github.com/plexusone/omni-livekit/avatar"

func init() {
	avatar.RegisterProvider(avatar.ProviderTavus, func(cfg avatar.Config) (avatar.Session, error) {
		return NewSession(SessionConfig{
			APIKey:      cfg.Tavus.APIKey,
			BaseURL:     cfg.Tavus.BaseURL,
			PalID:       cfg.Tavus.PalID,
			FaceID:      cfg.Tavus.FaceID,
			AudioConfig: cfg.Tavus.AudioConfig,
		})
	})
}
