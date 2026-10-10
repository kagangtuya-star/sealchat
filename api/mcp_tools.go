package api

import (
	"context"
	"gorm.io/gorm"
	"sealchat/model"
	"sealchat/service"
	"sealchat/utils"
)

type mcpEmpty struct{}
type mcpPageInput struct {
	Page  int `json:"page,omitempty"`
	Limit int `json:"limit,omitempty"`
}

func (p mcpPageInput) bounds() (int, int, error) {
	if p.Page == 0 {
		p.Page = 1
	}
	if p.Limit == 0 {
		p.Limit = 50
	}
	if p.Page < 1 || p.Page > 100000 || p.Limit < 1 || p.Limit > 200 {
		return 0, 0, mcpFailure("invalid_argument", "page 或 limit 无效；limit 最大 200")
	}
	return p.Page, p.Limit, nil
}

type mcpList[T any] struct {
	Items        []T    `json:"items"`
	Page         int    `json:"page"`
	Limit        int    `json:"limit"`
	HasMore      bool   `json:"hasMore"`
	Total        int64  `json:"total"`
	ContentTrust string `json:"contentTrust"`
}

func mcpPaged[T any](items []T, page, limit int, total int64) mcpList[T] {
	return mcpList[T]{items, page, limit, int64(page*limit) < total, total, "untrusted_user_generated"}
}
func mcpSlicePage[T any](items []T, p mcpPageInput) (mcpList[T], error) {
	page, limit, err := p.bounds()
	if err != nil {
		return mcpList[T]{}, err
	}
	total := len(items)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	return mcpPaged(items[start:end], page, limit, int64(total)), nil
}

type mcpItem[T any] struct {
	Item         T      `json:"item"`
	ContentTrust string `json:"contentTrust"`
}

func mcpDetail[T any](v T) mcpItem[T] { return mcpItem[T]{v, "untrusted_user_generated"} }

type mcpWriteRef struct {
	ID         string `json:"id,omitempty"`
	WorldID    string `json:"worldId,omitempty"`
	ChannelID  string `json:"channelId,omitempty"`
	Revision   int64  `json:"revision,omitempty"`
	PublishSeq *int64 `json:"publishSeq,omitempty"`
	ScopeType  string `json:"scopeType,omitempty"`
	ScopeID    string `json:"scopeId,omitempty"`
}

func mcpWriteDetail(a *service.MCPActor, readScope string, value any, ref mcpWriteRef) any {
	if a.Allows(readScope) {
		return mcpDetail(value)
	}
	return mcpDetail(ref)
}

type mcpOK struct {
	Success bool `json:"success"`
}
type mcpWorldInput struct {
	WorldID string `json:"worldId"`
}
type mcpChannelInput struct {
	WorldID   string `json:"worldId"`
	ChannelID string `json:"channelId"`
}
type mcpResourceInput struct {
	WorldID    string `json:"worldId"`
	ChannelID  string `json:"channelId,omitempty"`
	ResourceID string `json:"resourceId"`
}

func mcpChannel(a *service.MCPActor, worldID, channelID string) (*model.ChannelModel, error) {
	if worldID == "" || channelID == "" {
		return nil, mcpFailure("invalid_argument", "worldId 和 channelId 必填")
	}
	ch, err := model.ChannelGet(channelID)
	if err != nil {
		return nil, err
	}
	if ch == nil || ch.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	if ch.WorldID != worldID {
		return nil, mcpFailure("invalid_argument", "频道与世界不匹配")
	}
	if !service.CanReadChannelByUserId(a.User.ID, ch.ID) {
		return nil, service.ErrWorldPermission
	}
	return ch, nil
}
func mcpWorld(a *service.MCPActor, worldID string) error {
	if worldID == "" {
		return mcpFailure("invalid_argument", "worldId 必填")
	}
	if !service.IsWorldMember(worldID, a.User.ID) {
		return service.ErrWorldPermission
	}
	return nil
}

type mcpWorldDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
	Joined      bool   `json:"joined"`
}
type mcpChannelDTO struct {
	ID       string `json:"id"`
	WorldID  string `json:"worldId"`
	Name     string `json:"name"`
	ParentID string `json:"parentId"`
}
type mcpWorldListInput struct {
	mcpPageInput
	IncludePublic   bool   `json:"includePublic,omitempty"`
	IncludeArchived bool   `json:"includeArchived,omitempty"`
	Keyword         string `json:"keyword,omitempty"`
}
type mcpChannelListInput struct {
	mcpWorldInput
	mcpPageInput
}
type mcpMeDTO struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Nick  string `json:"nick"`
	KeyID string `json:"keyId"`
}
type mcpCapabilitiesDTO struct {
	Scopes []string        `json:"scopes"`
	Tools  []string        `json:"tools"`
	Limits utils.MCPConfig `json:"limits"`
}

func mcpBasicTools() []mcpToolSpec {
	return []mcpToolSpec{
		mcpSpec("sealchat_me", "认证用户与 MCP 凭证的公开标识。", nil, false, false, true, func(_ context.Context, a *service.MCPActor, _ mcpEmpty) (any, error) {
			return mcpMeDTO{a.User.ID, a.User.Username, a.User.Nickname, a.CredentialID}, nil
		}),
		mcpSpec("sealchat_capabilities", "当前 Key 与平台能力的交集上限；实际操作仍实时检查目标资源权限。", nil, false, false, true, func(_ context.Context, a *service.MCPActor, _ mcpEmpty) (any, error) {
			tools := []string{}
			for _, s := range mcpToolRegistry() {
				if a.Allows(s.scopes...) {
					tools = append(tools, s.tool.Name)
				}
			}
			return mcpCapabilitiesDTO{a.Scopes, tools, mcpConfigSnapshot().MCP}, nil
		}),
		mcpSpec("world_list", "默认列出已加入且未归档的世界；公开元数据不授予内容权限。", nil, false, false, true, func(_ context.Context, a *service.MCPActor, in mcpWorldListInput) (any, error) {
			page, limit, err := in.bounds()
			if err != nil {
				return nil, err
			}
			if limit > 50 {
				limit = 50
			}
			q := service.UserWorldListQuery(a.User.ID, service.UserWorldListOptions{JoinedOnly: !in.IncludePublic, IncludeArchived: in.IncludeArchived, Keyword: in.Keyword})
			var total int64
			if err = q.Count(&total).Error; err != nil {
				return nil, err
			}
			var rows []mcpWorldDTO
			err = service.OrderUserWorldListQuery(q, a.User.ID).Order("worlds.id DESC").Select("worlds.id, worlds.name, worlds.description, worlds.visibility").Offset((page - 1) * limit).Limit(limit).Scan(&rows).Error
			if err != nil {
				return nil, err
			}
			for i := range rows {
				rows[i].Joined = service.IsWorldMember(rows[i].ID, a.User.ID)
			}
			return mcpPaged(rows, page, limit, total), nil
		}),
		mcpSpec("channel_list", "当前用户在已加入世界中实际可见的频道。", nil, false, false, true, func(_ context.Context, a *service.MCPActor, in mcpChannelListInput) (any, error) {
			if err := mcpWorld(a, in.WorldID); err != nil {
				return nil, err
			}
			rows, err := service.ChannelList(a.User.ID, in.WorldID)
			if err != nil {
				return nil, err
			}
			items := []mcpChannelDTO{}
			for _, ch := range rows {
				items = append(items, mcpChannelDTO{ch.ID, ch.WorldID, ch.Name, ch.ParentID})
			}
			return mcpSlicePage(items, in.mcpPageInput)
		}),
	}
}

type mcpChatInput struct {
	WorldID string `json:"worldId"`
	Mode    string `json:"mode,omitempty"`
	service.UserChatInput
}
type mcpCountDTO struct {
	ChannelID string `json:"channelId"`
	Count     int64  `json:"count"`
}

func mcpChatTools() []mcpToolSpec {
	return []mcpToolSpec{
		mcpSpec("chat_history", "分页读取用户可见消息（默认 mode=messages）；mode=count 统计可见消息。游标绑定用户、频道和筛选条件。", []string{"chat:read"}, false, false, true, func(_ context.Context, a *service.MCPActor, in mcpChatInput) (any, error) {
			if _, err := mcpChannel(a, in.WorldID, in.ChannelID); err != nil {
				return nil, err
			}
			if in.Mode == "count" {
				n, err := service.QueryUserChatCount(a.User.ID, in.UserChatInput)
				return mcpCountDTO{in.ChannelID, n}, err
			}
			if in.Mode != "" && in.Mode != "messages" {
				return nil, mcpFailure("invalid_argument", "mode 必须为 messages/count")
			}
			return service.QueryUserChatMessages(a.User.ID, in.UserChatInput)
		}),
	}
}

type mcpBattleListInput struct {
	mcpWorldInput
	mcpPageInput
	ResourceID string `json:"resourceId,omitempty"`
	ChannelID  string `json:"channelId,omitempty"`
}
type mcpBattleWriteInput struct {
	mcpWorldInput
	ChannelID  string `json:"channelId,omitempty"`
	ResourceID string `json:"resourceId,omitempty"`
	mcpBattleContentInput
}
type mcpBattleContentInput struct {
	Title              *string `json:"title,omitempty"`
	Content            *string `json:"content,omitempty"`
	PeriodStart        *int64  `json:"periodStart,omitempty"`
	PeriodEnd          *int64  `json:"periodEnd,omitempty"`
	ContextReportCount *int    `json:"contextReportCount,omitempty"`
}
type mcpBattleSummaryInput struct {
	mcpChannelInput
	Source             string   `json:"source,omitempty"`
	Title              string   `json:"title"`
	PeriodStart        int64    `json:"periodStart"`
	PeriodEnd          int64    `json:"periodEnd"`
	ContextReportCount int      `json:"contextReportCount,omitempty"`
	SourceChannelIDs   []string `json:"sourceChannelIds,omitempty"`
}

func mcpBattleResource(a *service.MCPActor, in mcpResourceInput) (*model.BattleReportModel, error) {
	r, err := service.GetBattleReport(in.ResourceID, a.User.ID)
	if err != nil {
		return nil, err
	}
	if r.WorldID != in.WorldID || (in.ChannelID != "" && r.ChannelID != in.ChannelID) {
		return nil, mcpFailure("invalid_argument", "战报归属不匹配")
	}
	return r, nil
}
func mcpBattleTools() []mcpToolSpec {
	return []mcpToolSpec{
		mcpSpec("battle_report_read", "无 resourceId 分页列出世界共享战报；有 resourceId 读取详情及生成状态。", []string{"battle_report:read"}, false, false, true, func(_ context.Context, a *service.MCPActor, in mcpBattleListInput) (any, error) {
			if in.ResourceID != "" {
				r, err := mcpBattleResource(a, mcpResourceInput{in.WorldID, in.ChannelID, in.ResourceID})
				if err != nil {
					return nil, err
				}
				return mcpDetail(mcpBattleDTO(r, true)), nil
			}
			if err := mcpWorld(a, in.WorldID); err != nil {
				return nil, err
			}
			page, limit, err := in.bounds()
			if err != nil {
				return nil, err
			}
			q, err := service.BattleReportListQuery(in.WorldID, a.User.ID)
			if err != nil {
				return nil, err
			}
			if in.ChannelID != "" {
				q = q.Where("channel_id = ?", in.ChannelID)
			}
			var total int64
			if err = q.Count(&total).Error; err != nil {
				return nil, err
			}
			rows := []*model.BattleReportModel{}
			if err = q.Order("id DESC").Offset((page - 1) * limit).Limit(limit).Find(&rows).Error; err != nil {
				return nil, err
			}
			items := []battleReportResponse{}
			for _, r := range rows {
				items = append(items, mcpBattleDTO(r, false))
			}
			return mcpPaged(items, page, limit, total), nil
		}),
		mcpSpec("battle_report_save", "无 resourceId 创建（需 channelId）；有 resourceId 仅更新提交字段，保留正文和时间范围。不会调用 AI。", []string{"battle_report:write"}, true, true, false, func(_ context.Context, a *service.MCPActor, in mcpBattleWriteInput) (any, error) {
			if in.ResourceID == "" {
				if _, err := mcpChannel(a, in.WorldID, in.ChannelID); err != nil {
					return nil, err
				}
				input := service.BattleReportInput{}
				mcpMergeBattleInput(&input, in.mcpBattleContentInput)
				r, err := service.CreateBattleReport(in.ChannelID, a.User.ID, input)
				if err != nil {
					return nil, err
				}
				return mcpWriteDetail(a, "battle_report:read", mcpBattleDTO(r, true), mcpWriteRef{ID: r.ID, WorldID: r.WorldID, ChannelID: r.ChannelID}), nil
			}
			r, err := mcpBattleResource(a, mcpResourceInput{in.WorldID, in.ChannelID, in.ResourceID})
			if err != nil {
				return nil, err
			}
			input := service.BattleReportInput{Title: r.Title, Content: r.Content, PeriodStart: r.PeriodStart, PeriodEnd: r.PeriodEnd, ContextReportCount: r.ContextReportCount}
			mcpMergeBattleInput(&input, in.mcpBattleContentInput)
			r, err = service.UpdateBattleReport(r.ID, a.User.ID, input)
			if err != nil {
				return nil, err
			}
			return mcpWriteDetail(a, "battle_report:read", mcpBattleDTO(r, true), mcpWriteRef{ID: r.ID, WorldID: r.WorldID, ChannelID: r.ChannelID}), nil
		}),
		mcpSpec("battle_report_delete", "删除世界共享战报并同步展示频道。", []string{"battle_report:write"}, true, true, false, func(_ context.Context, a *service.MCPActor, in mcpResourceInput) (any, error) {
			r, err := mcpBattleResource(a, in)
			if err != nil {
				return nil, err
			}
			err = service.DeleteBattleReport(r.ID, a.User.ID)
			return mcpOK{err == nil}, err
		}),
		mcpSpec("battle_report_context", "读取原生总结上下文供 Agent 自行总结；不创建战报，不调用 AI 或计费。", []string{"battle_report:read", "chat:read"}, false, false, true, func(_ context.Context, a *service.MCPActor, in mcpBattleSummaryInput) (any, error) {
			if err := mcpValidateSummary(a, in); err != nil {
				return nil, err
			}
			prompt, err := service.BuildBattleReportSummaryPrompt(in.ChannelID, a.User.ID, service.BattleReportSummaryPromptInput{Title: in.Title, PeriodStart: unixMilliToTime(in.PeriodStart), PeriodEnd: unixMilliToTime(in.PeriodEnd), ContextReportCount: in.ContextReportCount, SourceChannelIDs: in.SourceChannelIDs, AIConfig: mcpConfigSnapshot().AI})
			return struct {
				Input        string `json:"input"`
				ContentTrust string `json:"contentTrust"`
			}{prompt, "untrusted_user_generated"}, err
		}),
		mcpSpec("battle_report_generate", "调用原生 AI/配额/计费链路，创建世界可见战报。可能收费；返回报告 ID，之后通过 battle_report_read 查询。不得自动重试。", []string{"battle_report:write", "battle_report:generate", "chat:read"}, true, false, false, func(_ context.Context, a *service.MCPActor, in mcpBattleSummaryInput) (any, error) {
			if err := mcpValidateSummary(a, in); err != nil {
				return nil, err
			}
			cfg := mcpConfigSnapshot()
			source := in.Source
			if source == "" {
				source = "platform"
			}
			runner := aiRunnerFactory(func() *utils.AppConfig { c := mcpConfigSnapshot(); return &c })
			r, err := service.StartBattleReportSummary(in.ChannelID, a.User.ID, service.BattleReportSummaryInput{Title: in.Title, PeriodStart: unixMilliToTime(in.PeriodStart), PeriodEnd: unixMilliToTime(in.PeriodEnd), ContextReportCount: in.ContextReportCount, SourceChannelIDs: in.SourceChannelIDs, Source: source, AIConfig: cfg.AI, Runner: runner})
			if err != nil {
				return nil, err
			}
			return mcpDetail(mcpBattleDTO(r, false)), nil
		}),
	}
}
func mcpBattleDTO(r *model.BattleReportModel, content bool) battleReportResponse {
	dto := battleReportToResponse(r, content)
	if dto.ErrorMessage != "" {
		dto.ErrorMessage = "生成过程中出现错误"
	}
	return dto
}
func mcpMergeBattleInput(out *service.BattleReportInput, in mcpBattleContentInput) {
	if in.Title != nil {
		out.Title = *in.Title
	}
	if in.Content != nil {
		out.Content = *in.Content
	}
	if in.PeriodStart != nil {
		out.PeriodStart = unixMilliToTime(*in.PeriodStart)
	}
	if in.PeriodEnd != nil {
		out.PeriodEnd = unixMilliToTime(*in.PeriodEnd)
	}
	if in.ContextReportCount != nil {
		out.ContextReportCount = *in.ContextReportCount
	}
}
func mcpValidateSummary(a *service.MCPActor, in mcpBattleSummaryInput) error {
	if in.Source != "" && in.Source != "platform" && in.Source != "user" {
		return mcpFailure("invalid_argument", "source 仅允许 platform/user")
	}
	if in.PeriodStart <= 0 || in.PeriodEnd <= in.PeriodStart || in.ContextReportCount < 0 || in.ContextReportCount > 20 || len(in.SourceChannelIDs) > 20 {
		return mcpFailure("invalid_argument", "时间范围、来源频道数量或上下文数量无效")
	}
	if _, err := mcpChannel(a, in.WorldID, in.ChannelID); err != nil {
		return err
	}
	for _, id := range in.SourceChannelIDs {
		if _, err := mcpChannel(a, in.WorldID, id); err != nil {
			return err
		}
	}
	return nil
}

func mcpToolRegistry() []mcpToolSpec {
	ret := mcpBasicTools()
	ret = append(ret, mcpSearchTools()...)
	ret = append(ret, mcpChatTools()...)
	ret = append(ret, mcpBattleTools()...)
	ret = append(ret, mcpClueTools()...)
	ret = append(ret, mcpGlossaryTools()...)
	ret = append(ret, mcpIdentityTools()...)
	ret = append(ret, mcpAudioTools()...)
	ret = append(ret, mcpNoteTools()...)
	ret = append(ret, mcpFileTools()...)
	ret = append(ret, mcpTheaterTools()...)
	ret = append(ret, mcpEmbedTools()...)
	return ret
}
