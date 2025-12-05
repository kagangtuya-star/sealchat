package model

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dchest/captcha"
)

// CaptchaCreate 生成验证码，返回 captchaId
func CaptchaCreate() string {
	return captcha.New()
}

// CaptchaVerify 验证验证码，验证后自动删除
func CaptchaVerify(captchaId, userInput string) bool {
	if captchaId == "" || userInput == "" {
		return false
	}
	return captcha.VerifyString(captchaId, userInput)
}

// CaptchaImage 返回验证码图片 (PNG)
func CaptchaImage(captchaId string, width, height int) ([]byte, error) {
	if width <= 0 {
		width = captcha.StdWidth
	}
	if height <= 0 {
		height = captcha.StdHeight
	}
	
	var buf []byte
	writer := &bufferWriter{buf: &buf}
	err := captcha.WriteImage(writer, captchaId, width, height)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// CaptchaReload 重新加载验证码数字
func CaptchaReload(captchaId string) bool {
	return captcha.Reload(captchaId)
}

// bufferWriter 简单的字节缓冲写入器
type bufferWriter struct {
	buf *[]byte
}

func (w *bufferWriter) Write(p []byte) (n int, err error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

// TurnstileVerifyResponse Cloudflare Turnstile 验证响应
type TurnstileVerifyResponse struct {
	Success     bool     `json:"success"`
	ChallengeTS string   `json:"challenge_ts"`
	Hostname    string   `json:"hostname"`
	ErrorCodes  []string `json:"error-codes"`
	Action      string   `json:"action"`
	CData       string   `json:"cdata"`
}

// TurnstileVerify 验证 Cloudflare Turnstile token
func TurnstileVerify(token, secretKey, remoteIP string) (bool, error) {
	if token == "" || secretKey == "" {
		return false, fmt.Errorf("token or secret key is empty")
	}

	formData := url.Values{}
	formData.Set("secret", secretKey)
	formData.Set("response", token)
	if remoteIP != "" {
		formData.Set("remoteip", remoteIP)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.PostForm(
		"https://challenges.cloudflare.com/turnstile/v0/siteverify",
		formData,
	)
	if err != nil {
		return false, fmt.Errorf("failed to verify turnstile: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	var result TurnstileVerifyResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return false, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success && len(result.ErrorCodes) > 0 {
		return false, fmt.Errorf("turnstile verification failed: %s", strings.Join(result.ErrorCodes, ", "))
	}

	return result.Success, nil
}
