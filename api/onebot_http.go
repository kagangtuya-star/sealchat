package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/service"
)

func oneBotHTTPWorks(app *fiber.App) {
	app.Use(func(c *fiber.Ctx) error {
		if !oneBotHTTPSupportsMethod(c.Method()) {
			return c.Next()
		}
		if _, ok := matchOneBotHTTPActionForAnyBot(c); !ok {
			return c.Next()
		}
		return oneBotHTTPHandler(c)
	})
}

func oneBotHTTPHandler(c *fiber.Ctx) error {
	action, ok := matchOneBotHTTPActionForAnyBot(c)
	if !ok {
		return c.SendStatus(fiber.StatusNotFound)
	}

	params, err := parseOneBotHTTPParams(c)
	if err != nil {
		var httpErr *fiber.Error
		if errors.As(err, &httpErr) {
			return c.Status(httpErr.Code).JSON(oneBotFailureResponse(oneBotBadRequest(httpErr.Message), nil))
		}
		return c.Status(fiber.StatusBadRequest).JSON(oneBotFailureResponse(oneBotBadRequest("invalid params"), nil))
	}

	token := resolveOneBotAccessToken(c.Get("Authorization"))
	if token == "" {
		token = resolveOneBotAccessToken(valueString(params["access_token"]))
	}
	if token == "" {
		token = resolveOneBotAccessToken(c.Query("access_token"))
	}
	if token == "" {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	delete(params, "access_token")

	botUser, _, err := resolveOneBotBotFromToken(token)
	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(oneBotFailureResponse(err, nil))
	}
	action, ok = matchOneBotHTTPActionForBot(c.Path(), botUser.ID)
	if !ok {
		return c.SendStatus(fiber.StatusNotFound)
	}

	session, err := newOneBotHTTPSession(botUser)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(oneBotFailureResponse(err, nil))
	}

	rawParams, err := json.Marshal(params)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(oneBotFailureResponse(oneBotBadRequest("invalid params"), nil))
	}

	resp := dispatchOneBotAction(session, &oneBotActionRequest{
		Action: action,
		Params: rawParams,
	})
	return c.Status(oneBotHTTPStatusCode(resp)).JSON(resp)
}

func oneBotHTTPSupportsMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case fiber.MethodGet, fiber.MethodPost:
		return true
	default:
		return false
	}
}

func newOneBotHTTPSession(botUser *model.UserModel) (*oneBotSession, error) {
	selfID, err := service.GetOrCreateOneBotID(service.OneBotEntityBotUser, botUser.ID)
	if err != nil {
		return nil, err
	}
	session := newOneBotSession(botUser, oneBotSessionRoleUniversal, oneBotSessionSourceHTTP, nil)
	session.SelfID = selfID
	return session, nil
}

func oneBotHTTPStatusCode(resp *oneBotActionResponse) int {
	if resp == nil {
		return fiber.StatusInternalServerError
	}
	if resp.Status == "ok" || resp.RetCode == 0 {
		return fiber.StatusOK
	}
	switch resp.RetCode {
	case 1400:
		return fiber.StatusBadRequest
	case 1403:
		return fiber.StatusForbidden
	case 1404:
		return fiber.StatusNotFound
	default:
		return fiber.StatusOK
	}
}

func matchOneBotHTTPActionForAnyBot(c *fiber.Ctx) (string, bool) {
	requestPath := normalizeOneBotRequestPath(c.Path())
	if action, ok := parseOneBotHTTPActionFromPath(requestPath, model.DefaultOneBotHTTPPathSuffix); ok {
		return action, true
	}

	token := resolveOneBotAccessToken(c.Get("Authorization"))
	if token == "" {
		token = resolveOneBotAccessToken(c.Query("access_token"))
	}
	if token != "" {
		if botUser, _, err := resolveOneBotBotFromToken(token); err == nil && botUser != nil {
			return matchOneBotHTTPActionForBot(requestPath, botUser.ID)
		}
	}

	configs, err := model.BotOneBotConfigListEnabled()
	if err != nil {
		return "", false
	}
	for _, cfg := range configs {
		if cfg == nil || cfg.TransportType != model.OneBotTransportHTTP {
			continue
		}
		if action, ok := parseOneBotHTTPActionFromPath(requestPath, cfg.HTTPPathSuffix); ok {
			return action, true
		}
	}
	return "", false
}

func matchOneBotHTTPActionForBot(requestPath string, botUserID string) (string, bool) {
	requestPath = normalizeOneBotRequestPath(requestPath)
	if action, ok := parseOneBotHTTPActionFromPath(requestPath, model.DefaultOneBotHTTPPathSuffix); ok {
		return action, true
	}
	if action, ok := parseOneBotHTTPRootAction(requestPath); ok {
		return action, true
	}
	cfg, err := model.BotOneBotConfigGet(botUserID)
	if err != nil || cfg == nil || !cfg.Enabled || cfg.TransportType != model.OneBotTransportHTTP {
		return "", false
	}
	return parseOneBotHTTPActionFromPath(requestPath, cfg.HTTPPathSuffix)
}

func parseOneBotHTTPActionFromPath(requestPath string, basePath string) (string, bool) {
	basePath = normalizeOneBotRequestPath(basePath)
	requestPath = normalizeOneBotRequestPath(requestPath)
	if basePath == "" || requestPath == "" {
		return "", false
	}
	if !strings.HasPrefix(requestPath, basePath+"/") {
		return "", false
	}
	action := strings.Trim(strings.TrimPrefix(requestPath, basePath), "/")
	if action == "" || strings.Contains(action, "/") {
		return "", false
	}
	return action, true
}

func parseOneBotHTTPRootAction(requestPath string) (string, bool) {
	requestPath = normalizeOneBotRequestPath(requestPath)
	if requestPath == "" || requestPath == "/" {
		return "", false
	}
	action := strings.Trim(requestPath, "/")
	if action == "" || strings.Contains(action, "/") || !isOneBotSupportedAction(action) {
		return "", false
	}
	return action, true
}

func normalizeOneBotRequestPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	trimmed = strings.TrimRight(trimmed, "/")
	if trimmed == "" {
		return "/"
	}
	return trimmed
}

func parseOneBotHTTPParams(c *fiber.Ctx) (map[string]any, error) {
	params := map[string]any{}
	c.Context().QueryArgs().VisitAll(func(key, value []byte) {
		params[string(key)] = normalizeOneBotHTTPScalar(string(value))
	})

	method := strings.ToUpper(strings.TrimSpace(c.Method()))
	if method != fiber.MethodPost {
		return params, nil
	}
	if len(c.Body()) == 0 {
		return params, nil
	}

	contentType := strings.ToLower(strings.TrimSpace(c.Get(fiber.HeaderContentType)))
	contentType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	switch contentType {
	case fiber.MIMEApplicationJSON:
		decoder := json.NewDecoder(bytes.NewReader(c.Body()))
		decoder.UseNumber()
		bodyParams := map[string]any{}
		if err := decoder.Decode(&bodyParams); err != nil {
			return nil, fiber.NewError(fiber.StatusBadRequest, "invalid json body")
		}
		for key, value := range bodyParams {
			params[key] = normalizeOneBotHTTPValue(value)
		}
		return params, nil
	case fiber.MIMEApplicationForm, fiber.MIMEMultipartForm:
		c.Context().PostArgs().VisitAll(func(key, value []byte) {
			params[string(key)] = normalizeOneBotHTTPScalar(string(value))
		})
		return params, nil
	default:
		return nil, fiber.NewError(fiber.StatusNotAcceptable, "unsupported content type")
	}
}

func normalizeOneBotHTTPValue(value any) any {
	switch v := value.(type) {
	case string:
		return normalizeOneBotHTTPScalar(v)
	case map[string]any:
		out := map[string]any{}
		for key, item := range v {
			out[key] = normalizeOneBotHTTPValue(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, normalizeOneBotHTTPValue(item))
		}
		return out
	default:
		return value
	}
}

func normalizeOneBotHTTPScalar(raw string) any {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	switch lower {
	case "true":
		return true
	case "false":
		return false
	}
	if (strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}")) || (strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]")) {
		var decoded any
		decoder := json.NewDecoder(strings.NewReader(value))
		decoder.UseNumber()
		if err := decoder.Decode(&decoded); err == nil {
			return normalizeOneBotHTTPValue(decoded)
		}
	}
	if i, err := strconv.ParseInt(value, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return f
	}
	return raw
}

func valueString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return ""
	}
}
