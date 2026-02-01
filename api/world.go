package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/samber/lo"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/protocol"
	"sealchat/service"
)

func parseQueryIntDefault(c *fiber.Ctx, key string, def int) int {
	value := strings.TrimSpace(c.Query(key, ""))
	if value == "" {
		return def
	}
	num, err := strconv.Atoi(value)
	if err != nil {
		return def
	}
	if num <= 0 {
		return def
	}
	return num
}

func WorldList(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	db := model.GetDB()
	joinedOnly := c.QueryBool("joined")
	keyword := strings.TrimSpace(c.Query("keyword"))
	visibility := strings.TrimSpace(c.Query("visibility"))
	page := parseQueryIntDefault(c, "page", 1)
	pageSize := parseQueryIntDefault(c, "pageSize", 20)
	if pageSize > 50 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	q := db.Table("worlds").Where("worlds.status = ?", "active")
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("worlds.name LIKE ? OR worlds.description LIKE ?", like, like)
	}
	memberSub := db.Table("world_members").Select("world_id").Where("user_id = ?", user.ID)
	if joinedOnly {
		q = q.Where("worlds.id IN (?)", memberSub)
	} else {
		if visibility != "" {
			q = q.Where("worlds.visibility = ?", visibility)
		} else {
			q = q.Where("worlds.visibility = ? OR worlds.id IN (?)", model.WorldVisibilityPublic, memberSub)
		}
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "获取世界列表失败"})
	}

	lastActiveSub := db.Table("channels").
		Select("world_id, MAX(recent_sent_at) as last_active").
		Group("world_id")

	var worlds []*model.WorldModel
	if err := q.Joins("LEFT JOIN (?) as world_last_activity ON world_last_activity.world_id = worlds.id", lastActiveSub).
		Joins("LEFT JOIN world_favorites wf ON wf.world_id = worlds.id AND wf.user_id = ?", user.ID).
		Order("CASE WHEN wf.world_id IS NULL THEN 0 ELSE 1 END DESC").
		Order("COALESCE(world_last_activity.last_active, 0) DESC").
		Order("worlds.created_at DESC").
		Select("worlds.*").
		Offset(offset).Limit(pageSize).
		Find(&worlds).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "获取世界列表失败"})
	}
	if len(worlds) == 0 {
		return c.JSON(fiber.Map{
			"items":    []any{},
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		})
	}

	worldIDs := lo.Map(worlds, func(w *model.WorldModel, _ int) string { return w.ID })
	membership := map[string]string{}
	var memberRows []*model.WorldMemberModel
	if err := db.Where("world_id IN ? AND user_id = ?", worldIDs, user.ID).Find(&memberRows).Error; err == nil {
		for _, row := range memberRows {
			membership[row.WorldID] = row.Role
		}
	}
	countMap := map[string]int64{}
	var countRows []struct {
		WorldID string
		Count   int64
	}
	if err := db.Table("world_members").Select("world_id, COUNT(*) as count").Where("world_id IN ?", worldIDs).Group("world_id").Scan(&countRows).Error; err == nil {
		for _, row := range countRows {
			countMap[row.WorldID] = row.Count
		}
	}

	favoriteIDs, _ := service.ListWorldFavorites(user.ID)
	favoriteSet := map[string]struct{}{}
	for _, id := range favoriteIDs {
		favoriteSet[id] = struct{}{}
	}

	items := make([]fiber.Map, 0, len(worlds))
	for _, w := range worlds {
		_, isFavorite := favoriteSet[w.ID]
		items = append(items, fiber.Map{
			"world":       w,
			"isMember":    membership[w.ID] != "",
			"memberRole":  membership[w.ID],
			"memberCount": countMap[w.ID],
			"isFavorite":  isFavorite,
		})
	}

	return c.JSON(fiber.Map{
		"items":            items,
		"total":            total,
		"page":             page,
		"pageSize":         pageSize,
		"favoriteWorldIds": favoriteIDs,
	})
}

func WorldCreateHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Visibility  string `json:"visibility"`
		Avatar      string `json:"avatar"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	world, channel, err := service.WorldCreate(user.ID, service.WorldCreateParams{
		Name:        body.Name,
		Description: body.Description,
		Visibility:  body.Visibility,
		Avatar:      body.Avatar,
	})
	if err != nil {
		if errors.Is(err, service.ErrWorldCreateForbidden) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": err.Error()})
		}
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	return c.JSON(fiber.Map{
		"world":          world,
		"defaultChannel": channel,
	})
}

func WorldDetail(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	world, err := service.GetWorldByID(worldID)
	if err != nil {
		if errors.Is(err, service.ErrWorldNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "世界不存在"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "获取世界失败"})
	}
	db := model.GetDB()
	var member model.WorldMemberModel
	if err := db.Where("world_id = ? AND user_id = ?", worldID, user.ID).Limit(1).Find(&member).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "校验成员失败"})
	}
	if world.Visibility == model.WorldVisibilityPrivate && member.ID == "" && !pm.CanWithSystemRole(user.ID, pm.PermModAdmin) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "私有世界需通过邀请访问"})
	}
	var memberCount int64
	_ = db.Model(&model.WorldMemberModel{}).Where("world_id = ?", worldID).Count(&memberCount).Error

	// 获取世界拥有者昵称
	var ownerNickname string
	if world.OwnerID != "" {
		var owner model.UserModel
		if err := db.Where("id = ?", world.OwnerID).Select("id, nickname, username").Limit(1).Find(&owner).Error; err == nil && owner.ID != "" {
			ownerNickname = owner.Nickname
			if ownerNickname == "" {
				ownerNickname = owner.Username
			}
		}
	}

	// 判断编辑通知是否已确认
	editNoticeAcked := member.EditNoticeAckedAt != nil

	return c.JSON(fiber.Map{
		"world":                   world,
		"isMember":                member.ID != "",
		"memberRole":              member.Role,
		"memberCount":             memberCount,
		"allowAdminEditMessages":  world.AllowAdminEditMessages,
		"allowMemberEditKeywords": world.AllowMemberEditKeywords,
		"ownerNickname":           ownerNickname,
		"editNoticeAcked":         editNoticeAcked,
	})
}

func WorldPublicDetail(c *fiber.Ctx) error {
	worldID := c.Params("worldId")
	world, err := service.GetWorldByID(worldID)
	if err != nil {
		if errors.Is(err, service.ErrWorldNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "世界不存在"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "获取世界失败"})
	}
	if world.Visibility != model.WorldVisibilityPublic {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "世界未开放公开访问"})
	}
	var memberCount int64
	_ = model.GetDB().Model(&model.WorldMemberModel{}).Where("world_id = ?", worldID).Count(&memberCount).Error
	return c.JSON(fiber.Map{
		"world":       world,
		"isMember":    false,
		"memberRole":  "",
		"memberCount": memberCount,
	})
}

func WorldUpdateHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	var body service.WorldUpdateParams
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	world, err := service.WorldUpdate(worldID, user.ID, body)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWorldNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "世界不存在"})
		case errors.Is(err, service.ErrWorldPermission):
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "无权编辑世界"})
		case errors.Is(err, service.ErrWorldDescriptionTooLong):
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		default:
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "更新世界失败"})
		}
	}
	if world != nil && world.ID != "" {
		broadcastWorldUpdated(world)
	}
	return c.JSON(fiber.Map{"world": world})
}

func broadcastWorldUpdated(world *model.WorldModel) {
	if world == nil || strings.TrimSpace(world.ID) == "" {
		return
	}
	event := &protocol.Event{
		Type: protocol.EventWorldUpdated,
		Argv: &protocol.Argv{
			Options: map[string]interface{}{
				"world": world,
			},
		},
	}
	broadcastEventToWorld(world.ID, event)
}

func WorldDeleteHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	if err := service.WorldDelete(worldID, user.ID); err != nil {
		switch {
		case errors.Is(err, service.ErrWorldPermission):
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "无权删除世界"})
		case errors.Is(err, service.ErrWorldNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "世界不存在"})
		default:
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "删除世界失败"})
		}
	}
	return c.JSON(fiber.Map{"message": "世界已删除"})
}

func WorldJoinHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	world, err := service.GetWorldByID(worldID)
	if err != nil {
		if errors.Is(err, service.ErrWorldNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "世界不存在"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "获取世界失败"})
	}
	if world.Visibility == model.WorldVisibilityPrivate && !service.IsWorldAdmin(worldID, user.ID) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "该世界仅通过邀请加入"})
	}
	member, err := service.WorldJoin(worldID, user.ID, model.WorldRoleMember)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "加入失败"})
	}
	return c.JSON(fiber.Map{"member": member})
}

func WorldLeaveHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	if err := service.WorldLeave(worldID, user.ID); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "已退出"})
}

func WorldSectionsHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	sectionsParam := strings.TrimSpace(c.Query("sections"))
	if sectionsParam == "" {
		sectionsParam = "channels,members"
	}
	sections := lo.Filter(strings.Split(sectionsParam, ","), func(s string, _ int) bool {
		return strings.TrimSpace(s) != ""
	})
	if len(sections) == 0 {
		sections = []string{"channels"}
	}
	world, err := service.GetWorldByID(worldID)
	if err != nil {
		if errors.Is(err, service.ErrWorldNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "世界不存在"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "获取世界失败"})
	}
	if world.Visibility == model.WorldVisibilityPrivate && !service.IsWorldAdmin(worldID, user.ID) {
		var member model.WorldMemberModel
		if err := model.GetDB().Where("world_id = ? AND user_id = ?", worldID, user.ID).Limit(1).Find(&member).Error; err != nil || member.ID == "" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "私有世界需加入后访问"})
		}
	}
	resp := fiber.Map{"worldId": worldID}
	for _, section := range sections {
		switch strings.TrimSpace(section) {
		case "channels":
			channels, err := service.ChannelList(user.ID, worldID)
			if err == nil {
				resp["channels"] = channels
			}
		case "members":
			members, err := service.ListWorldMembers(worldID, 100)
			if err == nil {
				resp["members"] = members
			}
		case "invites":
			if service.IsWorldAdmin(worldID, user.ID) {
				var invites []*model.WorldInviteModel
				now := time.Now()
				query := model.GetDB().
					Where("world_id = ? AND status = ?", worldID, "active").
					Where("(expire_at IS NULL OR expire_at > ?)", now).
					Where("(max_use = 0 OR used_count < max_use)")
				if err := query.Order("created_at DESC").Limit(20).Find(&invites).Error; err == nil {
					resp["invites"] = invites
				}
			}
		}
	}
	return c.JSON(resp)
}

func WorldInviteCreateHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	var body struct {
		TTLMinutes int    `json:"ttlMinutes"`
		MaxUse     int    `json:"maxUse"`
		Memo       string `json:"memo"`
		Role       string `json:"role"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	invite, err := service.WorldInviteCreate(worldID, user.ID, body.TTLMinutes, body.MaxUse, body.Memo, body.Role)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWorldPermission):
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "无权创建邀请"})
		case errors.Is(err, service.ErrWorldNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "世界不存在"})
		default:
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "创建邀请失败"})
		}
	}
	return c.JSON(fiber.Map{"invite": invite})
}

func WorldInviteConsumeHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	slug := c.Params("slug")
	invite, world, member, alreadyJoined, err := service.WorldInviteConsume(slug, user.ID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWorldInviteInvalid):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "邀请链接无效或已过期"})
		case errors.Is(err, service.ErrWorldNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "世界不存在"})
		default:
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "加入失败"})
		}
	}
	return c.JSON(fiber.Map{
		"invite":         invite,
		"world":          world,
		"member":         member,
		"already_joined": alreadyJoined,
	})
}

func WorldMemberListHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	if !service.IsWorldAdmin(worldID, user.ID) && !pm.CanWithSystemRole(user.ID, pm.PermModAdmin) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "无权查看成员"})
	}
	page := parseQueryIntDefault(c, "page", 1)
	pageSize := parseQueryIntDefault(c, "pageSize", 20)
	keyword := strings.TrimSpace(c.Query("keyword"))
	items, total, err := service.ListWorldMembersDetail(worldID, page, pageSize, keyword)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "获取成员失败"})
	}
	return c.JSON(fiber.Map{
		"items":    items,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

func WorldMemberRemoveHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	targetUserID := c.Params("userId")
	if err := service.WorldRemoveMember(worldID, user.ID, targetUserID); err != nil {
		switch {
		case errors.Is(err, service.ErrWorldPermission):
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "无权操作"})
		case errors.Is(err, service.ErrWorldOwnerImmutable):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "世界拥有者不可移除"})
		case errors.Is(err, service.ErrWorldMemberInvalid):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "成员不存在"})
		default:
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "操作失败"})
		}
	}
	return c.JSON(fiber.Map{"message": "已移除"})
}

func WorldMemberRoleHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	targetUserID := c.Params("userId")
	var body struct {
		Role string `json:"role"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	if err := service.WorldUpdateMemberRole(worldID, user.ID, targetUserID, body.Role); err != nil {
		switch {
		case errors.Is(err, service.ErrWorldPermission):
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "无权操作"})
		case errors.Is(err, service.ErrWorldOwnerImmutable):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "世界拥有者不可变更角色"})
		case errors.Is(err, service.ErrWorldMemberInvalid):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "成员不存在或角色无效"})
		default:
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "更新失败"})
		}
	}
	return c.JSON(fiber.Map{"message": "已更新"})
}

func WorldFavoriteListHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldIDs, err := service.ListWorldFavorites(user.ID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "获取收藏失败"})
	}
	return c.JSON(fiber.Map{"worldIds": worldIDs})
}

func WorldFavoriteToggleHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	var body struct {
		Favorite bool `json:"favorite"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	worldIDs, err := service.ToggleWorldFavorite(worldID, user.ID, body.Favorite)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWorldNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "世界不存在"})
		case errors.Is(err, service.ErrWorldPermission):
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "需要先加入该世界"})
		default:
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "更新收藏失败"})
		}
	}
	return c.JSON(fiber.Map{"worldIds": worldIDs})
}

func WorldAckEditNoticeHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	db := model.GetDB()
	now := time.Now()
	result := db.Model(&model.WorldMemberModel{}).
		Where("world_id = ? AND user_id = ?", worldID, user.ID).
		Update("edit_notice_acked_at", now)
	if result.Error != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "确认失败"})
	}
	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "未加入该世界"})
	}
	return c.JSON(fiber.Map{"message": "已确认", "ackedAt": now.UnixMilli()})
}
