package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/service"
)

// Channel embed MCP tools are a façade over the native ChannelIForm REST rules:
// create/update/delete run createChannelIForm/updateChannelIForm/deleteChannelIForm
// with a REST-equivalent JSON body, so URL, embedCode size, template reference,
// TemplateOverrides and BridgePolicy normalization stay identical.

type mcpEmbedReadInput struct {
	mcpChannelInput
	mcpPageInput
	ResourceID string `json:"resourceId,omitempty"`
	Search     string `json:"search,omitempty"`
}

type mcpEmbedCatalogInput struct {
	mcpPageInput
	Search string `json:"search,omitempty"`
	Origin string `json:"origin,omitempty"`
}

type mcpEmbedSaveInput struct {
	mcpChannelInput
	ResourceID       string                          `json:"resourceId,omitempty"`
	Name             *string                         `json:"name,omitempty"`
	URL              *string                         `json:"url,omitempty"`
	EmbedCode        *string                         `json:"embedCode,omitempty"`
	DefaultWidth     *int                            `json:"defaultWidth,omitempty"`
	DefaultHeight    *int                            `json:"defaultHeight,omitempty"`
	DefaultCollapsed *bool                           `json:"defaultCollapsed,omitempty"`
	DefaultFloating  *bool                           `json:"defaultFloating,omitempty"`
	AllowPopout      *bool                           `json:"allowPopout,omitempty"`
	OrderIndex       *int                            `json:"orderIndex,omitempty"`
	MediaOptions     *model.ChannelIFormMediaOptions `json:"mediaOptions,omitempty"`
	BridgePolicy     *model.ChannelIFormBridgePolicy `json:"bridgePolicy,omitempty"`
	TemplateRef      *string                         `json:"templateRef,omitempty"`
	// Native sparse override semantics: a key set to null restores the template
	// default; keys are validated by validateTemplateOverridesPayload.
	TemplateOverrides map[string]any `json:"templateOverrides,omitempty"`
}

// mcpEmbedTheaterRef is ready for theater_object content.iframe.url and
// theater_scene surface_embed_set url; it passes mcpTheaterValidateIframe when
// the Theater scope's inputChannelId (and channel Theater channelId) is ChannelID.
type mcpEmbedTheaterRef struct {
	WorldID       string   `json:"worldId"`
	ChannelID     string   `json:"channelId"`
	FormID        string   `json:"formId"`
	URL           string   `json:"url,omitempty"`
	AlternateURLs []string `json:"alternateUrls,omitempty"`
	InternalPath  string   `json:"internalPath"`
}

type mcpEmbedSummaryDTO struct {
	ID               string                         `json:"id"`
	Name             string                         `json:"name"`
	WorldID          string                         `json:"worldId"`
	ChannelID        string                         `json:"channelId"`
	SourceChannelID  string                         `json:"sourceChannelId"`
	URL              string                         `json:"url,omitempty"`
	HasEmbedCode     bool                           `json:"hasEmbedCode"`
	EmbedCodeBytes   int                            `json:"embedCodeBytes"`
	SourceEditable   bool                           `json:"sourceEditable"`
	DefaultWidth     int                            `json:"defaultWidth"`
	DefaultHeight    int                            `json:"defaultHeight"`
	BridgePolicy     model.ChannelIFormBridgePolicy `json:"bridgePolicy"`
	TemplateRef      string                         `json:"templateRef,omitempty"`
	TemplateOrigin   string                         `json:"templateOrigin,omitempty"`
	TemplateName     string                         `json:"templateName,omitempty"`
	TemplateMissing  bool                           `json:"templateMissing,omitempty"`
	TemplateArchived bool                           `json:"templateArchived,omitempty"`
	WorldShared      bool                           `json:"worldShared"`
	SharedRef        bool                           `json:"sharedRef"`
	UpdatedAt        int64                          `json:"updatedAt"`
	Theater          mcpEmbedTheaterRef             `json:"theater"`
}

type mcpEmbedDetailDTO struct {
	mcpEmbedSummaryDTO
	EmbedCode         string                               `json:"embedCode"`
	DefaultCollapsed  bool                                 `json:"defaultCollapsed"`
	DefaultFloating   bool                                 `json:"defaultFloating"`
	AllowPopout       bool                                 `json:"allowPopout"`
	OrderIndex        int                                  `json:"orderIndex"`
	MediaOptions      model.ChannelIFormMediaOptions       `json:"mediaOptions"`
	TemplateOverrides *model.ChannelIFormTemplateOverrides `json:"templateOverrides,omitempty"`
	SharedWorldID     string                               `json:"sharedWorldId,omitempty"`
	CreatedBy         string                               `json:"createdBy"`
	UpdatedBy         string                               `json:"updatedBy"`
	CreatedAt         int64                                `json:"createdAt"`
}

type mcpEmbedWriteRefDTO struct {
	ID        string             `json:"id"`
	WorldID   string             `json:"worldId"`
	ChannelID string             `json:"channelId"`
	Changed   bool               `json:"changed"`
	Theater   mcpEmbedTheaterRef `json:"theater"`
}

type mcpEmbedCapability struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Mutating    bool   `json:"mutating,omitempty"`
	Context     string `json:"context,omitempty"`
}

// Keep in sync with defaultCapabilities and the theater capabilities accepted by
// ui/src/bridge/channelEmbedHost.ts. This is a catalog, not an allowlist: the
// host still decides the effective capabilities for each session.
var mcpEmbedCapabilities = []mcpEmbedCapability{
	{ID: "context.read", Description: "读取当前世界、频道、用户、连接与权限上下文"},
	{ID: "user.read", Description: "读取当前登录用户公开资料"},
	{ID: "members.read", Description: "读取在线成员与公会成员列表"},
	{ID: "world.admins.read", Description: "读取世界管理员列表"},
	{ID: "characters.read", Description: "读取当前频道可用角色与当前角色"},
	{ID: "characterCard.read", Description: "读取当前人物卡与人物卡快照"},
	{ID: "characterCard.write", Description: "修改当前用户自己的当前人物卡属性", Mutating: true},
	{ID: "permissions.read", Description: "读取当前有效能力与权限"},
	{ID: "storage.read", Description: "读取 (频道, iForm) 作用域 JSON Storage"},
	{ID: "storage.write", Description: "写入 (频道, iForm) 作用域 JSON Storage", Mutating: true},
	{ID: "events.subscribe", Description: "订阅同一频道、同一 iForm 的瞬时事件"},
	{ID: "events.publish", Description: "发布瞬时事件；在 Theater 普通 iframe 对象中成功发布后，可按对象保存的 embedEventBindings 触发该对象动作", Mutating: true},
	{ID: "messages.send", Description: "经正常消息链路向当前频道发送消息", Mutating: true},
	{ID: "attachments.upload", Description: "上传图片附件", Mutating: true},
	{ID: "theater.dialogue.subscribe", Description: "订阅小剧场透明对话（非默认能力）"},
	{ID: "theater.character.read", Description: "读取小剧场角色快照", Context: "仅小剧场幕布中的直接 iForm 宣告"},
}

func mcpEmbedChannel(a *service.MCPActor, worldID, channelID string) (*model.ChannelModel, error) {
	if strings.Contains(channelID, ":") {
		return nil, mcpFailure("invalid_argument", "私聊频道不支持频道嵌入")
	}
	return mcpChannel(a, worldID, channelID)
}

func mcpEmbedFind(channelID, formID string) (*service.ChannelIFormView, error) {
	forms, err := service.ListEffectiveChannelIForms(channelID)
	if err != nil {
		return nil, err
	}
	for _, item := range forms {
		if item != nil && item.ChannelIFormModel != nil && item.ID == formID {
			return item, nil
		}
	}
	return nil, mcpFailure("not_found", "频道嵌入不存在或不可见")
}

// mcpEmbedTheaterURLs builds internal iForm links on configured public origins,
// primary domain first; the current MCP origin is only a loopback fallback.
func mcpEmbedTheaterURLs(ctx context.Context, worldID, channelID, formID string) (string, []string, string) {
	query := url.Values{"world": {worldID}, "channel": {channelID}}
	internalPath := "#/internal/iform/" + url.PathEscape(formID) + "?" + query.Encode()
	cfg := mcpConfigSnapshot()
	scheme := "https"
	requestOrigin := ""
	if resource, _ := ctx.Value(mcpResourceContextKey{}).(string); resource != "" {
		if u, err := url.Parse(resource); err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") {
			scheme = u.Scheme
			requestOrigin = strings.ToLower(u.Scheme + "://" + u.Host)
		}
	}
	origins := mcpPublicOrigins(cfg, scheme)
	if len(origins) == 0 && requestOrigin != "" {
		origins = []string{requestOrigin}
	}
	base := "/" + strings.Trim(cfg.WebUrl, "/")
	if base != "/" {
		base += "/"
	}
	urls := make([]string, 0, len(origins))
	for _, origin := range origins {
		urls = append(urls, origin+base+internalPath)
	}
	if len(urls) == 0 {
		return "", nil, internalPath
	}
	return urls[0], urls[1:], internalPath
}

func mcpEmbedSummary(ctx context.Context, worldID, channelID string, item *service.ChannelIFormView) mcpEmbedSummaryDTO {
	form := item.ChannelIFormModel
	source := strings.TrimSpace(item.SourceChannelID)
	if source == "" {
		source = form.ChannelID
	}
	primary, alternates, internalPath := mcpEmbedTheaterURLs(ctx, worldID, channelID, form.ID)
	return mcpEmbedSummaryDTO{
		ID: form.ID, Name: form.Name, WorldID: worldID, ChannelID: channelID, SourceChannelID: source,
		URL: form.Url, HasEmbedCode: form.EmbedCode != "", EmbedCodeBytes: len(form.EmbedCode),
		SourceEditable: strings.TrimSpace(form.TemplateRef) == "",
		DefaultWidth:   form.DefaultWidth, DefaultHeight: form.DefaultHeight, BridgePolicy: form.BridgePolicy,
		TemplateRef: form.TemplateRef, TemplateOrigin: item.TemplateOrigin, TemplateName: item.TemplateName,
		TemplateMissing: item.TemplateMissing, TemplateArchived: item.TemplateArchived,
		WorldShared: item.WorldShared, SharedRef: item.SharedRef, UpdatedAt: form.UpdatedAt.UnixMilli(),
		Theater: mcpEmbedTheaterRef{WorldID: worldID, ChannelID: channelID, FormID: form.ID, URL: primary, AlternateURLs: alternates, InternalPath: internalPath},
	}
}

func mcpEmbedDetail(ctx context.Context, worldID, channelID string, item *service.ChannelIFormView) mcpEmbedDetailDTO {
	form := item.ChannelIFormModel
	dto := mcpEmbedDetailDTO{
		mcpEmbedSummaryDTO: mcpEmbedSummary(ctx, worldID, channelID, item),
		EmbedCode:          form.EmbedCode, DefaultCollapsed: form.DefaultCollapsed, DefaultFloating: form.DefaultFloating,
		AllowPopout: form.AllowPopout, OrderIndex: form.OrderIndex, MediaOptions: form.MediaOptions,
		SharedWorldID: item.SharedWorldID, CreatedBy: form.CreatedBy, UpdatedBy: form.UpdatedBy, CreatedAt: form.CreatedAt.UnixMilli(),
	}
	if strings.TrimSpace(form.TemplateRef) != "" {
		overrides := form.TemplateOverrides
		dto.TemplateOverrides = &overrides
	}
	return dto
}

// mcpEmbedRequestBody rebuilds the REST request body: only submitted fields are
// present, which drives the native field-presence and partial-update rules.
func mcpEmbedRequestBody(in mcpEmbedSaveInput) ([]byte, error) {
	body := map[string]any{}
	set := func(key string, present bool, value any) {
		if present {
			body[key] = value
		}
	}
	set("name", in.Name != nil, in.Name)
	set("url", in.URL != nil, in.URL)
	set("embedCode", in.EmbedCode != nil, in.EmbedCode)
	set("defaultWidth", in.DefaultWidth != nil, in.DefaultWidth)
	set("defaultHeight", in.DefaultHeight != nil, in.DefaultHeight)
	set("defaultCollapsed", in.DefaultCollapsed != nil, in.DefaultCollapsed)
	set("defaultFloating", in.DefaultFloating != nil, in.DefaultFloating)
	set("allowPopout", in.AllowPopout != nil, in.AllowPopout)
	set("orderIndex", in.OrderIndex != nil, in.OrderIndex)
	set("mediaOptions", in.MediaOptions != nil, in.MediaOptions)
	set("bridgePolicy", in.BridgePolicy != nil, in.BridgePolicy)
	set("templateRef", in.TemplateRef != nil, in.TemplateRef)
	set("templateOverrides", in.TemplateOverrides != nil, in.TemplateOverrides)
	return json.Marshal(body)
}

func mcpEmbedMutationError(err error) error {
	var mutationErr *iformMutationError
	if !errors.As(err, &mutationErr) {
		return err
	}
	switch mutationErr.Status {
	case fiber.StatusBadRequest:
		return mcpFailure("invalid_argument", mutationErr.Message)
	case fiber.StatusForbidden:
		return mcpFailure("forbidden", mutationErr.Message)
	case fiber.StatusNotFound:
		return mcpFailure("not_found", mutationErr.Message)
	default:
		return mcpFailure("operation_failed", mutationErr.Message)
	}
}

func mcpEmbedWriteResult(ctx context.Context, a *service.MCPActor, ch *model.ChannelModel, formID string, changed bool) (any, error) {
	item, err := mcpEmbedFind(ch.ID, formID)
	if err != nil {
		return nil, err
	}
	if a.Allows("embed:read") {
		return struct {
			mcpItem[mcpEmbedDetailDTO]
			Changed bool `json:"changed"`
		}{mcpDetail(mcpEmbedDetail(ctx, ch.WorldID, ch.ID, item)), changed}, nil
	}
	summary := mcpEmbedSummary(ctx, ch.WorldID, ch.ID, item)
	return mcpDetail(mcpEmbedWriteRefDTO{ID: formID, WorldID: ch.WorldID, ChannelID: ch.ID, Changed: changed, Theater: summary.Theater}), nil
}

func mcpEmbedTools() []mcpToolSpec {
	return []mcpToolSpec{
		mcpSpec("embed_read", "无 resourceId 分页列出当前频道有效的频道嵌入（含世界共享引用），轻量摘要不含 embedCode；有 resourceId 读取详情（源码、URL、尺寸、媒体、BridgePolicy、模板与共享来源）。每项 theater 字段给出可直接用于 Theater iframe/surface_embed_set 的内部 iForm URL；使用时 Theater 的 inputChannelId 必须为该 channelId。", []string{"embed:read"}, false, false, true, func(ctx context.Context, a *service.MCPActor, in mcpEmbedReadInput) (any, error) {
			ch, err := mcpEmbedChannel(a, in.WorldID, in.ChannelID)
			if err != nil {
				return nil, err
			}
			if in.ResourceID != "" {
				item, err := mcpEmbedFind(ch.ID, in.ResourceID)
				if err != nil {
					return nil, err
				}
				return mcpDetail(mcpEmbedDetail(ctx, ch.WorldID, ch.ID, item)), nil
			}
			forms, err := service.ListEffectiveChannelIForms(ch.ID)
			if err != nil {
				return nil, err
			}
			search := strings.ToLower(strings.TrimSpace(in.Search))
			items := []mcpEmbedSummaryDTO{}
			for _, item := range forms {
				if item == nil || item.ChannelIFormModel == nil || !catalogSearchMatch(search, item.Name, item.TemplateName) {
					continue
				}
				items = append(items, mcpEmbedSummary(ctx, ch.WorldID, ch.ID, item))
			}
			return mcpSlicePage(items, in.mcpPageInput)
		}),
		mcpSpec("embed_catalog", "频道嵌入目录：可安装的 builtin/platform 模板（只读，templateRef 用于 embed_save 引用安装）、Channel Embed capabilities、当前 embedCode 大小限制、瞬时事件限制与 Theater 事件绑定约束。不提供平台模板管理。", []string{"embed:read"}, false, false, true, func(_ context.Context, _ *service.MCPActor, in mcpEmbedCatalogInput) (any, error) {
			origin := strings.TrimSpace(in.Origin)
			if origin != "" && origin != "builtin" && origin != "platform" {
				return nil, mcpFailure("invalid_argument", "origin 仅允许 builtin/platform")
			}
			// MCP always uses the public catalog view, even for platform admins.
			templates, err := listChannelIFormTemplateCatalog(strings.ToLower(strings.TrimSpace(in.Search)), origin, false)
			if err != nil {
				return nil, mcpEmbedMutationError(err)
			}
			page, err := mcpSlicePage(templates, in.mcpPageInput)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"templates":    page,
				"capabilities": mcpEmbedCapabilities,
				"limits": map[string]any{
					"embedCodeMaxKB": channelEmbedMaxCodeSizeKB(), "embedCodeMaxBytes": channelEmbedMaxCodeSizeKB() * 1024,
					"nameMaxChars": 64, "defaultSizeMax": map[string]int{"width": 1920, "height": 1440},
					"urlSchemes": []string{"http", "https"},
				},
				"events": map[string]any{
					"topicPattern": service.ChannelEmbedTopicPattern.String(), "payloadMaxBytes": embedEventPayloadMax,
					"publishPerUserPerSecond": 60, "persistent": false,
				},
				"bridgePolicy": map[string]any{
					"shape":   map[string]any{"enabled": "bool", "allowedOrigins": "[]string（srcdoc 嵌入的 origin 为 null，通常留空）", "capabilities": "[]string"},
					"default": "省略 bridgePolicy 时 Embed API 关闭；需要 SealChatEmbed 时显式启用并只授予必要 capability",
				},
				"theater": map[string]any{
					"reference":            "embed_read/embed_save 返回的 theater.url 可直接用于 theater_object content.iframe.url 或 theater_scene surface_embed_set url；Theater scope 的 inputChannelId（频道 Theater 还有 channelId）必须等于 theater.channelId",
					"embedEventBindings":   "仅普通 iframe 对象：theater_object fields.embedEventBindings=[{topic, actionIds}] 把精确 topic 绑定到该对象已保存 actions；surfaceEmbeds 不支持事件绑定",
					"maxBindings":          service.TheaterMaxEmbedEventBindings,
					"maxActionsPerBinding": service.TheaterMaxEmbedEventBindingActions,
				},
				"contentTrust": "untrusted_user_generated",
			}, nil
		}),
		mcpSpec("embed_save", "无 resourceId 创建频道嵌入（url 或 embedCode 二选一，或 templateRef 引用模板）；有 resourceId 仅更新提交字段。沿用原生 iForm 规则：频道 iForm 管理权限、URL 仅 http/https、embedCode 平台大小限制、模板引用不能改 url/embedCode/templateRef（用 templateOverrides，键为 null 恢复模板默认）、BridgePolicy 归一化、世界共享源更新与实时广播。theater:write 不包含此能力。", []string{"embed:write"}, true, true, false, func(ctx context.Context, a *service.MCPActor, in mcpEmbedSaveInput) (any, error) {
			ch, err := mcpEmbedChannel(a, in.WorldID, in.ChannelID)
			if err != nil {
				return nil, err
			}
			if !canManageIForm(a.User.ID, ch.ID) {
				return nil, mcpFailure("forbidden", "没有权限管理 iForm 控件")
			}
			body, err := mcpEmbedRequestBody(in)
			if err != nil {
				return nil, err
			}
			if in.ResourceID == "" {
				var payload channelIFormCreateRequest
				if err := json.Unmarshal(body, &payload); err != nil {
					return nil, mcpFailure("invalid_argument", "请求体解析失败")
				}
				form, err := createChannelIForm(a.User, ch.ID, &payload, body)
				if err != nil {
					return nil, mcpEmbedMutationError(err)
				}
				return mcpEmbedWriteResult(ctx, a, ch, form.ID, true)
			}
			form, sourceChannelID, err := resolveEffectiveIFormForMutation(ch.ID, in.ResourceID)
			if err != nil {
				return nil, err
			}
			if form == nil {
				return nil, mcpFailure("not_found", "控件不存在")
			}
			var payload channelIFormUpdateRequest
			if err := json.Unmarshal(body, &payload); err != nil {
				return nil, mcpFailure("invalid_argument", "请求体解析失败")
			}
			_, changed, err := updateChannelIForm(a.User, form, sourceChannelID, &payload, body)
			if err != nil {
				return nil, mcpEmbedMutationError(err)
			}
			return mcpEmbedWriteResult(ctx, a, ch, in.ResourceID, changed)
		}),
		mcpSpec("embed_delete", "永久删除当前频道拥有的频道嵌入及其 Storage，并移除世界共享绑定；世界共享引用需在源频道删除。引用它的 Theater iframe 将显示不可用。", []string{"embed:write"}, true, true, false, func(_ context.Context, a *service.MCPActor, in mcpResourceInput) (any, error) {
			ch, err := mcpEmbedChannel(a, in.WorldID, in.ChannelID)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(in.ResourceID) == "" {
				return nil, mcpFailure("invalid_argument", "resourceId 必填")
			}
			if !canManageIForm(a.User.ID, ch.ID) {
				return nil, mcpFailure("forbidden", "没有权限管理 iForm 控件")
			}
			if err := deleteChannelIForm(a.User, ch.ID, strings.TrimSpace(in.ResourceID)); err != nil {
				return nil, mcpEmbedMutationError(err)
			}
			return mcpOK{true}, nil
		}),
	}
}
