package api

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/utils"
)

func captchaSceneFromCtx(c *fiber.Ctx) utils.CaptchaScene {
	scene := strings.TrimSpace(strings.ToLower(c.Query("scene")))
	if scene == string(utils.CaptchaSceneSignin) {
		return utils.CaptchaSceneSignin
	}
	return utils.CaptchaSceneSignup
}

// CaptchaNew 生成新的验证码
// GET /api/v1/captcha/new
func CaptchaNew(c *fiber.Ctx) error {
	scene := captchaSceneFromCtx(c)
	conf := appConfig.Captcha.Target(scene)
	if conf.Mode != utils.CaptchaModeLocal {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"message": "当前场景未启用本地验证码",
		})
	}

	captchaId := model.CaptchaCreate()
	return c.JSON(fiber.Map{
		"id": captchaId,
	})
}

// CaptchaImage 获取验证码图片
// GET /api/v1/captcha/:id.png
func CaptchaImage(c *fiber.Ctx) error {
	if !appConfig.Captcha.HasLocalEnabled() {
		return c.Status(http.StatusNotFound).SendString("本地验证码未启用")
	}

	captchaId := c.Params("id")
	if captchaId == "" {
		return c.Status(http.StatusBadRequest).SendString("缺少验证码ID")
	}

	// 生成验证码图片，使用默认尺寸
	imageData, err := model.CaptchaImage(captchaId, 0, 0)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString("生成验证码图片失败")
	}

	c.Set("Content-Type", "image/png")
	c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")

	return c.Send(imageData)
}

// CaptchaReload 刷新验证码
// GET /api/v1/captcha/:id/reload
func CaptchaReload(c *fiber.Ctx) error {
	scene := captchaSceneFromCtx(c)
	conf := appConfig.Captcha.Target(scene)
	if conf.Mode != utils.CaptchaModeLocal {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"message": "当前场景未启用本地验证码",
		})
	}

	captchaId := c.Params("id")
	if captchaId == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "缺少验证码ID",
		})
	}

	success := model.CaptchaReload(captchaId)
	if !success {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"message": "验证码不存在或已过期",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
	})
}
