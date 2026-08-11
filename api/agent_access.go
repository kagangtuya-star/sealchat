package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

type worldAgentAccessUpdateRequest struct {
	Enabled *bool `json:"enabled"`
	Rotate  bool  `json:"rotate"`
}

// SetAgentCrawlGuide supplies the bundled Markdown guide before the HTTP app starts.
func SetAgentCrawlGuide(markdown string) {
	agentCrawlGuideMarkdown = markdown
}

func AgentCrawlGuideHandler(c *fiber.Ctx) error {
	if strings.TrimSpace(agentCrawlGuideMarkdown) == "" {
		return c.Status(http.StatusInternalServerError).SendString("Agent 爬取指南未随服务打包。")
	}
	c.Set(fiber.HeaderCacheControl, "public, max-age=300")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("X-Robots-Tag", "noindex, nofollow")
	c.Set("Referrer-Policy", "no-referrer")
	c.Set(fiber.HeaderContentType, "text/markdown; charset=utf-8")
	return c.Status(http.StatusOK).SendString(agentCrawlGuideMarkdown)
}

func AgentAccessHandler(c *fiber.Ctx) error {
	setAgentAccessResponseHeaders(c)
	token := strings.TrimSpace(c.Params("token"))
	world, err := service.ResolveWorldAgentAccess(token)
	if err != nil {
		if errors.Is(err, service.ErrWorldAgentAccessInvalid) {
			return sendAgentAccessError(c, http.StatusNotFound, "agent_link_not_found", "AI Agent 访问链接无效、已关闭或已轮换。")
		}
		return sendAgentAccessError(c, http.StatusInternalServerError, "agent_link_resolve_failed", "解析 AI Agent 访问链接失败。")
	}

	remaining, retryAfter, allowed := service.ConsumeWorldAgentAccessRateLimit(world, time.Now().UTC())
	c.Set("X-RateLimit-Limit", strconv.Itoa(service.WorldAgentRateLimitPerMinute))
	c.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
	if !allowed {
		c.Set("Retry-After", strconv.Itoa(retryAfter))
		return sendAgentAccessError(c, http.StatusTooManyRequests, "rate_limited", "访问频率超过链接限制，请稍后重试。")
	}

	values := url.Values{}
	c.Context().QueryArgs().VisitAll(func(key, value []byte) {
		values.Add(string(key), string(value))
	})
	req, err := service.ParseAgentFeedRequest(values)
	if err != nil {
		return sendAgentAccessError(c, http.StatusBadRequest, "invalid_query", err.Error())
	}
	req.BasePath = c.Path()

	switch req.Resource {
	case "docs":
		return AgentCrawlGuideHandler(c)
	case "manifest":
		manifest, buildErr := service.BuildAgentFeedManifest(world, c.Path())
		if buildErr != nil {
			return sendAgentAccessError(c, http.StatusInternalServerError, "manifest_failed", "生成 Agent 访问文档失败。")
		}
		return sendAgentAccessJSON(c, http.StatusOK, manifest)
	case "schema":
		return sendAgentAccessJSON(c, http.StatusOK, service.BuildAgentFeedSchema(c.Path()))
	case "messages":
		response, queryErr := service.QueryAgentFeedMessages(world, req)
		if queryErr != nil {
			return handleAgentFeedQueryError(c, queryErr)
		}
		body, contentType, renderErr := service.RenderAgentFeedMessages(response, req.Format)
		if renderErr != nil {
			return sendAgentAccessError(c, http.StatusInternalServerError, "render_failed", "渲染消息响应失败。")
		}
		c.Set(fiber.HeaderContentType, contentType)
		return c.Status(http.StatusOK).Send(body)
	case "counts":
		response, queryErr := service.QueryAgentFeedCounts(world, req)
		if queryErr != nil {
			return handleAgentFeedQueryError(c, queryErr)
		}
		body, contentType, renderErr := service.RenderAgentFeedCounts(response, req.Format)
		if renderErr != nil {
			return sendAgentAccessError(c, http.StatusInternalServerError, "render_failed", "渲染计数响应失败。")
		}
		c.Set(fiber.HeaderContentType, contentType)
		return c.Status(http.StatusOK).Send(body)
	default:
		return sendAgentAccessError(c, http.StatusBadRequest, "invalid_resource", "不支持的 resource。")
	}
}

func WorldAgentAccessGetHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	state, err := service.WorldAgentAccessGet(strings.TrimSpace(c.Params("worldId")), user.ID)
	if err != nil {
		return handleWorldAgentAccessManagementError(c, err)
	}
	return c.JSON(buildWorldAgentAccessManagementState(state))
}

func WorldAgentAccessUpdateHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := strings.TrimSpace(c.Params("worldId"))
	var body worldAgentAccessUpdateRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请求参数错误"})
	}
	enabled := false
	if body.Enabled == nil {
		current, err := service.WorldAgentAccessGet(worldID, user.ID)
		if err != nil {
			return handleWorldAgentAccessManagementError(c, err)
		}
		enabled = current.Enabled
	} else {
		enabled = *body.Enabled
	}
	state, err := service.WorldAgentAccessUpdate(worldID, user.ID, enabled, body.Rotate)
	if err != nil {
		return handleWorldAgentAccessManagementError(c, err)
	}
	message := "AI Agent 访问链接已保存"
	if body.Rotate {
		message = "AI Agent 访问令牌已轮换，旧链接立即失效"
	}
	return c.JSON(fiber.Map{
		"message":     message,
		"agentAccess": buildWorldAgentAccessManagementState(state),
	})
}

func handleWorldAgentAccessManagementError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrWorldAgentAccessDenied):
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "仅世界拥有者或管理员可管理 AI Agent 访问链接"})
	case errors.Is(err, service.ErrWorldAgentAccessInvalid), errors.Is(err, service.ErrWorldNotFound):
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"message": "世界不存在或不可用"})
	case errors.Is(err, service.ErrWorldAgentTokenConflict):
		return c.Status(http.StatusConflict).JSON(fiber.Map{"message": "生成访问令牌时发生冲突，请重试"})
	default:
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "管理 AI Agent 访问链接失败"})
	}
}

func buildWorldAgentAccessManagementState(state *service.WorldAgentAccessState) fiber.Map {
	if state == nil {
		return fiber.Map{
			"worldId":  "",
			"publicId": "",
			"hasToken": false,
			"enabled":  false,
		}
	}
	return fiber.Map{
		"worldId":      state.WorldID,
		"publicId":     state.PublicID,
		"hasToken":     state.HasToken,
		"token":        state.Token,
		"tokenTail":    state.TokenTail,
		"enabled":      state.Enabled,
		"rotatedAt":    state.RotatedAt,
		"lastAccessAt": state.LastAccessAt,
	}
}

func handleAgentFeedQueryError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrAgentFeedBadRequest),
		errors.Is(err, service.ErrAgentFeedUnsupportedValue),
		errors.Is(err, service.ErrAgentFeedCursorChannel):
		return sendAgentAccessError(c, http.StatusBadRequest, "invalid_query", err.Error())
	case errors.Is(err, service.ErrAgentFeedChannelNotFound):
		return sendAgentAccessError(c, http.StatusNotFound, "channel_not_found", err.Error())
	case errors.Is(err, service.ErrAgentFeedTooManyChannels):
		return sendAgentAccessError(c, http.StatusRequestEntityTooLarge, "too_many_channels", err.Error())
	default:
		return sendAgentAccessError(c, http.StatusInternalServerError, "query_failed", "读取 Agent 数据失败。")
	}
}

func setAgentAccessResponseHeaders(c *fiber.Ctx) {
	c.Set(fiber.HeaderCacheControl, "private, no-store")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("X-Robots-Tag", "noindex, nofollow")
	c.Set("Referrer-Policy", "no-referrer")
}

func sendAgentAccessError(c *fiber.Ctx, status int, code, message string) error {
	setAgentAccessResponseHeaders(c)
	return sendAgentAccessJSON(c, status, fiber.Map{
		"schema_version": service.AgentFeedSchemaVersion,
		"encoding":       "utf-8",
		"error": fiber.Map{
			"code":    strings.TrimSpace(code),
			"message": strings.TrimSpace(message),
		},
	})
}

func sendAgentAccessJSON(c *fiber.Ctx, status int, value any) error {
	var buf strings.Builder
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	return c.Status(status).Send([]byte(strings.TrimSuffix(buf.String(), "\n")))
}
