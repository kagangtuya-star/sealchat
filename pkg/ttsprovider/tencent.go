package ttsprovider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
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
	"unicode"
	"unicode/utf8"
)

const TencentEndpoint = "https://tts.tencentcloudapi.com"
const TencentMaxTextRunes = 145

type TencentClient struct {
	SecretID  string
	SecretKey string
	Endpoint  string
	HTTP      *http.Client
	Now       func() time.Time
}

// Only the account identifier participates in scope; neither secret is exposed.
func TencentCredentialScope(secretID string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(secretID)))
	return "tencent:" + hex.EncodeToString(h[:12])
}

func TencentSampleRate(model string) int {
	if model == "tencent-tts-classic" {
		return 16000
	}
	return 24000
}

func TencentSpeed(rate float64) float64 {
	anchors := [][2]float64{{0.6, -2}, {0.8, -1}, {1, 0}, {1.2, 1}, {1.5, 2}, {2, 4}}
	if rate <= anchors[0][0] {
		return -2
	}
	for i := 1; i < len(anchors); i++ {
		if rate <= anchors[i][0] {
			a, b := anchors[i-1], anchors[i]
			return math.Round((a[1]+(rate-a[0])*(b[1]-a[1])/(b[0]-a[0]))*100) / 100
		}
	}
	return 4
}

func TencentVolume(volume int) float64 {
	return math.Max(-10, math.Min(10, (float64(volume)-50)/5))
}

func TencentPrimaryLanguage(model, voice, text, explicitLanguage string) int {
	switch explicitLanguage {
	case "zh":
		return 1
	case "en":
		return 2
	}
	languages := VoiceLanguages(ProviderTencent, model, voice)
	zh, en := slices.Contains(languages, "zh"), slices.Contains(languages, "en")
	if en && !zh {
		return 2
	}
	if zh && en {
		latin, otherLetters := 0, 0
		for _, r := range text {
			if unicode.Is(unicode.Han, r) {
				return 1
			}
			if unicode.Is(unicode.Latin, r) {
				latin++
			} else if unicode.IsLetter(r) {
				otherLetters++
			}
		}
		if latin > otherLetters {
			return 2
		}
	}
	return 1
}

func ValidateTencentInput(input Input) error {
	if input.Pitch != 1 {
		return errors.New("腾讯传统 TTS 不支持音调调整，Pitch 必须为 1")
	}
	if input.Instruction != "" {
		return errors.New("腾讯传统 TTS 不支持朗读指令")
	}
	if input.Language != "" && input.Language != "zh" && input.Language != "en" {
		return errors.New("腾讯传统 TTS 仅支持中文或英文")
	}
	if input.Rate < 0.5 || input.Rate > 2 || math.IsNaN(input.Rate) || math.IsInf(input.Rate, 0) || input.Volume < 0 || input.Volume > 100 {
		return errors.New("语音参数超出范围")
	}
	return nil
}

type tencentTextToVoiceRequest struct {
	Text            string  `json:"Text"`
	SessionID       string  `json:"SessionId"`
	Volume          float64 `json:"Volume"`
	Speed           float64 `json:"Speed"`
	ProjectID       int     `json:"ProjectId"`
	ModelType       int     `json:"ModelType"`
	VoiceType       int     `json:"VoiceType"`
	PrimaryLanguage int     `json:"PrimaryLanguage,omitempty"`
	SampleRate      int     `json:"SampleRate"`
	Codec           string  `json:"Codec"`
}

func tencentHash(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func tencentHMAC(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(value))
	return h.Sum(nil)
}

// TC3 signs exactly the bytes sent. Timestamp/version are Cloud API headers;
// content-type, host and the lowercased action are the signed canonical headers.
func (c *TencentClient) post(ctx context.Context, body tencentTextToVoiceRequest) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = TencentEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil || req.URL.Host == "" || req.URL.User != nil || req.URL.RawQuery != "" || (req.URL.Path != "" && req.URL.Path != "/") {
		return nil, errors.New("腾讯云 TTS 接口地址无效")
	}
	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	date := now.UTC().Format("2006-01-02")
	timestamp := strconv.FormatInt(now.Unix(), 10)
	contentType := "application/json; charset=utf-8"
	signedHeaders := "content-type;host;x-tc-action"
	canonicalHeaders := "content-type:" + contentType + "\nhost:" + req.URL.Host + "\nx-tc-action:texttovoice\n"
	canonicalRequest := "POST\n/\n\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + tencentHash(data)
	scope := date + "/tts/tc3_request"
	stringToSign := "TC3-HMAC-SHA256\n" + timestamp + "\n" + scope + "\n" + tencentHash([]byte(canonicalRequest))
	dateKey := tencentHMAC([]byte("TC3"+c.SecretKey), date)
	serviceKey := tencentHMAC(dateKey, "tts")
	signingKey := tencentHMAC(serviceKey, "tc3_request")
	signature := hex.EncodeToString(tencentHMAC(signingKey, stringToSign))
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-TC-Action", "TextToVoice")
	req.Header.Set("X-TC-Version", "2019-08-23")
	req.Header.Set("X-TC-Timestamp", timestamp)
	req.Header.Set("Authorization", "TC3-HMAC-SHA256 Credential="+c.SecretID+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
	h := &http.Client{Timeout: 90 * time.Second}
	if c.HTTP != nil {
		*h = *c.HTTP
	}
	// Redirects must never forward credentials or repeat a possible charge.
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := h.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("无法连接腾讯云语音合成服务；不会自动重试合成")
	}
	return resp, nil
}

// textToVoice returns complete audio without confirming task-level billing.
func (c *TencentClient) textToVoice(ctx context.Context, model string, input Input, codec string) ([]byte, string, error) {
	if err := ValidateTencentInput(input); err != nil {
		return nil, "", err
	}
	spec, ok := LookupModel(ProviderTencent, model)
	if !ok || spec.Runtime != RuntimeTencentTTS {
		return nil, "", errors.New("腾讯传统 TTS 模型不受支持")
	}
	voice, err := strconv.Atoi(input.Voice)
	if err != nil || voice <= 0 {
		return nil, "", errors.New("腾讯云 VoiceType 必须是整数音色 ID")
	}
	if !VoiceSupported(ProviderTencent, model, input.Voice) {
		return nil, "", errors.New("腾讯云音色与模型不匹配")
	}
	if input.Language != "" && !VoiceSpeechLanguageSupported(ProviderTencent, model, input.Voice, input.Language) {
		return nil, "", errors.New("当前腾讯云音色不支持此朗读语言")
	}
	if n := utf8.RuneCountInString(input.Text); n == 0 || n > TencentMaxTextRunes {
		return nil, "", errors.New("腾讯传统 TTS 单段文本须为 1–145 字符")
	}
	if codec != "wav" && codec != "mp3" && codec != "pcm" {
		return nil, "", errors.New("腾讯传统 TTS 音频格式不受支持")
	}
	if strings.TrimSpace(c.SecretID) == "" || strings.TrimSpace(c.SecretKey) == "" {
		return nil, "", errors.New("请同时配置腾讯云 SecretId 与 SecretKey")
	}
	var session [16]byte
	if _, err = rand.Read(session[:]); err != nil {
		return nil, "", errors.New("无法生成腾讯云 TTS SessionId")
	}
	body := tencentTextToVoiceRequest{Text: input.Text, SessionID: hex.EncodeToString(session[:]), Volume: TencentVolume(input.Volume), Speed: TencentSpeed(input.Rate), ProjectID: 0, ModelType: 1, VoiceType: voice, SampleRate: TencentSampleRate(model), Codec: codec}
	body.PrimaryLanguage = TencentPrimaryLanguage(model, input.Voice, input.Text, input.Language)
	resp, err := c.post(ctx, body)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	var payload struct {
		Response struct {
			Audio     string `json:"Audio"`
			RequestID string `json:"RequestId"`
			Error     *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"Response"`
	}
	// Bound JSON/base64 before allocating decoded media. Do not echo vendor
	// messages, response bodies or signing material in errors.
	limit := int64(base64.StdEncoding.EncodedLen(MaxAudioBytes) + (64 << 10))
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", errors.New("腾讯云语音合成响应读取失败")
	}
	if int64(len(raw)) > limit {
		return nil, "", errors.New("tts audio exceeds spool limit")
	}
	if json.Unmarshal(raw, &payload) != nil {
		return nil, "", errors.New("腾讯云语音合成返回了无法解析的数据")
	}
	r := payload.Response
	if r.Error != nil {
		return nil, r.RequestID, &ProviderError{Status: resp.StatusCode, Code: r.Error.Code, RequestID: r.RequestID}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, r.RequestID, &ProviderError{Status: resp.StatusCode, Code: "HTTPError", RequestID: r.RequestID}
	}
	if r.Audio == "" || r.RequestID == "" {
		return nil, r.RequestID, errors.New("腾讯云语音合成响应缺少 Audio 或 RequestId")
	}
	if len(r.Audio) > base64.StdEncoding.EncodedLen(MaxAudioBytes) {
		return nil, r.RequestID, errors.New("tts audio exceeds spool limit")
	}
	audio, err := base64.StdEncoding.DecodeString(r.Audio)
	if err != nil {
		return nil, r.RequestID, errors.New("腾讯云语音合成 Audio base64 无效")
	}
	if len(audio) == 0 || len(audio) > MaxAudioBytes {
		return nil, r.RequestID, errors.New("tts audio is empty or exceeds spool limit")
	}
	if ctx.Err() != nil {
		return nil, r.RequestID, ctx.Err()
	}
	return audio, r.RequestID, nil
}

func (c *TencentClient) Synthesize(ctx context.Context, model string, input Input, sink io.Writer) (Result, error) {
	audio, requestID, err := c.textToVoice(ctx, model, input, input.Format)
	result := Result{RequestID: requestID}
	if err != nil {
		return result, err
	}
	return tencentWriteAudio(ctx, result, audio, input.Text, sink)
}

func (c *TencentClient) SynthesizeSegments(ctx context.Context, model string, input Input, segments []string, sink io.Writer) (Result, error) {
	if len(segments) < 2 || strings.Join(segments, "") != input.Text {
		return Result{}, errors.New("tts segments do not match input text")
	}
	for _, segment := range segments {
		if n := utf8.RuneCountInString(segment); n == 0 || n > TencentMaxTextRunes {
			return Result{}, errors.New("腾讯传统 TTS 单段文本须为 1–145 字符")
		}
	}
	if input.Format != "wav" {
		return Result{}, errors.New("腾讯传统 TTS 长文本当前仅支持 WAV")
	}
	var result Result
	var pcm []byte
	for _, segment := range segments {
		part := input
		part.Text = segment
		audio, requestID, err := c.textToVoice(ctx, model, part, "pcm")
		if requestID != "" {
			result.RequestID = requestID
		}
		if err != nil {
			return result, err
		}
		if len(audio)%2 != 0 {
			return result, errors.New("腾讯云 PCM16 音频数据不完整")
		}
		if len(pcm)+len(audio)+44 > MaxAudioBytes {
			return result, errors.New("tts audio exceeds spool limit")
		}
		pcm = append(pcm, audio...)
	}
	wav, err := EncodePCM16MonoWAV(pcm, TencentSampleRate(model))
	if err != nil {
		return result, err
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	return tencentWriteAudio(ctx, result, wav, input.Text, sink)
}

func tencentWriteAudio(ctx context.Context, result Result, audio []byte, text string, sink io.Writer) (Result, error) {
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
	result.Complete = true
	result.Characters = int64(utf8.RuneCountInString(text))
	result.UsageConfirmed = true
	return result, nil
}

// EncodePCM16MonoWAV wraps little-endian mono PCM16 in one bounded RIFF file.
func EncodePCM16MonoWAV(pcm []byte, sampleRate int) ([]byte, error) {
	if len(pcm) == 0 || len(pcm)%2 != 0 || len(pcm)+44 > MaxAudioBytes || (sampleRate != 8000 && sampleRate != 16000 && sampleRate != 24000) {
		return nil, errors.New("无效或过大的 PCM16 mono 音频")
	}
	wav := make([]byte, 44+len(pcm))
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], uint32(sampleRate))
	binary.LittleEndian.PutUint32(wav[28:], uint32(sampleRate*2))
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], uint32(len(pcm)))
	copy(wav[44:], pcm)
	return wav, nil
}

func TencentErrorMessage(code string) string {
	switch {
	case strings.HasPrefix(code, "AuthFailure."):
		return "腾讯云 SecretId / SecretKey 鉴权失败"
	case code == "InvalidParameterValue.VoiceType":
		return "腾讯云音色不可用"
	case code == "InvalidParameterValue.PrimaryLanguage":
		return "腾讯云朗读语言参数无效"
	case code == "UnsupportedOperation.ServerNotOpen" || code == "InvalidParameterValue.AppIdNotRegistered":
		return "腾讯云语音合成服务尚未开通或账号不可用"
	case strings.HasPrefix(code, "LimitExceeded."):
		return "腾讯云语音合成请求达到并发限制"
	default:
		return "腾讯云语音合成请求失败"
	}
}

func TencentProviderErrorMessage(err *ProviderError) string {
	return fmt.Sprintf("%s（Code: %s；RequestId: %s）", TencentErrorMessage(err.Code), err.Code, err.RequestID)
}
