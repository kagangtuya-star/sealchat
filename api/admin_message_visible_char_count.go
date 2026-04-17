package api

import (
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
)

func AdminMessageVisibleCharCountStatus(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}

	state, err := model.MessageVisibleCharCountBackfillStateGet()
	if err != nil {
		return wrapErrorStatus(c, http.StatusInternalServerError, err, "获取输入统计字数修复状态失败")
	}

	if state == nil {
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"message": "ok",
			"state": fiber.Map{
				"status":          "idle",
				"mode":            "backfill_missing",
				"phase":           "sentinel",
				"processed_count": 0,
			},
		})
	}

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message": "ok",
		"state":   state,
	})
}

func AdminMessageVisibleCharCountRebuild(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}

	if err := model.RebuildMessageVisibleCharCount(); err != nil {
		if errors.Is(err, model.ErrMessageVisibleCharCountRebuildRunning) {
			return wrapErrorStatus(c, http.StatusConflict, err, "已有输入统计字数修复任务正在运行")
		}
		return wrapErrorStatus(c, http.StatusInternalServerError, err, "执行消息可见字数重算失败")
	}

	state, err := model.MessageVisibleCharCountBackfillStateGet()
	if err != nil {
		return wrapErrorStatus(c, http.StatusInternalServerError, err, "读取输入统计字数修复结果失败")
	}

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message": "ok",
		"state":   state,
	})
}
