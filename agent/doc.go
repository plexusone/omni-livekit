// Package agent provides a full-featured LiveKit agent implementation.
//
// This package contains LiveKit-specific agent functionality that goes beyond
// the generic omnimeet-core interfaces. Use this package when you need:
//
//   - Video publishing (static images, webcam simulation)
//   - Advanced media mode selection (audio-only, audio+image, audio+video)
//   - LiveKit-specific configuration options
//   - Direct access to LiveKit SDK features
//
// For generic meeting participation that works across providers, use the
// omnimeet package which implements the omnimeet-core interfaces.
//
// # Media Modes
//
// The agent supports three media modes:
//
//   - AudioOnly: Agent publishes only audio (default)
//   - AudioWithImage: Agent publishes audio + static image as video
//   - AudioWithVideo: Agent publishes audio + video frames
//
// # Usage
//
//	agent, err := agent.New(agent.Options{
//	    Config: omnimeet.Config{
//	        APIKey:    "...",
//	        APISecret: "...",
//	        ServerURL: "wss://...",
//	    },
//	    MediaMode:   agent.AudioWithImage,
//	    Name:        "AI Assistant",
//	    ImagePath:   "/path/to/avatar.png",
//	    AudioConfig: agent.AudioConfig{
//	        SampleRate: 48000,
//	        Channels:   1,
//	    },
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Join a meeting
//	err = agent.Join(ctx, "room-name")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer agent.Leave(ctx)
//
//	// Start publishing audio
//	writer, err := agent.StartAudio(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer agent.StopAudio(ctx)
//
//	// Write PCM16 audio frames
//	writer.Write(pcmData)
package agent
