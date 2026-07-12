// Package heygen provides HeyGen LiveAvatar integration for omni-livekit.
//
// LiveAvatar is HeyGen's real-time avatar streaming product that renders
// AI avatars with lip-sync in response to audio input. This package
// implements the avatar.Session interface for LiveAvatar's LITE mode.
//
// # LITE Mode
//
// LITE mode is designed for "bring your own AI stack" scenarios where:
//   - Your agent handles STT, LLM, and TTS
//   - LiveAvatar receives TTS audio and renders the avatar
//   - The avatar joins as a LiveKit participant and publishes video
//
// # Usage
//
// Import this package to register the HeyGen provider:
//
//	import _ "github.com/plexusone/omni-livekit/avatar/heygen"
//
// Then create a session:
//
//	session, err := avatar.NewSession(avatar.Config{
//	    Provider: avatar.ProviderHeyGen,
//	    HeyGen: avatar.HeyGenConfig{
//	        APIKey:   os.Getenv("LIVEAVATAR_API_KEY"),
//	        AvatarID: "65f9e3c9-d48b-4118-b73a-4ae2e3cbb8f0", // Sandbox avatar
//	        Sandbox:  true,
//	    },
//	})
//
// # API Key
//
// LiveAvatar uses a separate API key from HeyGen video generation.
// Get your key from: https://app.liveavatar.com/developers
//
// Set the LIVEAVATAR_API_KEY environment variable or pass it in config.
package heygen
