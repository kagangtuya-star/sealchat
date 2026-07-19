package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/webp"
)

type animatedWebPFrame struct {
	x        int
	y        int
	width    int
	height   int
	duration int
	dispose  bool
	blend    bool
	payload  []byte
}

func decodeAnimatedWebPFrames(ctx context.Context, sourcePath, targetDir string, metadata theaterMediaMetadata) (string, error) {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", err
	}
	width, height, background, frames, err := parseAnimatedWebP(data)
	if err != nil {
		return "", err
	}
	if width != metadata.Width || height != metadata.Height || len(frames) != metadata.FrameCount {
		return "", errors.New("Animated WebP metadata changed during conversion")
	}
	canvasBackground := animatedWebPCanvasBackground(background, metadata.HasAlpha)
	framesDir := filepath.Join(targetDir, "webp-frames")
	if err := os.MkdirAll(framesDir, 0o700); err != nil {
		return "", err
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(canvasBackground), image.Point{}, draw.Src)
	framePaths := make([]string, 0, len(frames))
	durations := make([]int, 0, len(frames))
	for index, frame := range frames {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		decoded, err := decodeAnimatedWebPFrame(frame)
		if err != nil {
			return "", fmt.Errorf("decode frame %d: %w", index+1, err)
		}
		target := image.Rect(frame.x, frame.y, frame.x+frame.width, frame.y+frame.height)
		if !target.In(canvas.Bounds()) {
			return "", fmt.Errorf("frame %d exceeds WebP canvas", index+1)
		}
		operator := draw.Over
		if !frame.blend {
			operator = draw.Src
		}
		draw.Draw(canvas, target, decoded, decoded.Bounds().Min, operator)
		framePath := filepath.Join(framesDir, fmt.Sprintf("frame-%06d.png", index))
		if err := writeAnimatedWebPPNG(framePath, canvas); err != nil {
			return "", err
		}
		framePaths = append(framePaths, framePath)
		durations = append(durations, frame.duration)
		if frame.dispose {
			draw.Draw(canvas, target, image.NewUniform(canvasBackground), image.Point{}, draw.Src)
		}
	}
	concatPath := filepath.Join(targetDir, "webp-frames.txt")
	var concat strings.Builder
	for index, framePath := range framePaths {
		fmt.Fprintf(&concat, "file '%s'\n", ffmpegConcatPath(framePath))
		fmt.Fprintf(&concat, "duration %.6f\n", float64(durations[index])/1000)
	}
	if len(framePaths) > 0 {
		fmt.Fprintf(&concat, "file '%s'\n", ffmpegConcatPath(framePaths[len(framePaths)-1]))
	}
	if err := os.WriteFile(concatPath, []byte(concat.String()), 0o600); err != nil {
		return "", err
	}
	return concatPath, nil
}

func animatedWebPCanvasBackground(declared color.NRGBA, hasAlpha bool) color.NRGBA {
	if hasAlpha {
		// ANIM background is a viewer hint; keep alpha-preserving output transparent.
		return color.NRGBA{}
	}
	return declared
}

func parseAnimatedWebP(data []byte) (int, int, color.NRGBA, []animatedWebPFrame, error) {
	if len(data) < 20 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return 0, 0, color.NRGBA{}, nil, errors.New("invalid Animated WebP")
	}
	width, height := 0, 0
	background := color.NRGBA{}
	frames := []animatedWebPFrame{}
	for offset := 12; offset+8 <= len(data); {
		chunkType := string(data[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		chunkStart := offset + 8
		chunkEnd := chunkStart + chunkSize
		if chunkSize < 0 || chunkEnd > len(data) {
			return 0, 0, color.NRGBA{}, nil, errors.New("invalid Animated WebP chunk")
		}
		chunk := data[chunkStart:chunkEnd]
		switch chunkType {
		case "VP8X":
			if len(chunk) >= 10 {
				width = 1 + uint24LE(chunk[4:7])
				height = 1 + uint24LE(chunk[7:10])
			}
		case "ANIM":
			if len(chunk) >= 4 {
				background = color.NRGBA{R: chunk[2], G: chunk[1], B: chunk[0], A: chunk[3]}
			}
		case "ANMF":
			if len(chunk) < 16 {
				return 0, 0, color.NRGBA{}, nil, errors.New("invalid Animated WebP frame")
			}
			duration := uint24LE(chunk[12:15])
			if duration <= 0 {
				duration = 10
			}
			frames = append(frames, animatedWebPFrame{
				x: uint24LE(chunk[0:3]) * 2, y: uint24LE(chunk[3:6]) * 2,
				width: 1 + uint24LE(chunk[6:9]), height: 1 + uint24LE(chunk[9:12]), duration: duration,
				dispose: chunk[15]&0x01 != 0, blend: chunk[15]&0x02 == 0,
				payload: append([]byte(nil), chunk[16:]...),
			})
		}
		offset = chunkEnd + chunkSize%2
	}
	if width <= 0 || height <= 0 || len(frames) <= 1 {
		return 0, 0, color.NRGBA{}, nil, errors.New("Animated WebP canvas or frames missing")
	}
	return width, height, background, frames, nil
}

func decodeAnimatedWebPFrame(frame animatedWebPFrame) (image.Image, error) {
	chunks, hasAlpha, err := animatedWebPFrameChunks(frame.payload)
	if err != nil {
		return nil, err
	}
	content := make([]byte, 0, len(chunks)+32)
	if hasAlpha {
		vp8x := make([]byte, 10)
		vp8x[0] = 0x10
		putAnimatedWebPUint24(vp8x[4:7], frame.width-1)
		putAnimatedWebPUint24(vp8x[7:10], frame.height-1)
		content = appendAnimatedWebPChunk(content, "VP8X", vp8x)
	}
	content = append(content, chunks...)
	data := append([]byte("RIFF\x00\x00\x00\x00WEBP"), content...)
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
	decoded, err := webp.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if decoded.Bounds().Dx() != frame.width || decoded.Bounds().Dy() != frame.height {
		return nil, errors.New("Animated WebP frame dimensions mismatch")
	}
	return decoded, nil
}

func animatedWebPFrameChunks(payload []byte) ([]byte, bool, error) {
	result := []byte{}
	hasAlpha := false
	hasImage := false
	for offset := 0; offset+8 <= len(payload); {
		chunkType := string(payload[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(payload[offset+4 : offset+8]))
		chunkEnd := offset + 8 + chunkSize
		paddedEnd := chunkEnd + chunkSize%2
		if chunkSize < 0 || chunkEnd > len(payload) || paddedEnd > len(payload) {
			return nil, false, errors.New("invalid Animated WebP frame chunk")
		}
		if chunkType == "ALPH" || chunkType == "VP8 " || chunkType == "VP8L" {
			result = append(result, payload[offset:paddedEnd]...)
		}
		if chunkType == "ALPH" {
			hasAlpha = true
		}
		if chunkType == "VP8 " || chunkType == "VP8L" {
			hasImage = true
		}
		offset = paddedEnd
	}
	if !hasImage {
		return nil, false, errors.New("Animated WebP frame image data missing")
	}
	return result, hasAlpha, nil
}

func appendAnimatedWebPChunk(target []byte, name string, payload []byte) []byte {
	target = append(target, []byte(name)...)
	size := make([]byte, 4)
	binary.LittleEndian.PutUint32(size, uint32(len(payload)))
	target = append(target, size...)
	target = append(target, payload...)
	if len(payload)%2 != 0 {
		target = append(target, 0)
	}
	return target
}

func putAnimatedWebPUint24(target []byte, value int) {
	target[0] = byte(value)
	target[1] = byte(value >> 8)
	target[2] = byte(value >> 16)
}

func writeAnimatedWebPPNG(path string, frame image.Image) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	encodeErr := png.Encode(file, frame)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}

func ffmpegConcatPath(path string) string {
	return strings.ReplaceAll(filepath.ToSlash(path), "'", "'\\''")
}
