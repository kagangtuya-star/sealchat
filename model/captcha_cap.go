package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sealchat/utils"
)

type CaptchaCapChallengeSeed struct {
	C int `json:"c"`
	S int `json:"s"`
	D int `json:"d"`
}

type CaptchaCapChallengeResponse struct {
	Challenge CaptchaCapChallengeSeed `json:"challenge"`
	Token     string                  `json:"token"`
	Expires   int64                   `json:"expires"`
}

type CaptchaCapRedeemResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Token   string `json:"token,omitempty"`
	Expires int64  `json:"expires,omitempty"`
}

type CaptchaCapChallengeModel struct {
	Token               string    `gorm:"primaryKey;size:128" json:"token"`
	Scene               string    `gorm:"size:32;index;not null" json:"scene"`
	ChallengeCount      int       `gorm:"not null" json:"challengeCount"`
	ChallengeSize       int       `gorm:"not null" json:"challengeSize"`
	ChallengeDifficulty int       `gorm:"not null" json:"challengeDifficulty"`
	TokenTTLSeconds     int       `gorm:"not null" json:"tokenTTLSeconds"`
	ExpiresAt           time.Time `gorm:"index;not null" json:"expiresAt"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

func (*CaptchaCapChallengeModel) TableName() string {
	return "captcha_cap_challenges"
}

type CaptchaCapTokenModel struct {
	Key       string    `gorm:"primaryKey;size:160" json:"key"`
	Scene     string    `gorm:"size:32;index;not null" json:"scene"`
	ExpiresAt time.Time `gorm:"index;not null" json:"expiresAt"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (*CaptchaCapTokenModel) TableName() string {
	return "captcha_cap_tokens"
}

func normalizeCaptchaCapRuntimeConfig(cfg utils.CaptchaCapConfig) utils.CaptchaCapConfig {
	if cfg.ChallengeCount <= 0 {
		cfg.ChallengeCount = 50
	}
	if cfg.ChallengeSize <= 0 {
		cfg.ChallengeSize = 32
	}
	if cfg.ChallengeDifficulty <= 0 {
		cfg.ChallengeDifficulty = 4
	}
	if cfg.ChallengeExpiresSeconds <= 0 {
		cfg.ChallengeExpiresSeconds = 600
	}
	if cfg.TokenTTLSeconds <= 0 {
		cfg.TokenTTLSeconds = 1200
	}
	return cfg
}

func captchaCapRandomHex(byteCount int) (string, error) {
	buf := make([]byte, byteCount)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func captchaCapSHA256(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

func captchaCapPRNG(seed string, length int) string {
	fnv1a := func(str string) uint32 {
		hash := uint32(2166136261)
		for i := 0; i < len(str); i++ {
			hash ^= uint32(str[i])
			hash += (hash << 1) + (hash << 4) + (hash << 7) + (hash << 8) + (hash << 24)
		}
		return hash
	}

	state := fnv1a(seed)
	result := ""
	next := func() uint32 {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		return state
	}

	for len(result) < length {
		result += fmt.Sprintf("%08x", next())
	}
	return result[:length]
}

func CaptchaCapCleanupExpired() error {
	if db == nil {
		return nil
	}
	now := time.Now()
	if err := db.Where("expires_at <= ?", now).Delete(&CaptchaCapChallengeModel{}).Error; err != nil {
		return err
	}
	return db.Where("expires_at <= ?", now).Delete(&CaptchaCapTokenModel{}).Error
}

func CaptchaCapCreateChallenge(scene utils.CaptchaScene, cfg utils.CaptchaCapConfig) (*CaptchaCapChallengeResponse, error) {
	if err := CaptchaCapCleanupExpired(); err != nil {
		return nil, err
	}

	cfg = normalizeCaptchaCapRuntimeConfig(cfg)
	token, err := captchaCapRandomHex(25)
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(time.Duration(cfg.ChallengeExpiresSeconds) * time.Second)
	record := &CaptchaCapChallengeModel{
		Token:               token,
		Scene:               string(scene),
		ChallengeCount:      cfg.ChallengeCount,
		ChallengeSize:       cfg.ChallengeSize,
		ChallengeDifficulty: cfg.ChallengeDifficulty,
		TokenTTLSeconds:     cfg.TokenTTLSeconds,
		ExpiresAt:           expiresAt,
	}
	if err := db.Create(record).Error; err != nil {
		return nil, err
	}

	return &CaptchaCapChallengeResponse{
		Challenge: CaptchaCapChallengeSeed{
			C: cfg.ChallengeCount,
			S: cfg.ChallengeSize,
			D: cfg.ChallengeDifficulty,
		},
		Token:   token,
		Expires: expiresAt.UnixMilli(),
	}, nil
}

func CaptchaCapRedeemChallenge(scene utils.CaptchaScene, token string, solutions []int64) (*CaptchaCapRedeemResponse, error) {
	if token == "" || len(solutions) == 0 {
		return &CaptchaCapRedeemResponse{Success: false, Message: "Invalid body"}, nil
	}
	if err := CaptchaCapCleanupExpired(); err != nil {
		return nil, err
	}

	var challenge CaptchaCapChallengeModel
	err := db.Where("token = ? AND scene = ?", token, string(scene)).First(&challenge).Error
	if err != nil {
		return &CaptchaCapRedeemResponse{Success: false, Message: "Challenge invalid or expired"}, nil
	}
	_ = db.Where("token = ?", token).Delete(&CaptchaCapChallengeModel{}).Error

	if !challenge.ExpiresAt.After(time.Now()) {
		return &CaptchaCapRedeemResponse{Success: false, Message: "Challenge invalid or expired"}, nil
	}

	if len(solutions) != challenge.ChallengeCount {
		return &CaptchaCapRedeemResponse{Success: false, Message: "Invalid solution"}, nil
	}

	for i := 1; i <= challenge.ChallengeCount; i++ {
		salt := captchaCapPRNG(fmt.Sprintf("%s%d", token, i), challenge.ChallengeSize)
		target := captchaCapPRNG(fmt.Sprintf("%s%dd", token, i), challenge.ChallengeDifficulty)
		hash := captchaCapSHA256(salt + strconv.FormatInt(solutions[i-1], 10))
		if !strings.HasPrefix(hash, target) {
			return &CaptchaCapRedeemResponse{Success: false, Message: "Invalid solution"}, nil
		}
	}

	verifyToken, err := captchaCapRandomHex(15)
	if err != nil {
		return nil, err
	}
	tokenID, err := captchaCapRandomHex(8)
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(time.Duration(challenge.TokenTTLSeconds) * time.Second)
	tokenKey := tokenID + ":" + captchaCapSHA256(verifyToken)
	record := &CaptchaCapTokenModel{
		Key:       tokenKey,
		Scene:     string(scene),
		ExpiresAt: expiresAt,
	}
	if err := db.Create(record).Error; err != nil {
		return nil, err
	}

	return &CaptchaCapRedeemResponse{
		Success: true,
		Token:   tokenID + ":" + verifyToken,
		Expires: expiresAt.UnixMilli(),
	}, nil
}

func CaptchaCapValidateToken(scene utils.CaptchaScene, token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	if err := CaptchaCapCleanupExpired(); err != nil {
		return false, err
	}

	parts := strings.Split(token, ":")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return false, nil
	}

	key := parts[0] + ":" + captchaCapSHA256(parts[1])
	var record CaptchaCapTokenModel
	if err := db.Where("key = ? AND scene = ?", key, string(scene)).First(&record).Error; err != nil {
		return false, nil
	}
	if !record.ExpiresAt.After(time.Now()) {
		_ = db.Where("key = ?", key).Delete(&CaptchaCapTokenModel{}).Error
		return false, nil
	}
	if err := db.Where("key = ?", key).Delete(&CaptchaCapTokenModel{}).Error; err != nil {
		return false, err
	}
	return true, nil
}
