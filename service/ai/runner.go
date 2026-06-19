package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"sealchat/utils"
)

type CompletionRequest struct {
	Model        string
	SystemPrompt string
	UserInput    string
	Params       utils.AIModelParams
}

type CompletionResult struct {
	Text       string
	Model      string
	Usage      RunUsage
	StartedAt  time.Time
	FinishedAt time.Time
}

type ChatClient interface {
	Complete(ctx context.Context, req CompletionRequest) (CompletionResult, error)
}

type RunRequest struct {
	FeatureKey string
	UserID     string
	WorldID    string
	Input      string
	Source     string
}

type RunUsage struct {
	PromptTokens     int64
	CompletionTokens int64
	CacheTokens      int64
}

type RunResult struct {
	FeatureKey string
	Result     string
	Model      string
	ProviderID string
	Usage      RunUsage
	StartedAt  time.Time
	FinishedAt time.Time
}

type ProviderSelector struct{}

func (s *ProviderSelector) EnabledProviders(cfg utils.AIConfig) []utils.AIProviderConfig {
	out := make([]utils.AIProviderConfig, 0, len(cfg.Providers))
	for _, provider := range cfg.Providers {
		if provider.Enabled {
			out = append(out, provider)
		}
	}
	return out
}

func (s *ProviderSelector) OrderedProviders(cfg utils.AIConfig, offset int) []utils.AIProviderConfig {
	providers := s.EnabledProviders(cfg)
	if len(providers) <= 1 {
		return providers
	}
	offset = offset % len(providers)
	if offset == 0 {
		return providers
	}
	ordered := make([]utils.AIProviderConfig, 0, len(providers))
	ordered = append(ordered, providers[offset:]...)
	ordered = append(ordered, providers[:offset]...)
	return ordered
}

type Runner struct {
	configProvider func() *utils.AppConfig
	selector       *ProviderSelector
	clientFactory  func(provider utils.AIProviderConfig) ChatClient
	mu             sync.Mutex
	nextProvider   int
}

func NewRunner(cfgProvider func() *utils.AppConfig, clientFactory func(provider utils.AIProviderConfig) ChatClient) *Runner {
	factory := clientFactory
	if factory == nil {
		factory = func(provider utils.AIProviderConfig) ChatClient {
			return newOpenAIChatClient(provider)
		}
	}
	return &Runner{
		configProvider: cfgProvider,
		selector:       &ProviderSelector{},
		clientFactory:  factory,
	}
}

func (r *Runner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	if r == nil || r.configProvider == nil {
		return RunResult{}, errors.New("ai runner unavailable")
	}
	appCfg := r.configProvider()
	if appCfg == nil {
		return RunResult{}, errors.New("ai config unavailable")
	}
	aiCfg := utils.NormalizeAIConfig(appCfg.AI)
	if !IsFeatureAvailable(aiCfg, req.FeatureKey, req.UserID, req.WorldID) {
		return RunResult{}, errors.New("ai feature unavailable")
	}
	definition, ok := BuiltinFeatures()[req.FeatureKey]
	if !ok {
		return RunResult{}, fmt.Errorf("unknown ai feature: %s", req.FeatureKey)
	}
	featureCfg := aiCfg.Features[req.FeatureKey]
	maxInputChars := featureCfg.Params.MaxInputChars
	if maxInputChars <= 0 {
		maxInputChars = definition.InputMaxChars
	}
	input := strings.TrimSpace(req.Input)
	if input == "" {
		return RunResult{}, errors.New("ai input required")
	}
	currentChars := len([]rune(input))
	if maxInputChars > 0 && currentChars > maxInputChars {
		return RunResult{}, FormatInputTooLongError(req.FeatureKey, currentChars, maxInputChars)
	}
	if strings.EqualFold(strings.TrimSpace(req.Source), "user") {
		userProviders, err := loadUserProviders(req.UserID)
		if err != nil {
			return RunResult{}, err
		}
		if len(userProviders) == 0 {
			return RunResult{}, errors.New("no ai provider available")
		}
		aiCfg.Providers = userProviders
	}
	startOffset := r.nextProviderOffset(aiCfg)
	providers := r.selector.OrderedProviders(aiCfg, startOffset)
	if len(providers) == 0 {
		return RunResult{}, errors.New("no ai provider available")
	}
	var lastErr error
	for _, provider := range providers {
		client := r.clientFactory(provider)
		if client == nil {
			lastErr = fmt.Errorf("provider %s client unavailable", provider.ID)
			continue
		}
		model := featureCfg.DefaultModel
		if strings.EqualFold(strings.TrimSpace(req.Source), "user") && strings.TrimSpace(provider.SelectedModel) != "" {
			model = strings.TrimSpace(provider.SelectedModel)
		}
		if model == "" && len(provider.Models) > 0 {
			model = provider.Models[0]
		}
		result, err := r.completeWithRetry(ctx, client, CompletionRequest{
			Model:        model,
			SystemPrompt: featureCfg.DefaultPrompt,
			UserInput:    input,
			Params:       featureCfg.Params,
		}, aiCfg.Retry)
		if err == nil {
			if result.Model == "" {
				result.Model = model
			}
			return RunResult{
				FeatureKey: req.FeatureKey,
				Result:     result.Text,
				Model:      result.Model,
				ProviderID: provider.ID,
				Usage:      result.Usage,
				StartedAt:  result.StartedAt,
				FinishedAt: result.FinishedAt,
			}, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no ai provider available")
	}
	return RunResult{}, lastErr
}

func (r *Runner) nextProviderOffset(cfg utils.AIConfig) int {
	enabledCount := len(r.selector.EnabledProviders(cfg))
	if enabledCount <= 1 {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	offset := r.nextProvider % enabledCount
	r.nextProvider = (r.nextProvider + 1) % enabledCount
	return offset
}

func (r *Runner) completeWithRetry(ctx context.Context, client ChatClient, req CompletionRequest, retry utils.AIRetryConfig) (CompletionResult, error) {
	attempts := retry.MaxAttempts
	if attempts <= 0 {
		attempts = 1
	}
	delay := retry.InitialDelayMs
	if delay <= 0 {
		delay = 1
	}
	maxDelay := retry.MaxDelayMs
	if maxDelay <= 0 {
		maxDelay = delay
	}
	if maxDelay < delay {
		maxDelay = delay
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		result, err := client.Complete(ctx, req)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if attempt == attempts {
			break
		}
		waitMs := delay
		if waitMs > maxDelay {
			waitMs = maxDelay
		}
		timer := time.NewTimer(time.Duration(waitMs) * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return CompletionResult{}, ctx.Err()
		case <-timer.C:
		}
		if delay < maxDelay {
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}
	if lastErr == nil {
		lastErr = errors.New("ai request failed")
	}
	return CompletionResult{}, lastErr
}

type openAIChatClient struct {
	client *openai.Client
}

func newOpenAIChatClient(provider utils.AIProviderConfig) ChatClient {
	config := openai.DefaultConfig(provider.APIKey)
	config.BaseURL = provider.BaseURL
	return &openAIChatClient{client: openai.NewClientWithConfig(config)}
}

func (c *openAIChatClient) Complete(ctx context.Context, req CompletionRequest) (CompletionResult, error) {
	startedAt := time.Now()
	request := openai.ChatCompletionRequest{
		Model: req.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: req.SystemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: req.UserInput},
		},
		MaxTokens: req.Params.MaxTokens,
	}
	if req.Params.Temperature != nil {
		request.Temperature = *req.Params.Temperature
	}
	if req.Params.TopP != nil {
		request.TopP = *req.Params.TopP
	}
	resp, err := c.client.CreateChatCompletion(ctx, request)
	if err != nil {
		return CompletionResult{}, err
	}
	if len(resp.Choices) == 0 {
		return CompletionResult{}, errors.New("empty ai response")
	}
	cacheTokens := int64(0)
	if resp.Usage.PromptTokensDetails != nil {
		cacheTokens = int64(resp.Usage.PromptTokensDetails.CachedTokens)
	}
	return CompletionResult{
		Text:  resp.Choices[0].Message.Content,
		Model: resp.Model,
		Usage: RunUsage{
			PromptTokens:     int64(resp.Usage.PromptTokens),
			CompletionTokens: int64(resp.Usage.CompletionTokens),
			CacheTokens:      cacheTokens,
		},
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
	}, nil
}
