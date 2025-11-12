package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	defaultIFormWidth  = 640
	defaultIFormHeight = 360
)

// ChannelIFormMediaOptions 控制嵌入窗媒体行为
// 实现 driver.Valuer / sql.Scanner 以JSON形式落库，兼容多种数据库
type ChannelIFormMediaOptions struct {
	AutoPlay   bool `json:"autoPlay"`
	AutoUnmute bool `json:"autoUnmute"`
	AutoExpand bool `json:"autoExpand"`
	AllowAudio bool `json:"allowAudio"`
	AllowVideo bool `json:"allowVideo"`
}

func (opts ChannelIFormMediaOptions) Value() (driver.Value, error) {
	data, err := json.Marshal(opts)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (opts *ChannelIFormMediaOptions) Scan(value interface{}) error {
	if value == nil {
		*opts = ChannelIFormMediaOptions{}
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return errors.New("unsupported media options type")
	}
	if len(data) == 0 {
		*opts = ChannelIFormMediaOptions{}
		return nil
	}
	return json.Unmarshal(data, opts)
}

// ChannelIFormModel 表示频道级别的 iForm 嵌入配置
// 允许同时提供 URL 或嵌入代码，默认按顺序渲染
// TableName: channel_iforms
// 包含排序、默认布局和媒体优化选项
// json 标签面向前端直接序列化使用

type ChannelIFormModel struct {
	StringPKBaseModel
	ChannelID        string                   `json:"channelId" gorm:"index;not null"`
	Name             string                   `json:"name"`
	Url              string                   `json:"url"`
	EmbedCode        string                   `json:"embedCode"`
	DefaultWidth     int                      `json:"defaultWidth"`
	DefaultHeight    int                      `json:"defaultHeight"`
	DefaultCollapsed bool                     `json:"defaultCollapsed"`
	DefaultFloating  bool                     `json:"defaultFloating"`
	AllowPopout      bool                     `json:"allowPopout"`
	OrderIndex       int                      `json:"orderIndex"`
	CreatedBy        string                   `json:"createdBy"`
	UpdatedBy        string                   `json:"updatedBy"`
	MediaOptions     ChannelIFormMediaOptions `json:"mediaOptions" gorm:"type:json"`
}

func (*ChannelIFormModel) TableName() string {
	return "channel_iforms"
}

func (m *ChannelIFormModel) BeforeSave(tx *gorm.DB) error {
	m.Normalize()
	return nil
}

func (m *ChannelIFormModel) Normalize() {
	m.Name = strings.TrimSpace(m.Name)
	m.Url = strings.TrimSpace(m.Url)
	m.EmbedCode = strings.TrimSpace(m.EmbedCode)
	if m.DefaultWidth <= 0 {
		m.DefaultWidth = defaultIFormWidth
	}
	if m.DefaultWidth > 1920 {
		m.DefaultWidth = 1920
	}
	if m.DefaultHeight <= 0 {
		m.DefaultHeight = defaultIFormHeight
	}
	if m.DefaultHeight > 1440 {
		m.DefaultHeight = 1440
	}
	m.OrderIndex = normalizeOrderIndex(m.OrderIndex)
}

func normalizeOrderIndex(current int) int {
	if current == 0 {
		return int(time.Now().Unix())
	}
	return current
}

func ChannelIFormList(channelID string) ([]*ChannelIFormModel, error) {
	var items []*ChannelIFormModel
	err := db.Where("channel_id = ?", channelID).
		Order("order_index DESC").
		Order("created_at ASC").
		Find(&items).Error
	return items, err
}

func ChannelIFormGet(channelID, formID string) (*ChannelIFormModel, error) {
	var item ChannelIFormModel
	err := db.Where("channel_id = ? AND id = ?", channelID, formID).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &item, err
}

func ChannelIFormCreate(form *ChannelIFormModel) error {
	if form == nil {
		return errors.New("form is nil")
	}
	form.Normalize()
	if form.OrderIndex == 0 {
		var max int
		_ = db.Model(&ChannelIFormModel{}).
			Where("channel_id = ?", form.ChannelID).
			Select("COALESCE(MAX(order_index),0)").
			Scan(&max)
		form.OrderIndex = max + 1
	}
	return db.Create(form).Error
}

func ChannelIFormUpdate(channelID, formID string, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	updates["updated_at"] = time.Now()
	return db.Model(&ChannelIFormModel{}).
		Where("channel_id = ? AND id = ?", channelID, formID).
		Updates(updates).Error
}

func ChannelIFormDelete(channelID, formID string) error {
	return db.Where("channel_id = ? AND id = ?", channelID, formID).
		Delete(&ChannelIFormModel{}).Error
}

func ChannelIFormCloneToChannel(source *ChannelIFormModel, targetChannelID, actor string) (*ChannelIFormModel, error) {
	if source == nil {
		return nil, errors.New("source is nil")
	}
	clone := *source
	clone.StringPKBaseModel = StringPKBaseModel{}
	clone.ChannelID = targetChannelID
	clone.CreatedBy = actor
	clone.UpdatedBy = actor
	clone.OrderIndex = 0
	clone.Normalize()
	if err := ChannelIFormCreate(&clone); err != nil {
		return nil, err
	}
	return &clone, nil
}
