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
	"time"
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

	return "", fmt.Errorf("webp encoder tool %q not found for %s/%s (dir=%s), tried: %s", name, runtime.GOOS, runtime.GOARCH, platformDir, strings.Join(tried, ", "))
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

func VerifyBundledWebPTools() error {
	return VerifyBundledWebPToolsWithLog(nil)
}

func VerifyBundledWebPToolsWithLog(logf func(format string, args ...any)) error {
	tools := []string{"cwebp", "gif2webp"}
	var errs []string
	for _, tool := range tools {
		if err := verifyBundledWebPTool(tool, logf); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("webp tools sanity check failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

func verifyBundledWebPTool(tool string, logf func(format string, args ...any)) error {
	if logf != nil {
		logf("WebP 工具自检：开始检查 %s", tool)
	}
	toolPath, err := resolveBundledWebPTool(tool)
	if err != nil {
		cwd, _ := os.Getwd()
		exe, _ := os.Executable()
		return fmt.Errorf("resolve %s failed: %w (goos=%s goarch=%s cwd=%q exe=%q)", tool, err, runtime.GOOS, runtime.GOARCH, cwd, exe)
	}
	if logf != nil {
		logf("WebP 工具自检：%s 路径=%q", tool, toolPath)
	}

	info, err := os.Stat(toolPath)
	if err != nil {
		return fmt.Errorf("%s not accessible: %w (path=%q)", tool, err, toolPath)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory, expected executable file (path=%q)", tool, toolPath)
	}
	if logf != nil {
		logf("WebP 工具自检：%s 文件权限=%s 大小=%dB", tool, info.Mode().String(), info.Size())
	}

	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		return fmt.Errorf("%s is not executable (mode=%s path=%q), try chmod +x", tool, info.Mode().String(), toolPath)
	}

	// 试调用一次，尽量选择不会依赖输入文件的参数；不同版本可能对参数支持略有差异，做一次兜底。
	out1, err1 := runExternalToolOnce(toolPath, []string{"-version"}, 5*time.Second)
	if err1 == nil {
		if logf != nil {
			logf("WebP 工具自检：%s -version 成功，输出=%q", tool, truncateForLog(out1, 512))
			logf("WebP 工具自检：%s 检查通过", tool)
		}
		return nil
	}
	if logf != nil {
		logf("WebP 工具自检：%s -version 失败：%v，输出=%q", tool, err1, truncateForLog(out1, 512))
	}
	out2, err2 := runExternalToolOnce(toolPath, []string{"-h"}, 5*time.Second)
	if err2 == nil {
		if logf != nil {
			logf("WebP 工具自检：%s -h 成功，输出=%q", tool, truncateForLog(out2, 512))
			logf("WebP 工具自检：%s 检查通过", tool)
		}
		return nil
	}
	if logf != nil {
		logf("WebP 工具自检：%s -h 失败：%v，输出=%q", tool, err2, truncateForLog(out2, 512))
	}

	return fmt.Errorf(
		"%s sanity check failed (path=%q mode=%s): attempt1(-version)=%v output=%q; attempt2(-h)=%v output=%q",
		tool,
		toolPath,
		info.Mode().String(),
		err1,
		truncateForLog(out1, 4096),
		err2,
		truncateForLog(out2, 4096),
	)
}

func runExternalToolOnce(path string, args []string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, args...)
	out, err := cmd.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		if msg == "" {
			return "", fmt.Errorf("timeout after %s", timeout)
		}
		return msg, fmt.Errorf("timeout after %s: %s", timeout, msg)
	}
	return msg, err
}

func truncateForLog(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
