package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

func S3MigrationPreview(c *fiber.Ctx) error {
	kind := service.S3MigrationKind(strings.TrimSpace(c.Query("type")))
	target := service.StorageMigrationTarget(strings.TrimSpace(c.Query("target", string(service.StorageMigrationTargetS3))))
	stats, err := service.GetStorageMigrationPreview(kind, target)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrS3MigrationBadRequest) {
			status = http.StatusBadRequest
		}
		return wrapErrorStatus(c, status, err, "获取迁移预览失败")
	}
	return c.JSON(fiber.Map{"stats": stats})
}

type S3MigrationExecuteRequest struct {
	Type         string `json:"type"`
	Target       string `json:"target"`
	BatchSize    int    `json:"batchSize"`
	DryRun       bool   `json:"dryRun"`
	DeleteSource bool   `json:"deleteSource"`
}

func S3MigrationExecute(c *fiber.Ctx) error {
	var req S3MigrationExecuteRequest
	if err := c.BodyParser(&req); err != nil {
		req.BatchSize = 100
		req.DryRun = false
	}
	kind := service.S3MigrationKind(strings.TrimSpace(req.Type))
	target := service.StorageMigrationTarget(strings.TrimSpace(req.Target))
	stats, results, err := service.ExecuteStorageMigration(kind, target, req.BatchSize, req.DryRun, req.DeleteSource)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrS3MigrationBadRequest) || errors.Is(err, service.ErrS3MigrationS3NotReady) {
			status = http.StatusBadRequest
		}
		return wrapErrorStatus(c, status, err, "执行迁移失败")
	}
	return c.JSON(fiber.Map{
		"stats":   stats,
		"results": results,
		"dryRun":  req.DryRun,
	})
}
