package service

import (
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
	"sealchat/protocol"
)

var ErrChatSkinAttachmentNotImage = errors.New("聊天装扮只能引用图片附件")

// ensureChatSkinAttachmentsAccessible 校验装扮引用的附件均为操作者可用的图片。
func ensureChatSkinAttachmentsAccessible(ownerUserID, operatorUserID, channelID string, document map[string]any) error {
	for _, attachmentID := range protocol.ChatSkinAttachmentIDs(document) {
		att, err := ResolveAttachmentAccessible(ownerUserID, operatorUserID, channelID, attachmentID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("聊天装扮引用的附件不存在")
			}
			return errors.New("聊天装扮引用了无法使用的附件")
		}
		if att == nil {
			return errors.New("聊天装扮引用的附件不存在")
		}
		if mimeType := strings.ToLower(strings.TrimSpace(att.MimeType)); mimeType != "" && !strings.HasPrefix(mimeType, "image/") {
			return ErrChatSkinAttachmentNotImage
		}
	}
	return nil
}

// NormalizeChannelIdentityChatSkin 清理装扮结构并校验附件权限；返回 nil 表示清除装扮。
func NormalizeChannelIdentityChatSkin(ownerUserID, operatorUserID, channelID string, skin protocol.ChatSkin) (protocol.ChatSkin, error) {
	normalized, err := protocol.SanitizeChatSkin(skin)
	if err != nil || normalized == nil {
		return nil, err
	}
	if err := ensureChatSkinAttachmentsAccessible(ownerUserID, operatorUserID, channelID, normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

// NormalizeChannelIdentityChatSkinPatch 清理差分覆写并校验附件权限；返回 nil 表示沿用基础装扮。
func NormalizeChannelIdentityChatSkinPatch(ownerUserID, operatorUserID, channelID string, patch *protocol.ChatSkinPatch) (*protocol.ChatSkinPatch, error) {
	normalized, err := protocol.SanitizeChatSkinPatch(patch)
	if err != nil || normalized == nil {
		return nil, err
	}
	if normalized.Skin != nil {
		if err := ensureChatSkinAttachmentsAccessible(ownerUserID, operatorUserID, channelID, normalized.Skin); err != nil {
			return nil, err
		}
	}
	return normalized, nil
}

func resolveChatSkinActor(actor *ChannelIdentityActorContext, channelID string) (*ChannelIdentityActorContext, error) {
	if actor == nil {
		return nil, ErrChannelPermissionDenied
	}
	if !actor.IsBotTarget {
		return actor, nil
	}
	resolved, err := ensureBotManagedChannelIdentityActor(actor, channelID)
	if err != nil {
		return nil, err
	}
	if err := EnsureBotChannelIdentity(resolved.TargetUserID, channelID); err != nil {
		return nil, err
	}
	return resolved, nil
}

type ChannelIdentityChatSkinResult struct {
	Item   *model.ChannelIdentityModel   `json:"item"`
	Copies []*model.ChannelIdentityModel `json:"-"`
}

// sharedChannelIdentityDataChatSkin 读取共享角色权威文档中的聊天装扮。
func sharedChannelIdentityDataChatSkin(raw string) protocol.ChatSkin {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var document struct {
		ChatSkin protocol.ChatSkin `json:"chatSkin"`
	}
	if json.Unmarshal([]byte(raw), &document) != nil {
		return nil
	}
	return document.ChatSkin
}

// sharedChannelIdentityDataWithChatSkin 写入聊天装扮，保留文档中的其他（含未知）字段。
func sharedChannelIdentityDataWithChatSkin(raw string, skin protocol.ChatSkin) (string, error) {
	document := map[string]json.RawMessage{}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &document); err != nil {
			return "", err
		}
	}
	if len(skin) == 0 {
		delete(document, "chatSkin")
	} else {
		encoded, err := json.Marshal(skin)
		if err != nil {
			return "", err
		}
		document["chatSkin"] = encoded
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// ChannelIdentityChatSkinSet 保存频道角色的基础聊天装扮。共享角色会更新权威文档并同步全部副本；
// 已发送消息只持有发送时快照，不受影响。
func ChannelIdentityChatSkinSet(actor *ChannelIdentityActorContext, channelID, identityID string, skin protocol.ChatSkin) (*ChannelIdentityChatSkinResult, error) {
	channelID = strings.TrimSpace(channelID)
	resolved, err := resolveChatSkinActor(actor, channelID)
	if err != nil {
		return nil, err
	}
	identity, err := ValidateChannelIdentityActorIdentity(resolved, channelID, identityID)
	if err != nil {
		return nil, err
	}
	normalized, err := NormalizeChannelIdentityChatSkin(resolved.TargetUserID, resolved.OperatorUserID, channelID, skin)
	if err != nil {
		return nil, err
	}
	if identity.SharedIdentityID == "" {
		values := map[string]any{"chat_skin": normalized}
		if err := model.ChannelIdentityEncodeStoredValues(values); err != nil {
			return nil, err
		}
		if err := model.GetDB().Model(&model.ChannelIdentityModel{}).Where("id = ?", identity.ID).Updates(values).Error; err != nil {
			return nil, err
		}
		updated, err := model.ChannelIdentityGetByID(identity.ID)
		if err != nil {
			return nil, err
		}
		return &ChannelIdentityChatSkinResult{Item: updated, Copies: []*model.ChannelIdentityModel{updated}}, nil
	}
	if !resolved.IsBotTarget {
		if err := ensureSharedChannelIdentityOwner(resolved.TargetUserID, resolved.OperatorUserID); err != nil {
			return nil, ErrSharedChannelIdentitySynchronizedFieldsReadOnly
		}
	}
	result := &ChannelIdentityChatSkinResult{}
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		var template model.SharedChannelIdentityModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", identity.SharedIdentityID, resolved.TargetUserID).Take(&template).Error; err != nil {
			return err
		}
		data, err := sharedChannelIdentityDataWithChatSkin(template.SharedDataJSON, normalized)
		if err != nil {
			return err
		}
		template.SharedDataJSON = data
		template.Revision++
		if err := tx.Model(&template).Select("shared_data_json", "revision").Updates(&template).Error; err != nil {
			return err
		}
		values := map[string]any{"chat_skin": normalized, "shared_revision": template.Revision}
		if err := model.ChannelIdentityEncodeStoredValues(values); err != nil {
			return err
		}
		var copies []*model.ChannelIdentityModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("shared_identity_id = ? AND user_id = ?", template.ID, resolved.TargetUserID).
			Order("channel_id ASC, created_at ASC").Find(&copies).Error; err != nil {
			return err
		}
		for _, copy := range copies {
			if err := tx.Model(copy).Updates(values).Error; err != nil {
				return err
			}
			if err := tx.Where("id = ?", copy.ID).Take(copy).Error; err != nil {
				return err
			}
			if copy.ID == identity.ID {
				result.Item = copy
			}
		}
		result.Copies = copies
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result.Item == nil {
		return nil, errors.New("共享角色当前频道投影不存在")
	}
	return result, nil
}

// ChannelIdentityVariantChatSkinSet 保存差分的聊天装扮覆写（存放于 appearance.chatSkin）。
// patch 为 nil 时移除覆写，差分回到基础装扮。
func ChannelIdentityVariantChatSkinSet(actor *ChannelIdentityActorContext, channelID, variantID string, patch *protocol.ChatSkinPatch) (*model.ChannelIdentityVariantModel, error) {
	channelID = strings.TrimSpace(channelID)
	resolved, err := resolveChatSkinActor(actor, channelID)
	if err != nil {
		return nil, err
	}
	item, err := ChannelIdentityVariantGetForUser(resolved.TargetUserID, channelID, variantID)
	if err != nil {
		return nil, err
	}
	identity, err := ValidateChannelIdentityActorIdentity(resolved, channelID, item.IdentityID)
	if err != nil {
		return nil, err
	}
	normalized, err := NormalizeChannelIdentityChatSkinPatch(resolved.TargetUserID, resolved.OperatorUserID, channelID, patch)
	if err != nil {
		return nil, err
	}
	appearance := item.Appearance()
	if normalized == nil {
		delete(appearance, "chatSkin")
	} else {
		appearance["chatSkin"] = normalized
	}
	appearanceJSON, err := serializeChannelIdentityVariantAppearanceJSON(appearance)
	if err != nil {
		return nil, err
	}
	if item.SharedVariantID != "" && identity.SharedIdentityID != "" {
		source := *item
		source.AppearanceJSON = appearanceJSON
		return syncSharedChannelIdentityVariant(identity, &source, 0)
	}
	if err := model.ChannelIdentityVariantUpdate(item.ID, map[string]any{"appearance_json": appearanceJSON}); err != nil {
		return nil, err
	}
	return model.ChannelIdentityVariantGetByID(item.ID)
}

// mapChannelIdentityChatSkinAttachments 复制频道时把装扮引用的附件映射到新频道；
// 无法复制的附件引用会被移除（对应图层不再显示），不阻断整个复制流程。
func mapChannelIdentityChatSkinAttachments(document map[string]any, clone func(attachmentID string) (string, error)) map[string]any {
	if document == nil {
		return nil
	}
	return protocol.MapChatSkinAttachmentRefs(document, func(attachmentID string) string {
		cloned, err := clone(attachmentID)
		if err != nil {
			return ""
		}
		return cloned
	})
}
