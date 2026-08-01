package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sealchat/utils"
)

const (
	VisualMediaOutputDisplay   = "display"
	VisualMediaOutputFallback  = "fallback"
	VisualMediaOutputPoster    = "poster"
	VisualMediaOutputThumbnail = "thumbnail"

	TheaterAppearanceErrorAudioNotAllowed      = "AUDIO_TRACK_NOT_ALLOWED"
	TheaterAppearanceErrorAlphaRequired        = "ALPHA_REQUIRED"
	TheaterAppearanceErrorCodecUnsupported     = "VIDEO_CODEC_UNSUPPORTED"
	TheaterAppearanceErrorProcessorUnavailable = "PROCESSOR_UNAVAILABLE"
	TheaterAppearanceErrorProcessingTimeout    = "PROCESSING_TIMEOUT"
)

// VisualMediaProcessor probes and transforms media without knowing how results are stored.
type VisualMediaProcessor struct {
	config    utils.TheaterMediaConfig
	toolchain MediaToolchain
	runner    MediaCommandRunner
}

type VisualMediaOutput struct {
	Name       string
	Path       string
	MimeType   string
	Width      int
	Height     int
	DurationMS int64
	IsSource   bool
}

type VisualMediaProcessResult struct {
	Metadata theaterMediaMetadata
	Outputs  []VisualMediaOutput
	cleanup  func()
}

func (result *VisualMediaProcessResult) Cleanup() {
	if result != nil && result.cleanup != nil {
		result.cleanup()
		result.cleanup = nil
	}
}

func NewVisualMediaProcessor(config utils.TheaterMediaConfig, toolchain MediaToolchain, runner MediaCommandRunner) *VisualMediaProcessor {
	if runner == nil {
		runner = execMediaCommandRunner{}
	}
	return &VisualMediaProcessor{config: normalizeTheaterMediaConfig(config), toolchain: toolchain, runner: runner}
}

func (processor *VisualMediaProcessor) Probe(ctx context.Context, sourcePath, kind, mimeType string) (theaterMediaMetadata, error) {
	return probeTheaterMedia(ctx, sourcePath, kind, mimeType, processor.config, processor.toolchain, processor.runner)
}

// ProcessTheaterResource preserves the room-scoped resource variant behavior.
func (processor *VisualMediaProcessor) ProcessTheaterResource(ctx context.Context, sourcePath, kind, mimeType string) (*VisualMediaProcessResult, error) {
	metadata, err := processor.Probe(ctx, sourcePath, kind, mimeType)
	if err != nil {
		return nil, err
	}
	result := &VisualMediaProcessResult{Metadata: metadata}
	switch metadata.Kind {
	case "static_image":
		return result, nil
	case "animated_image":
		if canUseOriginalAnimatedWebM(mimeType, metadata) {
			result.Outputs = append(result.Outputs, visualSourceOutput(VisualMediaOutputDisplay, sourcePath, mimeType, metadata))
			return result, nil
		}
		if !processor.toolchain.FFmpegAvailable() {
			// Browsers and the Konva renderer can play GIF, Animated WebP and APNG
			// directly. Keep the validated source usable when optional transcoding is
			// unavailable instead of rejecting every animated stage resource.
			result.Outputs = append(result.Outputs, visualSourceOutput(VisualMediaOutputDisplay, sourcePath, mimeType, metadata))
			return result, nil
		}
		tempDir, err := os.MkdirTemp("", "sealchat-theater-animation-*")
		if err != nil {
			return nil, err
		}
		result.cleanup = func() { _ = os.RemoveAll(tempDir) }
		output, err := processor.transcodeTransparentWebM(ctx, sourcePath, filepath.Join(tempDir, "display.webm"), metadata, false)
		if err != nil {
			result.Cleanup()
			return nil, err
		}
		result.Outputs = append(result.Outputs, output)
		return result, nil
	case "video":
		if !processor.toolchain.FFmpegAvailable() {
			return nil, errors.New(TheaterMediaErrorProcessorUnavailable + ": ffmpeg unavailable")
		}
		tempDir, err := os.MkdirTemp("", "sealchat-theater-video-*")
		if err != nil {
			return nil, err
		}
		result.cleanup = func() { _ = os.RemoveAll(tempDir) }
		outputs, err := processor.transcodeTheaterVideo(ctx, sourcePath, tempDir, metadata)
		if err != nil {
			result.Cleanup()
			return nil, err
		}
		result.Outputs = outputs
		return result, nil
	default:
		return nil, errors.New(TheaterMediaErrorUnsupported)
	}
}

// ProcessAppearance stores static images as-is and converts supported animations for playback.
func (processor *VisualMediaProcessor) ProcessAppearance(ctx context.Context, sourcePath, kind, mimeType string) (*VisualMediaProcessResult, error) {
	config := processor.config
	config.MaxDimension = minPositive(config.MaxDimension, 4096)
	config.VideoMaxWidth = minPositive(config.VideoMaxWidth, 4096)
	config.VideoMaxHeight = minPositive(config.VideoMaxHeight, 4096)
	config.MaxAnimatedDurationMS = minPositiveInt64(config.MaxAnimatedDurationMS, 60_000)
	config.VideoMaxDurationMS = minPositiveInt64(config.VideoMaxDurationMS, 60_000)
	appearanceProcessor := *processor
	appearanceProcessor.config = config
	metadata, err := appearanceProcessor.Probe(ctx, sourcePath, kind, mimeType)
	if err != nil {
		return nil, err
	}
	result := &VisualMediaProcessResult{Metadata: metadata}
	if metadata.Kind == "static_image" {
		if mimeType != "image/png" && mimeType != "image/jpeg" && mimeType != "image/webp" {
			return nil, errors.New(TheaterMediaErrorUnsupported + ": appearance static image must be PNG, JPEG, or WebP")
		}
		result.Outputs = []VisualMediaOutput{visualSourceOutput(VisualMediaOutputDisplay, sourcePath, mimeType, metadata)}
		return result, nil
	}
	if metadata.Kind != "animated_image" || mimeType != "video/webm" && !strings.HasPrefix(mimeType, "image/") {
		return nil, errors.New(TheaterMediaErrorUnsupported + ": appearance video unsupported")
	}
	if metadata.AudioCodec != "" {
		return nil, errors.New(TheaterAppearanceErrorAudioNotAllowed)
	}
	if mimeType == "video/webm" {
		if metadata.VideoCodec != "vp8" && metadata.VideoCodec != "vp9" {
			return nil, errors.New(TheaterAppearanceErrorCodecUnsupported)
		}
		if !metadata.HasAlpha {
			return nil, errors.New(TheaterAppearanceErrorAlphaRequired)
		}
	}
	if !processor.toolchain.FFmpegAvailable() {
		return nil, errors.New(TheaterAppearanceErrorProcessorUnavailable + ": ffmpeg unavailable")
	}
	tempDir, err := os.MkdirTemp("", "sealchat-appearance-animation-*")
	if err != nil {
		return nil, err
	}
	result.cleanup = func() { _ = os.RemoveAll(tempDir) }
	if canUseOriginalAnimatedWebM(mimeType, metadata) && metadata.Width <= 1920 && metadata.Height <= 1920 {
		result.Outputs = append(result.Outputs, visualSourceOutput(VisualMediaOutputDisplay, sourcePath, mimeType, metadata))
	} else {
		output, err := processor.transcodeTransparentWebM(ctx, sourcePath, filepath.Join(tempDir, "display.webm"), metadata, true)
		if err != nil {
			result.Cleanup()
			return nil, err
		}
		result.Outputs = append(result.Outputs, output)
	}
	fallback, err := processor.extractAppearanceFallback(ctx, sourcePath, filepath.Join(tempDir, "fallback.webp"), metadata)
	if err != nil {
		result.Cleanup()
		return nil, err
	}
	result.Outputs = append(result.Outputs, fallback)
	return result, nil
}

func (processor *VisualMediaProcessor) transcodeTransparentWebM(ctx context.Context, sourcePath, outputPath string, metadata theaterMediaMetadata, fitBothDimensions bool) (VisualMediaOutput, error) {
	transcodeCtx, cancel := context.WithTimeout(ctx, time.Duration(processor.config.TranscodeTimeoutSeconds)*time.Second)
	defer cancel()
	filter := "scale=min(1920\\,iw):-2:flags=lanczos,format=yuva420p"
	width, height := scaledTheaterDimensions(metadata.Width, metadata.Height, 1920)
	if fitBothDimensions {
		filter = "scale='min(1920,iw)':'min(1920,ih)':force_original_aspect_ratio=decrease:flags=lanczos,format=yuva420p"
		width, height = scaledVisualDimensions(metadata.Width, metadata.Height, 1920)
	}
	inputArgs := []string{"-y"}
	if metadata.MimeType == "image/gif" {
		inputArgs = append(inputArgs, "-ignore_loop", "1")
	}
	inputArgs = append(inputArgs, "-i", sourcePath, "-map", "0:v:0", "-an",
		"-vf", filter,
		"-c:v", "libvpx-vp9", "-deadline", "good", "-cpu-used", "4", "-crf", "30", "-b:v", "0",
		"-row-mt", "1", "-auto-alt-ref", "0", "-metadata:s:v:0", "alpha_mode=1", outputPath)
	output, err := processor.runner.Run(transcodeCtx, processor.toolchain.FFmpegPath, inputArgs...)
	if err != nil && metadata.MimeType == "image/webp" && transcodeCtx.Err() == nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		concatPath, decodeErr := decodeAnimatedWebPFrames(transcodeCtx, sourcePath, filepath.Dir(outputPath), metadata)
		if decodeErr != nil {
			return VisualMediaOutput{}, fmt.Errorf("animated display: %s; Animated WebP fallback: %w", truncateTheaterBroadcastError(string(output)), decodeErr)
		}
		output, err = processor.runner.Run(transcodeCtx, processor.toolchain.FFmpegPath,
			"-y", "-f", "concat", "-safe", "0", "-i", concatPath, "-map", "0:v:0", "-an",
			"-vf", filter, "-vsync", "vfr",
			"-c:v", "libvpx-vp9", "-deadline", "good", "-cpu-used", "4", "-crf", "30", "-b:v", "0",
			"-row-mt", "1", "-auto-alt-ref", "0", "-metadata:s:v:0", "alpha_mode=1", outputPath,
		)
	}
	if err != nil {
		return VisualMediaOutput{}, fmt.Errorf("animated display: %s: %w", truncateTheaterBroadcastError(string(output)), err)
	}
	return VisualMediaOutput{Name: VisualMediaOutputDisplay, Path: outputPath, MimeType: "video/webm", Width: width, Height: height, DurationMS: metadata.DurationMS}, nil
}

func (processor *VisualMediaProcessor) extractAppearanceFallback(ctx context.Context, sourcePath, outputPath string, metadata theaterMediaMetadata) (VisualMediaOutput, error) {
	transcodeCtx, cancel := context.WithTimeout(ctx, time.Duration(processor.config.TranscodeTimeoutSeconds)*time.Second)
	defer cancel()
	output, err := processor.runner.Run(transcodeCtx, processor.toolchain.FFmpegPath,
		"-y", "-i", sourcePath, "-map", "0:v:0", "-frames:v", "1",
		"-vf", "scale='min(1920,iw)':'min(1920,ih)':force_original_aspect_ratio=decrease:flags=lanczos",
		"-c:v", "libwebp", "-lossless", "1", outputPath,
	)
	if err != nil {
		return VisualMediaOutput{}, fmt.Errorf("animated fallback: %s: %w", truncateTheaterBroadcastError(string(output)), err)
	}
	width, height := scaledVisualDimensions(metadata.Width, metadata.Height, 1920)
	return VisualMediaOutput{Name: VisualMediaOutputFallback, Path: outputPath, MimeType: "image/webp", Width: width, Height: height}, nil
}

func (processor *VisualMediaProcessor) transcodeTheaterVideo(ctx context.Context, sourcePath, tempDir string, metadata theaterMediaMetadata) ([]VisualMediaOutput, error) {
	timeout := time.Duration(processor.config.TranscodeTimeoutSeconds) * time.Second
	transcodeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	displayPath, displayMIME, err := transcodeTheaterDisplay(transcodeCtx, sourcePath, tempDir, processor.toolchain, processor.runner)
	if err != nil {
		return nil, err
	}
	posterPath := filepath.Join(tempDir, "poster.jpg")
	if output, err := processor.runner.Run(transcodeCtx, processor.toolchain.FFmpegPath, "-y", "-ss", "0", "-i", sourcePath, "-frames:v", "1", "-vf", "scale=min(1920\\,iw):-2", "-q:v", "2", posterPath); err != nil {
		return nil, fmt.Errorf("poster: %s: %w", truncateTheaterBroadcastError(string(output)), err)
	}
	thumbnailPath := filepath.Join(tempDir, "thumbnail.jpg")
	if output, err := processor.runner.Run(transcodeCtx, processor.toolchain.FFmpegPath, "-y", "-ss", "0", "-i", sourcePath, "-frames:v", "1", "-vf", "scale=min(480\\,iw):-2", "-q:v", "4", thumbnailPath); err != nil {
		return nil, fmt.Errorf("thumbnail: %s: %w", truncateTheaterBroadcastError(string(output)), err)
	}
	displayWidth, displayHeight := scaledTheaterDimensions(metadata.Width, metadata.Height, 1920)
	thumbWidth, thumbHeight := scaledTheaterDimensions(metadata.Width, metadata.Height, 480)
	return []VisualMediaOutput{
		{Name: VisualMediaOutputDisplay, Path: displayPath, MimeType: displayMIME, Width: displayWidth, Height: displayHeight, DurationMS: metadata.DurationMS},
		{Name: VisualMediaOutputPoster, Path: posterPath, MimeType: "image/jpeg", Width: displayWidth, Height: displayHeight},
		{Name: VisualMediaOutputThumbnail, Path: thumbnailPath, MimeType: "image/jpeg", Width: thumbWidth, Height: thumbHeight},
	}, nil
}

func visualSourceOutput(name, sourcePath, mimeType string, metadata theaterMediaMetadata) VisualMediaOutput {
	return VisualMediaOutput{Name: name, Path: sourcePath, MimeType: mimeType, Width: metadata.Width, Height: metadata.Height, DurationMS: metadata.DurationMS, IsSource: true}
}

func canUseOriginalAnimatedWebM(mimeType string, metadata theaterMediaMetadata) bool {
	return mimeType == "video/webm" && metadata.AudioCodec == "" && (metadata.VideoCodec == "vp8" || metadata.VideoCodec == "vp9")
}

func scaledVisualDimensions(width, height, maxDimension int) (int, int) {
	if width <= 0 || height <= 0 || maxDimension <= 0 || width <= maxDimension && height <= maxDimension {
		return width, height
	}
	scale := float64(maxDimension) / float64(width)
	if height > width {
		scale = float64(maxDimension) / float64(height)
	}
	return max(1, int(float64(width)*scale)), max(1, int(float64(height)*scale))
}

func minPositive(value, limit int) int {
	if value <= 0 || value > limit {
		return limit
	}
	return value
}

func minPositiveInt64(value, limit int64) int64 {
	if value <= 0 || value > limit {
		return limit
	}
	return value
}
