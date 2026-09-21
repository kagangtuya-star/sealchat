package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	aiService "sealchat/service/ai"
	"sealchat/model"
	"sealchat/utils"
)

func TestAITaskSSEEncoding(t *testing.T) {
	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	if !writeAITaskComment(writer, "ready") {
		t.Fatal("writeAITaskComment returned false")
	}
	if !writeAITaskSSE(writer, "result", fiber.Map{"featureKey": "polish", "result": "ok"}) {
		t.Fatal("writeAITaskSSE result returned false")
	}
	if !writeAITaskSSE(writer, "error", fiber.Map{"message": "failed"}) {
		t.Fatal("writeAITaskSSE error returned false")
	}
	want := ": ready\n\nevent: result\ndata: {\"featureKey\":\"polish\",\"result\":\"ok\"}\n\nevent: error\ndata: {\"message\":\"failed\"}\n\n"
	if got := buf.String(); got != want {
		t.Fatalf("SSE = %q, want %q", got, want)
	}
}

func TestAICapabilitiesGetIncludesFeatureRuntimeConfig(t *testing.T) {
	originalConfig := appConfig
	defer func() {
		appConfig = originalConfig
	}()

	appConfig = utils.ReadConfig()
	appConfig.AI = utils.NormalizeAIConfig(utils.AIConfig{
		Enabled: true,
		Features: map[string]utils.AIFeatureConfig{
			"polish": {
				Enabled:       true,
				DefaultPrompt: "polish prompt",
				DefaultModel:  "deepseek-v4-flash",
				Params: utils.AIModelParams{
					MaxTokens: 256,
				},
				Access: utils.AIFeatureAccessConfig{
					Mode: utils.AIFeatureAccessAll,
				},
			},
		},
	})

	app := fiber.New()
	app.Get("/ai/capabilities", func(c *fiber.Ctx) error {
		c.Locals("user", &model.UserModel{StringPKBaseModel: model.StringPKBaseModel{ID: "user-1"}})
		return AICapabilitiesGet(c)
	})

	req := httptest.NewRequest("GET", "/ai/capabilities", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d, body=%s", resp.StatusCode, fiber.StatusOK, string(raw))
	}

	var payload struct {
		Features []aiService.FeatureCapability `json:"features"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("Decode error: %v", err)
	}

	var polish *aiService.FeatureCapability
	for i := range payload.Features {
		if payload.Features[i].Key == "polish" {
			polish = &payload.Features[i]
			break
		}
	}
	if polish == nil {
		t.Fatalf("polish feature missing: %#v", payload.Features)
	}
	if !polish.Enabled {
		t.Fatalf("polish enabled = false, want true")
	}
	if polish.DefaultPrompt != "polish prompt" {
		t.Fatalf("defaultPrompt = %q, want %q", polish.DefaultPrompt, "polish prompt")
	}
	if polish.DefaultModel != "deepseek-v4-flash" {
		t.Fatalf("defaultModel = %q, want %q", polish.DefaultModel, "deepseek-v4-flash")
	}
	if polish.Params.MaxTokens != 256 {
		t.Fatalf("params.maxTokens = %d, want %d", polish.Params.MaxTokens, 256)
	}
}

func TestAITaskRunReturnsServiceUnavailableWhenUserSourceHasNoProvider(t *testing.T) {
	originalConfig := appConfig
	defer func() {
		appConfig = originalConfig
	}()

	cfg := utils.ReadConfig()
	cfg.DSN = "file::memory:?cache=shared"
	model.DBInit(cfg)
	appConfig = cfg
	appConfig.AI = utils.NormalizeAIConfig(utils.AIConfig{
		Enabled: true,
		Features: map[string]utils.AIFeatureConfig{
			"polish": {
				Enabled:       true,
				DefaultPrompt: "prompt",
				DefaultModel:  "deepseek-v4-flash",
				Access: utils.AIFeatureAccessConfig{
					Mode: utils.AIFeatureAccessAll,
				},
			},
		},
	})

	app := fiber.New()
	app.Post("/ai/tasks/:featureKey", func(c *fiber.Ctx) error {
		c.Locals("user", &model.UserModel{StringPKBaseModel: model.StringPKBaseModel{ID: "user-no-provider"}})
		return AITaskRun(c)
	})

	body, err := json.Marshal(map[string]any{
		"input":  "hello",
		"source": "user",
	})
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	req := httptest.NewRequest("POST", "/ai/tasks/polish", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusServiceUnavailable)
	}
}

func TestAITaskRunReturnsBadRequestForOversizedBattleSummaryInput(t *testing.T) {
	originalConfig := appConfig
	originalFactory := aiRunnerFactory
	defer func() {
		appConfig = originalConfig
		aiRunnerFactory = originalFactory
	}()

	appConfig = utils.ReadConfig()
	appConfig.AI = utils.NormalizeAIConfig(utils.AIConfig{
		Enabled: true,
		Providers: []utils.AIProviderConfig{{
			ID:      "deepseek-default",
			Name:    "DeepSeek",
			Enabled: true,
			BaseURL: "https://api.deepseek.com/v1",
			APIKey:  "secret",
			Models:  []string{"deepseek-v4-flash"},
			Weight:  1,
		}},
		Features: map[string]utils.AIFeatureConfig{
			"battle_summary": {
				Enabled:       true,
				DefaultPrompt: "summary",
				DefaultModel:  "deepseek-v4-flash",
				Params: utils.AIModelParams{
					MaxInputChars: 100,
				},
				Access: utils.AIFeatureAccessConfig{
					Mode: utils.AIFeatureAccessAll,
				},
			},
		},
	})
	aiRunnerFactory = func(_ func() *utils.AppConfig) aiTaskRunner {
		return aiTaskRunnerFunc(func(_ context.Context, req aiService.RunRequest) (aiService.RunResult, error) {
			return aiService.RunResult{}, aiService.ErrInputTooLong
		})
	}

	app := fiber.New()
	app.Post("/ai/tasks/:featureKey", func(c *fiber.Ctx) error {
		c.Locals("user", &model.UserModel{StringPKBaseModel: model.StringPKBaseModel{ID: "user-1"}})
		return AITaskRun(c)
	})

	body, err := json.Marshal(map[string]any{
		"input":  strings.Repeat("字", 101),
		"source": "user",
	})
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	req := httptest.NewRequest("POST", "/ai/tasks/battle_summary", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusBadRequest {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d, body=%s", resp.StatusCode, fiber.StatusBadRequest, string(raw))
	}
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if !strings.Contains(payload.Message, "战报总结输入过长") {
		t.Fatalf("message = %q, want battle summary business message", payload.Message)
	}
}

type aiTaskRunnerFunc func(ctx context.Context, req aiService.RunRequest) (aiService.RunResult, error)

func (fn aiTaskRunnerFunc) Run(ctx context.Context, req aiService.RunRequest) (aiService.RunResult, error) {
	return fn(ctx, req)
}

func TestAdminAIConfigUpdatePersistsIncomingProviderAPIKey(t *testing.T) {
	originalConfig := appConfig
	originalWd, _ := os.Getwd()
	defer func() {
		appConfig = originalConfig
		if originalWd != "" {
			_ = os.Chdir(originalWd)
		}
	}()

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir error: %v", err)
	}
	appConfig = utils.ReadConfig()
	appConfig.DSN = "file::memory:?cache=shared"
	model.DBInit(appConfig)
	appConfig.AI = utils.NormalizeAIConfig(utils.AIConfig{
		Enabled: true,
		Providers: []utils.AIProviderConfig{{
			ID:      "deepseek-default",
			Name:    "DeepSeek",
			Enabled: true,
			BaseURL: "https://api.deepseek.com/v1",
			APIKey:  "",
			Models:  []string{"deepseek-v4-flash"},
			Weight:  1,
		}},
		Features: map[string]utils.AIFeatureConfig{
			"polish": {
				Enabled:       true,
				DefaultPrompt: "prompt",
				DefaultModel:  "deepseek-v4-flash",
				Access: utils.AIFeatureAccessConfig{Mode: utils.AIFeatureAccessAll},
			},
		},
	})

	app := fiber.New()
	app.Put("/admin/ai/config", AdminAIConfigUpdate)

	body, err := json.Marshal(map[string]any{
		"config": map[string]any{
			"enabled": true,
			"routing": map[string]any{"mode": "round_robin"},
			"retry": map[string]any{
				"maxAttempts":    2,
				"initialDelayMs": 300,
				"maxDelayMs":     3000,
			},
			"providers": []map[string]any{{
				"id":      "deepseek-default",
				"name":    "DeepSeek",
				"enabled": true,
				"baseUrl": "https://api.deepseek.com/v1",
				"apiKey":  "new-secret",
				"models":  []string{"deepseek-v4-flash"},
				"weight":  1,
			}},
			"features": map[string]any{
				"polish": map[string]any{
					"enabled":       true,
					"defaultPrompt": "prompt",
					"defaultModel":  "deepseek-v4-flash",
					"params":        map[string]any{},
					"access": map[string]any{
						"mode":     "all",
						"userIds":  []string{},
						"worldIds": []string{},
					},
				},
				"battle_summary": map[string]any{
					"enabled":       false,
					"defaultPrompt": "summary",
					"defaultModel":  "deepseek-v4-flash",
					"params":        map[string]any{},
					"access": map[string]any{
						"mode":     "all",
						"userIds":  []string{},
						"worldIds": []string{},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	req := httptest.NewRequest("PUT", "/admin/ai/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d, body=%s", resp.StatusCode, fiber.StatusOK, string(raw))
	}
	if got := appConfig.AI.Providers[0].APIKey; got != "new-secret" {
		t.Fatalf("APIKey = %q, want %q", got, "new-secret")
	}
	rawConfig, err := os.ReadFile(filepath.Join(tmpDir, "config.yaml"))
	if err != nil {
		t.Fatalf("ReadFile config.yaml error: %v", err)
	}
	if !bytes.Contains(rawConfig, []byte("apiKey: new-secret")) {
		t.Fatalf("config.yaml missing apiKey, got:\n%s", string(rawConfig))
	}
}
