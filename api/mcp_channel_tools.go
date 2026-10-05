package api

import (
	"context"
	"math"
	"sealchat/model"
	"sealchat/pm"
	"sealchat/service"
	"time"
)

type mcpIdentityInput struct {
	mcpChannelInput
	TargetUserID string `json:"targetUserId,omitempty"`
	ResourceID   string `json:"resourceId,omitempty"`
}
type mcpIdentityListInput struct {
	mcpIdentityInput
	mcpPageInput
}
type mcpIdentityWriteInput struct {
	mcpIdentityInput
	DisplayName        *string `json:"displayName,omitempty"`
	Color              *string `json:"color,omitempty"`
	AvatarAttachmentID *string `json:"avatarAttachmentId,omitempty"`
}
type mcpIdentityDTO struct {
	ID                 string `json:"id"`
	ChannelID          string `json:"channelId"`
	UserID             string `json:"userId"`
	DisplayName        string `json:"displayName"`
	Color              string `json:"color"`
	AvatarAttachmentID string `json:"avatarAttachmentId"`
	IsDefault          bool   `json:"isDefault"`
	IsTemporary        bool   `json:"isTemporary"`
}

func mcpIdentityDTOFrom(v *model.ChannelIdentityModel) mcpIdentityDTO {
	return mcpIdentityDTO{v.ID, v.ChannelID, v.UserID, v.DisplayName, v.Color, v.AvatarAttachmentID, v.IsDefault, v.IsTemporary}
}
func mcpIdentityActor(a *service.MCPActor, in mcpIdentityInput) (*service.ChannelIdentityActorContext, error) {
	if _, err := mcpChannel(a, in.WorldID, in.ChannelID); err != nil {
		return nil, err
	}
	return service.ResolveChannelIdentityActor(in.ChannelID, a.User.ID, in.TargetUserID)
}
func mcpIdentityResource(actor *service.ChannelIdentityActorContext, channelID, identityID string) (*model.ChannelIdentityModel, error) {
	identity, err := service.ValidateChannelIdentityActorIdentity(actor, channelID, identityID)
	if err != nil {
		return nil, err
	}
	if identity.IsHidden {
		return nil, service.ErrChannelPermissionDenied
	}
	return identity, nil
}
func mcpIdentityTools() []mcpToolSpec {
	ret := []mcpToolSpec{
		mcpSpec("channel_identity_list", "读取可管理目标的现有频道角色，不创建 BOT 身份或修复资料。", []string{"identity:read"}, false, false, true, func(_ context.Context, a *service.MCPActor, in mcpIdentityListInput) (any, error) {
			if in.ResourceID != "" {
				return nil, mcpFailure("invalid_argument", "列表不接受 resourceId")
			}
			actor, err := mcpIdentityActor(a, in.mcpIdentityInput)
			if err != nil {
				return nil, err
			}
			page, limit, err := in.bounds()
			if err != nil {
				return nil, err
			}
			q := model.GetDB().Model(&model.ChannelIdentityModel{}).Where("channel_id = ? AND user_id = ? AND (is_hidden = ? OR is_hidden IS NULL)", in.ChannelID, actor.TargetUserID, false)
			var total int64
			if err = q.Count(&total).Error; err != nil {
				return nil, err
			}
			rows := []*model.ChannelIdentityModel{}
			if err = q.Order("sort_order ASC, created_at ASC, id ASC").Offset((page - 1) * limit).Limit(limit).Find(&rows).Error; err != nil {
				return nil, err
			}
			items := []mcpIdentityDTO{}
			for _, v := range rows {
				items = append(items, mcpIdentityDTOFrom(v))
			}
			return mcpPaged(items, page, limit, total), nil
		}),
		mcpSpec("channel_identity_get", "通过原生委托/共享角色规则读取指定 ChannelIdentity。", []string{"identity:read"}, false, false, true, func(_ context.Context, a *service.MCPActor, in mcpIdentityInput) (any, error) {
			actor, err := mcpIdentityActor(a, in)
			if err != nil {
				return nil, err
			}
			v, err := mcpIdentityResource(actor, in.ChannelID, in.ResourceID)
			if err != nil {
				return nil, err
			}
			return mcpDetail(mcpIdentityDTOFrom(v)), nil
		}),
		mcpSpec("channel_identity_manage_targets", "查询当前用户按原生等级和委托开关可管理的目标。", []string{"identity:read"}, false, false, true, func(_ context.Context, a *service.MCPActor, in struct {
			mcpChannelInput
			mcpPageInput
			Keyword string `json:"keyword,omitempty"`
		}) (any, error) {
			if _, err := mcpChannel(a, in.WorldID, in.ChannelID); err != nil {
				return nil, err
			}
			page, limit, err := in.bounds()
			if err != nil {
				return nil, err
			}
			v, err := service.ListChannelIdentityManageCandidates(service.ChannelIdentityManageCandidateQuery{ChannelID: in.ChannelID, ActorID: a.User.ID, Page: page, PageSize: limit, Keyword: in.Keyword})
			if err != nil {
				return nil, err
			}
			return mcpPaged(v.Items, page, limit, v.Total), nil
		}),
		mcpSpec("channel_identity_delete", "删除可管理的用户频道角色，保留共享角色删除与广播。", []string{"identity:write"}, true, true, false, func(_ context.Context, a *service.MCPActor, in mcpIdentityInput) (any, error) {
			actor, err := mcpIdentityActor(a, in)
			if err != nil {
				return nil, err
			}
			if actor.IsBotTarget {
				return nil, mcpFailure("forbidden", "BOT 默认角色不能删除")
			}
			v, err := mcpIdentityResource(actor, in.ChannelID, in.ResourceID)
			if err != nil {
				return nil, err
			}
			copies := []*model.ChannelIdentityModel{v}
			if v.SharedIdentityID != "" {
				copies, err = model.SharedChannelIdentityCopies(v.SharedIdentityID)
				if err != nil {
					return nil, err
				}
			}
			if err = service.ChannelIdentityDeleteWithAccess(actor.TargetUserID, a.User.ID, in.ChannelID, in.ResourceID); err != nil {
				return nil, err
			}
			for _, copy := range copies {
				broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{ChannelID: copy.ChannelID, TargetUserID: actor.TargetUserID, OperatorUserID: a.User.ID, Reason: "identity-delete"})
			}
			return mcpOK{true}, nil
		}),
	}
	for _, op := range []string{"create", "update"} {
		operation := op
		ret = append(ret, mcpSpec("channel_identity_"+op, "编辑显示名称、颜色和头像。目标仅用于原生角色委托，操作人始终为 Key 所属用户。", []string{"identity:write"}, true, op == "update", false, func(_ context.Context, a *service.MCPActor, in mcpIdentityWriteInput) (any, error) {
			if operation == "create" && in.ResourceID != "" {
				return nil, mcpFailure("invalid_argument", "创建不接受 resourceId")
			}
			actor, err := mcpIdentityActor(a, in.mcpIdentityInput)
			if err != nil {
				return nil, err
			}
			if actor.IsBotTarget {
				return nil, mcpFailure("forbidden", "第一版 MCP 不修改 BOT 身份")
			}
			input := &service.ChannelIdentityInput{ChannelID: in.ChannelID, ConfirmMCPAvatar: true}
			if operation == "update" {
				old, err := mcpIdentityResource(actor, in.ChannelID, in.ResourceID)
				if err != nil {
					return nil, err
				}
				if err := service.ApplyTemporaryIdentityActivateModes(actor.TargetUserID, []*model.ChannelIdentityModel{old}); err != nil {
					return nil, err
				}
				input.DisplayName = old.DisplayName
				input.Color = old.Color
				input.AvatarAttachmentID = old.AvatarAttachmentID
				input.AvatarDecorations = old.AvatarDecorations
				input.IsDefault = old.IsDefault
				input.IsTemporary = old.IsTemporary
				input.ICOOCOnActivate = old.ICOOCOnActivate
			}
			if in.DisplayName != nil {
				input.DisplayName = *in.DisplayName
			}
			if in.Color != nil {
				input.Color = *in.Color
			}
			if in.AvatarAttachmentID != nil {
				input.AvatarAttachmentID = *in.AvatarAttachmentID
			}
			var v *model.ChannelIdentityModel
			if operation == "create" {
				v, err = service.ChannelIdentityCreateWithAccess(actor.TargetUserID, a.User.ID, input)
			} else {
				v, err = service.ChannelIdentityUpdateWithAccess(actor.TargetUserID, a.User.ID, in.ResourceID, input)
			}
			if err != nil {
				return nil, err
			}
			broadcastUpdatedSharedChannelIdentityCopies(v, actor.TargetUserID, a.User.ID, "identity-"+operation, false)
			return mcpWriteDetail(a, "identity:read", mcpIdentityDTOFrom(v), mcpWriteRef{ID: v.ID, WorldID: in.WorldID, ChannelID: in.ChannelID}), nil
		}))
	}
	return ret
}

type mcpAudioAssetInput struct {
	mcpWorldInput
	mcpPageInput
	Query string `json:"query,omitempty"`
}
type mcpAudioAssetDTO struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Duration float64  `json:"duration"`
	Tags     []string `json:"tags"`
	Scope    string   `json:"scope"`
	WorldID  *string  `json:"worldId"`
	Status   string   `json:"status"`
}
type mcpAudioUpdateInput struct {
	mcpChannelInput
	ExpectedScopeType    string                `json:"expectedScopeType"`
	ExpectedScopeID      string                `json:"expectedScopeId"`
	ExpectedRevision     *int64                `json:"expectedRevision"`
	Tracks               *[]mcpAudioTrackInput `json:"tracks,omitempty"`
	SceneID              *string               `json:"sceneId,omitempty"`
	ClearScene           bool                  `json:"clearScene,omitempty"`
	IsPlaying            *bool                 `json:"isPlaying,omitempty"`
	Position             *float64              `json:"position,omitempty"`
	LoopEnabled          *bool                 `json:"loopEnabled,omitempty"`
	PlaybackRate         *float64              `json:"playbackRate,omitempty"`
	WorldPlaybackEnabled *bool                 `json:"worldPlaybackEnabled,omitempty"`
}

// Keep the writable track fields explicit, independently of persistence models.
type mcpAudioTrackInput struct {
	Type             string   `json:"type"`
	AssetID          *string  `json:"assetId"`
	Volume           float64  `json:"volume"`
	Muted            bool     `json:"muted"`
	Solo             bool     `json:"solo"`
	FadeIn           int      `json:"fadeIn"`
	FadeOut          int      `json:"fadeOut"`
	IsPlaying        bool     `json:"isPlaying"`
	Position         float64  `json:"position"`
	LoopEnabled      bool     `json:"loopEnabled"`
	PlaybackRate     float64  `json:"playbackRate"`
	PlaylistFolderID *string  `json:"playlistFolderId,omitempty"`
	PlaylistMode     *string  `json:"playlistMode,omitempty"`
	PlaylistAssetIDs []string `json:"playlistAssetIds,omitempty"`
	PlaylistIndex    int      `json:"playlistIndex"`
}

func (t mcpAudioTrackInput) state() service.AudioTrackState {
	return service.AudioTrackState{Type: t.Type, AssetID: t.AssetID, Volume: t.Volume, Muted: t.Muted, Solo: t.Solo, FadeIn: t.FadeIn, FadeOut: t.FadeOut, IsPlaying: t.IsPlaying, Position: t.Position, LoopEnabled: t.LoopEnabled, PlaybackRate: t.PlaybackRate, PlaylistFolderID: t.PlaylistFolderID, PlaylistMode: t.PlaylistMode, PlaylistAssetIDs: t.PlaylistAssetIDs, PlaylistIndex: t.PlaylistIndex}
}

type mcpAudioStateDTO struct {
	ChannelID            string                    `json:"channelId"`
	SceneID              *string                   `json:"sceneId"`
	Tracks               []service.AudioTrackState `json:"tracks"`
	IsPlaying            bool                      `json:"isPlaying"`
	Position             float64                   `json:"position"`
	LoopEnabled          bool                      `json:"loopEnabled"`
	PlaybackRate         float64                   `json:"playbackRate"`
	WorldPlaybackEnabled bool                      `json:"worldPlaybackEnabled"`
	Revision             int64                     `json:"revision"`
	ScopeType            string                    `json:"scopeType"`
	ScopeID              string                    `json:"scopeId"`
	UpdatedAt            int64                     `json:"updatedAt"`
}

func mcpAudioState(v *service.AudioPlaybackStateSnapshot, channelID string) mcpAudioStateDTO {
	if v == nil {
		return mcpAudioStateDTO{ChannelID: channelID, ScopeType: "channel", ScopeID: channelID, Tracks: []service.AudioTrackState{}, PlaybackRate: 1}
	}
	return mcpAudioStateDTO{channelID, v.SceneID, v.Tracks, v.IsPlaying, v.Position, v.LoopEnabled, v.PlaybackRate, v.WorldPlaybackEnabled, v.Revision, v.ScopeType, v.ScopeID, v.UpdatedAt.UnixMilli()}
}
func mcpAudioAccess(a *service.MCPActor, worldID, channelID string) error {
	cfg := mcpConfigSnapshot()
	if !pm.CanWithSystemRole(a.User.ID, pm.PermModAdmin) && !cfg.Audio.AllowWorldAudioWorkbench {
		return mcpFailure("forbidden", "平台未开放音频工作台")
	}
	if channelID != "" {
		if _, err := mcpChannel(a, worldID, channelID); err != nil {
			return err
		}
		if err := ensureChannelMembership(a.User.ID, channelID); err != nil {
			return service.ErrWorldPermission
		}
		return nil
	}
	return mcpWorld(a, worldID)
}
func mcpAudioTools() []mcpToolSpec {
	return []mcpToolSpec{
		mcpSpec("audio_assets_list", "读取当前世界及通用音频资产的公开资料，不返回存储内部字段。", []string{"audio:read"}, false, false, true, func(_ context.Context, a *service.MCPActor, in mcpAudioAssetInput) (any, error) {
			if err := mcpAudioAccess(a, in.WorldID, ""); err != nil {
				return nil, err
			}
			page, limit, err := in.bounds()
			if err != nil {
				return nil, err
			}
			rows, total, err := service.AudioListAssets(service.AudioAssetFilters{Query: in.Query, Page: page, PageSize: limit, Scope: model.AudioScopeWorld, WorldID: &in.WorldID, IncludeCommon: true, ExcludeTags: []string{service.TheaterFeatureAudioTag}})
			if err != nil {
				return nil, err
			}
			items := []mcpAudioAssetDTO{}
			for _, v := range rows {
				items = append(items, mcpAudioAssetDTO{v.ID, v.Name, v.DurationSeconds, append([]string(nil), v.Tags...), string(v.Scope), v.WorldID, string(v.TranscodeStatus)})
			}
			return mcpPaged(items, page, limit, total), nil
		}),
		mcpSpec("audio_state_get", "读取真实作用范围和 revision；空状态的频道 revision 为 0。", []string{"audio:read"}, false, false, true, func(_ context.Context, a *service.MCPActor, in mcpChannelInput) (any, error) {
			if err := mcpAudioAccess(a, in.WorldID, in.ChannelID); err != nil {
				return nil, err
			}
			v, err := service.AudioGetPlaybackState(in.ChannelID)
			return mcpDetail(mcpAudioState(v, in.ChannelID)), err
		}),
		mcpSpec("audio_state_update", "保留未提交状态；要求读取时的 scopeType/scopeId/revision。切换世界播放必须显式提交 worldPlaybackEnabled。", []string{"audio:write"}, true, true, false, mcpAudioUpdate),
	}
}
func mcpAudioUpdate(_ context.Context, a *service.MCPActor, in mcpAudioUpdateInput) (any, error) {
	if err := mcpAudioAccess(a, in.WorldID, in.ChannelID); err != nil {
		return nil, err
	}
	if in.ExpectedRevision == nil || *in.ExpectedRevision < 0 || in.ExpectedScopeType == "" || in.ExpectedScopeID == "" {
		return nil, mcpFailure("invalid_argument", "scope 和 expectedRevision 必填")
	}
	old, err := service.AudioGetPlaybackState(in.ChannelID)
	if err != nil {
		return nil, err
	}
	state := mcpAudioState(old, in.ChannelID)
	if state.Revision != *in.ExpectedRevision || state.ScopeType != in.ExpectedScopeType || state.ScopeID != in.ExpectedScopeID {
		return nil, mcpFailure("conflict", "播放范围或 revision 已改变")
	}
	input := service.AudioPlaybackUpdateInput{ChannelID: in.ChannelID, SceneID: state.SceneID, Tracks: state.Tracks, IsPlaying: state.IsPlaying, Position: state.Position, CapturedAtMs: time.Now().UnixMilli(), LoopEnabled: state.LoopEnabled, PlaybackRate: state.PlaybackRate, WorldPlaybackEnabled: state.WorldPlaybackEnabled, BaseRevision: state.Revision, ExpectedScopeType: state.ScopeType, ExpectedScopeID: state.ScopeID, ActorID: a.User.ID, Persist: true, SyncReason: "mcp"}
	if in.Tracks != nil {
		if len(*in.Tracks) > 20 {
			return nil, mcpFailure("invalid_argument", "音轨数量过多")
		}
		input.Tracks = make([]service.AudioTrackState, len(*in.Tracks))
		for i, track := range *in.Tracks {
			input.Tracks[i] = track.state()
		}
	}
	if in.SceneID != nil {
		input.SceneID = in.SceneID
	}
	if in.ClearScene {
		input.SceneID = nil
	}
	if in.IsPlaying != nil {
		input.IsPlaying = *in.IsPlaying
	}
	if in.Position != nil {
		input.Position = *in.Position
	}
	if in.LoopEnabled != nil {
		input.LoopEnabled = *in.LoopEnabled
	}
	if in.PlaybackRate != nil {
		input.PlaybackRate = *in.PlaybackRate
	}
	if in.WorldPlaybackEnabled != nil {
		input.WorldPlaybackEnabled = *in.WorldPlaybackEnabled
	}
	if math.IsNaN(input.Position) || math.IsInf(input.Position, 0) || input.Position < 0 || input.PlaybackRate < 0.25 || input.PlaybackRate > 4 {
		return nil, mcpFailure("invalid_argument", "播放位置或倍速无效")
	}
	if err := service.ValidateMCPAudioReferences(in.WorldID, input); err != nil {
		return nil, err
	}
	v, err := service.AudioUpsertPlaybackState(input)
	if err != nil {
		return nil, err
	}
	broadcastAudioPlaybackState(a.User, v)
	return mcpWriteDetail(a, "audio:read", mcpAudioState(v, in.ChannelID), mcpWriteRef{WorldID: in.WorldID, ChannelID: in.ChannelID, Revision: v.Revision, ScopeType: v.ScopeType, ScopeID: v.ScopeID}), nil
}
