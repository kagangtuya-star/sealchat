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
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func mpsTestInput() Input {
	return Input{Text: "你好A", Voice: "dynamic-voice", Format: "wav", Rate: 1, Pitch: 1, Volume: 50}
}

type mpsTestTransport func(*http.Request) (*http.Response, error)

func (f mpsTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func mpsTestWAV(t *testing.T, value byte) []byte {
	t.Helper()
	wav, err := EncodePCM16MonoWAV(bytes.Repeat([]byte{value, 0}, 240), 24000)
	if err != nil {
		t.Fatal(err)
	}
	return wav
}

func TestTencentMPSModelCatalog(t *testing.T) {
	count := 0
	for _, spec := range ModelCatalog() {
		if spec.Runtime != RuntimeTencentMPS {
			continue
		}
		count++
		price, display := 0.00035, "0.175 元 / 500 计费字符"
		if strings.HasSuffix(spec.ID, "-turbo") {
			price, display = 0.0002, "0.1 元 / 500 计费字符"
		}
		if !slices.Contains(TencentMPSModels(), spec.ID) || spec.ProviderKind != ProviderTencent || spec.Pricing.Mode != PricingCharacter || spec.Pricing.CharacterPrice == nil || *spec.Pricing.CharacterPrice != price || spec.Pricing.DisplayPrice != display || spec.DefaultVoice != "" || spec.Capabilities != (ModelCapabilities{HTTPStreaming: true}) {
			t.Fatalf("invalid MPS model: %+v", spec)
		}
	}
	if count != 6 {
		t.Fatalf("MPS model count=%d", count)
	}
	for _, id := range []string{"TTS-F", "tts-f", "tencent-mps", "tencent-mts"} {
		if _, ok := LookupModel(ProviderTencent, id); ok {
			t.Fatalf("unexpected model %s", id)
		}
	}
	if len(TencentMPSTextLanguages()) != 40 || slices.Contains(TencentMPSTextLanguages(), "hu") || !slices.Contains(TencentMPSTextLanguages(), "hun") {
		t.Fatal("language contract changed")
	}
}

func TestTencentMPSTC3Signature(t *testing.T) {
	for _, action := range []string{"TextToSpeech", "DescribeVoices"} {
		t.Run(action, func(t *testing.T) {
			calls := 0
			cloud := TencentCloudClient{SecretID: "test-id", SecretKey: "test-key", Now: func() time.Time { return time.Unix(1551113065, 0) }, HTTP: &http.Client{Transport: mpsTestTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				body, _ := io.ReadAll(r.Body)
				if r.URL.String() != TencentMPSEndpoint || r.Header.Get("X-TC-Action") != action || r.Header.Get("X-TC-Version") != "2019-06-12" || r.Header.Get("X-TC-Region") != "" || string(body) != `{"Text":"你好A"}` {
					t.Error("MPS request/header mismatch")
				}
				hash := func(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
				mac := func(key []byte, s string) []byte {
					h := hmac.New(sha256.New, key)
					h.Write([]byte(s))
					return h.Sum(nil)
				}
				canonical := "POST\n/\n\ncontent-type:application/json; charset=utf-8\nhost:mps.tencentcloudapi.com\nx-tc-action:" + strings.ToLower(action) + "\n\ncontent-type;host;x-tc-action\n" + hash(string(body))
				scope := "2019-02-25/mps/tc3_request"
				key := mac(mac(mac([]byte("TC3test-key"), "2019-02-25"), "mps"), "tc3_request")
				want := "TC3-HMAC-SHA256 Credential=test-id/" + scope + ", SignedHeaders=content-type;host;x-tc-action, Signature=" + hex.EncodeToString(mac(key, "TC3-HMAC-SHA256\n1551113065\n"+scope+"\n"+hash(canonical)))
				if r.Header.Get("Authorization") != want {
					t.Error("MPS signature mismatch")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"Response":{"RequestId":"fixed","ErrorCode":0}}`))}, nil
			})}}
			var out struct{ ErrorCode int }
			id, err := cloud.Post(context.Background(), TencentMPSEndpoint, "mps", action, "2019-06-12", struct{ Text string }{"你好A"}, &out)
			if err != nil || id != "fixed" || calls != 1 {
				t.Fatalf("id=%s err=%v", id, err)
			}
		})
	}
}

func TestTencentMPSDescribeVoicesPaginationAndBounds(t *testing.T) {
	for _, total := range []int{205, 999999} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			var body struct {
				VoiceType         string
				PageNum, PageSize int
				ExtParam          string
			}
			json.NewDecoder(r.Body).Decode(&body)
			if r.Header.Get("X-TC-Action") != "DescribeVoices" || body.VoiceType != "system" || body.PageNum != calls || body.PageSize != 100 || body.ExtParam != `{"engine":"minimax"}` || r.Header.Get("X-TC-Region") != "" {
				t.Error("wrong DescribeVoices request")
			}
			voices := []TencentMPSVoice{}
			for i := (calls - 1) * 100; i < min(calls*100, total); i++ {
				category := "system"
				if i == 1 {
					category = "clone"
				}
				if i == 2 {
					category = "design"
				}
				voices = append(voices, TencentMPSVoice{VoiceID: fmt.Sprintf("v-%d", i), Category: category, Languages: []string{"zh"}})
			}
			json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{"ErrorCode": 0, "TotalCount": total, "Voices": voices, "RequestId": fmt.Sprintf("page-%d", calls)}})
		}))
		c := TencentMPSClient{SecretID: "id", SecretKey: "key", Endpoint: server.URL}
		voices, id, err := c.DescribeVoices(context.Background())
		server.Close()
		wantCalls := min((total+99)/100, 25)
		if err != nil || calls != wantCalls || len(voices) != min(total, TencentMPSMaxVoices)-2 || id != fmt.Sprintf("page-%d", calls) {
			t.Fatalf("voices=%d calls=%d id=%s err=%v", len(voices), calls, id, err)
		}
		for _, v := range voices {
			if v.Category != "system" {
				t.Fatal("personal voice included")
			}
		}
	}
}

func TestTencentMPSVoiceMapping(t *testing.T) {
	before := VoiceCatalog()
	voices := TencentMPSVoiceSpecs([]TencentMPSVoice{
		{VoiceID: "a", Category: "system", Name: "新闻", Gender: "female", Age: "middle_aged", Languages: []string{"zh", "ja", "hun", "hu", "unsupported", "zh"}, Labels: []string{"知性", "中年", "30"}, Scenes: []string{"解说", "知性"}, Description: "do not put the full description into tags"},
		{VoiceID: "b", Category: "system", Gender: "unknown", Age: "unknown"},
		{VoiceID: "c", Category: "clone"},
	})
	if len(voices) != 2 || voices[0].PresetSource != "tencent2" || voices[0].ProviderKind != "tencent" || voices[0].Kind != "mps-system" || !slices.Equal(voices[0].Models, TencentMPSModels()) || !slices.Equal(voices[0].Languages, []string{"zh", "ja", "hun"}) || !slices.Equal(voices[0].SpeechLanguages, voices[0].Languages) || voices[0].Tags != "女声、中年、知性、解说" || voices[1].Name != "b" || len(voices[1].Languages) != 0 || len(voices[1].SpeechLanguages) != 0 || voices[1].Tags != "" {
		t.Fatalf("mapping=%+v", voices)
	}
	if !reflect.DeepEqual(before, VoiceCatalog()) {
		t.Fatal("mutable static catalog")
	}
}

func TestTencentMPSParameterMapping(t *testing.T) {
	for _, test := range []struct {
		pitch float64
		want  int
	}{{0.5, -12}, {1, 0}, {2, 12}, {0.1, -12}, {4, 12}} {
		if got := TencentMPSPitch(test.pitch); got != test.want {
			t.Fatalf("pitch=%v got=%d", test.pitch, got)
		}
	}
	for _, test := range []struct {
		volume int
		want   float64
	}{{0, 0.1}, {50, 1}, {100, 10}, {-10, 0.1}, {200, 10}} {
		if got := TencentMPSVolume(test.volume); got != test.want {
			t.Fatalf("volume=%d got=%v", test.volume, got)
		}
	}
	for _, lang := range []string{"", "ja", "hun"} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if r.Header.Get("X-TC-Action") != "TextToSpeech" || body["Text"] != "你好A" || body["VoiceId"] != "dynamic-voice" || len(body) != 3+boolInt(lang != "") {
				t.Error("MPS request mismatch or extra fields")
			}
			if lang == "" {
				if _, ok := body["TextLang"]; ok {
					t.Error("auto detection must omit TextLang")
				}
			} else if body["TextLang"] != lang {
				t.Error("wrong TextLang")
			}
			var ext map[string]any
			json.Unmarshal([]byte(body["ExtParam"].(string)), &ext)
			syn := ext["synExt"].(map[string]any)
			if ext["engine"] != "minimax" || len(ext) != 2 || len(syn) != 6 || syn["model"] != "speech-2.8-hd" || syn["format"] != "wav" || syn["sampleRate"] != 24000. || syn["speed"] != 1.2 || syn["vol"] != 10. || syn["pitch"] != 12. {
				t.Error("wrong synExt")
			}
			json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{"ErrorCode": 0, "RequestId": "synth", "AudioData": base64.StdEncoding.EncodeToString(mpsTestWAV(t, 1))}})
		}))
		input := mpsTestInput()
		input.Language, input.Rate, input.Pitch, input.Volume, input.Seed, input.BitRate, input.SampleRate = lang, 1.2, 2, 100, 42, 64, 8000
		c := TencentMPSClient{SecretID: "id", SecretKey: "key", Endpoint: server.URL}
		result, err := c.Synthesize(context.Background(), "speech-2.8-hd", input, io.Discard)
		server.Close()
		if err != nil || calls != 1 || result.Characters != 5 || !result.UsageConfirmed || !result.Complete || result.RequestID != "synth" {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestTencentMPSSegmentsWAVBillingAndMP3Preflight(t *testing.T) {
	for _, fail := range []int{0, 2} {
		calls := 0
		segments := []string{"你好", "世A", "界"}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			var body struct{ Text, ExtParam string }
			json.NewDecoder(r.Body).Decode(&body)
			if body.Text != segments[calls-1] || !strings.Contains(body.ExtParam, `"format":"wav"`) {
				t.Error("segment request")
			}
			if calls == fail {
				fmt.Fprint(w, `{"Response":{"ErrorCode":42,"Msg":"private-key","RequestId":"failed-2"}}`)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{"ErrorCode": 0, "RequestId": fmt.Sprintf("part-%d", calls), "AudioData": base64.StdEncoding.EncodeToString(mpsTestWAV(t, byte(calls)))}})
		}))
		c := TencentMPSClient{SecretID: "id", SecretKey: "key", Endpoint: server.URL}
		input := mpsTestInput()
		input.Text = strings.Join(segments, "")
		var sink bytes.Buffer
		result, err := c.SynthesizeSegments(context.Background(), "speech-2.8-turbo", input, segments, &sink)
		if fail == 2 {
			var p *ProviderError
			if !errors.As(err, &p) || p.Code != "MPS_42" || p.RequestID != "failed-2" || calls != 2 || result.UsageConfirmed || result.Complete || result.RequestID != "failed-2" || sink.Len() != 0 || strings.Contains(err.Error(), "private-key") {
				t.Fatalf("partial=%+v err=%v", result, err)
			}
		} else {
			media, me := InspectMedia(sink.Bytes())
			pcm, pe := ParsePCM16MonoWAV(sink.Bytes(), 24000)
			if err != nil || me != nil || pe != nil || media.SampleRate != 24000 || media.ChannelCount != 1 || media.DataSize != 1440 || sink.Len() != 1484 || result.Characters != 9 || !result.Complete || !result.UsageConfirmed || result.RequestID != "part-3" {
				t.Fatalf("merged=%+v media=%+v err=%v", result, media, err)
			}
			for i := range 3 {
				if !bytes.Equal(pcm[i*480:(i+1)*480], bytes.Repeat([]byte{byte(i + 1), 0}, 240)) {
					t.Fatal("wrong PCM order")
				}
			}
		}
		before := calls
		input.Format = "mp3"
		if _, err := c.SynthesizeSegments(context.Background(), "speech-2.8-turbo", input, segments, io.Discard); err == nil || !strings.Contains(err.Error(), "腾讯 MPS 长文本当前仅支持 WAV 合成") || calls != before {
			t.Fatal("MP3 preflight incurred a charge")
		}
		server.Close()
	}
}

func TestTencentMPSSingleMP3AndValidationFailures(t *testing.T) {
	// One complete MPEG2 mono frame: 24 kHz, 64 kbps, 192 bytes.
	mp3 := make([]byte, 192)
	copy(mp3, []byte{0xff, 0xf3, 0x84, 0xc0})
	if _, err := InspectMedia(mp3); err != nil {
		t.Fatal(err)
	}
	calls := 0
	c := TencentMPSClient{SecretID: "id", SecretKey: "key", HTTP: &http.Client{Transport: mpsTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"Response":{"ErrorCode":0,"RequestId":"mp3","AudioData":%q}}`, base64.StdEncoding.EncodeToString(mp3))))}, nil
	})}}
	input := mpsTestInput()
	input.Format = "mp3"
	result, err := c.Synthesize(context.Background(), "speech-02-hd", input, io.Discard)
	if err != nil || result.Bytes != 192 || !result.UsageConfirmed {
		t.Fatalf("MP3 result=%+v err=%v", result, err)
	}
	for _, change := range []func(*Input){func(i *Input) { i.Voice = "" }, func(i *Input) { i.Instruction = "温柔" }, func(i *Input) { i.Language = "hu" }, func(i *Input) { i.Rate = 3 }, func(i *Input) { i.Pitch = 0.1 }, func(i *Input) { i.Rate = math.NaN() }, func(i *Input) { i.Pitch = math.Inf(1) }, func(i *Input) { i.Volume = -1 }, func(i *Input) { i.Format = "pcm" }} {
		i := input
		change(&i)
		if _, err := c.Synthesize(context.Background(), "speech-02-hd", i, io.Discard); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	if _, err := c.Synthesize(context.Background(), "tencent-tts-classic", input, io.Discard); err == nil {
		t.Fatal("traditional model accepted by MPS")
	}
	if calls != 1 {
		t.Fatal("validation made provider request")
	}
}

type mpsShortWriter struct{}

func (mpsShortWriter) Write(b []byte) (int, error) { return len(b) - 1, nil }

func TestTencentMPSResponseErrorsAndUnconfirmedUsage(t *testing.T) {
	for _, body := range []string{
		`{"Response":{"ErrorCode":12,"Msg":"secret-vendor-message","RequestId":"failed"}}`,
		`{"Response":{"Error":{"Code":"UnauthorizedOperation","Message":"secret-vendor-message"},"RequestId":"failed"}}`,
		`{"Response":{"ErrorCode":0,"AudioData":"???","RequestId":"failed"}}`,
		`{"Response":{"ErrorCode":0,"AudioData":"AAE=","RequestId":"failed"}}`,
		`{"Response":{"ErrorCode":0,"AudioData":"","RequestId":"failed"}}`,
		`{"Response":{"ErrorCode":0,"AudioData":"AAE="}}`,
		`{"Response":{"AudioData":"AAE=","RequestId":"failed"}}`, `{`,
	} {
		calls := 0
		c := TencentMPSClient{SecretID: "id", SecretKey: "key", HTTP: &http.Client{Transport: mpsTestTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}}
		result, err := c.Synthesize(context.Background(), "speech-2.8-hd", mpsTestInput(), io.Discard)
		if err == nil || result.UsageConfirmed || result.Complete || calls != 1 || strings.Contains(err.Error(), "secret-vendor-message") {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		if strings.Contains(body, `"ErrorCode":12`) || strings.Contains(body, `"Error":`) {
			var p *ProviderError
			if !errors.As(err, &p) || p.RequestID != "failed" {
				t.Fatal("lost provider error")
			}
			if _, id, e := c.DescribeVoices(context.Background()); e == nil || id != "failed" || strings.Contains(e.Error(), "secret-vendor-message") {
				t.Fatal("unsafe DescribeVoices error")
			}
		}
	}
	c := TencentMPSClient{SecretID: "id", SecretKey: "key", HTTP: &http.Client{Transport: mpsTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"Response":{"ErrorCode":0,"RequestId":"short","AudioData":%q}}`, base64.StdEncoding.EncodeToString(mpsTestWAV(t, 1)))))}, nil
	})}}
	if result, err := c.Synthesize(context.Background(), "speech-2.8-hd", mpsTestInput(), mpsShortWriter{}); !errors.Is(err, io.ErrShortWrite) || result.UsageConfirmed || result.Complete {
		t.Fatal("short write confirmed usage")
	}
	for _, rate := range []int{16000, 24000} {
		wav, _ := EncodePCM16MonoWAV([]byte{1, 0}, rate)
		_, err := ParsePCM16MonoWAV(wav, 24000)
		if (err == nil) != (rate == 24000) {
			t.Fatal("wrong sample rate accepted")
		}
		wav[34] = 8
		if _, err := ParsePCM16MonoWAV(wav, 24000); err == nil {
			t.Fatal("8-bit WAV accepted")
		}
	}
}
