// Package gateway provides a WebRTC voice gateway using LiveKit for real-time
// browser and mobile voice AI applications.
//
// Architecture:
//
//	┌───────────────┐        ┌─────────────────┐        ┌───────────────────┐
//	│  Browser/App  │◄──────►│  LiveKit Cloud  │◄──────►│   OmniVoice       │
//	│   (WebRTC)    │ WebRTC │    or Server    │ WebRTC │   Voice Gateway   │
//	└───────────────┘        └─────────────────┘        └───────────────────┘
//
// Unlike PSTN-based gateways (Twilio, Telnyx, Vonage, Plivo) that handle phone
// calls, the LiveKit gateway enables voice AI for web and mobile applications:
//
//   - Browser clients connect via LiveKit Client SDK (JavaScript, React)
//   - Mobile clients connect via LiveKit Client SDK (iOS, Android, Flutter)
//   - Go backend joins as a participant using LiveKit Server SDK
//
// Flow:
//  1. Client joins a LiveKit room
//  2. Gateway joins the same room as a participant
//  3. Client publishes audio track
//  4. Gateway receives audio via PCMRemoteTrack (Opus → PCM16)
//  5. Gateway processes with STT → LLM → TTS
//  6. Gateway sends audio via PCMLocalTrack (PCM16 → Opus)
//  7. Client receives audio and plays it
//
// Key differences from PSTN gateways:
//   - No phone numbers; uses room names and participant identities
//   - No HTTP webhooks; uses WebRTC signaling
//   - PCM16 audio at 16kHz or 24kHz (configurable)
//   - Direct integration with browser/mobile clients
//   - Lower latency than PSTN (typically <200ms vs 500ms+)
//
// Usage:
//
//	gw, err := gateway.New(gateway.Config{
//	    LiveKitURL:    "wss://your-app.livekit.cloud",
//	    LiveKitAPIKey: os.Getenv("LIVEKIT_API_KEY"),
//	    LiveKitSecret: os.Getenv("LIVEKIT_API_SECRET"),
//	    RoomName:      "voice-agent",
//	    AgentIdentity: "ai-agent",
//	    SampleRate:    24000,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	gw.OnCall(func(call *gateway.CallInfo) error {
//	    log.Printf("Participant joined: %s", call.From)
//	    return nil
//	})
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	if err := gw.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
package gateway
