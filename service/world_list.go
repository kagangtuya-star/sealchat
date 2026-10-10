package service

import (
	"gorm.io/gorm"
	"sealchat/model"
	"strings"
)

type UserWorldListOptions struct {
	JoinedOnly, ArchivedOnly, IncludeArchived bool
	Keyword, Visibility                       string
}

// UserWorldListQuery is metadata visibility only, not a grant of content access.
func UserWorldListQuery(userID string, in UserWorldListOptions) *gorm.DB {
	db := model.GetDB()
	q := db.Table("worlds").Where("worlds.status = ?", "active")
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		q = q.Where("worlds.name LIKE ? OR worlds.description LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	member := db.Table("world_members").Select("world_id").Where("user_id = ?", userID)
	if in.JoinedOnly {
		q = q.Where("worlds.id IN (?)", member)
		archive := db.Table("world_archives").Select("world_id").Where("user_id = ?", userID)
		if in.ArchivedOnly {
			q = q.Where("worlds.id IN (?)", archive)
		} else if !in.IncludeArchived {
			q = q.Where("worlds.id NOT IN (?)", archive)
		}
	} else if in.Visibility != "" {
		q = q.Where("worlds.visibility = ?", in.Visibility)
	} else {
		q = q.Where("worlds.visibility = ? OR worlds.id IN (?)", model.WorldVisibilityPublic, member)
	}
	return q
}
func OrderUserWorldListQuery(q *gorm.DB, userID string) *gorm.DB {
	last := model.GetDB().Table("channels").Select("world_id, MAX(recent_sent_at) as last_active").Group("world_id")
	return q.Joins("LEFT JOIN (?) as world_last_activity ON world_last_activity.world_id = worlds.id", last).Joins("LEFT JOIN world_favorites wf ON wf.world_id = worlds.id AND wf.user_id = ?", userID).Order("CASE WHEN wf.world_id IS NULL THEN 0 ELSE 1 END DESC").Order("COALESCE(world_last_activity.last_active, 0) DESC").Order("worlds.created_at DESC")
}
