package utils

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func EncodeImageToWebPWithCWebP(img image.Image, quality int) ([]byte, error) {
	if img == nil {
		return nil, errors.New("nil image")
	}
	quality = clampWebPQuality(quality)

	cwebpPath, err := resolveBundledWebPTool("cwebp")
	if err != nil {
		return nil, err
	}

	in, err := os.CreateTemp("", "sealchat-cwebp-*.png")
	if err != nil {
		return nil, err
	}
	inPath := in.Name()
	defer os.Remove(inPath)

	if err := png.Encode(in, img); err != nil {
		_ = in.Close()
		return nil, err
	}
	if err := in.Close(); err != nil {
		return nil, err
	}

	out, err := os.CreateTemp("", "sealchat-cwebp-*.webp")
	if err != nil {
		return nil, err
	}
	outPath := out.Name()
	_ = out.Close()
	defer os.Remove(outPath)

	args := []string{
		"-quiet",
		"-metadata", "none",
		"-q", strconv.Itoa(quality),
		"-alpha_q", "100",
		inPath,
		"-o", outPath,
	}

	var stderr bytes.Buffer
	cmd := exec.CommandContext(context.Background(), cwebpPath, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return nil, fmt.Errorf("cwebp failed: %w", err)
		}
		return nil, fmt.Errorf("cwebp failed: %w: %s", err, msg)
	}

	return os.ReadFile(outPath)
}

func EncodeGIFToWebPWithGIF2WebP(gifData []byte, quality int) ([]byte, error) {
	if len(gifData) == 0 {
		return nil, errors.New("empty gif data")
	}
	quality = clampWebPQuality(quality)

	gif2webpPath, err := resolveBundledWebPTool("gif2webp")
	if err != nil {
		return nil, err
	}

	in, err := os.CreateTemp("", "sealchat-gif2webp-*.gif")
	if err != nil {
		return nil, err
	}
	inPath := in.Name()
	defer os.Remove(inPath)

	if _, err := in.Write(gifData); err != nil {
		_ = in.Close()
		return nil, err
	}
	if err := in.Close(); err != nil {
		return nil, err
	}

	out, err := os.CreateTemp("", "sealchat-gif2webp-*.webp")
	if err != nil {
		return nil, err
	}
	outPath := out.Name()
	_ = out.Close()
	defer os.Remove(outPath)

	args := []string{
		"-quiet",
		"-metadata", "none",
		"-q", strconv.Itoa(quality),
		inPath,
		"-o", outPath,
	}

	var stderr bytes.Buffer
	cmd := exec.CommandContext(context.Background(), gif2webpPath, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return nil, fmt.Errorf("gif2webp failed: %w", err)
		}
		return nil, fmt.Errorf("gif2webp failed: %w: %s", err, msg)
	}

	return os.ReadFile(outPath)
}

func resolveBundledWebPTool(tool string) (string, error) {
	name := strings.TrimSpace(tool)
	if name == "" || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("invalid tool name: %q", tool)
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		name += ".exe"
	}

	platformDir, err := bundledWebPPlatformDir()
	if err != nil {
		return "", err
	}

	roots := make([]string, 0, 3)
	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		roots = append(roots, cwd)
	}
	if exe, err := os.Executable(); err == nil && strings.TrimSpace(exe) != "" {
		exeDir := filepath.Dir(exe)
		roots = append(roots, exeDir)
		parent := filepath.Dir(exeDir)
		if parent != exeDir {
			roots = append(roots, parent)
		}
	}

	seen := map[string]struct{}{}
	var tried []string
	for _, root := range roots {
		root = filepath.Clean(root)
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}

		candidate := filepath.Join(root, "bin", platformDir, name)
		tried = append(tried, candidate)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("webp encoder tool not found, tried: %s", strings.Join(tried, ", "))
}

func bundledWebPPlatformDir() (string, error) {
	switch runtime.GOOS {
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			return "linux-x64", nil
		case "arm64":
			return "linux-arm64", nil
		default:
			return "", fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
		}
	case "windows":
		switch runtime.GOARCH {
		case "amd64":
			return "win-x64", nil
		default:
			return "", fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
		}
	default:
		return "", fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func clampWebPQuality(val int) int {
	switch {
	case val < 1:
		return 85
	case val > 100:
		return 100
	default:
		return val
	}
}
