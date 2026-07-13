package room

import (
	"context"
	"fmt"

	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

// RecordingLayout specifies how participants are arranged in the recording.
type RecordingLayout string

const (
	// LayoutGrid shows all participants in a grid.
	LayoutGrid RecordingLayout = "grid"
	// LayoutSpeaker focuses on the active speaker.
	LayoutSpeaker RecordingLayout = "speaker"
	// LayoutSingleSpeaker shows only the active speaker.
	LayoutSingleSpeaker RecordingLayout = "single-speaker"
)

// RecordingFormat specifies the output file format.
type RecordingFormat string

const (
	FormatMP4 RecordingFormat = "mp4"
	FormatOGG RecordingFormat = "ogg"
)

// RecordingConfig configures room recording.
type RecordingConfig struct {
	// RoomName is the room to record.
	RoomName string

	// Layout determines participant arrangement.
	Layout RecordingLayout

	// Format is the output file format.
	Format RecordingFormat

	// FilePath is the local file path for the recording.
	// Use this for local file output.
	FilePath string

	// S3 configures S3 upload (optional).
	S3 *S3Config

	// Width is the video width (default: 1920).
	Width int

	// Height is the video height (default: 1080).
	Height int

	// AudioOnly records audio only (no video).
	AudioOnly bool
}

// S3Config configures S3 upload for recordings.
type S3Config struct {
	AccessKey string
	Secret    string
	Region    string
	Bucket    string
	Endpoint  string // Optional, for S3-compatible services
}

// RecordingInfo contains information about an active recording.
type RecordingInfo struct {
	// EgressID is the unique identifier for this recording.
	EgressID string

	// RoomName is the room being recorded.
	RoomName string

	// Status is the current recording status.
	Status string

	// StartedAt is when the recording started (Unix timestamp).
	StartedAt int64
}

// RecordingClient provides recording operations.
type RecordingClient struct {
	egress *lksdk.EgressClient
}

// NewRecordingClient creates a new recording client.
func NewRecordingClient(serverURL, apiKey, apiSecret string) *RecordingClient {
	return &RecordingClient{
		egress: lksdk.NewEgressClient(serverURL, apiKey, apiSecret),
	}
}

// StartRecording starts recording a room.
func (c *RecordingClient) StartRecording(ctx context.Context, cfg RecordingConfig) (*RecordingInfo, error) {
	if cfg.RoomName == "" {
		return nil, fmt.Errorf("room name is required")
	}
	if cfg.FilePath == "" && cfg.S3 == nil {
		return nil, fmt.Errorf("either FilePath or S3 config is required")
	}

	// Set defaults
	if cfg.Layout == "" {
		cfg.Layout = LayoutSpeaker
	}
	if cfg.Format == "" {
		cfg.Format = FormatMP4
	}
	if cfg.Width == 0 {
		cfg.Width = 1920
	}
	if cfg.Height == 0 {
		cfg.Height = 1080
	}

	// Build file output
	fileType := livekit.EncodedFileType_MP4
	if cfg.Format == FormatOGG {
		fileType = livekit.EncodedFileType_OGG
	}

	var output livekit.EncodedFileOutput
	output.FileType = fileType

	if cfg.S3 != nil {
		output.Output = &livekit.EncodedFileOutput_S3{
			S3: &livekit.S3Upload{
				AccessKey: cfg.S3.AccessKey,
				Secret:    cfg.S3.Secret,
				Region:    cfg.S3.Region,
				Bucket:    cfg.S3.Bucket,
				Endpoint:  cfg.S3.Endpoint,
			},
		}
	} else {
		output.Filepath = cfg.FilePath
	}

	req := &livekit.RoomCompositeEgressRequest{
		RoomName:  cfg.RoomName,
		Layout:    string(cfg.Layout),
		AudioOnly: cfg.AudioOnly,
		Output: &livekit.RoomCompositeEgressRequest_File{
			File: &output,
		},
		VideoOnly: false,
	}

	info, err := c.egress.StartRoomCompositeEgress(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("start recording: %w", err)
	}

	return &RecordingInfo{
		EgressID:  info.EgressId,
		RoomName:  cfg.RoomName,
		Status:    info.Status.String(),
		StartedAt: info.StartedAt,
	}, nil
}

// StopRecording stops an active recording.
func (c *RecordingClient) StopRecording(ctx context.Context, egressID string) error {
	_, err := c.egress.StopEgress(ctx, &livekit.StopEgressRequest{
		EgressId: egressID,
	})
	if err != nil {
		return fmt.Errorf("stop recording: %w", err)
	}
	return nil
}

// ListRecordings lists active recordings for a room.
func (c *RecordingClient) ListRecordings(ctx context.Context, roomName string) ([]*RecordingInfo, error) {
	resp, err := c.egress.ListEgress(ctx, &livekit.ListEgressRequest{
		RoomName: roomName,
		Active:   true,
	})
	if err != nil {
		return nil, fmt.Errorf("list recordings: %w", err)
	}

	var recordings []*RecordingInfo
	for _, info := range resp.Items {
		recordings = append(recordings, &RecordingInfo{
			EgressID:  info.EgressId,
			RoomName:  roomName,
			Status:    info.Status.String(),
			StartedAt: info.StartedAt,
		})
	}
	return recordings, nil
}
