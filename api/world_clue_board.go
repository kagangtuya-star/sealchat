package api

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/protocol"
	"sealchat/service"
)

const worldClueBoardRequestLimit = service.WorldClueBoardMaxDocumentBytes

// BindWorldClueBoardRoutes registers the board contract separately
// from the clue CRUD routes. The path intentionally has no channel dimension.
func BindWorldClueBoardRoutes(group fiber.Router) {
	group.Get("/:worldId/clue-boards/:boardKey", worldClueBoardGetHandler)
	group.Put("/:worldId/clue-boards/:boardKey", worldClueBoardPutHandler)
	group.Post("/:worldId/clue-boards/:boardKey/ops", worldClueBoardOperationHandler)
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
	item, err := service.WorldClueBoardGet(c.Params("worldId"), c.Params("boardKey"), userID, c.Query("scope"))
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
	scope := ""
	if body.Scope != nil {
		scope = *body.Scope
	}
	item, err := service.WorldClueBoardPut(c.Params("worldId"), c.Params("boardKey"), userID, service.WorldClueBoardPutInput{
		Scope:            scope,
		ExpectedRevision: body.ExpectedRevision,
		Document:         body.Document,
	})
	if err != nil {
		return worldClueBoardError(c, err)
	}
	if item.Scope == protocol.WorldClueBoardScopeShared {
		broadcastWorldClueBoardChanged(protocol.WorldClueBoardEventPayload{WorldID: item.WorldID, BoardKey: item.BoardKey, Scope: item.Scope, Revision: item.Revision, UpdatedBy: userID})
	}
	return c.JSON(item)
}

func worldClueBoardOperationHandler(c *fiber.Ctx) error {
	userID, err := worldClueBoardUser(c)
	if err != nil {
		return err
	}
	if len(c.Body()) > worldClueBoardRequestLimit {
		return worldClueBoardError(c, service.ErrWorldClueBoardTooLarge)
	}
	var body service.WorldClueBoardOperationInput
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return worldClueBoardError(c, service.ErrWorldClueBoardInvalid)
	}
	result, err := service.WorldClueBoardApplyOperation(c.Params("worldId"), c.Params("boardKey"), userID, body)
	if err != nil {
		return worldClueBoardError(c, err)
	}
	if result.Changed {
		broadcastWorldClueBoardChanged(protocol.WorldClueBoardEventPayload{WorldID: result.WorldID, BoardKey: result.BoardKey, Scope: result.Scope, Revision: result.Revision, UpdatedBy: userID, ClientID: body.ClientID, Operation: &body.Operation}, result.RemovedRelation)
	}
	return c.JSON(result)
}

func broadcastWorldClueBoardChanged(board protocol.WorldClueBoardEventPayload, removed ...*protocol.WorldClueBoardRelation) {
	go func() {
		recipients, err := buildOnlineWorldMemberRecipients(board.WorldID, userId2ConnInfoGlobal)
		if err != nil {
			return
		}
		var removedRelation *protocol.WorldClueBoardRelation
		if len(removed) > 0 {
			removedRelation = removed[0]
		}
		isQuickdraw := board.Operation != nil && board.Operation.Type == protocol.WorldClueBoardQuickdrawDiff
		quickdrawRealtimeAllowed := false
		if isQuickdraw {
			raw, marshalErr := json.Marshal(board.Operation)
			quickdrawRealtimeAllowed = marshalErr == nil && len(raw) <= service.WorldClueBoardRealtimeEventMaxBytes
		}
		for _, userID := range recipients {
			filtered := board
			if isQuickdraw {
				if !quickdrawRealtimeAllowed {
					filtered.Operation = nil
				}
			} else {
				filtered.Operation, err = service.FilterWorldClueBoardOperationForActor(board.WorldID, userID, board.Operation, removedRelation)
				if err != nil {
					continue
				}
			}
			payload := struct {
				protocol.Event
				Op protocol.Opcode `json:"op"`
			}{Event: protocol.Event{Type: protocol.EventWorldClueBoardChanged, Timestamp: time.Now().Unix(), WorldClueBoard: &filtered}, Op: protocol.OpEvent}
			conns, ok := userId2ConnInfoGlobal.Load(userID)
			if !ok || conns == nil {
				continue
			}
			conns.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
				if info != nil && !info.IsGuest && !info.IsObserver && info.WorldId == board.WorldID {
					_ = conn.WriteJSON(payload)
				}
				return true
			})
		}
	}()
}
