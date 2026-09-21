package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/pm"
	"sealchat/service/perfprofiler"
)

func AdminPerfStatus(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	manager := perfprofiler.Get()
	if manager == nil {
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"message": "ok",
			"state": fiber.Map{
				"enabled": false,
				"status":  "uninitialized",
			},
		})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message": "ok",
		"state":   manager.CurrentState(),
	})
}

func AdminPerfHistory(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	manager := perfprofiler.Get()
	if manager == nil {
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"message": "ok",
			"points":  []perfprofiler.SamplePoint{},
		})
	}

	start, end, err := parsePerfRange(c)
	if err != nil {
		return wrapErrorStatus(c, http.StatusBadRequest, err, "性能检测时间范围无效")
	}
	points, err := manager.QueryHistory(start, end)
	if err != nil {
		return wrapErrorStatus(c, http.StatusInternalServerError, err, "读取性能检测历史失败")
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message": "ok",
		"points":  points,
	})
}

func AdminPerfArtifacts(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	manager := perfprofiler.Get()
	if manager == nil {
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"message": "ok",
			"items":   []perfprofiler.Artifact{},
		})
	}
	items, err := manager.ListArtifacts()
	if err != nil {
		return wrapErrorStatus(c, http.StatusInternalServerError, err, "读取性能检测文件列表失败")
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message": "ok",
		"items":   items,
	})
}

func AdminPerfTopFunctions(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	manager := perfprofiler.Get()
	if manager == nil {
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"message": "ok",
			"items":   []perfprofiler.TopFunction{},
		})
	}
	items, err := manager.TopFunctions(10)
	if err != nil {
		return wrapErrorStatus(c, http.StatusInternalServerError, err, "读取性能检测热点函数失败")
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message": "ok",
		"items":   items,
	})
}

func AdminPerfCPUSessionStart(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	manager := perfprofiler.Get()
	if manager == nil {
		return wrapErrorStatus(c, http.StatusServiceUnavailable, nil, "性能检测管理器未初始化")
	}
	var payload struct {
		DurationSec int `json:"durationSec"`
	}
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&payload); err != nil {
			return wrapErrorStatus(c, http.StatusBadRequest, err, "请求体解析失败")
		}
	}
	duration := time.Duration(payload.DurationSec) * time.Second
	session, err := manager.StartCPUSession(duration)
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case perfprofiler.ErrDisabled:
			status = http.StatusConflict
		case perfprofiler.ErrSessionActive:
			status = http.StatusConflict
		}
		return wrapErrorStatus(c, status, err, "启动连续 CPU 录制失败")
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message": "ok",
		"state":   session,
	})
}

func AdminPerfCPUSessionStop(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	manager := perfprofiler.Get()
	if manager == nil {
		return wrapErrorStatus(c, http.StatusServiceUnavailable, nil, "性能检测管理器未初始化")
	}
	if err := manager.StopCPUSession(); err != nil {
		return wrapErrorStatus(c, http.StatusInternalServerError, err, "停止连续 CPU 录制失败")
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message": "ok",
		"state":   manager.CurrentState().CPUSession,
	})
}

func AdminPerfMessagePipeline(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	windowSec, err := parsePerfWindowSec(c.Query("windowSec"))
	if err != nil {
		return wrapErrorStatus(c, http.StatusBadRequest, err, "消息链路时间窗口无效")
	}
	manager := perfprofiler.Get()
	if manager == nil {
		return c.Status(http.StatusOK).JSON(fiber.Map{"message": "ok", "summary": perfprofiler.MessagePipelineSummary{WindowSec: windowSec}})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message": "ok",
		"summary": manager.MessagePipelineSummary(time.Duration(windowSec) * time.Second),
	})
}

func AdminPerfMessagePipelineReset(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	manager := perfprofiler.Get()
	if manager == nil {
		return wrapErrorStatus(c, http.StatusServiceUnavailable, nil, "性能检测管理器未初始化")
	}
	manager.ResetMessagePipeline()
	return c.Status(http.StatusOK).JSON(fiber.Map{"message": "ok"})
}

func AdminPerfTraceSessionStart(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	manager := perfprofiler.Get()
	if manager == nil {
		return wrapErrorStatus(c, http.StatusServiceUnavailable, nil, "性能检测管理器未初始化")
	}
	var payload struct {
		DurationSec int `json:"durationSec"`
	}
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&payload); err != nil {
			return wrapErrorStatus(c, http.StatusBadRequest, err, "请求体解析失败")
		}
	}
	state, err := manager.StartTraceSession(time.Duration(payload.DurationSec) * time.Second)
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case perfprofiler.ErrDisabled, perfprofiler.ErrTraceSessionActive:
			status = http.StatusConflict
		case perfprofiler.ErrTraceDurationInvalid:
			status = http.StatusBadRequest
		}
		return wrapErrorStatus(c, status, err, "启动 Runtime Trace 失败")
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"message": "ok", "state": state})
}

func AdminPerfTraceSessionStop(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	manager := perfprofiler.Get()
	if manager == nil {
		return wrapErrorStatus(c, http.StatusServiceUnavailable, nil, "性能检测管理器未初始化")
	}
	if err := manager.StopTraceSession(); err != nil {
		return wrapErrorStatus(c, http.StatusInternalServerError, err, "停止 Runtime Trace 失败")
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"message": "ok", "state": manager.CurrentState().TraceSession})
}

func AdminPerfArtifactDownload(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	manager := perfprofiler.Get()
	if manager == nil {
		return wrapErrorStatus(c, http.StatusServiceUnavailable, nil, "性能检测管理器未初始化")
	}
	name := strings.TrimSpace(c.Params("name"))
	if name == "" || strings.ContainsAny(name, `/\`) {
		return wrapErrorStatus(c, http.StatusBadRequest, nil, "性能检测文件名无效")
	}
	item, err := manager.ResolveArtifact(name)
	if err != nil {
		return wrapErrorStatus(c, http.StatusNotFound, err, "性能检测文件不存在")
	}
	return c.SendFile(item.Path)
}

func parsePerfRange(c *fiber.Ctx) (int64, int64, error) {
	startRaw := strings.TrimSpace(c.Query("start"))
	endRaw := strings.TrimSpace(c.Query("end"))
	if startRaw != "" || endRaw != "" {
		start, err := strconv.ParseInt(startRaw, 10, 64)
		if err != nil {
			return 0, 0, err
		}
		end, err := strconv.ParseInt(endRaw, 10, 64)
		if err != nil {
			return 0, 0, err
		}
		return start, end, nil
	}

	rangeParam := strings.ToLower(strings.TrimSpace(c.Query("range", "1h")))
	now := time.Now().UnixMilli()
	switch rangeParam {
	case "15m":
		return now - int64((15*time.Minute)/time.Millisecond), now, nil
	case "1h":
		return now - int64(time.Hour/time.Millisecond), now, nil
	case "6h":
		return now - int64((6*time.Hour)/time.Millisecond), now, nil
	case "24h":
		return now - int64((24*time.Hour)/time.Millisecond), now, nil
	case "7d":
		return now - int64((7*24*time.Hour)/time.Millisecond), now, nil
	default:
		return 0, 0, strconv.ErrSyntax
	}
}

func parsePerfWindowSec(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 60, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 10 || value > 600 {
		return 0, strconv.ErrSyntax
	}
	return value, nil
}
