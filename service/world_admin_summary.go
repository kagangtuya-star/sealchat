package service

import (
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
)

// AdminWorldSummary 是平台状态页中展示的世界基本信息，只包含世界层面的概要与计数，不含频道/消息内容。
type AdminWorldSummary struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Visibility      string    `json:"visibility"`
	OwnerID         string    `json:"ownerId"`
	OwnerNickname   string    `json:"ownerNickname"`
	CreatedAt       time.Time `json:"createdAt"`
	MemberCount     int64     `json:"memberCount"`
	ChannelCount    int64     `json:"channelCount"`
	LastActiveAt    int64     `json:"lastActiveAt"` // 最新未删除消息的发送时间（含归档频道），毫秒；0 表示尚无消息
	MessageChars    int64     `json:"messageChars"`
	MessageCharsIC  int64     `json:"messageCharsIc"`
	MessageCharsOOC int64     `json:"messageCharsOoc"`
}

const (
	adminWorldSummaryDefaultPageSize = 20
	adminWorldSummaryMaxPageSize     = 100
)

// AdminWorldSummaryQuery 是管理员世界列表的分页与搜索参数。
type AdminWorldSummaryQuery struct {
	Page     int
	PageSize int
	Keyword  string // 匹配世界名称、简介或创建者昵称/用户名，不区分大小写
}

// AdminWorldSummaryPage 是管理员世界列表的一页结果。
type AdminWorldSummaryPage struct {
	Items    []AdminWorldSummary
	Total    int64
	Page     int
	PageSize int
}

// AdminListWorldSummaries 按创建时间升序分页返回正常状态世界（含私有世界）的概要信息，各项计数只统计当前页的世界。
func AdminListWorldSummaries(query AdminWorldSummaryQuery) (*AdminWorldSummaryPage, error) {
	page := query.Page
	if page < 1 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize < 1 {
		pageSize = adminWorldSummaryDefaultPageSize
	}
	if pageSize > adminWorldSummaryMaxPageSize {
		pageSize = adminWorldSummaryMaxPageSize
	}
	result := &AdminWorldSummaryPage{Items: []AdminWorldSummary{}, Page: page, PageSize: pageSize}

	db := model.GetDB()
	inactive := []string{"deleted", "archived"}

	worldQuery := db.Table("worlds AS w").
		Joins("LEFT JOIN users u ON u.id = w.owner_id").
		Where("w.status NOT IN ?", inactive)
	if keyword := strings.ToLower(strings.TrimSpace(query.Keyword)); keyword != "" {
		like := "%" + keyword + "%"
		worldQuery = worldQuery.Where(
			"LOWER(COALESCE(w.name, '')) LIKE ? OR LOWER(COALESCE(w.description, '')) LIKE ? OR LOWER(COALESCE(u.nickname, '')) LIKE ? OR LOWER(COALESCE(u.username, '')) LIKE ?",
			like, like, like, like,
		)
	}
	worldQuery = worldQuery.Session(&gorm.Session{})
	if err := worldQuery.Count(&result.Total).Error; err != nil {
		return nil, err
	}
	var worlds []model.WorldModel
	if err := worldQuery.
		Select("w.id, w.name, w.description, w.visibility, w.owner_id, w.created_at").
		Order("w.created_at ASC, w.id ASC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&worlds).Error; err != nil {
		return nil, err
	}
	if len(worlds) == 0 {
		return result, nil
	}
	items := make([]AdminWorldSummary, 0, len(worlds))

	worldIDs := make([]string, 0, len(worlds))
	ownerIDs := make([]string, 0, len(worlds))
	for _, w := range worlds {
		worldIDs = append(worldIDs, w.ID)
		if w.OwnerID != "" {
			ownerIDs = append(ownerIDs, w.OwnerID)
		}
	}

	ownerNames := map[string]string{}
	if len(ownerIDs) > 0 {
		var owners []model.UserModel
		if err := db.Model(&model.UserModel{}).Select("id, nickname, username").Where("id IN ?", ownerIDs).Find(&owners).Error; err != nil {
			return nil, err
		}
		for _, u := range owners {
			name := u.Nickname
			if name == "" {
				name = u.Username
			}
			ownerNames[u.ID] = name
		}
	}

	type memberRow struct {
		WorldID string
		Total   int64
	}
	var memberRows []memberRow
	if err := db.Model(&model.WorldMemberModel{}).
		Select("world_id, COUNT(*) AS total").
		Where("world_id IN ?", worldIDs).
		Group("world_id").
		Scan(&memberRows).Error; err != nil {
		return nil, err
	}
	memberCounts := make(map[string]int64, len(memberRows))
	for _, row := range memberRows {
		memberCounts[row.WorldID] = row.Total
	}

	type channelRow struct {
		WorldID string
		Total   int64
	}
	var channelRows []channelRow
	if err := db.Model(&model.ChannelModel{}).
		Select("world_id, COUNT(*) AS total").
		Where("world_id IN ? AND is_private = ? AND status NOT IN ?", worldIDs, false, []string{model.ChannelStatusDeleted, model.ChannelStatusArchived}).
		Group("world_id").
		Scan(&channelRows).Error; err != nil {
		return nil, err
	}
	channelStats := make(map[string]channelRow, len(channelRows))
	for _, row := range channelRows {
		channelStats[row.WorldID] = row
	}

	charRows, err := model.CountWorldMessageChars(worldIDs)
	if err != nil {
		return nil, err
	}
	charStats := make(map[string]model.WorldMessageCharStats, len(charRows))
	for _, row := range charRows {
		charStats[row.WorldID] = row
	}

	for _, w := range worlds {
		chars := charStats[w.ID]
		channels := channelStats[w.ID]
		items = append(items, AdminWorldSummary{
			ID:              w.ID,
			Name:            w.Name,
			Description:     w.Description,
			Visibility:      w.Visibility,
			OwnerID:         w.OwnerID,
			OwnerNickname:   ownerNames[w.OwnerID],
			CreatedAt:       w.CreatedAt,
			MemberCount:     memberCounts[w.ID],
			ChannelCount:    channels.Total,
			LastActiveAt:    chars.LastActiveAt,
			MessageChars:    chars.Chars,
			MessageCharsIC:  chars.CharsIC,
			MessageCharsOOC: chars.CharsOOC,
		})
	}
	result.Items = items
	return result, nil
}
