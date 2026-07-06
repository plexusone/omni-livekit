// Package tavus provides Tavus avatar integration for omni-livekit voice agents.
//
// Tavus is a conversational video AI platform that provides lip-sync avatars
// that can join LiveKit rooms and speak with real-time lip synchronization.
//
// # Quick Start
//
// Create a Tavus avatar session:
//
//	session, err := tavus.NewSession(tavus.SessionConfig{
//		APIKey: os.Getenv("TAVUS_API_KEY"),
//		PalID:  "pb87e71797da", // Default stock avatar
//	})
//	if err != nil {
//		return err
//	}
//
//	err = session.Start(ctx, avatar.StartOptions{
//		Room:             room,
//		AgentIdentity:    "agent-123",
//		LiveKitURL:       "wss://your-livekit-server.com",
//		LiveKitAPIKey:    os.Getenv("LIVEKIT_API_KEY"),
//		LiveKitAPISecret: os.Getenv("LIVEKIT_API_SECRET"),
//	})
//	if err != nil {
//		return err
//	}
//
//	// Wait for avatar to join
//	err = session.WaitForJoin(ctx, 30*time.Second)
//	if err != nil {
//		return err
//	}
//
//	// Stream TTS audio to avatar
//	audioOut := session.AudioOutput()
//	audioOut.CaptureFrame(ctx, pcmFrame)
//	audioOut.Flush(ctx)
//
//	// Clean up
//	session.Close(ctx)
//
// # Configuration
//
// Required environment variables:
//   - TAVUS_API_KEY: Your Tavus API key
//
// Optional configuration:
//   - PalID: The PAL (Personalized AI Likeness) ID to use
//   - FaceID: Override the PAL's default face
//   - APIURL: Custom API URL (default: https://tavusapi.com/v2)
//
// # PALs and Faces
//
// Tavus uses PALs (Personalized AI Likenesses) to define avatar personalities.
// Each PAL has a default face, but you can override it with a different face_id.
//
// The default stock PAL is "pb87e71797da".
//
// # Audio Format
//
// Tavus expects audio at 24kHz sample rate, mono, PCM16 format.
// The DataStreamAudioOutput automatically handles streaming to the avatar.
package tavus
