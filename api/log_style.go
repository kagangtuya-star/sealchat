package api

import (
	"bytes"
	"io"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"sealchat/service"
)

func bindLogStyleRoutes(r fiber.Router) {
	r.Get("/log/styles", logAuthentication, logStylesList)
	r.Post("/log/styles", logAuthentication, logStyleCreate)
	r.Post("/log/styles/import", logAuthentication, logStylesImport)
	r.Get("/log/styles/:id/versions/:version", logAuthentication, logStyleGet)
	r.Get("/log/styles/:id/versions/:version/assets/:assetId", logAuthentication, logStyleAssetGet)
	r.Get("/log/styles/:id/versions/:version/export", logAuthentication, logStyleExport)
	r.Post("/log/styles/:id/fork", logAuthentication, logStyleFork)
	r.Post("/log/styles/:id/versions", logAuthentication, logStyleSave)
	r.Delete("/log/styles/:id", logAuthentication, logStyleDelete)
}

func logStyleAssetGet(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	a, err := service.GetLogStyleAsset(c.Params("id"), c.Params("version"), c.Params("assetId"), u.ID)
	if err != nil {
		return logResponse(c, nil, err)
	}
	c.Set("Content-Type", a.MIME)
	c.Set("Cache-Control", "no-store")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Referrer-Policy", "no-referrer")
	return c.SendStream(bytes.NewReader(a.Data), len(a.Data))
}
func logStylesList(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	limit, err := strconv.Atoi(c.Query("limit", "50"))
	if err != nil {
		return logResponse(c, nil, &service.LogError{Code: "INVALID_INPUT", Message: "limit 无效"})
	}
	data, err := service.ListLogStyles(u.ID, c.Query("cursor"), limit)
	return logResponse(c, data, err)
}
func logStyleGet(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	data, err := service.GetLogStyle(c.Params("id"), c.Params("version"), u.ID)
	return logResponse(c, data, err)
}
func logStyleSave(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	var input struct {
		BaseVersion string                  `json:"baseVersion"`
		Package     service.LogStylePackage `json:"package"`
	}
	if err := logDecodeBody(c, &input); err != nil {
		return logResponse(c, nil, err)
	}
	data, err := service.SaveLogStyle(c.Params("id"), u.ID, input.BaseVersion, input.Package)
	return logResponse(c, data, err)
}
func logStyleCreate(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	var input struct {
		Package service.LogStylePackage `json:"package"`
	}
	if err := logDecodeBody(c, &input); err != nil {
		return logResponse(c, nil, err)
	}
	data, err := service.CreateLogStyle(u.ID, input.Package)
	return logResponse(c, data, err)
}
func logStyleDelete(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	err := service.DeleteLogStyle(c.Params("id"), u.ID)
	return logResponse(c, map[string]bool{"deleted": err == nil}, err)
}
func logStyleFork(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	var input struct {
		Version string `json:"version"`
		Name    string `json:"name"`
	}
	if err := logDecodeBody(c, &input); err != nil {
		return logResponse(c, nil, err)
	}
	data, err := service.ForkLogStyle(c.Params("id"), input.Version, u.ID, input.Name)
	return logResponse(c, data, err)
}
func logStylesImport(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	f, err := c.FormFile("file")
	if err != nil {
		return logResponse(c, nil, &service.LogError{Code: "INVALID_INPUT", Message: "缺少 ZIP 文件"})
	}
	if f.Size > 20<<20 {
		return logResponse(c, nil, &service.LogError{Code: "TOO_LARGE", Message: "ZIP 超过 20MB"})
	}
	r, err := f.Open()
	if err != nil {
		return logResponse(c, nil, err)
	}
	defer r.Close()
	b, err := io.ReadAll(io.LimitReader(r, (20<<20)+1))
	if err != nil {
		return logResponse(c, nil, err)
	}
	data, err := service.ImportLogStyle(u.ID, b)
	return logResponse(c, data, err)
}
func logStyleExport(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	b, err := service.ExportLogStyle(c.Params("id"), c.Params("version"), u.ID)
	if err != nil {
		return logResponse(c, nil, err)
	}
	c.Set("Cache-Control", "no-store")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Content-Type", "application/zip")
	c.Set("Content-Disposition", `attachment; filename="log-style.zip"`)
	return c.Send(b)
}
