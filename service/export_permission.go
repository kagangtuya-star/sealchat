package service

import (
	"errors"
	"fmt"
	"strings"

	"sealchat/model"
)

var (
	ErrExportPermission     = errors.New("无权限导出该频道")
	ErrMemberExportDisabled = errors.New("当前世界未允许成员导出聊天记录")
)

func exportChannelWorld(userID, channelID string) (*model.WorldModel, string, error) {
	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return nil, "", err
	}
	if channel.ID == "" {
		return nil, "", fmt.Errorf("频道不存在")
	}
	var world model.WorldModel
	if err := model.GetDB().Where("id = ? AND status = ?", channel.WorldID, "active").Take(&world).Error; err != nil {
		return nil, "", err
	}
	if world.OwnerID == userID {
		return &world, model.WorldRoleOwner, nil
	}
	var member model.WorldMemberModel
	if err := model.GetDB().Where("world_id = ? AND user_id = ?", world.ID, userID).Limit(1).Find(&member).Error; err != nil {
		return nil, "", err
	}
	return &world, strings.TrimSpace(member.Role), nil
}

// ResolveExportChannelVisibility validates server-side membership and returns
// whether the job must apply the requesting user's whisper visibility filter.
func ResolveExportChannelVisibility(userID, channelID string) (bool, error) {
	userID, channelID = strings.TrimSpace(userID), strings.TrimSpace(channelID)
	if userID == "" {
		return false, ErrExportPermission
	}
	if channelID == "" {
		return false, fmt.Errorf("channel_id 不能为空")
	}
	if len(channelID) >= 30 {
		friend, err := model.FriendRelationGetByID(channelID)
		if err != nil {
			return false, err
		}
		if friend.ID == "" {
			return false, fmt.Errorf("频道不存在")
		}
		if friend.UserID1 != userID && friend.UserID2 != userID {
			return false, ErrExportPermission
		}
		return false, nil
	}
	world, role, err := exportChannelWorld(userID, channelID)
	if err != nil {
		return false, err
	}
	switch role {
	case model.WorldRoleOwner, model.WorldRoleAdmin:
		return world.StrictWhisperPrivacy, nil
	case model.WorldRoleMember:
		if !world.AllowMemberExportMessages {
			return false, ErrMemberExportDisabled
		}
		if !CanReadChannelByUserId(userID, channelID) {
			return false, ErrExportPermission
		}
		return world.ExportRespectMessageVisibility, nil
	default:
		return false, ErrExportPermission
	}
}

// CanManageMessageExportJob requires management rights over every source of a
// batch. Reading another user's file also respects strict whisper privacy.
func CanManageMessageExportJob(userID string, job *model.MessageExportJobModel, readFile bool) bool {
	if strings.TrimSpace(userID) == "" || job == nil {
		return false
	}
	if job.UserID == userID {
		return true
	}
	if IsLogMessageExportJob(job) {
		return false
	}
	for _, channelID := range ExportJobChannelIDs(job) {
		if channelID == "" || len(channelID) >= 30 {
			if readFile && len(channelID) >= 30 {
				if _, err := ResolveExportChannelVisibility(userID, channelID); err == nil {
					continue
				}
			}
			return false
		}
		world, role, err := exportChannelWorld(userID, channelID)
		if err != nil || (role != model.WorldRoleOwner && role != model.WorldRoleAdmin) || (readFile && world.StrictWhisperPrivacy) {
			return false
		}
	}
	return true
}

func ExportJobChannelIDs(job *model.MessageExportJobModel) []string {
	ids := []string{job.ChannelID}
	return append(ids, parseExportExtraOptions(job.ExtraOptions).BatchChannelIDs...)
}

func snapshotExportVisibility(opts *ExportJobOptions, channelIDs []string) error {
	if opts.SourceKind == "log" {
		return nil
	}
	snapshot := make(map[string]bool, len(channelIDs))
	for _, channelID := range channelIDs {
		filter, err := ResolveExportChannelVisibility(opts.UserID, channelID)
		if err != nil {
			return err
		}
		if opts.visibilityByChannel != nil {
			// Retry preserves the original scope. Legacy jobs without a snapshot
			// conservatively use user visibility rather than gaining full access.
			var exists bool
			filter, exists = opts.visibilityByChannel[channelID]
			if !exists {
				filter = len(channelID) < 30
			}
		}
		snapshot[channelID] = filter
	}
	opts.visibilityByChannel = snapshot
	return nil
}
