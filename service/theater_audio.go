package service

import (
	"errors"
	"mime/multipart"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"sealchat/model"
)

const theaterFeatureAudioTag = "theater-feature-audio"

type TheaterAudioAssetListResult struct {
	Items []*model.AudioAsset `json:"items"`
	Quota *AudioQuotaSummary  `json:"quota"`
}

func theaterChannelAudioTag(channelID string) string {
	return "theater-channel:" + strings.TrimSpace(channelID)
}

func theaterAudioAssetName(worldName, requestedName, filename string) string {
	name := strings.TrimSpace(requestedName)
	if name == "" {
		name = strings.TrimSuffix(strings.TrimSpace(filepath.Base(filename)), filepath.Ext(filename))
	}
	if name == "" {
		name = "未命名素材"
	}
	name = strings.ReplaceAll(strings.ReplaceAll(name, "\r", " "), "\n", " ")
	prefix := strings.TrimSpace(worldName) + "-特性音频-"
	maximum := 255 - utf8.RuneCountInString(prefix)
	if maximum < 1 {
		maximum = 1
	}
	runes := []rune(name)
	if len(runes) > maximum {
		name = string(runes[:maximum])
	}
	return prefix + name
}

func hasAudioTag(asset *model.AudioAsset, target string) bool {
	if asset == nil {
		return false
	}
	for _, tag := range asset.Tags {
		if tag == target {
			return true
		}
	}
	return false
}

func ListTheaterAudioAssets(actorID, worldID, channelID string) (*TheaterAudioAssetListResult, error) {
	if !CanManageTheaterResources(actorID, worldID, channelID) {
		return nil, newTheaterError(TheaterErrorPermissionDenied, "没有 Theater 素材管理权限", 403, nil)
	}
	worldID = strings.TrimSpace(worldID)
	items, _, err := AudioListAssets(AudioAssetFilters{
		Tags:          []string{theaterFeatureAudioTag, theaterChannelAudioTag(channelID)},
		Page:          1,
		PageSize:      500,
		SortBy:        "updatedAt",
		SortOrder:     "desc",
		Scope:         model.AudioScopeWorld,
		WorldID:       &worldID,
		IncludeCommon: false,
	})
	if err != nil {
		return nil, err
	}
	quota, err := GetAudioQuotaSummary(actorID)
	if err != nil {
		return nil, err
	}
	return &TheaterAudioAssetListResult{Items: items, Quota: quota}, nil
}

func CreateTheaterAudioAsset(actorID, worldID, channelID string, file *multipart.FileHeader, requestedName string) (*model.AudioAsset, error) {
	world, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionResourceUpload)
	if err != nil {
		return nil, err
	}
	if file == nil {
		return nil, theaterPayloadError("file 必填")
	}
	worldID = strings.TrimSpace(worldID)
	asset, err := AudioCreateAssetFromUpload(file, AudioUploadOptions{
		Name:        theaterAudioAssetName(world.Name, requestedName, file.Filename),
		Tags:        []string{theaterFeatureAudioTag, theaterChannelAudioTag(channelID)},
		Description: "小剧场特性音频",
		Visibility:  model.AudioVisibilityRestricted,
		CreatedBy:   actorID,
		Scope:       model.AudioScopeWorld,
		WorldID:     &worldID,
	})
	if err == nil {
		return asset, nil
	}
	var quotaErr *AudioQuotaExceededError
	switch {
	case errors.Is(err, ErrAudioTooLarge):
		return nil, newTheaterError(TheaterErrorResourceLimitExceeded, err.Error(), 413, nil)
	case errors.Is(err, ErrAudioUnsupportedMime):
		return nil, newTheaterError(TheaterMediaErrorUnsupported, err.Error(), 415, nil)
	case errors.As(err, &quotaErr):
		return nil, newTheaterError(TheaterErrorResourceLimitExceeded, quotaErr.Error(), 413, map[string]any{
			"usedBytes": quotaErr.UsedBytes, "quotaBytes": quotaErr.QuotaBytes, "incomingBytes": quotaErr.IncomingBytes,
		})
	default:
		return nil, err
	}
}

func DeleteTheaterAudioAsset(actorID, worldID, channelID, assetID string) error {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionResourceDelete); err != nil {
		return err
	}
	asset, err := AudioGetAsset(strings.TrimSpace(assetID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return newTheaterError(TheaterErrorResourceNotFound, "音频素材不存在", 404, nil)
		}
		return err
	}
	if asset.Scope != model.AudioScopeWorld || asset.WorldID == nil || strings.TrimSpace(*asset.WorldID) != strings.TrimSpace(worldID) ||
		!hasAudioTag(asset, theaterFeatureAudioTag) || !hasAudioTag(asset, theaterChannelAudioTag(channelID)) {
		return newTheaterError(TheaterErrorResourceNotFound, "音频素材不存在", 404, nil)
	}
	referenced, err := theaterAudioAssetReferenced(worldID, asset.ID)
	if err != nil {
		return err
	}
	if referenced {
		return newTheaterError(TheaterErrorResourceInUse, "音频素材仍被小剧场特效引用", 409, nil)
	}
	return AudioSafeDeleteAsset(asset.ID, false)
}

func theaterAudioAssetReferenced(worldID, assetID string) (bool, error) {
	var roomIDs []string
	if err := model.GetDB().Model(&model.TheaterRoomModel{}).Where("world_id = ?", strings.TrimSpace(worldID)).Pluck("id", &roomIDs).Error; err != nil {
		return false, err
	}
	if len(roomIDs) == 0 {
		return false, nil
	}
	var count int64
	pattern := "%\"assetId\":\"" + strings.ReplaceAll(assetID, "%", "\\%") + "\"%"
	if err := model.GetDB().Model(&model.TheaterObjectModel{}).
		Where("room_id IN ? AND content_json LIKE ?", roomIDs, pattern).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
