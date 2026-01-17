package api

import (
	"github.com/gofiber/fiber/v2"

	"sealchat/pm"
	"sealchat/utils"
)

// AudioWorkbenchMiddleware 音频工作台权限中间件
// 允许平台管理员或（启用配置后）世界管理员访问音频工作台
func AudioWorkbenchMiddleware(c *fiber.Ctx) error {
	// 平台管理员始终允许
	if pm.CanWithSystemRole(getCurUser(c).ID, pm.PermModAdmin) {
		c.Locals("audioIsSystemAdmin", true)
		return c.Next()
	}
	// 检查配置开关
	if !utils.GetConfig().Audio.AllowWorldAudioWorkbench {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"message": "音频工作台仅管理员可用",
		})
	}
	// 世界管理员权限校验在各 handler 中进行
	c.Locals("audioIsSystemAdmin", false)
	return c.Next()
}
