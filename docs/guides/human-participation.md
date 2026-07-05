# Human Participation

When an AI agent joins a LiveKit room using omni-livekit, humans can join via several methods to interact with the agent.

## Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     LiveKit Room                            │
│                                                             │
│   ┌─────────────────┐              ┌─────────────────────┐  │
│   │   AI Agent      │◄────────────►│   Human Participant │  │
│   │ (omni-livekit)  │   WebRTC     │   (Browser/App)     │  │
│   └─────────────────┘              └─────────────────────┘  │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Option 1: LiveKit Meet (Recommended for Testing)

LiveKit provides a free hosted web UI at **https://meet.livekit.io** that requires zero frontend development.

### Generate a Join URL

```go
import (
    "fmt"
    "net/url"

    "github.com/plexusone/omni-livekit/room"
)

func generateJoinURL(serverURL, apiKey, apiSecret, roomName, userName string) (string, error) {
    client, err := room.NewClient(room.Config{
        APIKey:    apiKey,
        APISecret: apiSecret,
        URL:       serverURL,
    })
    if err != nil {
        return "", err
    }

    // Generate token for human participant
    token, err := client.GenerateClientToken(roomName, userName, userName)
    if err != nil {
        return "", err
    }

    // Build LiveKit Meet URL
    meetURL := fmt.Sprintf("https://meet.livekit.io/custom?liveKitUrl=%s&token=%s",
        url.QueryEscape(serverURL),
        url.QueryEscape(token),
    )

    return meetURL, nil
}
```

### Usage

1. Run your AI agent (joins the room)
2. Generate join URL for the human
3. Human opens URL in browser
4. Human grants microphone permission
5. Human and agent can now talk

### Demo Commands

The `cmd/agent-demo` and `cmd/voice-agent` commands automatically generate join URLs:

```bash
# Set environment variables
export LIVEKIT_URL="wss://your-project.livekit.cloud"
export LIVEKIT_API_KEY="your-api-key"
export LIVEKIT_API_SECRET="your-api-secret"

# Run the demo
go run ./cmd/agent-demo

# Output:
# Creating room: demo-1234567890
#
# ===========================================
#   LiveKit Agent Demo
# ===========================================
#
# Room: demo-1234567890
#
# Join as human participant:
#
#   https://meet.livekit.io/custom?liveKitUrl=wss%3A%2F%2F...&token=eyJ...
#
# Starting agent...
```

Share the URL with anyone who needs to join.

## Option 2: LiveKit Cloud Dashboard

If you're using [LiveKit Cloud](https://cloud.livekit.io):

1. Log in to your LiveKit Cloud dashboard
2. Navigate to your project
3. Go to "Rooms" section
4. Create or select a room
5. Use the "Join" button to generate a participant link

## Option 3: Custom Frontend

For production applications, build a custom frontend with full control over UX.

### Available SDKs

| Platform | Package | Documentation |
|----------|---------|---------------|
| JavaScript/TypeScript | `livekit-client` | [JS SDK Docs](https://docs.livekit.io/client-sdk-js/) |
| React | `@livekit/components-react` | [React Components](https://docs.livekit.io/reference/components/react/) |
| Vue | `@livekit/components-vue` | [Vue Components](https://docs.livekit.io/reference/components/vue/) |
| iOS (Swift) | `LiveKit` | [Swift SDK](https://docs.livekit.io/client-sdk-swift/) |
| Android (Kotlin) | `io.livekit:livekit-android` | [Android SDK](https://docs.livekit.io/client-sdk-android/) |
| Flutter | `livekit_client` | [Flutter SDK](https://docs.livekit.io/client-sdk-flutter/) |
| React Native | `@livekit/react-native` | [React Native SDK](https://docs.livekit.io/client-sdk-react-native/) |
| Unity | `LiveKit Unity SDK` | [Unity SDK](https://docs.livekit.io/client-sdk-unity/) |

### React Example (Full)

```bash
npm install @livekit/components-react @livekit/components-styles
```

```tsx
import { LiveKitRoom, VideoConference, RoomAudioRenderer } from '@livekit/components-react';
import '@livekit/components-styles';

interface MeetingProps {
  token: string;
  serverUrl: string;
}

export function Meeting({ token, serverUrl }: MeetingProps) {
  return (
    <LiveKitRoom
      token={token}
      serverUrl={serverUrl}
      connect={true}
      audio={true}
      video={true}
    >
      {/* Full video conference UI */}
      <VideoConference />

      {/* Or minimal audio-only */}
      {/* <RoomAudioRenderer /> */}
    </LiveKitRoom>
  );
}
```

### React Example (Minimal Voice-Only)

```tsx
import { LiveKitRoom, useRoomContext, RoomAudioRenderer } from '@livekit/components-react';
import { useEffect } from 'react';

function VoiceRoom({ token, serverUrl }) {
  return (
    <LiveKitRoom token={token} serverUrl={serverUrl} connect={true} audio={true}>
      <RoomAudioRenderer />
      <VoiceControls />
    </LiveKitRoom>
  );
}

function VoiceControls() {
  const room = useRoomContext();

  useEffect(() => {
    // Auto-enable microphone on join
    room.localParticipant.setMicrophoneEnabled(true);
  }, [room]);

  const toggleMute = () => {
    const enabled = room.localParticipant.isMicrophoneEnabled;
    room.localParticipant.setMicrophoneEnabled(!enabled);
  };

  return (
    <button onClick={toggleMute}>
      Toggle Microphone
    </button>
  );
}
```

### Vanilla JavaScript Example

```html
<!DOCTYPE html>
<html>
<head>
  <title>Voice Agent</title>
  <script src="https://unpkg.com/livekit-client/dist/livekit-client.umd.js"></script>
</head>
<body>
  <button id="connect">Connect</button>
  <button id="mute">Mute</button>
  <div id="status">Disconnected</div>

  <script>
    const room = new LivekitClient.Room();
    const serverUrl = 'wss://your-project.livekit.cloud';

    document.getElementById('connect').onclick = async () => {
      const token = await fetchTokenFromBackend();

      await room.connect(serverUrl, token);
      document.getElementById('status').textContent = 'Connected';

      // Enable microphone
      await room.localParticipant.setMicrophoneEnabled(true);

      // Handle remote participants
      room.on('participantConnected', (participant) => {
        console.log('Participant joined:', participant.identity);
      });
    };

    document.getElementById('mute').onclick = () => {
      const enabled = room.localParticipant.isMicrophoneEnabled;
      room.localParticipant.setMicrophoneEnabled(!enabled);
    };
  </script>
</body>
</html>
```

## Token Generation

### Backend Endpoint (Go)

Your backend should expose an endpoint that generates tokens:

```go
import (
    "encoding/json"
    "net/http"

    "github.com/plexusone/omni-livekit/room"
)

func handleToken(w http.ResponseWriter, r *http.Request) {
    roomName := r.URL.Query().Get("room")
    userName := r.URL.Query().Get("name")

    client, _ := room.NewClient(room.Config{
        APIKey:    os.Getenv("LIVEKIT_API_KEY"),
        APISecret: os.Getenv("LIVEKIT_API_SECRET"),
        URL:       os.Getenv("LIVEKIT_URL"),
    })

    token, _ := client.GenerateClientToken(roomName, userName, userName)

    json.NewEncoder(w).Encode(map[string]string{
        "token": token,
    })
}
```

### Frontend Token Fetch

```typescript
async function fetchToken(room: string, name: string): Promise<string> {
  const response = await fetch(`/api/token?room=${room}&name=${name}`);
  const data = await response.json();
  return data.token;
}
```

## Recommendation Summary

| Use Case | Recommended Approach |
|----------|---------------------|
| Development/Testing | LiveKit Meet (zero code) |
| Quick Prototype | React components |
| Voice-only App | Minimal React or vanilla JS |
| Full Video App | React VideoConference component |
| Mobile App | Native SDK (Swift/Kotlin/Flutter) |
| Production | Custom frontend with your branding |

## Troubleshooting

### Common Issues

**Microphone not working:**

- Check browser permissions
- Ensure HTTPS (required for WebRTC)
- Try a different browser

**Cannot connect:**

- Verify token is valid and not expired
- Check server URL (must start with `wss://`)
- Ensure room exists (agent should join first or room should be created)

**Audio echo:**

- Use headphones
- Enable echo cancellation in browser settings
- LiveKit handles this automatically in most cases

## See Also

- [Voice Agent Guide](voice-agent.md) - Build a complete voice agent
- [OmniMeet Integration](omnimeet-integration.md) - Use OmniMeet abstraction
- [LiveKit Documentation](https://docs.livekit.io/) - Official LiveKit docs
