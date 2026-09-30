package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sealchat/model"
	"sealchat/pkg/ttsprovider"
	aiService "sealchat/service/ai"
	"sealchat/utils"
)

type TTSTicket struct {
	UserID     string
	ResourceID string
	MessageID  string
	ChannelID  string
	Purpose    string
	Expires    time.Time
}

var ttsTickets = struct {
	sync.Mutex
	values map[string]TTSTicket
}{values: map[string]TTSTicket{}}

func TTSIssueTicket(t TTSTicket) (string, error) {
	ttsTickets.Lock()
	defer ttsTickets.Unlock()
	now := time.Now()
	count := 0
	for k, v := range ttsTickets.values {
		if now.After(v.Expires) {
			delete(ttsTickets.values, k)
		} else if v.UserID == t.UserID {
			count++
		}
	}
	if count >= 64 || len(ttsTickets.values) >= 4096 {
		return "", TTSValidationError("播放授权请求过多")
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	key := hex.EncodeToString(b)
	ttsTickets.values[key] = t
	return key, nil
}
func TTSReadTicket(key string, consume bool) (TTSTicket, bool) {
	ttsTickets.Lock()
	defer ttsTickets.Unlock()
	t, ok := ttsTickets.values[key]
	if !ok || time.Now().After(t.Expires) {
		delete(ttsTickets.values, key)
		return TTSTicket{}, false
	}
	if consume {
		delete(ttsTickets.values, key)
	}
	return t, true
}
func TTSCloneReadURL(userID, resourceID string) (string, error) {
	var a model.AttachmentModel
	if model.GetDB().Where("id = ? AND user_id = ? AND root_id_type = ? AND parent_id_type = ? AND deleted_at IS NULL", resourceID, userID, "tts", "clone_source").First(&a).Error != nil {
		return "", ErrTTSDenied
	}
	cfg := utils.GetConfig()
	base := strings.TrimRight(cfg.Domain, "/")
	if !strings.HasPrefix(base, "https://") {
		return "", TTSValidationError("复刻样本读取需要配置公开 HTTPS Domain")
	}
	token, err := TTSIssueTicket(TTSTicket{UserID: userID, ResourceID: resourceID, Purpose: "clone_source", Expires: time.Now().Add(5 * time.Minute)})
	if err != nil {
		return "", err
	}
	webRoot := strings.Trim(cfg.WebUrl, "/")
	if webRoot != "" {
		base += "/" + webRoot
	}
	return base + "/api/v1/tts/play/" + token, nil
}

var ttsMaintenanceAt time.Time

func ttsMaintainVoices(parent context.Context) {
	if time.Since(ttsMaintenanceAt) < 15*time.Second {
		return
	}
	ttsMaintenanceAt = time.Now()
	cfg := utils.GetConfig()
	if cfg == nil || cfg.AI.Speech == nil {
		return
	}
	db := model.GetDB()
	// Cache eligibility is temporary; jobs/messages keep their own file refs.
	_ = db.Where("expires_at <= ?", time.Now()).Delete(&model.TTSCache{}).Error
	var voices []model.TTSVoice
	if db.Where("deleted_at IS NULL AND (lifecycle = ? OR (lifecycle IN ? AND (provider_status <> ? OR preview_expires_at <= ?)))", "delete_pending", []string{"creating", "preview"}, "OK", time.Now()).Order("updated_at ASC, id ASC").Limit(20).Find(&voices).Error != nil {
		return
	}
	for _, v := range voices {
		// Failed cloud queries/deletions must not starve later rows forever.
		_ = db.Model(&model.TTSVoice{}).Where("id = ? AND lifecycle = ?", v.ID, v.Lifecycle).UpdateColumn("updated_at", time.Now()).Error
		if v.PreviewExpiresAt != nil && !time.Now().Before(*v.PreviewExpiresAt) && v.Lifecycle != "delete_pending" {
			_ = TTSExpirePreviews(db, v.OwnerUserID, time.Now())
			continue
		}
		if v.ProviderVoiceID == "" {
			continue
		}
		var provider *utils.SpeechProviderConfig
		for _, p := range cfg.AI.Speech.Providers {
			if p.ID == v.ProviderID && p.CredentialScope == v.CredentialScope && p.Region == v.Region && p.Workspace == v.Workspace {
				cp := p
				provider = &cp
				break
			}
		}
		if provider == nil {
			continue
		}
		client := ttsprovider.Client{APIKey: provider.APIKey, VoiceEndpoint: provider.VoiceEndpoint}
		ctx, cancel := context.WithTimeout(parent, 10*time.Second)
		if v.Lifecycle == "delete_pending" {
			var refs int64
			err := db.Model(&model.TTSJob{}).Where("voice_id = ? AND status IN ?", v.ID, []string{"queued", "running", "usage_unknown"}).Count(&refs).Error
			if err == nil && refs == 0 {
				if _, err = client.DeleteVoice(ctx, v.ProviderVoiceID); err == nil {
					_ = db.Model(&model.TTSVoice{}).Where("id = ? AND lifecycle = ?", v.ID, "delete_pending").Updates(map[string]any{"lifecycle": "deleted", "deleted_at": time.Now()}).Error
				}
			}
		} else if v.ProviderStatus != "OK" {
			r, err := client.QueryVoice(ctx, v.ProviderVoiceID)
			if err == nil && r.Output.TargetModel == v.TargetModel {
				updates := map[string]any{"provider_status": r.Output.Status}
				if r.Output.Status == "OK" || r.Output.Status == "UNDEPLOYED" {
					updates["lifecycle"] = "preview"
				}
				_ = db.Model(&model.TTSVoice{}).Where("id = ? AND lifecycle IN ?", v.ID, []string{"creating", "preview"}).Updates(updates).Error
			}
		}
		cancel()
	}
}

type TTSQuota struct {
	Enabled          bool                          `json:"enabled"`
	AutoSynthesis    bool                          `json:"autoSynthesis"`
	Policy           utils.AIQuotaPolicyConfig     `json:"policy"`
	Usage            *aiService.QuotaUsageSnapshot `json:"usage"`
	Saved            int64                         `json:"saved"`
	Slots            int                           `json:"slots"`
	Format           string                        `json:"format"`
	CharacterPrice   *float64                      `json:"characterPrice"`
	PricingMode      string                        `json:"pricingMode"`
	InputTokenPrice  *float64                      `json:"inputTokenPrice"`
	OutputTokenPrice *float64                      `json:"outputTokenPrice"`
	DesignPrice      *float64                      `json:"designPrice"`
	ClonePrice       *float64                      `json:"clonePrice"`
	DefaultModel     string                        `json:"defaultModel"`
	VoiceContext     TTSVoiceContext               `json:"voiceContext"`
	DefaultVoice     string                        `json:"defaultVoice"`
}

type TTSVoiceContext struct {
	ProviderKind string `json:"providerKind"`
	ProviderID   string `json:"providerId"`
	ModelID      string `json:"modelId"`
}

func TTSQuotaForUser(userID string) (TTSQuota, error) {
	q := TTSQuota{AutoSynthesis: TTSAutomaticEnabled(userID)}
	cfg := utils.GetConfig()
	var speech *utils.SpeechConfig
	if cfg != nil {
		speech = utils.NormalizeSpeechConfig(cfg.AI.Speech)
		q.Enabled = cfg.AI.Enabled && speech != nil && speech.Enabled
	}
	if speech != nil {
		q.Policy = speech.QuotaDefault
		q.Slots = speech.DefaultSlots
		q.Format = speech.Format
		q.DefaultVoice = speech.DefaultVoice
		for _, p := range speech.Providers {
			if p.ID == speech.DefaultProvider {
				q.VoiceContext = TTSVoiceContext{ProviderKind: p.EffectiveProviderKind(), ProviderID: p.ID, ModelID: p.Model}
				q.DefaultModel = p.Model
				q.CharacterPrice = p.CharacterPrice
				q.PricingMode = p.EffectivePricingMode()
				q.InputTokenPrice, q.OutputTokenPrice = p.InputTokenPrice, p.OutputTokenPrice
				q.DesignPrice = p.DesignPrice
				q.ClonePrice = p.ClonePrice
			}
		}
	}
	var p model.TTSUserPolicy
	err := model.GetDB().Where("user_id = ? AND deleted_at IS NULL", userID).First(&p).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return q, err
	}
	if p.OverrideEnabled {
		q.Policy = utils.AIQuotaPolicyConfig{DailyLimit: p.DailyLimit, MonthlyLimit: p.MonthlyLimit, LifetimeLimit: p.LifetimeLimit}
	}
	if p.Slots != nil {
		q.Slots = *p.Slots
	}
	q.Usage, err = aiService.QueryQuotaUsageSnapshotForKind(model.GetDB(), model.QuotaKindSpeech, userID, time.Now())
	if err != nil {
		return q, err
	}
	err = model.GetDB().Model(&model.TTSVoice{}).Where("owner_user_id = ? AND lifecycle = ? AND deleted_at IS NULL", userID, "saved").Count(&q.Saved).Error
	return q, err
}

func TTSRoleConfig(userID, identityID string) (*model.ChannelIdentityTTSConfig, error) {
	var identity model.ChannelIdentityModel
	if err := model.GetDB().Where("id = ? AND deleted_at IS NULL", identityID).First(&identity).Error; err != nil {
		return nil, err
	}
	actor, err := ResolveChannelIdentityActor(identity.ChannelID, userID, identity.UserID)
	if err != nil {
		return nil, err
	}
	if _, err = ValidateChannelIdentityActorIdentity(actor, identity.ChannelID, identityID); err != nil {
		return nil, err
	}
	var role model.ChannelIdentityTTSConfig
	err = model.GetDB().Where("identity_id = ? AND deleted_at IS NULL", identityID).First(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &model.ChannelIdentityTTSConfig{IdentityID: identityID, Rate: 1, Pitch: 1, Volume: 50}, nil
	}
	return &role, err
}
func TTSSaveRoleConfig(userID, identityID string, input model.ChannelIdentityTTSConfig) error {
	old, err := TTSRoleConfig(userID, identityID)
	if err != nil {
		return err
	}
	// Delegation authorizes managing the identity, not reading a third party's
	// private voice. Enforce this independently of platform availability.
	if input.VoiceID != "" {
		if _, err := ttsAccessibleVoice(model.GetDB(), userID, input.VoiceID); err != nil {
			return err
		}
	}
	if input.VoiceID != "" || input.SystemVoice != "" {
		s, err := ttsSnapshot(userID, TTSRequest{VoiceID: input.VoiceID, SystemVoice: input.SystemVoice, SystemVoiceProvider: input.SystemVoiceProvider, SystemVoiceModel: input.SystemVoiceModel, Instruction: input.Instruction, Rate: input.Rate, Pitch: input.Pitch, Volume: &input.Volume}, "role")
		if err != nil {
			return err
		}
		if input.SystemVoice != "" {
			input.SystemVoiceProvider = s.Provider.EffectiveProviderKind()
			input.SystemVoiceModel = s.Provider.Model
		}
	}
	if input.SystemVoice == "" {
		input.SystemVoiceProvider, input.SystemVoiceModel = "", ""
	}
	if input.Revision != old.Revision {
		return ErrTTSConflict
	}
	input.IdentityID = identityID
	input.Revision = old.Revision + 1
	if old.ID == "" {
		input.StringPKBaseModel = model.StringPKBaseModel{}
		return model.GetDB().Create(&input).Error
	}
	r := model.GetDB().Model(&model.ChannelIdentityTTSConfig{}).Where("id = ? AND revision = ?", old.ID, old.Revision).Updates(map[string]any{"voice_id": input.VoiceID, "system_voice": input.SystemVoice, "system_voice_provider": input.SystemVoiceProvider, "system_voice_model": input.SystemVoiceModel, "instruction": input.Instruction, "rate": input.Rate, "pitch": input.Pitch, "volume": input.Volume, "revision": input.Revision})
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return ErrTTSConflict
	}
	return nil
}

func TTSUpdateVoice(userID, id string, input model.TTSVoice) error {
	if len(input.Name) > 200 || len(input.Tags) > 500 || len(input.Description) > 2000 {
		return TTSValidationError("音色元数据过长")
	}
	var params TTSRequest
	if input.Parameters != "" && json.Unmarshal([]byte(input.Parameters), &params) != nil {
		return TTSValidationError("无效的默认合成参数")
	}
	if (params.Rate != 0 && (params.Rate < 0.5 || params.Rate > 2)) || (params.Pitch != 0 && (params.Pitch < 0.5 || params.Pitch > 2)) || (params.Volume != nil && (*params.Volume < 0 || *params.Volume > 100)) || len([]rune(params.Instruction)) > 500 {
		return TTSValidationError("默认合成参数超出范围")
	}
	if input.Parameters != "" {
		// Persist only the supported local defaults, never arbitrary client fields.
		raw, err := json.Marshal(struct {
			Instruction string  `json:"instruction,omitempty"`
			Rate        float64 `json:"rate,omitempty"`
			Pitch       float64 `json:"pitch,omitempty"`
			Volume      *int    `json:"volume,omitempty"`
		}{params.Instruction, params.Rate, params.Pitch, params.Volume})
		if err != nil {
			return err
		}
		input.Parameters = string(raw)
	}
	return withTTSUserPolicy(model.GetDB(), userID, func(tx *gorm.DB, _ *model.TTSUserPolicy) error {
		r := tx.Model(&model.TTSVoice{}).Where("id = ? AND owner_user_id = ? AND lifecycle IN ? AND deleted_at IS NULL AND revision = ?", id, userID, []string{"saved", "preview"}, input.Revision).Updates(map[string]any{"name": input.Name, "tags": input.Tags, "description": input.Description, "parameters": input.Parameters, "is_public": input.IsPublic, "revision": gorm.Expr("revision + 1")})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrTTSConflict
		}
		return nil
	})
}
func TTSDeleteVoice(userID, id string) error {
	return withTTSUserPolicy(model.GetDB(), userID, func(tx *gorm.DB, _ *model.TTSUserPolicy) error {
		r := tx.Model(&model.TTSVoice{}).Where("id = ? AND owner_user_id = ? AND deleted_at IS NULL", id, userID).Updates(map[string]any{"lifecycle": "delete_pending", "is_public": false, "revision": gorm.Expr("revision + 1")})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrTTSDenied
		}
		return nil
	})
}

func TTSSetPolicy(userID string, input model.TTSUserPolicy) error {
	if input.Slots != nil && *input.Slots < 0 {
		return TTSValidationError("槽位不能为负数")
	}
	for _, v := range []*float64{input.DailyLimit, input.MonthlyLimit, input.LifetimeLimit} {
		if v != nil && *v < 0 {
			return TTSValidationError("限制不能为负数")
		}
	}
	return withTTSUserPolicy(model.GetDB(), userID, func(tx *gorm.DB, _ *model.TTSUserPolicy) error {
		return tx.Model(&model.TTSUserPolicy{}).Where("user_id = ?", userID).Updates(map[string]any{"override_enabled": input.OverrideEnabled, "daily_limit": input.DailyLimit, "monthly_limit": input.MonthlyLimit, "lifetime_limit": input.LifetimeLimit, "slots": input.Slots}).Error
	})
}

func TTSResolveUnknown(actor, jobID, action, note string, units int64, tokenUsage ...ttsprovider.Result) error {
	if len(strings.TrimSpace(note)) < 3 {
		return TTSValidationError("请填写核对依据与供应商 request ID")
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		var j model.TTSJob
		if err := tx.Where("id = ? AND status = ?", jobID, "usage_unknown").First(&j).Error; err != nil {
			return err
		}
		if action == "settle" {
			if (j.Operation == "design" || j.Operation == "clone") && units != 0 && units != 1 {
				return TTSValidationError("单个音色创建任务的确认次数只能为 0 或 1")
			}
			var usage *ttsprovider.Result
			if len(tokenUsage) == 1 {
				usage = &tokenUsage[0]
			}
			if err := ttsSettleUsageJob(tx, j.ID, units, usage, time.Now()); err != nil {
				return err
			}
		} else if action == "release" {
			r := tx.Model(&model.TTSJob{}).Where("id = ? AND usage_status = ?", j.ID, "unknown").Update("usage_status", "released")
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return ErrTTSConflict
			}
			r = tx.Model(&model.AIQuotaReservationModel{}).Where("id = ? AND quota_kind = ? AND status = ?", j.ReservationID, "speech", "active").Update("status", "released")
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return ErrTTSConflict
			}
		} else {
			return ErrTTSConflict
		}
		return tx.Model(&model.TTSJob{}).Where("id = ?", j.ID).Updates(map[string]any{"status": "failed", "audit_actor": actor, "audit_note": note, "error_code": "admin_resolved"}).Error
	})
}

// The platform lock serializes bounded channel queues and provider account slots.
func ttsCapacityLock(tx *gorm.DB) error {
	p := model.TTSUserPolicy{UserID: "__tts_capacity__"}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoNothing: true}).Create(&p).Error; err != nil {
		return err
	}
	return tx.Model(&model.TTSUserPolicy{}).Where("user_id = ?", p.UserID).UpdateColumn("revision", gorm.Expr("revision + 1")).Error
}
