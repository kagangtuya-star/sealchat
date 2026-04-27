package api

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/utils"
)

type captchaCapRedeemRequest struct {
	Token     string  `json:"token"`
	Solutions []int64 `json:"solutions"`
}

func captchaSceneFromParam(raw string) (utils.CaptchaScene, bool) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case string(utils.CaptchaSceneSignup):
		return utils.CaptchaSceneSignup, true
	case string(utils.CaptchaSceneSignin):
		return utils.CaptchaSceneSignin, true
	case strings.ToLower(string(utils.CaptchaScenePasswordReset)), "password_reset", "password-reset":
		return utils.CaptchaScenePasswordReset, true
	default:
		return "", false
	}
}

func CaptchaCapChallenge(c *fiber.Ctx) error {
	scene, ok := captchaSceneFromParam(c.Params("scene"))
	if !ok {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "无效的验证码场景",
		})
	}

	conf := appConfig.Captcha.Target(scene)
	if conf.Mode != utils.CaptchaModeCap {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"message": "当前场景未启用 Cap 验证码",
		})
	}

	resp, err := model.CaptchaCapCreateChallenge(scene, conf.Cap)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"message": "生成 Cap 验证码失败",
		})
	}
	return c.JSON(resp)
}

func CaptchaCapRedeem(c *fiber.Ctx) error {
	scene, ok := captchaSceneFromParam(c.Params("scene"))
	if !ok {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "无效的验证码场景",
		})
	}

	conf := appConfig.Captcha.Target(scene)
	if conf.Mode != utils.CaptchaModeCap {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"message": "当前场景未启用 Cap 验证码",
		})
	}

	var req captchaCapRedeemRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "请求参数错误",
		})
	}

	resp, err := model.CaptchaCapRedeemChallenge(scene, req.Token, req.Solutions)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"message": "兑换 Cap 验证码失败",
		})
	}
	if !resp.Success {
		return c.Status(http.StatusBadRequest).JSON(resp)
	}
	return c.JSON(resp)
}
