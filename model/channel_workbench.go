package model

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

const ChannelDisplayModeWorkbench = "workbench"

// ChannelWorkbenchModel 保存布局频道的两类共享状态：
// LayoutJSON 是管理员在设计模式中发布的长期布局定义；
// PresentationJSON 是主持人演示时需要同步给观看者的共享状态（揭示/隐藏、共享聚焦）。
// 两者各自维护 revision，个人的折叠/弹出等展示状态只保存在浏览器本地。
type ChannelWorkbenchModel struct {
	ChannelID            string    `json:"channelId" gorm:"primaryKey;size:100"`
	WorldID              string    `json:"worldId" gorm:"size:100;index"`
	SchemaVersion        int       `json:"schemaVersion" gorm:"not null;default:1"`
	LayoutJSON           string    `json:"-" gorm:"type:text"`
	LayoutRevision       uint64    `json:"layoutRevision" gorm:"not null;default:0"`
	PresentationJSON     string    `json:"-" gorm:"type:text"`
	PresentationRevision uint64    `json:"presentationRevision" gorm:"not null;default:0"`
	UpdatedBy            string    `json:"updatedBy" gorm:"size:100"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

func (*ChannelWorkbenchModel) TableName() string {
	return "channel_workbenches"
}

// ChannelWorkbenchGet returns nil when the channel has never saved a layout.
func ChannelWorkbenchGet(conn *gorm.DB, channelID string) (*ChannelWorkbenchModel, error) {
	if conn == nil {
		conn = db
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, nil
	}
	var record ChannelWorkbenchModel
	if err := conn.Where("channel_id = ?", channelID).Limit(1).Find(&record).Error; err != nil {
		return nil, err
	}
	if record.ChannelID == "" {
		return nil, nil
	}
	return &record, nil
}

// ChannelWorkbenchLayoutCompareAndSet writes a new layout only when the stored
// layout revision still equals expectedRevision. A missing row is created only
// for expectedRevision 0; a concurrent create surfaces as applied=false.
func ChannelWorkbenchLayoutCompareAndSet(conn *gorm.DB, channelID, worldID string, schemaVersion int, layoutJSON string, expectedRevision uint64, updatedBy string) (applied bool, err error) {
	if conn == nil {
		conn = db
	}
	now := time.Now()
	if expectedRevision == 0 {
		existing, err := ChannelWorkbenchGet(conn, channelID)
		if err != nil {
			return false, err
		}
		if existing == nil {
			record := &ChannelWorkbenchModel{
				ChannelID:      channelID,
				WorldID:        worldID,
				SchemaVersion:  schemaVersion,
				LayoutJSON:     layoutJSON,
				LayoutRevision: 1,
				UpdatedBy:      updatedBy,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if err := conn.Create(record).Error; err != nil {
				// Another writer created the row first; let the caller report a conflict.
				if again, getErr := ChannelWorkbenchGet(conn, channelID); getErr == nil && again != nil {
					return false, nil
				}
				return false, err
			}
			return true, nil
		}
	}
	result := conn.Model(&ChannelWorkbenchModel{}).
		Where("channel_id = ? AND layout_revision = ?", channelID, expectedRevision).
		Updates(map[string]any{
			"world_id":        worldID,
			"schema_version":  schemaVersion,
			"layout_json":     layoutJSON,
			"layout_revision": gorm.Expr("layout_revision + 1"),
			"updated_by":      updatedBy,
			"updated_at":      now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// ChannelWorkbenchPresentationCompareAndSet updates only the shared presentation state.
func ChannelWorkbenchPresentationCompareAndSet(conn *gorm.DB, channelID, presentationJSON string, expectedRevision uint64, updatedBy string) (bool, error) {
	if conn == nil {
		conn = db
	}
	result := conn.Model(&ChannelWorkbenchModel{}).
		Where("channel_id = ? AND presentation_revision = ?", channelID, expectedRevision).
		Updates(map[string]any{
			"presentation_json":     presentationJSON,
			"presentation_revision": gorm.Expr("presentation_revision + 1"),
			"updated_by":            updatedBy,
			"updated_at":            time.Now(),
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func ChannelDisplayModeSet(channelID, displayMode string) error {
	return db.Model(&ChannelModel{}).
		Where("id = ?", channelID).
		Updates(map[string]any{"display_mode": displayMode}).Error
}
