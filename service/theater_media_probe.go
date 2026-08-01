package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	_ "golang.org/x/image/webp"

	"sealchat/utils"
)

type theaterMediaMetadata struct {
	Kind       string
	MimeType   string
	Width      int
	Height     int
	DurationMS int64
	FrameCount int
	FrameRate  float64
	LoopCount  *int
	Container  string
	VideoCodec string
	AudioCodec string
	HasAlpha   bool
}

func detectTheaterMediaType(head []byte) (string, string) {
	if len(head) >= 8 && bytes.Equal(head[:8], []byte("\x89PNG\r\n\x1a\n")) {
		if bytes.Contains(head, []byte("acTL")) {
			return "image/apng", "animated_image"
		}
		return "image/png", "static_image"
	}
	if len(head) >= 3 && bytes.Equal(head[:3], []byte("\xff\xd8\xff")) {
		return "image/jpeg", "static_image"
	}
	if len(head) >= 6 && (string(head[:6]) == "GIF87a" || string(head[:6]) == "GIF89a") {
		return "image/gif", "animated_image"
	}
	if len(head) >= 16 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP" {
		if bytes.Contains(head, []byte("ANIM")) || (len(head) > 20 && string(head[12:16]) == "VP8X" && head[20]&0x02 != 0) {
			return "image/webp", "animated_image"
		}
		return "image/webp", "static_image"
	}
	if len(head) >= 12 && string(head[4:8]) == "ftyp" {
		return "video/mp4", "video"
	}
	if len(head) >= 4 && bytes.Equal(head[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}) {
		return "video/webm", "video"
	}
	return "", ""
}

func probeTheaterMedia(ctx context.Context, path, kind, mimeType string, config utils.TheaterMediaConfig, toolchain MediaToolchain, runner MediaCommandRunner) (theaterMediaMetadata, error) {
	switch kind {
	case "static_image":
		file, err := os.Open(path)
		if err != nil {
			return theaterMediaMetadata{}, err
		}
		defer file.Close()
		decoded, _, err := image.DecodeConfig(file)
		if err != nil {
			if mimeType == "image/webp" {
				webpMetadata, parseErr := parseWebPMetadata(path)
				if parseErr == nil && !webpMetadata.Animated {
					return validateTheaterMediaMetadata(theaterMediaMetadata{Kind: kind, MimeType: mimeType, Width: webpMetadata.Width, Height: webpMetadata.Height, FrameCount: 1, HasAlpha: webpMetadata.HasAlpha}, config)
				}
			}
			return theaterMediaMetadata{}, fmt.Errorf("IMAGE_DECODE_FAILED: %w", err)
		}
		return validateTheaterMediaMetadata(theaterMediaMetadata{Kind: kind, MimeType: mimeType, Width: decoded.Width, Height: decoded.Height, FrameCount: 1}, config)
	case "animated_image":
		if mimeType == "video/webm" {
			metadata, err := probeTheaterVideo(ctx, path, mimeType, config, toolchain, runner)
			if err != nil {
				return theaterMediaMetadata{}, err
			}
			metadata.Kind = "animated_image"
			return validateTheaterMediaMetadata(metadata, config)
		}
		metadata, err := probeAnimatedImage(path, mimeType)
		if err != nil {
			return theaterMediaMetadata{}, err
		}
		return validateTheaterMediaMetadata(metadata, config)
	case "video":
		return probeTheaterVideo(ctx, path, mimeType, config, toolchain, runner)
	default:
		return theaterMediaMetadata{}, errors.New(TheaterMediaErrorUnsupported)
	}
}

func probeTheaterVideo(ctx context.Context, path, mimeType string, config utils.TheaterMediaConfig, toolchain MediaToolchain, runner MediaCommandRunner) (theaterMediaMetadata, error) {
	if !toolchain.FFprobeAvailable() {
		return theaterMediaMetadata{}, errors.New(TheaterMediaErrorProcessorUnavailable)
	}
	probeCtx, cancel := context.WithTimeout(ctx, time.Duration(config.ProbeTimeoutSeconds)*time.Second)
	defer cancel()
	output, err := runner.Run(probeCtx, toolchain.FFprobePath, "-v", "error", "-show_format", "-show_streams", "-of", "json", path)
	if err != nil {
		return theaterMediaMetadata{}, fmt.Errorf("%s: %w", TheaterMediaErrorProbeFailed, err)
	}
	metadata, err := parseFFprobeMetadata(output, mimeType)
	if err != nil {
		return theaterMediaMetadata{}, err
	}
	return validateTheaterMediaMetadata(metadata, config)
}

func probeAnimatedImage(path, mimeType string) (theaterMediaMetadata, error) {
	switch mimeType {
	case "image/gif":
		file, err := os.Open(path)
		if err != nil {
			return theaterMediaMetadata{}, err
		}
		defer file.Close()
		decoded, err := gif.DecodeAll(file)
		if err != nil {
			return theaterMediaMetadata{}, fmt.Errorf("IMAGE_DECODE_FAILED: %w", err)
		}
		duration := int64(0)
		for _, delay := range decoded.Delay {
			if delay < 2 {
				delay = 2
			}
			duration += int64(delay) * 10
		}
		kind := "animated_image"
		if len(decoded.Image) <= 1 {
			kind = "static_image"
		}
		return theaterMediaMetadata{Kind: kind, MimeType: mimeType, Width: decoded.Config.Width, Height: decoded.Config.Height, FrameCount: len(decoded.Image), DurationMS: duration, LoopCount: gifPlaybackCount(decoded.LoopCount)}, nil
	case "image/apng":
		return parseAPNGMetadata(path)
	case "image/webp":
		webpMetadata, err := parseWebPMetadata(path)
		if err != nil {
			return theaterMediaMetadata{}, errors.New("IMAGE_DECODE_FAILED: WebP animation metadata invalid")
		}
		kind := "animated_image"
		if !webpMetadata.Animated || webpMetadata.FrameCount <= 1 {
			kind = "static_image"
		}
		return theaterMediaMetadata{Kind: kind, MimeType: mimeType, Width: webpMetadata.Width, Height: webpMetadata.Height, FrameCount: webpMetadata.FrameCount, DurationMS: webpMetadata.DurationMS, LoopCount: webpMetadata.LoopCount, HasAlpha: webpMetadata.HasAlpha}, nil
	default:
		return theaterMediaMetadata{}, errors.New(TheaterMediaErrorUnsupported)
	}
}

func gifPlaybackCount(loopCount int) *int {
	if loopCount == 0 {
		return intPointer(0)
	}
	plays := 1
	if loopCount > 0 {
		// GIF stores additional repeats, while the stage property stores total plays.
		plays = min(loopCount+1, 65_535)
	}
	return &plays
}

func parseAPNGMetadata(path string) (theaterMediaMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return theaterMediaMetadata{}, err
	}
	if len(data) < 33 || !bytes.Contains(data, []byte("acTL")) {
		return theaterMediaMetadata{}, errors.New("IMAGE_DECODE_FAILED: invalid APNG")
	}
	width := int(binary.BigEndian.Uint32(data[16:20]))
	height := int(binary.BigEndian.Uint32(data[20:24]))
	index := bytes.Index(data, []byte("acTL"))
	frameCount := 1
	if index >= 4 && index+8 <= len(data) {
		frameCount = int(binary.BigEndian.Uint32(data[index+4 : index+8]))
	}
	duration := int64(0)
	for offset := 0; ; {
		index := bytes.Index(data[offset:], []byte("fcTL"))
		if index < 0 {
			break
		}
		index += offset
		if index+26 <= len(data) {
			numerator := binary.BigEndian.Uint16(data[index+20 : index+22])
			denominator := binary.BigEndian.Uint16(data[index+22 : index+24])
			if denominator == 0 {
				denominator = 100
			}
			duration += int64(numerator) * 1000 / int64(denominator)
		}
		offset = index + 4
	}
	var loopCount *int
	if index >= 4 && index+12 <= len(data) {
		plays := int(binary.BigEndian.Uint32(data[index+8 : index+12]))
		loopCount = &plays
	}
	return theaterMediaMetadata{Kind: "animated_image", MimeType: "image/apng", Width: width, Height: height, FrameCount: frameCount, DurationMS: duration, LoopCount: loopCount}, nil
}

type theaterWebPMetadata struct {
	Width      int
	Height     int
	Animated   bool
	FrameCount int
	DurationMS int64
	LoopCount  *int
	HasAlpha   bool
}

func parseWebPMetadata(path string) (theaterWebPMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return theaterWebPMetadata{}, err
	}
	if len(data) < 20 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return theaterWebPMetadata{}, errors.New("invalid WebP")
	}
	metadata := theaterWebPMetadata{}
	for offset := 12; offset+8 <= len(data); {
		chunkType := string(data[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		chunkStart := offset + 8
		chunkEnd := chunkStart + chunkSize
		if chunkSize < 0 || chunkEnd > len(data) {
			return theaterWebPMetadata{}, errors.New("invalid WebP chunk")
		}
		chunk := data[chunkStart:chunkEnd]
		switch chunkType {
		case "VP8X":
			if len(chunk) >= 10 {
				metadata.Animated = chunk[0]&0x02 != 0
				metadata.HasAlpha = chunk[0]&0x10 != 0
				metadata.Width = 1 + uint24LE(chunk[4:7])
				metadata.Height = 1 + uint24LE(chunk[7:10])
			}
		case "ANMF":
			if len(chunk) >= 16 {
				metadata.FrameCount++
				duration := uint24LE(chunk[12:15])
				if duration <= 0 {
					duration = 10
				}
				metadata.DurationMS += int64(duration)
			}
		case "ANIM":
			if len(chunk) >= 6 {
				plays := int(binary.LittleEndian.Uint16(chunk[4:6]))
				metadata.LoopCount = &plays
			}
		case "ALPH":
			metadata.HasAlpha = true
		}
		offset = chunkEnd + chunkSize%2
	}
	if metadata.Width <= 0 || metadata.Height <= 0 {
		return theaterWebPMetadata{}, errors.New("WebP dimensions unavailable")
	}
	if metadata.FrameCount == 0 {
		metadata.FrameCount = 1
	}
	metadata.Animated = metadata.Animated || metadata.FrameCount > 1
	return metadata, nil
}

func intPointer(value int) *int {
	return &value
}

func uint24LE(value []byte) int {
	if len(value) < 3 {
		return 0
	}
	return int(value[0]) | int(value[1])<<8 | int(value[2])<<16
}

func parseFFprobeMetadata(raw []byte, mimeType string) (theaterMediaMetadata, error) {
	var document struct {
		Streams []struct {
			CodecType    string            `json:"codec_type"`
			CodecName    string            `json:"codec_name"`
			Width        int               `json:"width"`
			Height       int               `json:"height"`
			AvgFrameRate string            `json:"avg_frame_rate"`
			NBFrames     string            `json:"nb_frames"`
			PixelFormat  string            `json:"pix_fmt"`
			Tags         map[string]string `json:"tags"`
		} `json:"streams"`
		Format struct {
			FormatName string `json:"format_name"`
			Duration   string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return theaterMediaMetadata{}, fmt.Errorf("%s: %w", TheaterMediaErrorProbeFailed, err)
	}
	metadata := theaterMediaMetadata{Kind: "video", MimeType: mimeType, Container: document.Format.FormatName}
	videoStreams := 0
	for _, stream := range document.Streams {
		switch stream.CodecType {
		case "video":
			videoStreams++
			metadata.VideoCodec = stream.CodecName
			metadata.Width = stream.Width
			metadata.Height = stream.Height
			metadata.FrameRate = parseFrameRate(stream.AvgFrameRate)
			metadata.FrameCount, _ = strconv.Atoi(stream.NBFrames)
			metadata.HasAlpha = strings.Contains(stream.PixelFormat, "a") || stream.Tags["alpha_mode"] == "1"
		case "audio":
			if metadata.AudioCodec == "" {
				metadata.AudioCodec = stream.CodecName
			}
		}
	}
	if videoStreams != 1 {
		return theaterMediaMetadata{}, errors.New("MEDIA_PROBE_FAILED: video stream count invalid")
	}
	duration, _ := strconv.ParseFloat(document.Format.Duration, 64)
	metadata.DurationMS = int64(duration * 1000)
	if metadata.FrameCount <= 0 && metadata.DurationMS > 0 && metadata.FrameRate > 0 {
		metadata.FrameCount = int(math.Ceil(float64(metadata.DurationMS) * metadata.FrameRate / 1000))
	}
	return metadata, nil
}

func parseFrameRate(value string) float64 {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		result, _ := strconv.ParseFloat(value, 64)
		return result
	}
	numerator, _ := strconv.ParseFloat(parts[0], 64)
	denominator, _ := strconv.ParseFloat(parts[1], 64)
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}

func validateTheaterMediaMetadata(metadata theaterMediaMetadata, config utils.TheaterMediaConfig) (theaterMediaMetadata, error) {
	if metadata.Width <= 0 || metadata.Height <= 0 || metadata.Width > config.MaxDimension || metadata.Height > config.MaxDimension || int64(metadata.Width)*int64(metadata.Height) > 64000000 {
		return theaterMediaMetadata{}, errors.New(TheaterMediaErrorLimitExceeded + ": dimensions")
	}
	if metadata.Kind == "animated_image" {
		if metadata.FrameCount <= 1 || metadata.FrameCount > config.MaxAnimatedFrames || metadata.DurationMS > config.MaxAnimatedDurationMS || int64(metadata.Width)*int64(metadata.Height)*int64(metadata.FrameCount) > config.MaxAnimatedPixelFrames {
			return theaterMediaMetadata{}, errors.New(TheaterMediaErrorLimitExceeded + ": animation")
		}
	}
	if metadata.Kind == "video" {
		if metadata.Width > config.VideoMaxWidth || metadata.Height > config.VideoMaxHeight || metadata.DurationMS > config.VideoMaxDurationMS || metadata.FrameRate > float64(config.VideoMaxFrameRate) {
			return theaterMediaMetadata{}, errors.New(TheaterMediaErrorLimitExceeded + ": video")
		}
	}
	return metadata, nil
}
