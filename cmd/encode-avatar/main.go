//go:build cgo

// Command encode-avatar pre-encodes an image as H.264 for use as a static avatar.
//
// This tool converts PNG/JPEG images to H.264 keyframes that can be used
// by the voice agent without requiring CGO at runtime.
//
// Usage:
//
//	# Basic encoding (uses original image dimensions)
//	go run ./cmd/encode-avatar -input avatar.png -output avatar.h264
//
//	# Resize to specific dimensions
//	go run ./cmd/encode-avatar -input avatar.png -output avatar.h264 -width 320 -height 320
//
//	# Embed in 16:9 canvas (recommended for LiveKit)
//	go run ./cmd/encode-avatar -input avatar.png -output avatar.h264 -canvas h360
//	go run ./cmd/encode-avatar -input avatar.png -output avatar.h264 -canvas 640x360 -bg black
//
// Canvas presets (16:9 aspect ratio):
//
//	h180  = 320x180
//	h360  = 640x360   (recommended)
//	h540  = 960x540
//	h720  = 1280x720
//
// The output .h264 file can be committed to your repository and loaded
// at runtime without any encoding dependencies.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strconv"
	"strings"

	"github.com/gen2brain/x264-go"
)

// LiveKit video presets (16:9 aspect ratio)
var canvasPresets = map[string][2]int{
	"h180": {320, 180},
	"h360": {640, 360},
	"h540": {960, 540},
	"h720": {1280, 720},
}

func main() {
	input := flag.String("input", "", "Input image file (PNG, JPEG, GIF)")
	output := flag.String("output", "", "Output H.264 file")
	width := flag.Int("width", 0, "Avatar width (0 = use original, scales proportionally)")
	height := flag.Int("height", 0, "Avatar height (0 = use original, scales proportionally)")
	canvas := flag.String("canvas", "", "Canvas size: preset (h180, h360, h540, h720) or WxH (e.g., 640x360)")
	bg := flag.String("bg", "black", "Background color: black, white, gray, or hex (#rrggbb)")
	preset := flag.String("preset", "medium", "Encoding preset (ultrafast, veryfast, fast, medium, slow)")
	flag.Parse()

	if *input == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "Usage: encode-avatar -input <image> -output <h264> [options]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Canvas presets (16:9, recommended for LiveKit):")
		fmt.Fprintln(os.Stderr, "  h180  = 320x180")
		fmt.Fprintln(os.Stderr, "  h360  = 640x360  (recommended)")
		fmt.Fprintln(os.Stderr, "  h540  = 960x540")
		fmt.Fprintln(os.Stderr, "  h720  = 1280x720")
		os.Exit(1)
	}

	// Read input image
	data, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}

	// Decode image
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding image: %v\n", err)
		os.Exit(1)
	}

	bounds := img.Bounds()
	origWidth := bounds.Dx()
	origHeight := bounds.Dy()

	fmt.Printf("Input:  %s (%s, %dx%d)\n", *input, format, origWidth, origHeight)

	// Parse canvas size
	canvasWidth, canvasHeight := 0, 0
	if *canvas != "" {
		canvasWidth, canvasHeight, err = parseCanvasSize(*canvas)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing canvas size: %v\n", err)
			os.Exit(1)
		}
	}

	// Parse background color
	bgColor, err := parseColor(*bg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing background color: %v\n", err)
		os.Exit(1)
	}

	// Determine avatar size
	avatarWidth := origWidth
	avatarHeight := origHeight
	if *width > 0 && *height > 0 {
		avatarWidth = *width
		avatarHeight = *height
	} else if *width > 0 {
		// Scale height proportionally
		avatarWidth = *width
		avatarHeight = origHeight * *width / origWidth
	} else if *height > 0 {
		// Scale width proportionally
		avatarHeight = *height
		avatarWidth = origWidth * *height / origHeight
	}

	var outputImg image.Image
	var outputWidth, outputHeight int

	if canvasWidth > 0 && canvasHeight > 0 {
		// Embed avatar in canvas
		outputWidth = canvasWidth
		outputHeight = canvasHeight

		// Scale avatar to fit within canvas if needed
		if avatarWidth > canvasWidth || avatarHeight > canvasHeight {
			scale := min(float64(canvasWidth)/float64(avatarWidth), float64(canvasHeight)/float64(avatarHeight))
			avatarWidth = int(float64(avatarWidth) * scale)
			avatarHeight = int(float64(avatarHeight) * scale)
		}

		// Create canvas and center avatar
		outputImg = compositeOnCanvas(img, canvasWidth, canvasHeight, avatarWidth, avatarHeight, bgColor)
		fmt.Printf("Canvas: %dx%d (avatar %dx%d centered)\n", canvasWidth, canvasHeight, avatarWidth, avatarHeight)
	} else {
		// Direct encoding (resize if specified)
		outputWidth = avatarWidth
		outputHeight = avatarHeight
		if avatarWidth != origWidth || avatarHeight != origHeight {
			outputImg = resizeImage(img, avatarWidth, avatarHeight)
		} else {
			outputImg = img
		}
	}

	// Ensure dimensions are even (H.264 requirement)
	outputWidth = outputWidth &^ 1
	outputHeight = outputHeight &^ 1

	fmt.Printf("Output: %s (H.264, %dx%d)\n", *output, outputWidth, outputHeight)

	// Encode to H.264
	h264Data, err := encodeToH264(outputImg, outputWidth, outputHeight, *preset)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding: %v\n", err)
		os.Exit(1)
	}

	// Write output
	if err := os.WriteFile(*output, h264Data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Encoded: %d bytes (%.1f KB)\n", len(h264Data), float64(len(h264Data))/1024)
	fmt.Println("Done! Use this file with ImageConfig.H264Path in your agent.")
}

// parseCanvasSize parses a canvas size string (preset name or WxH).
func parseCanvasSize(s string) (int, int, error) {
	// Check for preset
	if dims, ok := canvasPresets[strings.ToLower(s)]; ok {
		return dims[0], dims[1], nil
	}

	// Parse WxH format
	parts := strings.Split(strings.ToLower(s), "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid canvas size %q (use preset or WxH format)", s)
	}

	w, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid width: %w", err)
	}

	h, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid height: %w", err)
	}

	return w, h, nil
}

// parseColor parses a color string (name or hex).
func parseColor(s string) (color.Color, error) {
	switch strings.ToLower(s) {
	case "black":
		return color.Black, nil
	case "white":
		return color.White, nil
	case "gray", "grey":
		return color.Gray{Y: 128}, nil
	case "transparent":
		return color.Transparent, nil
	}

	// Parse hex color (#rrggbb or #rgb)
	if strings.HasPrefix(s, "#") {
		hex := s[1:]
		if len(hex) == 3 {
			// Expand #rgb to #rrggbb
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		if len(hex) != 6 {
			return nil, fmt.Errorf("invalid hex color %q", s)
		}

		r, err := strconv.ParseUint(hex[0:2], 16, 8)
		if err != nil {
			return nil, fmt.Errorf("invalid red component: %w", err)
		}
		g, err := strconv.ParseUint(hex[2:4], 16, 8)
		if err != nil {
			return nil, fmt.Errorf("invalid green component: %w", err)
		}
		b, err := strconv.ParseUint(hex[4:6], 16, 8)
		if err != nil {
			return nil, fmt.Errorf("invalid blue component: %w", err)
		}

		return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}, nil
	}

	return nil, fmt.Errorf("unknown color %q (use black, white, gray, transparent, or #rrggbb)", s)
}

// compositeOnCanvas creates a canvas with the avatar centered.
func compositeOnCanvas(avatar image.Image, canvasW, canvasH, avatarW, avatarH int, bg color.Color) image.Image {
	// Create canvas
	canvas := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))

	// Fill with background color
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	// Resize avatar if needed
	var scaledAvatar image.Image
	avatarBounds := avatar.Bounds()
	if avatarBounds.Dx() != avatarW || avatarBounds.Dy() != avatarH {
		scaledAvatar = resizeImage(avatar, avatarW, avatarH)
	} else {
		scaledAvatar = avatar
	}

	// Calculate centered position
	x := (canvasW - avatarW) / 2
	y := (canvasH - avatarH) / 2

	// Draw avatar onto canvas
	draw.Draw(canvas, image.Rect(x, y, x+avatarW, y+avatarH), scaledAvatar, image.Point{}, draw.Over)

	return canvas
}

// resizeImage resizes an image using nearest-neighbor scaling.
// For better quality, consider using golang.org/x/image/draw.
func resizeImage(img image.Image, width, height int) image.Image {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			srcX := x * srcW / width
			srcY := y * srcH / height
			dst.Set(x, y, img.At(bounds.Min.X+srcX, bounds.Min.Y+srcY))
		}
	}

	return dst
}

func encodeToH264(img image.Image, width, height int, preset string) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 64*1024))

	opts := &x264.Options{
		Width:     width,
		Height:    height,
		FrameRate: 1,
		KeyInt:    1, // Every frame is a keyframe
		Tune:      "stillimage",
		Preset:    preset,
		Profile:   "baseline", // Maximum browser compatibility
		LogLevel:  x264.LogNone,
	}

	enc, err := x264.NewEncoder(buf, opts)
	if err != nil {
		return nil, fmt.Errorf("create encoder: %w", err)
	}
	defer enc.Close()

	if err := enc.Encode(img); err != nil {
		return nil, fmt.Errorf("encode frame: %w", err)
	}

	if err := enc.Flush(); err != nil {
		return nil, fmt.Errorf("flush encoder: %w", err)
	}

	return buf.Bytes(), nil
}
