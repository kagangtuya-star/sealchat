package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sealchat/model"
	"sealchat/pkg/ttsprovider"
	"sealchat/service/ai"
	"sealchat/utils"
)

type ttsMPSTestTransport func(*http.Request) (*http.Response, error)

func (f ttsMPSTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type ttsMPSTranslateRunner struct {
	calls    atomic.Int32
	requests chan ai.RunRequest
	text     string
}

func (f *ttsMPSTranslateRunner) Run(_ context.Context, r ai.RunRequest) (ai.RunResult, error) {
	f.calls.Add(1)
	f.requests <- r
	return ai.RunResult{FeatureKey: r.FeatureKey, Result: f.text, ProviderID: "text-platform", Model: "translate-test", Usage: ai.RunUsage{PromptTokens: 10, CompletionTokens: 20}}, nil
}

func ttsMPSTranslationFixture(t *testing.T) (*model.UserModel, *model.ChannelIdentityModel, *ttsMPSTranslateRunner) {
	t.Helper()
	t.Chdir(t.TempDir())
	model.DBInit(&utils.AppConfig{DSN: "file:mps-translate-" + utils.NewID() + "?mode=memory&cache=shared", SQLite: utils.SQLiteConfig{ReadConnections: 1}})
	cfg := utils.ReadConfig()
	previous, factory := cfg.AI, ttsTranslateRunnerFactory
	t.Cleanup(func() { cfg.AI, ttsTranslateRunnerFactory = previous, factory })
	cfg.AI = utils.NormalizeAIConfig(utils.AIConfig{
		Enabled:   true,
		Providers: []utils.AIProviderConfig{{ID: "text-platform", Enabled: true, Models: []string{"translate-test"}}},
		Features:  map[string]utils.AIFeatureConfig{ai.FeatureTTSTranslate: {Enabled: true, DefaultModel: "translate-test"}},
		Pricing:   []utils.AIModelPricingConfig{{ProviderID: "text-platform", Model: "translate-test"}},
	})
	user := &model.UserModel{StringPKBaseModel: model.StringPKBaseModel{ID: "mps-user"}, Username: "mps-user"}
	channel := &model.ChannelModel{StringPKBaseModel: model.StringPKBaseModel{ID: "mps-channel"}, UserID: user.ID, WorldID: "mps-world"}
	identity := &model.ChannelIdentityModel{StringPKBaseModel: model.StringPKBaseModel{ID: "mps-role"}, ChannelID: channel.ID, UserID: user.ID}
	for _, row := range []any{user, channel, identity} {
		if err := model.GetDB().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := model.UserPreferenceUpsert(user.ID, TTSAutoPreference, "true"); err != nil {
		t.Fatal(err)
	}
	fake := &ttsMPSTranslateRunner{requests: make(chan ai.RunRequest, 10)}
	ttsTranslateRunnerFactory = func(utils.AIConfig) ai.TaskRunner { return fake }
	return user, identity, fake
}

func ttsMPSProviderFixture(id string) utils.SpeechProviderConfig {
	spec, _ := ttsprovider.LookupModel("tencent", "speech-2.8-hd")
	return utils.SpeechProviderConfig{ID: id, ProviderKind: "tencent", Enabled: true, Model: spec.ID, SecretID: "private-mps-id-" + id, SecretKey: "private-mps-key", CredentialScope: ttsprovider.TencentCredentialScope("private-mps-id-" + id), SynthesisEndpoint: ttsprovider.TencentMPSEndpoint, CharacterPrice: spec.Pricing.CharacterPrice}
}

func ttsMPSPrimeTestCache(t *testing.T, p utils.SpeechProviderConfig) {
	t.Helper()
	ttsMPSCatalog.prime(p.CredentialScope, ttsprovider.TencentMPSVoiceSpecs([]ttsprovider.TencentMPSVoice{{VoiceID: "dynamic", Name: "多语", Category: "system", Languages: []string{"zh", "ja"}}}))
	t.Cleanup(func() {
		ttsMPSCatalog.mu.Lock()
		defer ttsMPSCatalog.mu.Unlock()
		delete(ttsMPSCatalog.entries, ttsMPSCatalogKey(p.CredentialScope))
	})
}

func TestTencentMPSPrimeCatalogs(t *testing.T) {
	for _, test := range []struct {
		name      string
		duplicate bool
		failure   bool
	}{
		{name: "empty-cache"},
		{name: "shared-scope", duplicate: true},
		{name: "failure-isolation", duplicate: true, failure: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			cfg := utils.ReadConfig()
			previous, transport := cfg.AI, http.DefaultTransport
			p := ttsMPSProviderFixture("prime-" + test.name)
			providers := []utils.SpeechProviderConfig{p}
			if test.duplicate {
				duplicate := p
				duplicate.ID, duplicate.Model = "duplicate", "speech-2.6-turbo"
				providers = append(providers, duplicate)
			}
			other := ttsMPSProviderFixture("prime-other-" + test.name)
			if test.failure {
				providers = append(providers, other)
			}
			disabled, incomplete, traditional := p, p, p
			disabled.ID, disabled.Enabled = "disabled", false
			incomplete.ID, incomplete.SecretKey = "incomplete", ""
			traditional.ID, traditional.Model = "traditional", "tencent-tts-classic"
			providers = append([]utils.SpeechProviderConfig{disabled, incomplete, traditional}, providers...)
			cfg.AI = utils.AIConfig{Enabled: true, Speech: &utils.SpeechConfig{Enabled: true, Providers: providers}}
			t.Cleanup(func() {
				cfg.AI, http.DefaultTransport = previous, transport
				ttsMPSCatalog.mu.Lock()
				defer ttsMPSCatalog.mu.Unlock()
				delete(ttsMPSCatalog.entries, ttsMPSCatalogKey(p.CredentialScope))
				delete(ttsMPSCatalog.entries, ttsMPSCatalogKey(other.CredentialScope))
			})
			if len(ttsMPSCatalog.read(p.CredentialScope)) != 0 || len(ttsMPSCatalog.read(other.CredentialScope)) != 0 {
				t.Fatal("prime must start with an empty cache")
			}
			var calls, otherCalls atomic.Int32
			http.DefaultTransport = ttsMPSTestTransport(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("X-TC-Action") != "DescribeVoices" {
					t.Error("prime must only request DescribeVoices")
				}
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 15*time.Second {
					t.Error("prime must have its own bounded context")
				}
				if strings.Contains(r.Header.Get("Authorization"), other.SecretID) {
					otherCalls.Add(1)
				} else {
					calls.Add(1)
					if test.failure {
						return nil, errors.New("catalog unavailable")
					}
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"Response":{"ErrorCode":0,"RequestId":"prime","TotalCount":1,"Voices":[{"VoiceId":"dynamic","Category":"system","Languages":["zh"]}]}}`))}, nil
			})
			TTSPrimeMPSCatalogs()
			if calls.Load() != 1 {
				t.Fatalf("DescribeVoices calls per scope=%d", calls.Load())
			}
			if test.failure {
				if len(ttsMPSCatalog.read(p.CredentialScope)) != 0 || otherCalls.Load() != 1 || !ttsSystemVoiceSupported(other, other.Model, "dynamic") {
					t.Fatal("failed prime retried, populated the cache or affected another provider")
				}
			} else if !ttsSystemVoiceSupported(p, p.Model, "dynamic") || otherCalls.Load() != 0 {
				t.Fatal("prime did not populate the expected scope")
			}
		})
	}
}

func TestTencentMPSCacheFreshExpiredStaleAndConcurrent(t *testing.T) {
	c := ttsMPSVoiceCache{entries: map[string]ttsMPSCatalogEntry{}, flights: map[string]*ttsMPSCatalogFlight{}}
	voices := ttsprovider.TencentMPSVoiceSpecs([]ttsprovider.TencentMPSVoice{{VoiceID: "v", Category: "system", Languages: []string{"zh"}}})
	calls := 0
	fetch := func(context.Context) ([]ttsprovider.TTSVoiceSpec, error) { calls++; return voices, nil }
	for range 2 {
		got, err := c.refresh(context.Background(), "scope", fetch)
		if err != nil || len(got) != 1 {
			t.Fatal("missing cache")
		}
		got[0].Languages[0] = "mutated"
	}
	if calls != 1 || !slices.Equal(c.read("scope")[0].Languages, []string{"zh"}) {
		t.Fatal("fresh cache fetched or mutated")
	}
	entry := c.entries[ttsMPSCatalogKey("scope")]
	entry.updatedAt = time.Now().Add(-11 * time.Minute)
	c.entries[ttsMPSCatalogKey("scope")] = entry
	if got, err := c.refresh(context.Background(), "scope", fetch); err != nil || len(got) != 1 || calls != 2 {
		t.Fatal("expired cache not refreshed")
	}
	entry = c.entries[ttsMPSCatalogKey("scope")]
	entry.updatedAt = time.Now().Add(-11 * time.Minute)
	c.entries[ttsMPSCatalogKey("scope")] = entry
	fail := func(context.Context) ([]ttsprovider.TTSVoiceSpec, error) { return nil, errors.New("fetch failed") }
	if got, err := c.refresh(context.Background(), "scope", fail); err == nil || len(got) != 1 {
		t.Fatal("stale cache lost")
	}
	if got, err := c.refresh(context.Background(), "empty", fail); err == nil || len(got) != 0 {
		t.Fatal("failed empty cache fabricated data")
	}
	var requests atomic.Int32
	gate, started := make(chan struct{}), make(chan struct{})
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := c.refresh(context.Background(), "parallel", func(context.Context) ([]ttsprovider.TTSVoiceSpec, error) {
				if requests.Add(1) == 1 {
					close(started)
				}
				<-gate
				return voices, nil
			})
			if err != nil || len(got) != 1 {
				t.Error("parallel cache")
			}
		}()
	}
	<-started
	// Reads never wait on the catalog HTTP call.
	if len(c.read("parallel")) != 0 {
		t.Fatal("inflight data visible")
	}
	close(gate)
	wg.Wait()
	if requests.Load() != 1 {
		t.Fatalf("parallel fetches=%d", requests.Load())
	}
}

func TestResolveTencentMPSProviderAndRuntimeDispatch(t *testing.T) {
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	var actions []string
	http.DefaultTransport = ttsMPSTestTransport(func(r *http.Request) (*http.Response, error) {
		action := r.Header.Get("X-TC-Action")
		actions = append(actions, action)
		body := `{"Response":{"Audio":"AAE=","RequestId":"traditional"}}`
		if action == "DescribeVoices" {
			if r.URL.Host != "mps.tencentcloudapi.com" || r.Header.Get("X-TC-Version") != "2019-06-12" {
				t.Error("MPS wrong endpoint/version")
			}
			var request struct {
				VoiceType, ExtParam string
				PageNum, PageSize   int
			}
			json.NewDecoder(r.Body).Decode(&request)
			if request.VoiceType != "system" || request.ExtParam != `{"engine":"minimax"}` || request.PageNum != 1 || request.PageSize != 100 {
				t.Error("MPS resolver wrong query")
			}
			body = `{"Response":{"ErrorCode":0,"RequestId":"catalog","TotalCount":3,"Voices":[{"VoiceId":"en-first","Category":"system","Languages":["en"]},{"VoiceId":"zh-default","Category":"system","Languages":["zh","ja"]},{"VoiceId":"personal","Category":"clone"}]}}`
		} else if action != "TextToVoice" || r.URL.Host != "tts.tencentcloudapi.com" {
			t.Error("wrong runtime dispatched")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	p := ttsMPSProviderFixture("resolve")
	t.Cleanup(func() {
		ttsMPSCatalog.mu.Lock()
		defer ttsMPSCatalog.mu.Unlock()
		delete(ttsMPSCatalog.entries, ttsMPSCatalogKey(p.CredentialScope))
	})
	for _, id := range ttsprovider.TencentMPSModels() {
		before := len(actions)
		result, err := ResolveTTSProvider(context.Background(), TTSProviderResolveRequest{ProviderKind: "tencent", Model: id, ProviderID: p.ID, SavedProvider: &p})
		if err != nil || len(actions) != before+1 || actions[before] != "DescribeVoices" || len(result.Models) != 6 || result.SynthesisEndpoint != ttsprovider.TencentMPSEndpoint || result.ProviderID != p.ID || result.CredentialScope != p.CredentialScope || result.Region != "" || result.Workspace != "" || result.VoiceEndpoint != "" {
			t.Fatalf("resolution=%+v err=%v", result, err)
		}
		for _, m := range result.Models {
			if m.Runtime != ttsprovider.RuntimeTencentMPS || m.DefaultVoice != "zh-default" {
				t.Fatal("MPS resolved default/runtime")
			}
		}
		raw, _ := json.Marshal(result)
		if strings.Contains(string(raw), p.SecretID) || strings.Contains(string(raw), p.SecretKey) {
			t.Fatal("resolver exposed secrets")
		}
		if !ttsSystemVoiceSupported(p, id, "zh-default") || ttsDefaultSystemVoice(p, id) != "zh-default" {
			t.Fatal("resolver did not prime cache")
		}
	}
	if got := ttsMPSDefaultVoice([]ttsprovider.TTSVoiceSpec{{ID: "fallback", Languages: []string{"en"}}}); got != "fallback" {
		t.Fatal("default fallback")
	}
	for _, id := range []string{"", "tencent-tts-classic", "tencent-tts-large"} {
		result, err := ResolveTTSProvider(context.Background(), TTSProviderResolveRequest{ProviderKind: "tencent", Model: id, SecretID: p.SecretID, SecretKey: p.SecretKey})
		if err != nil || result.SynthesisEndpoint != ttsprovider.TencentEndpoint || len(result.Models) != 2 || actions[len(actions)-1] != "TextToVoice" {
			t.Fatal("traditional resolver changed")
		}
		for _, m := range result.Models {
			if m.Runtime != ttsprovider.RuntimeTencentTTS {
				t.Fatal("traditional runtime metadata")
			}
		}
	}
	before := len(actions)
	for _, request := range []TTSProviderResolveRequest{{Model: p.Model, SecretID: p.SecretID}, {Model: p.Model, SecretKey: p.SecretKey}, {Model: p.Model, ProviderID: "different", SavedProvider: &p}, {Model: "tencent-tts-classic", SecretID: p.SecretID, SecretKey: p.SecretKey}} {
		if _, err := resolveTencentMPSProvider(context.Background(), &ttsprovider.TencentMPSClient{}, request); err == nil {
			t.Fatal("invalid credentials/model accepted")
		}
	}
	if len(actions) != before {
		t.Fatal("invalid resolver sent a request")
	}
}

func TestTencentMPSDirectoryDedupAndFailureIsolation(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := utils.ReadConfig()
	previous := cfg.AI
	transport := http.DefaultTransport
	t.Cleanup(func() { cfg.AI = previous; http.DefaultTransport = transport })
	p := ttsMPSProviderFixture("directory")
	duplicate := p
	duplicate.ID, duplicate.Model = "second", "speech-2.6-turbo"
	cfg.AI = utils.AIConfig{Enabled: true, Speech: &utils.SpeechConfig{Enabled: true, Providers: []utils.SpeechProviderConfig{p, duplicate}}}
	calls := 0
	http.DefaultTransport = ttsMPSTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"Response":{"ErrorCode":0,"RequestId":"directory","TotalCount":1,"Voices":[{"VoiceId":"dynamic","Category":"system","Languages":["zh","ja"]}]}}`))}, nil
	})
	t.Cleanup(func() {
		ttsMPSCatalog.mu.Lock()
		defer ttsMPSCatalog.mu.Unlock()
		delete(ttsMPSCatalog.entries, ttsMPSCatalogKey(p.CredentialScope))
	})
	static := ttsprovider.VoiceCatalog()
	for range 2 {
		items := TTSSystemVoices(context.Background())
		if calls != 1 || len(items) != len(static)+1 || items[len(items)-1].PresetSource != "tencent2" || len(items[len(items)-1].Models) != 6 {
			t.Fatal("directory did not deduplicate cache/providers/models")
		}
	}
	if !reflect.DeepEqual(static, ttsprovider.VoiceCatalog()) {
		t.Fatal("static catalog was changed")
	}
	other := ttsMPSProviderFixture("unavailable")
	cfg.AI.Speech.Providers = []utils.SpeechProviderConfig{other}
	http.DefaultTransport = ttsMPSTestTransport(func(r *http.Request) (*http.Response, error) { return nil, errors.New("private-network-error") })
	if items := TTSSystemVoices(); len(items) != len(static) {
		t.Fatal("MPS failure lost static catalog")
	}
	cfg.AI.Speech.Providers = []utils.SpeechProviderConfig{p}
	ttsMPSCatalog.mu.Lock()
	entry := ttsMPSCatalog.entries[ttsMPSCatalogKey(p.CredentialScope)]
	entry.updatedAt = time.Now().Add(-11 * time.Minute)
	ttsMPSCatalog.entries[ttsMPSCatalogKey(p.CredentialScope)] = entry
	ttsMPSCatalog.mu.Unlock()
	if items := TTSSystemVoices(); len(items) != len(static)+1 {
		t.Fatal("directory lost stale MPS catalog")
	}
}

func TestTencentMPSPrepareLocalAndSpeechLanguageTranslation(t *testing.T) {
	user, identity, fake := ttsMPSTranslationFixture(t)
	p := ttsMPSProviderFixture("prepare")
	ttsMPSPrimeTestCache(t, p)
	cfg := utils.GetConfig()
	cfg.AI.Speech = &utils.SpeechConfig{Enabled: true, DefaultProvider: p.ID, DefaultVoice: "dynamic", Format: "wav", Providers: []utils.SpeechProviderConfig{p}}
	previous := http.DefaultTransport
	var requests atomic.Int32
	t.Cleanup(func() { http.DefaultTransport = previous })
	http.DefaultTransport = ttsMPSTestTransport(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("provider I/O forbidden during preparation")
	})
	role, _ := TTSRoleConfig(user.ID, identity.ID)
	role.SystemVoice, role.SystemVoiceProvider, role.SystemVoiceProviderID, role.SystemVoiceModel, role.SpeechLanguage = "dynamic", "tencent", p.ID, p.Model, "ja"
	role.Pitch = 1.3
	if err := TTSSaveRoleConfig(user.ID, identity.ID, *role); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ttsSystemVoiceLanguages(p, p.Model, "dynamic"), []string{"zh", "ja"}) || !slices.Equal(ttsSystemVoiceSpeechLanguages(p, p.Model, "dynamic"), []string{"zh", "ja"}) {
		t.Fatal("dynamic voice language expanded")
	}
	if _, err := ttsSnapshot(user.ID, TTSRequest{Text: "你好", SpeechLanguage: "en"}, "test"); err == nil {
		t.Fatal("unsupported en accepted")
	}
	if _, err := ttsSnapshot(user.ID, TTSRequest{Text: "你好", Instruction: "温柔"}, "test"); err == nil || !strings.Contains(err.Error(), "自由朗读指令") {
		t.Fatal("MPS instruction accepted")
	}
	// Expired data is usable locally; only the catalog endpoint may refresh it.
	ttsMPSCatalog.mu.Lock()
	entry := ttsMPSCatalog.entries[ttsMPSCatalogKey(p.CredentialScope)]
	entry.updatedAt = time.Now().Add(-11 * time.Minute)
	ttsMPSCatalog.entries[ttsMPSCatalogKey(p.CredentialScope)] = entry
	ttsMPSCatalog.mu.Unlock()
	m := model.MessageModel{StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()}, UserID: user.ID, ChannelID: identity.ChannelID, SenderIdentityID: identity.ID, ICMode: "ic", Content: "你们终于来了。"}
	TTSPrepareMessageIntent(&m, user, true)
	var count int64
	model.GetDB().Model(&model.MessageModel{}).Where("id = ?", m.ID).Count(&count)
	if m.TTSStatus != "pending" || requests.Load() != 0 || fake.calls.Load() != 0 || count != 0 {
		t.Fatal("message preparation performed I/O or persisted message")
	}
	var snapshot TTSSnapshot
	json.Unmarshal([]byte(m.TTSIntent), &snapshot)
	if snapshot.Translation == nil || snapshot.Translation.TargetLanguage != "ja" || snapshot.Input.Language != "" || snapshot.Provider.SecretID != "" || snapshot.Provider.SecretKey != "" {
		t.Fatal("bad intent")
	}
	ttsMPSCatalog.mu.Lock()
	delete(ttsMPSCatalog.entries, ttsMPSCatalogKey(p.CredentialScope))
	ttsMPSCatalog.mu.Unlock()
	m.TTSIntent, m.TTSStatus = "", ""
	TTSPrepareMessageIntent(&m, user, true)
	if m.TTSStatus != "unavailable" || requests.Load() != 0 {
		t.Fatal("missing cache queried provider")
	}
	http.DefaultTransport = ttsMPSTestTransport(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if r.Header.Get("X-TC-Action") != "DescribeVoices" {
			t.Error("prime requested synthesis")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"Response":{"ErrorCode":0,"RequestId":"prime-prepare","TotalCount":1,"Voices":[{"VoiceId":"dynamic","Category":"system","Languages":["zh","ja"]}]}}`))}, nil
	})
	TTSPrimeMPSCatalogs()
	TTSPrepareMessageIntent(&m, user, true)
	if m.TTSStatus != "pending" || requests.Load() != 1 || fake.calls.Load() != 0 {
		t.Fatal("primed preparation failed or performed provider I/O")
	}
	fake.text = "こんにちは。"
	for _, lang := range []string{"", "zh", "ja"} {
		before := fake.calls.Load()
		job, err := TTSSubmit(user.ID, "audition", TTSRequest{RequestKey: utils.NewID(), Text: "你好", SpeechLanguage: lang, Pitch: 1.3})
		if err != nil {
			t.Fatal(err)
		}
		snapshot = TTSSnapshot{}
		json.Unmarshal([]byte(job.Snapshot), &snapshot)
		want := "你好"
		if lang != "" {
			want = fake.text
			if fake.calls.Load() != before+1 {
				t.Fatal("translation skipped")
			}
			r := <-fake.requests
			if r.FeatureKey != "tts_translate" || r.Input != "目标语言代码："+lang+"\n\n你好" {
				t.Fatal("wrong full-text translation")
			}
		} else if fake.calls.Load() != before || snapshot.Translation != nil {
			t.Fatal("follow original called AI")
		}
		if snapshot.Input.Text != want || snapshot.Input.Language != lang || snapshot.Input.Pitch != 1.3 || strings.Contains(job.Snapshot, p.SecretID) || strings.Contains(job.Snapshot, p.SecretKey) {
			t.Fatal("bad translated snapshot")
		}
		wav, _ := ttsprovider.EncodePCM16MonoWAV(bytes.Repeat([]byte{1, 0}, 240), 24000)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if r.Header.Get("X-TC-Action") != "TextToSpeech" || body["Text"] != want {
				t.Error("MPS runtime failed")
			}
			if lang == "" {
				if _, ok := body["TextLang"]; ok {
					t.Error("TextLang must be omitted")
				}
			} else if body["TextLang"] != lang {
				t.Error("translation target lost")
			}
			fmt.Fprintf(w, `{"Response":{"ErrorCode":0,"RequestId":"translated","AudioData":%q}}`, base64.StdEncoding.EncodeToString(wav))
		}))
		http.DefaultTransport = previous
		snapshot.Provider.SynthesisEndpoint = server.URL
		result, err := ttsProviderSynthesize(context.Background(), p, job, snapshot, io.Discard)
		server.Close()
		if err != nil || !result.UsageConfirmed || result.Characters != TTSBillableCharacters(want) {
			t.Fatalf("runtime result=%+v err=%v", result, err)
		}
	}
}

func TestTencentMPSWorkerPartialFailureDoesNotRetry(t *testing.T) {
	t.Chdir(t.TempDir())
	model.DBInit(&utils.AppConfig{DSN: "file:mps-worker-" + utils.NewID() + "?mode=memory&cache=shared", SQLite: utils.SQLiteConfig{ReadConnections: 1}})
	if _, err := InitStorageManager(utils.StorageConfig{Mode: utils.StorageModeLocal, Local: utils.LocalStorageConfig{UploadDir: t.TempDir(), TempDir: t.TempDir()}}); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	wav, _ := ttsprovider.EncodePCM16MonoWAV(bytes.Repeat([]byte{1, 0}, 240), 24000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if r.Header.Get("X-TC-Action") != "TextToSpeech" {
			t.Error("worker queried catalog")
		}
		if n == 2 {
			fmt.Fprint(w, `{"Response":{"ErrorCode":42,"Msg":"private","RequestId":"failed-2"}}`)
		} else {
			fmt.Fprintf(w, `{"Response":{"ErrorCode":0,"RequestId":"part-1","AudioData":%q}}`, base64.StdEncoding.EncodeToString(wav))
		}
	}))
	defer server.Close()
	p := ttsMPSProviderFixture("worker")
	p.SynthesisEndpoint = server.URL
	ttsMPSPrimeTestCache(t, p)
	cfg := utils.ReadConfig()
	previous := cfg.AI
	t.Cleanup(func() { cfg.AI = previous })
	cfg.AI = utils.AIConfig{Enabled: true, Speech: &utils.SpeechConfig{Enabled: true, DefaultProvider: p.ID, DefaultVoice: "dynamic", Format: "wav", Providers: []utils.SpeechProviderConfig{p}}}
	job, err := TTSSubmit("worker-user", "audition", TTSRequest{RequestKey: utils.NewID(), Text: strings.Repeat("中", 500)})
	if err != nil || job.EstimatedUnits != 1000 {
		t.Fatalf("billing reserve=%+v err=%v", job, err)
	}
	ttsRun(context.Background(), job)
	var saved model.TTSJob
	model.GetDB().First(&saved, "id = ?", job.ID)
	if calls.Load() != 2 || saved.Status != "usage_unknown" || saved.UsageStatus != "unknown" || saved.ProviderRequestID != "failed-2" {
		t.Fatalf("worker calls=%d status=%s usage=%s id=%s", calls.Load(), saved.Status, saved.UsageStatus, saved.ProviderRequestID)
	}
	ttsRun(context.Background(), &saved)
	if calls.Load() != 2 {
		t.Fatal("ambiguous partial job submitted again")
	}
}

func TestTTSTencentMPSConfigDefaultsAndProviderIsolation(t *testing.T) {
	p := ttsMPSProviderFixture("config")
	ttsMPSPrimeTestCache(t, p)
	traditional := p
	traditional.ID, traditional.Model, traditional.SynthesisEndpoint = "traditional", "tencent-tts-classic", ttsprovider.TencentEndpoint
	cfg := utils.NormalizeSpeechConfigForWrite(&utils.SpeechConfig{Enabled: true, DefaultProvider: p.ID, DefaultVoice: "dynamic", Format: "wav", Providers: []utils.SpeechProviderConfig{traditional, p}})
	if cfg.DefaultVoice != "dynamic" || utils.NormalizeSpeechConfig(cfg).DefaultVoice != "dynamic" {
		t.Fatal("normalization erased dynamic default")
	}
	if err := utils.ValidateSpeechConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTTSDynamicDefaultVoice(cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 2 || cfg.Providers[0].CredentialScope != cfg.Providers[1].CredentialScope || cfg.Providers[0].ID == cfg.Providers[1].ID {
		t.Fatal("shared credentials merged provider instances")
	}
	for _, test := range []struct {
		voice, id, model string
		valid            bool
	}{
		{"dynamic", p.ID, p.Model, true}, {"dynamic", traditional.ID, traditional.Model, false}, {"101004", p.ID, p.Model, false}, {"101004", traditional.ID, traditional.Model, true},
	} {
		got, err := resolveSystemVoiceProvider(cfg, test.voice, "tencent", test.id, test.model)
		if (err == nil) != test.valid || (test.valid && got.ID != test.id) {
			t.Fatal("provider/model route crossed runtimes")
		}
	}
	for _, endpoint := range []string{ttsprovider.TencentEndpoint, "https://example.invalid"} {
		bad := *cfg
		bad.Providers = append([]utils.SpeechProviderConfig{}, cfg.Providers...)
		bad.Providers[1].SynthesisEndpoint = endpoint
		if utils.ValidateSpeechConfig(&bad) == nil {
			t.Fatal("MPS accepted traditional/foreign endpoint")
		}
	}
	cfg.DefaultVoice = "missing"
	if ValidateTTSDynamicDefaultVoice(cfg) == nil {
		t.Fatal("unknown dynamic default accepted")
	}
	cfg.DefaultVoice = "dynamic"
	cfg.Providers[1].SynthesisEndpoint = ttsprovider.TencentMPSEndpoint
	cfg.Providers[1].Region = "ap-guangzhou"
	if utils.ValidateSpeechConfig(cfg) == nil {
		t.Fatal("MPS Region accepted")
	}
}
