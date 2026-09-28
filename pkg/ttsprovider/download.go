package ttsprovider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrAudioURL = errors.New("tts audio url is not a trusted provider result")

// DownloadAudio fetches the complete provider file named by the terminal SSE
// event. The URL is pre-signed, so no credential, cookie or redirect is used,
// and only aliyuncs.com hosts are accepted to prevent SSRF. Errors never embed
// the URL because its query carries the provider signature.
func (c *Client) DownloadAudio(ctx context.Context, rawURL string, dst io.Writer) (int64, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Opaque != "" || u.User != nil || strings.Contains(rawURL, "#") {
		return 0, ErrAudioURL
	}
	host := strings.ToLower(u.Hostname())
	if host != "aliyuncs.com" && !strings.HasSuffix(host, ".aliyuncs.com") {
		return 0, ErrAudioURL
	}
	// Keep the exact signed spelling; re-encoding may invalidate the signature.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, ErrAudioURL
	}
	h := &http.Client{Timeout: 90 * time.Second}
	if c.HTTP != nil {
		*h = *c.HTTP
	}
	h.Jar = nil
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := h.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return 0, fmt.Errorf("tts audio download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return 0, fmt.Errorf("tts audio download: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > MaxAudioBytes {
		return 0, errors.New("tts audio exceeds spool limit")
	}
	n, err := io.Copy(dst, io.LimitReader(resp.Body, MaxAudioBytes+1))
	if err != nil {
		return n, fmt.Errorf("tts audio download: %w", err)
	}
	if n > MaxAudioBytes {
		return n, errors.New("tts audio exceeds spool limit")
	}
	return n, nil
}
