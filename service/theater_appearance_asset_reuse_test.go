package service

import (
	"testing"
	"time"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/utils"
)

func reusableAppearanceAsset() model.TheaterAppearanceAssetModel {
	id := utils.NewID()
	return model.TheaterAppearanceAssetModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: id},
		ChannelID:         "reuse-source-channel", OwnerUserID: "reuse-source-owner", IdentityID: "reuse-source-identity",
		Purpose: "portrait", SourceAttachmentID: id + "-source", DisplayAttachmentID: id + "-display",
		Kind: "static_image", MimeType: "image/png", SourceMimeType: "image/png", Status: "ready", Width: 16, Height: 24,
	}
}

func validateReusableAppearanceAsset(channelID, ownerID, identityID, variantID string, ref protocol.TheaterMediaRef) map[string]func() error {
	layer := protocol.TheaterVisualLayer{Media: ref}
	presentation := protocol.DefaultTheaterPresentation()
	presentation.Portrait = &layer
	patch := protocol.TheaterPresentationPatch{
		Portrait: protocol.TheaterPatchField[protocol.TheaterVisualLayer]{Set: true, Value: &layer},
	}
	return map[string]func() error{
		"base": func() error {
			return ValidateTheaterPresentationAppearanceAssets(model.GetDB(), channelID, ownerID, identityID, presentation)
		},
		"patch": func() error {
			return ValidateTheaterPresentationPatchAppearanceAssets(model.GetDB(), channelID, ownerID, identityID, patch)
		},
		"variant": func() error {
			return ValidateTheaterPresentationPatchAppearanceAssetsForVariant(model.GetDB(), channelID, ownerID, identityID, variantID, patch)
		},
	}
}

func TestTheaterAppearanceAssetReuseAcrossScopes(t *testing.T) {
	initTestDB(t)
	for _, tc := range []struct {
		name, channelID, ownerID, identityID, sourceVariantID, targetVariantID string
	}{
		{"same-scope", "reuse-source-channel", "reuse-source-owner", "reuse-source-identity", "", ""},
		{"identity", "reuse-source-channel", "reuse-source-owner", "other-identity", "", ""},
		{"channel", "other-channel", "reuse-source-owner", "reuse-source-identity", "", ""},
		{"owner", "reuse-source-channel", "other-owner", "reuse-source-identity", "", ""},
		{"base-to-variant", "reuse-source-channel", "reuse-source-owner", "reuse-source-identity", "", "target-variant"},
		{"variant-to-base", "reuse-source-channel", "reuse-source-owner", "reuse-source-identity", "source-variant", ""},
		{"variant-to-variant", "reuse-source-channel", "reuse-source-owner", "reuse-source-identity", "source-variant", "target-variant"},
		{"all", "other-channel", "other-owner", "other-identity", "source-variant", "target-variant"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			asset := reusableAppearanceAsset()
			asset.VariantID = tc.sourceVariantID
			if err := model.GetDB().Create(&asset).Error; err != nil {
				t.Fatal(err)
			}
			for name, validate := range validateReusableAppearanceAsset(tc.channelID, tc.ownerID, tc.identityID, tc.targetVariantID, theaterAppearanceAssetMediaRef(asset)) {
				t.Run(name, func(t *testing.T) {
					if err := validate(); err != nil {
						t.Fatalf("cross-scope asset reuse rejected: %v", err)
					}
				})
			}
		})
	}
}

func TestTheaterAppearanceAssetReuseRejectsInvalidMedia(t *testing.T) {
	initTestDB(t)
	for _, tc := range []struct {
		name, status, errorCode string
		deleted                 bool
		changeRef               func(*protocol.TheaterMediaRef)
	}{
		{name: "missing", status: "ready", errorCode: TheaterAppearanceAssetErrorNotFound, changeRef: func(ref *protocol.TheaterMediaRef) { ref.AssetID = "missing" }},
		{name: "deleted", status: "ready", deleted: true, errorCode: TheaterAppearanceAssetErrorNotFound},
		{name: "pending", status: "pending", errorCode: TheaterAppearanceAssetErrorNotReady},
		{name: "processing", status: "processing", errorCode: TheaterAppearanceAssetErrorNotReady},
		{name: "failed", status: "failed", errorCode: TheaterAppearanceAssetErrorNotReady},
		{name: "attachment", status: "ready", errorCode: TheaterAppearanceAssetErrorInvalid, changeRef: func(ref *protocol.TheaterMediaRef) { ref.ResourceAttachmentID = "forged" }},
		{name: "metadata", status: "ready", errorCode: TheaterAppearanceAssetErrorInvalid, changeRef: func(ref *protocol.TheaterMediaRef) { ref.Width++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			asset := reusableAppearanceAsset()
			asset.Status = tc.status
			if tc.deleted {
				now := time.Now()
				asset.DeletedAt = &now
			}
			if err := model.GetDB().Create(&asset).Error; err != nil {
				t.Fatal(err)
			}
			ref := theaterAppearanceAssetMediaRef(asset)
			if tc.changeRef != nil {
				tc.changeRef(&ref)
			}
			for name, validate := range validateReusableAppearanceAsset("other-channel", "other-owner", "other-identity", "other-variant", ref) {
				t.Run(name, func(t *testing.T) {
					if err := validate(); err == nil || !IsTheaterErrorCode(err, tc.errorCode) {
						t.Fatalf("error = %v, want %s", err, tc.errorCode)
					}
				})
			}
		})
	}
}

func TestSharedTheaterAppearanceAssetReuseAcrossScopes(t *testing.T) {
	initTestDB(t)
	canonical := reusableAppearanceAsset()
	canonical.VariantID = "unrelated-variant"
	if err := model.GetDB().Create(&canonical).Error; err != nil {
		t.Fatal(err)
	}
	source := reusableAppearanceAsset()
	source.SharedSourceAssetID = canonical.ID
	if err := model.GetDB().Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	for _, attachmentID := range []string{canonical.SourceAttachmentID, canonical.DisplayAttachmentID} {
		attachment := model.AttachmentModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: attachmentID},
			ChannelID:         canonical.ChannelID, UserID: canonical.OwnerUserID, MimeType: canonical.MimeType,
		}
		if err := model.GetDB().Create(&attachment).Error; err != nil {
			t.Fatal(err)
		}
	}
	target := model.ChannelIdentityModel{
		ChannelID: "other-channel", UserID: "other-owner", SharedIdentityID: "other-shared-identity",
	}
	if err := model.GetDB().Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	variant := model.ChannelIdentityVariantModel{
		ChannelID: target.ChannelID, UserID: target.UserID, IdentityID: target.ID, SharedVariantID: "other-shared-variant",
	}
	if err := model.GetDB().Create(&variant).Error; err != nil {
		t.Fatal(err)
	}
	for _, variantID := range []string{"", variant.ID} {
		ref := theaterAppearanceAssetMediaRef(source)
		resolved, origin, err := resolveSharedTheaterAppearanceAssetTx(model.GetDB(), target.ChannelID, target.ID, variantID, ref)
		if err != nil {
			t.Fatalf("shared cross-scope asset reuse rejected: %v", err)
		}
		if resolved.ID != source.ID || origin.ID != canonical.ID {
			t.Fatalf("wrong source chain: source=%s canonical=%s", resolved.ID, origin.ID)
		}
		mapped, err := mapSharedTheaterAppearanceAssetTx(model.GetDB(), target.ChannelID, target.ID, variantID, &target, variantID, ref)
		if err != nil {
			t.Fatalf("shared cross-scope mapping rejected: %v", err)
		}
		var stored model.TheaterAppearanceAssetModel
		if err := model.GetDB().Where("id = ?", mapped.AssetID).Take(&stored).Error; err != nil {
			t.Fatal(err)
		}
		if stored.ChannelID != target.ChannelID || stored.OwnerUserID != target.UserID || stored.IdentityID != target.ID || stored.VariantID != variantID || stored.SharedSourceAssetID != canonical.ID {
			t.Fatalf("wrong mapped scope: %#v", stored)
		}
		reused, err := mapSharedTheaterAppearanceAssetTx(model.GetDB(), target.ChannelID, target.ID, variantID, &target, variantID, ref)
		if err != nil || reused.AssetID != mapped.AssetID {
			t.Fatalf("existing mapping not reused: ref=%#v err=%v", reused, err)
		}
	}
	forged := theaterAppearanceAssetMediaRef(source)
	forged.Width++
	if _, err := mapSharedTheaterAppearanceAssetTx(model.GetDB(), target.ChannelID, target.ID, "", &target, "", forged); err == nil {
		t.Fatal("shared reuse accepted forged media metadata")
	}
	if err := model.GetDB().Model(&canonical).Update("status", "processing").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveSharedTheaterAppearanceAssetTx(model.GetDB(), "other-channel", "other-identity", "", theaterAppearanceAssetMediaRef(source)); err == nil {
		t.Fatal("shared reuse accepted an unready canonical asset")
	}
}
