package service

import (
	"context"
	"os"
	"strings"

	"sealchat/model"
	"sealchat/pm"
)

// MCP imports only world assets. Common-library administration stays native.
func ImportMCPAudioAsset(ctx context.Context, actorID, worldID, uploadID string, opts AudioUploadOptions) (*model.AudioAsset, error) {
	worldID = strings.TrimSpace(worldID)
	var world model.WorldModel
	if err := model.GetDB().Where("id = ? AND deleted_at IS NULL", worldID).First(&world).Error; err != nil {
		return nil, err
	}
	if world.Status != "active" || (!IsWorldAdmin(worldID, actorID) && !pm.CanWithSystemRole(actorID, pm.PermModAdmin)) {
		return nil, ErrWorldPermission
	}
	if opts.Visibility == "" {
		opts.Visibility = model.AudioVisibilityPublic
	}
	if opts.Visibility != model.AudioVisibilityPublic && opts.Visibility != model.AudioVisibilityRestricted {
		return nil, ErrWorldClueInvalid
	}
	for _, tag := range opts.Tags {
		if strings.TrimSpace(tag) == TheaterFeatureAudioTag || strings.HasPrefix(strings.TrimSpace(tag), "theater-channel:") {
			return nil, ErrWorldClueInvalid
		}
	}
	opts.CreatedBy, opts.Scope, opts.WorldID, opts.UseTheaterStorage = actorID, model.AudioScopeWorld, &worldID, false
	return importMCPAudioUpload(ctx, actorID, uploadID, opts, "")
}

func importMCPAudioUpload(ctx context.Context, actorID, uploadID string, opts AudioUploadOptions, theaterWorldName string) (*model.AudioAsset, error) {
	svc := GetAudioService()
	if svc == nil {
		return nil, ErrAudioUnsupportedMime
	}
	var asset *model.AudioAsset
	err := consumeMCPUpload(ctx, actorID, uploadID, svc.maxUploadBytes(), func(att *model.AttachmentModel, path string) error {
		if opts.UseTheaterStorage {
			opts.Name = theaterAudioAssetName(theaterWorldName, opts.Name, att.Filename)
		} else if strings.TrimSpace(opts.Name) == "" {
			opts.Name = att.Filename
		}
		var err error
		asset, err = AudioCreateAssetFromImport(path, opts)
		return err
	})
	return asset, err
}

type MCPTheaterAssetInput struct {
	TheaterScope
	Operation        string `json:"operation"`
	UploadID         string `json:"uploadId"`
	Name             string `json:"name,omitempty"`
	ClientResourceID string `json:"clientResourceId,omitempty"`
	MediaKind        string `json:"mediaKind,omitempty"`
	TargetObjectID   string `json:"targetObjectId,omitempty"`
	Purpose          string `json:"purpose,omitempty"`
	IdentityID       string `json:"identityId,omitempty"`
	VariantID        string `json:"variantId,omitempty"`
	TargetUserID     string `json:"targetUserId,omitempty"`
}

// The room scope and the concrete audio/appearance channel are distinct for
// World Theater, exactly as in theater_catalog audio_resources.
func ImportMCPTheaterAsset(ctx context.Context, actorID string, in MCPTheaterAssetInput) (any, error) {
	scope, err := NormalizeTheaterMCPScope(actorID, in.TheaterScope)
	if err != nil {
		return nil, err
	}
	channelID := scope.ChannelID
	if scope.ScopeType == "world" {
		channelID = scope.InputChannelID
	}
	switch in.Operation {
	case "audio_import":
		world, _, err := requireTheaterPermission(actorID, scope.WorldID, scope.ChannelID, TheaterPermissionResourceUpload)
		if err != nil {
			return nil, err
		}
		if channelID == "" {
			return nil, theaterPayloadError("audio_import 必须提供具体有效的频道上下文，World Theater 使用 inputChannelId")
		}
		if _, _, err := requireTheaterPermission(actorID, scope.WorldID, channelID, TheaterPermissionResourceUpload); err != nil {
			return nil, err
		}
		return importMCPAudioUpload(ctx, actorID, in.UploadID, AudioUploadOptions{
			Name: in.Name, Description: "小剧场特性音频",
			Tags: []string{TheaterFeatureAudioTag, theaterChannelAudioTag(channelID)}, Visibility: model.AudioVisibilityRestricted,
			CreatedBy: actorID, Scope: model.AudioScopeWorld, WorldID: &scope.WorldID, UseTheaterStorage: true,
		}, world.Name)
	case "media_import", "appearance_import":
		if in.Operation == "appearance_import" && channelID == "" {
			return nil, theaterPayloadError("appearance_import 必须提供具体有效的频道上下文，World Theater 使用 inputChannelId")
		}
		// Do not reserve or read a file before checking target business access.
		if in.Operation == "media_import" {
			if _, _, err := requireTheaterPermission(actorID, scope.WorldID, scope.ChannelID, TheaterPermissionResourceUpload); err != nil {
				if err := requireDelegatedTheaterImageUpload(actorID, scope.WorldID, scope.ChannelID, in.TargetObjectID); err != nil {
					return nil, err
				}
			}
		} else {
			if _, err := ResolveTheaterAppearanceActor(channelID, actorID, in.TargetUserID, in.IdentityID, in.VariantID, in.Purpose); err != nil {
				return nil, err
			}
		}
		c := normalizeTheaterMediaConfig(theaterMedia.config)
		var result any
		err := consumeMCPUpload(ctx, actorID, in.UploadID, max(c.ImageMaxSizeMB, c.VideoMaxSizeMB)<<20, func(att *model.AttachmentModel, path string) error {
			file, err := os.Open(path)
			if err != nil {
				return theaterPayloadError("无法读取上传")
			}
			defer file.Close()
			if in.Operation == "media_import" {
				result, err = CreateTheaterResourceUpload(ctx, actorID, scope.WorldID, scope.ChannelID, TheaterResourceUploadInput{Reader: file, Size: att.Size, Filename: att.Filename, ClientResourceID: in.ClientResourceID, MediaKind: in.MediaKind, TargetObjectID: in.TargetObjectID})
				if upload, ok := result.(*TheaterResourceUploadResult); err != nil && ok && upload != nil && upload.Resource.ID != "" {
					// The native service deliberately reports projection failures even
					// after the resource/job committed. MCP must consume the source once
					// a durable resource ID exists instead of replaying the import.
					err = nil
				}
			} else {
				result, err = CreateTheaterAppearanceAssetUpload(ctx, actorID, channelID, TheaterAppearanceAssetUploadInput{Reader: file, Size: att.Size, Filename: att.Filename, Purpose: in.Purpose, IdentityID: in.IdentityID, VariantID: in.VariantID, TargetUserID: in.TargetUserID})
				if asset, ok := result.(*TheaterAppearanceAssetPublic); ok && asset != nil {
					asset.FailureMessage = ""
				}
			}
			return err
		})
		return result, err
	default:
		return nil, theaterPayloadError("operation 必须为 media_import/audio_import/appearance_import")
	}
}

func ListMCPTheaterAppearanceAssets(actorID string, scope TheaterScope) ([]TheaterAppearanceAssetPublic, error) {
	s, err := NormalizeTheaterMCPScope(actorID, scope)
	if err != nil {
		return nil, err
	}
	channelID := s.ChannelID
	if s.ScopeType == "world" {
		channelID = s.InputChannelID
	}
	if channelID == "" {
		return nil, theaterPayloadError("appearance_resources 必须提供具体频道上下文")
	}
	var rows []model.TheaterAppearanceAssetModel
	if err := model.GetDB().Where("channel_id = ? AND deleted_at IS NULL", channelID).Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	items := []TheaterAppearanceAssetPublic{}
	for _, row := range rows {
		if _, err := resolveTheaterAppearanceActor(channelID, actorID, row.OwnerUserID, row.IdentityID, row.VariantID, row.Purpose, false); err == nil {
			public := theaterAppearanceAssetPublic(row)
			public.FailureMessage = ""
			items = append(items, public)
		}
	}
	return items, nil
}
