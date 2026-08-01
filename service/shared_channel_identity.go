package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/utils"
)

var (
	ErrSharedChannelIdentityOwnerOnly                  = errors.New("仅身份本人可管理共享角色")
	ErrSharedChannelIdentitySynchronizedFieldsReadOnly = errors.New("共享角色的昵称、颜色、头像、头像装饰和小剧场演出仅身份本人可修改")
	ErrSharedChannelIdentityUnsupported                = errors.New("临时、隐藏或 BOT 身份不支持共享")
	ErrSharedChannelIdentityWorldRequired              = errors.New("共享角色仅支持世界内频道")
	ErrSharedChannelIdentityRevisionConflict           = errors.New("共享角色已在其他位置更新，请刷新后重试")
	ErrChannelIdentityAlreadyShared                    = errors.New("频道身份已关联共享角色")
	ErrChannelIdentityNotShared                        = errors.New("频道身份未关联共享角色")
)

type SharedChannelIdentitySyncInput struct {
	DisplayName            string
	Color                  string
	AvatarAttachmentID     string
	AvatarDecorations      protocol.AvatarDecorationList
	TheaterPresentation    *protocol.TheaterPresentation
	TheaterPresentationSet bool
}

type SharedChannelIdentitySyncResult struct {
	Template           *model.SharedChannelIdentityModel `json:"template,omitempty"`
	Copies             []*model.ChannelIdentityModel     `json:"copies,omitempty"`
	FailedCopyIDs      map[string]string                 `json:"failedCopyIds,omitempty"`
	RetryScheduled     bool                              `json:"retryScheduled"`
	RetryScheduleError string                            `json:"retryScheduleError,omitempty"`
}

// serializeSharedChannelIdentityData keeps unknown keys from a newer server build.
// Known fields remain queryable columns; document is forward-compatible authority data.
func serializeSharedChannelIdentityData(previous string, item *model.SharedChannelIdentityModel) (string, error) {
	document := map[string]json.RawMessage{}
	if strings.TrimSpace(previous) != "" {
		if err := json.Unmarshal([]byte(previous), &document); err != nil {
			return "", err
		}
	}
	put := func(key string, value any) error {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		document[key] = raw
		return nil
	}
	for key, value := range map[string]any{
		"version": 1, "displayName": item.DisplayName, "color": item.Color,
		"avatarAttachmentId": item.AvatarAttachmentID, "avatarDecorations": item.AvatarDecorations,
		"theaterPresentation": item.TheaterPresentation,
	} {
		if err := put(key, value); err != nil {
			return "", err
		}
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func sharedChannelIdentityCopyValues(template *model.SharedChannelIdentityModel, presentation *protocol.TheaterPresentation) (map[string]any, error) {
	decorations, err := json.Marshal(template.AvatarDecorations)
	if err != nil {
		return nil, err
	}
	values := map[string]any{
		"display_name": template.DisplayName, "color": template.Color,
		"avatar_attachment_id": template.AvatarAttachmentID,
		"avatar_decoration":    string(decorations), "shared_revision": template.Revision,
	}
	if presentation == nil {
		values["theater_presentation"] = nil
		return values, nil
	}
	encoded, err := json.Marshal(presentation)
	if err != nil {
		return nil, err
	}
	values["theater_presentation"] = string(encoded)
	return values, nil
}

var sharedChannelIdentityCopyRepairHook func(*model.ChannelIdentityModel) error

func ensureSharedChannelIdentityOwner(ownerUserID, operatorUserID string) error {
	ownerUserID = strings.TrimSpace(ownerUserID)
	operatorUserID = strings.TrimSpace(operatorUserID)
	if ownerUserID == "" || operatorUserID == "" || ownerUserID != operatorUserID {
		return ErrSharedChannelIdentityOwnerOnly
	}
	return nil
}

func ensureSharedChannelIdentityEligibleTx(conn *gorm.DB, identity *model.ChannelIdentityModel) error {
	if identity == nil || identity.IsTemporary || identity.IsHidden {
		return ErrSharedChannelIdentityUnsupported
	}
	if conn == nil {
		conn = model.GetDB()
	}
	var user model.UserModel
	if err := conn.Select("id", "is_bot").Where("id = ?", identity.UserID).Limit(1).Find(&user).Error; err != nil {
		return err
	}
	if strings.TrimSpace(user.ID) == "" || user.IsBot {
		return ErrSharedChannelIdentityUnsupported
	}
	return nil
}

func ensureSharedChannelIdentityEligible(identity *model.ChannelIdentityModel) error {
	return ensureSharedChannelIdentityEligibleTx(model.GetDB(), identity)
}

func sharedChannelIdentityWorldIDTx(tx *gorm.DB, channelID string) (string, error) {
	var channel model.ChannelModel
	if err := tx.Select("id", "world_id").Where("id = ?", strings.TrimSpace(channelID)).Take(&channel).Error; err != nil {
		return "", err
	}
	return strings.TrimSpace(channel.WorldID), nil
}

func sharedChannelIdentityRequireWorldIDTx(tx *gorm.DB, channelID string) (string, error) {
	worldID, err := sharedChannelIdentityWorldIDTx(tx, channelID)
	if err != nil {
		return "", err
	}
	if worldID == "" {
		return "", ErrSharedChannelIdentityWorldRequired
	}
	return worldID, nil
}

func sharedChannelIdentityWorldPresentationTx(tx *gorm.DB, sharedIdentityID, worldID string) (*model.SharedChannelIdentityWorldPresentationModel, error) {
	var item model.SharedChannelIdentityWorldPresentationModel
	if err := tx.Where("shared_identity_id = ? AND world_id = ?", sharedIdentityID, worldID).Limit(1).Find(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, nil
	}
	return &item, nil
}

func upsertSharedChannelIdentityWorldPresentationTx(tx *gorm.DB, template *model.SharedChannelIdentityModel, source *model.ChannelIdentityModel) error {
	worldID, err := sharedChannelIdentityWorldIDTx(tx, source.ChannelID)
	if err != nil {
		return err
	}
	if worldID == "" {
		return nil
	}
	item, err := sharedChannelIdentityWorldPresentationTx(tx, template.ID, worldID)
	if err != nil {
		return err
	}
	if item == nil {
		item = &model.SharedChannelIdentityWorldPresentationModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
			SharedIdentityID:  template.ID,
			WorldID:           worldID,
		}
	}
	item.SourceChannelID = source.ChannelID
	item.SourceIdentityID = source.ID
	item.TheaterPresentation = cloneTheaterPresentation(source.TheaterPresentation)
	item.Revision = template.Revision
	return tx.Save(item).Error
}

func sharedChannelIdentityChannelsTx(tx *gorm.DB, source *model.ChannelIdentityModel, worldID string) ([]model.ChannelModel, error) {
	var roleMappings []model.UserRoleMappingModel
	if err := tx.Where("user_id = ? AND role_type = ?", source.UserID, "channel").Find(&roleMappings).Error; err != nil {
		return nil, err
	}
	channelIDSet := map[string]struct{}{source.ChannelID: {}}
	for _, mapping := range roleMappings {
		if channelID := strings.TrimSpace(model.ExtractChIdFromRoleId(mapping.RoleID)); channelID != "" {
			channelIDSet[channelID] = struct{}{}
		}
	}
	var ownedChannelIDs []string
	if err := tx.Model(&model.ChannelModel{}).Where("user_id = ?", source.UserID).Pluck("id", &ownedChannelIDs).Error; err != nil {
		return nil, err
	}
	for _, channelID := range ownedChannelIDs {
		if channelID = strings.TrimSpace(channelID); channelID != "" {
			channelIDSet[channelID] = struct{}{}
		}
	}
	channelIDs := make([]string, 0, len(channelIDSet))
	for channelID := range channelIDSet {
		channelIDs = append(channelIDs, channelID)
	}
	var channels []model.ChannelModel
	err := tx.Where("id IN ? AND world_id = ? AND (status IS NULL OR status <> ?)", channelIDs, worldID, model.ChannelStatusDeleted).
		Order("id ASC").Find(&channels).Error
	return channels, err
}

func createSharedChannelIdentityCopiesTx(tx *gorm.DB, source *model.ChannelIdentityModel, template *model.SharedChannelIdentityModel) error {
	channels, err := sharedChannelIdentityChannelsTx(tx, source, template.WorldID)
	if err != nil {
		return err
	}
	for _, channel := range channels {
		if channel.ID == source.ChannelID {
			continue
		}
		var existing int64
		if err := tx.Model(&model.ChannelIdentityModel{}).
			Where("shared_identity_id = ? AND channel_id = ?", template.ID, channel.ID).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			continue
		}
		var maxSort int
		if err := tx.Model(&model.ChannelIdentityModel{}).Where("channel_id = ? AND user_id = ?", channel.ID, source.UserID).
			Select("coalesce(max(sort_order), 0)").Scan(&maxSort).Error; err != nil {
			return err
		}
		var defaultCount int64
		if err := tx.Model(&model.ChannelIdentityModel{}).Where("channel_id = ? AND user_id = ? AND is_default = ?", channel.ID, source.UserID, true).
			Count(&defaultCount).Error; err != nil {
			return err
		}
		copy := &model.ChannelIdentityModel{
			ChannelID: channel.ID, UserID: source.UserID, SharedIdentityID: template.ID, SharedRevision: template.Revision,
			DisplayName: template.DisplayName, Color: template.Color, AvatarAttachmentID: template.AvatarAttachmentID,
			AvatarDecorations: append(protocol.AvatarDecorationList(nil), template.AvatarDecorations...),
			IsDefault:         defaultCount == 0, SortOrder: maxSort + 1,
		}
		if err := tx.Create(copy).Error; err != nil {
			return err
		}
		mapped, err := mapSharedTheaterPresentationTx(tx, template.SourceChannelID, template.SourceIdentityID, copy, template.TheaterPresentation)
		if err != nil {
			return err
		}
		values, err := sharedChannelIdentityCopyValues(template, mapped)
		if err != nil {
			return err
		}
		if err := tx.Model(copy).Updates(values).Error; err != nil {
			return err
		}
		copy.TheaterPresentation = mapped
	}
	return nil
}

func MaterializeSharedChannelIdentitiesForUserTx(tx *gorm.DB, userID string) error {
	userID = strings.TrimSpace(userID)
	if tx == nil || userID == "" {
		return nil
	}
	var templates []*model.SharedChannelIdentityModel
	if err := tx.Where("user_id = ?", userID).Order("created_at ASC").Find(&templates).Error; err != nil {
		return err
	}
	for _, template := range templates {
		if strings.TrimSpace(template.WorldID) == "" {
			continue
		}
		var source model.ChannelIdentityModel
		if err := tx.Where("shared_identity_id = ? AND user_id = ?", template.ID, userID).
			Order("shared_revision DESC, created_at ASC").Limit(1).Find(&source).Error; err != nil {
			return err
		}
		if source.ID == "" {
			continue
		}
		worldID, err := sharedChannelIdentityRequireWorldIDTx(tx, source.ChannelID)
		if err != nil {
			return err
		}
		if worldID != template.WorldID {
			continue
		}
		if err := createSharedChannelIdentityCopiesTx(tx, &source, template); err != nil {
			return err
		}
		copies, err := sharedIdentityCopiesTx(tx, template.ID)
		if err != nil {
			return err
		}
		var variants []*model.ChannelIdentityVariantModel
		if err := tx.Where("identity_id = ? AND shared_variant_id <> ''", source.ID).
			Order("sort_order ASC, created_at ASC").Find(&variants).Error; err != nil {
			return err
		}
		for _, variant := range variants {
			if err := createSharedVariantCopiesTx(tx, &source, variant, copies); err != nil {
				return err
			}
		}
	}
	return nil
}

func MaterializeSharedChannelIdentitiesForUser(userID string) error {
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		return MaterializeSharedChannelIdentitiesForUserTx(tx, userID)
	})
}

func sharedChannelIdentityCreateFromCopyTx(tx *gorm.DB, ownerUserID, identityID string, result *SharedChannelIdentitySyncResult) error {
	var identity model.ChannelIdentityModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", strings.TrimSpace(identityID), ownerUserID).Take(&identity).Error; err != nil {
		return err
	}
	if identity.SharedIdentityID != "" {
		return ErrChannelIdentityAlreadyShared
	}
	if err := ensureSharedChannelIdentityEligibleTx(tx, &identity); err != nil {
		return err
	}
	worldID, err := sharedChannelIdentityRequireWorldIDTx(tx, identity.ChannelID)
	if err != nil {
		return err
	}
	template := &model.SharedChannelIdentityModel{
		WorldID: worldID, UserID: identity.UserID, SourceChannelID: identity.ChannelID, SourceIdentityID: identity.ID,
		DisplayName: identity.DisplayName, Color: identity.Color,
		AvatarAttachmentID:  identity.AvatarAttachmentID,
		AvatarDecorations:   append(protocol.AvatarDecorationList(nil), identity.AvatarDecorations...),
		TheaterPresentation: cloneTheaterPresentation(identity.TheaterPresentation), Revision: 1,
	}
	var dataErr error
	template.SharedDataJSON, dataErr = serializeSharedChannelIdentityData("", template)
	if dataErr != nil {
		return dataErr
	}
	if err := tx.Create(template).Error; err != nil {
		return err
	}
	if err := tx.Model(&identity).Updates(map[string]any{"shared_identity_id": template.ID, "shared_revision": template.Revision}).Error; err != nil {
		return err
	}
	identity.SharedIdentityID = template.ID
	identity.SharedRevision = template.Revision
	if err := upsertSharedChannelIdentityWorldPresentationTx(tx, template, &identity); err != nil {
		return err
	}
	if err := createSharedChannelIdentityCopiesTx(tx, &identity, template); err != nil {
		return err
	}
	if err := promoteSharedChannelIdentityVariantsTx(tx, &identity, template); err != nil {
		return err
	}
	if err := tx.Where("shared_identity_id = ? AND user_id = ?", template.ID, ownerUserID).
		Order("channel_id ASC, created_at ASC").Find(&result.Copies).Error; err != nil {
		return err
	}
	result.Template = template
	return nil
}

func SharedChannelIdentityCreateFromCopy(ownerUserID, operatorUserID, identityID string) (*SharedChannelIdentitySyncResult, error) {
	if err := ensureSharedChannelIdentityOwner(ownerUserID, operatorUserID); err != nil {
		return nil, err
	}
	var result SharedChannelIdentitySyncResult
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		return sharedChannelIdentityCreateFromCopyTx(tx, ownerUserID, identityID, &result)
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func sharedChannelIdentityInputMatchesCopy(identity *model.ChannelIdentityModel, template *model.SharedChannelIdentityModel, input *ChannelIdentityInput) bool {
	if identity == nil || template == nil || input == nil {
		return false
	}
	if strings.TrimSpace(input.DisplayName) != template.DisplayName || model.ChannelIdentityNormalizeColor(input.Color) != template.Color ||
		strings.TrimSpace(input.AvatarAttachmentID) != template.AvatarAttachmentID ||
		!sharedAvatarDecorationsEqual(input.AvatarDecorations, template.AvatarDecorations) {
		return false
	}
	if input.TheaterPresentationSet || input.TheaterPresentation != nil {
		return reflect.DeepEqual(input.TheaterPresentation, identity.TheaterPresentation)
	}
	return true
}

func sharedAvatarDecorationsEqual(left, right protocol.AvatarDecorationList) bool {
	if len(left) != len(right) {
		return false
	}
	if len(left) == 0 {
		return true
	}
	return reflect.DeepEqual(left, right)
}

func validateDelegatedSharedChannelIdentityInput(identity *model.ChannelIdentityModel, input *ChannelIdentityInput) error {
	template, err := model.SharedChannelIdentityGetByID(identity.SharedIdentityID)
	if err != nil {
		return err
	}
	if !sharedChannelIdentityInputMatchesCopy(identity, template, input) {
		return ErrSharedChannelIdentitySynchronizedFieldsReadOnly
	}
	return nil
}

func SharedChannelIdentitySyncFromCopy(ownerUserID, operatorUserID, identityID string, input *SharedChannelIdentitySyncInput) (*SharedChannelIdentitySyncResult, error) {
	if err := ensureSharedChannelIdentityOwner(ownerUserID, operatorUserID); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, errors.New("参数不能为空")
	}
	validationInput := &ChannelIdentityInput{DisplayName: input.DisplayName, Color: input.Color, AvatarAttachmentID: input.AvatarAttachmentID, TheaterPresentation: input.TheaterPresentation}
	if err := validateIdentityInput(validationInput); err != nil {
		return nil, err
	}
	identity, err := model.ChannelIdentityGetByID(strings.TrimSpace(identityID))
	if err != nil {
		return nil, err
	}
	if identity.UserID != ownerUserID {
		return nil, gorm.ErrRecordNotFound
	}
	if identity.SharedIdentityID == "" {
		return nil, ErrChannelIdentityNotShared
	}
	if err := ensureSharedChannelIdentityEligible(identity); err != nil {
		return nil, err
	}
	if err := ensureAttachmentAccessible(ownerUserID, operatorUserID, identity.ChannelID, input.AvatarAttachmentID); err != nil {
		return nil, err
	}
	avatarDecorations, err := NormalizeAvatarDecorationsWithAccess(ownerUserID, operatorUserID, identity.ChannelID, input.AvatarDecorations)
	if err != nil {
		return nil, err
	}
	if input.TheaterPresentationSet && input.TheaterPresentation != nil {
		if err := ValidateTheaterPresentationAppearanceAssets(model.GetDB(), identity.ChannelID, ownerUserID, identity.ID, *input.TheaterPresentation); err != nil {
			return nil, err
		}
	}

	result := &SharedChannelIdentitySyncResult{}
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		var template model.SharedChannelIdentityModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", identity.SharedIdentityID, ownerUserID).Take(&template).Error; err != nil {
			return err
		}
		worldID, err := sharedChannelIdentityRequireWorldIDTx(tx, identity.ChannelID)
		if err != nil {
			return err
		}
		if template.WorldID != worldID {
			return ErrSharedChannelIdentityWorldRequired
		}
		template.DisplayName = strings.TrimSpace(validationInput.DisplayName)
		template.Color = validationInput.Color
		template.AvatarAttachmentID = strings.TrimSpace(input.AvatarAttachmentID)
		template.AvatarDecorations = append(protocol.AvatarDecorationList(nil), avatarDecorations...)
		if input.TheaterPresentationSet {
			template.TheaterPresentation = cloneTheaterPresentation(input.TheaterPresentation)
			template.SourceChannelID, template.SourceIdentityID = identity.ChannelID, identity.ID
		}
		template.Revision++
		template.SharedDataJSON, err = serializeSharedChannelIdentityData(template.SharedDataJSON, &template)
		if err != nil {
			return err
		}
		if err := tx.Model(&template).Select("world_id", "source_channel_id", "source_identity_id", "display_name", "color", "avatar_attachment_id", "avatar_decoration", "theater_presentation", "shared_data_json", "revision").Updates(&template).Error; err != nil {
			return err
		}
		if input.TheaterPresentationSet {
			source := *identity
			source.TheaterPresentation = cloneTheaterPresentation(input.TheaterPresentation)
			if err := upsertSharedChannelIdentityWorldPresentationTx(tx, &template, &source); err != nil {
				return err
			}
		}
		var copies []*model.ChannelIdentityModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("shared_identity_id = ? AND user_id = ?", template.ID, ownerUserID).
			Order("channel_id ASC, created_at ASC").Find(&copies).Error; err != nil {
			return err
		}
		for _, copy := range copies {
			copyWorldID, err := sharedChannelIdentityRequireWorldIDTx(tx, copy.ChannelID)
			if err != nil {
				return err
			}
			if copyWorldID != template.WorldID {
				return fmt.Errorf("共享角色副本跨世界: %s", copy.ID)
			}
			if sharedChannelIdentityCopyRepairHook != nil {
				if err := sharedChannelIdentityCopyRepairHook(copy); err != nil {
					return err
				}
			}
			presentation, err := mapSharedTheaterPresentationTx(tx, template.SourceChannelID, template.SourceIdentityID, copy, template.TheaterPresentation)
			if err != nil {
				return err
			}
			values, err := sharedChannelIdentityCopyValues(&template, presentation)
			if err != nil {
				return err
			}
			if err := tx.Model(copy).Updates(values).Error; err != nil {
				return err
			}
			if err := tx.Where("id = ?", copy.ID).Take(copy).Error; err != nil {
				return err
			}
			result.Copies = append(result.Copies, copy)
		}
		result.Template = &template
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SharedChannelIdentityTheaterPresentationSet updates the shared theater
// authority and every channel projection in one transaction. Generic channel
// identity updates deliberately do not own this field.
func SharedChannelIdentityTheaterPresentationSet(ownerUserID, operatorUserID, identityID, channelID string, presentation *protocol.TheaterPresentation, expectedRevision int64) (*SharedChannelIdentitySyncResult, error) {
	if err := ensureSharedChannelIdentityOwner(ownerUserID, operatorUserID); err != nil {
		return nil, err
	}
	identity, err := model.ChannelIdentityValidateOwnership(strings.TrimSpace(identityID), ownerUserID, strings.TrimSpace(channelID))
	if err != nil {
		return nil, err
	}
	if identity.SharedIdentityID == "" {
		return nil, ErrChannelIdentityNotShared
	}
	if presentation != nil {
		if err := protocol.ValidateTheaterPresentation(*presentation); err != nil {
			return nil, fmt.Errorf("演出外观无效: %w", err)
		}
		if err := ValidateTheaterPresentationAppearanceAssets(model.GetDB(), identity.ChannelID, ownerUserID, identity.ID, *presentation); err != nil {
			return nil, err
		}
	}

	var affectedAssetIDs []string
	if copies, copiesErr := model.SharedChannelIdentityCopies(identity.SharedIdentityID); copiesErr == nil {
		for _, copy := range copies {
			affectedAssetIDs = append(affectedAssetIDs, theaterPresentationAssetIDs(copy.TheaterPresentation)...)
		}
	}

	result := &SharedChannelIdentitySyncResult{}
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		var source model.ChannelIdentityModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND channel_id = ? AND shared_identity_id = ?", identity.ID, ownerUserID, identity.ChannelID, identity.SharedIdentityID).
			Take(&source).Error; err != nil {
			return err
		}
		var template model.SharedChannelIdentityModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", identity.SharedIdentityID, ownerUserID).Take(&template).Error; err != nil {
			return err
		}
		if expectedRevision > 0 && template.Revision != expectedRevision {
			return ErrSharedChannelIdentityRevisionConflict
		}
		worldID, err := sharedChannelIdentityRequireWorldIDTx(tx, source.ChannelID)
		if err != nil {
			return err
		}
		if template.WorldID != worldID {
			return ErrSharedChannelIdentityWorldRequired
		}

		template.TheaterPresentation = cloneTheaterPresentation(presentation)
		template.SourceChannelID = source.ChannelID
		template.SourceIdentityID = source.ID
		template.Revision++
		template.SharedDataJSON, err = serializeSharedChannelIdentityData(template.SharedDataJSON, &template)
		if err != nil {
			return err
		}
		if err := tx.Model(&template).
			Select("source_channel_id", "source_identity_id", "theater_presentation", "shared_data_json", "revision").
			Updates(&template).Error; err != nil {
			return err
		}
		source.TheaterPresentation = cloneTheaterPresentation(presentation)
		if err := upsertSharedChannelIdentityWorldPresentationTx(tx, &template, &source); err != nil {
			return err
		}

		var copies []*model.ChannelIdentityModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("shared_identity_id = ? AND user_id = ?", template.ID, ownerUserID).
			Order("channel_id ASC, created_at ASC").Find(&copies).Error; err != nil {
			return err
		}
		for _, copy := range copies {
			if sharedChannelIdentityCopyRepairHook != nil {
				if err := sharedChannelIdentityCopyRepairHook(copy); err != nil {
					return err
				}
			}
			mapped, err := mapSharedTheaterPresentationTx(tx, template.SourceChannelID, template.SourceIdentityID, copy, template.TheaterPresentation)
			if err != nil {
				return err
			}
			values, err := sharedChannelIdentityCopyValues(&template, mapped)
			if err != nil {
				return err
			}
			if err := tx.Model(copy).Updates(map[string]any{
				"theater_presentation": values["theater_presentation"],
				"shared_revision":      values["shared_revision"],
			}).Error; err != nil {
				return err
			}
			if err := tx.Where("id = ?", copy.ID).Take(copy).Error; err != nil {
				return err
			}
			result.Copies = append(result.Copies, copy)
		}
		result.Template = &template
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, copy := range result.Copies {
		affectedAssetIDs = append(affectedAssetIDs, theaterPresentationAssetIDs(copy.TheaterPresentation)...)
	}
	if err := reconcileTheaterAppearanceAssetOrphans(context.Background(), affectedAssetIDs); err != nil {
		log.Printf("共享角色演出保存后资源清理失败[identity=%s]: %v", identity.ID, err)
	}
	return result, nil
}

func SharedChannelIdentityRepairCopy(identityID string) (*model.ChannelIdentityModel, error) {
	copy, err := model.ChannelIdentityGetByID(strings.TrimSpace(identityID))
	if err != nil {
		return nil, err
	}
	if copy.SharedIdentityID == "" {
		return nil, ErrChannelIdentityNotShared
	}
	template, err := model.SharedChannelIdentityGetByID(copy.SharedIdentityID)
	if err != nil {
		return nil, err
	}
	return repairSharedChannelIdentityCopyToRevision(copy.ID, template)
}

func repairSharedChannelIdentityCopyToRevision(identityID string, template *model.SharedChannelIdentityModel) (*model.ChannelIdentityModel, error) {
	var updated model.ChannelIdentityModel
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		var copy model.ChannelIdentityModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND shared_identity_id = ?", identityID, template.ID).Take(&copy).Error; err != nil {
			return err
		}
		if copy.SharedRevision >= template.Revision {
			updated = copy
			return nil
		}
		worldID, err := sharedChannelIdentityRequireWorldIDTx(tx, copy.ChannelID)
		if err != nil {
			return err
		}
		if worldID != template.WorldID {
			return fmt.Errorf("共享角色副本跨世界: %s", copy.ID)
		}
		if sharedChannelIdentityCopyRepairHook != nil {
			if err := sharedChannelIdentityCopyRepairHook(&copy); err != nil {
				return err
			}
		}
		mapped, err := mapSharedTheaterPresentationTx(tx, template.SourceChannelID, template.SourceIdentityID, &copy, template.TheaterPresentation)
		if err != nil {
			return err
		}
		values, err := sharedChannelIdentityCopyValues(template, mapped)
		if err != nil {
			return err
		}
		if err := tx.Model(&copy).Updates(values).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", copy.ID).Take(&updated).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func mapSharedTheaterPresentationTx(tx *gorm.DB, sourceChannelID, sourceIdentityID string, target *model.ChannelIdentityModel, presentation *protocol.TheaterPresentation) (*protocol.TheaterPresentation, error) {
	if presentation == nil {
		return nil, nil
	}
	result := cloneTheaterPresentation(presentation)
	if target.ChannelID == sourceChannelID && target.ID == sourceIdentityID {
		return result, nil
	}
	var worldTemplateRef *protocol.TheaterMediaRef
	worldID, err := sharedChannelIdentityWorldIDTx(tx, target.ChannelID)
	if err != nil {
		return nil, err
	}
	if worldID != "" {
		var world model.WorldModel
		if err := tx.Where("id = ?", worldID).Limit(1).Find(&world).Error; err != nil {
			return nil, err
		}
		if frame := world.GetTheaterPresentationTemplate().Dialogue; frame != nil && frame.Frame != nil {
			ref := frame.Frame.Media
			worldTemplateRef = &ref
		}
	}
	remapLayer := func(layer *protocol.TheaterVisualLayer) error {
		if layer == nil || strings.TrimSpace(layer.Media.AssetID) == "" {
			return nil
		}
		if worldTemplateRef != nil && theaterMediaRefsEqual(layer.Media, *worldTemplateRef) {
			return nil
		}
		mapped, err := mapSharedTheaterAppearanceAssetTx(tx, sourceChannelID, sourceIdentityID, "", target, "", layer.Media)
		if err != nil {
			return err
		}
		layer.Media = mapped
		return nil
	}
	if err := remapLayer(result.Portrait); err != nil {
		return nil, err
	}
	for index := range result.PortraitDecorations {
		if err := remapLayer(&result.PortraitDecorations[index]); err != nil {
			return nil, err
		}
	}
	if err := remapLayer(result.Dialogue.Frame); err != nil {
		return nil, err
	}
	return result, nil
}

func mapSharedTheaterAppearanceAssetTx(tx *gorm.DB, sourceChannelID, sourceIdentityID, sourceVariantID string, target *model.ChannelIdentityModel, targetVariantID string, ref protocol.TheaterMediaRef) (protocol.TheaterMediaRef, error) {
	var source model.TheaterAppearanceAssetModel
	if err := tx.Where("id = ? AND deleted_at IS NULL", ref.AssetID).Take(&source).Error; err != nil {
		return protocol.TheaterMediaRef{}, err
	}
	if source.Status != "ready" || source.ChannelID != sourceChannelID || source.IdentityID != sourceIdentityID || source.VariantID != sourceVariantID || !theaterMediaRefMatchesAsset(ref, source) {
		return protocol.TheaterMediaRef{}, fmt.Errorf("共享角色演出资源无效或作用域不匹配: %s", ref.AssetID)
	}
	canonicalSourceID := source.SharedSourceAssetID
	if canonicalSourceID == "" {
		canonicalSourceID = source.ID
	}
	var existing model.TheaterAppearanceAssetModel
	query := tx.Unscoped().Where("shared_source_asset_id = ? AND shared_target_channel_id = ? AND shared_target_identity_id = ?", canonicalSourceID, target.ChannelID, target.ID)
	if targetVariantID == "" {
		query = query.Where("shared_target_variant_id = '' OR shared_target_variant_id IS NULL")
	} else {
		query = query.Where("shared_target_variant_id = ?", targetVariantID)
	}
	if err := query.Limit(1).Find(&existing).Error; err != nil {
		return protocol.TheaterMediaRef{}, err
	}
	if existing.ID != "" {
		if existing.Status != "ready" {
			return protocol.TheaterMediaRef{}, fmt.Errorf("共享角色演出资源映射未就绪: %s", existing.ID)
		}
		if existing.DeletedAt != nil || existing.OrphanedAt != nil {
			if err := tx.Unscoped().Model(&model.TheaterAppearanceAssetModel{}).Where("id = ?", existing.ID).
				Updates(map[string]any{"deleted_at": nil, "orphaned_at": nil}).Error; err != nil {
				return protocol.TheaterMediaRef{}, err
			}
			existing.DeletedAt = nil
			existing.OrphanedAt = nil
		}
		return theaterAppearanceAssetMediaRef(existing), nil
	}

	newAssetID := utils.NewID()
	attachmentMap := map[string]string{}
	for _, attachmentID := range []string{source.SourceAttachmentID, source.DisplayAttachmentID, source.FallbackAttachmentID} {
		if attachmentID == "" || attachmentMap[attachmentID] != "" {
			continue
		}
		var attachment model.AttachmentModel
		if err := tx.Where("id = ?", attachmentID).Take(&attachment).Error; err != nil {
			return protocol.TheaterMediaRef{}, fmt.Errorf("共享角色演出资源附件不存在 %s: %w", attachmentID, err)
		}
		attachment.StringPKBaseModel = model.StringPKBaseModel{ID: utils.NewID()}
		attachment.ChannelID = target.ChannelID
		attachment.UserID = target.UserID
		attachment.RootID = newAssetID
		attachment.RootIDType = theaterAttachmentRootAppearance
		attachment.ParentID = ""
		attachment.ParentIDType = ""
		if err := tx.Create(&attachment).Error; err != nil {
			return protocol.TheaterMediaRef{}, err
		}
		attachmentMap[attachmentID] = attachment.ID
	}
	clone := source
	clone.StringPKBaseModel = model.StringPKBaseModel{ID: newAssetID}
	clone.ChannelID = target.ChannelID
	clone.OwnerUserID = target.UserID
	clone.IdentityID = target.ID
	clone.VariantID = targetVariantID
	clone.SourceAttachmentID = attachmentMap[source.SourceAttachmentID]
	clone.DisplayAttachmentID = attachmentMap[source.DisplayAttachmentID]
	clone.FallbackAttachmentID = attachmentMap[source.FallbackAttachmentID]
	clone.SharedSourceAssetID = canonicalSourceID
	clone.SharedTargetChannelID = target.ChannelID
	clone.SharedTargetIdentityID = target.ID
	clone.SharedTargetVariantID = targetVariantID
	clone.OrphanedAt = nil
	if err := tx.Create(&clone).Error; err != nil {
		return protocol.TheaterMediaRef{}, err
	}
	return theaterAppearanceAssetMediaRef(clone), nil
}

var sharedChannelIdentityRetryWorkerOnce sync.Once

func scheduleSharedChannelIdentitySyncRetries(sharedIdentityID string, failed map[string]string) error {
	if len(failed) == 0 {
		return nil
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		for copyID, message := range failed {
			item := &model.SharedChannelIdentitySyncRetryModel{
				CopyID: copyID, SharedIdentityID: sharedIdentityID, LastError: message, NextAttemptAt: time.Now(),
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "copy_id"}},
				DoUpdates: clause.Assignments(map[string]any{
					"shared_identity_id": sharedIdentityID, "attempt_count": 0, "last_error": message,
					"next_attempt_at": time.Now(), "deleted_at": nil,
				}),
			}).Create(item).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func StartSharedChannelIdentitySyncRetryWorker() {
	sharedChannelIdentityRetryWorkerOnce.Do(func() {
		go runSharedChannelIdentitySyncRetryWorker()
	})
}

func runSharedChannelIdentitySyncRetryWorker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		retrySharedChannelIdentitySyncBatch()
		<-ticker.C
	}
}

func retrySharedChannelIdentitySyncBatch() {
	var items []*model.SharedChannelIdentitySyncRetryModel
	if err := model.GetDB().Where("next_attempt_at <= ?", time.Now()).Order("next_attempt_at ASC").Limit(50).Find(&items).Error; err != nil {
		log.Printf("共享角色同步重试查询失败: %v", err)
		return
	}
	for _, item := range items {
		if _, err := SharedChannelIdentityRepairCopy(item.CopyID); err == nil {
			_ = model.GetDB().Unscoped().Where("id = ?", item.ID).Delete(&model.SharedChannelIdentitySyncRetryModel{}).Error
			continue
		} else {
			attempts := item.AttemptCount + 1
			delay := time.Minute * time.Duration(1<<min(attempts, 10))
			_ = model.GetDB().Model(item).Updates(map[string]any{
				"attempt_count": attempts, "last_error": err.Error(), "next_attempt_at": time.Now().Add(delay),
			}).Error
		}
	}
}

func theaterAppearanceAssetMediaRef(asset model.TheaterAppearanceAssetModel) protocol.TheaterMediaRef {
	var duration *int64
	if asset.DurationMS > 0 {
		value := asset.DurationMS
		duration = &value
	}
	return protocol.TheaterMediaRef{
		AssetID: asset.ID, ResourceAttachmentID: asset.DisplayAttachmentID, FallbackAttachmentID: asset.FallbackAttachmentID,
		MIMEType: asset.MimeType, Kind: protocol.TheaterMediaKind(asset.Kind), Width: asset.Width, Height: asset.Height, DurationMS: duration,
	}
}

func SharedChannelIdentityDelete(ownerUserID, operatorUserID, identityID string) ([]*model.ChannelIdentityModel, error) {
	if err := ensureSharedChannelIdentityOwner(ownerUserID, operatorUserID); err != nil {
		return nil, err
	}
	var copies []*model.ChannelIdentityModel
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		var identity model.ChannelIdentityModel
		if err := tx.Where("id = ? AND user_id = ?", strings.TrimSpace(identityID), ownerUserID).Take(&identity).Error; err != nil {
			return err
		}
		if identity.SharedIdentityID == "" {
			return ErrChannelIdentityNotShared
		}
		if err := tx.Where("shared_identity_id = ? AND user_id = ?", identity.SharedIdentityID, ownerUserID).Find(&copies).Error; err != nil {
			return err
		}
		ids := make([]string, 0, len(copies))
		for _, copy := range copies {
			ids = append(ids, copy.ID)
			if err := model.ChannelIdentityModeConfigClearIdentityReferencesTx(tx, ownerUserID, copy.ChannelID, copy.ID); err != nil {
				return err
			}
			if err := syncTemporaryIdentityActivateModeTx(tx, ownerUserID, copy.ID, ""); err != nil {
				return err
			}
		}
		if len(ids) > 0 {
			if err := tx.Where("identity_id IN ?", ids).Delete(&model.ChannelIdentityVariantModel{}).Error; err != nil {
				return err
			}
			if err := tx.Where("identity_id IN ?", ids).Delete(&model.ChannelIdentityFolderMemberModel{}).Error; err != nil {
				return err
			}
			if err := tx.Where("identity_id IN ?", ids).Delete(&model.TheaterAppearanceAssetModel{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", ids).Delete(&model.ChannelIdentityModel{}).Error; err != nil {
				return err
			}
		}
		for _, copy := range copies {
			if !copy.IsDefault {
				continue
			}
			var fallback model.ChannelIdentityModel
			if err := tx.Where("channel_id = ? AND user_id = ?", copy.ChannelID, ownerUserID).
				Order("sort_order ASC, created_at ASC").Limit(1).Find(&fallback).Error; err != nil {
				return err
			}
			if fallback.ID != "" {
				if err := tx.Model(&fallback).Update("is_default", true).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Where("shared_identity_id = ?", identity.SharedIdentityID).Delete(&model.SharedChannelIdentityWorldPresentationModel{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("shared_identity_id = ?", identity.SharedIdentityID).Delete(&model.SharedChannelIdentitySyncRetryModel{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", identity.SharedIdentityID).Delete(&model.SharedChannelIdentityModel{}).Error
	})
	return copies, err
}
