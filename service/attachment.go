package service

import (
	"encoding/hex"
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"sealchat/model"
)

// ResolveAttachment tries to locate an attachment either by its ID or by the
// legacy hash_size filename token returned from the upload handler.
func ResolveAttachment(token string) (*model.AttachmentModel, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil
	}

	db := model.GetDB()
	var att model.AttachmentModel

	// First try by attachment id.
	if err := db.Where("id = ?", token).Limit(1).Find(&att).Error; err != nil {
		return nil, err
	}
	if att.ID != "" {
		return &att, nil
	}

	// Try legacy hash_size filename pattern.
	parts := strings.Split(token, "_")
	if len(parts) != 2 {
		return nil, nil
	}
	hashHex := parts[0]
	sizeStr := parts[1]

	hashBytes, err := hex.DecodeString(hashHex)
	if err != nil {
		return nil, nil
	}

	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		return nil, nil
	}

	if err := db.Where("hash = ? AND size = ?", hashBytes, size).Limit(1).Find(&att).Error; err != nil {
		return nil, err
	}
	if att.ID == "" {
		return nil, nil
	}
	return &att, nil
}

func ResolveAttachmentOwnership(userID string, token string) (*model.AttachmentModel, error) {
	if token == "" {
		return nil, nil
	}
	att, err := ResolveAttachment(token)
	if err != nil {
		return nil, err
	}
	if att == nil {
		return nil, gorm.ErrRecordNotFound
	}
	if att.UserID != userID {
		return nil, errors.New("无法使用他人上传的头像")
	}
	return att, nil
}
