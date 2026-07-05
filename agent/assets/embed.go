package assets

import _ "embed"

// DefaultAvatarH264 is the pre-encoded H.264 keyframe for the default agent avatar.
// This is the OmniAgent icon, encoded at 320x320 pixels.
// Source: docs/images/image_omniagent_icon_v1.png
//
//go:embed default_avatar.h264
var DefaultAvatarH264 []byte
