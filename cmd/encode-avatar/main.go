//go:build cgo

// Command encode-avatar pre-encodes an image as H.264 for use as a static avatar.
//
// This tool converts PNG/JPEG images to H.264 keyframes that can be used
// by the voice agent without requiring CGO at runtime.
//
// Usage:
//
//	go run ./cmd/encode-avatar -input avatar.png -output avatar.h264
//	go run ./cmd/encode-avatar -input avatar.png -output avatar.h264 -width 640 -height 480
//
// The output .h264 file can be committed to your repository and loaded
// at runtime without any encoding dependencies.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"

	"github.com/gen2brain/x264-go"
)

func main() {
	input := flag.String("input", "", "Input image file (PNG, JPEG, GIF)")
	output := flag.String("output", "", "Output H.264 file")
	width := flag.Int("width", 0, "Target width (0 = use original)")
	height := flag.Int("height", 0, "Target height (0 = use original)")
	quality := flag.String("preset", "medium", "Encoding preset (ultrafast, veryfast, fast, medium, slow)")
	flag.Parse()

	if *input == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "Usage: encode-avatar -input <image> -output <h264>")
		flag.PrintDefaults()
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
	imgWidth := bounds.Dx()
	imgHeight := bounds.Dy()

	// Use target dimensions if specified
	if *width > 0 {
		imgWidth = *width
	}
	if *height > 0 {
		imgHeight = *height
	}

	// Ensure dimensions are even (H.264 requirement)
	imgWidth = imgWidth &^ 1
	imgHeight = imgHeight &^ 1

	fmt.Printf("Input:  %s (%s, %dx%d)\n", *input, format, bounds.Dx(), bounds.Dy())
	fmt.Printf("Output: %s (H.264, %dx%d)\n", *output, imgWidth, imgHeight)

	// Encode to H.264
	h264Data, err := encodeToH264(img, imgWidth, imgHeight, *quality)
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
