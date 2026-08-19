package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	EmbedStorageKeyMax       = 128
	EmbedStorageValueMax     = 64 * 1024
	EmbedStorageNamespaceMax = 2 * 1024 * 1024
	EmbedStorageDocumentsMax = 256
)

type ChannelIFormStorageNamespaceModel struct {
	StringPKBaseModel
	ChannelID    string `json:"channelId" gorm:"size:100;not null;uniqueIndex:idx_channel_iform_storage_namespace,priority:1"`
	FormID       string `json:"formId" gorm:"size:100;not null;uniqueIndex:idx_channel_iform_storage_namespace,priority:2"`
	Seq          uint64 `json:"seq" gorm:"not null;default:0"`
	CurrentBytes int64  `json:"currentBytes" gorm:"not null;default:0"`
}

func (*ChannelIFormStorageNamespaceModel) TableName() string {
	return "channel_iform_storage_namespaces"
}

type ChannelIFormStorageDocumentModel struct {
	StringPKBaseModel
	NamespaceID string `json:"namespaceId" gorm:"size:100;not null;uniqueIndex:idx_channel_iform_storage_document,priority:1;index"`
	Key         string `json:"key" gorm:"size:128;not null;uniqueIndex:idx_channel_iform_storage_document,priority:2"`
	JSONValue   string `json:"value" gorm:"type:text;not null"`
	Revision    uint64 `json:"revision" gorm:"not null;default:1"`
	UpdatedBy   string `json:"updatedBy" gorm:"size:100;index"`
}

func (*ChannelIFormStorageDocumentModel) TableName() string { return "channel_iform_storage_documents" }

type ChannelIFormStorageItem struct {
	Key      string          `json:"key"`
	Value    json.RawMessage `json:"value"`
	Revision uint64          `json:"revision"`
	Seq      uint64          `json:"seq"`
}

func normalizeEmbedStorageKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > EmbedStorageKeyMax {
		return "", fmt.Errorf("INVALID_PARAMS: key must be 1-%d characters", EmbedStorageKeyMax)
	}
	return key, nil
}

func normalizeEmbedStorageValue(value json.RawMessage) (string, error) {
	if len(value) == 0 || len(value) > EmbedStorageValueMax {
		return "", errors.New("PAYLOAD_TOO_LARGE: value exceeds 64 KiB")
	}
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return "", errors.New("INVALID_PARAMS: value must be valid JSON")
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return "", errors.New("INVALID_PARAMS: value must be valid JSON")
	}
	return string(canonical), nil
}

func ChannelIFormStorageNamespaceGetOrCreate(tx *gorm.DB, channelID, formID string) (*ChannelIFormStorageNamespaceModel, error) {
	channelID = strings.TrimSpace(channelID)
	formID = strings.TrimSpace(formID)
	if channelID == "" || formID == "" {
		return nil, errors.New("INVALID_PARAMS: missing storage scope")
	}
	var namespace ChannelIFormStorageNamespaceModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("channel_id = ? AND form_id = ?", channelID, formID).First(&namespace).Error
	if err == nil {
		return &namespace, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	namespace = ChannelIFormStorageNamespaceModel{ChannelID: channelID, FormID: formID}
	if err := namespace.BeforeCreate(tx); err != nil {
		return nil, err
	}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "channel_id"}, {Name: "form_id"}}, DoNothing: true}).Create(&namespace).Error; err != nil {
		return nil, err
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("channel_id = ? AND form_id = ?", channelID, formID).First(&namespace).Error; err != nil {
		return nil, err
	}
	return &namespace, nil
}

func ChannelIFormStorageSnapshot(channelID, formID string) (*ChannelIFormStorageNamespaceModel, []ChannelIFormStorageItem, error) {
	db := GetDB()
	var namespace ChannelIFormStorageNamespaceModel
	var items []ChannelIFormStorageItem
	err := db.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("channel_id = ? AND form_id = ?", channelID, formID).First(&namespace).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			namespace = ChannelIFormStorageNamespaceModel{ChannelID: channelID, FormID: formID}
			items = []ChannelIFormStorageItem{}
			return nil
		}
		if err != nil {
			return err
		}
		var docs []ChannelIFormStorageDocumentModel
		if err := tx.Where("namespace_id = ?", namespace.ID).Order("key ASC").Find(&docs).Error; err != nil {
			return err
		}
		items = make([]ChannelIFormStorageItem, 0, len(docs))
		for _, doc := range docs {
			items = append(items, ChannelIFormStorageItem{Key: doc.Key, Value: json.RawMessage(doc.JSONValue), Revision: doc.Revision, Seq: namespace.Seq})
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &namespace, items, nil
}

func ChannelIFormStorageGet(channelID, formID, key string) (*ChannelIFormStorageItem, error) {
	key, err := normalizeEmbedStorageKey(key)
	if err != nil {
		return nil, err
	}
	ns, items, err := ChannelIFormStorageSnapshot(channelID, formID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Key == key {
			item.Seq = ns.Seq
			return &item, nil
		}
	}
	return nil, nil
}

type ChannelIFormStorageMutation struct {
	Item      *ChannelIFormStorageItem
	Namespace *ChannelIFormStorageNamespaceModel
	Deleted   bool
}

func ChannelIFormStorageSet(channelID, formID, key string, value json.RawMessage, ifRevision *uint64, updatedBy string) (*ChannelIFormStorageMutation, error) {
	key, err := normalizeEmbedStorageKey(key)
	if err != nil {
		return nil, err
	}
	jsonValue, err := normalizeEmbedStorageValue(value)
	if err != nil {
		return nil, err
	}
	db := GetDB()
	var mutation ChannelIFormStorageMutation
	err = db.Transaction(func(tx *gorm.DB) error {
		ns, err := ChannelIFormStorageNamespaceGetOrCreate(tx, channelID, formID)
		if err != nil {
			return err
		}
		var doc ChannelIFormStorageDocumentModel
		existingDoc := false
		oldJSONValue := ""
		err = tx.Where("namespace_id = ? AND key = ?", ns.ID, key).First(&doc).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if ifRevision != nil && *ifRevision != 0 {
				return fmt.Errorf("REVISION_CONFLICT: currentRevision=0")
			}
			var count int64
			if err := tx.Model(&ChannelIFormStorageDocumentModel{}).Where("namespace_id = ?", ns.ID).Count(&count).Error; err != nil {
				return err
			}
			if count >= EmbedStorageDocumentsMax {
				return errors.New("QUOTA_EXCEEDED: document limit reached")
			}
			doc = ChannelIFormStorageDocumentModel{NamespaceID: ns.ID, Key: key, JSONValue: jsonValue, Revision: 1, UpdatedBy: updatedBy}
		} else if err != nil {
			return err
		} else {
			existingDoc = true
			oldJSONValue = doc.JSONValue
			if ifRevision != nil && doc.Revision != *ifRevision {
				return fmt.Errorf("REVISION_CONFLICT: currentRevision=%d", doc.Revision)
			}
			doc.JSONValue = jsonValue
			doc.Revision++
			doc.UpdatedBy = updatedBy
		}
		oldBytes := int64(0)
		if existingDoc {
			oldBytes = int64(len(oldJSONValue))
		}
		newBytes := ns.CurrentBytes - oldBytes + int64(len(jsonValue))
		if newBytes > EmbedStorageNamespaceMax {
			return errors.New("QUOTA_EXCEEDED: namespace exceeds 2 MiB")
		}
		if err := tx.Save(&doc).Error; err != nil {
			return err
		}
		ns.Seq++
		ns.CurrentBytes = newBytes
		if err := tx.Save(ns).Error; err != nil {
			return err
		}
		mutation.Namespace = ns
		mutation.Item = &ChannelIFormStorageItem{Key: key, Value: json.RawMessage(jsonValue), Revision: doc.Revision, Seq: ns.Seq}
		return nil
	})
	return &mutation, err
}

func ChannelIFormStorageDelete(channelID, formID, key string, ifRevision *uint64, updatedBy string) (*ChannelIFormStorageMutation, error) {
	key, err := normalizeEmbedStorageKey(key)
	if err != nil {
		return nil, err
	}
	db := GetDB()
	var mutation ChannelIFormStorageMutation
	err = db.Transaction(func(tx *gorm.DB) error {
		ns, err := ChannelIFormStorageNamespaceGetOrCreate(tx, channelID, formID)
		if err != nil {
			return err
		}
		var doc ChannelIFormStorageDocumentModel
		err = tx.Where("namespace_id = ? AND key = ?", ns.ID, key).First(&doc).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if ifRevision != nil && *ifRevision != 0 {
				return fmt.Errorf("REVISION_CONFLICT: currentRevision=0")
			}
			mutation.Namespace = ns
			return nil
		}
		if err != nil {
			return err
		}
		if ifRevision != nil && doc.Revision != *ifRevision {
			return fmt.Errorf("REVISION_CONFLICT: currentRevision=%d", doc.Revision)
		}
		if err := tx.Unscoped().Delete(&doc).Error; err != nil {
			return err
		}
		ns.Seq++
		ns.CurrentBytes -= int64(len(doc.JSONValue))
		if ns.CurrentBytes < 0 {
			ns.CurrentBytes = 0
		}
		if err := tx.Save(ns).Error; err != nil {
			return err
		}
		mutation.Namespace, mutation.Deleted = ns, true
		mutation.Item = &ChannelIFormStorageItem{Key: key, Revision: doc.Revision, Seq: ns.Seq}
		return nil
	})
	return &mutation, err
}

func ChannelIFormStorageList(channelID, formID, prefix, cursor string, limit int) (*ChannelIFormStorageNamespaceModel, []ChannelIFormStorageItem, error) {
	if limit <= 0 || limit > 256 {
		limit = 256
	}
	db := GetDB()
	var ns ChannelIFormStorageNamespaceModel
	var items []ChannelIFormStorageItem
	err := db.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("channel_id = ? AND form_id = ?", channelID, formID).First(&ns).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ns = ChannelIFormStorageNamespaceModel{ChannelID: channelID, FormID: formID}
			items = []ChannelIFormStorageItem{}
			return nil
		}
		if err != nil {
			return err
		}
		q := tx.Where("namespace_id = ?", ns.ID).Order("key ASC").Limit(limit)
		if strings.TrimSpace(prefix) != "" {
			q = q.Where("key LIKE ?", strings.TrimSpace(prefix)+"%")
		}
		if strings.TrimSpace(cursor) != "" {
			q = q.Where("key > ?", strings.TrimSpace(cursor))
		}
		var docs []ChannelIFormStorageDocumentModel
		if err := q.Find(&docs).Error; err != nil {
			return err
		}
		items = make([]ChannelIFormStorageItem, 0, len(docs))
		for _, doc := range docs {
			items = append(items, ChannelIFormStorageItem{Key: doc.Key, Value: json.RawMessage(doc.JSONValue), Revision: doc.Revision, Seq: ns.Seq})
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &ns, items, nil
}

func ChannelIFormStorageNamespaceDelete(channelID, formID string) error {
	return GetDB().Where("channel_id = ? AND form_id = ?", channelID, formID).Delete(&ChannelIFormStorageNamespaceModel{}).Error
}
