package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"sealchat/model"
)

func logStyleAccess(id, version, userID string) (*model.LogStylePackageModel, error) {
	if userID == "" {
		return nil, logError("UNAUTHENTICATED", "请先登录")
	}
	var p model.LogStylePackageModel
	if err := model.GetDB().Where("id = ?", id).Take(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, logError("NOT_FOUND", "样式不存在")
		}
		return nil, err
	}
	if p.Builtin || p.OwnerID == userID {
		return &p, nil
	}
	// Search the derived binding index, then recheck the authoritative draft.
	last := ""
	for {
		var bindings []model.LogProjectStyleBindingModel
		q := model.GetDB().Table("log_project_style_bindings b").Select("b.*").Joins("JOIN log_projects p ON p.id = b.project_id").Joins("JOIN worlds w ON w.id = p.world_id").Where("b.package_id = ? AND b.version = ? AND b.project_id > ? AND p.enabled = ?", id, version, last, true).Where("w.owner_id = ? OR EXISTS (SELECT 1 FROM world_members wm WHERE wm.world_id = w.id AND wm.user_id = ? AND (wm.role IN ? OR (wm.role = ? AND w.allow_member_access_log_layout = ?)))", userID, userID, []string{"owner", "admin"}, "member", true)
		if err := q.Order("b.project_id").Limit(100).Find(&bindings).Error; err != nil {
			return nil, err
		}
		for _, binding := range bindings {
			last = binding.ProjectID
			project, _, err := loadLogProjectForDesign(binding.ProjectID, userID)
			if err != nil {
				continue
			}
			var d LogDraft
			if json.Unmarshal([]byte(project.DraftJSON), &d) == nil && d.StylePackageID == id && d.StylePackageVersion == version {
				return &p, nil
			}
		}
		if len(bindings) < 100 {
			break
		}
	}
	return nil, logError("PERMISSION_DENIED", "无权读取样式")
}

func GetLogStyle(id, version, userID string) (*LogStylePackage, error) {
	if _, err := logStyleAccess(id, version, userID); err != nil {
		return nil, err
	}
	return logLoadStyle(id, version)
}

func GetLogStyleAsset(id, version, assetID, userID string) (*model.LogStyleAssetModel, error) {
	if _, err := logStyleAccess(id, version, userID); err != nil {
		return nil, err
	}
	var a model.LogStyleAssetModel
	if err := model.GetDB().Where("package_id = ? AND version = ? AND asset_id = ?", id, version, assetID).Take(&a).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, logError("NOT_FOUND", "资源不存在")
		}
		return nil, err
	}
	return &a, nil
}
func logLoadStyle(id, version string) (*LogStylePackage, error) {
	var v model.LogStyleVersionModel
	if err := model.GetDB().Where("package_id = ? AND version = ?", id, version).Take(&v).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, logError("NOT_FOUND", "样式版本不存在")
		}
		return nil, err
	}
	var p LogStylePackage
	if err := json.Unmarshal([]byte(v.PackageJSON), &p); err != nil {
		return nil, err
	}
	return &p, nil
}
func logStyleAssets(id, version string) (map[string][]byte, error) {
	var rows []model.LogStyleAssetModel
	if err := model.GetDB().Where("package_id = ? AND version = ?", id, version).Find(&rows).Error; err != nil {
		return nil, err
	}
	data := map[string][]byte{}
	for _, r := range rows {
		data[r.AssetID] = r.Data
	}
	return data, nil
}
func ListLogStyles(userID, cursor string, limit int) (map[string]any, error) {
	if limit < 1 || limit > 50 || len(cursor) > 300 {
		return nil, logError("INVALID_INPUT", "分页参数无效")
	}
	last := ""
	if cursor != "" {
		b, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return nil, logError("INVALID_INPUT", "游标无效")
		}
		last = string(b)
	}
	var rows []model.LogStylePackageModel
	if err := model.GetDB().Where("((builtin = ? AND id IN ?) OR (builtin = ? AND owner_id = ?)) AND id > ?", true, logCanonicalBuiltinIDs(), false, userID, last).Order("id").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > limit {
		rows = rows[:limit]
		s := base64.RawURLEncoding.EncodeToString([]byte(rows[len(rows)-1].ID))
		next = &s
	}
	items := []map[string]any{}
	for _, p := range rows {
		items = append(items, map[string]any{"id": p.ID, "version": p.LatestVersion, "name": p.Name, "builtin": p.Builtin, "previewAssetId": nil})
	}
	return map[string]any{"items": items, "nextCursor": next}, nil
}
func logVersionAfter(next, base string) bool {
	if !logSemver.MatchString(next) || !logSemver.MatchString(base) {
		return false
	}
	a, b := strings.Split(next, "."), strings.Split(base, ".")
	for i := range a {
		x, e1 := strconv.ParseUint(a[i], 10, 32)
		y, e2 := strconv.ParseUint(b[i], 10, 32)
		if e1 != nil || e2 != nil {
			return false
		}
		if x != y {
			return x > y
		}
	}
	return false
}
func logStoreStyle(tx *gorm.DB, p *LogStylePackage, assets map[string][]byte) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	v := model.LogStyleVersionModel{PackageID: p.ID, Version: p.Version, PackageJSON: string(b), CreatedAt: time.Now().UTC()}
	if err := tx.Create(&v).Error; err != nil {
		return err
	}
	for _, a := range p.Assets {
		row := model.LogStyleAssetModel{PackageID: p.ID, Version: p.Version, AssetID: a.ID, Path: a.Path, MIME: a.MIME, Hash: a.SHA256, Data: assets[a.ID]}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}
func SaveLogStyle(id, userID, base string, p LogStylePackage) (*LogStylePackage, error) {
	owner, err := logStyleAccess(id, base, userID)
	if err != nil {
		return nil, err
	}
	if owner.Builtin || owner.OwnerID != userID {
		return nil, logError("PERMISSION_DENIED", "仅所有者可以保存新版本")
	}
	if p.ID != id || !logVersionAfter(p.Version, base) {
		return nil, logTemplateError()
	}
	assets, err := logStyleAssets(id, base)
	if err != nil {
		return nil, err
	}
	if err := logStyleUploadedAssets(userID, p.Assets, assets); err != nil {
		return nil, err
	}
	if err := ValidateLogStyle(&p, assets); err != nil {
		return nil, err
	}
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		r := tx.Model(&model.LogStylePackageModel{}).Where("id = ? AND owner_id = ? AND latest_version = ?", id, userID, base).Updates(map[string]any{"latest_version": p.Version, "name": p.Name})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return logError("VERSION_CONFLICT", "样式已有新版本")
		}
		return logStoreStyle(tx, &p, assets)
	})
	return &p, err
}

// logStyleUploadedAssets reads package assets that are not yet stored bytes from the
// caller's own attachments; the attachment ID doubles as the new asset ID.
func logStyleUploadedAssets(userID string, declared []LogStyleAsset, assets map[string][]byte) error {
	for _, asset := range declared {
		if _, ok := assets[asset.ID]; ok {
			continue
		}
		a, err := ResolveAttachmentOwnership(userID, asset.ID)
		if err != nil || a == nil || a.IsTTSManaged() || a.Size > 20<<20 {
			return logTemplateError()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		reader, err := OpenAttachmentRead(ctx, a)
		if err != nil {
			cancel()
			return err
		}
		b, readErr := io.ReadAll(io.LimitReader(reader, (20<<20)+1))
		reader.Close()
		cancel()
		if readErr != nil || len(b) > 20<<20 {
			return logTemplateError()
		}
		assets[asset.ID] = b
	}
	return nil
}

// CreateLogStyle stores a brand-new private package as its first version. The server
// assigns the package ID and version 1.0.0; assets can only come from the caller's
// own attachments because a new package has no stored bytes to inherit.
func CreateLogStyle(userID string, p LogStylePackage) (*LogStylePackage, error) {
	if userID == "" {
		return nil, logError("UNAUTHENTICATED", "请先登录")
	}
	row := model.LogStylePackageModel{OwnerID: userID, Name: p.Name, LatestVersion: "1.0.0"}
	row.Init()
	row.ID = "style-" + row.ID
	p.ID = row.ID
	p.Version = "1.0.0"
	assets := map[string][]byte{}
	if err := logStyleUploadedAssets(userID, p.Assets, assets); err != nil {
		return nil, err
	}
	if err := ValidateLogStyle(&p, assets); err != nil {
		return nil, err
	}
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return logStoreStyle(tx, &p, assets)
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func DeleteLogStyle(id, userID string) error {
	if userID == "" {
		return logError("UNAUTHENTICATED", "请先登录")
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		var p model.LogStylePackageModel
		if err := tx.Where("id = ?", id).Take(&p).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return logError("NOT_FOUND", "样式不存在")
			}
			return err
		}
		if p.Builtin || p.OwnerID != userID {
			return logError("PERMISSION_DENIED", "只能删除自己的自定义样式")
		}
		var refs int64
		if err := tx.Model(&model.LogProjectStyleBindingModel{}).Where("package_id = ?", id).Count(&refs).Error; err != nil {
			return err
		}
		if refs > 0 {
			return logError("STYLE_IN_USE", "该样式仍被 LOG 工程使用，请先切换这些工程的样式")
		}
		if err := tx.Where("package_id = ?", id).Delete(&model.LogStyleAssetModel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("package_id = ?", id).Delete(&model.LogStyleVersionModel{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ? AND owner_id = ? AND builtin = ?", id, userID, false).Delete(&model.LogStylePackageModel{}).Error
	})
}

func ForkLogStyle(id, version, userID, name string) (*LogStylePackage, error) {
	owner, err := logStyleAccess(id, version, userID)
	if err != nil {
		return nil, err
	}
	p, err := logLoadStyle(id, version)
	if err != nil {
		return nil, err
	}
	if !owner.Builtin && owner.OwnerID != userID && !logStyleDistributionLicense(p.License) {
		return nil, logError("PERMISSION_DENIED", "私有包未授予资源分发权限")
	}
	assets, err := logStyleAssets(id, version)
	if err != nil {
		return nil, err
	}
	row := model.LogStylePackageModel{OwnerID: userID, Name: name, LatestVersion: "1.0.0"}
	row.Init()
	row.ID = "style-" + row.ID
	p.ID = row.ID
	p.Version = "1.0.0"
	p.Name = name
	if err := ValidateLogStyle(p, assets); err != nil {
		return nil, err
	}
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return logStoreStyle(tx, p, assets)
	})
	return p, err
}

func ImportLogStyle(userID string, data []byte) (map[string]any, error) {
	if len(data) > 20<<20 {
		return nil, logError("TOO_LARGE", "ZIP 超过 20MB")
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, logTemplateError()
	}
	if len(z.File) > 200 {
		return nil, logTemplateError()
	}
	files := map[string][]byte{}
	total := int64(0)
	for _, f := range z.File {
		name := f.Name
		if f.FileInfo().IsDir() {
			clean := strings.TrimSuffix(name, "/")
			if path.Clean(clean) != clean || strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, "..") || strings.ContainsAny(clean, "\\\x00:") || f.Flags&1 != 0 {
				return nil, logTemplateError()
			}
			continue
		}
		if name == "" || path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\\x00:") || strings.HasPrefix(name, "../") || f.Mode()&0111 != 0 || !f.Mode().IsRegular() || f.Flags&1 != 0 {
			return nil, logTemplateError()
		}
		if _, ok := files[name]; ok {
			return nil, logTemplateError()
		}
		limit := int64(512 << 10)
		if strings.HasPrefix(name, "assets/") || strings.HasPrefix(name, "previews/") {
			limit = 50 << 20
		}
		if f.UncompressedSize64 > uint64(limit) || f.UncompressedSize64 > uint64((50<<20)-total) {
			return nil, logError("TOO_LARGE", "ZIP 解压配额超限")
		}
		r, err := f.Open()
		if err != nil {
			return nil, logTemplateError()
		}
		b, readErr := io.ReadAll(io.LimitReader(r, limit+1))
		closeErr := r.Close()
		if readErr != nil || closeErr != nil || int64(len(b)) > limit {
			return nil, logTemplateError()
		}
		total += int64(len(b))
		if total > 50<<20 {
			return nil, logTemplateError()
		}
		files[name] = b
	}
	var p LogStylePackage
	for _, required := range []string{"manifest.json", "tokens.json", "settings.schema.json", "styles/main.css"} {
		if _, ok := files[required]; !ok {
			return nil, logTemplateError()
		}
	}
	if err := logStrictJSON(files["manifest.json"], &p); err != nil {
		return nil, err
	}
	if p.Templates != nil || p.Tokens != nil || p.CSS != "" || p.SettingsSchema != nil {
		return nil, logTemplateError()
	}
	if logStrictJSON(files["tokens.json"], &p.Tokens) != nil || logStrictJSON(files["settings.schema.json"], &p.SettingsSchema) != nil {
		return nil, logTemplateError()
	}
	p.CSS = string(files["styles/main.css"])
	p.Templates = map[string]string{}
	assets := map[string][]byte{}
	known := map[string]bool{"manifest.json": true, "tokens.json": true, "settings.schema.json": true, "styles/main.css": true}
	for name, b := range files {
		if strings.HasPrefix(name, "templates/") && strings.HasSuffix(name, ".html") {
			key := strings.TrimSuffix(strings.TrimPrefix(name, "templates/"), ".html")
			p.Templates[key] = string(b)
			known[name] = true
		}
	}
	for _, a := range p.Assets {
		b, ok := files[a.Path]
		if !ok {
			return nil, logTemplateError()
		}
		assets[a.ID] = b
		known[a.Path] = true
	}
	for name := range files {
		if !known[name] {
			if strings.HasPrefix(name, "previews/") {
				cfg, format, err := image.DecodeConfig(bytes.NewReader(files[name]))
				if err == nil && (format == "png" || format == "jpeg" || format == "webp") && cfg.Width > 0 && cfg.Height > 0 && int64(cfg.Width)*int64(cfg.Height) <= 40000000 {
					continue
				}
			}
			return nil, logTemplateError()
		}
	}
	if err := ValidateLogStyle(&p, assets); err != nil {
		return nil, err
	}
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		var existing model.LogStylePackageModel
		err := tx.Where("id = ?", p.ID).Take(&existing).Error
		if err == nil {
			if existing.OwnerID != userID || existing.Builtin {
				return logError("PERMISSION_DENIED", "无权导入此包的新版本")
			}
			if !logVersionAfter(p.Version, existing.LatestVersion) {
				return logError("VERSION_CONFLICT", "样式版本必须递增")
			}
			r := tx.Model(&existing).Where("latest_version = ?", existing.LatestVersion).Updates(map[string]any{"latest_version": p.Version, "name": p.Name})
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return logError("VERSION_CONFLICT", "样式已有新版本")
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			row := model.LogStylePackageModel{OwnerID: userID, Name: p.Name, LatestVersion: p.Version}
			row.Init()
			row.ID = p.ID
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		} else {
			return err
		}
		return logStoreStyle(tx, &p, assets)
	})
	return map[string]any{"stylePackageId": p.ID, "version": p.Version, "warnings": []string{}}, err
}
func logStrictJSON(b []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil {
		return logTemplateError()
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return logTemplateError()
	}
	return nil
}
func ExportLogStyle(id, version, userID string) ([]byte, error) {
	owner, err := logStyleAccess(id, version, userID)
	if err != nil {
		return nil, err
	}
	p, err := logLoadStyle(id, version)
	if err != nil {
		return nil, err
	}
	if !owner.Builtin && owner.OwnerID != userID && !logStyleDistributionLicense(p.License) {
		return nil, logError("PERMISSION_DENIED", "私有包未授予分发权限")
	}
	assets, err := logStyleAssets(id, version)
	if err != nil {
		return nil, err
	}
	if err := ValidateLogStyle(p, assets); err != nil {
		return nil, err
	}
	files := map[string][]byte{"styles/main.css": []byte(p.CSS)}
	files["tokens.json"], _ = json.Marshal(p.Tokens)
	files["settings.schema.json"], _ = json.Marshal(p.SettingsSchema)
	for id, src := range p.Templates {
		files["templates/"+id+".html"] = []byte(src)
	}
	for _, a := range p.Assets {
		files[a.Path] = assets[a.ID]
	}
	p.Tokens = nil
	p.SettingsSchema = nil
	p.Templates = nil
	p.CSS = ""
	// JSON null fields in the manifest are accepted, but never package owner metadata.
	files["manifest.json"], _ = json.Marshal(p)
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	names := []string{}
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		f, err := w.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(files[name]); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func logStyleDistributionLicense(license string) bool {
	switch license {
	case "MIT", "Apache-2.0", "CC0-1.0", "CC-BY-4.0":
		return true
	}
	return false
}
