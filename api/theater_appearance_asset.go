package api

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

func TheaterAppearanceAssetUpload(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	channelID := strings.TrimSpace(c.Params("channelId"))
	actor, err := resolveChannelIdentityActorFromRequest(c, channelID, strings.TrimSpace(c.FormValue("targetUserId")))
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return theaterErrorResponse(c, requestID, service.NewTheaterPayloadErrorForAPI("file 必填"))
	}
	file, err := fileHeader.Open()
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	defer file.Close()
	asset, err := service.CreateTheaterAppearanceAssetUpload(c.Context(), actor.OperatorUserID, channelID, service.TheaterAppearanceAssetUploadInput{
		Reader: file, Filename: fileHeader.Filename, Size: fileHeader.Size,
		Purpose: c.FormValue("purpose"), IdentityID: c.FormValue("identityId"), VariantID: c.FormValue("variantId"),
		TargetUserID: actor.TargetUserID,
	})
	if err != nil {
		return theaterAppearanceAssetErrorResponse(c, requestID, err)
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"ok": true, "requestId": requestID, "asset": asset})
}

func TheaterAppearanceAssetImport(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	if user == nil {
		return theaterErrorResponse(c, requestID, &service.TheaterError{Code: service.TheaterErrorAuthRequired, Message: "未登录", HTTPStatus: fiber.StatusUnauthorized})
	}
	var payload struct {
		AttachmentID string `json:"attachmentId"`
		Purpose      string `json:"purpose"`
		IdentityID   string `json:"identityId"`
		TargetUserID string `json:"targetUserId"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return theaterErrorResponse(c, requestID, service.NewTheaterPayloadErrorForAPI("请求参数解析失败"))
	}
	asset, err := service.CreateTheaterAppearanceAssetFromAttachment(c.Context(), user.ID, c.Params("channelId"), service.TheaterAppearanceAssetAttachmentInput{
		AttachmentID: payload.AttachmentID, Purpose: payload.Purpose, IdentityID: payload.IdentityID, TargetUserID: payload.TargetUserID,
	})
	if err != nil {
		return theaterAppearanceAssetErrorResponse(c, requestID, err)
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"ok": true, "requestId": requestID, "asset": asset})
}

func TheaterAppearanceAssetGet(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	if user == nil {
		return theaterErrorResponse(c, requestID, &service.TheaterError{Code: service.TheaterErrorAuthRequired, Message: "未登录", HTTPStatus: fiber.StatusUnauthorized})
	}
	asset, err := service.GetTheaterAppearanceAsset(c.Context(), user.ID, c.Params("channelId"), c.Params("assetId"))
	if err != nil {
		return theaterAppearanceAssetErrorResponse(c, requestID, err)
	}
	return c.JSON(fiber.Map{"ok": true, "requestId": requestID, "asset": asset})
}

func TheaterAppearanceAssetDelete(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	if user == nil {
		return theaterErrorResponse(c, requestID, &service.TheaterError{Code: service.TheaterErrorAuthRequired, Message: "未登录", HTTPStatus: fiber.StatusUnauthorized})
	}
	if err := service.DeleteTheaterAppearanceAsset(c.Context(), user.ID, c.Params("channelId"), c.Params("assetId")); err != nil {
		return theaterAppearanceAssetErrorResponse(c, requestID, err)
	}
	return c.Status(fiber.StatusNoContent).Send(nil)
}

func theaterAppearanceAssetErrorResponse(c *fiber.Ctx, requestID string, err error) error {
	var theaterErr *service.TheaterError
	if errors.As(err, &theaterErr) {
		return theaterErrorResponse(c, requestID, err)
	}
	return handleChannelIdentityActorErr(c, err)
}
