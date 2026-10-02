package service

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
	"sealchat/model"
	"sealchat/utils"
)

const ttsSystemPreviewVersion = 1
const ttsSystemPreviewScope = "system-preview:v1"
const ttsSystemPreviewRetryCooldown = time.Minute

type TTSSystemPreviewRequest struct {
	SystemVoice  string `json:"systemVoice"`
	ProviderKind string `json:"providerKind"`
	ProviderID   string `json:"providerId"`
	ModelID      string `json:"modelId"`
}

func ttsSystemPreviewKey(p utils.SpeechProviderConfig, voice string) string {
	identity, _ := json.Marshal(struct {
		Version         int    `json:"previewVersion"`
		Kind            string `json:"providerKind"`
		ProviderID      string `json:"providerId"`
		CredentialScope string `json:"credentialScope"`
		Region          string `json:"region"`
		Workspace       string `json:"workspace"`
		Model           string `json:"model"`
		Voice           string `json:"voice"`
	}{ttsSystemPreviewVersion, p.EffectiveProviderKind(), p.ID, p.CredentialScope, p.Region, p.Workspace, p.Model, voice})
	return ttsHash(ttsSystemPreviewScope + ":" + string(identity))
}

func ttsSystemPreviewText(languages []string) string {
	if slices.Contains(languages, "zh") {
		return "你好，很高兴认识你。这是我的语音试听。"
	}
	if slices.Contains(languages, "en") {
		return "Hello, nice to meet you. This is my voice preview."
	}
	// Keep the fallback local and small; previews never invoke translation AI.
	for _, language := range languages {
		switch language {
		case "yue":
			return "你好，很高兴认识你。这是我的语音试听。"
		case "ja":
			return "こんにちは。お会いできてうれしいです。これは私の声のサンプルです。"
		case "ko":
			return "안녕하세요. 만나서 반갑습니다. 제 목소리 미리 듣기입니다."
		}
	}
	return "Hello, nice to meet you. This is my voice preview."
}

func ttsSystemPreviewRequestKey(previewKey, previousID string) string {
	if previousID == "" {
		return ttsHash("system-preview:first:" + previewKey)
	}
	return ttsHash("system-preview:retry:" + previewKey + ":" + previousID)
}

func ttsSystemPreviewResourceExists(job *model.TTSJob) (bool, error) {
	if job.ResourceID == "" {
		return false, nil
	}
	var attachment model.AttachmentModel
	err := model.GetDB().Where("id = ? AND root_id_type = ? AND parent_id_type = ? AND root_id = ? AND deleted_at IS NULL", job.ResourceID, "tts", "job_audio", job.ID).First(&attachment).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return TTSResourceExists(context.Background(), &attachment)
}

// GET only validates the existing asset; rebuilding belongs to ensure/POST.
func TTSReadSystemPreviewJob(id string) (*model.TTSJob, error) {
	var job model.TTSJob
	if err := model.GetDB().Where("id = ? AND operation = ? AND deleted_at IS NULL", id, "system_preview").First(&job).Error; err != nil {
		return nil, err
	}
	if job.Status == "succeeded" {
		exists, err := ttsSystemPreviewResourceExists(&job)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, gorm.ErrRecordNotFound
		}
	}
	return &job, nil
}

// TTSEnsureSystemPreview reserves a platform asset, never a user's audition.
// Stable request keys and the existing database reservation lock deduplicate
// concurrent callers across processes, including retries of known terminal jobs.
func TTSEnsureSystemPreview(r TTSSystemPreviewRequest) (*model.TTSJob, error) {
	r.SystemVoice, r.ProviderKind = strings.TrimSpace(r.SystemVoice), strings.TrimSpace(r.ProviderKind)
	r.ProviderID, r.ModelID = strings.TrimSpace(r.ProviderID), strings.TrimSpace(r.ModelID)
	if r.SystemVoice == "" || r.ProviderKind == "" || r.ProviderID == "" || r.ModelID == "" {
		return nil, TTSValidationError("请完整指定系统音色、provider 和模型")
	}
	cfg, err := ttsConfig()
	if err != nil {
		return nil, err
	}
	p, err := resolveSystemVoiceProvider(cfg, r.SystemVoice, r.ProviderKind, r.ProviderID, r.ModelID)
	if err != nil {
		return nil, err
	}
	previewKey := ttsSystemPreviewKey(p, r.SystemVoice)
	var previous model.TTSJob
	err = model.GetDB().Where("operation = ? AND input_hash = ? AND deleted_at IS NULL", "system_preview", previewKey).Order("created_at DESC, id DESC").First(&previous).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err == nil {
		switch previous.Status {
		case "queued", "running", "storage_pending", "archiving", "usage_unknown":
			return &previous, nil
		case "succeeded":
			exists, resourceErr := ttsSystemPreviewResourceExists(&previous)
			if resourceErr != nil {
				return nil, resourceErr
			}
			if exists {
				return &previous, nil
			}
		case "failed", "cancelled", "skipped", "unavailable":
			if time.Since(previous.UpdatedAt) < ttsSystemPreviewRetryCooldown {
				return &previous, nil
			}
		default:
			return nil, ErrTTSConflict
		}
	}
	payer := "__tts_system_preview__:" + p.ID
	if len(payer) > 100 {
		return nil, ErrTTSConflict
	}
	s, err := ttsSnapshotForOperation(payer, TTSRequest{
		Text:        ttsSystemPreviewText(ttsSystemVoiceLanguages(p, p.Model, r.SystemVoice)),
		SystemVoice: r.SystemVoice, SystemVoiceProvider: r.ProviderKind,
		SystemVoiceProviderID: r.ProviderID, SystemVoiceModel: r.ModelID,
		Rate: 1, Pitch: 1,
	}, ttsSystemPreviewScope, false, "system_preview")
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	job := &model.TTSJob{Operation: "system_preview", InputHash: previewKey,
		RequestKey:  ttsSystemPreviewRequestKey(previewKey, previous.ID),
		PayerUserID: payer, Snapshot: string(raw), Deadline: time.Now().Add(ttsMessageQueueWait)}
	if err := ttsReserveSnapshot(job, s); err != nil {
		return nil, err
	}
	TTSWake()
	return job, nil
}
