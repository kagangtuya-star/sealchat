package service

import (
	"encoding/json"
	"strings"
	"time"

	"sealchat/model"
)

type TheaterAuditFilter struct {
	ActorID string
	Type    string
	Outcome string
	From    *time.Time
	To      *time.Time
	Offset  int
	Limit   int
}

type TheaterAuditSummary struct {
	PayloadBytes int `json:"payloadBytes"`
}

type TheaterAuditItem struct {
	ID             string              `json:"id"`
	ActorUserID    string              `json:"actorUserId"`
	ActorName      string              `json:"actorName"`
	MutationID     string              `json:"mutationId"`
	RevisionBefore int64               `json:"revisionBefore"`
	RevisionAfter  *int64              `json:"revisionAfter"`
	Type           string              `json:"type"`
	Outcome        string              `json:"outcome"`
	ReasonCode     string              `json:"reasonCode,omitempty"`
	ReasonMessage  string              `json:"reasonMessage,omitempty"`
	Source         string              `json:"source"`
	Summary        TheaterAuditSummary `json:"summary"`
	CreatedAt      time.Time           `json:"createdAt"`
}

type TheaterAuditResult struct {
	Items  []TheaterAuditItem `json:"items"`
	Total  int64              `json:"total"`
	Offset int                `json:"offset"`
	Limit  int                `json:"limit"`
}

func ListTheaterAudit(actorID, worldID, channelID string, filter TheaterAuditFilter) (*TheaterAuditResult, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionAdminRestore); err != nil {
		return nil, err
	}
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	query := model.GetDB().Model(&model.TheaterAuditLogModel{}).Where("world_id = ? AND channel_id = ?", worldID, channelID)
	if value := strings.TrimSpace(filter.ActorID); value != "" {
		query = query.Where("actor_user_id = ?", value)
	}
	if value := strings.TrimSpace(filter.Type); value != "" {
		query = query.Where("mutation_type = ?", value)
	}
	if value := strings.TrimSpace(filter.Outcome); value != "" {
		query = query.Where("outcome = ?", value)
	}
	if filter.From != nil {
		query = query.Where("created_at >= ?", *filter.From)
	}
	if filter.To != nil {
		query = query.Where("created_at <= ?", *filter.To)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []model.TheaterAuditLogModel
	if err := query.Order("created_at DESC").Offset(filter.Offset).Limit(filter.Limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]TheaterAuditItem, 0, len(rows))
	for _, row := range rows {
		var raw struct {
			PayloadBytes int `json:"payloadBytes"`
		}
		_ = json.Unmarshal([]byte(row.SummaryJSON), &raw)
		items = append(items, TheaterAuditItem{
			ID: row.ID, ActorUserID: row.ActorUserID, ActorName: row.ActorNameSnapshot, MutationID: row.MutationID,
			RevisionBefore: row.RevisionBefore, RevisionAfter: row.RevisionAfter, Type: row.MutationType,
			Outcome: row.Outcome, ReasonCode: row.ReasonCode, ReasonMessage: row.ReasonMessage,
			Source: row.RequestSource, Summary: TheaterAuditSummary{PayloadBytes: raw.PayloadBytes}, CreatedAt: row.CreatedAt,
		})
	}
	return &TheaterAuditResult{Items: items, Total: total, Offset: filter.Offset, Limit: filter.Limit}, nil
}

func auditTheaterResourceState(resourceID, outcome, reasonCode string) {
	var resource model.TheaterResourceModel
	if err := model.GetDB().Where("id = ?", resourceID).First(&resource).Error; err != nil {
		return
	}
	var room model.TheaterRoomModel
	if err := model.GetDB().Where("id = ?", resource.RoomID).First(&room).Error; err != nil {
		return
	}
	actorName := ""
	var user model.UserModel
	if err := model.GetDB().Select("id", "nickname").Where("id = ?", resource.CreatedBy).Limit(1).Find(&user).Error; err == nil {
		actorName = user.Nickname
	}
	summary, _ := json.Marshal(map[string]any{"resourceId": resource.ID, "kind": resource.Kind, "mimeType": resource.MimeType, "sizeBytes": resource.SizeBytes})
	_ = model.GetDB().Create(&model.TheaterAuditLogModel{
		RoomID: room.ID, WorldID: room.WorldID, ChannelID: room.ChannelID, ActorUserID: resource.CreatedBy, ActorNameSnapshot: actorName,
		RevisionBefore: room.Revision, MutationType: "resource." + outcome, Outcome: outcome, ReasonCode: reasonCode,
		RequestSource: "system", SummaryJSON: string(summary),
	}).Error
}
