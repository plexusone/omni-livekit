package agent

import (
	"time"
)

// MediaMode defines what media tracks the agent publishes.
type MediaMode string

const (
	// AudioOnly publishes only an audio track (default).
	AudioOnly MediaMode = "audio_only"
	// AudioWithImage publishes audio + a static image as video.
	AudioWithImage MediaMode = "audio_with_image"
	// AudioWithVideo publishes audio + video frames.
	AudioWithVideo MediaMode = "audio_with_video"
)

// Options configures a LiveKit agent.
type Options struct {
	// LiveKit connection settings
	APIKey    string
	APISecret string
	ServerURL string

	// Agent identity
	Name     string // Display name in the meeting
	Identity string // Unique identity (defaults to generated UUID)
	Metadata map[string]string

	// Media mode selection
	MediaMode MediaMode // Defaults to AudioOnly

	// Audio configuration
	Audio AudioConfig

	// Video configuration (for AudioWithVideo mode)
	Video VideoConfig

	// Image configuration (for AudioWithImage mode)
	Image ImageConfig

	// Behavior
	AutoSubscribe bool // Auto-subscribe to all tracks (default: true)
}

// AudioConfig configures audio publishing.
type AudioConfig struct {
	// SampleRate in Hz (default: 48000)
	SampleRate int
	// Channels: 1 = mono, 2 = stereo (default: 1)
	Channels int
	// FrameDuration for audio frames (default: 20ms)
	FrameDuration time.Duration
	// TrackName for the published audio track
	TrackName string
}

// VideoConfig configures video publishing.
type VideoConfig struct {
	// Width in pixels (default: 640)
	Width int
	// Height in pixels (default: 480)
	Height int
	// FrameRate in fps (default: 30)
	FrameRate int
	// Codec: "vp8" or "h264" (default: "vp8")
	Codec string
	// Bitrate in bps (default: auto)
	Bitrate int
	// TrackName for the published video track
	TrackName string
}

// ImageConfig configures static image video publishing.
type ImageConfig struct {
	// Path to the image file (PNG, JPEG, etc.)
	// Requires CGO for runtime encoding. For no-CGO deployment, use H264Path.
	Path string
	// Data is raw image data (alternative to Path)
	// Requires CGO for runtime encoding. For no-CGO deployment, use H264Data.
	Data []byte

	// H264Path is the path to a pre-encoded H.264 keyframe file.
	// Use the encode-avatar tool to create this file.
	// This is the recommended approach - no CGO required at runtime.
	H264Path string
	// H264Data is pre-encoded H.264 keyframe data (alternative to H264Path).
	// Use the encode-avatar tool to create this data.
	H264Data []byte

	// Width to resize to (0 = use original). Only used with Path/Data.
	Width int
	// Height to resize to (0 = use original). Only used with Path/Data.
	Height int
	// FrameRate for the static image video track (default: 1)
	// Lower values use less bandwidth for static content.
	FrameRate int
	// TrackName for the published video track
	TrackName string
}

// ImageWriter is the interface for static image video publishing.
type ImageWriter interface {
	// UpdateImage updates the image being published.
	UpdateImage(data []byte) error
	// Close stops the image writer.
	Close() error
}

// applyDefaults fills in default values for Options.
func (o *Options) applyDefaults() {
	if o.MediaMode == "" {
		o.MediaMode = AudioOnly
	}

	// Audio defaults
	if o.Audio.SampleRate == 0 {
		o.Audio.SampleRate = 48000
	}
	if o.Audio.Channels == 0 {
		o.Audio.Channels = 1
	}
	if o.Audio.FrameDuration == 0 {
		o.Audio.FrameDuration = 20 * time.Millisecond
	}
	if o.Audio.TrackName == "" {
		o.Audio.TrackName = "audio"
	}

	// Video defaults
	if o.Video.Width == 0 {
		o.Video.Width = 640
	}
	if o.Video.Height == 0 {
		o.Video.Height = 480
	}
	if o.Video.FrameRate == 0 {
		o.Video.FrameRate = 30
	}
	if o.Video.Codec == "" {
		o.Video.Codec = "vp8"
	}
	if o.Video.TrackName == "" {
		o.Video.TrackName = "video"
	}

	// Image defaults
	if o.Image.FrameRate == 0 {
		o.Image.FrameRate = 1 // 1 fps for static images
	}
	if o.Image.TrackName == "" {
		o.Image.TrackName = "video"
	}
}
