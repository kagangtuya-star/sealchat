package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
	"sealchat/pm"
)

var (
	ErrChannelNotFound                   = errors.New("channel not found")
	ErrChannelPermissionDenied           = errors.New("channel permission denied")
	ErrChannelWorldRequired              = errors.New("channel world required")
	ErrChannelTargetRoleNotFound         = errors.New("channel target role not found")
	ErrChannelMemberRoleMissing          = errors.New("channel member role missing")
	ErrChannelMemberCandidateRoleInvalid = errors.New("channel member candidate role invalid")
)

type ChannelMemberCandidateQuery struct {
	ChannelID        string
	ActorID          string
	Page             int
	PageSize         int
	Keyword          string
	RoleKey          string
	IncludeSpectator bool
	ExcludeExisting  bool
}

type ChannelMemberCandidate struct {
	UserID           string    `json:"userId"`
	Username         string    `json:"username"`
	Nickname         string    `json:"nickname"`
	Avatar           string    `json:"avatar"`
	WorldRole        string    `json:"worldRole"`
	JoinedAt         time.Time `json:"joinedAt"`
	AlreadyInChannel bool      `json:"alreadyInChannel"`
}

type ChannelMemberCandidateResult struct {
	Items    []*ChannelMemberCandidate `json:"items"`
	Total    int64                     `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"pageSize"`
}

type ChannelAddWorldMembersParams struct {
	ChannelID        string
	ActorID          string
	IncludeSpectator bool
}

type ChannelAddWorldMembersResult struct {
	RoleID               string `json:"roleId"`
	CandidateCount       int64  `json:"candidateCount"`
	AddedCount           int64  `json:"addedCount"`
	SkippedExistingCount int64  `json:"skippedExistingCount"`
}

type channelMemberCandidateRow struct {
	UserID           string
	Username         string
	Nickname         string
	Avatar           string
	WorldRole        string
	JoinedAt         time.Time
	AlreadyInChannel int
}

func normalizeChannelMemberCandidateRoleKey(roleKey string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(roleKey))
	if normalized == "" {
		return "member", nil
	}
	switch normalized {
	case "owner", "admin", "member", "spectator", "ob", "bot":
		return normalized, nil
	default:
		return "", ErrChannelMemberCandidateRoleInvalid
	}
}

func normalizeChannelMemberCandidatePage(page int) int {
	if page <= 0 {
		return 1
	}
	return page
}

func normalizeChannelMemberCandidatePageSize(pageSize int) int {
	if pageSize <= 0 {
		return 20
	}
	if pageSize > 100 {
		return 100
	}
	return pageSize
}

func buildChannelRoleID(channelID, roleKey string) string {
	return fmt.Sprintf("ch-%s-%s", strings.TrimSpace(channelID), strings.TrimSpace(roleKey))
}

func resolveManagedChannelForActor(channelID, actorID string) (*model.ChannelModel, error) {
	channelID = strings.TrimSpace(channelID)
	actorID = strings.TrimSpace(actorID)
	if channelID == "" {
		return nil, ErrChannelNotFound
	}
	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return nil, err
	}
	if channel == nil || strings.TrimSpace(channel.ID) == "" {
		return nil, ErrChannelNotFound
	}
	if strings.TrimSpace(channel.WorldID) == "" {
		return nil, ErrChannelWorldRequired
	}
	if pm.CanWithSystemRole(actorID, pm.PermModAdmin) {
		return channel, nil
	}
	if !pm.CanWithChannelRole(actorID, channelID, pm.PermFuncChannelRoleLink, pm.PermFuncChannelRoleLinkRoot) {
		return nil, ErrChannelPermissionDenied
	}
	return channel, nil
}

func ensureChannelTargetRole(channelID, roleKey string) (string, error) {
	targetRoleID := buildChannelRoleID(channelID, roleKey)
	role, err := model.ChannelRoleGet(targetRoleID)
	if err != nil {
		return "", err
	}
	if role == nil || strings.TrimSpace(role.ID) == "" {
		if roleKey == "member" {
			return "", ErrChannelMemberRoleMissing
		}
		return "", ErrChannelTargetRoleNotFound
	}
	return targetRoleID, nil
}

func buildChannelMemberCandidateBaseQuery(db *gorm.DB, channel *model.ChannelModel, targetRoleID, keyword string, includeSpectator bool) *gorm.DB {
	allowedRoles := []string{model.WorldRoleOwner, model.WorldRoleAdmin, model.WorldRoleMember}
	if includeSpectator {
		allowedRoles = append(allowedRoles, model.WorldRoleSpectator)
	}

	query := db.Table("world_members AS wm").
		Select("wm.user_id, wm.role AS world_role, wm.joined_at, u.username, u.nickname, u.avatar, CASE WHEN existing.id IS NULL THEN 0 ELSE 1 END AS already_in_channel").
		Joins("LEFT JOIN users u ON u.id = wm.user_id").
		Joins("LEFT JOIN perm_user_role_mappings existing ON existing.user_id = wm.user_id AND existing.role_id = ?", targetRoleID).
		Where("wm.world_id = ? AND wm.role IN ?", channel.WorldID, allowedRoles)

	normalizedKeyword := strings.ToLower(strings.TrimSpace(keyword))
	if normalizedKeyword == "" {
		return query
	}

	like := "%" + normalizedKeyword + "%"
	query = query.Where(
		"wm.user_id LIKE ? OR LOWER(COALESCE(u.username, '')) LIKE ? OR LOWER(COALESCE(u.nickname, '')) LIKE ?",
		like, like, like,
	)
	return query
}

func applyChannelMemberCandidateOrdering(query *gorm.DB, keyword string) *gorm.DB {
	normalizedKeyword := strings.ToLower(strings.TrimSpace(keyword))
	if normalizedKeyword != "" {
		prefix := normalizedKeyword + "%"
		query = query.Order(clause.Expr{
			SQL: `CASE
				WHEN wm.user_id = ? THEN 0
				WHEN LOWER(COALESCE(u.username, '')) = ? THEN 1
				WHEN LOWER(COALESCE(u.nickname, '')) = ? THEN 2
				WHEN wm.user_id LIKE ? THEN 3
				WHEN LOWER(COALESCE(u.username, '')) LIKE ? THEN 4
				WHEN LOWER(COALESCE(u.nickname, '')) LIKE ? THEN 5
				ELSE 6
			END`,
			Vars: []any{
				normalizedKeyword,
				normalizedKeyword,
				normalizedKeyword,
				prefix,
				prefix,
				prefix,
			},
		})
	}

	return query.
		Order("wm.joined_at DESC").
		Order("LOWER(COALESCE(NULLIF(u.nickname, ''), u.username, wm.user_id)) ASC")
}

func ListChannelMemberCandidates(query ChannelMemberCandidateQuery) (*ChannelMemberCandidateResult, error) {
	roleKey, err := normalizeChannelMemberCandidateRoleKey(query.RoleKey)
	if err != nil {
		return nil, err
	}
	page := normalizeChannelMemberCandidatePage(query.Page)
	pageSize := normalizeChannelMemberCandidatePageSize(query.PageSize)

	channel, err := resolveManagedChannelForActor(query.ChannelID, query.ActorID)
	if err != nil {
		return nil, err
	}
	targetRoleID, err := ensureChannelTargetRole(channel.ID, roleKey)
	if err != nil {
		return nil, err
	}

	db := model.GetDB()
	base := buildChannelMemberCandidateBaseQuery(db, channel, targetRoleID, query.Keyword, query.IncludeSpectator)
	if query.ExcludeExisting {
		base = base.Where("existing.id IS NULL")
	}

	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, err
	}

	var rows []channelMemberCandidateRow
	offset := (page - 1) * pageSize
	if err := applyChannelMemberCandidateOrdering(base, query.Keyword).
		Offset(offset).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]*ChannelMemberCandidate, 0, len(rows))
	for _, row := range rows {
		items = append(items, &ChannelMemberCandidate{
			UserID:           row.UserID,
			Username:         row.Username,
			Nickname:         row.Nickname,
			Avatar:           row.Avatar,
			WorldRole:        row.WorldRole,
			JoinedAt:         row.JoinedAt,
			AlreadyInChannel: row.AlreadyInChannel > 0,
		})
	}

	return &ChannelMemberCandidateResult{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func AddWorldMembersToChannel(params ChannelAddWorldMembersParams) (*ChannelAddWorldMembersResult, error) {
	channel, err := resolveManagedChannelForActor(params.ChannelID, params.ActorID)
	if err != nil {
		return nil, err
	}
	memberRoleID, err := ensureChannelTargetRole(channel.ID, "member")
	if err != nil {
		return nil, err
	}

	db := model.GetDB()
	base := buildChannelMemberCandidateBaseQuery(db, channel, memberRoleID, "", params.IncludeSpectator)

	var candidateCount int64
	if err := base.Session(&gorm.Session{}).Count(&candidateCount).Error; err != nil {
		return nil, err
	}

	var userIDs []string
	var rows []struct {
		UserID string
	}
	if err := base.
		Where("existing.id IS NULL").
		Select("wm.user_id").
		Order("wm.joined_at DESC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	userIDs = make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.UserID) != "" {
			userIDs = append(userIDs, row.UserID)
		}
	}

	result := &ChannelAddWorldMembersResult{
		RoleID:               memberRoleID,
		CandidateCount:       candidateCount,
		SkippedExistingCount: candidateCount - int64(len(userIDs)),
		AddedCount:           0,
	}
	if len(userIDs) == 0 {
		return result, nil
	}

	addedCount, err := UserRoleLink([]string{memberRoleID}, userIDs)
	if err != nil {
		return nil, err
	}
	result.AddedCount = addedCount
	return result, nil
}
