package service

import (
	"context"
	"os/exec"
	"strings"

	"sealchat/utils"
)

type MediaToolchain struct {
	FFmpegPath  string
	FFprobePath string
}

func ResolveMediaToolchain(audioConfig *utils.AudioConfig) MediaToolchain {
	if audioConfig == nil {
		audioConfig = &utils.AudioConfig{}
	}
	ffmpegPath, ffprobePath := resolveFFmpegPathsLegacy(audioConfig)
	return MediaToolchain{FFmpegPath: ffmpegPath, FFprobePath: ffprobePath}
}

func (toolchain MediaToolchain) FFmpegAvailable() bool {
	return strings.TrimSpace(toolchain.FFmpegPath) != ""
}

func (toolchain MediaToolchain) FFprobeAvailable() bool {
	return strings.TrimSpace(toolchain.FFprobePath) != ""
}

type MediaCommandRunner interface {
	Run(ctx context.Context, path string, args ...string) ([]byte, error)
}

type execMediaCommandRunner struct{}

func (execMediaCommandRunner) Run(ctx context.Context, path string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, path, args...).CombinedOutput()
}
