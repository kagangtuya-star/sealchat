package service

import (
	"sort"
	"strings"

	"sealchat/model"
)

type TheaterMCPAudioResource struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Duration        float64 `json:"duration"`
	TranscodeStatus string  `json:"transcodeStatus"`
	Ready           bool    `json:"ready"`
}

func ListTheaterMCPAudioResources(actorID, worldID, theaterChannelID, audioChannelID string) ([]TheaterMCPAudioResource, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, theaterChannelID, TheaterPermissionView); err != nil {
		return nil, err
	}
	if !CanManageTheaterResources(actorID, worldID, theaterChannelID) {
		return nil, newTheaterError(TheaterErrorPermissionDenied, "没有 Theater 素材管理权限", 403, nil)
	}
	worldID = strings.TrimSpace(worldID)
	assets, _, err := AudioListAssets(AudioAssetFilters{
		Tags: []string{TheaterFeatureAudioTag, theaterChannelAudioTag(audioChannelID)},
		Page: 1, PageSize: 500, SortBy: "updatedAt", SortOrder: "desc",
		Scope: model.AudioScopeWorld, WorldID: &worldID, IncludeCommon: false,
	})
	if err != nil {
		return nil, err
	}
	items := make([]TheaterMCPAudioResource, 0, len(assets))
	for _, asset := range assets {
		if asset == nil {
			continue
		}
		items = append(items, TheaterMCPAudioResource{ID: asset.ID, Name: asset.Name, Duration: asset.DurationSeconds, TranscodeStatus: string(asset.TranscodeStatus), Ready: string(asset.TranscodeStatus) == "ready"})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}
