package assets

import _ "embed"

// DefaultAvatarH264 is the pre-encoded H.264 keyframe for the default agent avatar.
// This is the OmniAgent icon centered on a 640x360 (16:9) canvas.
// The 16:9 aspect ratio ensures proper display in LiveKit video slots.
// Source: docs/images/image_omniagent_icon_v1.png
//
//go:embed default_avatar.h264
var DefaultAvatarH264 []byte
