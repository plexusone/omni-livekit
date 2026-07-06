// Package avatar provides lip-sync avatar support for omni-livekit voice agents.
//
// This package enables voice agents to display real-time lip-synced video avatars
// that move their mouths in sync with generated speech. It supports multiple
// avatar providers (Tavus, Anam, Simli, etc.) through a pluggable architecture.
//
// # Architecture
//
// The avatar system consists of three main components:
//
//  1. Session - Manages the avatar lifecycle (start, wait for join, close)
//  2. AudioDestination - Streams TTS audio to the avatar provider
//  3. RPC Handlers - Handle playback control (start, finish, clear buffer)
//
// The avatar provider joins the LiveKit room as a separate participant and
// publishes video+audio tracks on behalf of the agent using the
// "lk.publish_on_behalf" participant attribute.
//
// # Basic Usage
//
//	// Create avatar session with a provider
//	session, err := tavus.New(tavus.Config{
//	    APIKey: os.Getenv("TAVUS_API_KEY"),
//	    FaceID: "your-face-id",
//	})
//
//	// Start the avatar
//	err = session.Start(ctx, avatar.StartOptions{
//	    Room:          agent.Room(),
//	    AgentIdentity: "meeting-pm",
//	    LiveKitURL:    os.Getenv("LIVEKIT_URL"),
//	    // ...
//	})
//
//	// Wait for avatar to join
//	err = session.WaitForJoin(ctx, 10*time.Second)
//
//	// Get audio destination for TTS output
//	audioOut := session.AudioOutput()
//
//	// Stream TTS audio to avatar
//	audioOut.CaptureFrame(ctx, pcmFrame)
//	audioOut.Flush(ctx)
//
//	// Handle interruption
//	audioOut.ClearBuffer(ctx)
//
// # Providers
//
// Avatar providers are implemented in sub-packages:
//
//   - avatar/tavus - Tavus avatar provider
//   - avatar/anam - Anam avatar provider
//   - avatar/simli - Simli avatar provider
//
// Each provider implements the Session interface and handles provider-specific
// API calls to create avatar sessions.
//
// # Audio Streaming
//
// Audio is streamed from the agent to the avatar using LiveKit's ByteStream API.
// The DataStreamAudioOutput implementation handles:
//
//   - Streaming PCM audio frames to the avatar
//   - Managing stream lifecycle (create, write, close)
//   - RPC-based playback control
//
// # Playback Control
//
// The avatar communicates playback state via RPC:
//
//   - lk.playback_started - Avatar started speaking
//   - lk.playback_finished - Avatar finished speaking (with position and interrupted flag)
//   - lk.clear_buffer - Agent requests playback interruption
package avatar
