package service

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sealchat/model"
)

const LogMaxVersion int64 = 1<<53 - 1

type LogError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *LogError) Error() string         { return e.Code + ": " + e.Message }
func logError(code, message string) error { return &LogError{Code: code, Message: message} }
func logConflict(version int64) error {
	return &LogError{Code: "VERSION_CONFLICT", Message: "工程版本已变化，请重新加载", Details: map[string]any{"draftVersion": version}}
}

type LogCapabilities struct {
	EditDraft          bool `json:"editDraft"`
	ManageSources      bool `json:"manageSources"`
	ManageLinks        bool `json:"manageLinks"`
	Publish            bool `json:"publish"`
	EditOwnMessages    bool `json:"editOwnMessages"`
	EditOthersMessages bool `json:"editOthersMessages"`
	BulkAppearance     bool `json:"bulkAppearance"`
}
type LogManagement struct {
	LayoutSlug string `json:"layoutSlug"`
	ViewSlug   string `json:"viewSlug"`
}
type LogProjectContext struct {
	ProjectID          string          `json:"projectId"`
	WorldID            string          `json:"worldId"`
	Enabled            bool            `json:"enabled"`
	DraftVersion       int64           `json:"draftVersion"`
	Draft              json.RawMessage `json:"draft"`
	Capabilities       LogCapabilities `json:"capabilities"`
	PublishedReleaseID *string         `json:"publishedReleaseId"`
	Management         *LogManagement  `json:"management"`
}
type LogProjectUpdate struct {
	ExpectedVersion *int64  `json:"expectedVersion"`
	Enabled         *bool   `json:"enabled"`
	LayoutSlug      *string `json:"layoutSlug"`
	ViewSlug        *string `json:"viewSlug"`
}

var logSlugPattern = regexp.MustCompile(`^[a-z0-9]{4,64}$`)

// Rejection sampling avoids the bias of byte % 36 for arbitrary random bytes.
func randomLogSlug() (string, error) {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	out := make([]byte, 0, 10)
	var buf [32]byte
	for len(out) < 10 {
		if _, err := rand.Read(buf[:]); err != nil {
			return "", err
		}
		for _, b := range buf {
			if b < 252 {
				out = append(out, alphabet[int(b)%len(alphabet)])
			}
			if len(out) == 10 {
				break
			}
		}
	}
	return string(out), nil
}

func logWorldAccess(worldID, userID string, management bool) (*model.WorldModel, bool, error) {
	if userID == "" {
		return nil, false, logError("UNAUTHENTICATED", "请先登录")
	}
	w, err := GetWorldByID(worldID)
	if errors.Is(err, ErrWorldNotFound) {
		return nil, false, logError("NOT_FOUND", "工程不存在")
	}
	if err != nil {
		return nil, false, err
	}
	admin := IsWorldAdmin(worldID, userID)
	if !admin {
		if management || !w.AllowMemberAccessLogLayout {
			return nil, false, logError("PERMISSION_DENIED", "无权访问排版工程")
		}
		var member model.WorldMemberModel
		if err := model.GetDB().Where("world_id = ? AND user_id = ? AND role = ?", worldID, userID, model.WorldRoleMember).Take(&member).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, false, logError("PERMISSION_DENIED", "无权访问排版工程")
			}
			return nil, false, err
		}
	}
	return w, admin, nil
}

func logCheckSources(project *model.LogProjectModel, userID string) error {
	var draft struct {
		Sections []struct {
			Source struct {
				ChannelID string `json:"channelId"`
			} `json:"source"`
		} `json:"sections"`
	}
	if err := json.Unmarshal([]byte(project.DraftJSON), &draft); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, section := range draft.Sections {
		id := section.Source.ChannelID
		if seen[id] {
			continue
		}
		seen[id] = true
		var channel model.ChannelModel
		if err := model.GetDB().Where("id = ? AND world_id = ? AND status = ? AND is_private = ?", id, project.WorldID, model.ChannelStatusActive, false).Take(&channel).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return logError("PERMISSION_DENIED", "来源频道不可读")
			}
			return err
		}
		if !CanReadChannelByUserId(userID, id) {
			return logError("PERMISSION_DENIED", "来源频道不可读")
		}
	}
	return nil
}

func logProjectContext(p *model.LogProjectModel, w *model.WorldModel, admin bool) (*LogProjectContext, error) {
	result := &LogProjectContext{ProjectID: p.ID, WorldID: p.WorldID, Enabled: p.Enabled, DraftVersion: p.DraftVersion, Draft: json.RawMessage(p.DraftJSON), Capabilities: LogCapabilities{
		EditDraft: true, ManageSources: admin, ManageLinks: admin, Publish: admin, EditOwnMessages: true, EditOthersMessages: admin && w.AllowAdminEditMessages, BulkAppearance: true,
	}}
	if p.ActiveReleaseID != "" {
		id := p.ActiveReleaseID
		result.PublishedReleaseID = &id
	}
	if admin {
		var aliases []model.LogRouteAliasModel
		if err := model.GetDB().Where("project_id = ? AND active = ?", p.ID, true).Find(&aliases).Error; err != nil {
			return nil, err
		}
		result.Management = &LogManagement{}
		for _, alias := range aliases {
			if alias.Kind == "layout" {
				result.Management.LayoutSlug = alias.Slug
			}
			if alias.Kind == "view" {
				result.Management.ViewSlug = alias.Slug
			}
		}
	}
	return result, nil
}

func GetLogProject(worldID, userID string) (*LogProjectContext, error) {
	w, admin, err := logWorldAccess(worldID, userID, true)
	if err != nil {
		return nil, err
	}
	var p model.LogProjectModel
	err = model.GetDB().Where("world_id = ?", worldID).Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return logProjectContext(&p, w, admin)
}

func ResolveLogLayout(slug, userID string) (*LogProjectContext, error) {
	if userID == "" {
		return nil, logError("UNAUTHENTICATED", "请先登录")
	}
	var alias model.LogRouteAliasModel
	if err := model.GetDB().Where("slug = ? AND kind = ? AND active = ?", slug, "layout", true).Take(&alias).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, logError("NOT_FOUND", "工程不存在")
		}
		return nil, err
	}
	var p model.LogProjectModel
	if err := model.GetDB().Where("id = ? AND enabled = ?", alias.ProjectID, true).Take(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, logError("NOT_FOUND", "工程不存在")
		}
		return nil, err
	}
	w, admin, err := logWorldAccess(p.WorldID, userID, false)
	if err != nil {
		return nil, err
	}
	if !admin {
		if err := logCheckSources(&p, userID); err != nil {
			return nil, err
		}
	}
	return logProjectContext(&p, w, admin)
}

func defaultLogDraft(title string) string {
	b, _ := json.Marshal(map[string]any{
		"schemaVersion": 1, "title": title, "sections": []any{}, "elements": []any{}, "stylePackageId": logDefaultStyleID, "stylePackageVersion": logDefaultStyleVersion, "themeTokens": map[string]any{},
		"page": map[string]any{"preset": "a5", "widthMm": 148, "heightMm": 210, "marginsMm": map[string]any{"top": 15, "right": 15, "bottom": 15, "left": 15}, "gutterMm": 0, "columns": 1, "backgroundColor": "#ffffff", "backgroundAssetId": nil, "fontFamily": "sans-serif", "fontSizePt": 12, "lineHeight": 1.5, "headerTemplateId": nil, "footerTemplateId": nil, "showPageNumbers": true, "mergeNeighbors": true},
	})
	return string(b)
}

// ON CONFLICT DO NOTHING keeps PostgreSQL transactions usable on collisions.
// All other database errors propagate rather than being mistaken for collisions.
func insertLogAlias(tx *gorm.DB, projectID, kind, requested string) error {
	for attempt := 0; attempt < 16; attempt++ {
		slug := requested
		if slug == "" {
			var err error
			slug, err = randomLogSlug()
			if err != nil {
				return err
			}
		}
		a := model.LogRouteAliasModel{Slug: slug, ProjectID: projectID, Kind: kind, Active: true}
		r := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&a)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 1 {
			var stored model.LogRouteAliasModel
			if err := tx.Where("slug = ?", slug).Take(&stored).Error; err != nil {
				return err
			}
			if stored.ProjectID == projectID && stored.Kind == kind && stored.Active {
				return nil
			}
		}
		if requested != "" {
			return logError("SLUG_TAKEN", "入口名称已被使用")
		}
	}
	return logError("SLUG_TAKEN", "无法分配入口名称，请重试")
}

func UpdateLogProject(worldID, userID string, input LogProjectUpdate) (*LogProjectContext, error) {
	w, admin, err := logWorldAccess(worldID, userID, true)
	if err != nil {
		return nil, err
	}
	if input.Enabled == nil {
		return nil, logError("INVALID_INPUT", "enabled 必填")
	}
	if input.ExpectedVersion != nil && (*input.ExpectedVersion < 1 || *input.ExpectedVersion >= LogMaxVersion) {
		return nil, logError("INVALID_INPUT", "版本无效")
	}
	for _, slug := range []*string{input.LayoutSlug, input.ViewSlug} {
		if slug != nil && *slug != "" && !logSlugPattern.MatchString(*slug) {
			return nil, logError("INVALID_INPUT", "入口名称须为 4～64 位小写字母或数字")
		}
	}
	if input.LayoutSlug != nil && input.ViewSlug != nil && *input.LayoutSlug != "" && *input.LayoutSlug == *input.ViewSlug {
		return nil, logError("SLUG_TAKEN", "两个入口不能相同")
	}
	var p model.LogProjectModel
	if err := model.GetDB().Where("world_id = ?", worldID).Take(&p).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if p.ID != "" {
		if input.ExpectedVersion == nil || p.DraftVersion != *input.ExpectedVersion {
			return nil, logConflict(p.DraftVersion)
		}
	}
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		err := tx.Where("world_id = ?", worldID).Take(&p).Error
		creating := errors.Is(err, gorm.ErrRecordNotFound)
		if err != nil && !creating {
			return err
		}
		if creating {
			if input.ExpectedVersion != nil {
				return logConflict(0)
			}
			p = model.LogProjectModel{WorldID: worldID, Enabled: *input.Enabled, DraftVersion: 1, DraftJSON: defaultLogDraft(w.Name)}
			r := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&p)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return logConflict(1)
			}
			var stored model.LogProjectModel
			if err := tx.Where("world_id = ?", worldID).Take(&stored).Error; err != nil {
				return err
			}
			if stored.ID != p.ID {
				return logConflict(stored.DraftVersion)
			}
		} else {
			if input.ExpectedVersion == nil || p.DraftVersion != *input.ExpectedVersion {
				return logConflict(p.DraftVersion)
			}
			r := tx.Model(&model.LogProjectModel{}).Where("id = ? AND draft_version = ?", p.ID, *input.ExpectedVersion).Updates(map[string]any{"enabled": *input.Enabled, "draft_version": gorm.Expr("draft_version + 1")})
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return logConflict(p.DraftVersion)
			}
			p.Enabled = *input.Enabled
			p.DraftVersion++
		}
		for _, route := range []struct {
			kind string
			slug *string
		}{{"layout", input.LayoutSlug}, {"view", input.ViewSlug}} {
			if !creating && route.slug == nil {
				continue
			}
			requested := ""
			if route.slug != nil {
				requested = *route.slug
			}
			var current model.LogRouteAliasModel
			if !creating {
				if err := tx.Where("project_id = ? AND kind = ? AND active = ?", p.ID, route.kind, true).Take(&current).Error; err != nil {
					return err
				}
				if requested == current.Slug {
					continue
				}
				if err := tx.Model(&model.LogRouteAliasModel{}).Where("slug = ?", current.Slug).Updates(map[string]any{"active": false, "invalidated_at": time.Now().UTC()}).Error; err != nil {
					return err
				}
			}
			if err := insertLogAlias(tx, p.ID, route.kind, requested); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return logProjectContext(&p, w, admin)
}
