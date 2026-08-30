package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/samber/lo"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/protocol"
	"sealchat/service"
)

const (
	defaultEmbedWidth  = 640
	defaultEmbedHeight = 360
	maxEmbedSize       = 4096
)

type channelIFormCreateRequest struct {
	Name              string                               `json:"name"`
	Url               string                               `json:"url"`
	EmbedCode         string                               `json:"embedCode"`
	DefaultWidth      int                                  `json:"defaultWidth"`
	DefaultHeight     int                                  `json:"defaultHeight"`
	DefaultCollapsed  bool                                 `json:"defaultCollapsed"`
	DefaultFloating   bool                                 `json:"defaultFloating"`
	AllowPopout       bool                                 `json:"allowPopout"`
	OrderIndex        int                                  `json:"orderIndex"`
	MediaOptions      model.ChannelIFormMediaOptions       `json:"mediaOptions"`
	BridgePolicy      *model.ChannelIFormBridgePolicy      `json:"bridgePolicy"`
	TemplateRef       string                               `json:"templateRef"`
	TemplateOverrides *model.ChannelIFormTemplateOverrides `json:"templateOverrides"`
}

type channelIFormUpdateRequest struct {
	Name              *string                              `json:"name"`
	Url               *string                              `json:"url"`
	EmbedCode         *string                              `json:"embedCode"`
	DefaultWidth      *int                                 `json:"defaultWidth"`
	DefaultHeight     *int                                 `json:"defaultHeight"`
	DefaultCollapsed  *bool                                `json:"defaultCollapsed"`
	DefaultFloating   *bool                                `json:"defaultFloating"`
	AllowPopout       *bool                                `json:"allowPopout"`
	OrderIndex        *int                                 `json:"orderIndex"`
	MediaOptions      *model.ChannelIFormMediaOptions      `json:"mediaOptions"`
	BridgePolicy      *model.ChannelIFormBridgePolicy      `json:"bridgePolicy"`
	TemplateRef       *string                              `json:"templateRef"`
	TemplateOverrides *model.ChannelIFormTemplateOverrides `json:"templateOverrides"`
}

type channelIFormPushRequest struct {
	FormID        string                              `json:"formId"`
	Force         bool                                `json:"force"`
	TargetUserIDs []string                            `json:"targetUserIds"`
	State         *protocol.ChannelIFormStatePayload  `json:"state"`
	States        []protocol.ChannelIFormStatePayload `json:"states"`
}

type channelIFormMigrateRequest struct {
	TargetChannelIds []string `json:"targetChannelIds"`
	FormIds          []string `json:"formIds"`
	Mode             string   `json:"mode"`
}

type channelIFormWorldShareRequest struct {
	FormIDs []string `json:"formIds"`
	Enabled bool     `json:"enabled"`
}

func canManageIForm(userID, channelID string) bool {
	if pm.CanWithChannelRole(userID, channelID, pm.PermFuncChannelIFormManage) {
		return true
	}
	ch, err := model.ChannelGet(channelID)
	if err == nil && ch != nil && ch.UserID == userID {
		return true
	}
	return false
}

func canBroadcastIForm(userID, channelID string) bool {
	if pm.CanWithChannelRole(userID, channelID, pm.PermFuncChannelIFormBroadcast) {
		return true
	}
	ch, err := model.ChannelGet(channelID)
	if err == nil && ch != nil && ch.UserID == userID {
		return true
	}
	return false
}

func ChannelIFormList(c *fiber.Ctx) error {
	channelID, user, err := resolveIFormContext(c)
	if err != nil {
		return err
	}
	if !service.CanReadChannelByUserId(user.ID, channelID) {
		return wrapErrorStatus(c, fiber.StatusForbidden, nil, "没有权限访问该频道")
	}
	forms, err := service.ListEffectiveChannelIForms(channelID)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "获取嵌入窗失败")
	}
	return c.JSON(fiber.Map{
		"items": convertIFormViewListToProtocol(forms),
		"total": len(forms),
	})
}

func ChannelIFormCreate(c *fiber.Ctx) error {
	channelID, user, err := resolveIFormContext(c)
	if err != nil {
		return err
	}
	if !canManageIForm(user.ID, channelID) {
		return wrapErrorStatus(c, fiber.StatusForbidden, nil, "没有权限管理 iForm 控件")
	}
	var payload channelIFormCreateRequest
	if err := c.BodyParser(&payload); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求体解析失败")
	}
	var form *model.ChannelIFormModel
	if strings.TrimSpace(payload.TemplateRef) != "" {
		if jsonFieldPresent(c.Body(), "url") || jsonFieldPresent(c.Body(), "embedCode") {
			return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "模板引用控件的 URL 和嵌入代码由模板管理")
		}
		if err := validateTemplateOverridesPayload(c.Body()); err != nil {
			return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
		}
		form, err = buildIFormModelFromTemplate(&payload, channelID, user.ID)
	} else {
		form, err = buildIFormModelFromCreate(&payload, channelID, user.ID)
	}
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
	}
	if err := model.ChannelIFormCreate(form); err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "创建嵌入窗失败")
	}
	if err := broadcastIFormSnapshotsForFormIDs(user, []string{form.ID}); err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "广播更新失败")
	}
	responseForm, responseMeta := resolveIFormResponse(form)
	return c.JSON(fiber.Map{
		"item":             responseForm,
		"templateOrigin":   responseMeta.Origin,
		"templateName":     responseMeta.Name,
		"templateMissing":  responseMeta.TemplateMissing,
		"templateArchived": responseMeta.Archived,
		"message":          "创建成功",
	})
}

func ChannelIFormUpdate(c *fiber.Ctx) error {
	channelID, user, err := resolveIFormContext(c)
	if err != nil {
		return err
	}
	if !canManageIForm(user.ID, channelID) {
		return wrapErrorStatus(c, fiber.StatusForbidden, nil, "没有权限管理 iForm 控件")
	}
	formID := strings.TrimSpace(c.Params("formId"))
	if formID == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "缺少控件ID")
	}
	form, sourceChannelID, err := resolveEffectiveIFormForMutation(channelID, formID)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "获取控件失败")
	}
	if form == nil {
		return wrapErrorStatus(c, fiber.StatusNotFound, nil, "控件不存在")
	}
	var payload channelIFormUpdateRequest
	if err := c.BodyParser(&payload); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求体解析失败")
	}
	if strings.TrimSpace(form.TemplateRef) != "" && (jsonFieldPresent(c.Body(), "url") || jsonFieldPresent(c.Body(), "embedCode")) {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "模板引用控件的 URL 和嵌入代码由模板管理")
	}
	if jsonFieldPresent(c.Body(), "templateRef") {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "模板引用关系不可直接修改")
	}
	if strings.TrimSpace(form.TemplateRef) != "" {
		if err := validateTemplateOverridesPayload(c.Body()); err != nil {
			return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
		}
	}
	var updates map[string]interface{}
	if strings.TrimSpace(form.TemplateRef) != "" {
		updates, err = buildIFormReferenceUpdateMap(&payload, form, c.Body())
	} else {
		updates, err = buildIFormUpdateMap(&payload, form)
	}
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
	}
	if len(updates) == 0 {
		return c.JSON(fiber.Map{"item": form, "message": "未检测到需要更新的字段"})
	}
	updates["updated_by"] = user.ID
	if err := model.ChannelIFormUpdate(sourceChannelID, formID, updates); err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "更新控件失败")
	}
	if err := broadcastIFormSnapshotsForFormIDs(user, []string{formID}); err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "广播更新失败")
	}
	form, _ = model.ChannelIFormGet(sourceChannelID, formID)
	responseForm, responseMeta := resolveIFormResponse(form)
	return c.JSON(fiber.Map{
		"item":             responseForm,
		"templateOrigin":   responseMeta.Origin,
		"templateName":     responseMeta.Name,
		"templateMissing":  responseMeta.TemplateMissing,
		"templateArchived": responseMeta.Archived,
		"message":          "更新成功",
	})
}

func ChannelIFormDelete(c *fiber.Ctx) error {
	channelID, user, err := resolveIFormContext(c)
	if err != nil {
		return err
	}
	if !canManageIForm(user.ID, channelID) {
		return wrapErrorStatus(c, fiber.StatusForbidden, nil, "没有权限管理 iForm 控件")
	}
	formID := strings.TrimSpace(c.Params("formId"))
	if formID == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "缺少控件ID")
	}
	form, err := model.ChannelIFormGet(channelID, formID)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "获取控件失败")
	}
	if form == nil {
		return wrapErrorStatus(c, fiber.StatusNotFound, nil, "控件不存在")
	}
	affectedChannels, err := collectAffectedChannelsForForms([]string{formID})
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "计算受影响频道失败")
	}
	if err := service.ChannelIFormPermanentDelete(channelID, formID); err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "删除控件失败")
	}
	_ = model.GetDB().Where("form_id = ?", formID).Delete(&model.WorldIFormBindingModel{}).Error
	if err := broadcastIFormSnapshotsForChannels(user, affectedChannels); err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "广播更新失败")
	}
	return c.JSON(fiber.Map{"message": "删除成功"})
}

func ChannelIFormPush(c *fiber.Ctx) error {
	channelID, user, err := resolveIFormContext(c)
	if err != nil {
		return err
	}
	if !canBroadcastIForm(user.ID, channelID) {
		return wrapErrorStatus(c, fiber.StatusForbidden, nil, "没有权限推送 iForm 控件")
	}
	var payload channelIFormPushRequest
	if err := c.BodyParser(&payload); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求体解析失败")
	}
	states := payload.States
	if payload.State != nil {
		states = append(states, *payload.State)
	}
	if len(states) == 0 && payload.FormID != "" {
		states = append(states, protocol.ChannelIFormStatePayload{FormID: payload.FormID})
	}
	if len(states) == 0 {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "缺少推送内容")
	}
	forms, err := service.ListEffectiveChannelIForms(channelID)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "加载控件失败")
	}
	formMap := lo.KeyBy(forms, func(item *service.ChannelIFormView) string { return item.ID })
	normalized := make([]protocol.ChannelIFormStatePayload, 0, len(states))
	for _, state := range states {
		if state.FormID == "" {
			return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "存在缺少 formId 的推送请求")
		}
		form := formMap[state.FormID]
		if form == nil {
			return wrapErrorStatus(c, fiber.StatusBadRequest, nil, fmt.Sprintf("控件 %s 不存在", state.FormID))
		}
		normalized = append(normalized, normalizeStatePayload(state, form.ChannelIFormModel, payload.Force))
	}
	trimmedUserTargets := lo.Map(payload.TargetUserIDs, func(item string, _ int) string {
		return strings.TrimSpace(item)
	})
	targets := lo.Uniq(lo.Filter(trimmedUserTargets, func(item string, _ int) bool { return item != "" }))
	event := &protocol.Event{
		Type:    protocol.EventChannelIFormPushed,
		Channel: &protocol.Channel{ID: channelID},
		User: func() *protocol.User {
			if user == nil {
				return nil
			}
			return user.ToProtocolType()
		}(),
		IForm: &protocol.ChannelIFormEventPayload{
			States:        normalized,
			Forms:         convertIFormViewListToProtocol(filteredFormViews(formMap, normalized)),
			TargetUserIDs: targets,
			Action:        "push",
		},
	}
	dispatchIFormEvent(channelID, event, targets)
	return c.JSON(fiber.Map{"message": "推送成功", "count": len(normalized)})
}

func ChannelIFormWorldShare(c *fiber.Ctx) error {
	channelID, user, err := resolveIFormContext(c)
	if err != nil {
		return err
	}
	var payload channelIFormWorldShareRequest
	if err := c.BodyParser(&payload); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求体解析失败")
	}
	if len(payload.FormIDs) == 0 {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "请至少选择一个控件")
	}
	affectedBefore, err := collectAffectedChannelsForForms(payload.FormIDs)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "计算受影响频道失败")
	}
	if err := service.SetWorldSharedChannelIForms(channelID, user.ID, payload.FormIDs, payload.Enabled); err != nil {
		switch {
		case errors.Is(err, service.ErrWorldPermission):
			return wrapErrorStatus(c, fiber.StatusForbidden, nil, "没有权限管理世界共享 iForm")
		case errors.Is(err, service.ErrWorldNotFound):
			return wrapErrorStatus(c, fiber.StatusNotFound, nil, "所属世界不存在")
		default:
			return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
		}
	}
	affectedAfter, err := collectAffectedChannelsForForms(payload.FormIDs)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "计算受影响频道失败")
	}
	if err := broadcastIFormSnapshotsForChannels(user, mergeChannelIDs(affectedBefore, affectedAfter)); err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "广播更新失败")
	}
	return c.JSON(fiber.Map{"message": "操作成功", "enabled": payload.Enabled, "count": len(payload.FormIDs)})
}

func ChannelIFormMigrate(c *fiber.Ctx) error {
	channelID, user, err := resolveIFormContext(c)
	if err != nil {
		return err
	}
	if !pm.CanWithChannelRole(user.ID, channelID, pm.PermFuncChannelIFormManage) {
		return wrapErrorStatus(c, fiber.StatusForbidden, nil, "没有权限管理 iForm 控件")
	}
	var payload channelIFormMigrateRequest
	if err := c.BodyParser(&payload); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求体解析失败")
	}
	trimmedTargets := lo.Map(payload.TargetChannelIds, func(item string, _ int) string {
		return strings.TrimSpace(item)
	})
	targets := lo.Uniq(lo.Filter(trimmedTargets, func(item string, _ int) bool {
		return item != "" && !strings.Contains(item, ":") && item != channelID
	}))
	if len(targets) == 0 {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "请至少选择一个目标频道")
	}
	mode := strings.ToLower(strings.TrimSpace(payload.Mode))
	if mode == "" {
		mode = "copy"
	}
	if mode != "copy" && mode != "move" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "模式仅支持 copy 或 move")
	}
	if mode == "move" && len(targets) != 1 {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "移动模式仅支持一个目标频道")
	}
	sourceForms, err := model.ChannelIFormList(channelID)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "加载控件失败")
	}
	selected := sourceForms
	if len(payload.FormIds) > 0 {
		ids := lo.SliceToMap(payload.FormIds, func(id string) (string, struct{}) {
			return strings.TrimSpace(id), struct{}{}
		})
		filtered := []*model.ChannelIFormModel{}
		for _, form := range sourceForms {
			if _, ok := ids[form.ID]; ok {
				filtered = append(filtered, form)
			}
		}
		selected = filtered
	}
	if len(selected) == 0 {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "未找到可迁移的控件")
	}
	summary := []fiber.Map{}
	for _, targetID := range targets {
		if !canManageIForm(user.ID, targetID) {
			return wrapErrorStatus(c, fiber.StatusForbidden, nil, fmt.Sprintf("没有权限操作目标频道 %s", targetID))
		}
		tgt, err := model.ChannelGet(targetID)
		if err != nil {
			return wrapErrorStatus(c, fiber.StatusInternalServerError, err, fmt.Sprintf("获取频道 %s 失败", targetID))
		}
		if tgt == nil || tgt.ID == "" {
			return wrapErrorStatus(c, fiber.StatusNotFound, nil, fmt.Sprintf("频道 %s 不存在", targetID))
		}
		for _, form := range selected {
			if _, err := model.ChannelIFormCloneToChannel(form, targetID, user.ID); err != nil {
				return wrapErrorStatus(c, fiber.StatusInternalServerError, err, fmt.Sprintf("复制控件 %s 失败", form.ID))
			}
		}
		_ = broadcastIFormSnapshot(user, targetID)
		summary = append(summary, fiber.Map{
			"channelId": targetID,
			"count":     len(selected),
		})
	}
	if mode == "move" {
		for _, form := range selected {
			_ = service.ChannelIFormPermanentDelete(channelID, form.ID)
		}
		_ = broadcastIFormSnapshot(user, channelID)
	}
	return c.JSON(fiber.Map{
		"message": "操作完成",
		"mode":    mode,
		"targets": summary,
	})
}

func resolveIFormContext(c *fiber.Ctx) (string, *model.UserModel, error) {
	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return "", nil, wrapErrorStatus(c, fiber.StatusBadRequest, nil, "缺少频道ID")
	}
	if strings.Contains(channelID, ":") {
		return "", nil, wrapErrorStatus(c, fiber.StatusBadRequest, nil, "暂不支持私聊频道使用 iForm 控件")
	}
	user := getCurUser(c)
	if user == nil {
		return "", nil, wrapErrorStatus(c, fiber.StatusUnauthorized, nil, "未登录")
	}
	ch, err := model.ChannelGet(channelID)
	if err != nil {
		return "", nil, wrapErrorStatus(c, fiber.StatusInternalServerError, err, "校验频道失败")
	}
	if ch == nil || ch.ID == "" {
		return "", nil, wrapErrorStatus(c, fiber.StatusNotFound, nil, "频道不存在")
	}
	return channelID, user, nil
}

func resolveEffectiveIFormForMutation(channelID, formID string) (*model.ChannelIFormModel, string, error) {
	form, err := model.ChannelIFormGet(channelID, formID)
	if err != nil {
		return nil, "", err
	}
	if form != nil {
		return form, channelID, nil
	}

	forms, err := service.ListEffectiveChannelIForms(channelID)
	if err != nil {
		return nil, "", err
	}
	for _, item := range forms {
		if item == nil || item.ChannelIFormModel == nil || strings.TrimSpace(item.ID) != formID {
			continue
		}
		sourceChannelID := strings.TrimSpace(item.ChannelIFormModel.ChannelID)
		if sourceChannelID == "" {
			sourceChannelID = strings.TrimSpace(item.SourceChannelID)
		}
		if sourceChannelID == "" {
			return nil, "", nil
		}
		return item.ChannelIFormModel, sourceChannelID, nil
	}
	return nil, "", nil
}

func buildIFormModelFromCreate(payload *channelIFormCreateRequest, channelID, actor string) (*model.ChannelIFormModel, error) {
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		return nil, errors.New("名称不能为空")
	}
	if utf8.RuneCountInString(name) > 64 {
		return nil, errors.New("名称长度不能超过64字符")
	}
	urlVal, err := normalizeURL(payload.Url)
	if err != nil {
		return nil, err
	}
	embedVal, err := sanitizeEmbedCode(payload.EmbedCode)
	if err != nil {
		return nil, err
	}
	if urlVal == "" && embedVal == "" {
		return nil, errors.New("需要提供 URL 或嵌入代码")
	}
	form := &model.ChannelIFormModel{
		ChannelID:        channelID,
		Name:             name,
		Url:              urlVal,
		EmbedCode:        embedVal,
		DefaultWidth:     sanitizeSize(payload.DefaultWidth, defaultEmbedWidth),
		DefaultHeight:    sanitizeSize(payload.DefaultHeight, defaultEmbedHeight),
		DefaultCollapsed: payload.DefaultCollapsed,
		DefaultFloating:  payload.DefaultFloating,
		AllowPopout:      payload.AllowPopout,
		OrderIndex:       payload.OrderIndex,
		CreatedBy:        actor,
		UpdatedBy:        actor,
		MediaOptions:     normalizeMediaOptions(payload.MediaOptions),
		BridgePolicy:     normalizeBridgePolicy(payload.BridgePolicy),
	}
	return form, nil
}

func buildIFormModelFromTemplate(payload *channelIFormCreateRequest, channelID, actor string) (*model.ChannelIFormModel, error) {
	ref := strings.TrimSpace(payload.TemplateRef)
	template, err := service.ResolveChannelIFormTemplate(ref)
	if err != nil {
		return nil, err
	}
	if !template.Installable {
		return nil, errors.New("模板已归档或未启用，不能安装")
	}
	overrides := model.ChannelIFormTemplateOverrides{}
	if payload.TemplateOverrides != nil {
		overrides = *payload.TemplateOverrides
	}
	if err := validateIFormTemplateOverrides(&overrides); err != nil {
		return nil, err
	}
	return &model.ChannelIFormModel{
		ChannelID: channelID, TemplateRef: ref, TemplateOverrides: overrides,
		OrderIndex: payload.OrderIndex, CreatedBy: actor, UpdatedBy: actor,
	}, nil
}

func validateIFormTemplateOverrides(overrides *model.ChannelIFormTemplateOverrides) error {
	if overrides == nil {
		return nil
	}
	if overrides.Name != nil {
		name := strings.TrimSpace(*overrides.Name)
		if name == "" || utf8.RuneCountInString(name) > 64 {
			return errors.New("名称长度必须为1-64字符")
		}
		overrides.Name = &name
	}
	if overrides.DefaultWidth != nil && (*overrides.DefaultWidth <= 0 || *overrides.DefaultWidth > maxEmbedSize) {
		return errors.New("默认宽度超出范围")
	}
	if overrides.DefaultHeight != nil && (*overrides.DefaultHeight <= 0 || *overrides.DefaultHeight > maxEmbedSize) {
		return errors.New("默认高度超出范围")
	}
	return nil
}

func buildIFormReferenceUpdateMap(payload *channelIFormUpdateRequest, current *model.ChannelIFormModel, body []byte) (map[string]interface{}, error) {
	if payload.Url != nil || payload.EmbedCode != nil {
		return nil, errors.New("模板引用控件的 URL 和嵌入代码由模板管理")
	}
	overrides, err := mergeIFormTemplateOverridesPayload(body, current.TemplateOverrides)
	if err != nil {
		return nil, err
	}
	if payload.Name != nil {
		value := strings.TrimSpace(*payload.Name)
		overrides.Name = &value
	}
	if payload.DefaultWidth != nil {
		overrides.DefaultWidth = payload.DefaultWidth
	}
	if payload.DefaultHeight != nil {
		overrides.DefaultHeight = payload.DefaultHeight
	}
	if payload.DefaultCollapsed != nil {
		overrides.DefaultCollapsed = payload.DefaultCollapsed
	}
	if payload.DefaultFloating != nil {
		overrides.DefaultFloating = payload.DefaultFloating
	}
	if payload.AllowPopout != nil {
		overrides.AllowPopout = payload.AllowPopout
	}
	if payload.MediaOptions != nil {
		overrides.MediaOptions = payload.MediaOptions
	}
	if payload.BridgePolicy != nil {
		overrides.BridgePolicy = payload.BridgePolicy
	}
	if err := validateIFormTemplateOverrides(&overrides); err != nil {
		return nil, err
	}
	updates := map[string]interface{}{"template_overrides": overrides}
	if payload.OrderIndex != nil {
		updates["order_index"] = *payload.OrderIndex
	}
	return updates, nil
}

// mergeIFormTemplateOverridesPayload applies only the keys present in the
// request. A JSON null explicitly removes that override; omitted keys keep the
// existing sparse value. This preserves both sparse semantics and a practical
// per-field "restore default" API.
func mergeIFormTemplateOverridesPayload(body []byte, current model.ChannelIFormTemplateOverrides) (model.ChannelIFormTemplateOverrides, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return current, errors.New("请求体 JSON 无效")
	}
	raw, ok := root["templateOverrides"]
	if !ok {
		return current, nil
	}
	if strings.TrimSpace(string(raw)) == "null" {
		return model.ChannelIFormTemplateOverrides{}, nil
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return current, errors.New("templateOverrides 必须是对象")
	}
	for key, rawValue := range values {
		if strings.TrimSpace(string(rawValue)) == "null" {
			switch key {
			case "name":
				current.Name = nil
			case "defaultWidth":
				current.DefaultWidth = nil
			case "defaultHeight":
				current.DefaultHeight = nil
			case "defaultCollapsed":
				current.DefaultCollapsed = nil
			case "defaultFloating":
				current.DefaultFloating = nil
			case "allowPopout":
				current.AllowPopout = nil
			case "mediaOptions":
				current.MediaOptions = nil
			case "bridgePolicy":
				current.BridgePolicy = nil
			}
			continue
		}
		switch key {
		case "name":
			var decoded string
			if err := json.Unmarshal(rawValue, &decoded); err != nil {
				return current, errors.New("模板覆盖字段 name 类型无效")
			}
			current.Name = &decoded
		case "defaultWidth":
			var decoded int
			if err := json.Unmarshal(rawValue, &decoded); err != nil {
				return current, errors.New("模板覆盖字段 defaultWidth 类型无效")
			}
			current.DefaultWidth = &decoded
		case "defaultHeight":
			var decoded int
			if err := json.Unmarshal(rawValue, &decoded); err != nil {
				return current, errors.New("模板覆盖字段 defaultHeight 类型无效")
			}
			current.DefaultHeight = &decoded
		case "defaultCollapsed":
			var decoded bool
			if err := json.Unmarshal(rawValue, &decoded); err != nil {
				return current, errors.New("模板覆盖字段 defaultCollapsed 类型无效")
			}
			current.DefaultCollapsed = &decoded
		case "defaultFloating":
			var decoded bool
			if err := json.Unmarshal(rawValue, &decoded); err != nil {
				return current, errors.New("模板覆盖字段 defaultFloating 类型无效")
			}
			current.DefaultFloating = &decoded
		case "allowPopout":
			var decoded bool
			if err := json.Unmarshal(rawValue, &decoded); err != nil {
				return current, errors.New("模板覆盖字段 allowPopout 类型无效")
			}
			current.AllowPopout = &decoded
		case "mediaOptions":
			var decoded model.ChannelIFormMediaOptions
			if err := json.Unmarshal(rawValue, &decoded); err != nil {
				return current, errors.New("模板覆盖字段 mediaOptions 类型无效")
			}
			current.MediaOptions = &decoded
		case "bridgePolicy":
			var decoded model.ChannelIFormBridgePolicy
			if err := json.Unmarshal(rawValue, &decoded); err != nil {
				return current, errors.New("模板覆盖字段 bridgePolicy 类型无效")
			}
			current.BridgePolicy = &decoded
		}
	}
	return current, nil
}

func jsonFieldPresent(body []byte, field string) bool {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	_, ok := payload[field]
	if !ok && field == "embedCode" {
		_, ok = payload["embed_code"]
	}
	return ok
}

func resolveIFormResponse(raw *model.ChannelIFormModel) (*model.ChannelIFormModel, service.ChannelIFormTemplateMetadata) {
	if raw == nil {
		return nil, service.ChannelIFormTemplateMetadata{}
	}
	resolved, err := service.ResolveChannelIForm(raw)
	if err != nil {
		// A reference must never fall back to a stored source snapshot when its
		// registry/database lookup fails. Keep the relationship visible while
		// ensuring runtime consumers cannot execute stale URL/embed content.
		if strings.TrimSpace(raw.TemplateRef) != "" {
			missing := *raw
			missing.Url = ""
			missing.EmbedCode = ""
			return &missing, service.ChannelIFormTemplateMetadata{
				Ref:             raw.TemplateRef,
				Origin:          strings.SplitN(raw.TemplateRef, ":", 2)[0],
				TemplateMissing: true,
			}
		}
		return raw, service.ChannelIFormTemplateMetadata{}
	}
	if resolved == nil || resolved.Form == nil {
		return raw, service.ChannelIFormTemplateMetadata{}
	}
	return resolved.Form, resolved.Metadata
}

func validateTemplateOverridesPayload(body []byte) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return errors.New("请求体 JSON 无效")
	}
	raw, ok := root["templateOverrides"]
	if !ok || strings.TrimSpace(string(raw)) == "null" {
		return nil
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return errors.New("templateOverrides 必须是对象")
	}
	allowed := map[string]struct{}{
		"name": {}, "defaultWidth": {}, "defaultHeight": {}, "defaultCollapsed": {},
		"defaultFloating": {}, "allowPopout": {}, "mediaOptions": {}, "bridgePolicy": {},
	}
	for key, value := range values {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("不支持的模板覆盖字段: %s", key)
		}
		if key == "mediaOptions" || key == "bridgePolicy" {
			var nested map[string]json.RawMessage
			if err := json.Unmarshal(value, &nested); err != nil {
				return fmt.Errorf("模板覆盖字段 %s 必须是对象", key)
			}
			nestedAllowed := map[string]struct{}{}
			if key == "mediaOptions" {
				nestedAllowed = map[string]struct{}{"autoPlay": {}, "autoUnmute": {}, "autoExpand": {}, "allowAudio": {}, "allowVideo": {}}
			} else {
				nestedAllowed = map[string]struct{}{"enabled": {}, "allowedOrigins": {}, "capabilities": {}}
			}
			for nestedKey := range nested {
				if _, ok := nestedAllowed[nestedKey]; !ok {
					return fmt.Errorf("不支持的模板覆盖字段: %s.%s", key, nestedKey)
				}
			}
		}
	}
	return nil
}

func buildIFormUpdateMap(payload *channelIFormUpdateRequest, current *model.ChannelIFormModel) (map[string]interface{}, error) {
	updates := map[string]interface{}{}
	if payload.Name != nil {
		name := strings.TrimSpace(*payload.Name)
		if name == "" {
			return nil, errors.New("名称不能为空")
		}
		if utf8.RuneCountInString(name) > 64 {
			return nil, errors.New("名称长度不能超过64字符")
		}
		updates["name"] = name
	}
	finalURL := strings.TrimSpace(current.Url)
	finalEmbed := strings.TrimSpace(current.EmbedCode)
	if payload.Url != nil {
		urlVal, err := normalizeURL(*payload.Url)
		if err != nil {
			return nil, err
		}
		updates["url"] = urlVal
		finalURL = urlVal
	}
	if payload.EmbedCode != nil {
		embedVal, err := sanitizeEmbedCode(*payload.EmbedCode)
		if err != nil {
			return nil, err
		}
		updates["embed_code"] = embedVal
		finalEmbed = embedVal
	}
	if strings.TrimSpace(finalURL) == "" && strings.TrimSpace(finalEmbed) == "" {
		return nil, errors.New("需要保留 URL 或嵌入代码")
	}
	if payload.DefaultWidth != nil {
		updates["default_width"] = sanitizeSize(*payload.DefaultWidth, defaultEmbedWidth)
	}
	if payload.DefaultHeight != nil {
		updates["default_height"] = sanitizeSize(*payload.DefaultHeight, defaultEmbedHeight)
	}
	if payload.DefaultCollapsed != nil {
		updates["default_collapsed"] = *payload.DefaultCollapsed
	}
	if payload.DefaultFloating != nil {
		updates["default_floating"] = *payload.DefaultFloating
	}
	if payload.AllowPopout != nil {
		updates["allow_popout"] = *payload.AllowPopout
	}
	if payload.OrderIndex != nil {
		updates["order_index"] = *payload.OrderIndex
	}
	if payload.MediaOptions != nil {
		updates["media_options"] = normalizeMediaOptions(*payload.MediaOptions)
	}
	if payload.BridgePolicy != nil {
		updates["bridge_policy"] = normalizeBridgePolicy(payload.BridgePolicy)
	}
	return updates, nil
}

func normalizeURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" {
		return "", errors.New("URL 仅支持 http/https 协议")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("URL 仅支持 http/https 协议")
	}
	return parsed.String(), nil
}

func sanitizeEmbedCode(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if len(trimmed) > 88192 {
		return "", errors.New("嵌入代码过长")
	}
	return trimmed, nil
}

func sanitizeSize(input, fallback int) int {
	if input <= 0 {
		return fallback
	}
	if input > maxEmbedSize {
		return maxEmbedSize
	}
	return input
}

func normalizeMediaOptions(opts model.ChannelIFormMediaOptions) model.ChannelIFormMediaOptions {
	normalized := opts
	if !normalized.AllowAudio && !normalized.AllowVideo {
		normalized.AllowAudio = true
		normalized.AllowVideo = true
	}
	return normalized
}

func normalizeBridgePolicy(policy *model.ChannelIFormBridgePolicy) model.ChannelIFormBridgePolicy {
	if policy == nil {
		return model.ChannelIFormBridgePolicy{}
	}
	result := *policy
	seen := map[string]struct{}{}
	allowed := make([]string, 0, len(result.AllowedOrigins))
	for _, origin := range result.AllowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		if _, ok := seen[origin]; ok {
			continue
		}
		seen[origin] = struct{}{}
		allowed = append(allowed, origin)
	}
	result.AllowedOrigins = allowed
	seen = map[string]struct{}{}
	caps := make([]string, 0, len(result.Capabilities))
	for _, capability := range result.Capabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" {
			continue
		}
		if _, ok := seen[capability]; ok {
			continue
		}
		seen[capability] = struct{}{}
		caps = append(caps, capability)
	}
	result.Capabilities = caps
	return result
}

func broadcastIFormSnapshot(user *model.UserModel, channelID string) error {
	forms, err := service.ListEffectiveChannelIForms(channelID)
	if err != nil {
		return err
	}
	payload := &protocol.ChannelIFormEventPayload{
		Forms:  convertIFormViewListToProtocol(forms),
		Action: "snapshot",
	}
	event := &protocol.Event{
		Type:    protocol.EventChannelIFormUpdated,
		Channel: &protocol.Channel{ID: channelID},
		User: func() *protocol.User {
			if user == nil {
				return nil
			}
			return user.ToProtocolType()
		}(),
		IForm: payload,
	}
	dispatchIFormEvent(channelID, event, nil)
	return nil
}

func broadcastIFormSnapshotsForFormIDs(user *model.UserModel, formIDs []string) error {
	channelSet := map[string]struct{}{}
	for _, raw := range formIDs {
		formID := strings.TrimSpace(raw)
		if formID == "" {
			continue
		}
		channelIDs, err := service.ListChannelsAffectedByIForm(formID)
		if err != nil {
			return err
		}
		for _, channelID := range channelIDs {
			channelID = strings.TrimSpace(channelID)
			if channelID == "" {
				continue
			}
			channelSet[channelID] = struct{}{}
		}
	}
	channelIDs := make([]string, 0, len(channelSet))
	for channelID := range channelSet {
		channelIDs = append(channelIDs, channelID)
	}
	return broadcastIFormSnapshotsForChannels(user, channelIDs)
}

func broadcastIFormSnapshotsForChannels(user *model.UserModel, channelIDs []string) error {
	for _, channelID := range channelIDs {
		channelID = strings.TrimSpace(channelID)
		if channelID == "" {
			continue
		}
		if err := broadcastIFormSnapshot(user, channelID); err != nil {
			return err
		}
	}
	return nil
}

func collectAffectedChannelsForForms(formIDs []string) ([]string, error) {
	channelSet := map[string]struct{}{}
	for _, raw := range formIDs {
		formID := strings.TrimSpace(raw)
		if formID == "" {
			continue
		}
		channelIDs, err := service.ListChannelsAffectedByIForm(formID)
		if err != nil {
			return nil, err
		}
		for _, channelID := range channelIDs {
			channelID = strings.TrimSpace(channelID)
			if channelID == "" {
				continue
			}
			channelSet[channelID] = struct{}{}
		}
	}
	result := make([]string, 0, len(channelSet))
	for channelID := range channelSet {
		result = append(result, channelID)
	}
	return result, nil
}

func mergeChannelIDs(chunks ...[]string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, chunk := range chunks {
		for _, raw := range chunk {
			channelID := strings.TrimSpace(raw)
			if channelID == "" {
				continue
			}
			if _, ok := seen[channelID]; ok {
				continue
			}
			seen[channelID] = struct{}{}
			result = append(result, channelID)
		}
	}
	return result
}

func convertIFormListToProtocol(items []*model.ChannelIFormModel) []*protocol.ChannelIForm {
	result := make([]*protocol.ChannelIForm, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		result = append(result, convertIFormToProtocol(item))
	}
	return result
}

func convertIFormToProtocol(item *model.ChannelIFormModel) *protocol.ChannelIForm {
	if item == nil {
		return nil
	}
	opts := item.MediaOptions
	protoOpts := &protocol.ChannelIFormMediaOptions{
		AutoPlay:   opts.AutoPlay,
		AutoUnmute: opts.AutoUnmute,
		AutoExpand: opts.AutoExpand,
		AllowAudio: opts.AllowAudio,
		AllowVideo: opts.AllowVideo,
	}
	var templateOverrides any
	if strings.TrimSpace(item.TemplateRef) != "" {
		templateOverrides = item.TemplateOverrides
	}
	return &protocol.ChannelIForm{
		ID:                item.ID,
		ChannelID:         item.ChannelID,
		Name:              item.Name,
		Url:               item.Url,
		EmbedCode:         item.EmbedCode,
		DefaultWidth:      item.DefaultWidth,
		DefaultHeight:     item.DefaultHeight,
		DefaultCollapsed:  item.DefaultCollapsed,
		DefaultFloating:   item.DefaultFloating,
		AllowPopout:       item.AllowPopout,
		OrderIndex:        item.OrderIndex,
		MediaOptions:      protoOpts,
		CreatedBy:         item.CreatedBy,
		UpdatedBy:         item.UpdatedBy,
		CreatedAt:         item.CreatedAt.UnixMilli(),
		UpdatedAt:         item.UpdatedAt.UnixMilli(),
		TemplateRef:       item.TemplateRef,
		TemplateOverrides: templateOverrides,
		BridgePolicy: &protocol.ChannelIFormBridgePolicy{
			Enabled:        item.BridgePolicy.Enabled,
			AllowedOrigins: append([]string(nil), item.BridgePolicy.AllowedOrigins...),
			Capabilities:   append([]string(nil), item.BridgePolicy.Capabilities...),
		},
	}
}

func convertIFormViewListToProtocol(items []*service.ChannelIFormView) []*protocol.ChannelIForm {
	result := make([]*protocol.ChannelIForm, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		result = append(result, convertIFormViewToProtocol(item))
	}
	return result
}

func convertIFormViewToProtocol(item *service.ChannelIFormView) *protocol.ChannelIForm {
	if item == nil || item.ChannelIFormModel == nil {
		return nil
	}
	protoItem := convertIFormToProtocol(item.ChannelIFormModel)
	if protoItem == nil {
		return nil
	}
	protoItem.SourceChannelID = item.SourceChannelID
	protoItem.WorldShared = item.WorldShared
	protoItem.SharedRef = item.SharedRef
	protoItem.SharedWorldID = item.SharedWorldID
	protoItem.Readonly = item.Readonly
	protoItem.TemplateOrigin = item.TemplateOrigin
	protoItem.TemplateName = item.TemplateName
	protoItem.TemplateMissing = item.TemplateMissing
	protoItem.TemplateArchived = item.TemplateArchived
	return protoItem
}

func dispatchIFormEvent(channelID string, event *protocol.Event, targets []string) {
	if event == nil || channelUsersMapGlobal == nil || userId2ConnInfoGlobal == nil {
		return
	}
	ctx := &ChatContext{
		ChannelUsersMap: channelUsersMapGlobal,
		UserId2ConnInfo: userId2ConnInfoGlobal,
	}
	if len(targets) > 0 {
		ctx.BroadcastEventInChannelToUsers(channelID, targets, event)
		return
	}
	ctx.BroadcastEventInChannel(channelID, event)
}

func normalizeStatePayload(state protocol.ChannelIFormStatePayload, form *model.ChannelIFormModel, force bool) protocol.ChannelIFormStatePayload {
	normalized := state
	normalized.Width = sanitizeSize(normalized.Width, form.DefaultWidth)
	normalized.Height = sanitizeSize(normalized.Height, form.DefaultHeight)
	normalized.AutoPlay = normalized.AutoPlay || form.MediaOptions.AutoPlay
	normalized.AutoUnmute = normalized.AutoUnmute || form.MediaOptions.AutoUnmute
	if force {
		normalized.Force = true
	}
	return normalized
}

func filteredForms(formMap map[string]*model.ChannelIFormModel, states []protocol.ChannelIFormStatePayload) []*model.ChannelIFormModel {
	seen := map[string]struct{}{}
	result := make([]*model.ChannelIFormModel, 0, len(states))
	for _, state := range states {
		if _, exists := seen[state.FormID]; exists {
			continue
		}
		if form := formMap[state.FormID]; form != nil {
			result = append(result, form)
			seen[state.FormID] = struct{}{}
		}
	}
	return result
}

func filteredFormViews(formMap map[string]*service.ChannelIFormView, states []protocol.ChannelIFormStatePayload) []*service.ChannelIFormView {
	seen := map[string]struct{}{}
	result := make([]*service.ChannelIFormView, 0, len(states))
	for _, state := range states {
		if _, exists := seen[state.FormID]; exists {
			continue
		}
		if form := formMap[state.FormID]; form != nil {
			result = append(result, form)
			seen[state.FormID] = struct{}{}
		}
	}
	return result
}
