package api

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
)

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
		c.Cookie(&fiber.Cookie{
			Name:   "Authorization",
			Value:  token,
			MaxAge: 3600, // 存活时间为3600秒
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
