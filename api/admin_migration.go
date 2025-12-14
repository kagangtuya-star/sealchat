package api

import (
	"net/http"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

// ImageMigrationPreview returns statistics about images pending migration to WebP
func ImageMigrationPreview(c *fiber.Ctx) error {
	stats, err := service.GetMigrationPreview()
	if err != nil {
		return wrapErrorStatus(c, http.StatusInternalServerError, err, "获取迁移预览失败")
	}
	return c.JSON(fiber.Map{
		"stats": stats,
	})
}

// ImageMigrationExecuteRequest represents the request body for migration execution
type ImageMigrationExecuteRequest struct {
	BatchSize int  `json:"batchSize"`
	DryRun    bool `json:"dryRun"`
}

// ImageMigrationExecute performs the image migration to WebP format
func ImageMigrationExecute(c *fiber.Ctx) error {
	var req ImageMigrationExecuteRequest
	if err := c.BodyParser(&req); err != nil {
		// Use defaults if parsing fails
		req.BatchSize = 100
		req.DryRun = false
	}

	if req.BatchSize <= 0 {
		req.BatchSize = 100
	}

	if req.BatchSize > 1000 {
		req.BatchSize = 1000 // Cap at 1000 per batch for safety
	}

	stats, results, err := service.MigrateImages(req.BatchSize, req.DryRun)
	if err != nil {
		return wrapErrorStatus(c, http.StatusInternalServerError, err, "执行迁移失败")
	}

	return c.JSON(fiber.Map{
		"stats":   stats,
		"results": results,
		"dryRun":  req.DryRun,
	})
}
