package api

import (
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

func TheaterAudioAssetList(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	result, err := service.ListTheaterAudioAssets(user.ID, c.Params("worldId"), c.Params("channelId"))
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	return c.JSON(fiber.Map{
		"ok":        true,
		"requestId": requestID,
		"items":     result.Items,
		"quota":     result.Quota,
	})
}

func TheaterAudioAssetUpload(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	file, err := c.FormFile("file")
	if err != nil {
		return theaterErrorResponse(c, requestID, service.NewTheaterPayloadErrorForAPI("file 必填"))
	}
	asset, err := service.CreateTheaterAudioAsset(user.ID, c.Params("worldId"), c.Params("channelId"), file, c.FormValue("name"))
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	status := "success"
	if asset.TranscodeStatus == "pending" {
		status = "processing"
	} else if asset.TranscodeStatus == "failed" {
		status = "failed"
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"ok":        true,
		"requestId": requestID,
		"item":      asset,
		"status":    status,
	})
}

func TheaterAudioAssetDelete(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	err := service.DeleteTheaterAudioAsset(user.ID, c.Params("worldId"), c.Params("channelId"), c.Params("assetId"))
	if err != nil {
		var referenced *service.AudioAssetReferencedError
		if errors.As(err, &referenced) {
			return theaterErrorResponse(c, requestID, &service.TheaterError{
				Code:       service.TheaterErrorResourceInUse,
				Message:    "音频素材仍被音频场景或播放状态引用",
				HTTPStatus: fiber.StatusConflict,
			})
		}
		return theaterErrorResponse(c, requestID, err)
	}
	return c.Status(fiber.StatusNoContent).Send(nil)
}
