package api

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"sealchat/service"
	"sealchat/utils"
)

// Character card API skeleton for future SealDice integration
// These APIs follow the protocol defined in docs/sealchat-protocol.md

// CharacterPendingRequest stores a pending character API request
type CharacterPendingRequest struct {
	Echo      string
	API       string
	Data      any
	CreatedAt time.Time
	Response  chan json.RawMessage
}

// characterPendingRequests stores pending character API requests by echo ID
var characterPendingRequests = &sync.Map{}

// characterRequestTimeout is the timeout for character API requests
const characterRequestTimeout = 5 * time.Second

// apiCharacterGet handles character.get requests
// This is a SealChat → SealDice API that retrieves character card data
func apiCharacterGet(ctx *ChatContext, msg []byte) {
	data := struct {
		Echo string `json:"echo"`
		Data struct {
			GroupID string `json:"group_id"` // channel_id
			UserID  string `json:"user_id"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(msg, &data); err != nil {
		sendCharacterError(ctx, data.Echo, "请求解析失败")
		return
	}

	// Find selected BOT for this channel
	botConn, err := findBotConnectionForChannel(ctx, data.Data.GroupID)
	if err != nil {
		sendCharacterError(ctx, data.Echo, err.Error())
		return
	}

	// Forward request to BOT and wait for response
	resp := forwardCharacterRequest(botConn, "character.get", data.Echo, data.Data)
	if resp == nil {
		sendCharacterError(ctx, data.Echo, "请求超时")
		return
	}

	sendCharacterResponse(ctx, data.Echo, resp)
}

// apiCharacterSet handles character.set requests
// This is a SealChat → SealDice API that writes character card data
func apiCharacterSet(ctx *ChatContext, msg []byte) {
	data := struct {
		Echo string `json:"echo"`
		Data struct {
			GroupID string                 `json:"group_id"` // channel_id
			UserID  string                 `json:"user_id"`
			Name    string                 `json:"name"`
			Attrs   map[string]interface{} `json:"attrs"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(msg, &data); err != nil {
		sendCharacterError(ctx, data.Echo, "请求解析失败")
		return
	}

	botConn, err := findBotConnectionForChannel(ctx, data.Data.GroupID)
	if err != nil {
		sendCharacterError(ctx, data.Echo, err.Error())
		return
	}

	resp := forwardCharacterRequest(botConn, "character.set", data.Echo, data.Data)
	if resp == nil {
		sendCharacterError(ctx, data.Echo, "请求超时")
		return
	}

	sendCharacterResponse(ctx, data.Echo, resp)
}

// apiCharacterList handles character.list requests
// This is a SealChat → SealDice API that lists user's character cards
func apiCharacterList(ctx *ChatContext, msg []byte) {
	data := struct {
		Echo string `json:"echo"`
		Data struct {
			UserID  string `json:"user_id"`
			GroupID string `json:"group_id"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(msg, &data); err != nil {
		sendCharacterError(ctx, data.Echo, "请求解析失败")
		return
	}

	channelID := resolveCharacterChannelID(ctx, data.Data.GroupID)
	if channelID == "" {
		sendCharacterError(ctx, data.Echo, "缺少频道ID")
		return
	}
	data.Data.GroupID = channelID

	botConn, err := findBotConnectionForChannel(ctx, channelID)
	if err != nil {
		sendCharacterError(ctx, data.Echo, err.Error())
		return
	}

	resp := forwardCharacterRequest(botConn, "character.list", data.Echo, data.Data)
	if resp == nil {
		sendCharacterError(ctx, data.Echo, "请求超时")
		return
	}

	sendCharacterResponse(ctx, data.Echo, resp)
}

// findBotConnectionForChannel finds a BOT WebSocket connection for a specific channel
func findBotConnectionForChannel(ctx *ChatContext, channelID string) (*WsSyncConn, error) {
	if userId2ConnInfoGlobal == nil {
		return nil, errors.New("未找到可用的 BOT 连接")
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, errors.New("缺少频道ID")
	}
	botID, err := service.SelectedBotIdByChannelId(channelID)
	if err != nil {
		return nil, err
	}
	if x, ok := userId2ConnInfoGlobal.Load(botID); ok {
		var activeConn *WsSyncConn
		var activeAt int64 = -1
		x.Range(func(conn *WsSyncConn, value *ConnInfo) bool {
			if value == nil {
				return true
			}
			lastAlive := value.LastAliveTime
			if lastAlive == 0 {
				lastAlive = value.LastPingTime
			}
			if lastAlive > activeAt {
				activeAt = lastAlive
				activeConn = conn
			}
			return true
		})
		if activeConn != nil {
			return activeConn, nil
		}
	}
	return nil, errors.New("所选 BOT 未在线")
}

// findAnyBotConnection finds any available BOT WebSocket connection
func findAnyBotConnection(ctx *ChatContext) *WsSyncConn {
	if userId2ConnInfoGlobal == nil {
		return nil
	}

	var activeConn *WsSyncConn
	var activeAt int64 = -1

	userId2ConnInfoGlobal.Range(func(userID string, connMap *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		connMap.Range(func(conn *WsSyncConn, value *ConnInfo) bool {
			if value == nil || value.User == nil || !value.User.IsBot {
				return true
			}
			lastAlive := value.LastAliveTime
			if lastAlive == 0 {
				lastAlive = value.LastPingTime
			}
			if lastAlive > activeAt {
				activeAt = lastAlive
				activeConn = conn
			}
			return true
		})
		return true
	})

	return activeConn
}

func resolveCharacterChannelID(ctx *ChatContext, groupID string) string {
	channelID := strings.TrimSpace(groupID)
	if channelID == "" && ctx != nil && ctx.ConnInfo != nil {
		channelID = strings.TrimSpace(ctx.ConnInfo.ChannelId)
	}
	return channelID
}

// forwardCharacterRequest forwards a character API request to a BOT
func forwardCharacterRequest(botConn *WsSyncConn, api, echo string, data any) json.RawMessage {
	if botConn == nil {
		return nil
	}

	// Create pending request with response channel
	respChan := make(chan json.RawMessage, 1)
	pending := &CharacterPendingRequest{
		Echo:      echo,
		API:       api,
		Data:      data,
		CreatedAt: time.Now(),
		Response:  respChan,
	}
	characterPendingRequests.Store(echo, pending)
	defer characterPendingRequests.Delete(echo)

	// Send request to BOT
	req := map[string]any{
		"api":  api,
		"echo": echo,
		"data": data,
	}
	if err := botConn.WriteJSON(req); err != nil {
		return nil
	}

	// Wait for response with timeout
	select {
	case resp := <-respChan:
		return resp
	case <-time.After(characterRequestTimeout):
		return nil
	}
}

// HandleCharacterResponse processes a character API response from BOT
// This should be called when receiving a response with empty "api" field
func HandleCharacterResponse(echo string, data json.RawMessage) bool {
	pending, ok := characterPendingRequests.Load(echo)
	if !ok {
		return false
	}

	req := pending.(*CharacterPendingRequest)

	select {
	case req.Response <- data:
	default:
	}

	return true
}

func sendCharacterError(ctx *ChatContext, echo, errMsg string) {
	resp := map[string]any{
		"api":  "",
		"echo": echo,
		"data": map[string]any{
			"ok":    false,
			"error": errMsg,
		},
	}
	_ = ctx.Conn.WriteJSON(resp)
}

func sendCharacterResponse(ctx *ChatContext, echo string, data json.RawMessage) {
	result := map[string]any{
		"api":  "",
		"echo": echo,
	}
	if len(data) == 0 {
		result["data"] = map[string]any{
			"ok":    false,
			"error": "响应为空",
		}
	} else {
		result["data"] = json.RawMessage(data)
	}
	_ = ctx.Conn.WriteJSON(result)
}

// apiCharacterNew handles character.new requests
func apiCharacterNew(ctx *ChatContext, msg []byte) {
	data := struct {
		Echo string `json:"echo"`
		Data struct {
			UserID    string `json:"user_id"`
			GroupID   string `json:"group_id"`
			Name      string `json:"name"`
			SheetType string `json:"sheet_type"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(msg, &data); err != nil {
		sendCharacterError(ctx, data.Echo, "请求解析失败")
		return
	}

	botConn, err := findBotConnectionForChannel(ctx, data.Data.GroupID)
	if err != nil {
		sendCharacterError(ctx, data.Echo, err.Error())
		return
	}

	resp := forwardCharacterRequest(botConn, "character.new", data.Echo, data.Data)
	if resp == nil {
		sendCharacterError(ctx, data.Echo, "请求超时")
		return
	}

	sendCharacterResponse(ctx, data.Echo, resp)
}

// apiCharacterSave handles character.save requests
func apiCharacterSave(ctx *ChatContext, msg []byte) {
	data := struct {
		Echo string `json:"echo"`
		Data struct {
			UserID    string `json:"user_id"`
			GroupID   string `json:"group_id"`
			Name      string `json:"name"`
			SheetType string `json:"sheet_type"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(msg, &data); err != nil {
		sendCharacterError(ctx, data.Echo, "请求解析失败")
		return
	}

	botConn, err := findBotConnectionForChannel(ctx, data.Data.GroupID)
	if err != nil {
		sendCharacterError(ctx, data.Echo, err.Error())
		return
	}

	resp := forwardCharacterRequest(botConn, "character.save", data.Echo, data.Data)
	if resp == nil {
		sendCharacterError(ctx, data.Echo, "请求超时")
		return
	}

	sendCharacterResponse(ctx, data.Echo, resp)
}

// apiCharacterTag handles character.tag requests
func apiCharacterTag(ctx *ChatContext, msg []byte) {
	data := struct {
		Echo string `json:"echo"`
		Data struct {
			UserID  string `json:"user_id"`
			GroupID string `json:"group_id"`
			Name    string `json:"name"`
			ID      string `json:"id"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(msg, &data); err != nil {
		sendCharacterError(ctx, data.Echo, "请求解析失败")
		return
	}

	botConn, err := findBotConnectionForChannel(ctx, data.Data.GroupID)
	if err != nil {
		sendCharacterError(ctx, data.Echo, err.Error())
		return
	}

	resp := forwardCharacterRequest(botConn, "character.tag", data.Echo, data.Data)
	if resp == nil {
		sendCharacterError(ctx, data.Echo, "请求超时")
		return
	}

	sendCharacterResponse(ctx, data.Echo, resp)
}

// apiCharacterUntagAll handles character.untagAll requests
func apiCharacterUntagAll(ctx *ChatContext, msg []byte) {
	data := struct {
		Echo string `json:"echo"`
		Data struct {
			UserID  string `json:"user_id"`
			GroupID string `json:"group_id"`
			Name    string `json:"name"`
			ID      string `json:"id"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(msg, &data); err != nil {
		sendCharacterError(ctx, data.Echo, "请求解析失败")
		return
	}

	channelID := resolveCharacterChannelID(ctx, data.Data.GroupID)
	if channelID == "" {
		sendCharacterError(ctx, data.Echo, "缺少频道ID")
		return
	}
	data.Data.GroupID = channelID
	botConn, err := findBotConnectionForChannel(ctx, channelID)
	if err != nil {
		sendCharacterError(ctx, data.Echo, err.Error())
		return
	}

	resp := forwardCharacterRequest(botConn, "character.untagAll", data.Echo, data.Data)
	if resp == nil {
		sendCharacterError(ctx, data.Echo, "请求超时")
		return
	}

	sendCharacterResponse(ctx, data.Echo, resp)
}

// apiCharacterLoad handles character.load requests
func apiCharacterLoad(ctx *ChatContext, msg []byte) {
	data := struct {
		Echo string `json:"echo"`
		Data struct {
			UserID  string `json:"user_id"`
			GroupID string `json:"group_id"`
			Name    string `json:"name"`
			ID      string `json:"id"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(msg, &data); err != nil {
		sendCharacterError(ctx, data.Echo, "请求解析失败")
		return
	}

	botConn, err := findBotConnectionForChannel(ctx, data.Data.GroupID)
	if err != nil {
		sendCharacterError(ctx, data.Echo, err.Error())
		return
	}

	resp := forwardCharacterRequest(botConn, "character.load", data.Echo, data.Data)
	if resp == nil {
		sendCharacterError(ctx, data.Echo, "请求超时")
		return
	}

	sendCharacterResponse(ctx, data.Echo, resp)
}

// apiCharacterDelete handles character.delete requests
func apiCharacterDelete(ctx *ChatContext, msg []byte) {
	data := struct {
		Echo string `json:"echo"`
		Data struct {
			UserID  string `json:"user_id"`
			GroupID string `json:"group_id"`
			Name    string `json:"name"`
			ID      string `json:"id"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(msg, &data); err != nil {
		sendCharacterError(ctx, data.Echo, "请求解析失败")
		return
	}

	channelID := resolveCharacterChannelID(ctx, data.Data.GroupID)
	if channelID == "" {
		sendCharacterError(ctx, data.Echo, "缺少频道ID")
		return
	}
	data.Data.GroupID = channelID

	botConn, err := findBotConnectionForChannel(ctx, channelID)
	if err != nil {
		sendCharacterError(ctx, data.Echo, err.Error())
		return
	}

	resp := forwardCharacterRequest(botConn, "character.delete", data.Echo, data.Data)
	if resp == nil {
		sendCharacterError(ctx, data.Echo, "请求超时")
		return
	}

	sendCharacterResponse(ctx, data.Echo, resp)
}
