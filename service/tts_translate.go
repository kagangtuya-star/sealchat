package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"sealchat/model"
	"sealchat/service/ai"
	"sealchat/utils"
)

var ttsTranslateRunnerFactory = func(cfg utils.AIConfig) ai.TaskRunner {
	app := utils.GetConfig()
	if app == nil {
		return nil
	}
	snapshot := *app
	snapshot.AI = cfg
	return ai.NewRunner(func() *utils.AppConfig { return &snapshot }, nil)
}

// TTSTranslationError means the pre-TTS platform AI step failed before a TTS job exists.
// It must not be reported as an uncertain speech-provider submission.
type TTSTranslationError struct{ Message string }

func (e *TTSTranslationError) Error() string { return e.Message }

func ttsTranslationAIConfig(raw utils.AIConfig) (utils.AIConfig, error) {
	cfg := utils.NormalizeAIConfig(raw)
	feature := cfg.Features[ai.FeatureTTSTranslate]
	model := strings.TrimSpace(feature.DefaultModel)
	compatible := func(provider utils.AIProviderConfig, model string) bool {
		return provider.Enabled && model != "" && slices.Contains(provider.Models, model)
	}
	selectProviders := func(model string) []utils.AIProviderConfig {
		providers := make([]utils.AIProviderConfig, 0, len(cfg.Providers))
		for _, provider := range cfg.Providers {
			if compatible(provider, model) {
				providers = append(providers, provider)
			}
		}
		return providers
	}
	providers := selectProviders(model)
	if len(providers) == 0 {
		for _, provider := range cfg.Providers {
			if !provider.Enabled || len(provider.Models) == 0 {
				continue
			}
			candidate := strings.TrimSpace(provider.SelectedModel)
			if !slices.Contains(provider.Models, candidate) {
				candidate = strings.TrimSpace(provider.Models[0])
			}
			if candidate == "" {
				continue
			}
			model = candidate
			providers = selectProviders(model)
			break
		}
	}
	if model == "" || len(providers) == 0 {
		return cfg, &TTSTranslationError{Message: "平台语音翻译没有可用的后端 AI 模型"}
	}
	feature.DefaultModel = model
	cfg.Features[ai.FeatureTTSTranslate] = feature
	cfg.Providers = providers
	return cfg, nil
}

func ttsTranslationRunError(err error) error {
	var quota *ai.AIQuotaExceededError
	switch {
	case errors.As(err, &quota):
		return &TTSTranslationError{Message: "平台 AI 额度不足，无法进行语音翻译"}
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		return &TTSTranslationError{Message: "平台语音翻译请求超时，请稍后重试"}
	case strings.Contains(err.Error(), "no ai provider available"):
		return &TTSTranslationError{Message: "平台语音翻译没有可用的后端 AI provider"}
	default:
		return &TTSTranslationError{Message: "平台语音翻译调用失败，请检查后端 AI provider 与模型配置"}
	}
}

// Separate from synthesis slots: slow text AI must not occupy a speech worker.
var ttsTranslationSlots = make(chan struct{}, 2)

var ttsTranslationLanguageCode = regexp.MustCompile(`^[a-z0-9-]+$`)

// Role auditions share the automatic message's world access policy. Identity
// context is authorized on the server rather than trusting a client world ID.
func ttsAuditionWorldID(userID, identityID string) (string, error) {
	if identityID == "" {
		return "", nil
	}
	if _, err := TTSRoleConfig(userID, identityID); err != nil {
		return "", err
	}
	var identity model.ChannelIdentityModel
	var channel model.ChannelModel
	db := model.GetDB()
	if err := db.Where("id = ? AND deleted_at IS NULL", identityID).First(&identity).Error; err != nil {
		return "", err
	}
	if err := db.Where("id = ? AND deleted_at IS NULL", identity.ChannelID).First(&channel).Error; err != nil {
		return "", err
	}
	return channel.WorldID, nil
}

func ttsTranslationTimeout() time.Duration {
	seconds := 60
	if cfg := utils.GetConfig(); cfg != nil {
		seconds = utils.NormalizeAIConfig(cfg.AI).RequestTimeoutSeconds
	}
	return max(2*time.Minute, time.Duration(seconds)*time.Second+30*time.Second)
}

func ttsTranslateText(ctx context.Context, user *model.UserModel, worldID, targetLanguage, sourceText string) (string, error) {
	// Voice support is validated when creating the snapshot; only guard the input header here.
	targetLanguage = strings.ToLower(strings.TrimSpace(targetLanguage))
	if len(targetLanguage) == 0 || len(targetLanguage) > 16 || !ttsTranslationLanguageCode.MatchString(targetLanguage) || user == nil {
		return "", TTSValidationError("无效的朗读语言或用户")
	}
	cfg := utils.GetConfig()
	if cfg == nil || !ai.IsFeatureAvailable(cfg.AI, ai.FeatureTTSTranslate, user.ID, worldID) {
		return "", TTSValidationError("平台语音翻译未启用或无权使用")
	}
	aiCfg, err := ttsTranslationAIConfig(cfg.AI)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(sourceText) == "" || utf8.RuneCountInString(sourceText) > ttsMessageSynthesisMaxRunes {
		return "", TTSValidationError("语音翻译原文须为 1–20000 字符")
	}
	ctx, cancel := context.WithTimeout(ctx, ttsTranslationTimeout()-15*time.Second)
	defer cancel()
	output, err := ai.RunTaskWithBilling(ctx, ai.BilledRunInput{
		Config: aiCfg, User: user, WorldID: worldID,
		FeatureKey: ai.FeatureTTSTranslate, Source: "platform",
		Input:  "目标语言代码：" + targetLanguage + "\n\n" + sourceText,
		Runner: ttsTranslateRunnerFactory(aiCfg),
	})
	if err != nil {
		return "", ttsTranslationRunError(err)
	}
	text := strings.TrimSpace(output.Result.Result)
	if text == "" || utf8.RuneCountInString(text) > ttsMessageSynthesisMaxRunes {
		return "", TTSValidationError("语音翻译结果为空或超过 20000 字符")
	}
	return text, nil
}

func ttsCompleteTranslation(s *TTSSnapshot, text string) {
	s.Translation.State = "done"
	s.Translation.StartedAt = 0
	s.Input.Text, s.Input.Language = text, s.Translation.TargetLanguage
	s.Fingerprint = ""
	raw, _ := json.Marshal(s)
	s.Fingerprint = ttsHash(string(raw))
}

// All state transitions match the exact intent and revision, including revoke
// and delete flags. No transaction is held across the AI request.
func ttsTranslationMessageCAS(db *gorm.DB, m model.MessageModel) *gorm.DB {
	return db.Model(&model.MessageModel{}).Where(
		"id = ? AND tts_status = ? AND edit_count = ? AND tts_intent = ? AND is_deleted = ? AND (is_revoked = ? OR is_revoked IS NULL) AND deleted_at IS NULL",
		m.ID, "pending", m.EditCount, m.TTSIntent, false, false)
}

func ttsFailTranslation(db *gorm.DB, m model.MessageModel) {
	_ = ttsTranslationMessageCAS(db, m).Update("tts_status", "unavailable").Error
}

func ttsFinalizeTranslation(ctx context.Context, db *gorm.DB, m model.MessageModel, s TTSSnapshot) {
	job := &model.TTSJob{MessageID: m.ID, ChannelID: m.ChannelID, PayerUserID: m.UserID, MessageRevision: int64(m.EditCount), Snapshot: m.TTSIntent}
	if !ttsMessageCurrentWithDB(db, job, s) {
		return
	}
	var user model.UserModel
	var channel model.ChannelModel
	if db.Where("id = ? AND deleted_at IS NULL", m.UserID).First(&user).Error != nil ||
		db.Where("id = ? AND deleted_at IS NULL", m.ChannelID).First(&channel).Error != nil {
		ttsFailTranslation(db, m)
		return
	}
	text, err := ttsTranslateText(ctx, &user, channel.WorldID, s.Translation.TargetLanguage, s.SourceText)
	if err != nil {
		ttsFailTranslation(db, m)
		return
	}
	if !ttsMessageCurrentWithDB(db, job, s) {
		return
	}
	ttsCompleteTranslation(&s, text)
	raw, err := json.Marshal(s)
	if err != nil {
		ttsFailTranslation(db, m)
		return
	}
	result := ttsTranslationMessageCAS(db, m).Update("tts_intent", string(raw))
	if result.Error == nil && result.RowsAffected == 1 {
		TTSWake()
	}
}

// Returns true whenever translation still owns the outbox item.
func ttsRecoverTranslation(ctx context.Context, db *gorm.DB, m model.MessageModel, s TTSSnapshot) bool {
	if s.Translation == nil {
		return false
	}
	switch s.Translation.State {
	case "done":
		return false
	case "running":
		if s.Translation.StartedAt <= 0 || time.Since(time.Unix(s.Translation.StartedAt, 0)) >= ttsTranslationTimeout() {
			// Interrupted work may already be billed. Never replay it.
			ttsFailTranslation(db, m)
		}
	case "pending":
		select {
		case ttsTranslationSlots <- struct{}{}:
		default:
			return true
		}
		s.Translation.State, s.Translation.StartedAt = "running", time.Now().Unix()
		raw, err := json.Marshal(s)
		if err != nil {
			<-ttsTranslationSlots
			return true
		}
		result := ttsTranslationMessageCAS(db, m).Update("tts_intent", string(raw))
		if result.Error != nil || result.RowsAffected != 1 {
			<-ttsTranslationSlots
			return true
		}
		m.TTSIntent = string(raw)
		go func() {
			defer func() { <-ttsTranslationSlots; TTSWake() }()
			ttsFinalizeTranslation(ctx, db, m, s)
		}()
	default:
		ttsFailTranslation(db, m)
	}
	return true
}
