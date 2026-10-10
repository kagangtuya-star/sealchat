package ttsprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

const TencentMPSEndpoint = "https://mps.tencentcloudapi.com"
const TencentMPSMaxVoices = 2500

// TencentMPSTextLanguages returns the synchronous TextLang codes, including hun.
func TencentMPSTextLanguages() []string {
	return []string{"zh", "en", "ja", "de", "fr", "ko", "ru", "uk", "pt", "it", "es", "id", "nl", "tr", "fil", "ms", "el", "fi", "hr", "sk", "pl", "sv", "hi", "bg", "ro", "ar", "cs", "da", "ta", "hun", "vi", "no", "yue", "th", "he", "ca", "nn", "af", "fa", "sl"}
}

type TencentMPSClient struct {
	SecretID  string
	SecretKey string
	Endpoint  string
	HTTP      *http.Client
	Now       func() time.Time
}

func (c *TencentMPSClient) post(ctx context.Context, action string, body, out any) (string, error) {
	if strings.TrimSpace(c.SecretID) == "" || strings.TrimSpace(c.SecretKey) == "" {
		return "", errors.New("请同时配置腾讯云 SecretId 与 SecretKey")
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = TencentMPSEndpoint
	}
	cloud := TencentCloudClient{SecretID: c.SecretID, SecretKey: c.SecretKey, HTTP: c.HTTP, Now: c.Now}
	return cloud.Post(ctx, endpoint, "mps", action, "2019-06-12", body, out)
}

type TencentMPSVoice struct {
	VoiceID     string `json:"VoiceId"`
	Name        string
	Category    string
	Description string
	Gender      string
	Age         string
	Languages   []string
	Labels      []string
	Scenes      []string
	AudioURL    string `json:"AudioUrl"`
}

func tencentMPSResponseError(code *int, requestID string) error {
	if code == nil || requestID == "" {
		return errors.New("腾讯 MPS 响应缺少 ErrorCode 或 RequestId")
	}
	if *code != 0 {
		return &ProviderError{Status: http.StatusOK, Code: "MPS_" + strconv.Itoa(*code), RequestID: requestID}
	}
	return nil
}

// DescribeVoices fetches only MiniMax system voices. Both raw rows and page count
// are bounded, even if TotalCount changes or the server repeats a page.
func (c *TencentMPSClient) DescribeVoices(ctx context.Context) ([]TencentMPSVoice, string, error) {
	voices := []TencentMPSVoice{}
	seen := map[string]bool{}
	count, requestID := 0, ""
	for page := 1; page <= TencentMPSMaxVoices/100; page++ {
		body := struct {
			VoiceType string
			PageNum   int
			PageSize  int
			ExtParam  string
		}{"system", page, 100, `{"engine":"minimax"}`}
		var out struct {
			ErrorCode  *int
			TotalCount int
			Voices     []TencentMPSVoice
		}
		id, err := c.post(ctx, "DescribeVoices", body, &out)
		if id != "" {
			requestID = id
		}
		if err != nil {
			return nil, requestID, err
		}
		if err = tencentMPSResponseError(out.ErrorCode, id); err != nil {
			return nil, requestID, err
		}
		for _, voice := range out.Voices {
			count++
			if voice.Category == "system" && voice.VoiceID != "" && !seen[voice.VoiceID] {
				seen[voice.VoiceID] = true
				voices = append(voices, voice)
			}
			if count >= TencentMPSMaxVoices {
				break
			}
		}
		if len(out.Voices) == 0 || count >= out.TotalCount || count >= TencentMPSMaxVoices {
			break
		}
	}
	return voices, requestID, nil
}

// TencentMPSVoiceSpecs maps dynamic data without touching the static catalog.
func TencentMPSVoiceSpecs(voices []TencentMPSVoice) []TTSVoiceSpec {
	items := []TTSVoiceSpec{}
	seen := map[string]bool{}
	for _, v := range voices {
		if v.Category != "system" || v.VoiceID == "" || seen[v.VoiceID] {
			continue
		}
		seen[v.VoiceID] = true
		name := v.Name
		if name == "" {
			name = v.VoiceID
		}
		languages := []string{}
		for _, language := range v.Languages {
			if slices.Contains(TencentMPSTextLanguages(), language) && !slices.Contains(languages, language) {
				languages = append(languages, language)
			}
		}
		tags := []string{}
		gender := map[string]string{"male": "男声", "female": "女声"}[v.Gender]
		age := map[string]string{"child": "儿童", "teenager": "少年", "youth": "青年", "middle_aged": "中年", "senior": "老年"}[v.Age]
		for _, tag := range append(append([]string{gender, age}, v.Labels...), v.Scenes...) {
			tag = strings.TrimSpace(tag)
			if tag == "" || slices.Contains(tags, tag) {
				continue
			}
			if _, err := strconv.ParseFloat(tag, 64); err == nil {
				continue
			}
			tags = append(tags, tag)
		}
		items = append(items, TTSVoiceSpec{ID: v.VoiceID, Name: name, ProviderKind: ProviderTencent, PresetSource: "tencent2", Models: TencentMPSModels(), Languages: languages, SpeechLanguages: slices.Clone(languages), Kind: "mps-system", Tags: strings.Join(tags, "、")})
	}
	return items
}

func TencentMPSPitch(pitch float64) int {
	return int(math.Max(-12, math.Min(12, math.Round(12*math.Log2(pitch)))))
}

func TencentMPSVolume(volume int) float64 {
	volume = max(0, min(100, volume))
	return math.Round(math.Pow(10, (float64(volume)-50)/50)*100) / 100
}

func ValidateTencentMPSInput(input Input) error {
	if input.Instruction != "" {
		return errors.New("腾讯 MPS MiniMax 当前不支持 SealChat 自由朗读指令")
	}
	if input.Language != "" && !slices.Contains(TencentMPSTextLanguages(), input.Language) {
		return errors.New("腾讯 MPS 不支持此朗读语言")
	}
	if input.Rate < 0.5 || input.Rate > 2 || math.IsNaN(input.Rate) || math.IsInf(input.Rate, 0) || input.Pitch < 0.5 || input.Pitch > 2 || math.IsNaN(input.Pitch) || math.IsInf(input.Pitch, 0) || input.Volume < 0 || input.Volume > 100 {
		return errors.New("语音参数超出范围")
	}
	if input.Format != "wav" && input.Format != "mp3" {
		return errors.New("腾讯 MPS 音频格式不受支持")
	}
	return nil
}

type tencentMPSSynthesisOptions struct {
	Model      string  `json:"model"`
	Format     string  `json:"format"`
	SampleRate int     `json:"sampleRate"`
	Speed      float64 `json:"speed"`
	Vol        float64 `json:"vol"`
	Pitch      int     `json:"pitch"`
}

func (c *TencentMPSClient) textToSpeech(ctx context.Context, model string, input Input) ([]byte, string, error) {
	spec, ok := LookupModel(ProviderTencent, model)
	if !ok || spec.Runtime != RuntimeTencentMPS {
		return nil, "", errors.New("腾讯 MPS MiniMax 模型不受支持")
	}
	if strings.TrimSpace(input.Voice) == "" || input.Text == "" {
		return nil, "", errors.New("腾讯 MPS 音色 ID 和文本不能为空")
	}
	if err := ValidateTencentMPSInput(input); err != nil {
		return nil, "", err
	}
	ext, _ := json.Marshal(struct {
		Engine string                     `json:"engine"`
		SynExt tencentMPSSynthesisOptions `json:"synExt"`
	}{Engine: "minimax", SynExt: tencentMPSSynthesisOptions{Model: model, Format: input.Format, SampleRate: 24000, Speed: math.Max(0.5, math.Min(2, input.Rate)), Vol: TencentMPSVolume(input.Volume), Pitch: TencentMPSPitch(input.Pitch)}})
	body := struct {
		Text     string
		VoiceID  string `json:"VoiceId"`
		TextLang string `json:"TextLang,omitempty"`
		ExtParam string
	}{input.Text, input.Voice, input.Language, string(ext)}
	var out struct {
		ErrorCode *int
		AudioData string
	}
	id, err := c.post(ctx, "TextToSpeech", body, &out)
	if err != nil {
		return nil, id, err
	}
	if err = tencentMPSResponseError(out.ErrorCode, id); err != nil {
		return nil, id, err
	}
	if out.AudioData == "" || len(out.AudioData) > base64.StdEncoding.EncodedLen(MaxAudioBytes) {
		return nil, id, errors.New("腾讯 MPS 音频为空或超过大小限制")
	}
	audio, err := base64.StdEncoding.DecodeString(out.AudioData)
	if err != nil || len(audio) == 0 || len(audio) > MaxAudioBytes {
		return nil, id, errors.New("腾讯 MPS AudioData 无效或超过大小限制")
	}
	// Media validity is part of successful synthesis, before usage confirmation.
	if input.Format == "wav" {
		_, err = ParsePCM16MonoWAV(audio, 24000)
	} else {
		var media Media
		media, err = InspectMedia(audio)
		if err == nil && media.Container != "mp3" {
			err = ErrMedia
		}
	}
	if err != nil {
		return nil, id, err
	}
	return audio, id, nil
}

func (c *TencentMPSClient) Synthesize(ctx context.Context, model string, input Input, sink io.Writer) (Result, error) {
	audio, id, err := c.textToSpeech(ctx, model, input)
	result := Result{RequestID: id}
	if err != nil {
		return result, err
	}
	return tencentMPSWriteAudio(ctx, result, audio, input.Text, sink)
}

func (c *TencentMPSClient) SynthesizeSegments(ctx context.Context, model string, input Input, segments []string, sink io.Writer) (Result, error) {
	if len(segments) < 2 || strings.Join(segments, "") != input.Text || slices.Contains(segments, "") {
		return Result{}, errors.New("tts segments do not match input text")
	}
	if input.Format == "mp3" {
		return Result{}, errors.New("腾讯 MPS 长文本当前仅支持 WAV 合成")
	}
	if err := ValidateTencentMPSInput(input); err != nil {
		return Result{}, err
	}
	var result Result
	var pcm []byte
	for _, segment := range segments {
		part := input
		part.Text = segment
		audio, id, err := c.textToSpeech(ctx, model, part)
		if id != "" {
			result.RequestID = id
		}
		if err != nil {
			return result, err
		}
		data, err := ParsePCM16MonoWAV(audio, 24000)
		if err != nil {
			return result, err
		}
		if len(pcm)+len(data)+44 > MaxAudioBytes {
			return result, errors.New("tts audio exceeds spool limit")
		}
		pcm = append(pcm, data...)
	}
	wav, err := EncodePCM16MonoWAV(pcm, 24000)
	if err != nil {
		return result, err
	}
	return tencentMPSWriteAudio(ctx, result, wav, input.Text, sink)
}

func tencentMPSWriteAudio(ctx context.Context, result Result, audio []byte, text string, sink io.Writer) (Result, error) {
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	n, err := sink.Write(audio)
	result.Bytes = int64(n)
	if err != nil {
		return result, err
	}
	if n != len(audio) {
		return result, io.ErrShortWrite
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	result.Characters = BillingCharactersHanDouble(text)
	result.Complete, result.UsageConfirmed = true, true
	return result, nil
}

func TencentMPSProviderErrorMessage(err *ProviderError) string {
	message := "腾讯 MPS 请求失败"
	switch {
	case strings.HasPrefix(err.Code, "AuthFailure."):
		message = "腾讯云鉴权失败"
	case strings.HasPrefix(err.Code, "UnauthorizedOperation"):
		message = "腾讯 MPS 未授权或服务角色权限不足"
	case strings.HasPrefix(err.Code, "RequestLimitExceeded"):
		message = "腾讯 MPS 请求达到限频"
	}
	return fmt.Sprintf("%s（Code: %s；RequestId: %s）", message, err.Code, err.RequestID)
}
