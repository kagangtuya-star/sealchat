package model

import (
	"time"

	"sealchat/protocol"
	"sealchat/utils"
)

// StickyNoteModel 便签数据模型
type StickyNoteModel struct {
	StringPKBaseModel
	ChannelID   string     `json:"channel_id" gorm:"size:100;index:idx_sticky_channel_order,priority:1" binding:"required"`
	WorldID     string     `json:"world_id" gorm:"size:100;index"`
	Title       string     `json:"title" gorm:"size:255"`
	Content     string     `json:"content" gorm:"type:text"`      // HTML 富文本
	ContentText string     `json:"content_text" gorm:"type:text"` // 纯文本版本（用于搜索）
	Color       string     `json:"color" gorm:"size:32;default:'yellow'"`
	CreatorID   string     `json:"creator_id" gorm:"size:100;index"`
	IsPublic    bool       `json:"is_public" gorm:"default:true"`
	IsPinned    bool       `json:"is_pinned" gorm:"default:false"`
	OrderIndex  int        `json:"order_index" gorm:"default:0;index:idx_sticky_channel_order,priority:2"`

	// 默认布局
	DefaultX int `json:"default_x" gorm:"default:100"`
	DefaultY int `json:"default_y" gorm:"default:100"`
	DefaultW int `json:"default_w" gorm:"default:300"`
	DefaultH int `json:"default_h" gorm:"default:250"`

	// 软删除
	IsDeleted bool       `json:"is_deleted" gorm:"default:false;index"`
	DeletedAt *time.Time `json:"deleted_at"`
	DeletedBy string     `json:"deleted_by" gorm:"size:100"`

	// 关联
	Creator *UserModel `json:"creator" gorm:"-"`
}

func (*StickyNoteModel) TableName() string {
	return "sticky_notes"
}

// StickyNoteUserStateModel 用户便签状态
type StickyNoteUserStateModel struct {
	StringPKBaseModel
	StickyNoteID string `json:"sticky_note_id" gorm:"size:100;index;uniqueIndex:idx_note_user"`
	UserID       string `json:"user_id" gorm:"size:100;index;uniqueIndex:idx_note_user"`

	IsOpen    bool `json:"is_open" gorm:"default:false"`
	PositionX int  `json:"position_x" gorm:"default:0"`
	PositionY int  `json:"position_y" gorm:"default:0"`
	Width     int  `json:"width" gorm:"default:0"`
	Height    int  `json:"height" gorm:"default:0"`
	Minimized bool `json:"minimized" gorm:"default:false"`
	ZIndex    int  `json:"z_index" gorm:"default:1000"`

	LastOpenedAt *time.Time `json:"last_opened_at"`
}

func (*StickyNoteUserStateModel) TableName() string {
	return "sticky_note_user_states"
}

// StickyNoteGet 获取单个便签
func StickyNoteGet(id string) (*StickyNoteModel, error) {
	var note StickyNoteModel
	err := db.Where("id = ? AND is_deleted = ?", id, false).First(&note).Error
	return &note, err
}

// StickyNoteListByChannel 获取频道的所有便签
func StickyNoteListByChannel(channelID string, includeDeleted bool) ([]*StickyNoteModel, error) {
	var notes []*StickyNoteModel
	query := db.Where("channel_id = ?", channelID)
	if !includeDeleted {
		query = query.Where("is_deleted = ?", false)
	}
	err := query.Order("order_index ASC, created_at ASC").Find(&notes).Error
	return notes, err
}

// StickyNoteCreate 创建便签
func StickyNoteCreate(note *StickyNoteModel) error {
	if note.ID == "" {
		note.ID = utils.NewID()
	}
	return db.Create(note).Error
}

// StickyNoteUpdate 更新便签
func StickyNoteUpdate(id string, updates map[string]interface{}) error {
	return db.Model(&StickyNoteModel{}).
		Where("id = ? AND is_deleted = ?", id, false).
		Updates(updates).Error
}

// StickyNoteDelete 软删除便签
func StickyNoteDelete(id string, deletedBy string) error {
	now := time.Now()
	return db.Model(&StickyNoteModel{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"is_deleted": true,
			"deleted_at": now,
			"deleted_by": deletedBy,
		}).Error
}

// StickyNoteUserStateUpsert 更新或插入用户状态
func StickyNoteUserStateUpsert(state *StickyNoteUserStateModel) error {
	if state.ID == "" {
		state.ID = utils.NewID()
	}
	return db.Save(state).Error
}

// StickyNoteUserStateGet 获取用户状态
func StickyNoteUserStateGet(noteID, userID string) (*StickyNoteUserStateModel, error) {
	var state StickyNoteUserStateModel
	err := db.Where("sticky_note_id = ? AND user_id = ?", noteID, userID).
		First(&state).Error
	return &state, err
}

// StickyNoteUserStateListByUser 获取用户在某频道的所有便签状态
func StickyNoteUserStateListByUser(userID string, channelID string) ([]*StickyNoteUserStateModel, error) {
	var states []*StickyNoteUserStateModel
	err := db.
		Joins("JOIN sticky_notes ON sticky_notes.id = sticky_note_user_states.sticky_note_id").
		Where("sticky_note_user_states.user_id = ? AND sticky_notes.channel_id = ? AND sticky_notes.is_deleted = ?", userID, channelID, false).
		Find(&states).Error
	return states, err
}

// StickyNoteUserStateListByNoteID 获取便签的所有用户状态
func StickyNoteUserStateListByNoteID(noteID string) ([]*StickyNoteUserStateModel, error) {
	var states []*StickyNoteUserStateModel
	err := db.Where("sticky_note_id = ?", noteID).Find(&states).Error
	return states, err
}

// 加载创建者信息
func (s *StickyNoteModel) LoadCreator() {
	if s.CreatorID != "" && s.Creator == nil {
		user := UserGet(s.CreatorID)
		if user != nil {
			s.Creator = user
		}
	}
}

// ToProtocolType 转换为协议类型
func (s *StickyNoteModel) ToProtocolType() *protocol.StickyNote {
	note := &protocol.StickyNote{
		ID:          s.ID,
		ChannelID:   s.ChannelID,
		WorldID:     s.WorldID,
		Title:       s.Title,
		Content:     s.Content,
		ContentText: s.ContentText,
		Color:       s.Color,
		CreatorID:   s.CreatorID,
		IsPublic:    s.IsPublic,
		IsPinned:    s.IsPinned,
		OrderIndex:  s.OrderIndex,
		DefaultX:    s.DefaultX,
		DefaultY:    s.DefaultY,
		DefaultW:    s.DefaultW,
		DefaultH:    s.DefaultH,
		CreatedAt:   s.CreatedAt.UnixMilli(),
		UpdatedAt:   s.UpdatedAt.UnixMilli(),
	}
	if s.Creator != nil {
		note.Creator = s.Creator.ToProtocolType()
	}
	return note
}

// ToProtocolUserState 转换用户状态为协议类型
func (s *StickyNoteUserStateModel) ToProtocolType() *protocol.StickyNoteUserState {
	return &protocol.StickyNoteUserState{
		NoteID:    s.StickyNoteID,
		IsOpen:    s.IsOpen,
		PositionX: s.PositionX,
		PositionY: s.PositionY,
		Width:     s.Width,
		Height:    s.Height,
		Minimized: s.Minimized,
		ZIndex:    s.ZIndex,
	}
}
