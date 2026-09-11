package api

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"

	"sealchat/protocol"
	"sealchat/service"
)

const worldClueBoardRequestLimit = service.WorldClueBoardMaxDocumentBytes

// BindWorldClueBoardRoutes registers the personal board contract separately
// from the clue CRUD routes. The path intentionally has no channel dimension.
func BindWorldClueBoardRoutes(group fiber.Router) {
	group.Get("/:worldId/clue-boards/:boardKey", worldClueBoardGetHandler)
	group.Put("/:worldId/clue-boards/:boardKey", worldClueBoardPutHandler)
}

func worldClueBoardError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrWorldClueBoardTooLarge):
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{"message": "画板文档超过 8MiB 限制"})
	case errors.Is(err, service.ErrWorldClueBoardConflict):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"message": "画板已被其他窗口更新"})
	case errors.Is(err, service.ErrWorldClueBoardDenied), errors.Is(err, service.ErrWorldArchiveForbidden):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "无权访问该世界的线索板"})
	case errors.Is(err, service.ErrWorldNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "世界不存在或已失效"})
	case errors.Is(err, service.ErrWorldClueBoardInvalid), errors.Is(err, service.ErrWorldClueBoardUnsupported):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "线索板操作失败"})
	}
}

func worldClueBoardUser(c *fiber.Ctx) (string, error) {
	user := getCurUser(c)
	if user == nil {
		return "", c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	return user.ID, nil
}

func worldClueBoardGetHandler(c *fiber.Ctx) error {
	userID, err := worldClueBoardUser(c)
	if err != nil {
		return err
	}
	item, err := service.WorldClueBoardGet(c.Params("worldId"), c.Params("boardKey"), userID)
	if err != nil {
		return worldClueBoardError(c, err)
	}
	return c.JSON(item)
}

type worldClueBoardPutBody struct {
	ExpectedRevision *int64          `json:"expectedRevision"`
	Scope            *string         `json:"scope"`
	Document         json.RawMessage `json:"document"`
}

func worldClueBoardPutHandler(c *fiber.Ctx) error {
	userID, err := worldClueBoardUser(c)
	if err != nil {
		return err
	}
	if len(c.Body()) > worldClueBoardRequestLimit {
		return worldClueBoardError(c, service.ErrWorldClueBoardTooLarge)
	}
	var body worldClueBoardPutBody
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求格式错误"})
	}
	if body.Scope != nil && *body.Scope != protocol.WorldClueBoardScopePersonal {
		return worldClueBoardError(c, fmt.Errorf("%w: only personal scope is supported", service.ErrWorldClueBoardInvalid))
	}
	item, err := service.WorldClueBoardPut(c.Params("worldId"), c.Params("boardKey"), userID, service.WorldClueBoardPutInput{
		ExpectedRevision: body.ExpectedRevision,
		Document:         body.Document,
	})
	if err != nil {
		return worldClueBoardError(c, err)
	}
	return c.JSON(item)
}
