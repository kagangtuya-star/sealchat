package service

import (
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/utils"
)

const worldClueEditingLockTTL = 10 * time.Second

var worldClueEditLockFields = map[string]struct{}{
	"title":       {},
	"content":     {},
	"embedUrl":    {},
	"imageUrl":    {},
	"managerNote": {},
}

func worldClueEditLockDTO(row *model.WorldClueEditLockModel, user *model.UserModel) *protocol.WorldClueEditingLock {
	if row == nil {
		return nil
	}
	result := &protocol.WorldClueEditingLock{
		Field: row.Field, UserID: row.UserID, SessionID: row.SessionID, ExpireAt: row.ExpireAt.UnixMilli(),
	}
	if user != nil {
		result.User = user.ToProtocolType()
	}
	return result
}

func loadWorldClueEditLockUser(db *gorm.DB, userID string) (*model.UserModel, error) {
	var user model.UserModel
	if err := db.Where("id = ?", userID).Limit(1).Find(&user).Error; err != nil {
		return nil, err
	}
	if user.ID == "" {
		return nil, nil
	}
	return &user, nil
}

// validateWorldClueEditLockField checks both the field whitelist and the
// server-side world/clue permissions. It never trusts a client supplied role.
func validateWorldClueEditLockField(db *gorm.DB, worldID, clueID, actorID, field string) error {
	field = strings.TrimSpace(field)
	if field == "" {
		return ErrWorldClueInvalid
	}
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return err
	}
	if role == "" {
		return ErrWorldClueDenied
	}
	var clue model.WorldClueModel
	if err := db.Where("world_id = ? AND id = ? AND status <> ?", worldID, clueID, model.WorldClueStatusArchived).Limit(1).Find(&clue).Error; err != nil {
		return err
	}
	if clue.ID == "" {
		return ErrWorldClueNotFound
	}

	if strings.HasPrefix(field, "private:") {
		if !worldClueIsAdminRole(role) {
			return ErrWorldClueDenied
		}
		rawTargetID := strings.TrimPrefix(field, "private:")
		targetID := strings.TrimSpace(rawTargetID)
		if targetID == "" || targetID != rawTargetID || strings.ContainsAny(targetID, ": \t\r\n") {
			return ErrWorldClueInvalid
		}
		targetRole, err := worldClueRole(db, worldID, targetID)
		if err != nil {
			return err
		}
		if targetRole == "" {
			return ErrWorldClueInvalid
		}
		return nil
	}
	if _, ok := worldClueEditLockFields[field]; !ok {
		return ErrWorldClueInvalid
	}
	if field == "managerNote" {
		if !worldClueIsAdminRole(role) {
			return ErrWorldClueDenied
		}
		return nil
	}
	accessRow, err := getWorldClueAccessRow(db, worldID, clueID, actorID)
	if err != nil {
		return err
	}
	override := model.WorldClueAccessInherit
	if accessRow != nil {
		override = accessRow.AccessOverride
	}
	if !worldClueIsAdminRole(role) && effectiveWorldClueAccess(role, clue.DefaultAccess, override) != model.WorldClueAccessEdit {
		return ErrWorldClueDenied
	}
	return nil
}

func WorldClueListEditLocks(worldID, clueID, actorID string) ([]protocol.WorldClueEditingLock, error) {
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return nil, err
	}
	if role == "" {
		return nil, ErrWorldClueDenied
	}
	var clue model.WorldClueModel
	if err := db.Where("world_id = ? AND id = ? AND status <> ?", worldID, clueID, model.WorldClueStatusArchived).Limit(1).Find(&clue).Error; err != nil {
		return nil, err
	}
	if clue.ID == "" {
		return nil, ErrWorldClueNotFound
	}
	accessRow, err := getWorldClueAccessRow(db, worldID, clueID, actorID)
	if err != nil {
		return nil, err
	}
	override := model.WorldClueAccessInherit
	if accessRow != nil {
		override = accessRow.AccessOverride
	}
	if !worldClueIsAdminRole(role) && effectiveWorldClueAccess(role, clue.DefaultAccess, override) == model.WorldClueAccessNone {
		return nil, ErrWorldClueDenied
	}

	now := time.Now()
	var rows []model.WorldClueEditLockModel
	if err := db.Where("world_id = ? AND clue_id = ? AND expire_at > ?", worldID, clueID, now).
		Order("field ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	userIDs := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	filtered := make([]model.WorldClueEditLockModel, 0, len(rows))
	for _, row := range rows {
		if row.Field == "managerNote" || strings.HasPrefix(row.Field, "private:") {
			if !worldClueIsAdminRole(role) {
				continue
			}
		}
		filtered = append(filtered, row)
		if _, ok := seen[row.UserID]; !ok {
			seen[row.UserID] = struct{}{}
			userIDs = append(userIDs, row.UserID)
		}
	}
	users := make(map[string]*model.UserModel, len(userIDs))
	if len(userIDs) > 0 {
		var userRows []model.UserModel
		if err := db.Where("id IN ?", userIDs).Find(&userRows).Error; err != nil {
			return nil, err
		}
		for i := range userRows {
			users[userRows[i].ID] = &userRows[i]
		}
	}
	result := make([]protocol.WorldClueEditingLock, 0, len(filtered))
	for i := range filtered {
		if lock := worldClueEditLockDTO(&filtered[i], users[filtered[i].UserID]); lock != nil {
			result = append(result, *lock)
		}
	}
	return result, nil
}

func WorldClueAcquireEditLock(worldID, clueID, actorID, field, sessionID string) (*protocol.WorldClueEditingLock, bool, error) {
	db := model.GetDB()
	field, sessionID = strings.TrimSpace(field), strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, false, ErrWorldClueInvalid
	}
	if err := validateWorldClueEditLockField(db, worldID, clueID, actorID, field); err != nil {
		return nil, false, err
	}
	now := time.Now()
	expireAt := now.Add(worldClueEditingLockTTL)
	result := db.Model(&model.WorldClueEditLockModel{}).
		Where("world_id = ? AND clue_id = ? AND field = ?", worldID, clueID, field).
		Where("expire_at <= ? OR (user_id = ? AND session_id = ?)", now, actorID, sessionID).
		Updates(map[string]any{"user_id": actorID, "session_id": sessionID, "expire_at": expireAt, "updated_at": now})
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 1 {
		var row model.WorldClueEditLockModel
		if err := db.Where("world_id = ? AND clue_id = ? AND field = ?", worldID, clueID, field).Limit(1).Find(&row).Error; err != nil {
			return nil, false, err
		}
		user, err := loadWorldClueEditLockUser(db, row.UserID)
		return worldClueEditLockDTO(&row, user), true, err
	}

	row := model.WorldClueEditLockModel{WorldID: worldID, ClueID: clueID, Field: field, UserID: actorID, SessionID: sessionID, ExpireAt: expireAt}
	row.ID = utils.NewID()
	insert := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "world_id"}, {Name: "clue_id"}, {Name: "field"}},
		DoNothing: true,
	}).Create(&row)
	if insert.Error != nil {
		return nil, false, insert.Error
	}
	if insert.RowsAffected == 1 {
		user, err := loadWorldClueEditLockUser(db, actorID)
		return worldClueEditLockDTO(&row, user), true, err
	}

	var existing model.WorldClueEditLockModel
	if err := db.Where("world_id = ? AND clue_id = ? AND field = ?", worldID, clueID, field).Limit(1).Find(&existing).Error; err != nil {
		return nil, false, err
	}
	if existing.ID == "" {
		return nil, false, nil
	}
	user, err := loadWorldClueEditLockUser(db, existing.UserID)
	return worldClueEditLockDTO(&existing, user), false, err
}

func WorldClueReleaseEditLock(worldID, clueID, actorID, field, sessionID string) (bool, error) {
	db := model.GetDB()
	field, sessionID = strings.TrimSpace(field), strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, ErrWorldClueInvalid
	}
	if err := validateWorldClueEditLockField(db, worldID, clueID, actorID, field); err != nil {
		return false, err
	}
	result := db.Unscoped().Where("world_id = ? AND clue_id = ? AND field = ? AND user_id = ? AND session_id = ?", worldID, clueID, field, actorID, sessionID).
		Delete(&model.WorldClueEditLockModel{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}
