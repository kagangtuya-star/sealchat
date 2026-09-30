// Package ttsprovider implements the Qwen-Audio-TTS provider contracts.
package ttsprovider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const MaxAudioBytes = 16 << 20

var ErrIncomplete = errors.New("tts stream has no successful terminal event")

type Input struct {
	Text        string  `json:"text"`
	Voice       string  `json:"voice"`
	Format      string  `json:"format"`
	SampleRate  int     `json:"sample_rate"`
	BitRate     int     `json:"bit_rate,omitempty"`
	Instruction string  `json:"instruction,omitempty"`
	Rate        float64 `json:"rate"`
	Pitch       float64 `json:"pitch"`
	Volume      int     `json:"volume"`
	Seed        int     `json:"seed"`
}

type Result struct {
	RequestID           string
	Characters          int64
	InputTokens         *int64
	OutputTokens        *int64
	TokenUsageConfirmed bool
	UsageConfirmed      bool
	Complete            bool
	AudioURL            string
	Bytes               int64
}

type Client struct {
	HTTP              *http.Client
	SynthesisEndpoint string
	VoiceEndpoint     string
	APIKey            string
}

type ProviderError struct {
	Status    int
	Code      string
	RequestID string
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("tts provider error (%d, %s)", e.Status, e.Code)
}

func (c *Client) post(ctx context.Context, endpoint string, body any, sse bool) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	if sse {
		req.Header.Set("X-DashScope-SSE", "enable")
		req.Header.Set("Accept", "text/event-stream")
	}
	h := &http.Client{Timeout: 90 * time.Second}
	if c.HTTP != nil {
		*h = *c.HTTP
	}
	// Never forward credentials or repeat a charge through an HTTP redirect.
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return h.Do(req)
}

func (c *Client) Synthesize(ctx context.Context, model string, input Input, sink io.Writer) (Result, error) {
	resp, err := c.post(ctx, c.SynthesisEndpoint, struct {
		Model string `json:"model"`
		Input Input  `json:"input"`
	}{model, input}, true)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}, readProviderError(resp)
	}
	return ReadSSE(resp.Body, sink)
}

func readProviderError(resp *http.Response) error {
	var v struct {
		Code      string `json:"code"`
		RequestID string `json:"request_id"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&v)
	return &ProviderError{Status: resp.StatusCode, Code: v.Code, RequestID: v.RequestID}
}

// ReadSSE joins data lines independently of HTTP read boundaries. Audio data is
// a single ordered byte stream, not independently decodable SSE packets.
func ReadSSE(src io.Reader, sink io.Writer) (Result, error) {
	var result Result
	usageSeen := false
	scanner := bufio.NewScanner(io.LimitReader(src, 32<<20))
	scanner.Buffer(make([]byte, 4096), 24<<20)
	var data strings.Builder
	event := ""
	consume := func() error {
		if data.Len() == 0 {
			event = ""
			return nil
		}
		var v struct {
			RequestID string `json:"request_id"`
			Code      string `json:"code"`
			Output    struct {
				FinishReason string `json:"finish_reason"`
				Audio        struct {
					Data string `json:"data"`
					URL  string `json:"url"`
				} `json:"audio"`
			} `json:"output"`
			Usage *struct {
				Characters   *int64 `json:"characters"`
				InputTokens  *int64 `json:"input_tokens"`
				OutputTokens *int64 `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data.String()), &v); err != nil {
			return err
		}
		data.Reset()
		if v.RequestID != "" {
			result.RequestID = v.RequestID
		}
		if v.Usage != nil {
			if v.Usage.Characters != nil && *v.Usage.Characters >= 0 {
				usageSeen = true
				if *v.Usage.Characters > result.Characters {
					result.Characters = *v.Usage.Characters
				}
			}
			updateTokenUsage(&result.InputTokens, v.Usage.InputTokens)
			updateTokenUsage(&result.OutputTokens, v.Usage.OutputTokens)
		}
		if event == "error" || v.Code != "" {
			return &ProviderError{Code: v.Code, RequestID: result.RequestID}
		}
		if v.Output.FinishReason == "stop" {
			result.UsageConfirmed = usageSeen
			result.TokenUsageConfirmed = result.InputTokens != nil && result.OutputTokens != nil
		}
		event = ""
		if result.Complete {
			return errors.New("tts data after terminal event")
		}
		if v.Output.Audio.Data != "" {
			b, err := base64.StdEncoding.DecodeString(v.Output.Audio.Data)
			if err != nil {
				return err
			}
			if result.Bytes+int64(len(b)) > MaxAudioBytes {
				return errors.New("tts audio exceeds spool limit")
			}
			n, err := sink.Write(b)
			result.Bytes += int64(n)
			if err != nil {
				return err
			}
			if n != len(b) {
				return io.ErrShortWrite
			}
		}
		if v.Output.Audio.URL != "" {
			result.AudioURL = v.Output.Audio.URL
		}
		if v.Output.FinishReason == "stop" {
			result.Complete = true
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := consume(); err != nil {
				return result, err
			}
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			value = ""
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			event = value
		case "data":
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	// An unterminated SSE event is not dispatched at EOF.
	if !result.Complete || data.Len() > 0 {
		return result, ErrIncomplete
	}
	return result, nil
}

func updateTokenUsage(current **int64, value *int64) {
	if value != nil && *value >= 0 && (*current == nil || *value > **current) {
		copyValue := *value
		*current = &copyValue
	}
}

type CreateVoiceInput struct {
	Action        string   `json:"action"`
	TargetModel   string   `json:"target_model"`
	Prefix        string   `json:"prefix"`
	VoicePrompt   string   `json:"voice_prompt,omitempty"`
	PreviewText   string   `json:"preview_text,omitempty"`
	URL           string   `json:"url,omitempty"`
	LanguageHints []string `json:"language_hints,omitempty"`
}
type Voice struct {
	VoiceID      string `json:"voice_id"`
	TargetModel  string `json:"target_model"`
	Status       string `json:"status"`
	PreviewAudio *struct {
		Data       string `json:"data"`
		SampleRate int    `json:"sample_rate"`
		Format     string `json:"response_format"`
	} `json:"preview_audio,omitempty"`
}
type VoiceResult struct {
	RequestID string `json:"request_id"`
	Output    struct {
		Voice
		VoiceList []Voice `json:"voice_list"`
	} `json:"output"`
	Usage struct {
		Count *int64 `json:"count"`
	} `json:"usage"`
	Code string `json:"code"`
}

func (c *Client) voice(ctx context.Context, input any, preview bool) (VoiceResult, error) {
	body := map[string]any{"model": "voice-enrollment", "input": input}
	if preview {
		body["parameters"] = map[string]any{"sample_rate": 24000, "response_format": "wav"}
	}
	resp, err := c.post(ctx, c.VoiceEndpoint, body, false)
	if err != nil {
		return VoiceResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return VoiceResult{}, readProviderError(resp)
	}
	var out VoiceResult
	if err = json.NewDecoder(io.LimitReader(resp.Body, 24<<20)).Decode(&out); err != nil {
		return out, err
	}
	if out.Code != "" {
		return out, &ProviderError{Code: out.Code, RequestID: out.RequestID}
	}
	return out, nil
}
func (c *Client) CreateVoice(ctx context.Context, input CreateVoiceInput) (VoiceResult, error) {
	input.Action = "create_voice"
	return c.voice(ctx, input, input.VoicePrompt != "")
}
func (c *Client) QueryVoice(ctx context.Context, id string) (VoiceResult, error) {
	return c.voice(ctx, map[string]any{"action": "query_voice", "voice_id": id}, false)
}
func (c *Client) ListVoice(ctx context.Context, prefix string, page, size int) (VoiceResult, error) {
	return c.voice(ctx, map[string]any{"action": "list_voice", "prefix": prefix, "page_index": page, "page_size": size}, false)
}
func (c *Client) DeleteVoice(ctx context.Context, id string) (VoiceResult, error) {
	return c.voice(ctx, map[string]any{"action": "delete_voice", "voice_id": id}, false)
}
