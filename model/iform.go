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

// ChannelIFormBridgePolicy controls the host-mediated embed runtime. Empty
// legacy values stay disabled for backwards compatibility.
type ChannelIFormBridgePolicy struct {
	Enabled        bool     `json:"enabled"`
	AllowedOrigins []string `json:"allowedOrigins"`
	Capabilities   []string `json:"capabilities"`
}

// ChannelIFormTemplateOverrides stores only fields explicitly customized by a
// channel reference. Pointer fields preserve false/zero/empty overrides.
type ChannelIFormTemplateOverrides struct {
	Name             *string                   `json:"name,omitempty"`
	DefaultWidth     *int                      `json:"defaultWidth,omitempty"`
	DefaultHeight    *int                      `json:"defaultHeight,omitempty"`
	DefaultCollapsed *bool                     `json:"defaultCollapsed,omitempty"`
	DefaultFloating  *bool                     `json:"defaultFloating,omitempty"`
	AllowPopout      *bool                     `json:"allowPopout,omitempty"`
	MediaOptions     *ChannelIFormMediaOptions `json:"mediaOptions,omitempty"`
	BridgePolicy     *ChannelIFormBridgePolicy `json:"bridgePolicy,omitempty"`
}

func (overrides ChannelIFormTemplateOverrides) Value() (driver.Value, error) {
	data, err := json.Marshal(overrides)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (overrides *ChannelIFormTemplateOverrides) Scan(value interface{}) error {
	if value == nil {
		*overrides = ChannelIFormTemplateOverrides{}
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return errors.New("unsupported channel iForm template overrides type")
	}
	if len(data) == 0 {
		*overrides = ChannelIFormTemplateOverrides{}
		return nil
	}
	return json.Unmarshal(data, overrides)
}

// ChannelIFormTemplateModel is a platform-managed template. Builtin tools are
// represented by stable keys and never stored in this table.
type ChannelIFormTemplateModel struct {
	StringPKBaseModel
	Name             string                   `json:"name"`
	Description      string                   `json:"description"`
	Url              string                   `json:"url"`
	EmbedCode        string                   `json:"embedCode"`
	DefaultWidth     int                      `json:"defaultWidth"`
	DefaultHeight    int                      `json:"defaultHeight"`
	DefaultCollapsed bool                     `json:"defaultCollapsed"`
	DefaultFloating  bool                     `json:"defaultFloating"`
	AllowPopout      bool                     `json:"allowPopout"`
	MediaOptions     ChannelIFormMediaOptions `json:"mediaOptions" gorm:"type:json"`
	BridgePolicy     ChannelIFormBridgePolicy `json:"bridgePolicy" gorm:"type:json"`
	Enabled          bool                     `json:"enabled"`
	Archived         bool                     `json:"archived"`
	CreatedBy        string                   `json:"createdBy"`
	UpdatedBy        string                   `json:"updatedBy"`
}

func (*ChannelIFormTemplateModel) TableName() string { return "channel_iform_templates" }

func (m *ChannelIFormTemplateModel) BeforeSave(tx *gorm.DB) error {
	m.Normalize()
	return nil
}

func (m *ChannelIFormTemplateModel) Normalize() {
	m.Name = strings.TrimSpace(m.Name)
	m.Description = strings.TrimSpace(m.Description)
	m.Url = strings.TrimSpace(m.Url)
	m.EmbedCode = strings.TrimSpace(m.EmbedCode)
	if m.DefaultWidth <= 0 {
		m.DefaultWidth = defaultIFormWidth
	}
	if m.DefaultHeight <= 0 {
		m.DefaultHeight = defaultIFormHeight
	}
	if m.DefaultWidth > 1920 {
		m.DefaultWidth = 1920
	}
	if m.DefaultHeight > 1440 {
		m.DefaultHeight = 1440
	}
}

func (policy ChannelIFormBridgePolicy) Value() (driver.Value, error) {
	data, err := json.Marshal(policy)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (policy *ChannelIFormBridgePolicy) Scan(value interface{}) error {
	if value == nil {
		*policy = ChannelIFormBridgePolicy{}
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return errors.New("unsupported bridge policy type")
	}
	if len(data) == 0 {
		*policy = ChannelIFormBridgePolicy{}
		return nil
	}
	return json.Unmarshal(data, policy)
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
	ChannelID         string                        `json:"channelId" gorm:"index;not null"`
	Name              string                        `json:"name"`
	Url               string                        `json:"url"`
	EmbedCode         string                        `json:"embedCode"`
	DefaultWidth      int                           `json:"defaultWidth"`
	DefaultHeight     int                           `json:"defaultHeight"`
	DefaultCollapsed  bool                          `json:"defaultCollapsed"`
	DefaultFloating   bool                          `json:"defaultFloating"`
	AllowPopout       bool                          `json:"allowPopout"`
	OrderIndex        int                           `json:"orderIndex"`
	CreatedBy         string                        `json:"createdBy"`
	UpdatedBy         string                        `json:"updatedBy"`
	MediaOptions      ChannelIFormMediaOptions      `json:"mediaOptions" gorm:"type:json"`
	BridgePolicy      ChannelIFormBridgePolicy      `json:"bridgePolicy" gorm:"type:json"`
	TemplateRef       string                        `json:"templateRef,omitempty" gorm:"size:160;index"`
	TemplateOverrides ChannelIFormTemplateOverrides `json:"templateOverrides,omitempty" gorm:"type:json"`
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
	m.TemplateRef = strings.TrimSpace(m.TemplateRef)
	if m.TemplateRef == "" {
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
	if strings.TrimSpace(form.TemplateRef) == "" {
		form.TemplateOverrides = ChannelIFormTemplateOverrides{}
	} else if strings.TrimSpace(form.Url) != "" || strings.TrimSpace(form.EmbedCode) != "" {
		return errors.New("模板引用控件的 URL 和 EmbedCode 由模板管理")
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
	var current ChannelIFormModel
	if err := db.Where("channel_id = ? AND id = ?", channelID, formID).First(&current).Error; err == nil && strings.TrimSpace(current.TemplateRef) != "" {
		if _, ok := updates["url"]; ok {
			return errors.New("模板引用控件的 URL 由模板管理")
		}
		if _, ok := updates["embed_code"]; ok {
			return errors.New("模板引用控件的 EmbedCode 由模板管理")
		}
		if _, ok := updates["template_ref"]; ok {
			return errors.New("模板引用控件不可切换模板")
		}
		if _, ok := updates["templateRef"]; ok {
			return errors.New("模板引用控件不可切换模板")
		}
	}
	updates["updated_at"] = time.Now()
	return db.Model(&ChannelIFormModel{}).
		Where("channel_id = ? AND id = ?", channelID, formID).
		Updates(updates).Error
}

func ChannelIFormDelete(channelID, formID string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		return ChannelIFormDeleteTx(tx, channelID, formID)
	})
}

func ChannelIFormDeleteTx(tx *gorm.DB, channelID, formID string) error {
	if tx == nil {
		return errors.New("db transaction is nil")
	}
	if err := tx.Unscoped().Where("channel_id = ? AND id = ?", channelID, formID).Delete(&ChannelIFormModel{}).Error; err != nil {
		return err
	}
	return ChannelIFormStorageDeleteByFormIDTx(tx, formID)
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
