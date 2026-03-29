package model

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/utils"
)

// UserPreferenceModel 用户偏好表
type UserPreferenceModel struct {
	StringPKBaseModel
	UserID    string `json:"userId" gorm:"index:idx_user_pref_user_key,unique;not null"`
	PrefKey   string `json:"key" gorm:"column:pref_key;size:64;index:idx_user_pref_user_key,unique;not null"`
	PrefValue string `json:"value" gorm:"column:pref_value;size:4096;not null"`
}

func (*UserPreferenceModel) TableName() string {
	return "user_preferences"
}

func userPreferenceGetWithDB(conn *gorm.DB, userID, key string) (*UserPreferenceModel, error) {
	var record UserPreferenceModel
	err := conn.Where("user_id = ? AND pref_key = ?", userID, key).Limit(1).Find(&record).Error
	if err != nil {
		return nil, err
	}
	if record.ID == "" {
		return nil, nil
	}
	return &record, nil
}

// UserPreferenceGet 获取用户偏好
func UserPreferenceGet(userID, key string) (*UserPreferenceModel, error) {
	return userPreferenceGetWithDB(db, userID, key)
}

// UserPreferenceListByPrefix 按 key 前缀获取用户偏好
func UserPreferenceListByPrefix(userID, prefix string) ([]UserPreferenceModel, error) {
	userID = strings.TrimSpace(userID)
	prefix = strings.TrimSpace(prefix)
	if userID == "" || prefix == "" {
		return []UserPreferenceModel{}, nil
	}
	var items []UserPreferenceModel
	if err := db.Where("user_id = ? AND pref_key LIKE ?", userID, prefix+"%").
		Order("pref_key ASC").
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// UserPreferenceUpsert 创建或更新用户偏好
func UserPreferenceUpsert(userID, key, value string) (*UserPreferenceModel, error) {
	return UserPreferenceUpsertTx(db, userID, key, value)
}

// UserPreferenceUpsertTx 创建或更新用户偏好（事务版）
func UserPreferenceUpsertTx(conn *gorm.DB, userID, key, value string) (*UserPreferenceModel, error) {
	record := &UserPreferenceModel{
		StringPKBaseModel: StringPKBaseModel{ID: utils.NewID()},
		UserID:            userID,
		PrefKey:           key,
		PrefValue:         value,
	}

	err := conn.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "pref_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"pref_value", "updated_at"}),
	}).Create(record).Error
	if err != nil {
		return nil, err
	}
	// 直接返回请求值，避免读偏差
	record.PrefKey = key
	record.PrefValue = value
	return record, nil
}

// UserPreferenceDelete 删除用户偏好
func UserPreferenceDelete(userID, key string) error {
	return UserPreferenceDeleteTx(db, userID, key)
}

// UserPreferenceDeleteTx 删除用户偏好（事务版）
func UserPreferenceDeleteTx(conn *gorm.DB, userID, key string) error {
	userID = strings.TrimSpace(userID)
	key = strings.TrimSpace(key)
	if userID == "" || key == "" {
		return nil
	}
	return conn.Where("user_id = ? AND pref_key = ?", userID, key).Delete(&UserPreferenceModel{}).Error
}
