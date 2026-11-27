package service

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"sealchat/model"
)

// LogBookmark is a private or shared jump target, independent of drafts and releases.
type LogBookmark struct {
	ID         string    `json:"id"`
	SectionID  string    `json:"sectionId"`
	NodeID     string    `json:"nodeId"`
	Label      string    `json:"label"`
	CreatedAt  time.Time `json:"createdAt"`
	CreatorID  string    `json:"creatorId"`
	Visibility string    `json:"visibility"`
	CanManage  bool      `json:"canManage"`
}

type LogBookmarkCreate struct {
	SectionID  string `json:"sectionId"`
	NodeID     string `json:"nodeId"`
	Label      string `json:"label"`
	Visibility string `json:"visibility"`
}

type LogBookmarkUpdate struct {
	Label string `json:"label"`
}

const (
	logMaxBookmarksPerUser = 100
	logMaxBookmarkLabel    = 60
	LogBookmarkPrivate     = "private"
	LogBookmarkShared      = "shared"
)

// CheckLogProjectDesigner reports whether the user may open the project's design workbench.
func CheckLogProjectDesigner(projectID, userID string) error {
	_, _, err := loadLogProjectForDesign(projectID, userID)
	return err
}

func logBookmarkCanManage(m *model.LogBookmarkModel, userID string, admin bool) bool {
	return m.UserID == userID || (m.Visibility == LogBookmarkShared && admin)
}

func logBookmarkView(m *model.LogBookmarkModel, userID string, admin bool) LogBookmark {
	visibility := LogBookmarkPrivate
	if m.Visibility == LogBookmarkShared {
		visibility = LogBookmarkShared
	}
	return LogBookmark{ID: m.ID, SectionID: m.SectionID, NodeID: m.NodeID, Label: m.Label, CreatedAt: m.CreatedAt,
		CreatorID: m.UserID, Visibility: visibility, CanManage: logBookmarkCanManage(m, userID, admin)}
}

func normalizeLogBookmarkLabel(label string) (string, error) {
	label = strings.TrimSpace(label)
	if label == "" || !utf8.ValidString(label) || utf8.RuneCountInString(label) > logMaxBookmarkLabel || strings.ContainsAny(label, "\r\n") {
		return "", logError("INVALID_INPUT", "书签名称需为 1-60 个字符")
	}
	return label, nil
}

func ListLogBookmarks(projectID, userID string) ([]LogBookmark, error) {
	_, admin, err := loadLogProjectForDesign(projectID, userID)
	if err != nil {
		return nil, err
	}
	var rows []model.LogBookmarkModel
	if err := visibleLogBookmarks(model.GetDB(), projectID, userID).Order("created_at ASC").Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]LogBookmark, 0, len(rows))
	for i := range rows {
		// Some database collations compare enum strings case-insensitively.
		if rows[i].UserID == userID || rows[i].Visibility == LogBookmarkShared {
			out = append(out, logBookmarkView(&rows[i], userID, admin))
		}
	}
	return out, nil
}

func CreateLogBookmark(projectID, userID string, input LogBookmarkCreate) (*LogBookmark, error) {
	p, admin, err := loadLogProjectForDesign(projectID, userID)
	if err != nil {
		return nil, err
	}
	visibility := input.Visibility
	if visibility == "" {
		visibility = LogBookmarkPrivate
	}
	if visibility != LogBookmarkPrivate && visibility != LogBookmarkShared {
		return nil, logError("INVALID_INPUT", "书签可见范围无效")
	}
	label, err := normalizeLogBookmarkLabel(input.Label)
	if err != nil {
		return nil, err
	}
	nodeID := strings.TrimSpace(input.NodeID)
	if nodeID == "" || nodeID != input.NodeID || len(nodeID) > 100 {
		return nil, logError("INVALID_INPUT", "书签位置无效")
	}
	var draft struct {
		Sections []struct {
			ID string `json:"id"`
		} `json:"sections"`
	}
	if err := json.Unmarshal([]byte(p.DraftJSON), &draft); err != nil {
		return nil, err
	}
	found := false
	for _, s := range draft.Sections {
		if s.ID == input.SectionID {
			found = true
			break
		}
	}
	if !found {
		return nil, logError("INVALID_INPUT", "书签所在的来源片段不存在")
	}
	row := model.LogBookmarkModel{ProjectID: projectID, UserID: userID, SectionID: input.SectionID, NodeID: nodeID, Label: label, Visibility: visibility}
	row.Init()
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.LogBookmarkModel{}).Where("project_id = ? AND user_id = ?", projectID, userID).Count(&count).Error; err != nil {
			return err
		}
		if count >= logMaxBookmarksPerUser {
			return logError("INVALID_INPUT", "每人在每个 LOG 最多创建 100 个书签")
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return nil, err
	}
	view := logBookmarkView(&row, userID, admin)
	return &view, nil
}

// Only the explicit shared value broadens visibility; absent/unknown values stay private.
func visibleLogBookmarks(tx *gorm.DB, projectID, userID string) *gorm.DB {
	return tx.Where("project_id = ? AND (user_id = ? OR visibility = ?)", projectID, userID, LogBookmarkShared)
}

func loadVisibleLogBookmark(tx *gorm.DB, projectID, userID, bookmarkID string) (*model.LogBookmarkModel, error) {
	var row model.LogBookmarkModel
	if err := visibleLogBookmarks(tx, projectID, userID).Where("id = ?", bookmarkID).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, logError("NOT_FOUND", "书签不存在")
		}
		return nil, err
	}
	if row.UserID != userID && row.Visibility != LogBookmarkShared {
		return nil, logError("NOT_FOUND", "书签不存在")
	}
	return &row, nil
}

func GetLogBookmark(projectID, userID, bookmarkID string) (*LogBookmark, error) {
	_, admin, err := loadLogProjectForDesign(projectID, userID)
	if err != nil {
		return nil, err
	}
	row, err := loadVisibleLogBookmark(model.GetDB(), projectID, userID, bookmarkID)
	if err != nil {
		return nil, err
	}
	view := logBookmarkView(row, userID, admin)
	return &view, nil
}

func UpdateLogBookmark(projectID, userID, bookmarkID string, input LogBookmarkUpdate) (*LogBookmark, error) {
	_, admin, err := loadLogProjectForDesign(projectID, userID)
	if err != nil {
		return nil, err
	}
	label, err := normalizeLogBookmarkLabel(input.Label)
	if err != nil {
		return nil, err
	}
	db := model.GetDB()
	row, err := loadVisibleLogBookmark(db, projectID, userID, bookmarkID)
	if err != nil {
		return nil, err
	}
	if !logBookmarkCanManage(row, userID, admin) {
		return nil, logError("PERMISSION_DENIED", "仅创建者或世界管理者可以修改共享书签")
	}
	now := time.Now()
	// Visibility and creator are immutable; renaming never changes either field.
	r := db.Model(&model.LogBookmarkModel{}).Where("id = ? AND project_id = ? AND user_id = ? AND visibility = ?", row.ID, projectID, row.UserID, row.Visibility).Updates(map[string]any{"label": label, "updated_at": now})
	if r.Error != nil {
		return nil, r.Error
	}
	if r.RowsAffected != 1 {
		return nil, logError("NOT_FOUND", "书签不存在")
	}
	row.Label = label
	row.UpdatedAt = now
	view := logBookmarkView(row, userID, admin)
	return &view, nil
}

func DeleteLogBookmark(projectID, userID, bookmarkID string) error {
	_, admin, err := loadLogProjectForDesign(projectID, userID)
	if err != nil {
		return err
	}
	row, err := loadVisibleLogBookmark(model.GetDB(), projectID, userID, bookmarkID)
	if err != nil {
		return err
	}
	if !logBookmarkCanManage(row, userID, admin) {
		return logError("PERMISSION_DENIED", "仅创建者或世界管理者可以删除共享书签")
	}
	r := model.GetDB().Where("id = ? AND project_id = ? AND user_id = ? AND visibility = ?", row.ID, projectID, row.UserID, row.Visibility).Delete(&model.LogBookmarkModel{})
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return logError("NOT_FOUND", "书签不存在")
	}
	return nil
}
