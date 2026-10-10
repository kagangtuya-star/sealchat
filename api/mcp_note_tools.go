package api

import (
	"context"
	"time"
	"unicode/utf8"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

type mcpNoteListInput struct {
	mcpChannelInput
	mcpPageInput
	ResourceID string `json:"resourceId,omitempty"`
}
type mcpNoteWriteInput struct {
	mcpChannelInput
	ResourceID  string  `json:"resourceId,omitempty"`
	Title       *string `json:"title,omitempty"`
	Content     *string `json:"content,omitempty"`
	ContentText *string `json:"contentText,omitempty"`
	Color       *string `json:"color,omitempty"`
	IsPinned    *bool   `json:"isPinned,omitempty"`
}
type mcpNoteDTO struct {
	ID            string                         `json:"id"`
	WorldID       string                         `json:"worldId"`
	ChannelID     string                         `json:"channelId"`
	Title         string                         `json:"title"`
	Content       string                         `json:"content"`
	ContentText   string                         `json:"contentText"`
	Color         string                         `json:"color"`
	NoteType      model.StickyNoteType           `json:"noteType"`
	TypeData      string                         `json:"typeData"`
	Appearance    *protocol.StickyNoteAppearance `json:"appearance"`
	Visibility    model.StickyNoteVisibility     `json:"visibility"`
	IsPinned      bool                           `json:"isPinned"`
	CreatorID     string                         `json:"creatorId"`
	DefaultX      int                            `json:"defaultX"`
	DefaultY      int                            `json:"defaultY"`
	DefaultW      int                            `json:"defaultW"`
	DefaultH      int                            `json:"defaultH"`
	EditingLocked bool                           `json:"editingLocked"`
	UpdatedAt     int64                          `json:"updatedAt"`
}

func mcpNoteDTOFrom(n *model.StickyNoteModel) mcpNoteDTO {
	return mcpNoteDTO{n.ID, n.WorldID, n.ChannelID, n.Title, n.Content, n.ContentText, n.Color, n.NoteType, n.TypeData, n.ToProtocolType().Appearance, n.Visibility, n.IsPinned, n.CreatorID, n.DefaultX, n.DefaultY, n.DefaultW, n.DefaultH, n.IsEditingLockActive(time.Now()), n.UpdatedAt.UnixMilli()}
}
func mcpNoteResource(a *service.MCPActor, in mcpResourceInput) (*model.StickyNoteModel, error) {
	if _, err := mcpChannel(a, in.WorldID, in.ChannelID); err != nil {
		return nil, err
	}
	n, err := model.StickyNoteGet(in.ResourceID)
	if err != nil {
		return nil, err
	}
	if n.WorldID != in.WorldID || n.ChannelID != in.ChannelID {
		return nil, mcpFailure("invalid_argument", "便签归属不匹配")
	}
	if !service.CanViewStickyNote(n, a.User.ID) {
		return nil, service.ErrWorldPermission
	}
	return n, nil
}
func mcpNoteTools() []mcpToolSpec {
	return []mcpToolSpec{
		mcpSpec("note_read", "无 resourceId 分页读取可见便签；有 resourceId 读取详情。私人便签先过滤再计数和分页。", []string{"note:read"}, false, false, true, func(_ context.Context, a *service.MCPActor, in mcpNoteListInput) (any, error) {
			if in.ResourceID != "" {
				n, err := mcpNoteResource(a, mcpResourceInput{in.WorldID, in.ChannelID, in.ResourceID})
				if err != nil {
					return nil, err
				}
				return mcpDetail(mcpNoteDTOFrom(n)), nil
			}
			if _, err := mcpChannel(a, in.WorldID, in.ChannelID); err != nil {
				return nil, err
			}
			page, limit, err := in.bounds()
			if err != nil {
				return nil, err
			}
			items := []mcpNoteDTO{}
			var total int64
			offset := 0
			for {
				rows := []model.StickyNoteModel{}
				err := model.GetDB().Where("channel_id = ? AND is_deleted = ?", in.ChannelID, false).Order("order_index ASC, created_at ASC, id ASC").Offset(offset).Limit(200).Find(&rows).Error
				if err != nil {
					return nil, err
				}
				for i := range rows {
					n := &rows[i]
					if !service.CanViewStickyNote(n, a.User.ID) {
						continue
					}
					if total >= int64((page-1)*limit) && len(items) < limit {
						items = append(items, mcpNoteDTOFrom(n))
					}
					total++
				}
				if len(rows) < 200 {
					break
				}
				offset += 200
			}
			return mcpPaged(items, page, limit, total), nil
		}),
		mcpSpec("note_save", "无 resourceId 创建普通文本便签；有 resourceId 部分更新正常内容字段，保留类型、外观、可见范围和布局并尊重编辑锁。", []string{"note:write"}, true, true, false, func(_ context.Context, a *service.MCPActor, in mcpNoteWriteInput) (any, error) {
			if in.ResourceID != "" {
				return mcpNoteUpdate(a, in)
			}
			ch, err := mcpChannel(a, in.WorldID, in.ChannelID)
			if err != nil {
				return nil, err
			}
			if err := service.EnsureStickyNoteChannelMembership(a.User.ID, ch.ID); err != nil {
				return nil, err
			}
			n := &model.StickyNoteModel{StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()}, ChannelID: ch.ID, WorldID: ch.WorldID, CreatorID: a.User.ID, Color: "yellow", NoteType: model.StickyNoteTypeText, Visibility: model.StickyNoteVisibilityAll, IsPublic: true, DefaultW: 300, DefaultH: 250}
			if _, err := mcpNoteUpdates(in); err != nil {
				return nil, err
			}
			n.Title = mcpValue(in.Title)
			n.Content = mcpValue(in.Content)
			n.ContentText = n.Content
			if in.ContentText != nil {
				n.ContentText = *in.ContentText
			}
			if in.Color != nil {
				n.Color, _ = model.StickyNoteNormalizeColor(*in.Color)
			}
			n.IsPinned = mcpValue(in.IsPinned)
			var world model.WorldModel
			if err := model.GetDB().Where("id = ?", in.WorldID).First(&world).Error; err != nil {
				return nil, err
			}
			n.AppearanceJSON = world.StickyNoteDefaultAppearanceJSON
			if err := model.StickyNoteCreate(n); err != nil {
				return nil, err
			}
			n.Creator = a.User
			broadcastStickyNoteToVisibleUsers(ch.ID, protocol.EventStickyNoteCreated, "create", n)
			return mcpWriteDetail(a, "note:read", mcpNoteDTOFrom(n), mcpWriteRef{ID: n.ID, WorldID: n.WorldID, ChannelID: n.ChannelID}), nil
		}),
		mcpSpec("note_delete", "仅创建者或频道管理员可删除可见便签；通知仍只发送给原可见用户。", []string{"note:write"}, true, true, false, func(_ context.Context, a *service.MCPActor, in mcpResourceInput) (any, error) {
			n, err := mcpNoteResource(a, in)
			if err != nil {
				return nil, err
			}
			if !service.MCPCanDeleteStickyNote(n, a.User.ID) {
				return nil, service.ErrWorldPermission
			}
			if err := model.StickyNoteDelete(n.ID, a.User.ID); err != nil {
				return nil, err
			}
			broadcastStickyNoteDeleteToVisibleUsers(n.ChannelID, n)
			return mcpOK{true}, nil
		}),
	}
}

func mcpNoteUpdate(a *service.MCPActor, in mcpNoteWriteInput) (any, error) {
	n, err := mcpNoteResource(a, mcpResourceInput{in.WorldID, in.ChannelID, in.ResourceID})
	if err != nil {
		return nil, err
	}
	if err := service.EnsureStickyNoteChannelMembership(a.User.ID, n.ChannelID); err != nil {
		return nil, err
	}
	now := time.Now()
	if n.IsEditingLockActive(now) && n.EditingLockUserID != a.User.ID {
		return nil, mcpFailure("conflict", "便签正在被其他用户编辑")
	}
	updates, err := mcpNoteUpdates(in)
	if err != nil {
		return nil, err
	}
	updates["updated_at"] = now
	r := model.GetDB().Model(&model.StickyNoteModel{}).Where("id = ? AND is_deleted = ? AND updated_at = ?", n.ID, false, n.UpdatedAt).Where("(editing_lock_user_id = '' OR editing_lock_user_id IS NULL OR editing_lock_expire_at IS NULL OR editing_lock_expire_at <= ? OR editing_lock_user_id = ?)", now, a.User.ID).Updates(updates)
	if r.Error != nil {
		return nil, r.Error
	}
	if r.RowsAffected != 1 {
		return nil, mcpFailure("conflict", "便签或编辑锁已改变")
	}
	current, err := loadStickyNoteForResponse(n.ID)
	if err != nil {
		return nil, err
	}
	broadcastStickyNoteUpdateTransition(n.ChannelID, n, current)
	return mcpWriteDetail(a, "note:read", mcpNoteDTOFrom(current), mcpWriteRef{ID: current.ID, WorldID: current.WorldID, ChannelID: current.ChannelID}), nil
}
func mcpNoteUpdates(in mcpNoteWriteInput) (map[string]any, error) {
	m := map[string]any{}
	if in.Title != nil {
		if utf8.RuneCountInString(*in.Title) > 255 {
			return nil, mcpFailure("invalid_argument", "标题过长")
		}
		m["title"] = *in.Title
	}
	if in.Content != nil {
		if len(*in.Content) > 1<<20 {
			return nil, mcpFailure("invalid_argument", "正文过长")
		}
		m["content"] = *in.Content
	}
	if in.ContentText != nil {
		if len(*in.ContentText) > 1<<20 {
			return nil, mcpFailure("invalid_argument", "纯文本过长")
		}
		m["content_text"] = *in.ContentText
	}
	if in.Color != nil {
		color, ok := model.StickyNoteNormalizeColor(*in.Color)
		if !ok {
			return nil, mcpFailure("invalid_argument", "便签颜色无效")
		}
		m["color"] = color
	}
	if in.IsPinned != nil {
		m["is_pinned"] = *in.IsPinned
	}
	return m, nil
}
