package api

import (
	"context"
	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
)

type mcpClueListInput struct {
	mcpWorldInput
	mcpPageInput
	Keyword string `json:"keyword,omitempty"`
}
type mcpClueWriteInput struct {
	mcpWorldInput
	ResourceID        string  `json:"resourceId,omitempty"`
	ExpectedRevision  *int64  `json:"expectedRevision,omitempty"`
	Title             *string `json:"title,omitempty"`
	Kind              *string `json:"kind,omitempty"`
	ContentFormat     *string `json:"contentFormat,omitempty"`
	Content           *string `json:"content,omitempty"`
	ImageAttachmentID *string `json:"imageAttachmentId,omitempty"`
	ImageURL          *string `json:"imageUrl,omitempty"`
	EmbedURL          *string `json:"embedUrl,omitempty"`
	ManagerNoteFormat *string `json:"managerNoteFormat,omitempty"`
	ManagerNote       *string `json:"managerNote,omitempty"`
}
type mcpCluePublishInput struct {
	mcpWorldInput
	ResourceID         string   `json:"resourceId"`
	ExpectedPublishSeq *int64   `json:"expectedPublishSeq"`
	UserIDs            []string `json:"userIds,omitempty"`
}

func mcpValue[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

func mcpClueWriteDetail(a *service.MCPActor, v *protocol.WorldClueDetail) any {
	return mcpWriteDetail(a, "clue:read", v, mcpWriteRef{ID: v.ID, WorldID: v.WorldID, Revision: v.Revision, PublishSeq: &v.PublishSeq})
}
func mcpClueTools() []mcpToolSpec {
	ret := []mcpToolSpec{
		mcpSpec("clue_list", "分页读取当前用户可见线索摘要。", []string{"clue:read"}, false, false, true, func(_ context.Context, a *service.MCPActor, in mcpClueListInput) (any, error) {
			if _, _, err := in.bounds(); err != nil {
				return nil, err
			}
			items, err := service.WorldClueList(in.WorldID, a.User.ID, in.Keyword)
			if err != nil {
				return nil, err
			}
			return mcpSlicePage(items, in.mcpPageInput)
		}),
		mcpSpec("clue_get", "按原生权限读取普通或管理详情，私人内容仅属于当前用户。", []string{"clue:read"}, false, false, true, func(_ context.Context, a *service.MCPActor, in mcpResourceInput) (any, error) {
			if in.ChannelID != "" {
				return nil, mcpFailure("invalid_argument", "线索按世界共享")
			}
			if service.IsWorldAdmin(in.WorldID, a.User.ID) {
				v, err := service.WorldClueGetAdmin(in.WorldID, in.ResourceID, a.User.ID)
				return mcpDetail(v), err
			}
			v, err := service.WorldClueGet(in.WorldID, in.ResourceID, a.User.ID)
			return mcpDetail(v), err
		}),
		mcpSpec("clue_create", "创建未发布的线索；不会隐式发布。", []string{"clue:write"}, true, false, false, func(_ context.Context, a *service.MCPActor, in mcpClueWriteInput) (any, error) {
			if in.ResourceID != "" || in.ExpectedRevision != nil {
				return nil, mcpFailure("invalid_argument", "创建不接受 resourceId/expectedRevision")
			}
			v, err := service.WorldClueCreate(in.WorldID, a.User.ID, service.WorldClueCreateInput{Title: mcpValue(in.Title), Kind: mcpValue(in.Kind), ContentFormat: mcpValue(in.ContentFormat), Content: mcpValue(in.Content), ImageAttachmentID: mcpValue(in.ImageAttachmentID), ImageURL: mcpValue(in.ImageURL), EmbedURL: mcpValue(in.EmbedURL), ManagerNoteFormat: mcpValue(in.ManagerNoteFormat), ManagerNote: mcpValue(in.ManagerNote)})
			if err != nil {
				return nil, err
			}
			broadcastWorldClueChanged(in.WorldID, v.ID, "upsert", v.Revision)
			return mcpWriteDetail(a, "clue:read", v, mcpWriteRef{ID: v.ID, WorldID: v.WorldID, Revision: v.Revision, PublishSeq: &v.PublishSeq}), nil
		}),
		mcpSpec("clue_update", "使用真实 expectedRevision 更新内容，尊重正在编辑的字段。", []string{"clue:write"}, true, true, false, func(_ context.Context, a *service.MCPActor, in mcpClueWriteInput) (any, error) {
			if in.ExpectedRevision == nil || *in.ExpectedRevision <= 0 {
				return nil, mcpFailure("invalid_argument", "expectedRevision 必填")
			}
			fields := []string{}
			if in.Kind != nil {
				fields = append(fields, "*")
			}
			if in.Title != nil {
				fields = append(fields, "title")
			}
			if in.Content != nil || in.ContentFormat != nil {
				fields = append(fields, "content")
			}
			if in.ImageURL != nil || in.ImageAttachmentID != nil || in.Kind != nil {
				fields = append(fields, "imageUrl")
			}
			if in.EmbedURL != nil {
				fields = append(fields, "embedUrl")
			}
			if in.ManagerNote != nil || in.ManagerNoteFormat != nil {
				fields = append(fields, "managerNote")
			}
			if err := service.MCPWorldClueCheckLocks(in.WorldID, in.ResourceID, a.User.ID, fields...); err != nil {
				return nil, err
			}
			v, err := service.WorldClueUpdate(in.WorldID, in.ResourceID, a.User.ID, service.WorldClueUpdateInput{ExpectedRevision: *in.ExpectedRevision, Title: in.Title, Kind: in.Kind, ContentFormat: in.ContentFormat, Content: in.Content, ImageAttachmentID: in.ImageAttachmentID, ImageURL: in.ImageURL, EmbedURL: in.EmbedURL, ManagerNoteFormat: in.ManagerNoteFormat, ManagerNote: in.ManagerNote})
			if err != nil {
				return nil, err
			}
			broadcastWorldClueChanged(in.WorldID, v.ID, "upsert", v.Revision)
			return mcpClueWriteDetail(a, v), nil
		}),
		mcpSpec("clue_delete", "使用 expectedRevision 删除线索并广播移除。", []string{"clue:write"}, true, true, false, func(_ context.Context, a *service.MCPActor, in struct {
			mcpWorldInput
			ResourceID       string `json:"resourceId"`
			ExpectedRevision int64  `json:"expectedRevision"`
		}) (any, error) {
			if err := service.MCPWorldClueCheckLocks(in.WorldID, in.ResourceID, a.User.ID, "*"); err != nil {
				return nil, err
			}
			err := service.WorldClueDelete(in.WorldID, in.ResourceID, a.User.ID, in.ExpectedRevision)
			if err == nil {
				broadcastWorldClueChanged(in.WorldID, in.ResourceID, "remove", 0)
			}
			return mcpOK{err == nil}, err
		}),
	}
	for _, operation := range []string{"publish", "reveal", "unpublish"} {
		op := operation
		ret = append(ret, mcpSpec("clue_"+op, "改变线索发布状态，影响其他用户；需要额外 publish 授权和真实 expectedPublishSeq。", []string{"clue:write", "clue:publish"}, true, true, false, func(_ context.Context, a *service.MCPActor, in mcpCluePublishInput) (any, error) {
			if in.ExpectedPublishSeq == nil || *in.ExpectedPublishSeq < 0 {
				return nil, mcpFailure("invalid_argument", "expectedPublishSeq 必填")
			}
			if len(in.UserIDs) > 200 {
				return nil, mcpFailure("invalid_argument", "接收者数量过多")
			}
			if op == "unpublish" {
				if len(in.UserIDs) > 0 {
					return nil, mcpFailure("invalid_argument", "unpublish 不接受 userIds")
				}
				v, err := service.WorldClueUnpublish(in.WorldID, in.ResourceID, a.User.ID, *in.ExpectedPublishSeq)
				if err != nil {
					return nil, err
				}
				broadcastWorldClueChanged(in.WorldID, v.ID, "upsert", v.Revision)
				return mcpClueWriteDetail(a, v), nil
			}
			var v *service.WorldCluePublishResult
			var err error
			if op == "publish" {
				if len(in.UserIDs) > 0 {
					return nil, mcpFailure("invalid_argument", "publish 不接受 userIds")
				}
				v, err = service.WorldCluePublish(in.WorldID, in.ResourceID, a.User.ID, *in.ExpectedPublishSeq)
			} else {
				v, err = service.WorldClueReveal(in.WorldID, in.ResourceID, a.User.ID, in.UserIDs, *in.ExpectedPublishSeq)
			}
			if err != nil {
				return nil, err
			}
			broadcastWorldClueChanged(in.WorldID, in.ResourceID, "upsert", v.Clue.Revision)
			broadcastWorldCluePublished(in.WorldID, in.ResourceID, v.Clue.PublishSeq, v.RecipientIDs)
			return mcpClueWriteDetail(a, v.Clue), nil
		}))
	}
	return ret
}

type mcpGlossaryListInput struct {
	mcpWorldInput
	mcpPageInput
	Query    string `json:"query,omitempty"`
	Category string `json:"category,omitempty"`
}
type mcpGlossaryWriteInput struct {
	mcpWorldInput
	ResourceID        string    `json:"resourceId,omitempty"`
	Keyword           *string   `json:"keyword,omitempty"`
	Category          *string   `json:"category,omitempty"`
	Aliases           *[]string `json:"aliases,omitempty"`
	MatchMode         *string   `json:"matchMode,omitempty"`
	Description       *string   `json:"description,omitempty"`
	DescriptionFormat *string   `json:"descriptionFormat,omitempty"`
	Display           *string   `json:"display,omitempty"`
	SortOrder         *int      `json:"sortOrder,omitempty"`
	Enabled           *bool     `json:"isEnabled,omitempty"`
}

func mcpKeywordDTO(v *model.WorldKeywordModel) service.EffectiveWorldKeywordItem {
	if v == nil {
		return service.EffectiveWorldKeywordItem{}
	}
	return service.EffectiveWorldKeywordItem{ID: v.ID, WorldID: v.WorldID, Keyword: v.Keyword, Category: v.Category, Aliases: append([]string(nil), v.Aliases...), MatchMode: string(v.MatchMode), Description: v.Description, DescriptionFormat: string(v.DescriptionFormat), Display: string(v.Display), SortOrder: v.SortOrder, IsEnabled: v.IsEnabled, SourceType: "world", SourceID: v.WorldID, SourceName: "当前世界", CanQuickEdit: true}
}
func mcpGlossaryTools() []mcpToolSpec {
	ret := []mcpToolSpec{
		mcpSpec("glossary_list", "读取有效世界术语，标明自建和外挂来源；外挂只读。", []string{"glossary:read"}, false, false, true, func(_ context.Context, a *service.MCPActor, in mcpGlossaryListInput) (any, error) {
			if _, _, err := in.bounds(); err != nil {
				return nil, err
			}
			items, err := service.EffectiveWorldKeywordList(in.WorldID, a.User.ID, service.EffectiveWorldKeywordListOptions{Query: in.Query, Category: in.Category})
			if err != nil {
				return nil, err
			}
			canWrite := a.Allows("glossary:write") && service.MCPWorldKeywordCanWrite(in.WorldID, a.User.ID)
			for _, item := range items {
				item.CanQuickEdit = item.CanQuickEdit && canWrite
			}
			page, err := mcpSlicePage(items, in.mcpPageInput)
			if err != nil {
				return nil, err
			}
			categories, err := service.WorldKeywordListCategoryInfos(in.WorldID, a.User.ID)
			if err != nil {
				return nil, err
			}
			return struct {
				mcpList[*service.EffectiveWorldKeywordItem]
				Categories []service.KeywordCategoryInfo `json:"categories"`
			}{page, categories}, nil
		}),
		mcpSpec("glossary_get", "按有效术语的来源读取详情。", []string{"glossary:read"}, false, false, true, func(_ context.Context, a *service.MCPActor, in mcpResourceInput) (any, error) {
			if in.ChannelID != "" {
				return nil, mcpFailure("invalid_argument", "术语按世界共享，不接受 channelId")
			}
			items, err := service.EffectiveWorldKeywordList(in.WorldID, a.User.ID, service.EffectiveWorldKeywordListOptions{IncludeAllMatches: true})
			if err != nil {
				return nil, err
			}
			for _, v := range items {
				if v.ID == in.ResourceID {
					v.CanQuickEdit = v.CanQuickEdit && a.Allows("glossary:write") && service.MCPWorldKeywordCanWrite(in.WorldID, a.User.ID)
					return mcpDetail(v), nil
				}
			}
			return nil, service.ErrWorldKeywordNotFound
		}),
		mcpSpec("glossary_delete", "删除世界自建术语，保留成员编辑开关规则。", []string{"glossary:write"}, true, true, false, func(_ context.Context, a *service.MCPActor, in mcpResourceInput) (any, error) {
			if in.ChannelID != "" {
				return nil, mcpFailure("invalid_argument", "术语按世界共享，不接受 channelId")
			}
			err := service.WorldKeywordDelete(in.WorldID, in.ResourceID, a.User.ID)
			if err == nil {
				broadcastWorldKeywordEvent(&worldKeywordEventPayload{WorldID: in.WorldID, Operation: "delete", DeletedIDs: []string{in.ResourceID}})
			}
			return mcpOK{err == nil}, err
		}),
	}
	for _, op := range []string{"create", "update"} {
		operation := op
		ret = append(ret, mcpSpec("glossary_"+op, "编辑正常术语字段；仅世界自建来源可写，遵循 AllowMemberEditKeywords。", []string{"glossary:write"}, true, op == "update", false, func(_ context.Context, a *service.MCPActor, in mcpGlossaryWriteInput) (any, error) {
			if operation == "create" && in.ResourceID != "" {
				return nil, mcpFailure("invalid_argument", "创建不接受 resourceId")
			}
			input := service.WorldKeywordInput{}
			if operation == "update" {
				old, err := service.MCPWorldKeywordGet(in.WorldID, in.ResourceID, a.User.ID)
				if err != nil {
					return nil, err
				}
				input = service.WorldKeywordInput{Keyword: old.Keyword, Category: old.Category, Aliases: append([]string(nil), old.Aliases...), MatchMode: string(old.MatchMode), Description: old.Description, DescriptionFormat: string(old.DescriptionFormat), Display: string(old.Display)}
			}
			if in.Keyword != nil {
				input.Keyword = *in.Keyword
			}
			if in.Category != nil {
				input.Category = *in.Category
			}
			if in.Aliases != nil {
				input.Aliases = *in.Aliases
			}
			if in.MatchMode != nil {
				input.MatchMode = *in.MatchMode
			}
			if in.Description != nil {
				input.Description = *in.Description
			}
			if in.DescriptionFormat != nil {
				input.DescriptionFormat = *in.DescriptionFormat
			}
			if in.Display != nil {
				input.Display = *in.Display
			}
			input.SortOrder = in.SortOrder
			input.Enabled = in.Enabled
			var v *model.WorldKeywordModel
			var err error
			if operation == "create" {
				v, err = service.WorldKeywordCreate(in.WorldID, a.User.ID, input)
			} else {
				v, err = service.WorldKeywordUpdate(in.WorldID, in.ResourceID, a.User.ID, input)
			}
			if err != nil {
				return nil, err
			}
			broadcastWorldKeywordEvent(&worldKeywordEventPayload{WorldID: in.WorldID, Operation: operation, Keywords: []*model.WorldKeywordModel{v}})
			return mcpWriteDetail(a, "glossary:read", mcpKeywordDTO(v), mcpWriteRef{ID: v.ID, WorldID: v.WorldID}), nil
		}))
	}
	return ret
}
