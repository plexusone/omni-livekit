package heygen

import (
	"github.com/plexusone/heygen-go/liveavatar"
	"github.com/plexusone/omni-livekit/avatar"
)

func init() {
	avatar.RegisterProvider(avatar.ProviderHeyGen, func(cfg avatar.Config) (avatar.Session, error) {
		return NewSession(SessionConfig{
			APIKey:       cfg.HeyGen.APIKey,
			BaseURL:      cfg.HeyGen.BaseURL,
			AvatarID:     cfg.HeyGen.AvatarID,
			Sandbox:      cfg.HeyGen.Sandbox,
			VideoQuality: liveavatar.VideoQuality(cfg.HeyGen.VideoQuality),
			AudioConfig:  cfg.HeyGen.AudioConfig,
		})
	})
}
