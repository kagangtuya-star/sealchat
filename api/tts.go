package api

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"sealchat/model"
	"sealchat/pkg/ttsprovider"
	"sealchat/pm"
	"sealchat/service"
	"sealchat/utils"
)

func ttsError(c *fiber.Ctx, err error) error {
	status := fiber.StatusInternalServerError
	message := "语音操作失败，请查询任务状态；不要重复提交请求"
	var validation service.TTSValidationError
	if errors.As(err, &validation) {
		status, message = 400, validation.Error()
	}
	var translation *service.TTSTranslationError
	if errors.As(err, &translation) {
		status, message = fiber.StatusServiceUnavailable, translation.Error()
	}
	if errors.Is(err, ttsprovider.ErrMedia) {
		status, message = 400, ttsprovider.ErrMedia.Error()
	}
	if errors.Is(err, service.ErrTTSDisabled) {
		status, message = 403, service.ErrTTSDisabled.Error()
	}
	if errors.Is(err, service.ErrTTSWorldDenied) {
		status, message = 403, service.ErrTTSWorldDenied.Error()
	}
	if errors.Is(err, service.ErrTTSDenied) || errors.Is(err, service.ErrChannelPermissionDenied) {
		status = 403
		message = "无权访问此语音资源"
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		status = 404
		message = "语音资源不存在或已不可用"
	}
	if errors.Is(err, service.ErrTTSConflict) {
		status = 409
		message = service.ErrTTSConflict.Error()
	}
	return c.Status(status).JSON(fiber.Map{"message": message})
}
func BindTTSPublicRoutes(v1 fiber.Router) {
	v1.Get("/tts/play/:ticket", ttsPlay)
	v1.Get("/tts/clone-source/:jobId", ttsCloneSource)
	v1.Get("/tts/ws", ttsWSUpgrade, ttsWSHandler())
}
func BindTTSRoutes(auth fiber.Router) {
	r := auth.Group("/tts")
	r.Get("/me", func(c *fiber.Ctx) error {
		q, err := service.TTSQuotaForChannel(getCurUser(c).ID, c.Query("channelId"))
		if err != nil {
			return ttsError(c, err)
		}
		return c.JSON(q)
	})
	r.Post("/worlds/:worldId/activate", func(c *fiber.Ctx) error {
		var body struct {
			Code string `json:"code"`
		}
		decoder := json.NewDecoder(strings.NewReader(string(c.Body())))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		if err := service.ActivateTTSWorld(getCurUser(c).ID, c.Params("worldId"), body.Code); err != nil {
			return ttsError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})
	r.Patch("/settings", func(c *fiber.Ctx) error {
		var b struct {
			AutoSynthesis bool `json:"autoSynthesis"`
		}
		if err := c.BodyParser(&b); err != nil {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		value := "false"
		if b.AutoSynthesis {
			value = "true"
		}
		_, err := model.UserPreferenceUpsert(getCurUser(c).ID, service.TTSAutoPreference, value)
		if err != nil {
			return ttsError(c, err)
		}
		return c.JSON(b)
	})
	r.Get("/voices", ttsVoices)
	r.Get("/voice-targets", func(c *fiber.Ctx) error {
		providers, err := service.TTSVoiceCreationProviders()
		if err != nil {
			return ttsError(c, err)
		}
		return c.JSON(providers)
	})
	r.Get("/voices/:id", func(c *fiber.Ctx) error {
		var voice model.TTSVoice
		if err := model.GetDB().Where("id = ? AND lifecycle = ? AND provider_status = ? AND deleted_at IS NULL AND (owner_user_id = ? OR is_public = ?)", c.Params("id"), "saved", "OK", getCurUser(c).ID, true).First(&voice).Error; err != nil {
			return ttsError(c, err)
		}
		return c.JSON(service.TTSPersonalVoiceResponse(voice))
	})
	r.Post("/sources", ttsSourceUpload)
	r.Post("/system-previews", ttsEnsureSystemPreview)
	r.Get("/system-previews/:id", ttsSystemPreview)
	r.Patch("/voices/:id", func(c *fiber.Ctx) error {
		var b model.TTSVoice
		if err := c.BodyParser(&b); err != nil {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		if err := service.TTSUpdateVoice(getCurUser(c).ID, c.Params("id"), b); err != nil {
			return ttsError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})
	r.Delete("/voices/:id", func(c *fiber.Ctx) error {
		if err := service.TTSDeleteVoice(getCurUser(c).ID, c.Params("id")); err != nil {
			return ttsError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})
	r.Post("/voices/:id/save", func(c *fiber.Ctx) error {
		var b struct {
			ReplaceID string `json:"replaceId"`
		}
		if len(c.Body()) > 0 {
			if err := c.BodyParser(&b); err != nil {
				return c.SendStatus(fiber.StatusBadRequest)
			}
		}
		q, err := service.TTSQuotaForUser(getCurUser(c).ID)
		if err != nil {
			return ttsError(c, err)
		}
		if err = service.TTSSaveVoice(model.GetDB(), getCurUser(c).ID, c.Params("id"), b.ReplaceID, q.Slots, time.Now()); err != nil {
			return ttsError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})
	r.Post("/jobs/:operation", func(c *fiber.Ctx) error {
		var b service.TTSRequest
		if err := c.BodyParser(&b); err != nil {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		job, err := service.TTSSubmit(getCurUser(c).ID, c.Params("operation"), b)
		if err != nil {
			return ttsError(c, err)
		}
		return c.Status(202).JSON(ttsJobResponse(job))
	})
	r.Get("/jobs/:id", func(c *fiber.Ctx) error {
		var j model.TTSJob
		if err := model.GetDB().Where("id = ? AND payer_user_id = ? AND deleted_at IS NULL", c.Params("id"), getCurUser(c).ID).First(&j).Error; err != nil {
			return ttsError(c, err)
		}
		return c.JSON(ttsJobResponse(&j))
	})
	r.Get("/roles/:id", func(c *fiber.Ctx) error {
		v, err := service.TTSRoleConfig(getCurUser(c).ID, c.Params("id"))
		if err != nil {
			return ttsError(c, err)
		}
		return c.JSON(v)
	})
	r.Put("/roles/:id", func(c *fiber.Ctx) error {
		var b model.ChannelIdentityTTSConfig
		if err := c.BodyParser(&b); err != nil {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		if err := service.TTSSaveRoleConfig(getCurUser(c).ID, c.Params("id"), b); err != nil {
			return ttsError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})
	r.Get("/messages/:id", func(c *fiber.Ctx) error {
		m, err := ttsReadMessage(getCurUser(c).ID, c.Params("id"))
		if err != nil {
			return ttsError(c, err)
		}
		return c.JSON(fiber.Map{"tts": m.ValidTTS()})
	})
	r.Post("/messages/:id/ticket", func(c *fiber.Ctx) error {
		m, err := ttsReadMessage(getCurUser(c).ID, c.Params("id"))
		if err != nil {
			return ttsError(c, err)
		}
		t := m.ValidTTS()
		if t == nil || t.Status != "ready" || t.AudioResourceID == "" {
			return c.SendStatus(404)
		}
		return ttsTicketResponse(c, service.TTSTicket{UserID: getCurUser(c).ID, MessageID: m.ID, ResourceID: t.AudioResourceID, ChannelID: m.ChannelID, Purpose: "message", Expires: time.Now().Add(2 * time.Minute)})
	})
	r.Post("/resources/:id/ticket", func(c *fiber.Ctx) error {
		a, err := ttsReadResource(getCurUser(c).ID, c.Params("id"))
		if err != nil {
			return ttsError(c, err)
		}
		if a.ParentIDType == "clone_source" {
			return c.SendStatus(fiber.StatusForbidden)
		}
		return ttsTicketResponse(c, service.TTSTicket{UserID: getCurUser(c).ID, ResourceID: a.ID, Purpose: "preview", Expires: time.Now().Add(2 * time.Minute)})
	})
	r.Post("/channels/:id/ws-ticket", ttsWSTicket)
	r.Post("/channels/:id/control", ttsChannelControl)
	r.Get("/channels/:id/queue", ttsChannelQueue)
	r.Get("/channels/:id/states", func(c *fiber.Ctx) error {
		userID, channelID := getCurUser(c).ID, c.Params("id")
		if !pm.CanWithChannelRole(userID, channelID, pm.PermFuncChannelRead, pm.PermFuncChannelReadAll) {
			return c.SendStatus(403)
		}
		var messages []model.MessageModel
		if err := model.GetDB().Where("channel_id = ? AND tts_status <> ? AND created_at >= ?", channelID, "", time.Now().Add(-5*time.Minute)).Order("created_at DESC, id DESC").Limit(100).Find(&messages).Error; err != nil {
			return ttsError(c, err)
		}
		items := []fiber.Map{}
		for _, message := range messages {
			if canUserAccessWhisperMessage(userID, channelID, &message) {
				items = append(items, fiber.Map{"id": message.ID, "tts": message.ValidTTS()})
			}
		}
		c.Set("Cache-Control", "private, no-store")
		return c.JSON(items)
	})
	service.TTSSetCallbacks(ttsBroadcastReady, ttsBroadcastCancel, ttsBroadcastLive)
}

func BindTTSAdminRoutes(authAdmin fiber.Router) {
	admin := authAdmin.Group("/tts/admin")
	admin.Get("/worlds", func(c *fiber.Ctx) error {
		result, err := service.AdminListTTSWorlds(c.QueryInt("page", 1), c.QueryInt("pageSize", 20), c.Query("search"), "")
		if err != nil {
			return ttsError(c, err)
		}
		return c.JSON(result)
	})
	admin.Get("/worlds/:worldId", ttsAdminWorldDetail)
	admin.Patch("/worlds/:worldId", func(c *fiber.Ctx) error {
		var patch service.TTSWorldPatch
		decoder := json.NewDecoder(strings.NewReader(string(c.Body())))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&patch); err != nil {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(c.Body(), &fields); err != nil || fields == nil {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		present := map[string]bool{}
		for key := range fields {
			present[key] = true
		}
		if present["allowlisted"] && patch.Allowlisted == nil || present["quotaOverrideEnabled"] && patch.QuotaOverrideEnabled == nil {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		if err := service.TTSPatchWorldPolicy(c.Params("worldId"), patch, present); err != nil {
			return ttsError(c, err)
		}
		return ttsAdminWorldDetail(c)
	})
	admin.Get("/config", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"config": sanitizeConfigForAdmin(appConfig).AI.Speech})
	})
	admin.Patch("/config", ttsAdminConfig)
	admin.Post("/provider/resolve", ttsAdminProviderResolve)
	admin.Get("/models", func(c *fiber.Ctx) error {
		return c.JSON(service.TTSModelCatalog())
	})
	admin.Get("/users/:id", func(c *fiber.Ctx) error {
		q, err := service.TTSQuotaForUser(c.Params("id"))
		if err != nil {
			return ttsError(c, err)
		}
		var p model.TTSUserPolicy
		_ = model.GetDB().Where("user_id = ?", c.Params("id")).First(&p).Error
		return c.JSON(fiber.Map{"quota": q, "policy": p})
	})
	admin.Put("/users/:id", func(c *fiber.Ctx) error {
		var p model.TTSUserPolicy
		if err := c.BodyParser(&p); err != nil {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		if model.UserGet(c.Params("id")) == nil {
			return c.SendStatus(404)
		}
		if err := service.TTSSetPolicy(c.Params("id"), p); err != nil {
			return ttsError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})
	admin.Get("/unknown", func(c *fiber.Ctx) error {
		var jobs []model.TTSJob
		err := model.GetDB().Where("status = ?", "usage_unknown").Order("created_at DESC, id DESC").Limit(100).Find(&jobs).Error
		if err != nil {
			return ttsError(c, err)
		}
		items := []fiber.Map{}
		for _, j := range jobs {
			items = append(items, fiber.Map{"id": j.ID, "operation": j.Operation, "payerUserId": j.PayerUserID, "providerRequestId": j.ProviderRequestID, "estimatedUnits": j.EstimatedUnits, "pricingMode": service.TTSJobPricingMode(&j), "errorCode": j.ErrorCode, "createdAt": j.CreatedAt})
		}
		return c.JSON(items)
	})
	admin.Post("/unknown/:id", func(c *fiber.Ctx) error {
		var b struct {
			Action       string `json:"action"`
			Note         string `json:"note"`
			Units        int64  `json:"units"`
			InputTokens  *int64 `json:"inputTokens"`
			OutputTokens *int64 `json:"outputTokens"`
		}
		if err := c.BodyParser(&b); err != nil {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		usage := ttsprovider.Result{InputTokens: b.InputTokens, OutputTokens: b.OutputTokens, TokenUsageConfirmed: b.InputTokens != nil && b.OutputTokens != nil}
		if err := service.TTSResolveUnknown(getCurUser(c).ID, c.Params("id"), b.Action, b.Note, b.Units, usage); err != nil {
			return ttsError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})
}

func ttsAdminWorldDetail(c *fiber.Ctx) error {
	result, err := service.AdminListTTSWorlds(1, 1, "", c.Params("worldId"))
	if err != nil {
		return ttsError(c, err)
	}
	if len(result.Items) == 0 {
		return c.SendStatus(fiber.StatusNotFound)
	}
	return c.JSON(result.Items[0])
}

func ttsJobResponse(job *model.TTSJob) any {
	var media *ttsprovider.Media
	if job.MediaJSON != "" {
		var value ttsprovider.Media
		if json.Unmarshal([]byte(job.MediaJSON), &value) == nil {
			media = &value
		}
	}
	actualCost := job.ActualCost
	if actualCost == nil && job.ActualUnits != nil {
		value := math.Round(float64(*job.ActualUnits)*job.UnitPrice*1e6) / 1e6
		actualCost = &value
	}
	modelID := ""
	if job.Snapshot != "" {
		var snapshot service.TTSSnapshot
		if json.Unmarshal([]byte(job.Snapshot), &snapshot) == nil {
			modelID = snapshot.Provider.Model
		}
	}
	publicJob := *job
	// Provider codes are internal diagnostics; preserve the safe client contract.
	if strings.HasPrefix(publicJob.ErrorCode, "provider_") {
		switch publicJob.ErrorCode {
		case "provider_usage_unknown", "provider_or_spool_failed", "provider_audio_download_failed", "provider_audio_invalid_media":
		default:
			publicJob.ErrorCode = "provider_or_spool_failed"
			if publicJob.Status == "usage_unknown" {
				publicJob.ErrorCode = "provider_usage_unknown"
			}
		}
	}
	return struct {
		*model.TTSJob
		Media       *ttsprovider.Media `json:"media,omitempty"`
		ActualCost  *float64           `json:"actualCost,omitempty"`
		Message     string             `json:"message,omitempty"`
		Model       string             `json:"model,omitempty"`
		PricingMode string             `json:"pricingMode"`
	}{&publicJob, media, actualCost, ttsJobMessage(&publicJob), modelID, service.TTSJobPricingMode(job)}
}

func ttsSystemPreviewResponse(job *model.TTSJob) any {
	// Platform diagnostics must not leak through the message's fallback either.
	messageJob := *job
	messageJob.ErrorCode = ""
	return struct {
		ID              string `json:"id"`
		Status          string `json:"status"`
		AudioResourceID string `json:"audioResourceId,omitempty"`
		Message         string `json:"message,omitempty"`
	}{job.ID, job.Status, job.ResourceID, ttsJobMessage(&messageJob)}
}

func ttsEnsureSystemPreview(c *fiber.Ctx) error {
	var body service.TTSSystemPreviewRequest
	decoder := json.NewDecoder(strings.NewReader(string(c.Body())))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	job, err := service.TTSEnsureSystemPreview(body)
	if err != nil {
		return ttsError(c, err)
	}
	return c.Status(fiber.StatusAccepted).JSON(ttsSystemPreviewResponse(job))
}

func ttsSystemPreview(c *fiber.Ctx) error {
	job, err := service.TTSReadSystemPreviewJob(c.Params("id"))
	if err != nil {
		return ttsError(c, err)
	}
	return c.JSON(ttsSystemPreviewResponse(job))
}

func ttsJobMessage(job *model.TTSJob) string {
	if job == nil {
		return ""
	}
	if job.Status == "succeeded" {
		return "合成完成"
	}
	if job.Status == "usage_unknown" {
		return "供应商用量无法确认，已暂停结算，请联系管理员处理"
	}
	if job.Status == "cancelled" {
		return "合成任务已取消"
	}
	if job.Status != "failed" {
		return ""
	}
	switch job.ErrorCode {
	case "cache_unavailable":
		return "缓存语音文件不可用"
	case "provider_or_spool_failed":
		return "供应商合成或语音文件处理失败"
	case "invalid_spool":
		return "语音文件校验失败"
	case "spool_unavailable":
		return "语音临时文件不可用"
	case "unsupported_or_incomplete_media":
		return "供应商返回了不完整或不支持的音频"
	case "provider_audio_download_failed":
		return "供应商完整音频下载失败"
	case "provider_audio_invalid_media":
		return "供应商完整音频校验失败"
	case "admin_resolved":
		return "管理员已终止该异常任务"
	default:
		if job.ErrorCode != "" {
			return "合成失败（" + job.ErrorCode + "）"
		}
		return "合成失败"
	}
}

func ttsVoices(c *fiber.Ctx) error {
	userID := getCurUser(c).ID
	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	size := c.QueryInt("pageSize", 20)
	if size < 1 || size > 100 {
		size = 20
	}
	q := model.GetDB().Model(&model.TTSVoice{}).Where("deleted_at IS NULL")
	// `scope` selects the picker catalog: saved voices only, where the caller's
	// own voices stay listed even when the provider reports them unhealthy.
	// Without it the legacy `mine` semantics (workbench lifecycles) apply.
	switch scope := c.Query("scope"); scope {
	case "":
		if c.Query("mine") == "true" {
			q = q.Where("owner_user_id = ? AND lifecycle <> ?", userID, "deleted")
		} else {
			q = q.Where("(owner_user_id = ? OR (is_public = ? AND lifecycle = ? AND provider_status = ?))", userID, true, "saved", "OK")
		}
	case "mine":
		q = q.Where("owner_user_id = ? AND lifecycle = ?", userID, "saved")
	case "public":
		q = q.Where("lifecycle = ? AND is_public = ? AND (owner_user_id = ? OR provider_status = ?)", "saved", true, userID, "OK")
	case "all":
		q = q.Where("lifecycle = ? AND (owner_user_id = ? OR (is_public = ? AND provider_status = ?))", "saved", userID, true, "OK")
	default:
		return c.SendStatus(fiber.StatusBadRequest)
	}
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		q = q.Where("name LIKE ? OR tags LIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if target := c.Query("model"); target != "" {
		q = q.Where("target_model = ?", target)
	}
	if providerID := strings.TrimSpace(c.Query("providerId")); providerID != "" {
		q = q.Where("provider_id = ?", providerID)
	}
	if kind := strings.TrimSpace(c.Query("kind")); kind != "" {
		q = q.Where("kind = ?", kind)
	}
	if tag := strings.TrimSpace(c.Query("tag")); tag != "" {
		q = q.Where("tags LIKE ?", "%"+tag+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return ttsError(c, err)
	}
	items := []model.TTSVoice{}
	if err := q.Order("created_at DESC, id DESC").Offset((page - 1) * size).Limit(size).Find(&items).Error; err != nil {
		return ttsError(c, err)
	}
	voices := make([]service.TTSPersonalVoice, 0, len(items))
	for _, voice := range items {
		voices = append(voices, service.TTSPersonalVoiceResponse(voice))
	}
	return c.JSON(fiber.Map{"items": voices, "total": total, "system": service.TTSSystemVoices(c.UserContext()), "providers": ttsprovider.ProviderCatalog(), "presetSources": ttsprovider.PresetSourceCatalog(), "catalogVersion": "2026-09-30"})
}
func ttsReadMessage(userID, id string) (*model.MessageModel, error) {
	var m model.MessageModel
	if err := model.GetDB().Where("id = ? AND is_deleted = ? AND (is_revoked = ? OR is_revoked IS NULL) AND deleted_at IS NULL", id, false, false).First(&m).Error; err != nil {
		return nil, err
	}
	if !pm.CanWithChannelRole(userID, m.ChannelID, pm.PermFuncChannelRead, pm.PermFuncChannelReadAll) || !canUserAccessWhisperMessage(userID, m.ChannelID, &m) {
		return nil, service.ErrTTSDenied
	}
	if data := m.ValidTTS(); data != nil && data.Status == "ready" && !service.TTSMessageAudioCurrent(&m) {
		return nil, service.ErrTTSDenied
	}
	return &m, nil
}
func ttsReadResource(userID, id string) (*model.AttachmentModel, error) {
	var a model.AttachmentModel
	if err := model.GetDB().Where("id = ? AND root_id_type = ? AND deleted_at IS NULL", id, "tts").First(&a).Error; err != nil {
		return nil, err
	}
	if a.ParentIDType == "clone_source" {
		if a.UserID != userID {
			return nil, service.ErrTTSDenied
		}
		return &a, nil
	}
	var j model.TTSJob
	if a.ParentIDType == "job_audio" {
		if err := model.GetDB().Where("id = ? AND deleted_at IS NULL", a.RootID).First(&j).Error; err != nil {
			return nil, service.ErrTTSDenied
		}
		if j.MessageID != "" {
			return nil, service.ErrTTSDenied
		}
		if userID != "" && j.Operation == "system_preview" && j.Status == "succeeded" && j.ResourceID == a.ID {
			return &a, nil
		}
	} else {
		return nil, service.ErrTTSDenied
	}
	if a.UserID == userID {
		return &a, nil
	}
	var count int64
	err := model.GetDB().Model(&model.TTSVoice{}).Where("preview_resource_id = ? AND is_public = ? AND lifecycle = ? AND provider_status = ? AND deleted_at IS NULL", a.ID, true, "saved", "OK").Count(&count).Error
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, service.ErrTTSDenied
	}
	return &a, nil
}
func ttsTicketResponse(c *fiber.Ctx, t service.TTSTicket) error {
	key, err := service.TTSIssueTicket(t)
	if err != nil {
		return ttsError(c, err)
	}
	return c.JSON(fiber.Map{"url": joinWebPath(appConfig.WebUrl, "api/v1/tts/play/"+key), "expiresAt": t.Expires})
}
func ttsPlay(c *fiber.Ctx) error {
	t, ok := service.TTSReadTicket(c.Params("ticket"), false)
	if !ok {
		return c.SendStatus(403)
	}
	var a *model.AttachmentModel
	var err error
	if t.Purpose == "message" {
		m, e := ttsReadMessage(t.UserID, t.MessageID)
		if e != nil || m.ChannelID != t.ChannelID {
			return c.SendStatus(403)
		}
		data := m.ValidTTS()
		if data == nil || data.AudioResourceID != t.ResourceID {
			return c.SendStatus(404)
		}
		a = &model.AttachmentModel{}
		err = model.GetDB().Where("id = ?", t.ResourceID).First(a).Error
	} else if t.Purpose == "preview" {
		a, err = ttsReadResource(t.UserID, t.ResourceID)
	} else {
		return c.SendStatus(403)
	}
	if err != nil {
		return c.SendStatus(404)
	}
	return ttsSendAttachment(c, a)
}

func ttsCloneSource(c *fiber.Ctx) error {
	a, err := service.TTSResolveCloneSource(c.Params("jobId"), c.Query("expires"), c.Query("token"))
	if err != nil {
		return c.SendStatus(403)
	}
	return ttsSendAttachment(c, a)
}

func ttsSendAttachment(c *fiber.Ctx, a *model.AttachmentModel) error {
	if a != nil && a.StorageType == model.StorageS3 {
		reader, err := service.TTSOpenResource(c.UserContext(), a)
		if err != nil {
			return c.SendStatus(404)
		}
		c.Set("Cache-Control", "private, no-store")
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("Content-Type", a.MimeType)
		// Fiber writes after the handler returns. Close in the stream writer,
		// not here; Range is intentionally ignored for remote resources.
		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			defer reader.Close()
			_, _ = io.Copy(w, reader)
		})
		return nil
	}
	path, err := service.TTSResourcePath(a)
	if err != nil {
		return c.SendStatus(404)
	}
	c.Set("Cache-Control", "private, no-store")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Content-Type", a.MimeType)
	c.Set("Accept-Ranges", "bytes")
	if c.Get("Range") != "" {
		f, e := os.Open(path)
		if e != nil {
			return c.SendStatus(404)
		}
		info, e := f.Stat()
		if e != nil {
			f.Close()
			return c.SendStatus(404)
		}
		return streamFileWithRange(c, f, info.Size(), a.MimeType)
	}
	return c.SendFile(path)
}
func ttsSourceUpload(c *fiber.Ctx) error {
	if c.FormValue("authorized") != "true" {
		return c.Status(400).JSON(fiber.Map{"message": "请确认你有权使用该复刻样本"})
	}
	h, err := c.FormFile("file")
	if err != nil {
		return ttsError(c, err)
	}
	if h.Size > 10<<20 {
		return c.SendStatus(413)
	}
	f, err := h.Open()
	if err != nil {
		return ttsError(c, err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (10<<20)+1))
	if err != nil {
		return ttsError(c, err)
	}
	if len(b) > 10<<20 {
		return c.SendStatus(413)
	}
	media, err := ttsprovider.InspectMedia(b)
	if err != nil {
		return ttsError(c, err)
	}
	if media.DurationMS < 10000 || media.DurationMS > 60000 {
		return c.Status(400).JSON(fiber.Map{"message": "复刻样本须为 10–60 秒的 WAV 或 MP3"})
	}
	dir, err := service.TTSSpoolDir()
	if err != nil {
		return ttsError(c, err)
	}
	tmp, err := os.CreateTemp(dir, "source-*")
	if err != nil {
		return ttsError(c, err)
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(b); err != nil {
		tmp.Close()
		return ttsError(c, err)
	}
	tmp.Close()
	a, err := service.TTSPersistAudio(getCurUser(c).ID, "", "", "clone_source", tmp.Name(), media)
	if err != nil {
		return ttsError(c, err)
	}
	return c.JSON(fiber.Map{"id": a.ID, "durationMs": media.DurationMS})
}
func ttsAdminConfig(c *fiber.Ctx) error {
	configMutationMu.Lock()
	defer configMutationMu.Unlock()
	var b map[string]json.RawMessage
	if err := json.Unmarshal(c.Body(), &b); err != nil || b == nil {
		return c.SendStatus(400)
	}
	if rawFormat, exists := b["format"]; exists {
		if err := validateExplicitTTSFormat(rawFormat); err != nil {
			var validation service.TTSValidationError
			if errors.As(err, &validation) {
				return ttsError(c, err)
			}
			return c.SendStatus(400)
		}
	}
	raw, err := json.Marshal(map[string]any{"ai": map[string]json.RawMessage{"speech": c.Body()}})
	if err != nil {
		return ttsError(c, err)
	}
	merged, err := mergeConfigPatchForWrite(appConfig, raw)
	if err != nil {
		return ttsError(c, err)
	}
	if err = utils.ValidateSpeechConfig(merged.AI.Speech); err != nil {
		return ttsError(c, service.TTSValidationError(err.Error()))
	}
	if err = service.ValidateTTSDynamicDefaultVoice(merged.AI.Speech); err != nil {
		return ttsError(c, err)
	}
	if err := utils.WriteConfigChecked(merged); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "配置文件写入失败，运行配置未修改",
		})
	}
	appConfig = merged
	SyncConfigToDB(appConfig, "api")
	go service.TTSPrimeMPSCatalogs()
	return c.JSON(fiber.Map{"config": sanitizeConfigForAdmin(appConfig).AI.Speech})
}

func ttsAdminProviderResolve(c *fiber.Ctx) error {
	var body struct {
		ProviderKind string `json:"providerKind"`
		Model        string `json:"model"`
		BaseURL      string `json:"baseUrl"`
		APIKey       string `json:"apiKey"`
		SecretID     string `json:"secretId"`
		SecretKey    string `json:"secretKey"`
		ProviderID   string `json:"providerId"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Provider 配置请求无效"})
	}
	request := service.TTSProviderResolveRequest{ProviderKind: body.ProviderKind, Model: body.Model, BaseURL: body.BaseURL, APIKey: body.APIKey, SecretID: body.SecretID, SecretKey: body.SecretKey, ProviderID: strings.TrimSpace(body.ProviderID)}
	if request.ProviderID != "" && appConfig != nil && appConfig.AI.Speech != nil {
		for _, provider := range appConfig.AI.Speech.Providers {
			if provider.ID == request.ProviderID {
				request.SavedProvider = &provider
				break
			}
		}
	}
	result, err := service.ResolveTTSProvider(c.UserContext(), request)
	if err == nil {
		return c.JSON(result)
	}
	status := fiber.StatusBadGateway
	var validation service.TTSValidationError
	if errors.As(err, &validation) || errors.Is(err, service.ErrTTSProviderCredential) {
		status = fiber.StatusBadRequest
	}
	return c.Status(status).JSON(fiber.Map{"message": err.Error()})
}

func validateExplicitTTSFormat(raw json.RawMessage) error {
	var format *string
	if err := json.Unmarshal(raw, &format); err != nil {
		return err
	}
	if format == nil {
		return errors.New("语音格式必须是字符串")
	}
	if *format != "" && *format != "wav" && *format != "mp3" {
		return service.TTSValidationError("语音格式不受支持")
	}
	return nil
}

func validateExplicitTTSFormatFromSpeechObject(raw json.RawMessage) error {
	var speech map[string]json.RawMessage
	if err := json.Unmarshal(raw, &speech); err != nil {
		return err
	}
	if speech == nil {
		return errors.New("语音配置必须为对象")
	}
	rawFormat, exists := speech["format"]
	if !exists {
		return nil
	}
	return validateExplicitTTSFormat(rawFormat)
}
