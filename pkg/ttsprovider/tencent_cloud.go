package ttsprovider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type TencentCloudClient struct {
	SecretID  string
	SecretKey string
	HTTP      *http.Client
	Now       func() time.Time
}

func tencentHash(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func tencentHMAC(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(value))
	return h.Sum(nil)
}

// post signs exactly the bytes sent, with a lower-case canonical action and no Region.
// There are no retries, including on redirect or ambiguous transport failures.
func (c *TencentCloudClient) post(ctx context.Context, endpoint, service, action, version string, body any) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, errors.New("腾讯云请求参数无效")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil || req.URL.Host == "" || req.URL.User != nil || req.URL.RawQuery != "" || req.URL.Fragment != "" || (req.URL.Scheme != "https" && req.URL.Scheme != "http") || (req.URL.Path != "" && req.URL.Path != "/") {
		return nil, errors.New("腾讯云接口地址无效")
	}
	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	date, timestamp := now.UTC().Format("2006-01-02"), strconv.FormatInt(now.Unix(), 10)
	contentType, signedHeaders := "application/json; charset=utf-8", "content-type;host;x-tc-action"
	canonicalHeaders := "content-type:" + contentType + "\nhost:" + req.URL.Host + "\nx-tc-action:" + strings.ToLower(action) + "\n"
	canonicalRequest := "POST\n/\n\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + tencentHash(data)
	scope := date + "/" + service + "/tc3_request"
	stringToSign := "TC3-HMAC-SHA256\n" + timestamp + "\n" + scope + "\n" + tencentHash([]byte(canonicalRequest))
	dateKey := tencentHMAC([]byte("TC3"+c.SecretKey), date)
	serviceKey := tencentHMAC(dateKey, service)
	signingKey := tencentHMAC(serviceKey, "tc3_request")
	signature := hex.EncodeToString(tencentHMAC(signingKey, stringToSign))
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", version)
	req.Header.Set("X-TC-Timestamp", timestamp)
	req.Header.Set("Authorization", "TC3-HMAC-SHA256 Credential="+c.SecretID+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
	h := &http.Client{Timeout: 90 * time.Second}
	if c.HTTP != nil {
		*h = *c.HTTP
	}
	if h.Timeout <= 0 || h.Timeout > 90*time.Second {
		h.Timeout = 90 * time.Second
	}
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := h.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("无法连接腾讯云语音服务；不会自动重试合成")
	}
	return resp, nil
}

// Post unwraps a bounded Cloud API response. Vendor messages and signing material
// never enter errors; only bounded machine codes and request identifiers survive.
func (c *TencentCloudClient) Post(ctx context.Context, endpoint, service, action, version string, body, out any) (string, error) {
	resp, err := c.post(ctx, endpoint, service, action, version, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	limit := int64(base64.StdEncoding.EncodedLen(MaxAudioBytes) + (64 << 10))
	if action == "DescribeVoices" {
		limit = 4 << 20
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return "", errors.New("腾讯云响应读取失败")
	}
	if int64(len(raw)) > limit {
		return "", errors.New("腾讯云响应超过大小限制")
	}
	var envelope struct{ Response json.RawMessage }
	if json.Unmarshal(raw, &envelope) != nil || len(envelope.Response) == 0 {
		return "", errors.New("腾讯云返回了无法解析的数据")
	}
	var meta struct {
		RequestID string                 `json:"RequestId"`
		Error     *struct{ Code string } `json:"Error"`
	}
	if json.Unmarshal(envelope.Response, &meta) != nil {
		return "", errors.New("腾讯云返回了无法解析的数据")
	}
	id := boundedTencentCode(meta.RequestID)
	if meta.Error != nil {
		return id, &ProviderError{Status: resp.StatusCode, Code: boundedTencentCode(meta.Error.Code), RequestID: id}
	}
	if resp.StatusCode != http.StatusOK {
		return id, &ProviderError{Status: resp.StatusCode, Code: "HTTPError", RequestID: id}
	}
	if json.Unmarshal(envelope.Response, out) != nil {
		return id, errors.New("腾讯云返回了无法解析的数据")
	}
	return id, nil
}

func boundedTencentCode(value string) string {
	if len(value) == 0 || len(value) > 128 {
		return ""
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.') {
			return ""
		}
	}
	return value
}
