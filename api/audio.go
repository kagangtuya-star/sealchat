package api

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
)

func AudioAssetList(c *fiber.Ctx) error {
	filters := service.AudioAssetFilters{
		Query:      c.Query("query"),
		Page:       c.QueryInt("page", 1),
		PageSize:   c.QueryInt("pageSize", 200),
		Tags:       queryStringSlice(c, "tags[]", "tags"),
		CreatorIDs: queryStringSlice(c, "creatorIds[]", "creatorIds"),
	}
	if folder := strings.TrimSpace(c.Query("folderId")); folder != "" {
		switch folder {
		case "null", "root":
			filters.FolderID = strPtr("")
		case "all":
			// no filter
		default:
			filters.FolderID = strPtr(folder)
		}
	}
	filters.HasSceneOnly = c.QueryBool("hasSceneOnly")
	if mins := queryFloatSlice(c, "durationRange[]", "durationRange"); len(mins) > 0 {
		filters.DurationMin = mins[0]
		if len(mins) > 1 {
			filters.DurationMax = mins[1]
		}
	}
	if v := strings.TrimSpace(c.Query("durationMin")); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			filters.DurationMin = parsed
		}
	}
	if v := strings.TrimSpace(c.Query("durationMax")); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			filters.DurationMax = parsed
		}
	}
	items, total, err := service.AudioListAssets(filters)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "加载音频素材失败")
	}
	return c.JSON(fiber.Map{
		"items":    items,
		"total":    total,
		"page":     filters.Page,
		"pageSize": filters.PageSize,
	})
}

func AudioAssetGet(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "缺少资源ID")
	}
	asset, err := service.AudioGetAsset(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "素材不存在")
		}
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取素材失败")
	}
	return c.JSON(asset)
}

func AudioAssetUpload(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		status := fiber.StatusBadRequest
		message := "未找到上传文件"
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) || strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			status = fiber.StatusRequestEntityTooLarge
			message = "音频文件超过服务器上传限制"
		}
		return wrapErrorStatus(c, status, err, message)
	}
	user := getCurUser(c)
	name := c.FormValue("name")
	folderID := parseOptionalString(c.FormValue("folderId"))
	tags := splitCSV(c.FormValue("tags"))
	visibility := model.AudioVisibilityPublic
	if v := strings.TrimSpace(c.FormValue("visibility")); v != "" {
		visibility = model.AudioAssetVisibility(v)
	}
	asset, err := service.AudioCreateAssetFromUpload(file, service.AudioUploadOptions{
		Name:        name,
		FolderID:    folderID,
		Tags:        tags,
		Description: c.FormValue("description"),
		Visibility:  visibility,
		CreatedBy:   user.ID,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAudioTooLarge):
			return wrapErrorStatus(c, fiber.StatusRequestEntityTooLarge, err, err.Error())
		case errors.Is(err, service.ErrAudioUnsupportedMime):
			return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
		default:
			return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "上传音频失败")
		}
	}
	needsTranscode := asset.TranscodeStatus == model.AudioTranscodePending
	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"item":           asset,
		"needsTranscode": needsTranscode,
	})
}

func AudioAssetUpdate(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "缺少资源ID")
	}
	var req struct {
		Name        *string                     `json:"name"`
		Description *string                     `json:"description"`
		Tags        []string                    `json:"tags"`
		Visibility  *model.AudioAssetVisibility `json:"visibility"`
		FolderID    *string                     `json:"folderId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求体格式错误")
	}
	input := service.AudioAssetUpdateInput{
		Name:        req.Name,
		Description: req.Description,
		Tags:        req.Tags,
		Visibility:  req.Visibility,
		FolderID:    req.FolderID,
		UpdatedBy:   getCurUser(c).ID,
	}
	asset, err := service.AudioUpdateAsset(id, input)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "更新素材失败")
	}
	return c.JSON(fiber.Map{"item": asset})
}

func AudioAssetDelete(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "缺少资源ID")
	}
	hard := c.QueryBool("hard")
	if err := service.AudioDeleteAsset(id, hard); err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "删除素材失败")
	}
	return c.JSON(fiber.Map{"message": "已删除"})
}

func AudioFolderList(c *fiber.Ctx) error {
	items, err := service.AudioListFolders()
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取文件夹失败")
	}
	return c.JSON(fiber.Map{"items": items})
}

func AudioFolderCreate(c *fiber.Ctx) error {
	var req struct {
		Name     string  `json:"name"`
		ParentID *string `json:"parentId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求体格式错误")
	}
	folder, err := service.AudioCreateFolder(service.AudioFolderPayload{
		Name:     req.Name,
		ParentID: req.ParentID,
		ActorID:  getCurUser(c).ID,
	})
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{"item": folder})
}

func AudioFolderUpdate(c *fiber.Ctx) error {
	id := c.Params("id")
	var req struct {
		Name     string  `json:"name"`
		ParentID *string `json:"parentId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求体格式错误")
	}
	folder, err := service.AudioUpdateFolder(id, service.AudioFolderPayload{
		Name:     req.Name,
		ParentID: req.ParentID,
		ActorID:  getCurUser(c).ID,
	})
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
	}
	return c.JSON(fiber.Map{"item": folder})
}

func AudioFolderDelete(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := service.AudioDeleteFolder(id); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
	}
	return c.JSON(fiber.Map{"message": "已删除"})
}

func AudioSceneList(c *fiber.Ctx) error {
	channelScope := strings.TrimSpace(c.Query("channelScope"))
	scenes, err := service.AudioListScenes(channelScope)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取场景失败")
	}
	return c.JSON(fiber.Map{"items": scenes})
}

func AudioSceneCreate(c *fiber.Ctx) error {
	var req audioSceneRequest
	if err := c.BodyParser(&req); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求体格式错误")
	}
	scene, err := service.AudioCreateScene(req.toInput(getCurUser(c).ID))
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{"item": scene})
}

func AudioSceneUpdate(c *fiber.Ctx) error {
	id := c.Params("id")
	var req audioSceneRequest
	if err := c.BodyParser(&req); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求体格式错误")
	}
	scene, err := service.AudioUpdateScene(id, req.toInput(getCurUser(c).ID))
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
	}
	return c.JSON(fiber.Map{"item": scene})
}

func AudioSceneDelete(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := service.AudioDeleteScene(id); err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "删除场景失败")
	}
	return c.JSON(fiber.Map{"message": "已删除"})
}

func AudioAssetStream(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "缺少资源ID")
	}
	asset, err := service.AudioGetAsset(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "素材不存在")
		}
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取素材失败")
	}
	variantLabel := c.Query("variant")
	variant := service.AudioVariantFor(asset, variantLabel)
	if variant.StorageType == model.AudioStorageS3 {
		return c.Redirect(variant.ObjectKey, fiber.StatusTemporaryRedirect)
	}
	file, info, resolved, err := service.AudioOpenLocalVariant(asset, variantLabel)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "打开音频文件失败")
	}

	if resolved.ObjectKey != "" {
		variant = resolved
	}
	contentType := guessContentType(variant.ObjectKey)
	c.Set("X-Asset-Bitrate", strconv.Itoa(variant.BitrateKbps))
	c.Set("X-Asset-Duration", fmt.Sprintf("%.3f", variant.Duration))
	c.Set("X-Asset-Size", strconv.FormatInt(variant.Size, 10))
	return streamFileWithRange(c, file, info.Size(), contentType)
}

func AudioPlaybackStateGet(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Query("channelId"))
	if channelID == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "缺少频道ID")
	}
	user := getCurUser(c)
	if user == nil {
		return wrapErrorStatus(c, fiber.StatusUnauthorized, nil, "未登录")
	}
	if err := ensureChannelMembership(user.ID, channelID); err != nil {
		return wrapErrorStatus(c, fiber.StatusForbidden, err, "仅频道成员可查看播放状态")
	}
	state, err := service.AudioGetPlaybackState(channelID)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取播放状态失败")
	}
	return c.JSON(fiber.Map{"state": buildAudioPlaybackResponse(state)})
}

func AudioPlaybackStateSet(c *fiber.Ctx) error {
	var req audioPlaybackStateRequest
	if err := c.BodyParser(&req); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求体解析失败")
	}
	req.ChannelID = strings.TrimSpace(req.ChannelID)
	if req.ChannelID == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "缺少频道ID")
	}
	user := getCurUser(c)
	if user == nil {
		return wrapErrorStatus(c, fiber.StatusUnauthorized, nil, "未登录")
	}
	if err := ensureChannelMembership(user.ID, req.ChannelID); err != nil {
		return wrapErrorStatus(c, fiber.StatusForbidden, err, "仅频道成员可更新播放状态")
	}
	state, err := service.AudioUpsertPlaybackState(service.AudioPlaybackUpdateInput{
		ChannelID:    req.ChannelID,
		SceneID:      req.SceneID,
		Tracks:       req.Tracks,
		IsPlaying:    req.IsPlaying,
		Position:     req.Position,
		LoopEnabled:  req.LoopEnabled,
		PlaybackRate: req.PlaybackRate,
		ActorID:      user.ID,
	})
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "更新播放状态失败")
	}
	if state != nil {
		broadcastAudioPlaybackState(user, state)
	}
	return c.JSON(fiber.Map{"state": buildAudioPlaybackResponse(state)})
}

type audioSceneRequest struct {
	Name         string                  `json:"name"`
	Description  string                  `json:"description"`
	Tracks       []model.AudioSceneTrack `json:"tracks"`
	Tags         []string                `json:"tags"`
	Order        int                     `json:"order"`
	ChannelScope *string                 `json:"channelScope"`
}

func (r audioSceneRequest) toInput(actor string) service.AudioSceneInput {
	return service.AudioSceneInput{
		Name:         r.Name,
		Description:  r.Description,
		Tracks:       r.Tracks,
		Tags:         r.Tags,
		Order:        r.Order,
		ChannelScope: r.ChannelScope,
		ActorID:      actor,
	}
}

func queryStringSlice(c *fiber.Ctx, keys ...string) []string {
	args := c.Context().QueryArgs()
	set := map[string]struct{}{}
	var out []string
	for _, key := range keys {
		values := args.PeekMulti(key)
		if len(values) == 0 && strings.HasSuffix(key, "[]") {
			values = args.PeekMulti(strings.TrimSuffix(key, "[]"))
		}
		for _, raw := range values {
			val := strings.TrimSpace(string(raw))
			if val == "" {
				continue
			}
			if _, ok := set[val]; ok {
				continue
			}
			set[val] = struct{}{}
			out = append(out, val)
		}
		if single := strings.TrimSpace(c.Query(strings.TrimSuffix(key, "[]"))); single != "" {
			for _, part := range strings.Split(single, ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				if _, ok := set[part]; ok {
					continue
				}
				set[part] = struct{}{}
				out = append(out, part)
			}
		}
	}
	return out
}

func queryFloatSlice(c *fiber.Ctx, keys ...string) []float64 {
	var result []float64
	for _, key := range keys {
		values := queryStringSlice(c, key)
		for _, val := range values {
			if parsed, err := strconv.ParseFloat(val, 64); err == nil {
				result = append(result, parsed)
			}
		}
	}
	return result
}

func parseOptionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func splitCSV(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, ",")
	var out []string
	set := map[string]struct{}{}
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		if _, ok := set[p]; ok {
			continue
		}
		set[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func strPtr(value string) *string {
	v := value
	return &v
}

type audioPlaybackStateRequest struct {
	ChannelID    string                    `json:"channelId"`
	SceneID      *string                   `json:"sceneId"`
	Tracks       []service.AudioTrackState `json:"tracks"`
	IsPlaying    bool                      `json:"isPlaying"`
	Position     float64                   `json:"position"`
	LoopEnabled  bool                      `json:"loopEnabled"`
	PlaybackRate float64                   `json:"playbackRate"`
}

func ensureChannelMembership(userID, channelID string) error {
	member, err := model.MemberGetByUserIDAndChannelIDBase(userID, channelID, "", false)
	if err != nil {
		return err
	}
	if member == nil {
		return fmt.Errorf("channel membership required")
	}
	return nil
}

func buildAudioPlaybackResponse(state *model.AudioPlaybackState) interface{} {
	if state == nil {
		return nil
	}
	tracks := []model.AudioTrackState(state.Tracks)
	return fiber.Map{
		"channelId":    state.ChannelID,
		"sceneId":      state.SceneID,
		"tracks":       tracks,
		"isPlaying":    state.IsPlaying,
		"position":     state.Position,
		"loopEnabled":  state.LoopEnabled,
		"playbackRate": state.PlaybackRate,
		"updatedBy":    state.UpdatedBy,
		"updatedAt":    state.UpdatedAt,
	}
}

func broadcastAudioPlaybackState(operator *model.UserModel, state *model.AudioPlaybackState) {
	if operator == nil || state == nil {
		return
	}
	if channelUsersMapGlobal == nil || userId2ConnInfoGlobal == nil {
		return
	}
	payload := &protocol.AudioPlaybackStatePayload{
		ChannelID:    state.ChannelID,
		SceneID:      state.SceneID,
		Tracks:       convertTrackStates(state.Tracks),
		IsPlaying:    state.IsPlaying,
		Position:     state.Position,
		LoopEnabled:  state.LoopEnabled,
		PlaybackRate: state.PlaybackRate,
		UpdatedBy:    state.UpdatedBy,
		UpdatedAt:    state.UpdatedAt.Unix(),
	}
	event := &protocol.Event{
		Type: protocol.EventAudioStateUpdated,
		Channel: &protocol.Channel{
			ID: state.ChannelID,
		},
		User: &protocol.User{
			ID:     operator.ID,
			Nick:   operator.Nickname,
			Name:   operator.Username,
			Avatar: operator.Avatar,
		},
		AudioState: payload,
	}
	ctx := &ChatContext{
		User:            operator,
		ChannelUsersMap: channelUsersMapGlobal,
		UserId2ConnInfo: userId2ConnInfoGlobal,
	}
	ctx.BroadcastEventInChannel(state.ChannelID, event)
}

func convertTrackStates(list model.JSONList[model.AudioTrackState]) []protocol.AudioTrackState {
	result := make([]protocol.AudioTrackState, 0, len(list))
	for _, item := range list {
		result = append(result, protocol.AudioTrackState{
			Type:    item.Type,
			AssetID: item.AssetID,
			Volume:  item.Volume,
			Muted:   item.Muted,
			Solo:    item.Solo,
			FadeIn:  item.FadeIn,
			FadeOut: item.FadeOut,
		})
	}
	return result
}

func guessContentType(objectKey string) string {
	switch strings.ToLower(filepath.Ext(objectKey)) {
	case ".ogg", ".opus":
		return "audio/ogg"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".aac":
		return "audio/aac"
	case ".flac":
		return "audio/flac"
	case ".webm":
		return "audio/webm"
	default:
		return "application/octet-stream"
	}
}

func streamFileWithRange(c *fiber.Ctx, file *os.File, size int64, contentType string) error {
	rangeHeader := c.Get("Range")
	c.Set("Accept-Ranges", "bytes")
	c.Set(fiber.HeaderContentType, contentType)
	if rangeHeader == "" {
		c.Set("Content-Length", strconv.FormatInt(size, 10))
		err := c.SendStream(file)
		_ = file.Close()
		return err
	}
	start, end, err := parseRange(rangeHeader, size)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusRequestedRangeNotSatisfiable, err, "无效的 Range 请求")
	}
	length := end - start + 1
	c.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
	c.Set("Content-Length", strconv.FormatInt(length, 10))
	c.Status(fiber.StatusPartialContent)
	section := io.NewSectionReader(file, start, length)
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer file.Close()
		if _, err := io.CopyN(w, section, length); err != nil && err != io.EOF {
			// 无法直接返回错误，只能记录
			fmt.Printf("audio stream copy error: %v\n", err)
		}
		if err := w.Flush(); err != nil {
			fmt.Printf("audio stream flush error: %v\n", err)
		}
	})
	return nil
}

func parseRange(header string, size int64) (int64, int64, error) {
	if header == "" || !strings.HasPrefix(header, "bytes=") {
		return 0, size - 1, nil
	}
	rangeSpec := strings.TrimPrefix(header, "bytes=")
	parts := strings.Split(rangeSpec, ",")
	segment := strings.TrimSpace(parts[0])
	se := strings.Split(segment, "-")
	if len(se) != 2 {
		return 0, 0, fmt.Errorf("invalid range")
	}
	var start, end int64
	var err error
	if se[0] == "" {
		// suffix range
		length, err := strconv.ParseInt(se[1], 10, 64)
		if err != nil {
			return 0, 0, err
		}
		if length > size {
			length = size
		}
		start = size - length
		end = size - 1
	} else {
		start, err = strconv.ParseInt(se[0], 10, 64)
		if err != nil {
			return 0, 0, err
		}
		if se[1] == "" {
			end = size - 1
		} else {
			end, err = strconv.ParseInt(se[1], 10, 64)
			if err != nil {
				return 0, 0, err
			}
		}
	}
	if start < 0 || end >= size || start > end {
		return 0, 0, fmt.Errorf("invalid range bounds")
	}
	return start, end, nil
}
