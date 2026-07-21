package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/utils"
)

const (
	TheaterAppearanceAssetErrorNotFound      = "ASSET_NOT_FOUND"
	TheaterAppearanceAssetErrorInUse         = "ASSET_IN_USE"
	TheaterAppearanceAssetErrorNotReady      = "ASSET_NOT_READY"
	TheaterAppearanceAssetErrorScopeMismatch = "ASSET_SCOPE_MISMATCH"
	TheaterAppearanceAssetErrorInvalid       = "ASSET_METADATA_INVALID"
)

type TheaterAppearanceAssetUploadInput struct {
	Reader       io.Reader
	Filename     string
	Size         int64
	Purpose      string
	IdentityID   string
	VariantID    string
	TargetUserID string
}

type TheaterAppearanceAssetAttachmentInput struct {
	AttachmentID string
	Purpose      string
	IdentityID   string
	VariantID    string
	TargetUserID string
}

func CreateTheaterAppearanceAssetFromAttachment(ctx context.Context, operatorUserID, channelID string, input TheaterAppearanceAssetAttachmentInput) (*TheaterAppearanceAssetPublic, error) {
	actor, err := ResolveChannelIdentityActor(channelID, operatorUserID, input.TargetUserID)
	if err != nil {
		return nil, err
	}
	purpose := strings.TrimSpace(input.Purpose)
	if purpose != "portrait" && purpose != "portrait-decoration" && purpose != "dialogue-frame" {
		return nil, newTheaterError(TheaterAppearanceAssetErrorInvalid, "purpose 无效", 400, nil)
	}
	identityID := strings.TrimSpace(input.IdentityID)
	if _, err := model.ChannelIdentityValidateOwnership(identityID, actor.TargetUserID, channelID); err != nil {
		return nil, newTheaterError(TheaterAppearanceAssetErrorScopeMismatch, "identity 不属于目标用户或频道", 400, nil)
	}
	variantID := strings.TrimSpace(input.VariantID)
	if variantID != "" {
		variant, variantErr := model.ChannelIdentityVariantGetByID(variantID)
		if variantErr != nil || variant.IdentityID != identityID || variant.ChannelID != channelID || variant.UserID != actor.TargetUserID {
			return nil, newTheaterError(TheaterAppearanceAssetErrorScopeMismatch, "variant 不属于目标 identity", 400, nil)
		}
	}
	resolvedAttachment, err := ResolveAttachmentAccessible(actor.TargetUserID, actor.OperatorUserID, channelID, strings.TrimSpace(input.AttachmentID))
	if err != nil {
		return nil, err
	}
	if resolvedAttachment == nil || !isAppearanceSourceTypeAllowed(resolvedAttachment.MimeType) {
		return nil, newTheaterError(TheaterAppearanceAssetErrorInvalid, "附件不可用于演出资源", 400, nil)
	}
	assetID := utils.NewID()
	attachment := *resolvedAttachment
	if attachment.ChannelID != channelID || attachment.UserID != actor.TargetUserID {
		attachment.StringPKBaseModel = model.StringPKBaseModel{ID: utils.NewID()}
		attachment.ChannelID = channelID
		attachment.UserID = actor.TargetUserID
		attachment.RootID = assetID
		attachment.RootIDType = theaterAttachmentRootAppearance
		if err := model.GetDB().Create(&attachment).Error; err != nil {
			return nil, err
		}
	}
	kind := "static_image"
	if attachment.IsAnimated || attachment.MimeType == "video/webm" {
		kind = "animated_image"
	}
	asset := model.TheaterAppearanceAssetModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: assetID},
		ChannelID:         channelID, OwnerUserID: actor.TargetUserID, IdentityID: identityID, VariantID: variantID, Purpose: purpose,
		SourceAttachmentID: attachment.ID, Kind: kind, SourceMimeType: attachment.MimeType, OriginalFilename: attachment.Filename,
		SizeBytes: attachment.Size, ContentHash: hex.EncodeToString(attachment.Hash), Status: "pending", CreatedBy: operatorUserID,
	}
	if err := model.GetDB().Create(&asset).Error; err != nil {
		return nil, err
	}
	theaterMedia.enqueueAppearance(asset.ID)
	public := theaterAppearanceAssetPublic(asset)
	return &public, nil
}

type TheaterAppearanceAssetPublic struct {
	ID             string                    `json:"id"`
	ChannelID      string                    `json:"channelId"`
	OwnerUserID    string                    `json:"ownerUserId"`
	IdentityID     string                    `json:"identityId"`
	VariantID      string                    `json:"variantId,omitempty"`
	Purpose        string                    `json:"purpose"`
	Status         string                    `json:"status"`
	Progress       float64                   `json:"progress"`
	FailureCode    string                    `json:"failureCode,omitempty"`
	FailureMessage string                    `json:"failureMessage,omitempty"`
	Media          *protocol.TheaterMediaRef `json:"media,omitempty"`
	CreatedAt      time.Time                 `json:"createdAt"`
	UpdatedAt      time.Time                 `json:"updatedAt"`
}

func CreateTheaterAppearanceAssetUpload(ctx context.Context, operatorUserID, channelID string, input TheaterAppearanceAssetUploadInput) (*TheaterAppearanceAssetPublic, error) {
	actor, err := ResolveChannelIdentityActor(channelID, operatorUserID, input.TargetUserID)
	if err != nil {
		return nil, err
	}
	purpose := strings.TrimSpace(input.Purpose)
	if purpose != "portrait" && purpose != "portrait-decoration" && purpose != "dialogue-frame" {
		return nil, newTheaterError(TheaterAppearanceAssetErrorInvalid, "purpose 无效", 400, nil)
	}
	identityID := strings.TrimSpace(input.IdentityID)
	if identityID == "" {
		return nil, newTheaterError(TheaterAppearanceAssetErrorInvalid, "identityId 必填", 400, nil)
	}
	if _, err := model.ChannelIdentityValidateOwnership(identityID, actor.TargetUserID, channelID); err != nil {
		return nil, newTheaterError(TheaterAppearanceAssetErrorScopeMismatch, "identity 不属于目标用户或频道", 400, nil)
	}
	variantID := strings.TrimSpace(input.VariantID)
	if variantID != "" {
		variant, err := model.ChannelIdentityVariantGetByID(variantID)
		if err != nil || variant.IdentityID != identityID || variant.ChannelID != channelID || variant.UserID != actor.TargetUserID {
			return nil, newTheaterError(TheaterAppearanceAssetErrorScopeMismatch, "variant 不属于目标 identity", 400, nil)
		}
	}
	if input.Reader == nil {
		return nil, newTheaterError(TheaterAppearanceAssetErrorInvalid, "file 必填", 400, nil)
	}
	config := normalizeTheaterMediaConfig(theaterMedia.config)
	maxBytes := config.VideoMaxSizeMB << 20
	if maxBytes < config.ImageMaxSizeMB<<20 {
		maxBytes = config.ImageMaxSizeMB << 20
	}
	temp, err := os.CreateTemp("", "sealchat-appearance-upload-*")
	if err != nil {
		return nil, err
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()
	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(temp, hasher), io.LimitReader(input.Reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if written == 0 || written > maxBytes {
		return nil, newTheaterError(TheaterMediaErrorLimitExceeded, "资源文件大小超限", 413, map[string]any{"limitBytes": maxBytes})
	}
	if err := temp.Close(); err != nil {
		return nil, err
	}
	head, err := readFilePrefix(tempPath, 4096)
	if err != nil {
		return nil, err
	}
	mimeType, kind := detectTheaterMediaType(head)
	if mimeType == "video/webm" {
		kind = "animated_image"
	}
	if !isAppearanceSourceTypeAllowed(mimeType) {
		return nil, newTheaterError(TheaterMediaErrorUnsupported, "不支持演出资源格式", 415, nil)
	}
	hashBytes := hasher.Sum(nil)
	location, err := PersistAttachmentFile(hashBytes, written, tempPath, mimeType)
	if err != nil {
		return nil, err
	}
	assetID := utils.NewID()
	attachment := model.AttachmentModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
		Hash:              hashBytes, Filename: sanitizeTheaterFilename(input.Filename), Size: written, MimeType: mimeType,
		IsAnimated: kind == "animated_image", UserID: actor.TargetUserID, ChannelID: channelID,
		StorageType: location.StorageType, ObjectKey: location.ObjectKey, ExternalURL: location.ExternalURL,
		RootID: assetID, RootIDType: theaterAttachmentRootAppearance, IsTemp: false,
	}
	asset := model.TheaterAppearanceAssetModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: assetID},
		ChannelID:         channelID, OwnerUserID: actor.TargetUserID, IdentityID: identityID, VariantID: variantID, Purpose: purpose,
		SourceAttachmentID: attachment.ID, Kind: kind, SourceMimeType: mimeType, OriginalFilename: attachment.Filename,
		SizeBytes: written, ContentHash: hex.EncodeToString(hashBytes), Status: "pending", CreatedBy: operatorUserID,
	}
	if err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&attachment).Error; err != nil {
			return err
		}
		return tx.Create(&asset).Error
	}); err != nil {
		return nil, err
	}
	theaterMedia.enqueueAppearance(asset.ID)
	public := theaterAppearanceAssetPublic(asset)
	return &public, nil
}

func GetTheaterAppearanceAsset(ctx context.Context, operatorUserID, channelID, assetID string) (*TheaterAppearanceAssetPublic, error) {
	asset, err := findTheaterAppearanceAsset(channelID, assetID)
	if err != nil {
		return nil, err
	}
	if _, err := ResolveChannelIdentityActor(channelID, operatorUserID, asset.OwnerUserID); err != nil {
		return nil, err
	}
	public := theaterAppearanceAssetPublic(*asset)
	return &public, nil
}

func DeleteTheaterAppearanceAsset(ctx context.Context, operatorUserID, channelID, assetID string) error {
	asset, err := findTheaterAppearanceAsset(channelID, assetID)
	if err != nil {
		return err
	}
	if _, err := ResolveChannelIdentityActor(channelID, operatorUserID, asset.OwnerUserID); err != nil {
		return err
	}
	inUse, err := TheaterAppearanceAssetInUse(model.GetDB(), asset.ID)
	if err != nil {
		return err
	}
	if inUse {
		return newTheaterError(TheaterAppearanceAssetErrorInUse, "演出资源仍被引用", 409, nil)
	}
	now := time.Now()
	return model.GetDB().Model(&model.TheaterAppearanceAssetModel{}).
		Where("id = ? AND deleted_at IS NULL", asset.ID).
		Updates(map[string]any{"orphaned_at": &now, "deleted_at": &now}).Error
}

func findTheaterAppearanceAsset(channelID, assetID string) (*model.TheaterAppearanceAssetModel, error) {
	var asset model.TheaterAppearanceAssetModel
	if err := model.GetDB().Where("id = ? AND channel_id = ? AND deleted_at IS NULL", strings.TrimSpace(assetID), strings.TrimSpace(channelID)).Limit(1).Find(&asset).Error; err != nil {
		return nil, err
	}
	if asset.ID == "" {
		return nil, newTheaterError(TheaterAppearanceAssetErrorNotFound, "演出资源不存在", 404, nil)
	}
	return &asset, nil
}

func theaterAppearanceAssetPublic(asset model.TheaterAppearanceAssetModel) TheaterAppearanceAssetPublic {
	result := TheaterAppearanceAssetPublic{
		ID: asset.ID, ChannelID: asset.ChannelID, OwnerUserID: asset.OwnerUserID, IdentityID: asset.IdentityID,
		VariantID: asset.VariantID, Purpose: asset.Purpose, Status: asset.Status, Progress: asset.Progress,
		FailureCode: asset.FailureCode, FailureMessage: asset.FailureMessage, CreatedAt: asset.CreatedAt, UpdatedAt: asset.UpdatedAt,
	}
	if asset.Status == "ready" {
		var duration *int64
		if asset.DurationMS > 0 {
			value := asset.DurationMS
			duration = &value
		}
		result.Media = &protocol.TheaterMediaRef{
			AssetID: asset.ID, ResourceAttachmentID: asset.DisplayAttachmentID, FallbackAttachmentID: asset.FallbackAttachmentID,
			MIMEType: asset.MimeType, Kind: protocol.TheaterMediaKind(asset.Kind), Width: asset.Width, Height: asset.Height, DurationMS: duration,
		}
	}
	return result
}

func isAppearanceSourceTypeAllowed(mimeType string) bool {
	switch mimeType {
	case "image/png", "image/jpeg", "image/webp", "image/gif", "image/apng", "video/webm":
		return true
	default:
		return false
	}
}

func (service *theaterMediaService) enqueueAppearance(assetID string) {
	if strings.TrimSpace(assetID) == "" || service.appearanceQueue == nil {
		return
	}
	select {
	case service.appearanceQueue <- assetID:
	default:
		go func() {
			select {
			case service.appearanceQueue <- assetID:
			case <-time.After(time.Second):
			}
		}()
	}
}

func (service *theaterMediaService) scanPendingAppearanceAssets(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		var ids []string
		_ = model.GetDB().Model(&model.TheaterAppearanceAssetModel{}).
			Where("status IN ? AND deleted_at IS NULL", []string{"pending", "processing"}).Limit(100).Pluck("id", &ids).Error
		for _, id := range ids {
			service.enqueueAppearance(id)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func persistTheaterAppearanceOutput(asset *model.TheaterAppearanceAssetModel, output VisualMediaOutput) (string, error) {
	if output.IsSource {
		return asset.SourceAttachmentID, nil
	}
	data, err := os.ReadFile(output.Path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	location, err := PersistAttachmentFile(hash[:], int64(len(data)), output.Path, output.MimeType)
	if err != nil {
		return "", err
	}
	attachment := model.AttachmentModel{
		Hash: hash[:], Filename: output.Name + filepath.Ext(output.Path), Size: int64(len(data)), MimeType: output.MimeType,
		IsAnimated: strings.HasPrefix(output.MimeType, "video/"), UserID: asset.OwnerUserID, ChannelID: asset.ChannelID,
		StorageType: location.StorageType, ObjectKey: location.ObjectKey, ExternalURL: location.ExternalURL,
		RootID: asset.ID, RootIDType: theaterAttachmentRootAppearanceVariant, IsTemp: false,
	}
	if tx, _ := model.AttachmentCreate(&attachment); tx.Error != nil {
		return "", tx.Error
	}
	return attachment.ID, nil
}

func theaterAppearanceFailure(assetID, code string, err error) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	_ = model.GetDB().Model(&model.TheaterAppearanceAssetModel{}).Where("id = ?", assetID).
		Updates(map[string]any{"status": "failed", "progress": 0, "failure_code": code, "failure_message": message}).Error
}

func classifyTheaterAppearanceError(err error) string {
	if err == nil {
		return TheaterMediaErrorTranscodeFailed
	}
	message := err.Error()
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(message), "deadline exceeded") {
		return TheaterAppearanceErrorProcessingTimeout
	}
	if strings.Contains(message, TheaterMediaErrorProcessorUnavailable) || strings.Contains(message, TheaterAppearanceErrorProcessorUnavailable) {
		return TheaterAppearanceErrorProcessorUnavailable
	}
	for _, code := range []string{
		TheaterMediaErrorProbeFailed,
		TheaterMediaErrorLimitExceeded, TheaterAppearanceErrorAudioNotAllowed,
		TheaterAppearanceErrorAlphaRequired, TheaterAppearanceErrorCodecUnsupported, "IMAGE_DECODE_FAILED", TheaterMediaErrorUnsupported,
	} {
		if strings.Contains(message, code) {
			return code
		}
	}
	return TheaterMediaErrorTranscodeFailed
}

func ValidateTheaterPresentationAppearanceAssets(tx *gorm.DB, channelID, ownerUserID, identityID string, presentation protocol.TheaterPresentation) error {
	refs := filterWorldTheaterTemplateMediaRefs(tx, channelID, theaterPresentationMediaRefs(presentation))
	if identityID == "" && len(refs) > 0 {
		return newTheaterError(TheaterAppearanceAssetErrorScopeMismatch, "新 identity 尚不能引用已有演出资源", 400, nil)
	}
	return validateTheaterAppearanceMediaRefs(tx, channelID, ownerUserID, identityID, refs)
}

func ValidateTheaterPresentationPatchAppearanceAssets(tx *gorm.DB, channelID, ownerUserID, identityID string, patch protocol.TheaterPresentationPatch) error {
	return validateTheaterPresentationPatchAppearanceAssets(tx, channelID, ownerUserID, identityID, "", false, patch)
}

func ValidateTheaterPresentationPatchAppearanceAssetsForVariant(tx *gorm.DB, channelID, ownerUserID, identityID, variantID string, patch protocol.TheaterPresentationPatch) error {
	return validateTheaterPresentationPatchAppearanceAssets(tx, channelID, ownerUserID, identityID, variantID, true, patch)
}

func validateTheaterPresentationPatchAppearanceAssets(tx *gorm.DB, channelID, ownerUserID, identityID, variantID string, enforceVariant bool, patch protocol.TheaterPresentationPatch) error {
	var refs []protocol.TheaterMediaRef
	if patch.Portrait.Set && patch.Portrait.Value != nil {
		refs = append(refs, patch.Portrait.Value.Media)
	}
	if patch.PortraitDecorations.Set && patch.PortraitDecorations.Value != nil {
		for _, layer := range *patch.PortraitDecorations.Value {
			refs = append(refs, layer.Media)
		}
	}
	if patch.Dialogue.Set && patch.Dialogue.Value != nil && patch.Dialogue.Value.Frame != nil {
		refs = append(refs, patch.Dialogue.Value.Frame.Media)
	}
	refs = filterWorldTheaterTemplateMediaRefs(tx, channelID, refs)
	if err := validateTheaterAppearanceMediaRefs(tx, channelID, ownerUserID, identityID, refs); err != nil {
		return err
	}
	if !enforceVariant {
		return nil
	}
	for _, ref := range refs {
		var asset model.TheaterAppearanceAssetModel
		if err := tx.Where("id = ? AND deleted_at IS NULL", ref.AssetID).Limit(1).Find(&asset).Error; err != nil {
			return err
		}
		if asset.ID == "" || asset.VariantID != variantID {
			return newTheaterError(TheaterAppearanceAssetErrorScopeMismatch, "演出资源不属于目标差分", 400, nil)
		}
	}
	return nil
}

func theaterPresentationMediaRefs(presentation protocol.TheaterPresentation) []protocol.TheaterMediaRef {
	refs := make([]protocol.TheaterMediaRef, 0, len(presentation.PortraitDecorations)+2)
	if presentation.Portrait != nil {
		refs = append(refs, presentation.Portrait.Media)
	}
	for _, layer := range presentation.PortraitDecorations {
		refs = append(refs, layer.Media)
	}
	if presentation.Dialogue.Frame != nil {
		refs = append(refs, presentation.Dialogue.Frame.Media)
	}
	return refs
}

func theaterMediaRefsEqual(left, right protocol.TheaterMediaRef) bool {
	if left.AssetID != right.AssetID || left.ResourceAttachmentID != right.ResourceAttachmentID ||
		left.FallbackAttachmentID != right.FallbackAttachmentID || left.Kind != right.Kind ||
		left.MIMEType != right.MIMEType || left.Width != right.Width || left.Height != right.Height {
		return false
	}
	if left.DurationMS == nil || right.DurationMS == nil {
		return left.DurationMS == nil && right.DurationMS == nil
	}
	return *left.DurationMS == *right.DurationMS
}

func filterWorldTheaterTemplateMediaRefs(tx *gorm.DB, channelID string, refs []protocol.TheaterMediaRef) []protocol.TheaterMediaRef {
	if len(refs) == 0 {
		return refs
	}
	if tx == nil {
		tx = model.GetDB()
	}
	var channel model.ChannelModel
	if err := tx.Where("id = ?", strings.TrimSpace(channelID)).Limit(1).Find(&channel).Error; err != nil || channel.ID == "" || channel.WorldID == "" {
		return refs
	}
	var world model.WorldModel
	if err := tx.Where("id = ? AND status = ?", channel.WorldID, "active").Limit(1).Find(&world).Error; err != nil || world.ID == "" {
		return refs
	}
	frame := world.GetTheaterPresentationTemplate().Dialogue
	if frame == nil || frame.Frame == nil {
		return refs
	}
	templateRef := frame.Frame.Media
	filtered := make([]protocol.TheaterMediaRef, 0, len(refs))
	for _, ref := range refs {
		if !theaterMediaRefsEqual(ref, templateRef) {
			filtered = append(filtered, ref)
		}
	}
	return filtered
}

func theaterMediaRefMatchesAsset(ref protocol.TheaterMediaRef, asset model.TheaterAppearanceAssetModel) bool {
	return ref.ResourceAttachmentID == asset.DisplayAttachmentID && ref.FallbackAttachmentID == asset.FallbackAttachmentID &&
		string(ref.Kind) == asset.Kind && ref.MIMEType == asset.MimeType && ref.Width == asset.Width && ref.Height == asset.Height &&
		((ref.DurationMS == nil && asset.DurationMS == 0) || ref.DurationMS != nil && *ref.DurationMS == asset.DurationMS)
}

func validateTheaterAppearanceMediaRefs(tx *gorm.DB, channelID, ownerUserID, identityID string, refs []protocol.TheaterMediaRef) error {
	if tx == nil {
		tx = model.GetDB()
	}
	for _, ref := range refs {
		var asset model.TheaterAppearanceAssetModel
		if err := tx.Where("id = ? AND deleted_at IS NULL", ref.AssetID).Limit(1).Find(&asset).Error; err != nil {
			return err
		}
		if asset.ID == "" || asset.ChannelID != channelID || asset.OwnerUserID != ownerUserID || identityID != "" && asset.IdentityID != identityID {
			return newTheaterError(TheaterAppearanceAssetErrorScopeMismatch, "演出资源不属于目标用户或频道", 400, nil)
		}
		if asset.Status != "ready" {
			return newTheaterError(TheaterAppearanceAssetErrorNotReady, "演出资源尚未 ready", 409, nil)
		}
		if !theaterMediaRefMatchesAsset(ref, asset) {
			return newTheaterError(TheaterAppearanceAssetErrorInvalid, "演出资源元数据与服务端记录不一致", 400, nil)
		}
	}
	return nil
}

func reassignTheaterAppearanceAssetsIdentityTx(tx *gorm.DB, oldIdentityID, newIdentityID, channelID, ownerUserID string) error {
	return tx.Model(&model.TheaterAppearanceAssetModel{}).
		Where("identity_id = ? AND channel_id = ? AND owner_user_id = ? AND deleted_at IS NULL", oldIdentityID, channelID, ownerUserID).
		Updates(map[string]any{"identity_id": newIdentityID, "variant_id": ""}).Error
}

func TheaterAppearanceAssetInUse(tx *gorm.DB, assetID string) (bool, error) {
	if tx == nil {
		tx = model.GetDB()
	}
	var variants []model.ChannelIdentityVariantModel
	if err := tx.Select("appearance_json").Where("appearance_json <> ''").Find(&variants).Error; err != nil {
		return false, err
	}
	for _, variant := range variants {
		var document struct {
			TheaterPresentation json.RawMessage `json:"theaterPresentation"`
		}
		if json.Unmarshal([]byte(variant.AppearanceJSON), &document) == nil && rawTheaterPresentationReferencesAsset(document.TheaterPresentation, assetID, true) {
			return true, nil
		}
	}
	if tx.Migrator().HasColumn(&model.WorldModel{}, "theater_presentation_template_json") {
		var templates []string
		if err := tx.Model(&model.WorldModel{}).
			Where("theater_presentation_template_json <> ''").
			Pluck("theater_presentation_template_json", &templates).Error; err != nil {
			return false, err
		}
		for _, raw := range templates {
			var template protocol.WorldTheaterPresentationTemplate
			if json.Unmarshal([]byte(raw), &template) == nil && template.Dialogue != nil && template.Dialogue.Frame != nil && template.Dialogue.Frame.Media.AssetID == assetID {
				return true, nil
			}
		}
	}
	checks := []struct {
		model  any
		column string
	}{
		{model: &model.ChannelIdentityModel{}, column: "theater_presentation"},
		{model: &model.MessageModel{}, column: "sender_theater_presentation"},
	}
	for _, check := range checks {
		if !tx.Migrator().HasColumn(check.model, check.column) {
			continue
		}
		var values []string
		if err := tx.Model(check.model).Where(check.column+" IS NOT NULL AND "+check.column+" <> ''").Pluck(check.column, &values).Error; err != nil {
			return false, err
		}
		for _, value := range values {
			if rawTheaterPresentationReferencesAsset(json.RawMessage(value), assetID, false) {
				return true, nil
			}
		}
	}
	return false, nil
}

func rawTheaterPresentationReferencesAsset(raw json.RawMessage, assetID string, patch bool) bool {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return false
	}
	if patch {
		var value protocol.TheaterPresentationPatch
		if json.Unmarshal(raw, &value) != nil {
			return false
		}
		refs := make([]protocol.TheaterMediaRef, 0)
		if value.Portrait.Set && value.Portrait.Value != nil {
			refs = append(refs, value.Portrait.Value.Media)
		}
		if value.PortraitDecorations.Set && value.PortraitDecorations.Value != nil {
			for _, layer := range *value.PortraitDecorations.Value {
				refs = append(refs, layer.Media)
			}
		}
		if value.Dialogue.Set && value.Dialogue.Value != nil && value.Dialogue.Value.Frame != nil {
			refs = append(refs, value.Dialogue.Value.Frame.Media)
		}
		return mediaRefsContainAsset(refs, assetID)
	}
	var value protocol.TheaterPresentation
	return json.Unmarshal(raw, &value) == nil && mediaRefsContainAsset(theaterPresentationMediaRefs(value), assetID)
}

func mediaRefsContainAsset(refs []protocol.TheaterMediaRef, assetID string) bool {
	for _, ref := range refs {
		if ref.AssetID == assetID {
			return true
		}
	}
	return false
}

func MarkTheaterAppearanceAssetOrphans(ctx context.Context) error {
	var assets []model.TheaterAppearanceAssetModel
	if err := model.GetDB().Where("deleted_at IS NULL").Find(&assets).Error; err != nil {
		return err
	}
	for _, asset := range assets {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		inUse, err := TheaterAppearanceAssetInUse(model.GetDB(), asset.ID)
		if err != nil {
			return err
		}
		updates := map[string]any{"orphaned_at": nil}
		if !inUse {
			now := time.Now()
			updates["orphaned_at"] = &now
		}
		if err := model.GetDB().Model(&model.TheaterAppearanceAssetModel{}).Where("id = ?", asset.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("mark appearance asset %s orphan: %w", asset.ID, err)
		}
	}
	return nil
}

func runTheaterAppearanceOrphanScanner(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = MarkTheaterAppearanceAssetOrphans(ctx)
		}
	}
}
