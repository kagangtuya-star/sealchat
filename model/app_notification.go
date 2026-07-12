package model

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/utils"
)

const (
	appNotificationInstanceRowID = "main"
	appNotificationTokenTTL      = 90 * 24 * time.Hour
)

var ErrAppNotificationDeviceTokenInvalid = errors.New("app notification device token invalid")

type AppNotificationInstanceModel struct {
	ID         string `gorm:"primaryKey;size:32"`
	InstanceID string `gorm:"size:100;not null;uniqueIndex"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (*AppNotificationInstanceModel) TableName() string {
	return "app_notification_instance"
}

type AppNotificationDeviceModel struct {
	StringPKBaseModel
	UserID          string     `json:"user_id" gorm:"size:100;not null;uniqueIndex:idx_app_notification_user_installation,priority:1;index"`
	InstallationID  string     `json:"installation_id" gorm:"size:160;not null;uniqueIndex:idx_app_notification_user_installation,priority:2"`
	TokenHash       string     `json:"-" gorm:"size:64;not null;uniqueIndex"`
	TokenExpiresAt  time.Time  `json:"token_expires_at" gorm:"not null;index"`
	ActiveWorldID   string     `json:"active_world_id" gorm:"size:100;index"`
	LastSequence    uint64     `json:"last_sequence" gorm:"not null;default:0"`
	LastConnectedAt *time.Time `json:"last_connected_at"`
	RevokedAt       *time.Time `json:"revoked_at" gorm:"index"`
	Name            string     `json:"name" gorm:"size:128"`
	Platform        string     `json:"platform" gorm:"size:32"`
	AppVersion      string     `json:"app_version" gorm:"size:64"`
	AppBuild        int        `json:"app_build"`
	OSVersion       string     `json:"os_version" gorm:"size:64"`
	Locale          string     `json:"locale" gorm:"size:32"`
}

func (*AppNotificationDeviceModel) TableName() string {
	return "app_notification_devices"
}

// AppNotificationPreferenceModel stores user-level notification routing preferences.
type AppNotificationPreferenceModel struct {
	UserID                string    `json:"user_id" gorm:"primaryKey;size:100"`
	WorldWhitelistEnabled bool      `json:"world_whitelist_enabled" gorm:"not null;default:false"`
	WorldWhitelistJSON    string    `json:"-" gorm:"type:text"`
	ServerChanEnabled     bool      `json:"server_chan_enabled" gorm:"not null;default:false"`
	ServerChanSendKey     string    `json:"-" gorm:"size:256"`
	MeowEnabled           bool      `json:"meow_enabled" gorm:"not null;default:false"`
	MeowNickname          string    `json:"-" gorm:"size:256"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

func (*AppNotificationPreferenceModel) TableName() string {
	return "app_notification_preferences"
}

type AppNotificationDeviceInput struct {
	InstallationID string
	Name           string
	Platform       string
	AppVersion     string
	AppBuild       int
	OSVersion      string
	Locale         string
}

func EnsureAppNotificationInstanceID() (string, error) {
	var instance AppNotificationInstanceModel
	err := db.Where("id = ?", appNotificationInstanceRowID).First(&instance).Error
	if err == nil {
		return instance.InstanceID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	instance = AppNotificationInstanceModel{
		ID:         appNotificationInstanceRowID,
		InstanceID: "sc_" + utils.NewID(),
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&instance).Error; err != nil {
		return "", err
	}
	if err := db.Where("id = ?", appNotificationInstanceRowID).First(&instance).Error; err != nil {
		return "", err
	}
	return instance.InstanceID, nil
}

func UpsertAppNotificationDevice(userID string, input AppNotificationDeviceInput) (*AppNotificationDeviceModel, string, error) {
	userID = strings.TrimSpace(userID)
	input.InstallationID = strings.TrimSpace(input.InstallationID)
	if userID == "" || input.InstallationID == "" {
		return nil, "", errors.New("user_id and installation_id are required")
	}
	rawToken, tokenHash, err := newAppNotificationDeviceToken()
	if err != nil {
		return nil, "", err
	}

	var device AppNotificationDeviceModel
	err = db.Where("user_id = ? AND installation_id = ?", userID, input.InstallationID).First(&device).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		device = AppNotificationDeviceModel{UserID: userID, InstallationID: input.InstallationID}
		device.Init()
	} else if err != nil {
		return nil, "", err
	}

	now := time.Now().UTC()
	device.TokenHash = tokenHash
	device.TokenExpiresAt = now.Add(appNotificationTokenTTL)
	device.RevokedAt = nil
	device.Name = strings.TrimSpace(input.Name)
	device.Platform = strings.TrimSpace(input.Platform)
	device.AppVersion = strings.TrimSpace(input.AppVersion)
	device.AppBuild = input.AppBuild
	device.OSVersion = strings.TrimSpace(input.OSVersion)
	device.Locale = strings.TrimSpace(input.Locale)
	if err := db.Save(&device).Error; err != nil {
		return nil, "", err
	}
	return &device, rawToken, nil
}

func VerifyAppNotificationDeviceToken(deviceID, rawToken string) (*AppNotificationDeviceModel, error) {
	var device AppNotificationDeviceModel
	if err := db.Where("id = ? AND revoked_at IS NULL", strings.TrimSpace(deviceID)).First(&device).Error; err != nil {
		return nil, ErrAppNotificationDeviceTokenInvalid
	}
	if !device.TokenExpiresAt.After(time.Now()) {
		return nil, ErrAppNotificationDeviceTokenInvalid
	}
	want, err := hex.DecodeString(device.TokenHash)
	if err != nil {
		return nil, ErrAppNotificationDeviceTokenInvalid
	}
	got := sha256.Sum256([]byte(strings.TrimSpace(rawToken)))
	if len(want) != len(got) || subtle.ConstantTimeCompare(want, got[:]) != 1 {
		return nil, ErrAppNotificationDeviceTokenInvalid
	}
	return &device, nil
}

func AdvanceAppNotificationSequence(deviceID string) (uint64, error) {
	var next uint64
	err := db.Transaction(func(tx *gorm.DB) error {
		var device AppNotificationDeviceModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", strings.TrimSpace(deviceID)).First(&device).Error; err != nil {
			return err
		}
		next = device.LastSequence + 1
		return tx.Model(&device).UpdateColumn("last_sequence", next).Error
	})
	return next, err
}

func GetAppNotificationDevice(deviceID string) (*AppNotificationDeviceModel, error) {
	var device AppNotificationDeviceModel
	if err := db.Where("id = ?", strings.TrimSpace(deviceID)).First(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

func ListActiveAppNotificationDevicesByWorld(worldID string) ([]AppNotificationDeviceModel, error) {
	var devices []AppNotificationDeviceModel
	err := db.Where("active_world_id = ? AND revoked_at IS NULL AND token_expires_at > ?", strings.TrimSpace(worldID), time.Now()).Find(&devices).Error
	return devices, err
}

func ListActiveAppNotificationDevices() ([]AppNotificationDeviceModel, error) {
	var devices []AppNotificationDeviceModel
	err := db.Where("revoked_at IS NULL AND token_expires_at > ?", time.Now()).Find(&devices).Error
	return devices, err
}

func GetAppNotificationPreference(userID string) (*AppNotificationPreferenceModel, error) {
	preference := &AppNotificationPreferenceModel{UserID: strings.TrimSpace(userID)}
	if preference.UserID == "" {
		return preference, nil
	}
	err := db.Where("user_id = ?", preference.UserID).First(preference).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return preference, nil
	}
	if err != nil {
		return nil, err
	}
	return preference, nil
}

func GetAppNotificationPreferences(userIDs []string) (map[string]*AppNotificationPreferenceModel, error) {
	uniqueIDs := make([]string, 0, len(userIDs))
	seen := make(map[string]struct{}, len(userIDs))
	for _, userID := range userIDs {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		uniqueIDs = append(uniqueIDs, userID)
	}
	preferences := make(map[string]*AppNotificationPreferenceModel, len(uniqueIDs))
	if len(uniqueIDs) == 0 {
		return preferences, nil
	}
	var rows []AppNotificationPreferenceModel
	if err := db.Where("user_id IN ?", uniqueIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for index := range rows {
		preference := rows[index]
		preferences[preference.UserID] = &preference
	}
	return preferences, nil
}

func UpsertAppNotificationPreference(userID string, worldWhitelistEnabled bool, worldWhitelistJSON string, serverChanEnabled bool, serverChanSendKey string, meowEnabled bool, meowNickname string) (*AppNotificationPreferenceModel, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, errors.New("user_id is required")
	}
	preference := AppNotificationPreferenceModel{
		UserID:                userID,
		WorldWhitelistEnabled: worldWhitelistEnabled,
		WorldWhitelistJSON:    strings.TrimSpace(worldWhitelistJSON),
		ServerChanEnabled:     serverChanEnabled,
		ServerChanSendKey:     strings.TrimSpace(serverChanSendKey),
		MeowEnabled:           meowEnabled,
		MeowNickname:          strings.TrimSpace(meowNickname),
	}
	if preference.ServerChanSendKey == "" || preference.MeowNickname == "" {
		if existing, err := GetAppNotificationPreference(userID); err != nil {
			return nil, err
		} else {
			if preference.ServerChanSendKey == "" {
				preference.ServerChanSendKey = existing.ServerChanSendKey
			}
			if preference.MeowNickname == "" {
				preference.MeowNickname = existing.MeowNickname
			}
		}
	}
	if preference.ServerChanEnabled && preference.ServerChanSendKey == "" {
		return nil, errors.New("server chan send key is required")
	}
	if preference.MeowEnabled && preference.MeowNickname == "" {
		return nil, errors.New("MeoW nickname is required")
	}
	if err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"world_whitelist_enabled": preference.WorldWhitelistEnabled,
			"world_whitelist_json":    preference.WorldWhitelistJSON,
			"server_chan_enabled":     preference.ServerChanEnabled,
			"server_chan_send_key":    preference.ServerChanSendKey,
			"meow_enabled":            preference.MeowEnabled,
			"meow_nickname":           preference.MeowNickname,
			"updated_at":              time.Now().UTC(),
		}),
	}).Create(&preference).Error; err != nil {
		return nil, err
	}
	return GetAppNotificationPreference(userID)
}

func ListServerChanAppNotificationPreferences() ([]AppNotificationPreferenceModel, error) {
	var preferences []AppNotificationPreferenceModel
	err := db.Where("world_whitelist_enabled = ? AND server_chan_enabled = ? AND server_chan_send_key <> ?", true, true, "").Find(&preferences).Error
	return preferences, err
}

func ListMeowAppNotificationPreferences() ([]AppNotificationPreferenceModel, error) {
	var preferences []AppNotificationPreferenceModel
	err := db.Where("world_whitelist_enabled = ? AND meow_enabled = ? AND meow_nickname <> ?", true, true, "").Find(&preferences).Error
	return preferences, err
}

func UpdateAppNotificationDeviceWorld(deviceID, worldID string) (*AppNotificationDeviceModel, error) {
	deviceID = strings.TrimSpace(deviceID)
	worldID = strings.TrimSpace(worldID)
	if err := db.Model(&AppNotificationDeviceModel{}).Where("id = ? AND revoked_at IS NULL", deviceID).Update("active_world_id", worldID).Error; err != nil {
		return nil, err
	}
	return GetAppNotificationDevice(deviceID)
}

func MarkAppNotificationDeviceConnected(deviceID string, at time.Time) error {
	return db.Model(&AppNotificationDeviceModel{}).Where("id = ? AND revoked_at IS NULL", strings.TrimSpace(deviceID)).Update("last_connected_at", at.UTC()).Error
}

func RevokeAppNotificationDevice(deviceID string) error {
	now := time.Now().UTC()
	return db.Model(&AppNotificationDeviceModel{}).Where("id = ?", strings.TrimSpace(deviceID)).Updates(map[string]any{
		"active_world_id": "",
		"revoked_at":      &now,
	}).Error
}

func newAppNotificationDeviceToken() (string, string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", "", err
	}
	raw := "scnt_" + base64.RawURLEncoding.EncodeToString(random)
	hash := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(hash[:]), nil
}
