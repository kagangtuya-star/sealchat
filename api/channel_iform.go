package api

import (
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
	Name             string                         `json:"name"`
	Url              string                         `json:"url"`
	EmbedCode        string                         `json:"embedCode"`
	DefaultWidth     int                            `json:"defaultWidth"`
	DefaultHeight    int                            `json:"defaultHeight"`
	DefaultCollapsed bool                           `json:"defaultCollapsed"`
	DefaultFloating  bool                           `json:"defaultFloating"`
	AllowPopout      bool                           `json:"allowPopout"`
	OrderIndex       int                            `json:"orderIndex"`
	MediaOptions     model.ChannelIFormMediaOptions `json:"mediaOptions"`
}

type channelIFormUpdateRequest struct {
	Name             *string                         `json:"name"`
	Url              *string                         `json:"url"`
	EmbedCode        *string                         `json:"embedCode"`
	DefaultWidth     *int                            `json:"defaultWidth"`
	DefaultHeight    *int                            `json:"defaultHeight"`
	DefaultCollapsed *bool                           `json:"defaultCollapsed"`
	DefaultFloating  *bool                           `json:"defaultFloating"`
	AllowPopout      *bool                           `json:"allowPopout"`
	OrderIndex       *int                            `json:"orderIndex"`
	MediaOptions     *model.ChannelIFormMediaOptions `json:"mediaOptions"`
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
	forms, err := model.ChannelIFormList(channelID)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "获取嵌入窗失败")
	}
	return c.JSON(fiber.Map{
		"items": forms,
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
	form, err := buildIFormModelFromCreate(&payload, channelID, user.ID)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
	}
	if err := model.ChannelIFormCreate(form); err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "创建嵌入窗失败")
	}
	if err := broadcastIFormSnapshot(user, channelID); err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "广播更新失败")
	}
	return c.JSON(fiber.Map{
		"item":    form,
		"message": "创建成功",
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
	form, err := model.ChannelIFormGet(channelID, formID)
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
	updates, err := buildIFormUpdateMap(&payload, form)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
	}
	if len(updates) == 0 {
		return c.JSON(fiber.Map{"item": form, "message": "未检测到需要更新的字段"})
	}
	updates["updated_by"] = user.ID
	if err := model.ChannelIFormUpdate(channelID, formID, updates); err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "更新控件失败")
	}
	if err := broadcastIFormSnapshot(user, channelID); err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "广播更新失败")
	}
	form, _ = model.ChannelIFormGet(channelID, formID)
	return c.JSON(fiber.Map{
		"item":    form,
		"message": "更新成功",
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
	if err := model.ChannelIFormDelete(channelID, formID); err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "删除控件失败")
	}
	if err := broadcastIFormSnapshot(user, channelID); err != nil {
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
	forms, err := model.ChannelIFormList(channelID)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "加载控件失败")
	}
	formMap := lo.KeyBy(forms, func(item *model.ChannelIFormModel) string { return item.ID })
	normalized := make([]protocol.ChannelIFormStatePayload, 0, len(states))
	for _, state := range states {
		if state.FormID == "" {
			return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "存在缺少 formId 的推送请求")
		}
		form := formMap[state.FormID]
		if form == nil {
			return wrapErrorStatus(c, fiber.StatusBadRequest, nil, fmt.Sprintf("控件 %s 不存在", state.FormID))
		}
		normalized = append(normalized, normalizeStatePayload(state, form, payload.Force))
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
			Forms:         convertIFormListToProtocol(filteredForms(formMap, normalized)),
			TargetUserIDs: targets,
			Action:        "push",
		},
	}
	dispatchIFormEvent(channelID, event, targets)
	return c.JSON(fiber.Map{"message": "推送成功", "count": len(normalized)})
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
			_ = model.ChannelIFormDelete(channelID, form.ID)
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
	}
	return form, nil
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
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "<script") {
		return "", errors.New("嵌入代码不可包含 <script>")
	}
	if len(trimmed) > 8192 {
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

func broadcastIFormSnapshot(user *model.UserModel, channelID string) error {
	forms, err := model.ChannelIFormList(channelID)
	if err != nil {
		return err
	}
	payload := &protocol.ChannelIFormEventPayload{
		Forms:  convertIFormListToProtocol(forms),
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
	return &protocol.ChannelIForm{
		ID:               item.ID,
		ChannelID:        item.ChannelID,
		Name:             item.Name,
		Url:              item.Url,
		EmbedCode:        item.EmbedCode,
		DefaultWidth:     item.DefaultWidth,
		DefaultHeight:    item.DefaultHeight,
		DefaultCollapsed: item.DefaultCollapsed,
		DefaultFloating:  item.DefaultFloating,
		AllowPopout:      item.AllowPopout,
		OrderIndex:       item.OrderIndex,
		MediaOptions:     protoOpts,
		CreatedBy:        item.CreatedBy,
		UpdatedBy:        item.UpdatedBy,
		CreatedAt:        item.CreatedAt.UnixMilli(),
		UpdatedAt:        item.UpdatedAt.UnixMilli(),
	}
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
