package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/utils"
)

const defaultAuthCookieMaxAgeDays = 15
const defaultSlidingRefreshThresholdDays = 7

func resolveAuthCookieMaxAgeDays() int {
	maxAgeDays := utils.ResolveAuthSessionMaxAgeDays()
	if maxAgeDays > 0 {
		return maxAgeDays
	}
	return defaultAuthCookieMaxAgeDays
}

func resolveSlidingRefreshThresholdDays() int {
	refreshThresholdDays := utils.ResolveAuthSessionRefreshThresholdDays()
	if refreshThresholdDays > 0 {
		return refreshThresholdDays
	}
	return defaultSlidingRefreshThresholdDays
}

func resolveDefaultAuthCookieMaxAge() int {
	return int((time.Duration(resolveAuthCookieMaxAgeDays()*24) * time.Hour) / time.Second)
}

func resolveAuthCookieMaxAge(token string) int {
	token = strings.TrimSpace(token)
	if token == "" {
		return resolveDefaultAuthCookieMaxAge()
	}

	// 机器人 token 不走 TokenCheck 签名格式，使用默认长期策略
	if len(token) == 32 {
		return resolveDefaultAuthCookieMaxAge()
	}

	ret := model.TokenCheck(token)
	if !ret.HashValid || !ret.TimeValid {
		return resolveDefaultAuthCookieMaxAge()
	}

	maxAge := int(ret.ExpireOffset * 60)
	if maxAge <= 0 {
		return resolveDefaultAuthCookieMaxAge()
	}

	return maxAge
}

func shouldRefreshUserToken(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}

	ret := model.TokenCheck(token)
	if !ret.HashValid || !ret.TimeValid {
		return false
	}

	remaining := time.Duration(ret.ExpireOffset) * time.Minute
	threshold := time.Duration(resolveSlidingRefreshThresholdDays()*24) * time.Hour
	return remaining <= threshold
}

func getToken(c *fiber.Ctx) string {
	token := strings.TrimSpace(c.Get("Authorization"))
	if token == "" {
		tokens := c.GetReqHeaders()["Authorization"]
		if len(tokens) > 0 {
			token = tokens[0]
		}
		token = strings.TrimSpace(token)
	}
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[len("bearer "):])
	}

	cookieToken := c.Cookies("Authorization")
	isWriteCookie := token != "" && cookieToken != token

	if token == "" {
		token = cookieToken
	}

	if isWriteCookie {
		maxAge := resolveAuthCookieMaxAge(token)
		c.Cookie(&fiber.Cookie{
			Name:     "Authorization",
			Value:    token,
			Path:     "/",
			SameSite: "Lax",
			Secure:   c.Protocol() == "https",
			MaxAge:   maxAge,
		})
	}

	return token
}

func SignCheckMiddleware(c *fiber.Ctx) error {
	token := getToken(c)

	var user *model.UserModel
	var err error

	if len(token) == 32 {
		user, err = model.BotVerifyAccessToken(token)
		if err != nil {
			return c.Status(http.StatusUnauthorized).JSON(
				fiber.Map{"message": err.Error()},
			)
		}
	} else {
		user, err = model.UserVerifyAccessToken(token)
		if err != nil {
			return c.Status(http.StatusUnauthorized).JSON(
				fiber.Map{"message": "凭证错误，需要重新登录"},
			)
		}

		if user.AccessToken != nil && user.AccessToken.ID != "" && shouldRefreshUserToken(token) {
			if refreshed, refreshErr := model.UserRefreshAccessToken(user.AccessToken.ID); refreshErr == nil {
				token = refreshed
				maxAge := resolveAuthCookieMaxAge(token)
				c.Cookie(&fiber.Cookie{
					Name:     "Authorization",
					Value:    token,
					Path:     "/",
					SameSite: "Lax",
					Secure:   c.Protocol() == "https",
					MaxAge:   maxAge,
				})
				c.Set("X-Access-Token-Refresh", token)
			}
		}
	}

	if user.Disabled {
		return c.Status(http.StatusUnauthorized).JSON(
			fiber.Map{"message": "帐号被禁用"},
		)
	}

	c.Locals("user", user)
	model.TimelineUpdate(user.ID)
	return c.Next()
}

func UserRoleAdminMiddleware(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	return c.Next()
}
