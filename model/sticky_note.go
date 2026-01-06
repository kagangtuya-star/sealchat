package model

import (
	"time"

	"sealchat/protocol"
	"sealchat/utils"
)

// StickyNoteType 便签类型
type StickyNoteType string

const (
	StickyNoteTypeText         StickyNoteType = "text"
	StickyNoteTypeCounter      StickyNoteType = "counter"
	StickyNoteTypeList         StickyNoteType = "list"
	StickyNoteTypeSlider       StickyNoteType = "slider"
	StickyNoteTypeChat         StickyNoteType = "chat"
	StickyNoteTypeTimer        StickyNoteType = "timer"
	StickyNoteTypeClock        StickyNoteType = "clock"
	StickyNoteTypeRoundCounter StickyNoteType = "roundCounter"
)

// StickyNoteVisibility 便签可见性
type StickyNoteVisibility string

const (
	StickyNoteVisibilityAll     StickyNoteVisibility = "all"
	StickyNoteVisibilityOwner   StickyNoteVisibility = "owner"
	StickyNoteVisibilityEditors StickyNoteVisibility = "editors"
	StickyNoteVisibilityViewers StickyNoteVisibility = "viewers"
)

// StickyNoteModel 便签数据模型
type StickyNoteModel struct {
	StringPKBaseModel
	ChannelID   string     `json:"channel_id" gorm:"size:100;index:idx_sticky_channel_order,priority:1" binding:"required"`
	WorldID     string     `json:"world_id" gorm:"size:100;index"`
	FolderID    string     `json:"folder_id" gorm:"size:100;index"` // 所属文件夹
	Title       string     `json:"title" gorm:"size:255"`
	Content     string     `json:"content" gorm:"type:text"`      // HTML 富文本
	ContentText string     `json:"content_text" gorm:"type:text"` // 纯文本版本（用于搜索）
	Color       string     `json:"color" gorm:"size:32;default:'yellow'"`
	CreatorID   string     `json:"creator_id" gorm:"size:100;index"`
	IsPublic    bool       `json:"is_public" gorm:"default:true"`
	IsPinned    bool       `json:"is_pinned" gorm:"default:false"`
	OrderIndex  int        `json:"order_index" gorm:"default:0;index:idx_sticky_channel_order,priority:2"`

	// 便签类型相关
	NoteType StickyNoteType `json:"note_type" gorm:"size:32;default:'text';index"` // text/counter/list/slider/chat/timer/clock/roundCounter
	TypeData string         `json:"type_data" gorm:"type:text"`                    // JSON 格式的类型特定数据

	// 权限相关
	Visibility StickyNoteVisibility `json:"visibility" gorm:"size:32;default:'all'"` // owner/editors/viewers/all
	ViewerIDs  string               `json:"viewer_ids" gorm:"type:text"`             // JSON 数组
	EditorIDs  string               `json:"editor_ids" gorm:"type:text"`             // JSON 数组

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
		FolderID:    s.FolderID,
		Title:       s.Title,
		Content:     s.Content,
		ContentText: s.ContentText,
		Color:       s.Color,
		CreatorID:   s.CreatorID,
		IsPublic:    s.IsPublic,
		IsPinned:    s.IsPinned,
		OrderIndex:  s.OrderIndex,
		NoteType:    string(s.NoteType),
		TypeData:    s.TypeData,
		Visibility:  string(s.Visibility),
		ViewerIDs:   s.ViewerIDs,
		EditorIDs:   s.EditorIDs,
		DefaultX:    s.DefaultX,
		DefaultY:    s.DefaultY,
		DefaultW:    s.DefaultW,
		DefaultH:    s.DefaultH,
		CreatedAt:   s.CreatedAt.UnixMilli(),
		UpdatedAt:   s.UpdatedAt.UnixMilli(),
	}
	// 默认值处理
	if note.NoteType == "" {
		note.NoteType = "text"
	}
	if note.Visibility == "" {
		note.Visibility = "all"
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

// StickyNoteFolderModel 便签文件夹模型
type StickyNoteFolderModel struct {
	StringPKBaseModel
	ChannelID  string `json:"channel_id" gorm:"size:100;index"`
	WorldID    string `json:"world_id" gorm:"size:100;index"`
	ParentID   string `json:"parent_id" gorm:"size:100;index"`
	Name       string `json:"name" gorm:"size:255"`
	Color      string `json:"color" gorm:"size:32"`
	OrderIndex int    `json:"order_index" gorm:"default:0"`
	CreatorID  string `json:"creator_id" gorm:"size:100"`

	IsDeleted bool       `json:"is_deleted" gorm:"default:false;index"`
	DeletedAt *time.Time `json:"deleted_at"`

	// 关联（不存储）
	Children []*StickyNoteFolderModel `json:"children" gorm:"-"`
}

func (*StickyNoteFolderModel) TableName() string {
	return "sticky_note_folders"
}

// StickyNoteFolderCreate 创建文件夹
func StickyNoteFolderCreate(folder *StickyNoteFolderModel) error {
	if folder.ID == "" {
		folder.ID = utils.NewID()
	}
	return db.Create(folder).Error
}

// StickyNoteFolderGet 获取单个文件夹
func StickyNoteFolderGet(id string) (*StickyNoteFolderModel, error) {
	var folder StickyNoteFolderModel
	err := db.Where("id = ? AND is_deleted = ?", id, false).First(&folder).Error
	return &folder, err
}

// StickyNoteFolderListByChannel 获取频道的所有文件夹
func StickyNoteFolderListByChannel(channelID string) ([]*StickyNoteFolderModel, error) {
	var folders []*StickyNoteFolderModel
	err := db.Where("channel_id = ? AND is_deleted = ?", channelID, false).
		Order("order_index ASC, created_at ASC").
		Find(&folders).Error
	return folders, err
}

// StickyNoteFolderUpdate 更新文件夹
func StickyNoteFolderUpdate(id string, updates map[string]interface{}) error {
	return db.Model(&StickyNoteFolderModel{}).
		Where("id = ? AND is_deleted = ?", id, false).
		Updates(updates).Error
}

// StickyNoteFolderDelete 软删除文件夹
func StickyNoteFolderDelete(id string) error {
	now := time.Now()
	return db.Model(&StickyNoteFolderModel{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"is_deleted": true,
			"deleted_at": now,
		}).Error
}

// StickyNoteClearFolder 清除文件夹内便签的 folder_id
func StickyNoteClearFolder(folderID string) error {
	return db.Model(&StickyNoteModel{}).
		Where("folder_id = ?", folderID).
		Update("folder_id", "").Error
}

// ToProtocolType 转换为协议类型
func (f *StickyNoteFolderModel) ToProtocolType() *protocol.StickyNoteFolder {
	folder := &protocol.StickyNoteFolder{
		ID:         f.ID,
		ChannelID:  f.ChannelID,
		WorldID:    f.WorldID,
		ParentID:   f.ParentID,
		Name:       f.Name,
		Color:      f.Color,
		OrderIndex: f.OrderIndex,
		CreatorID:  f.CreatorID,
		CreatedAt:  f.CreatedAt.UnixMilli(),
		UpdatedAt:  f.UpdatedAt.UnixMilli(),
	}
	if len(f.Children) > 0 {
		folder.Children = make([]*protocol.StickyNoteFolder, len(f.Children))
		for i, child := range f.Children {
			folder.Children[i] = child.ToProtocolType()
		}
	}
	return folder
}
