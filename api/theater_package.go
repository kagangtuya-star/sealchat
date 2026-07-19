package api

import (
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/service"
)

func TheaterPackageExportCreate(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	var body struct {
		InputChannelID string `json:"inputChannelId"`
	}
	if len(c.Body()) > 0 {
		if err := decodeTheaterBody(c, &body, 64<<10); err != nil {
			return theaterErrorResponse(c, requestID, err)
		}
	}
	if body.InputChannelID == "" {
		body.InputChannelID = c.Params("channelId")
	}
	job, err := service.CreateTheaterPackageExportJob(user.ID, c.Params("worldId"), body.InputChannelID)
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"ok": true, "requestId": requestID, "job": theaterPackageJobView(job)})
}

func TheaterPackageImportCreate(c *fiber.Ctx) error {
	return createTheaterPackageImport(c, service.CreateTheaterPackageImportJob)
}

func TheaterPackageCCFOLIAImportCreate(c *fiber.Ctx) error {
	return createTheaterPackageImport(c, service.CreateTheaterCCFOLIAImportJob)
}

func createTheaterPackageImport(c *fiber.Ctx, create func(string, string, string, string, io.Reader, int64) (*model.TheaterPackageJobModel, error)) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	file, err := c.FormFile("file")
	if err != nil || file == nil {
		return theaterErrorResponse(c, requestID, service.NewTheaterPayloadErrorForAPI("file 必填"))
	}
	input, err := file.Open()
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	defer input.Close()
	inputChannelID := strings.TrimSpace(c.FormValue("inputChannelId"))
	if inputChannelID == "" {
		inputChannelID = c.Params("channelId")
	}
	job, err := create(user.ID, c.Params("worldId"), inputChannelID, file.Filename, input, file.Size)
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"ok": true, "requestId": requestID, "job": theaterPackageJobView(job)})
}

func TheaterPackageJobGet(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	job, err := service.GetTheaterPackageJob(user.ID, c.Params("jobId"))
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	if !theaterPackageJobMatchesWorld(job, c.Params("worldId")) {
		return theaterErrorResponse(c, requestID, &service.TheaterError{Code: service.TheaterErrorNotFound, Message: "舞台包任务不存在", HTTPStatus: fiber.StatusNotFound})
	}
	return c.JSON(fiber.Map{"ok": true, "requestId": requestID, "job": theaterPackageJobView(job)})
}

func TheaterPackageDownload(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	job, err := service.GetTheaterPackageJob(user.ID, c.Params("jobId"))
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	if !theaterPackageJobMatchesWorld(job, c.Params("worldId")) || job.Type != model.TheaterPackageJobTypeExport || job.Status != model.TheaterPackageJobStatusDone || strings.TrimSpace(job.OutputFilePath) == "" {
		return theaterErrorResponse(c, requestID, &service.TheaterError{Code: service.TheaterErrorNotFound, Message: "导出文件不存在", HTTPStatus: fiber.StatusNotFound})
	}
	if _, err := os.Stat(job.OutputFilePath); err != nil {
		return theaterErrorResponse(c, requestID, &service.TheaterError{Code: service.TheaterErrorNotFound, Message: "导出文件已过期", HTTPStatus: fiber.StatusNotFound})
	}
	c.Set(fiber.HeaderContentType, "application/zip")
	return c.Download(job.OutputFilePath, job.OutputFileName)
}

func TheaterPackageJobDelete(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	job, err := service.GetTheaterPackageJob(user.ID, c.Params("jobId"))
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	if !theaterPackageJobMatchesWorld(job, c.Params("worldId")) {
		return theaterErrorResponse(c, requestID, &service.TheaterError{Code: service.TheaterErrorNotFound, Message: "舞台包任务不存在", HTTPStatus: fiber.StatusNotFound})
	}
	if err := service.DeleteTheaterPackageJob(user.ID, job.ID); err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	return c.JSON(fiber.Map{"ok": true, "requestId": requestID})
}

func theaterPackageJobMatchesWorld(job *model.TheaterPackageJobModel, worldID string) bool {
	if job == nil {
		return false
	}
	if job.Type == model.TheaterPackageJobTypeImport || job.Type == model.TheaterPackageJobTypeImportCCFOLIA {
		return job.TargetWorldID == worldID
	}
	return job.SourceWorldID == worldID
}

func theaterPackageJobView(job *model.TheaterPackageJobModel) fiber.Map {
	result := fiber.Map{
		"id": job.ID, "type": job.Type, "status": job.Status, "progress": job.Progress,
		"sourceWorldId": job.SourceWorldID, "targetWorldId": job.TargetWorldID,
		"inputChannelId": job.InputChannelID, "originalName": job.OriginalName,
		"outputFileName": job.OutputFileName, "outputFileSize": job.OutputFileSize,
		"packageHash": job.PackageHash, "errorCode": job.ErrorCode, "errorMessage": job.ErrorMessage,
		"createdAt": job.CreatedAt, "startedAt": job.StartedAt, "finishedAt": job.FinishedAt, "expiresAt": job.ExpiresAt,
	}
	if strings.TrimSpace(job.SummaryJSON) != "" {
		var summary service.TheaterPackageSummary
		if json.Unmarshal([]byte(job.SummaryJSON), &summary) == nil {
			result["summary"] = summary
		}
	}
	return result
}
