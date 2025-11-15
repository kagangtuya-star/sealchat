package model

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type BotTokenModel struct {
	StringPKBaseModel
	Name         string `json:"name"`
	Avatar       string `json:"avatar"`
	NickColor    string `json:"nickColor"`
	Token        string `json:"token" gorm:"index"`
	ExpiresAt    int64  `json:"expiresAt"`
	RecentUsedAt int64  `json:"recentUsedAt"`
}

func (*BotTokenModel) TableName() string {
	return "bot_tokens"
}

func BotTokenGet(id string) (*BotTokenModel, error) {
	if strings.TrimSpace(id) == "" {
		return nil, nil
	}
	var botToken BotTokenModel
	if err := db.Where("id = ?", id).Limit(1).Find(&botToken).Error; err != nil {
		return nil, err
	}
	if botToken.ID == "" {
		return nil, nil
	}
	return &botToken, nil
}

func BotVerifyAccessToken(tokenString string) (*UserModel, error) {
	// 解析 token
	var botToken BotTokenModel
	db.Select("id, expires_at").Where("token = ?", tokenString).Limit(1).Find(&botToken)

	// 注: 这里出现的慢查询有点疑惑，可能跟机器内存快满了有关，但是关于First(1)和Limit(1).Find()倒是可以得出一种结论
	// 这是两种的操作的语句区别：
	// [rows:0] SELECT id, expires_at FROM "bot_tokens" WHERE token = 'LXjFCF36wkF46NQNIIcq307jKUNqRYK4' ORDER BY "bot_tokens"."id" LIMIT 1
	// [rows:0] SELECT id, expires_at FROM "bot_tokens" WHERE token = 'LXjFCF36wkF46NQNIIcq307jKUNqRYK4' LIMIT 1
	// 根据psql的explain来说，Limit(1).Find(1)更优 (cost=9.51..9.52 rows=1 width=296) 对应 (cost=0.15..6.17 rows=1 width=136)

	if botToken.ID == "" {
		return nil, errors.New("bot not found")
	}

	// 验证 token 是否过期
	if botToken.ExpiresAt < time.Now().UnixMilli() {
		return nil, errors.New("token expired")
	}

	// 查询用户
	var user UserModel
	userID := botToken.ID
	if err := db.Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, fmt.Errorf("UserModel not found")
	}

	return &user, nil
}
