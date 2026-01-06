package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/utils"
)

// ========== REST API ==========

// BindStickyNoteRoutes 绑定便签相关的 REST 路由
// 接收已认证的路由组 (v1Auth)
func BindStickyNoteRoutes(group fiber.Router) {
	// 通过频道获取便签列表
	group.Get("/channels/:channelId/sticky-notes", apiChannelStickyNoteList)
	// 创建便签
	group.Post("/channels/:channelId/sticky-notes", apiChannelStickyNoteCreate)
	// 获取单个便签
	group.Get("/sticky-notes/:noteId", apiStickyNoteGet)
	// 更新便签
	group.Patch("/sticky-notes/:noteId", apiStickyNoteUpdateRest)
	// 删除便签
	group.Delete("/sticky-notes/:noteId", apiStickyNoteDeleteRest)
	// 更新用户状态
	group.Patch("/sticky-notes/:noteId/state", apiStickyNoteUserStateUpdate)
	// 推送便签
	group.Post("/sticky-notes/:noteId/push", apiStickyNotePushRest)

	// 文件夹相关
	group.Get("/channels/:channelId/sticky-note-folders", apiChannelStickyNoteFolderList)
	group.Post("/channels/:channelId/sticky-note-folders", apiChannelStickyNoteFolderCreate)
	group.Patch("/sticky-note-folders/:folderId", apiStickyNoteFolderUpdate)
	group.Delete("/sticky-note-folders/:folderId", apiStickyNoteFolderDelete)
}

// getStickyNoteUser 获取当前用户
// 中间件已验证用户，直接获取即可
func getStickyNoteUser(c *fiber.Ctx) *model.UserModel {
	return getCurUser(c)
}

func canManageStickyNote(userID, channelID, creatorID string) (bool, error) {
	if userID == "" || channelID == "" {
		return false, nil
	}
	if creatorID != "" && creatorID == userID {
		return true, nil
	}
	roleIDs, err := model.UserRoleMappingListByUserID(userID, channelID, "channel")
	if err != nil {
		return false, err
	}
	for _, roleID := range roleIDs {
		if strings.HasSuffix(roleID, "-owner") || strings.HasSuffix(roleID, "-admin") || strings.HasSuffix(roleID, "-member") {
			return true, nil
		}
	}
	return false, nil
}

// apiChannelStickyNoteList 获取频道的所有便签
func apiChannelStickyNoteList(c *fiber.Ctx) error {
	channelID := c.Params("channelId")
	user := getStickyNoteUser(c)
	if user == nil {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	notes, err := model.StickyNoteListByChannel(channelID, false)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 加载创建者信息
	for _, note := range notes {
		note.LoadCreator()
	}

	// 获取用户状态
	states, _ := model.StickyNoteUserStateListByUser(user.ID, channelID)
	stateMap := make(map[string]*model.StickyNoteUserStateModel)
	for _, s := range states {
		stateMap[s.StickyNoteID] = s
	}

	// 构建响应
	result := make([]fiber.Map, len(notes))
	for i, note := range notes {
		item := fiber.Map{
			"note": note.ToProtocolType(),
		}
		if state, ok := stateMap[note.ID]; ok {
			item["userState"] = state.ToProtocolType()
		}
		result[i] = item
	}

	return c.JSON(fiber.Map{"items": result})
}

// apiChannelStickyNoteCreate 创建便签
func apiChannelStickyNoteCreate(c *fiber.Ctx) error {
	channelID := c.Params("channelId")
	user := getStickyNoteUser(c)
	if user == nil {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		Title      string `json:"title"`
		Content    string `json:"content"`
		Color      string `json:"color"`
		NoteType   string `json:"noteType"`
		TypeData   string `json:"typeData"`
		Visibility string `json:"visibility"`
		ViewerIDs  string `json:"viewerIds"`
		EditorIDs  string `json:"editorIds"`
		FolderID   string `json:"folderId"`
		DefaultX   int    `json:"defaultX"`
		DefaultY   int    `json:"defaultY"`
		DefaultW   int    `json:"defaultW"`
		DefaultH   int    `json:"defaultH"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	// 获取频道信息
	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "channel not found"})
	}

	// 设置默认值
	if req.Color == "" {
		req.Color = "yellow"
	}
	if req.DefaultW == 0 {
		req.DefaultW = 300
	}
	if req.DefaultH == 0 {
		req.DefaultH = 250
	}
	if req.NoteType == "" {
		req.NoteType = "text"
	}
	if req.Visibility == "" {
		req.Visibility = "all"
	}

	note := &model.StickyNoteModel{
		ChannelID:   channelID,
		WorldID:     channel.WorldID,
		FolderID:    req.FolderID,
		Title:       req.Title,
		Content:     req.Content,
		ContentText: req.Content,
		Color:       req.Color,
		CreatorID:   user.ID,
		IsPublic:    true,
		NoteType:    model.StickyNoteType(req.NoteType),
		TypeData:    req.TypeData,
		Visibility:  model.StickyNoteVisibility(req.Visibility),
		ViewerIDs:   req.ViewerIDs,
		EditorIDs:   req.EditorIDs,
		DefaultX:    req.DefaultX,
		DefaultY:    req.DefaultY,
		DefaultW:    req.DefaultW,
		DefaultH:    req.DefaultH,
	}
	note.ID = utils.NewID()

	if err := model.StickyNoteCreate(note); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	note.Creator = user

	// WebSocket 广播
	go BroadcastStickyNoteToChannel(channelID, protocol.EventStickyNoteCreated, &protocol.StickyNoteEventPayload{
		Note:   note.ToProtocolType(),
		Action: "create",
	})

	return c.JSON(fiber.Map{"note": note.ToProtocolType()})
}

// apiStickyNoteGet 获取单个便签
func apiStickyNoteGet(c *fiber.Ctx) error {
	noteID := c.Params("noteId")

	note, err := model.StickyNoteGet(noteID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "note not found"})
	}

	note.LoadCreator()
	return c.JSON(fiber.Map{"note": note.ToProtocolType()})
}

// apiStickyNoteUpdateRest 更新便签 (REST)
func apiStickyNoteUpdateRest(c *fiber.Ctx) error {
	noteID := c.Params("noteId")

	note, err := model.StickyNoteGet(noteID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "note not found"})
	}

	var req struct {
		Title       *string `json:"title"`
		Content     *string `json:"content"`
		ContentText *string `json:"contentText"`
		Color       *string `json:"color"`
		IsPinned    *bool   `json:"isPinned"`
		NoteType    *string `json:"noteType"`
		TypeData    *string `json:"typeData"`
		Visibility  *string `json:"visibility"`
		ViewerIDs   *string `json:"viewerIds"`
		EditorIDs   *string `json:"editorIds"`
		FolderID    *string `json:"folderId"`
		DefaultX    *int    `json:"defaultX"`
		DefaultY    *int    `json:"defaultY"`
		DefaultW    *int    `json:"defaultW"`
		DefaultH    *int    `json:"defaultH"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}

	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Content != nil {
		updates["content"] = *req.Content
	}
	if req.ContentText != nil {
		updates["content_text"] = *req.ContentText
	}
	if req.Color != nil {
		updates["color"] = *req.Color
	}
	if req.IsPinned != nil {
		updates["is_pinned"] = *req.IsPinned
	}
	if req.NoteType != nil {
		updates["note_type"] = *req.NoteType
	}
	if req.TypeData != nil {
		updates["type_data"] = *req.TypeData
	}
	if req.Visibility != nil {
		updates["visibility"] = *req.Visibility
	}
	if req.ViewerIDs != nil {
		updates["viewer_ids"] = *req.ViewerIDs
	}
	if req.EditorIDs != nil {
		updates["editor_ids"] = *req.EditorIDs
	}
	if req.FolderID != nil {
		updates["folder_id"] = *req.FolderID
	}
	if req.DefaultX != nil {
		updates["default_x"] = *req.DefaultX
	}
	if req.DefaultY != nil {
		updates["default_y"] = *req.DefaultY
	}
	if req.DefaultW != nil {
		updates["default_w"] = *req.DefaultW
	}
	if req.DefaultH != nil {
		updates["default_h"] = *req.DefaultH
	}

	if err := model.StickyNoteUpdate(noteID, updates); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 重新加载
	note, _ = model.StickyNoteGet(noteID)
	note.LoadCreator()

	// 广播更新事件
	go BroadcastStickyNoteToChannel(note.ChannelID, protocol.EventStickyNoteUpdated, &protocol.StickyNoteEventPayload{
		Note:   note.ToProtocolType(),
		Action: "update",
	})

	return c.JSON(fiber.Map{"note": note.ToProtocolType()})
}

// apiStickyNoteDeleteRest 删除便签 (REST)
// 权限：仅创建者或频道管理员可删除
func apiStickyNoteDeleteRest(c *fiber.Ctx) error {
	noteID := c.Params("noteId")
	user := getStickyNoteUser(c)
	if user == nil {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	note, err := model.StickyNoteGet(noteID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "note not found"})
	}

	channelID := note.ChannelID

	// 权限检查：仅创建者或频道管理员可删除
	isCreator := note.CreatorID == user.ID
	isAdmin := false
	if !isCreator {
		roleIDs, _ := model.UserRoleMappingListByUserID(user.ID, channelID, "channel")
		for _, roleID := range roleIDs {
			if strings.HasSuffix(roleID, "-owner") || strings.HasSuffix(roleID, "-admin") {
				isAdmin = true
				break
			}
		}
	}
	if !isCreator && !isAdmin {
		return c.Status(403).JSON(fiber.Map{"error": "只有创建者或管理员可以删除便签"})
	}

	if err := model.StickyNoteDelete(noteID, user.ID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 广播删除事件
	go BroadcastStickyNoteToChannel(channelID, protocol.EventStickyNoteDeleted, &protocol.StickyNoteEventPayload{
		Note:   &protocol.StickyNote{ID: noteID, ChannelID: channelID},
		Action: "delete",
	})

	return c.JSON(fiber.Map{"success": true})
}

// apiStickyNoteUserStateUpdate 更新用户状态
func apiStickyNoteUserStateUpdate(c *fiber.Ctx) error {
	noteID := c.Params("noteId")
	user := getStickyNoteUser(c)
	if user == nil {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		IsOpen    *bool `json:"isOpen"`
		PositionX *int  `json:"positionX"`
		PositionY *int  `json:"positionY"`
		Width     *int  `json:"width"`
		Height    *int  `json:"height"`
		Minimized *bool `json:"minimized"`
		ZIndex    *int  `json:"zIndex"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	// 获取或创建用户状态
	state, err := model.StickyNoteUserStateGet(noteID, user.ID)
	if err != nil {
		state = &model.StickyNoteUserStateModel{
			StickyNoteID: noteID,
			UserID:       user.ID,
		}
		state.ID = utils.NewID()
	}

	// 更新字段
	if req.IsOpen != nil {
		state.IsOpen = *req.IsOpen
		if *req.IsOpen {
			now := time.Now()
			state.LastOpenedAt = &now
		}
	}
	if req.PositionX != nil {
		state.PositionX = *req.PositionX
	}
	if req.PositionY != nil {
		state.PositionY = *req.PositionY
	}
	if req.Width != nil {
		state.Width = *req.Width
	}
	if req.Height != nil {
		state.Height = *req.Height
	}
	if req.Minimized != nil {
		state.Minimized = *req.Minimized
	}
	if req.ZIndex != nil {
		state.ZIndex = *req.ZIndex
	}

	if err := model.StickyNoteUserStateUpsert(state); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"state": state.ToProtocolType()})
}

// apiStickyNotePushRest 推送便签给其他用户 (REST)
func apiStickyNotePushRest(c *fiber.Ctx) error {
	noteID := c.Params("noteId")

	var req struct {
		TargetUserIDs []string `json:"targetUserIds"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	if len(req.TargetUserIDs) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "no target users"})
	}

	note, err := model.StickyNoteGet(noteID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "note not found"})
	}

	note.LoadCreator()

	// 推送给指定用户
	go BroadcastStickyNoteToUsers(req.TargetUserIDs, protocol.EventStickyNotePushed, &protocol.StickyNoteEventPayload{
		Note:          note.ToProtocolType(),
		Action:        "push",
		TargetUserIDs: req.TargetUserIDs,
	})

	return c.JSON(fiber.Map{"success": true})
}

// ========== WebSocket API ==========

// apiStickyNoteUpdateWs WebSocket API: 更新便签内容
func apiStickyNoteUpdateWs(ctx *ChatContext, data *struct {
	NoteID      string `json:"noteId"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	ContentText string `json:"contentText"`
}) (any, error) {
	note, err := model.StickyNoteGet(data.NoteID)
	if err != nil {
		return nil, fmt.Errorf("note not found")
	}

	updates := map[string]interface{}{
		"title":        data.Title,
		"content":      data.Content,
		"content_text": data.ContentText,
		"updated_at":   time.Now(),
	}

	if err := model.StickyNoteUpdate(data.NoteID, updates); err != nil {
		return nil, err
	}

	// 重新加载
	note, _ = model.StickyNoteGet(data.NoteID)
	note.LoadCreator()

	// 广播到频道
	event := &protocol.Event{
		Type:    protocol.EventStickyNoteUpdated,
		Channel: &protocol.Channel{ID: note.ChannelID},
		StickyNote: &protocol.StickyNoteEventPayload{
			Note:   note.ToProtocolType(),
			Action: "update",
		},
	}
	ctx.BroadcastEventInChannel(note.ChannelID, event)

	return map[string]any{"note": note.ToProtocolType()}, nil
}

// apiStickyNoteDeleteWs WebSocket API: 删除便签
func apiStickyNoteDeleteWs(ctx *ChatContext, data *struct {
	NoteID string `json:"noteId"`
}) (any, error) {
	note, err := model.StickyNoteGet(data.NoteID)
	if err != nil {
		return nil, fmt.Errorf("note not found")
	}

	channelID := note.ChannelID

	if err := model.StickyNoteDelete(data.NoteID, ctx.User.ID); err != nil {
		return nil, err
	}

	// 广播删除事件
	event := &protocol.Event{
		Type:    protocol.EventStickyNoteDeleted,
		Channel: &protocol.Channel{ID: channelID},
		StickyNote: &protocol.StickyNoteEventPayload{
			Note:   &protocol.StickyNote{ID: data.NoteID, ChannelID: channelID},
			Action: "delete",
		},
	}
	ctx.BroadcastEventInChannel(channelID, event)

	return map[string]any{"success": true}, nil
}

// apiStickyNotePushWs WebSocket API: 推送便签
func apiStickyNotePushWs(ctx *ChatContext, data *struct {
	NoteID        string   `json:"noteId"`
	TargetUserIDs []string `json:"targetUserIds"`
}) (any, error) {
	if len(data.TargetUserIDs) == 0 {
		return nil, fmt.Errorf("no target users")
	}

	note, err := model.StickyNoteGet(data.NoteID)
	if err != nil {
		return nil, fmt.Errorf("note not found")
	}

	note.LoadCreator()

	// 推送到指定用户
	event := &protocol.Event{
		Type:    protocol.EventStickyNotePushed,
		Channel: &protocol.Channel{ID: note.ChannelID},
		StickyNote: &protocol.StickyNoteEventPayload{
			Note:          note.ToProtocolType(),
			Action:        "push",
			TargetUserIDs: data.TargetUserIDs,
		},
	}

	ctx.BroadcastEventInChannelToUsers(note.ChannelID, data.TargetUserIDs, event)
	return map[string]any{"success": true}, nil
}

// ========== 广播辅助函数 ==========

// BroadcastStickyNoteToChannel 广播便签事件到频道
func BroadcastStickyNoteToChannel(channelID string, eventType protocol.EventName, payload *protocol.StickyNoteEventPayload) {
	event := &protocol.Event{
		Type:       eventType,
		Channel:    &protocol.Channel{ID: channelID},
		StickyNote: payload,
		Timestamp:  time.Now().UnixMilli(),
	}

	userConnMap := getUserConnInfoMap()
	if userConnMap == nil {
		return
	}

	userConnMap.Range(func(userID string, connMap *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		connMap.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
			if info.ChannelId == channelID {
				_ = conn.WriteJSON(struct {
					protocol.Event
					Op protocol.Opcode `json:"op"`
				}{
					Event: *event,
					Op:    protocol.OpEvent,
				})
			}
			return true
		})
		return true
	})
}

// BroadcastStickyNoteToUsers 广播便签事件到指定用户
func BroadcastStickyNoteToUsers(userIDs []string, eventType protocol.EventName, payload *protocol.StickyNoteEventPayload) {
	event := &protocol.Event{
		Type:       eventType,
		StickyNote: payload,
		Timestamp:  time.Now().UnixMilli(),
	}
	if payload != nil && payload.Note != nil && payload.Note.ChannelID != "" {
		event.Channel = &protocol.Channel{ID: payload.Note.ChannelID}
	}

	targetSet := make(map[string]bool)
	for _, id := range userIDs {
		targetSet[id] = true
	}

	userConnMap := getUserConnInfoMap()
	if userConnMap == nil {
		return
	}

	userConnMap.Range(func(userID string, connMap *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		if !targetSet[userID] {
			return true
		}
		connMap.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
			_ = conn.WriteJSON(struct {
				protocol.Event
				Op protocol.Opcode `json:"op"`
			}{
				Event: *event,
				Op:    protocol.OpEvent,
			})
			return true
		})
		return true
	})
}

// ========== 文件夹 API ==========

// apiChannelStickyNoteFolderList 获取频道的所有文件夹
func apiChannelStickyNoteFolderList(c *fiber.Ctx) error {
	channelID := c.Params("channelId")
	user := getStickyNoteUser(c)
	if user == nil {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	folders, err := model.StickyNoteFolderListByChannel(channelID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 转换为协议类型
	result := make([]*protocol.StickyNoteFolder, len(folders))
	for i, f := range folders {
		result[i] = f.ToProtocolType()
	}

	return c.JSON(fiber.Map{"folders": result})
}

// apiChannelStickyNoteFolderCreate 创建文件夹
func apiChannelStickyNoteFolderCreate(c *fiber.Ctx) error {
	channelID := c.Params("channelId")
	user := getStickyNoteUser(c)
	if user == nil {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		Name     string `json:"name"`
		ParentID string `json:"parentId"`
		Color    string `json:"color"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}

	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "channel not found"})
	}

	folder := &model.StickyNoteFolderModel{
		ChannelID: channelID,
		WorldID:   channel.WorldID,
		ParentID:  req.ParentID,
		Name:      req.Name,
		Color:     req.Color,
		CreatorID: user.ID,
	}

	if err := model.StickyNoteFolderCreate(folder); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"folder": folder.ToProtocolType()})
}

// apiStickyNoteFolderUpdate 更新文件夹
func apiStickyNoteFolderUpdate(c *fiber.Ctx) error {
	folderID := c.Params("folderId")
	user := getStickyNoteUser(c)
	if user == nil {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	folder, err := model.StickyNoteFolderGet(folderID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "folder not found"})
	}

	// 权限检查
	canManage, _ := canManageStickyNote(user.ID, folder.ChannelID, folder.CreatorID)
	if !canManage {
		return c.Status(403).JSON(fiber.Map{"error": "permission denied"})
	}

	var req struct {
		Name       *string `json:"name"`
		ParentID   *string `json:"parentId"`
		Color      *string `json:"color"`
		OrderIndex *int    `json:"orderIndex"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}

	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.ParentID != nil {
		updates["parent_id"] = *req.ParentID
	}
	if req.Color != nil {
		updates["color"] = *req.Color
	}
	if req.OrderIndex != nil {
		updates["order_index"] = *req.OrderIndex
	}

	if err := model.StickyNoteFolderUpdate(folderID, updates); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	folder, _ = model.StickyNoteFolderGet(folderID)
	return c.JSON(fiber.Map{"folder": folder.ToProtocolType()})
}

// apiStickyNoteFolderDelete 删除文件夹
func apiStickyNoteFolderDelete(c *fiber.Ctx) error {
	folderID := c.Params("folderId")
	user := getStickyNoteUser(c)
	if user == nil {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	folder, err := model.StickyNoteFolderGet(folderID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "folder not found"})
	}

	// 权限检查
	canManage, _ := canManageStickyNote(user.ID, folder.ChannelID, folder.CreatorID)
	if !canManage {
		return c.Status(403).JSON(fiber.Map{"error": "permission denied"})
	}

	// 将文件夹内的便签移出
	_ = model.StickyNoteClearFolder(folderID)

	if err := model.StickyNoteFolderDelete(folderID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}
