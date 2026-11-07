package model

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"gorm.io/gorm"
)

// ChannelExtensionScope is the logical owner of channel-local extension data.
// Physical table/column names stay unchanged so existing iForm data needs no migration.
type ChannelExtensionScope struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

func (s ChannelExtensionScope) StorageNamespaceKey() string {
	if s.Type == "iform" {
		return s.ID
	}
	sum := sha256.Sum256([]byte(s.ID))
	return "uic_" + hex.EncodeToString(sum[:])
}

func (s ChannelExtensionScope) Valid() bool {
	return (s.Type == "iform" || s.Type == "ui-customization") && s.ID != "" && len(s.ID) <= 100 && strings.TrimSpace(s.ID) == s.ID && !strings.ContainsRune(s.ID, '\x00') && (s.Type != "iform" || !strings.HasPrefix(s.ID, "uic_"))
}

// Empty channelID removes this scope across channels, within the caller transaction.
func ChannelExtensionStorageCleanupTx(tx *gorm.DB, channelID string, scope ChannelExtensionScope) error {
	if channelID == "" {
		return ChannelExtensionStorageDeleteByNamespaceKeyTx(tx, scope.StorageNamespaceKey())
	}
	return ChannelExtensionStorageDeleteByNamespaceScopeTx(tx, channelID, scope.StorageNamespaceKey())
}
