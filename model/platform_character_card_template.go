package model

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// PlatformCharacterCardTemplateModel is the single source for platform-wide
// character card templates. References are stored as platform:<id> elsewhere.
type PlatformCharacterCardTemplateModel struct {
	StringPKBaseModel
	Name                       string `json:"name" gorm:"size:100;not null"`
	SheetType                  string `json:"sheetType" gorm:"size:32"`
	Content                    string `json:"content" gorm:"type:text;not null"`
	BadgeTemplateOverride      string `json:"badgeTemplateOverride" gorm:"size:512"`
	TheaterOverlayTemplateJSON string `json:"theaterOverlayTemplateJson" gorm:"type:text"`
	Enabled                    bool   `json:"enabled" gorm:"not null;default:true;index"`
	CreatedBy                  string `json:"createdBy" gorm:"size:100;not null"`
	UpdatedBy                  string `json:"updatedBy" gorm:"size:100;not null"`
}

func (*PlatformCharacterCardTemplateModel) TableName() string {
	return "platform_character_card_templates"
}

func (m *PlatformCharacterCardTemplateModel) Normalize() {
	m.Name = strings.TrimSpace(m.Name)
	m.SheetType = strings.TrimSpace(m.SheetType)
	m.Content = strings.TrimSpace(m.Content)
	m.BadgeTemplateOverride = strings.TrimSpace(m.BadgeTemplateOverride)
	m.TheaterOverlayTemplateJSON = strings.TrimSpace(m.TheaterOverlayTemplateJSON)
	m.CreatedBy = strings.TrimSpace(m.CreatedBy)
	m.UpdatedBy = strings.TrimSpace(m.UpdatedBy)
}

func (m *PlatformCharacterCardTemplateModel) BeforeSave(tx *gorm.DB) error {
	m.Normalize()
	return nil
}

func PlatformCharacterCardTemplateList(sheetType string, includeDisabled bool) ([]*PlatformCharacterCardTemplateModel, error) {
	var items []*PlatformCharacterCardTemplateModel
	query := db.Order("updated_at desc")
	if trimmed := strings.TrimSpace(sheetType); trimmed != "" {
		query = query.Where("sheet_type = ?", trimmed)
	}
	if !includeDisabled {
		query = query.Where("enabled = ?", true)
	}
	if err := query.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func PlatformCharacterCardTemplateGetByID(id string) (*PlatformCharacterCardTemplateModel, error) {
	item := &PlatformCharacterCardTemplateModel{}
	if err := db.Where("id = ?", strings.TrimSpace(id)).Take(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func PlatformCharacterCardTemplateCreate(item *PlatformCharacterCardTemplateModel) error {
	if item == nil {
		return gorm.ErrInvalidData
	}
	item.Normalize()
	if item.ID == "" {
		item.Init()
	} else {
		if item.CreatedAt.IsZero() {
			item.CreatedAt = time.Now()
		}
		if item.UpdatedAt.IsZero() {
			item.UpdatedAt = item.CreatedAt
		}
	}
	// Map create explicitly includes enabled=false; GORM struct create skips zero values with default tags.
	return db.Model(&PlatformCharacterCardTemplateModel{}).Create(map[string]any{
		"id": item.ID, "created_at": item.CreatedAt, "updated_at": item.UpdatedAt, "deleted_at": item.DeletedAt,
		"name": item.Name, "sheet_type": item.SheetType, "content": item.Content,
		"badge_template_override":       item.BadgeTemplateOverride,
		"theater_overlay_template_json": item.TheaterOverlayTemplateJSON,
		"enabled":                       item.Enabled, "created_by": item.CreatedBy, "updated_by": item.UpdatedBy,
	}).Error
}

func PlatformCharacterCardTemplateUpdate(id string, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	return db.Model(&PlatformCharacterCardTemplateModel{}).Where("id = ?", strings.TrimSpace(id)).Updates(values).Error
}

func PlatformCharacterCardTemplateDelete(id string) error {
	return db.Where("id = ?", strings.TrimSpace(id)).Delete(&PlatformCharacterCardTemplateModel{}).Error
}
