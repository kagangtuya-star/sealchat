package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"sealchat/model"
	"sealchat/pkg/ttsprovider"
	"sealchat/utils"
)

const TTSAutoPreference = "tts.autoSynthesis"

const ttsMessageSynthesisMaxRunes = 20000

type TTSRequest struct {
	RequestKey          string  `json:"requestKey"`
	Text                string  `json:"text"`
	VoiceID             string  `json:"voiceId"`
	SystemVoice         string  `json:"systemVoice"`
	SystemVoiceProvider string  `json:"systemVoiceProvider,omitempty"`
	SystemVoiceModel    string  `json:"systemVoiceModel,omitempty"`
	Instruction         string  `json:"instruction"`
	Rate                float64 `json:"rate"`
	Pitch               float64 `json:"pitch"`
	Volume              *int    `json:"volume"`
	Name                string  `json:"name"`
	Description         string  `json:"description"`
	SourceResourceID    string  `json:"sourceResourceId"`
}
type TTSSnapshot struct {
	Version          int                        `json:"version"`
	Provider         utils.SpeechProviderConfig `json:"provider"`
	Input            ttsprovider.Input          `json:"input"`
	VoiceID          string                     `json:"voiceId"`
	VoiceRevision    int64                      `json:"voiceRevision"`
	Owner            string                     `json:"owner"`
	Scope            string                     `json:"scope"`
	Fingerprint      string                     `json:"fingerprint"`
	SourceResourceID string                     `json:"sourceResourceId,omitempty"`
	Name             string                     `json:"name,omitempty"`
	Description      string                     `json:"description,omitempty"`
	Audience         []string                   `json:"audience,omitempty"`
	Whisper          bool                       `json:"whisper,omitempty"`
}

func ttsHash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func ttsConfig() (*utils.SpeechConfig, error) {
	cfg := utils.GetConfig()
	if cfg == nil || !cfg.AI.Enabled || cfg.AI.Speech == nil || !cfg.AI.Speech.Enabled {
		return nil, ErrTTSDisabled
	}
	return utils.NormalizeSpeechConfig(cfg.AI.Speech), nil
}
func TTSAutomaticEnabled(userID string) bool {
	p, err := model.UserPreferenceGet(userID, TTSAutoPreference)
	return err == nil && p != nil && p.PrefValue == "true"
}

func ttsSnapshot(userID string, r TTSRequest, scope string) (TTSSnapshot, error) {
	return ttsSnapshotForRequest(userID, r, scope, false)
}

func systemVoiceBindingSupported(p utils.SpeechProviderConfig, id, providerKind, modelID string) bool {
	legacy := providerKind == "" && modelID == ""
	return (legacy || (providerKind == p.EffectiveProviderKind() && modelID == p.Model)) &&
		ttsprovider.VoiceSupported(p.EffectiveProviderKind(), p.Model, id)
}

func ttsSnapshotForRequest(userID string, r TTSRequest, scope string, automatic bool) (TTSSnapshot, error) {
	cfg, err := ttsConfig()
	if err != nil {
		return TTSSnapshot{}, err
	}
	s := TTSSnapshot{Version: 1, Owner: userID, Scope: scope, VoiceID: r.VoiceID, SourceResourceID: r.SourceResourceID, Name: r.Name, Description: r.Description}
	if r.VoiceID != "" && r.SystemVoice != "" {
		return s, TTSValidationError("不能同时指定个人音色和系统音色")
	}
	providerID := cfg.DefaultProvider
	var voice *model.TTSVoice
	if r.VoiceID != "" {
		voice, err = ttsAccessibleVoice(model.GetDB(), userID, r.VoiceID)
		if err != nil {
			return s, err
		}
		providerID = voice.ProviderID
		s.VoiceRevision = voice.Revision
	}
	found := false
	for _, p := range cfg.Providers {
		if p.ID == providerID && p.Enabled {
			s.Provider = p
			found = true
			break
		}
	}
	if !found || s.Provider.APIKey == "" {
		return s, ErrTTSDisabled
	}
	if _, supported := ttsprovider.LookupModel(s.Provider.EffectiveProviderKind(), s.Provider.Model); !supported {
		return s, TTSValidationError("语音 provider 类型与模型不匹配")
	}
	s.Provider.APIKey = ""
	s.Provider.HasAPIKey = false
	v := r.SystemVoice
	if voice == nil && v != "" && !systemVoiceBindingSupported(s.Provider, v, r.SystemVoiceProvider, r.SystemVoiceModel) {
		if !automatic {
			return s, TTSValidationError("系统音色与当前 provider 类型或模型不匹配，需要重新选择")
		}
		v = ""
	}
	if v == "" {
		v = ttsprovider.DefaultVoice(s.Provider.EffectiveProviderKind(), s.Provider.Model)
		if s.Provider.ID == cfg.DefaultProvider && ttsprovider.VoiceSupported(s.Provider.EffectiveProviderKind(), s.Provider.Model, cfg.DefaultVoice) {
			v = cfg.DefaultVoice
		}
	}
	if voice != nil {
		if !PersonalVoiceSupported(s.Provider, *voice) {
			return s, ErrTTSDenied
		}
		v = voice.ProviderVoiceID
		if voice.Parameters != "" {
			var defaults TTSRequest
			if json.Unmarshal([]byte(voice.Parameters), &defaults) == nil {
				if r.Instruction == "" {
					r.Instruction = defaults.Instruction
				}
				if r.Rate == 0 {
					r.Rate = defaults.Rate
				}
				if r.Pitch == 0 {
					r.Pitch = defaults.Pitch
				}
				if r.Volume == nil {
					r.Volume = defaults.Volume
				}
			}
		}
	} else if !ttsprovider.VoiceSupported(s.Provider.EffectiveProviderKind(), s.Provider.Model, v) {
		return s, TTSValidationError("系统音色与当前模型不匹配")
	}
	if r.Rate == 0 {
		r.Rate = 1
	}
	if r.Pitch == 0 {
		r.Pitch = 1
	}
	volume := 50
	if r.Volume != nil {
		volume = *r.Volume
	}
	if r.Rate < 0.5 || r.Rate > 2 || r.Pitch < 0.5 || r.Pitch > 2 || volume < 0 || volume > 100 || utf8.RuneCountInString(r.Instruction) > 500 {
		return s, TTSValidationError("语音参数超出范围")
	}
	s.Input = ttsprovider.Input{Text: strings.TrimSpace(r.Text), Voice: v, Format: cfg.Format, SampleRate: 24000, BitRate: 64, Rate: r.Rate, Pitch: r.Pitch, Volume: volume, Instruction: r.Instruction}
	if cfg.Format != "opus" {
		s.Input.BitRate = 0
	}
	b, _ := json.Marshal(s)
	s.Fingerprint = ttsHash(string(b))
	return s, nil
}

// Unknown rich nodes are omitted, notably secrets, clues, images and cards.
func TTSPlainText(content string) (string, error) {
	return ttsPlainText(content, 500)
}

func ttsPlainText(content string, maxRunes int) (string, error) {
	input := strings.TrimSpace(content)
	if strings.HasPrefix(input, "{") {
		var node map[string]any
		if json.Unmarshal([]byte(input), &node) != nil {
			return "", TTSValidationError("无法解析朗读文本")
		}
		var clean func(map[string]any) map[string]any
		clean = func(n map[string]any) map[string]any {
			t, _ := n["type"].(string)
			switch t {
			case "doc", "paragraph", "text", "hardBreak", "heading", "bulletList", "orderedList", "listItem", "blockquote":
			default:
				return nil
			}
			out := map[string]any{"type": t}
			if t == "text" {
				text, ok := n["text"].(string)
				if !ok {
					return nil
				}
				out["text"] = text
			}
			if children, ok := n["content"].([]any); ok {
				list := []any{}
				for _, child := range children {
					if obj, ok := child.(map[string]any); ok {
						if v := clean(obj); v != nil {
							list = append(list, v)
						}
					}
				}
				out["content"] = list
			}
			return out
		}
		n := clean(node)
		if n == nil || n["type"] != "doc" {
			return "", TTSValidationError("此消息类型不朗读")
		}
		// The existing normalizer locates the canonical {"type":"doc" prefix.
		// A map marshal sorts "content" first and would make it read raw JSON.
		b, _ := json.Marshal(struct {
			Type    string `json:"type"`
			Content any    `json:"content"`
		}{Type: "doc", Content: n["content"]})
		input = string(b)
	} else if strings.ContainsAny(input, "<>") || strings.Contains(input, "[CQ:") || strings.Contains(input, "[clue:") {
		return "", TTSValidationError("此消息类型不朗读")
	}
	text := strings.TrimSpace(NormalizeMessageContentToPlainText(input))
	if text == "" || utf8.RuneCountInString(text) > maxRunes {
		if maxRunes == 500 {
			return "", TTSValidationError("朗读文本须为 1–500 字符")
		}
		return "", TTSValidationError("朗读文本须为 1–20000 字符")
	}
	if strings.HasPrefix(text, ".") || strings.HasPrefix(text, "/") || strings.HasPrefix(text, "。") {
		return "", TTSValidationError("命令不自动朗读")
	}
	return text, nil
}

func TTSPrepareMessageIntent(m *model.MessageModel, user *model.UserModel, optIn bool) {
	if !optIn || user == nil || user.IsBot || m.ICMode != "ic" || len(m.ChannelID) >= 30 || m.WidgetData != "" || !TTSAutomaticEnabled(user.ID) {
		return
	}
	text, err := ttsPlainText(m.Content, ttsMessageSynthesisMaxRunes)
	if err != nil {
		m.TTSStatus = "skipped"
		return
	}
	r := TTSRequest{Text: text}
	if m.SenderIdentityID != "" {
		var role model.ChannelIdentityTTSConfig
		if model.GetDB().Where("identity_id = ? AND deleted_at IS NULL", m.SenderIdentityID).First(&role).Error == nil {
			r.VoiceID = role.VoiceID
			r.SystemVoice = role.SystemVoice
			r.SystemVoiceProvider = role.SystemVoiceProvider
			r.SystemVoiceModel = role.SystemVoiceModel
			r.Instruction = role.Instruction
			r.Rate = role.Rate
			r.Pitch = role.Pitch
			r.Volume = &role.Volume
		}
	}
	// Start with an isolated scope, then freeze the actual audience below.
	s, err := ttsSnapshotForRequest(user.ID, r, m.ChannelID+":"+m.ID, true)
	if err != nil {
		m.TTSStatus = "unavailable"
		return
	}
	if m.IsWhisper {
		seen := map[string]bool{}
		for _, target := range m.WhisperTargets {
			if target != nil && target.ID != "" && !seen[target.ID] {
				seen[target.ID] = true
				s.Audience = append(s.Audience, target.ID)
			}
		}
		if m.WhisperTo != "" && !seen[m.WhisperTo] {
			s.Audience = append(s.Audience, m.WhisperTo)
		}
		sort.Strings(s.Audience)
	}
	s.Whisper = m.IsWhisper
	audience, _ := json.Marshal(s.Audience)
	s.Scope = m.ChannelID + ":" + ttsHash(string(audience))
	s.Fingerprint = ""
	fingerprint, _ := json.Marshal(s)
	s.Fingerprint = ttsHash(string(fingerprint))
	b, err := json.Marshal(s)
	if err != nil {
		return
	}
	m.TTSIntent = string(b)
	m.TTSStatus = "pending"
}

func TTSSubmit(userID, operation string, r TTSRequest) (*model.TTSJob, error) {
	if len(r.Name) > 200 || len(r.Description) > 2000 {
		return nil, TTSValidationError("音色名称或描述过长")
	}
	if len(r.RequestKey) < 8 || len(r.RequestKey) > 100 {
		return nil, TTSValidationError("缺少稳定的 requestKey")
	}
	key := ttsHash(userID + ":" + r.RequestKey)
	inputRaw, _ := json.Marshal(r)
	inputHash := ttsHash(operation + string(inputRaw))
	var existing model.TTSJob
	if err := model.GetDB().Where("request_key = ? AND payer_user_id = ?", key, userID).First(&existing).Error; err == nil {
		if existing.InputHash != inputHash {
			return nil, ErrTTSConflict
		}
		return &existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if operation != "design" && operation != "clone" && operation != "audition" {
		return nil, ErrTTSConflict
	}
	if operation == "audition" {
		text, err := TTSPlainText(r.Text)
		if err != nil {
			return nil, err
		}
		r.Text = text
	}
	if operation == "design" && (utf8.RuneCountInString(r.Description) < 1 || utf8.RuneCountInString(r.Description) > 500 || utf8.RuneCountInString(r.Text) < 15 || utf8.RuneCountInString(r.Text) > 200) {
		return nil, TTSValidationError("声音描述限 1–500 字符，预览文本限 15–200 字符")
	}
	s, err := ttsSnapshot(userID, r, "user:"+userID)
	if err != nil {
		return nil, err
	}
	spec, _ := ttsprovider.LookupModel(s.Provider.EffectiveProviderKind(), s.Provider.Model)
	if (operation == "design" && !spec.Capabilities.VoiceDesign) || (operation == "clone" && !spec.Capabilities.VoiceClone) {
		return nil, TTSValidationError("当前模型不支持此音色创建方式")
	}
	if operation == "clone" {
		var a model.AttachmentModel
		if model.GetDB().Where("id = ? AND user_id = ? AND root_id_type = ? AND parent_id_type = ? AND deleted_at IS NULL", r.SourceResourceID, userID, "tts", "clone_source").First(&a).Error != nil {
			return nil, ErrTTSDenied
		}
	}
	b, _ := json.Marshal(s)
	job := &model.TTSJob{Operation: operation, RequestKey: key, PayerUserID: userID, Snapshot: string(b), InputHash: inputHash, Deadline: time.Now().Add(2 * time.Minute), VoiceID: r.VoiceID}
	// Auditions are cached only inside this authenticated user's safety domain.
	// Resource ownership is checked again on every playback authorization.
	if operation == "audition" {
		var cached model.TTSCache
		if model.GetDB().Where("fingerprint = ? AND complete = ? AND expires_at > ? AND deleted_at IS NULL", s.Fingerprint, true, time.Now()).First(&cached).Error == nil {
			var resource model.AttachmentModel
			if model.GetDB().Where("id = ? AND user_id = ? AND root_id_type = ? AND deleted_at IS NULL", cached.ResourceID, userID, "tts").First(&resource).Error == nil {
				if path, pathErr := TTSResourcePath(&resource); pathErr == nil {
					if _, statErr := os.Stat(path); statErr == nil {
						job.Status, job.UsageStatus = "succeeded", "cached"
						job.ResourceID, job.MediaJSON = cached.ResourceID, cached.Metadata
						if err := model.GetDB().Create(job).Error; err != nil {
							var duplicate model.TTSJob
							if model.GetDB().Where("request_key = ? AND payer_user_id = ? AND input_hash = ?", key, userID, inputHash).First(&duplicate).Error == nil {
								return &duplicate, nil
							}
							return nil, err
						}
						return job, nil
					}
				}
			}
		}
	}
	if err = ttsReserveSnapshot(job, s); err != nil {
		return nil, err
	}
	TTSWake()
	return job, nil
}
func ttsReserveSnapshot(job *model.TTSJob, s TTSSnapshot) error {
	cfg, err := ttsConfig()
	if err != nil {
		return err
	}
	if job.Operation == "message_synthesis" {
		var cached model.TTSCache
		if model.GetDB().Where("fingerprint = ? AND complete = ? AND expires_at > ? AND deleted_at IS NULL", s.Fingerprint, true, time.Now()).First(&cached).Error == nil {
			var resource model.AttachmentModel
			if model.GetDB().Where("id = ? AND user_id = ? AND channel_id = ? AND root_id_type = ? AND deleted_at IS NULL", cached.ResourceID, job.PayerUserID, job.ChannelID, "tts").First(&resource).Error == nil {
				if path, err := TTSResourcePath(&resource); err == nil {
					if _, err := os.Stat(path); err == nil {
						job.UsageStatus, job.ResourceID, job.MediaJSON = "cached", cached.ResourceID, cached.Metadata
						return ttsReserveJob(model.GetDB(), cfg, job, s.Provider, 0, time.Now())
					}
				}
			}
		}
	}
	price := s.Provider.CharacterPrice
	if s.Provider.EffectivePricingMode() == ttsprovider.PricingToken && s.Provider.SynthesisPriceConfirmed() {
		estimatedPrice := *s.Provider.InputTokenPrice + *s.Provider.OutputTokenPrice
		price = &estimatedPrice
	}
	job.EstimatedUnits = int64(utf8.RuneCountInString(s.Input.Text))
	if spec, supported := ttsprovider.LookupModel(s.Provider.EffectiveProviderKind(), s.Provider.Model); supported && spec.Pricing.Mode == ttsprovider.PricingCharacter {
		job.EstimatedUnits = TTSBillableCharacters(s.Input.Text)
	}
	if job.Operation == "design" {
		price = s.Provider.DesignPrice
		job.EstimatedUnits = 1
	}
	if job.Operation == "clone" {
		price = s.Provider.ClonePrice
		job.EstimatedUnits = 1
	}
	if job.Operation != "design" && job.Operation != "clone" && !s.Provider.SynthesisPriceConfirmed() {
		return TTSValidationError("管理员尚未确认合成单价")
	}
	if price == nil {
		return TTSValidationError("管理员尚未确认此操作参数")
	}
	return ttsReserveJob(model.GetDB(), cfg, job, s.Provider, *price, time.Now())
}
