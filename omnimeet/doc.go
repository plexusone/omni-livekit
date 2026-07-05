// Package omnimeet provides a LiveKit implementation of the OmniMeet provider interface.
//
// This package implements both MeetingProvider (control plane) and AgentParticipant
// (media plane) interfaces for LiveKit.
//
// # Usage
//
// Import this package to register the LiveKit provider:
//
//	import (
//	    "github.com/plexusone/omnimeet-core"
//	    _ "github.com/plexusone/omni-livekit/omnimeet"
//	)
//
//	func main() {
//	    provider, _ := omnimeet.GetMeetingProvider("livekit",
//	        omnimeet.WithAPIKey("your-api-key"),
//	        omnimeet.WithAPISecret("your-api-secret"),
//	        omnimeet.WithServerURL("wss://your-server.livekit.cloud"),
//	    )
//
//	    // Create a meeting
//	    meeting, _ := provider.CreateMeeting(ctx, omnimeet.CreateMeetingRequest{
//	        Name: "My Meeting",
//	    })
//
//	    // Generate join token
//	    token, _ := provider.CreateJoinToken(ctx, omnimeet.CreateJoinTokenRequest{
//	        MeetingID: meeting.ID,
//	        Participant: omnimeet.ParticipantInfo{
//	            Name: "AI Assistant",
//	            Kind: omnimeet.ParticipantKindAgent,
//	        },
//	    })
//	}
//
// # Agent Participation
//
// For AI agents that need to join meetings and process audio:
//
//	factory := provider.(omnimeet.AgentParticipantFactory)
//	agent, _ := factory.CreateAgentParticipant(omnimeet.AgentParticipantOptions{
//	    AutoSubscribe: true,
//	})
//
//	agent.JoinMeeting(ctx, meeting.ID, token)
//	defer agent.LeaveMeeting(ctx)
//
//	audioCh, _ := agent.SubscribeToAllAudio(ctx)
//	for frame := range audioCh {
//	    // Process audio
//	}
//
// # Configuration
//
// The provider can be configured with the following options:
//
//   - api_key: LiveKit API key
//   - api_secret: LiveKit API secret
//   - server_url: LiveKit server URL (e.g., "wss://your-server.livekit.cloud")
//   - webhook_secret: Secret for validating webhooks
package omnimeet
