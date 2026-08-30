package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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
	NamespaceID string     `json:"namespaceId" gorm:"size:100;not null;uniqueIndex:idx_channel_iform_storage_document,priority:1;index"`
	Key         string     `json:"key" gorm:"size:128;not null;uniqueIndex:idx_channel_iform_storage_document,priority:2"`
	JSONValue   string     `json:"value" gorm:"type:text;not null"`
	Revision    uint64     `json:"revision" gorm:"not null;default:1"`
	UpdatedBy   string     `json:"updatedBy" gorm:"size:100;index"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty" gorm:"index"`
}

func (*ChannelIFormStorageDocumentModel) TableName() string { return "channel_iform_storage_documents" }

type ChannelIFormStorageItem struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	Revision  uint64          `json:"revision"`
	Seq       uint64          `json:"seq"`
	ExpiresAt *time.Time      `json:"expiresAt,omitempty"`
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
		if err := tx.Where("namespace_id = ? AND (expires_at IS NULL OR expires_at > ?)", namespace.ID, time.Now()).Order("key ASC").Find(&docs).Error; err != nil {
			return err
		}
		items = make([]ChannelIFormStorageItem, 0, len(docs))
		for _, doc := range docs {
			items = append(items, ChannelIFormStorageItem{Key: doc.Key, Value: json.RawMessage(doc.JSONValue), Revision: doc.Revision, Seq: namespace.Seq, ExpiresAt: doc.ExpiresAt})
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
	return ChannelIFormStorageSetWithExpiry(channelID, formID, key, value, ifRevision, updatedBy, nil)
}

func ChannelIFormStorageSetWithExpiry(channelID, formID, key string, value json.RawMessage, ifRevision *uint64, updatedBy string, expiresAt *time.Time) (*ChannelIFormStorageMutation, error) {
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
			expired := doc.ExpiresAt != nil && !doc.ExpiresAt.After(time.Now())
			if expired {
				if ifRevision != nil && *ifRevision != 0 {
					return fmt.Errorf("REVISION_CONFLICT: currentRevision=0")
				}
				doc.Revision = 1
			} else if ifRevision != nil && doc.Revision != *ifRevision {
				return fmt.Errorf("REVISION_CONFLICT: currentRevision=%d", doc.Revision)
			} else {
				doc.Revision++
			}
			doc.JSONValue = jsonValue
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
		doc.ExpiresAt = expiresAt
		if err := tx.Save(&doc).Error; err != nil {
			return err
		}
		ns.Seq++
		ns.CurrentBytes = newBytes
		if err := tx.Save(ns).Error; err != nil {
			return err
		}
		mutation.Namespace = ns
		mutation.Item = &ChannelIFormStorageItem{Key: key, Value: json.RawMessage(jsonValue), Revision: doc.Revision, Seq: ns.Seq, ExpiresAt: doc.ExpiresAt}
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
		mutation.Item = &ChannelIFormStorageItem{Key: key, Revision: doc.Revision, Seq: ns.Seq, ExpiresAt: doc.ExpiresAt}
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
		q := tx.Where("namespace_id = ? AND (expires_at IS NULL OR expires_at > ?)", ns.ID, time.Now()).Order("key ASC").Limit(limit)
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
			items = append(items, ChannelIFormStorageItem{Key: doc.Key, Value: json.RawMessage(doc.JSONValue), Revision: doc.Revision, Seq: ns.Seq, ExpiresAt: doc.ExpiresAt})
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &ns, items, nil
}

func ChannelIFormStorageNamespaceDelete(channelID, formID string) error {
	return GetDB().Transaction(func(tx *gorm.DB) error {
		return ChannelIFormStorageDeleteByNamespaceScopeTx(tx, channelID, formID)
	})
}

func ChannelIFormStorageDeleteByNamespaceScopeTx(tx *gorm.DB, channelID, formID string) error {
	if !channelIFormStorageTablesReady(tx) {
		return nil
	}
	var namespaces []ChannelIFormStorageNamespaceModel
	if err := tx.Unscoped().Where("channel_id = ? AND form_id = ?", channelID, formID).Find(&namespaces).Error; err != nil {
		return err
	}
	if len(namespaces) == 0 {
		return nil
	}
	ids := make([]string, 0, len(namespaces))
	for _, namespace := range namespaces {
		ids = append(ids, namespace.ID)
	}
	if err := tx.Unscoped().Where("namespace_id IN ?", ids).Delete(&ChannelIFormStorageDocumentModel{}).Error; err != nil {
		return err
	}
	return tx.Unscoped().Where("id IN ?", ids).Delete(&ChannelIFormStorageNamespaceModel{}).Error
}

func ChannelIFormStorageDeleteByFormIDTx(tx *gorm.DB, formID string) error {
	if !channelIFormStorageTablesReady(tx) {
		return nil
	}
	formID = strings.TrimSpace(formID)
	if formID == "" {
		return nil
	}
	var namespaces []ChannelIFormStorageNamespaceModel
	if err := tx.Unscoped().Where("form_id = ?", formID).Find(&namespaces).Error; err != nil {
		return err
	}
	if len(namespaces) == 0 {
		return nil
	}
	ids := make([]string, 0, len(namespaces))
	for _, namespace := range namespaces {
		ids = append(ids, namespace.ID)
	}
	if err := tx.Unscoped().Where("namespace_id IN ?", ids).Delete(&ChannelIFormStorageDocumentModel{}).Error; err != nil {
		return err
	}
	return tx.Unscoped().Where("id IN ?", ids).Delete(&ChannelIFormStorageNamespaceModel{}).Error
}

func ChannelIFormStorageDeleteByChannelIDsTx(tx *gorm.DB, channelIDs []string) error {
	if !channelIFormStorageTablesReady(tx) {
		return nil
	}
	ids := make([]string, 0, len(channelIDs))
	seen := map[string]struct{}{}
	for _, raw := range channelIDs {
		channelID := strings.TrimSpace(raw)
		if channelID == "" {
			continue
		}
		if _, ok := seen[channelID]; ok {
			continue
		}
		seen[channelID] = struct{}{}
		ids = append(ids, channelID)
	}
	if len(ids) == 0 {
		return nil
	}
	var namespaces []ChannelIFormStorageNamespaceModel
	if err := tx.Unscoped().Where("channel_id IN ?", ids).Find(&namespaces).Error; err != nil {
		return err
	}
	if len(namespaces) == 0 {
		return nil
	}
	namespaceIDs := make([]string, 0, len(namespaces))
	for _, namespace := range namespaces {
		namespaceIDs = append(namespaceIDs, namespace.ID)
	}
	if err := tx.Unscoped().Where("namespace_id IN ?", namespaceIDs).Delete(&ChannelIFormStorageDocumentModel{}).Error; err != nil {
		return err
	}
	return tx.Unscoped().Where("id IN ?", namespaceIDs).Delete(&ChannelIFormStorageNamespaceModel{}).Error
}

type ChannelIFormStorageExpiredMutation struct {
	ChannelID string
	FormID    string
	Namespace *ChannelIFormStorageNamespaceModel
	Item      ChannelIFormStorageItem
}

func ChannelIFormStorageDeleteExpired(now time.Time, batchSize int) ([]ChannelIFormStorageExpiredMutation, error) {
	if now.IsZero() {
		now = time.Now()
	}
	if batchSize <= 0 {
		batchSize = 256
	}
	var mutations []ChannelIFormStorageExpiredMutation
	err := GetDB().Transaction(func(tx *gorm.DB) error {
		if !channelIFormStorageTablesReady(tx) {
			return nil
		}
		var candidates []ChannelIFormStorageDocumentModel
		if err := tx.Unscoped().Where("expires_at IS NOT NULL AND expires_at <= ?", now).Order("id ASC").Limit(batchSize).Find(&candidates).Error; err != nil {
			return err
		}
		for _, candidate := range candidates {
			var namespace ChannelIFormStorageNamespaceModel
			if loadErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Unscoped().Where("id = ?", candidate.NamespaceID).First(&namespace).Error; loadErr != nil {
				if errors.Is(loadErr, gorm.ErrRecordNotFound) {
					continue
				}
				return loadErr
			}
			var doc ChannelIFormStorageDocumentModel
			loadErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Unscoped().Where("id = ?", candidate.ID).First(&doc).Error
			if loadErr != nil {
				if errors.Is(loadErr, gorm.ErrRecordNotFound) {
					continue
				}
				return loadErr
			}
			if doc.ExpiresAt == nil || doc.ExpiresAt.After(now) {
				continue
			}
			if err := tx.Unscoped().Delete(&doc).Error; err != nil {
				return err
			}
			namespace.Seq++
			namespace.CurrentBytes -= int64(len(doc.JSONValue))
			if namespace.CurrentBytes < 0 {
				namespace.CurrentBytes = 0
			}
			if err := tx.Save(&namespace).Error; err != nil {
				return err
			}
			mutations = append(mutations, ChannelIFormStorageExpiredMutation{ChannelID: namespace.ChannelID, FormID: namespace.FormID, Namespace: &namespace, Item: ChannelIFormStorageItem{Key: doc.Key, Revision: doc.Revision, Seq: namespace.Seq, ExpiresAt: doc.ExpiresAt}})
		}
		return nil
	})
	return mutations, err
}

func channelIFormStorageTablesReady(tx *gorm.DB) bool {
	if tx == nil {
		return false
	}
	return tx.Migrator().HasTable(&ChannelIFormStorageNamespaceModel{}) && tx.Migrator().HasTable(&ChannelIFormStorageDocumentModel{})
}
