// Package bithuman provides bitHuman avatar integration for omni-livekit voice agents.
//
// bitHuman is a real-time avatar animation platform that creates digital avatars
// with lip-sync to audio at 25 FPS. It supports both realistic humans and animated
// characters, and can run on CPU without requiring a GPU.
//
// # Quick Start
//
// Create a bitHuman avatar session:
//
//	session, err := bithuman.NewSession(bithuman.SessionConfig{
//		APIKey:  os.Getenv("BITHUMAN_API_KEY"),
//		AgentID: "your-agent-id",
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
//   - BITHUMAN_API_KEY: Your bitHuman API key
//
// Required configuration:
//   - AgentID: The bitHuman agent ID to use for the session
//
// Optional configuration:
//   - BaseURL: Custom API URL (default: https://api.bithuman.ai)
//
// # Agents
//
// bitHuman uses agents to define avatar appearance and behavior.
// Agents can be realistic humans or animated characters.
// Create agents via the bitHuman dashboard or API.
//
// # Audio Format
//
// bitHuman expects audio at 24kHz sample rate, mono, PCM16 format.
// The DataStreamAudioOutput automatically handles streaming to the avatar.
//
// # Features
//
//   - Realistic and animated character support
//   - CPU-only rendering (no GPU required)
//   - <100ms latency
//   - LiveKit native integration
//   - Self-hostable for privacy
package bithuman
