package api

import (
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/utils"
)

func AdminSQLiteVacuumStatus(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	cfg := utils.GetConfig()
	if cfg == nil {
		return wrapErrorStatus(c, http.StatusInternalServerError, nil, "配置未加载")
	}
	if !model.IsSQLite() {
		return wrapErrorStatus(c, http.StatusBadRequest, nil, "当前数据库不是 SQLite")
	}

	sizeBytes, sizeErr := model.SQLiteFileSizeBytes()
	sizeValue := any(nil)
	sizeErrMsg := ""
	if sizeErr == nil {
		sizeValue = sizeBytes
	} else {
		sizeErrMsg = sizeErr.Error()
	}

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message":     "ok",
		"autoEnabled": cfg.SQLite.AutoVacuumEnabled,
		"intervalHrs": cfg.SQLite.AutoVacuumIntervalHours,
		"dbSizeBytes": sizeValue,
		"dbSizeError": sizeErrMsg,
	})
}

func AdminSQLiteVacuumExecute(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	cfg := utils.GetConfig()
	if cfg == nil {
		return wrapErrorStatus(c, http.StatusInternalServerError, nil, "配置未加载")
	}
	if !model.IsSQLite() {
		return wrapErrorStatus(c, http.StatusBadRequest, nil, "当前数据库不是 SQLite")
	}

	beforeSize, beforeErr := model.SQLiteFileSizeBytes()
	if err := model.VacuumSQLite(); err != nil {
		return wrapErrorStatus(c, http.StatusInternalServerError, err, "执行 SQLite 空间整理失败")
	}

	afterSize, afterErr := model.SQLiteFileSizeBytes()
	beforeSizeValue := any(nil)
	beforeErrMsg := ""
	if beforeErr == nil {
		beforeSizeValue = beforeSize
	} else {
		beforeErrMsg = beforeErr.Error()
	}
	afterSizeValue := any(nil)
	afterErrMsg := ""
	if afterErr == nil {
		afterSizeValue = afterSize
	} else {
		afterErrMsg = afterErr.Error()
	}
	reclaimedBytes := any(nil)
	if beforeErr == nil && afterErr == nil {
		reclaimedBytes = beforeSize - afterSize
	}

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message":         "ok",
		"executedAt":      time.Now().UnixMilli(),
		"autoEnabled":     cfg.SQLite.AutoVacuumEnabled,
		"intervalHrs":     cfg.SQLite.AutoVacuumIntervalHours,
		"beforeSizeBytes": beforeSizeValue,
		"afterSizeBytes":  afterSizeValue,
		"reclaimedBytes":  reclaimedBytes,
		"beforeSizeError": beforeErrMsg,
		"afterSizeError":  afterErrMsg,
	})
}
