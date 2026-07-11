package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/service"
	"sealchat/utils"
)

const (
	appNotificationClientID           = "sealchat_android"
	appNotificationRedirectURI        = "sealchat-app://notify-auth"
	appNotificationHeartbeat          = 45
	appNotificationMaxWhitelistWorlds = 100
)

func bindAppNotificationRoutes(app *fiber.App, webURL string) {
	service.StartAppNotificationCleanup()
	app.Get("/.well-known/sealchat-app.json", AppNotificationDiscovery)
	base := joinWebPath(webURL, "api/app-notify/v1")
	app.Get(base+"/authorize", SignCheckMiddleware, AppNotificationAuthorizeGet)
	app.Post(base+"/authorize", SignCheckMiddleware, AppNotificationAuthorizePost)
	app.Post(base+"/token", AppNotificationToken)
	device := app.Group(base, AppNotificationDeviceMiddleware)
	device.Get("/device", AppNotificationDeviceGet)
	device.Put("/device/context", AppNotificationDeviceContextPut)
	device.Delete("/device", AppNotificationDeviceDelete)
	device.Get("/stream", AppNotificationStream)
	device.Post("/acks", AppNotificationAcks)
}

var enqueueAppNotificationForMessage = service.EnqueueAppNotificationForMessage

func notifyAppMessageCreated(messageID string) {
	go func() {
		if err := enqueueAppNotificationForMessage(messageID, currentAppWebURL()); err != nil {
			log.Printf("app-notify: 构建消息通知失败 message=%s err=%v", messageID, err)
		}
	}()
}

func AppNotificationDiscovery(c *fiber.Ctx) error {
	instanceID, err := model.EnsureAppNotificationInstanceID()
	if err != nil {
		return sendAppNotificationError(c, http.StatusInternalServerError, "internal_error", "无法读取实例标识")
	}
	webURL := ""
	name := "SealChat"
	if appConfig != nil {
		webURL = appConfig.WebUrl
		if strings.TrimSpace(appConfig.PageTitle) != "" {
			name = appConfig.PageTitle
		}
	}
	base := joinWebPath(webURL, "api/app-notify/v1")
	origin := c.Protocol() + "://" + c.Hostname()
	if c.Context().URI().Host() != nil {
		origin = c.Protocol() + "://" + string(c.Context().URI().Host())
	}
	return c.JSON(fiber.Map{
		"schema_version": "1.0",
		"instance":       fiber.Map{"instance_id": instanceID, "name": name, "base_url": origin + normalizeWebRoot(webURL)},
		"app_notification": fiber.Map{
			"enabled": true, "api_version": "1", "api_base": base,
			"authorization_endpoint": base + "/authorize", "token_endpoint": base + "/token",
			"stream_endpoint": base + "/stream", "ack_endpoint": base + "/acks",
			"device_endpoint": base + "/device", "context_endpoint": base + "/device/context",
			"heartbeat_seconds": appNotificationHeartbeat, "event_retention_seconds": 3600,
			"features": fiber.Map{"sse": true, "event_replay": true, "opened_ack": true, "world_scoped": true, "world_whitelist": true},
		},
	})
}

func AppNotificationAuthorizeGet(c *fiber.Ctx) error {
	user, ok := c.Locals("user").(*model.UserModel)
	if !ok || user == nil || user.ID == "" {
		return sendAppNotificationError(c, http.StatusUnauthorized, "login_required", "请先登录")
	}
	if c.Query("response_type") != "code" || c.Query("client_id") != appNotificationClientID || c.Query("redirect_uri") != appNotificationRedirectURI || c.Query("code_challenge_method") != "S256" {
		return sendAppNotificationError(c, http.StatusBadRequest, "invalid_request", "授权参数无效")
	}
	challenge := strings.TrimSpace(c.Query("code_challenge"))
	state := strings.TrimSpace(c.Query("state"))
	installationID := strings.TrimSpace(c.Query("installation_id"))
	if challenge == "" || len(state) < 16 || installationID == "" {
		return sendAppNotificationError(c, http.StatusBadRequest, "invalid_request", "缺少授权参数")
	}
	requestID := service.DefaultAppNotificationHub.StoreAuthorizationRequest(service.AppNotificationAuthorizationRequest{
		UserID: user.ID, InstallationID: installationID, CodeChallenge: challenge,
		ClientID: appNotificationClientID, RedirectURI: appNotificationRedirectURI, State: state,
		ExpiresAt: time.Now().Add(time.Minute),
	})
	c.Type("html", "utf-8")
	return c.SendString(fmt.Sprintf(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>设备通知授权</title></head><body><main><h1>允许此设备接收 SealChat 在线消息通知</h1><p>账号：%s</p><form method="post"><input type="hidden" name="request_id" value="%s"><button name="decision" value="allow" type="submit">允许</button><button name="decision" value="deny" type="submit">拒绝</button></form></main></body></html>`, html.EscapeString(user.Nickname), html.EscapeString(requestID)))
}

func AppNotificationAuthorizePost(c *fiber.Ctx) error {
	requestID := strings.TrimSpace(c.FormValue("request_id"))
	decision := strings.TrimSpace(c.FormValue("decision"))
	request, ok := service.DefaultAppNotificationHub.GetAuthorizationRequest(requestID)
	if !ok {
		return sendAppNotificationError(c, http.StatusBadRequest, "authorization_expired", "授权请求已失效")
	}
	user, _ := c.Locals("user").(*model.UserModel)
	if user == nil || user.ID != request.UserID {
		return sendAppNotificationError(c, http.StatusForbidden, "authorization_mismatch", "授权用户不匹配")
	}
	query := url.Values{"state": []string{request.State}}
	if decision != "allow" {
		service.DefaultAppNotificationHub.DenyAuthorization(requestID)
		query.Set("error", "access_denied")
		return c.Redirect(appNotificationRedirectURI+"?"+query.Encode(), http.StatusSeeOther)
	}
	code, err := service.DefaultAppNotificationHub.ApproveAuthorization(requestID)
	if err != nil {
		return sendAppNotificationError(c, http.StatusBadRequest, "authorization_expired", "授权请求已失效")
	}
	query.Set("code", code)
	return c.Redirect(appNotificationRedirectURI+"?"+query.Encode(), http.StatusSeeOther)
}

type appNotificationTokenRequest struct {
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	Code         string `json:"code"`
	RedirectURI  string `json:"redirect_uri"`
	CodeVerifier string `json:"code_verifier"`
	Device       struct {
		InstallationID string `json:"installation_id"`
		Name           string `json:"name"`
		Platform       string `json:"platform"`
		AppVersion     string `json:"app_version"`
		AppBuild       int    `json:"app_build"`
		OSVersion      string `json:"os_version"`
		Locale         string `json:"locale"`
	} `json:"device"`
}

func AppNotificationToken(c *fiber.Ctx) error {
	var body appNotificationTokenRequest
	if err := c.BodyParser(&body); err != nil {
		return sendAppNotificationError(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
	}
	if body.GrantType != "authorization_code" || body.ClientID != appNotificationClientID || body.RedirectURI != appNotificationRedirectURI {
		return sendAppNotificationError(c, http.StatusBadRequest, "invalid_grant", "授权参数无效")
	}
	grant, err := service.DefaultAppNotificationHub.RedeemAuthorizationCode(body.Code)
	if err != nil || grant.ClientID != body.ClientID || grant.RedirectURI != body.RedirectURI || grant.InstallationID != body.Device.InstallationID {
		return sendAppNotificationError(c, http.StatusBadRequest, "invalid_grant", "授权码无效")
	}
	if err := validateAppNotificationPKCE(body.CodeVerifier, grant.CodeChallenge); err != nil {
		return sendAppNotificationError(c, http.StatusBadRequest, "invalid_grant", "PKCE 校验失败")
	}
	device, token, err := model.UpsertAppNotificationDevice(grant.UserID, model.AppNotificationDeviceInput{
		InstallationID: body.Device.InstallationID, Name: body.Device.Name, Platform: body.Device.Platform,
		AppVersion: body.Device.AppVersion, AppBuild: body.Device.AppBuild, OSVersion: body.Device.OSVersion, Locale: body.Device.Locale,
	})
	if err != nil {
		return sendAppNotificationError(c, http.StatusInternalServerError, "internal_error", "创建设备失败")
	}
	cancelAppNotificationStream(device.ID)
	service.DefaultAppNotificationHub.ResetDevice(device.ID)
	return c.JSON(fiber.Map{
		"token_type": "Bearer", "access_token": token, "expires_at": device.TokenExpiresAt,
		"scope":  []string{"notification:stream", "notification:ack", "device:self"},
		"device": fiber.Map{"device_id": device.ID, "installation_id": device.InstallationID},
		"stream": fiber.Map{"url": joinWebPath(currentAppWebURL(), "api/app-notify/v1/stream"), "heartbeat_seconds": appNotificationHeartbeat},
	})
}

func AppNotificationDeviceMiddleware(c *fiber.Ctx) error {
	deviceID := strings.TrimSpace(c.Get("X-SealChat-Device-ID"))
	token := strings.TrimSpace(c.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	if deviceID == "" || token == "" {
		return sendAppNotificationError(c, http.StatusUnauthorized, "invalid_token", "设备令牌无效")
	}
	device, err := model.VerifyAppNotificationDeviceToken(deviceID, token)
	if err != nil {
		return sendAppNotificationError(c, http.StatusUnauthorized, "invalid_token", "设备令牌无效或已过期")
	}
	user := model.UserGet(device.UserID)
	if user == nil {
		return sendAppNotificationError(c, http.StatusUnauthorized, "invalid_token", "设备绑定账号不存在")
	}
	if user.Disabled {
		return sendAppNotificationError(c, http.StatusUnauthorized, "account_disabled", "账号已禁用")
	}
	if device.ActiveWorldID != "" && !service.IsWorldMember(device.ActiveWorldID, device.UserID) {
		updated, updateErr := model.UpdateAppNotificationDeviceWorld(device.ID, "")
		if updateErr != nil {
			return sendAppNotificationError(c, http.StatusInternalServerError, "internal_error", "清理设备上下文失败")
		}
		device = updated
		cancelAppNotificationStream(device.ID)
		service.DefaultAppNotificationHub.ResetDevice(device.ID)
	}
	c.Locals("appNotifyDevice", device)
	return c.Next()
}

func AppNotificationDeviceGet(c *fiber.Ctx) error {
	device := appNotificationDeviceFromContext(c)
	user := model.UserGet(device.UserID)
	preference, err := model.GetAppNotificationPreference(device.UserID)
	if err != nil {
		return sendAppNotificationError(c, http.StatusInternalServerError, "internal_error", "读取推送设置失败")
	}
	displayName := ""
	if user != nil {
		displayName = user.Nickname
	}
	return c.JSON(fiber.Map{
		"device_id": device.ID, "installation_id": device.InstallationID, "authorized": true,
		"token_expires_at": device.TokenExpiresAt, "last_connected_at": device.LastConnectedAt,
		"active_world_id":       nullableAppNotificationWorldID(device.ActiveWorldID),
		"notification_settings": appNotificationPreferenceResponse(preference),
		"user":                  fiber.Map{"id": device.UserID, "display_name": displayName},
	})
}

func AppNotificationSettingsGet(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	preference, err := model.GetAppNotificationPreference(user.ID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "读取推送设置失败"})
	}
	return c.JSON(appNotificationPreferenceResponse(preference))
}

func AppNotificationSettingsPut(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	var body struct {
		WorldWhitelistEnabled bool     `json:"world_whitelist_enabled"`
		WorldWhitelistIDs     []string `json:"world_whitelist_ids"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请求格式错误"})
	}
	worldIDs := normalizeAppNotificationWorldIDs(body.WorldWhitelistIDs)
	if len(worldIDs) > appNotificationMaxWhitelistWorlds {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "白名单世界数量超出限制"})
	}
	for _, worldID := range worldIDs {
		if !service.IsWorldMember(worldID, user.ID) {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "白名单中存在无权访问的世界"})
		}
	}
	encoded, err := json.Marshal(worldIDs)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "白名单格式错误"})
	}
	preference, err := model.UpsertAppNotificationPreference(user.ID, body.WorldWhitelistEnabled, string(encoded))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "保存推送设置失败"})
	}
	return c.JSON(appNotificationPreferenceResponse(preference))
}

func AppNotificationDeviceContextPut(c *fiber.Ctx) error {
	device := appNotificationDeviceFromContext(c)
	var body struct {
		DeviceID      string  `json:"device_id"`
		ActiveWorldID *string `json:"active_world_id"`
	}
	if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.DeviceID) != device.ID {
		return sendAppNotificationError(c, http.StatusBadRequest, "invalid_request", "设备参数无效")
	}
	worldID := ""
	if body.ActiveWorldID != nil {
		worldID = strings.TrimSpace(*body.ActiveWorldID)
		if worldID != "" && !service.IsWorldMember(worldID, device.UserID) {
			return sendAppNotificationError(c, http.StatusForbidden, "world_access_denied", "当前用户不属于该世界")
		}
	}
	updated, err := model.UpdateAppNotificationDeviceWorld(device.ID, worldID)
	if err != nil {
		return sendAppNotificationError(c, http.StatusInternalServerError, "internal_error", "更新设备上下文失败")
	}
	cancelAppNotificationStream(device.ID)
	service.DefaultAppNotificationHub.ResetDevice(device.ID)
	return c.JSON(fiber.Map{
		"active_world_id": nullableAppNotificationWorldID(updated.ActiveWorldID),
		"stream_cursor":   fiber.Map{"latest_event_id": "", "latest_sequence": updated.LastSequence},
	})
}

func AppNotificationDeviceDelete(c *fiber.Ctx) error {
	device := appNotificationDeviceFromContext(c)
	if err := model.RevokeAppNotificationDevice(device.ID); err != nil {
		return sendAppNotificationError(c, http.StatusInternalServerError, "internal_error", "注销设备失败")
	}
	cancelAppNotificationStream(device.ID)
	service.DefaultAppNotificationHub.ResetDevice(device.ID)
	return c.SendStatus(http.StatusNoContent)
}

func appNotificationPKCEChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func validateAppNotificationPKCE(verifier, challenge string) error {
	if len(verifier) < 43 || len(verifier) > 128 {
		return fmt.Errorf("invalid code_verifier length")
	}
	got := appNotificationPKCEChallenge(verifier)
	if len(got) != len(challenge) || subtle.ConstantTimeCompare([]byte(got), []byte(challenge)) != 1 {
		return fmt.Errorf("code_challenge mismatch")
	}
	return nil
}

func sendAppNotificationError(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(fiber.Map{"error": fiber.Map{"code": code, "message": message, "request_id": "req_" + utils.NewID()}})
}

func appNotificationDeviceFromContext(c *fiber.Ctx) *model.AppNotificationDeviceModel {
	device, _ := c.Locals("appNotifyDevice").(*model.AppNotificationDeviceModel)
	return device
}

func currentAppWebURL() string {
	if appConfig == nil {
		return ""
	}
	return appConfig.WebUrl
}

func nullableAppNotificationWorldID(worldID string) any {
	if strings.TrimSpace(worldID) == "" {
		return nil
	}
	return worldID
}

func appNotificationPreferenceResponse(preference *model.AppNotificationPreferenceModel) fiber.Map {
	worldIDs := []string{}
	if preference != nil {
		worldIDs = normalizeAppNotificationWorldIDs(appNotificationWorldIDs(preference.WorldWhitelistJSON))
	}
	return fiber.Map{
		"world_whitelist_enabled": preference != nil && preference.WorldWhitelistEnabled,
		"world_whitelist_ids":     worldIDs,
	}
}

func appNotificationWorldIDs(raw string) []string {
	var worldIDs []string
	if json.Unmarshal([]byte(raw), &worldIDs) != nil {
		return []string{}
	}
	return worldIDs
}

func normalizeAppNotificationWorldIDs(worldIDs []string) []string {
	result := make([]string, 0, len(worldIDs))
	seen := make(map[string]struct{}, len(worldIDs))
	for _, worldID := range worldIDs {
		worldID = strings.TrimSpace(worldID)
		if worldID == "" {
			continue
		}
		if _, ok := seen[worldID]; ok {
			continue
		}
		seen[worldID] = struct{}{}
		result = append(result, worldID)
	}
	return result
}
