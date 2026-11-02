package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"sealchat/model"
	"sealchat/pm"
	"sealchat/utils"
)

type UISurface string

const (
	UISurfaceAuth              UISurface = "auth"
	UISurfaceLobby             UISurface = "lobby"
	UISurfaceWorldDetail       UISurface = "world-detail"
	UISurfaceChat              UISurface = "chat"
	UISurfaceSplit             UISurface = "split"
	UISurfaceEmbed             UISurface = "embed"
	UISurfaceTheater           UISurface = "theater"
	UISurfaceObserver          UISurface = "observer"
	UISurfaceInvite            UISurface = "invite"
	UISurfaceLogLayout         UISurface = "log-layout"
	UISurfaceLogPublic         UISurface = "log-public"
	UISurfaceStatus            UISurface = "status"
	UISurfaceInternal          UISurface = "internal"
	UISurfaceGlobal            UISurface = "global"
	UICustomizationMaxPackages           = 50
	UICustomizationMaxCSSBytes           = 256 * 1024
	UICustomizationMaxJSBytes            = 256 * 1024
	UICustomizationMaxCodeSize           = 384 * 1024
)

func (s UISurface) Valid() bool {
	switch s {
	case UISurfaceAuth, UISurfaceLobby, UISurfaceWorldDetail, UISurfaceChat,
		UISurfaceSplit, UISurfaceEmbed, UISurfaceTheater, UISurfaceObserver,
		UISurfaceInvite, UISurfaceLogLayout, UISurfaceLogPublic, UISurfaceStatus,
		UISurfaceInternal, UISurfaceGlobal:
		return true
	}
	return false
}

var (
	ErrUICustomizationInvalid  = errors.New("UI customization invalid")
	ErrUICustomizationDenied   = errors.New("UI customization permission denied")
	ErrUICustomizationNotFound = errors.New("UI customization not found")
	ErrUICustomizationLimit    = errors.New("UI customization package limit reached")
	ErrUICustomizationBuiltin  = errors.New("builtin UI customization is read-only")
	// SealChat runs a single server process. This serializes count/append and
	// reorder operations in that process; it is not a distributed quota lock.
	uiCustomizationMutationMu sync.Mutex
)

type UICustomizationScope struct {
	Type string
	ID   string
}

type UICustomizationPatch struct {
	Name                *string      `json:"name"`
	Description         *string      `json:"description"`
	Enabled             *bool        `json:"enabled"`
	Surfaces            *[]UISurface `json:"surfaces"`
	CSS                 *string      `json:"css"`
	JS                  *string      `json:"js"`
	ChannelCapabilities *[]string    `json:"channelCapabilities"`
}

type UICustomizationPackage struct {
	ID                  string      `json:"id"`
	Ref                 string      `json:"ref"`
	Origin              string      `json:"origin"`
	BuiltinKey          string      `json:"builtinKey,omitempty"`
	Version             string      `json:"version,omitempty"`
	Readonly            bool        `json:"readonly"`
	ScopeType           string      `json:"scopeType"`
	ScopeID             string      `json:"scopeId"`
	Name                string      `json:"name"`
	Description         string      `json:"description"`
	Enabled             bool        `json:"enabled"`
	SortOrder           int         `json:"sortOrder"`
	Surfaces            []UISurface `json:"surfaces"`
	CSS                 string      `json:"css"`
	JS                  string      `json:"js"`
	JSHash              string      `json:"jsHash"`
	ChannelCapabilities []string    `json:"channelCapabilities"`
	RuntimeFingerprint  string      `json:"runtimeFingerprint"`
	CreatedBy           string      `json:"createdBy"`
	UpdatedBy           string      `json:"updatedBy"`
	CreatedAt           int64       `json:"createdAt"`
	UpdatedAt           int64       `json:"updatedAt"`
	WorldEnabled        bool        `json:"worldEnabled,omitempty"`
}

func uiCustomizationView(m *model.UICustomizationPackageModel) (*UICustomizationPackage, error) {
	var surfaces []UISurface
	name, description, css, js, version := m.Name, m.Description, m.CSS, m.JS, m.BuiltinVersion
	if m.BuiltinKey != "" {
		item, err := builtinUICustomizationForKey(m.BuiltinKey)
		if err != nil {
			return nil, err
		}
		name, description, css, js, version = item.Name, item.Description, item.CSS, item.JS, item.Version
		surfaces = append([]UISurface(nil), item.Targets...)
	} else if err := json.Unmarshal([]byte(m.SurfacesJSON), &surfaces); err != nil {
		return nil, err
	}
	origin, ref := "user", m.ID
	if m.BuiltinKey != "" {
		origin, ref = "builtin", BuiltinUICustomizationRefPrefix+m.BuiltinKey
	}
	return &UICustomizationPackage{ID: m.ID, Ref: ref, Origin: origin, BuiltinKey: m.BuiltinKey,
		Version: version, Readonly: m.BuiltinKey != "", ScopeType: m.ScopeType, ScopeID: m.ScopeID,
		Name: name, Description: description, Enabled: m.Enabled, SortOrder: m.SortOrder,
		Surfaces: surfaces, CSS: css, JS: js, JSHash: UICustomizationJSHash(js), ChannelCapabilities: uiCustomizationChannelCapabilities(m), RuntimeFingerprint: UICustomizationRuntimeFingerprint(js, uiCustomizationChannelCapabilities(m)), CreatedBy: m.CreatedBy, UpdatedBy: m.UpdatedBy,
		CreatedAt: m.CreatedAt.UnixMilli(), UpdatedAt: m.UpdatedAt.UnixMilli()}, nil
}

// UICustomizationJSHash identifies a script version for browser trust. Line
// endings are normalized so CRLF/LF round trips keep the same identity. It is
// not a signature. Empty scripts have no hash.
func UICustomizationJSHash(js string) string {
	if js == "" {
		return ""
	}
	normalized := strings.ReplaceAll(strings.ReplaceAll(js, "\r\n", "\n"), "\r", "\n")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// Empty declarations retain the legacy JS identity. Capability order is immaterial.
func UICustomizationRuntimeFingerprint(js string, capabilities []string) string {
	if len(capabilities) == 0 {
		return UICustomizationJSHash(js)
	}
	canonical := append([]string(nil), capabilities...)
	sort.Strings(canonical)
	data, _ := json.Marshal(canonical)
	sum := sha256.Sum256([]byte("sealchat.ui-runtime.v2\x00" + UICustomizationJSHash(js) + "\x00" + string(data)))
	return hex.EncodeToString(sum[:])
}

func uiCustomizationManage(scope UICustomizationScope, actorID string) error {
	if actorID == "" {
		return ErrUICustomizationDenied
	}
	switch scope.Type {
	case "platform":
		if scope.ID != "" {
			return ErrUICustomizationInvalid
		}
		if !pm.CanWithSystemRole(actorID, pm.PermModAdmin) {
			return ErrUICustomizationDenied
		}
	case "world":
		if scope.ID == "" {
			return ErrUICustomizationInvalid
		}
		world, err := GetWorldByID(scope.ID)
		if err != nil {
			return err
		}
		if world.Status != "active" {
			return ErrWorldNotFound
		}
		// Follow world appearance/observer settings: world owner/admin only.
		if !IsWorldAdmin(scope.ID, actorID) {
			return ErrUICustomizationDenied
		}
	default:
		return ErrUICustomizationInvalid
	}
	return nil
}

func uiCustomizationQuery(db *gorm.DB, scope UICustomizationScope) *gorm.DB {
	return db.Model(&model.UICustomizationPackageModel{}).Where("scope_type = ? AND scope_id = ?", scope.Type, scope.ID)
}

// Older user-created packages can have NULL builtin_key from the migration
// that introduced builtin references. Treat NULL and empty string alike.
func uiCustomizationNonBuiltinQuery(db *gorm.DB, scope UICustomizationScope) *gorm.DB {
	return uiCustomizationQuery(db, scope).Where("builtin_key = ? OR builtin_key IS NULL", "")
}

func uiCustomizationRows(db *gorm.DB, scope UICustomizationScope) ([]model.UICustomizationPackageModel, error) {
	rows := []model.UICustomizationPackageModel{}
	err := uiCustomizationQuery(db, scope).Order("sort_order ASC, created_at ASC, id ASC").Find(&rows).Error
	return rows, err
}

func uiCustomizationUserPackageCount(rows []model.UICustomizationPackageModel) int {
	count := 0
	for _, row := range rows {
		if row.BuiltinKey == "" {
			count++
		}
	}
	return count
}

func ListUICustomizations(scope UICustomizationScope, actorID string) ([]*UICustomizationPackage, error) {
	if err := uiCustomizationManage(scope, actorID); err != nil {
		return nil, err
	}
	rows, err := uiCustomizationRows(model.GetDB(), scope)
	if err != nil {
		return nil, err
	}
	result := make([]*UICustomizationPackage, 0, len(rows))
	for i := range rows {
		view, err := uiCustomizationView(&rows[i])
		if err != nil {
			return nil, err
		}
		result = append(result, view)
	}
	if scope.Type == "world" {
		var refs []model.WorldUICustomizationPlatformRefModel
		if err := model.GetDB().Where("world_id = ?", scope.ID).Find(&refs).Error; err != nil {
			return nil, err
		}
		referenced := make(map[string]bool, len(refs))
		for _, ref := range refs {
			referenced[ref.PlatformPackageID] = true
		}
		var platformRows []model.UICustomizationPackageModel
		if err := uiCustomizationNonBuiltinQuery(model.GetDB(), UICustomizationScope{Type: "platform"}).
			Order("sort_order ASC, created_at ASC, id ASC").Find(&platformRows).Error; err != nil {
			return nil, err
		}
		platformItems := make([]*UICustomizationPackage, 0, len(platformRows))
		for i := range platformRows {
			view, err := uiCustomizationView(&platformRows[i])
			if err != nil {
				return nil, err
			}
			view.WorldEnabled = referenced[view.ID]
			platformItems = append(platformItems, view)
		}
		result = append(platformItems, result...)
	}
	return result, nil
}

func SetWorldPlatformUICustomizationReference(worldID, actorID, platformPackageID string, enabled bool) error {
	scope := UICustomizationScope{Type: "world", ID: strings.TrimSpace(worldID)}
	if err := uiCustomizationManage(scope, actorID); err != nil {
		return err
	}
	platformPackageID = strings.TrimSpace(platformPackageID)
	if platformPackageID == "" {
		return ErrUICustomizationInvalid
	}
	var pkg model.UICustomizationPackageModel
	if err := uiCustomizationNonBuiltinQuery(model.GetDB(), UICustomizationScope{Type: "platform"}).
		Where("id = ?", platformPackageID).Take(&pkg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUICustomizationNotFound
		}
		return err
	}
	query := model.GetDB().Where("world_id = ? AND platform_package_id = ?", scope.ID, platformPackageID)
	if !enabled {
		return query.Delete(&model.WorldUICustomizationPlatformRefModel{}).Error
	}
	var count int64
	if err := query.Model(&model.WorldUICustomizationPlatformRefModel{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return model.GetDB().Create(&model.WorldUICustomizationPlatformRefModel{
		WorldID: scope.ID, PlatformPackageID: platformPackageID,
	}).Error
}

func uiCustomizationInvalid(detail string) error {
	return fmt.Errorf("%w: %s", ErrUICustomizationInvalid, detail)
}

// applyUICustomizationPatch validates a create/update/import payload. When the
// platform disables world scripts, existing world JS is preserved untouched
// (so re-enabling restores it), may be cleared, but cannot be replaced.
func applyUICustomizationPatch(m *model.UICustomizationPackageModel, p UICustomizationPatch, policy *utils.UICustomizationPolicyConfig) error {
	invalid := uiCustomizationInvalid
	if p.ChannelCapabilities != nil {
		capabilities := []string{}
		for _, raw := range *p.ChannelCapabilities {
			capability := strings.TrimSpace(raw)
			if !HasChannelExtensionCapability(ChannelExtensionCapabilities, capability) {
				return invalid("频道能力无效: " + capability)
			}
			if !HasChannelExtensionCapability(capabilities, capability) {
				capabilities = append(capabilities, capability)
			}
		}
		sort.Strings(capabilities)
		data, _ := json.Marshal(capabilities)
		m.ChannelCapabilitiesJSON = string(data)
	}
	if p.JS != nil {
		if m.ScopeType == "world" && *p.JS != "" && *p.JS != m.JS && !utils.NormalizeUICustomizationPolicy(policy).AllowWorldJS {
			return invalid("平台未开放世界脚本，不能写入新的 JavaScript")
		}
		m.JS = *p.JS
	}
	if p.Name != nil {
		m.Name = strings.TrimSpace(*p.Name)
	}
	if p.Description != nil {
		m.Description = strings.TrimSpace(*p.Description)
	}
	if p.Enabled != nil {
		m.Enabled = *p.Enabled
	}
	if p.CSS != nil {
		m.CSS = *p.CSS
	}
	if m.Name == "" || utf8.RuneCountInString(m.Name) > 100 {
		return invalid("名称须为 1～100 个字符")
	}
	if utf8.RuneCountInString(m.Description) > 500 {
		return invalid("描述最多 500 个字符")
	}
	if len(m.CSS) > UICustomizationMaxCSSBytes {
		return invalid("CSS 最多 256 KiB")
	}
	if len(m.JS) > UICustomizationMaxJSBytes {
		return invalid("JavaScript 最多 256 KiB")
	}
	if len(m.CSS)+len(m.JS) > UICustomizationMaxCodeSize {
		return invalid("CSS 与 JavaScript 合计最多 384 KiB")
	}
	if p.Surfaces != nil {
		if len(*p.Surfaces) == 0 || len(*p.Surfaces) > 32 {
			return invalid("须指定 1～32 个 Surface")
		}
		surfaces := []UISurface{}
		seen := map[UISurface]bool{}
		for _, s := range *p.Surfaces {
			if !s.Valid() {
				return invalid("Surface 无效")
			}
			if !seen[s] {
				surfaces = append(surfaces, s)
				seen[s] = true
			}
		}
		data, err := json.Marshal(surfaces)
		if err != nil {
			return err
		}
		m.SurfacesJSON = string(data)
	}
	if m.SurfacesJSON == "" {
		return invalid("须指定 Surface")
	}
	return nil
}

func CreateUICustomization(scope UICustomizationScope, actorID string, input UICustomizationPatch, policy *utils.UICustomizationPolicyConfig) (*UICustomizationPackage, error) {
	if err := uiCustomizationManage(scope, actorID); err != nil {
		return nil, err
	}
	m := model.UICustomizationPackageModel{ScopeType: scope.Type, ScopeID: scope.ID, Enabled: true, CreatedBy: actorID, UpdatedBy: actorID}
	if err := applyUICustomizationPatch(&m, input, policy); err != nil {
		return nil, err
	}
	items, err := createUICustomizationModels(scope, []*model.UICustomizationPackageModel{&m})
	if err != nil {
		return nil, err
	}
	return items[0], nil
}

// createUICustomizationModels appends validated packages atomically, so a
// bundle import either fits the scope quota entirely or creates nothing.
func createUICustomizationModels(scope UICustomizationScope, models []*model.UICustomizationPackageModel) ([]*UICustomizationPackage, error) {
	uiCustomizationMutationMu.Lock()
	defer uiCustomizationMutationMu.Unlock()
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		rows, err := uiCustomizationRows(tx, scope)
		if err != nil {
			return err
		}
		if uiCustomizationUserPackageCount(rows)+len(models) > UICustomizationMaxPackages {
			return ErrUICustomizationLimit
		}
		next := 0
		if len(rows) > 0 {
			next = rows[len(rows)-1].SortOrder + 1
		}
		for _, m := range models {
			m.SortOrder = next
			next++
			if err := tx.Create(m).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := make([]*UICustomizationPackage, 0, len(models))
	for _, m := range models {
		view, err := uiCustomizationView(m)
		if err != nil {
			return nil, err
		}
		result = append(result, view)
	}
	return result, nil
}

func UpdateUICustomization(scope UICustomizationScope, actorID, id string, patch UICustomizationPatch, policy *utils.UICustomizationPolicyConfig) (*UICustomizationPackage, error) {
	if err := uiCustomizationManage(scope, actorID); err != nil {
		return nil, err
	}
	uiCustomizationMutationMu.Lock()
	defer uiCustomizationMutationMu.Unlock()
	var m model.UICustomizationPackageModel
	err := uiCustomizationQuery(model.GetDB(), scope).Where("id = ?", id).Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUICustomizationNotFound
	}
	if err != nil {
		return nil, err
	}
	if m.BuiltinKey != "" && (patch.Name != nil || patch.Description != nil || patch.Surfaces != nil || patch.CSS != nil || patch.JS != nil || patch.ChannelCapabilities != nil) {
		return nil, ErrUICustomizationBuiltin
	}
	if err := applyUICustomizationPatch(&m, patch, policy); err != nil {
		return nil, err
	}
	m.UpdatedBy = actorID
	m.UpdatedAt = time.Now()
	result := uiCustomizationQuery(model.GetDB(), scope).Where("id = ?", id).Updates(map[string]any{
		"name": m.Name, "description": m.Description, "enabled": m.Enabled, "surfaces_json": m.SurfacesJSON,
		"css": m.CSS, "js": m.JS, "channel_capabilities_json": m.ChannelCapabilitiesJSON, "updated_by": m.UpdatedBy, "updated_at": m.UpdatedAt,
	})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrUICustomizationNotFound
	}
	return uiCustomizationView(&m)
}

func DeleteUICustomization(scope UICustomizationScope, actorID, id string) error {
	if err := uiCustomizationManage(scope, actorID); err != nil {
		return err
	}
	uiCustomizationMutationMu.Lock()
	defer uiCustomizationMutationMu.Unlock()
	var builtin model.UICustomizationPackageModel
	if err := uiCustomizationQuery(model.GetDB(), scope).Where("id = ?", id).Take(&builtin).Error; err == nil && builtin.BuiltinKey != "" {
		// Builtins are references to shipped resources. They can be disabled, but
		// cannot be deleted from the platform catalog.
		return ErrUICustomizationBuiltin
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		result := uiCustomizationQuery(tx, scope).Where("id = ?", id).Delete(&model.UICustomizationPackageModel{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrUICustomizationNotFound
		}
		if err := model.ChannelExtensionStorageCleanupTx(tx, "", ChannelExtensionScope{Type: "ui-customization", ID: id}); err != nil {
			return err
		}
		if scope.Type == "platform" {
			if err := tx.Where("platform_package_id = ?", id).Delete(&model.WorldUICustomizationPlatformRefModel{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Reorder requires an exact permutation, so omitted or cross-scope IDs cannot
// silently change the order of a different scope or a concurrently added item.
func ReorderUICustomizations(scope UICustomizationScope, actorID string, ids []string) error {
	if err := uiCustomizationManage(scope, actorID); err != nil {
		return err
	}
	// Builtin references are outside the user-package quota but participate in
	// the exact permutation sent by the manager.
	if len(ids) > UICustomizationMaxPackages+len(builtinUICustomizationRegistrations) {
		return ErrUICustomizationInvalid
	}
	uiCustomizationMutationMu.Lock()
	defer uiCustomizationMutationMu.Unlock()
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		rows, err := uiCustomizationRows(tx, scope)
		if err != nil {
			return err
		}
		if len(rows) != len(ids) {
			return ErrUICustomizationInvalid
		}
		remaining := map[string]bool{}
		for _, m := range rows {
			remaining[m.ID] = true
		}
		for _, id := range ids {
			if !remaining[id] {
				return ErrUICustomizationInvalid
			}
			delete(remaining, id)
		}
		for i, id := range ids {
			if err := uiCustomizationQuery(tx, scope).Where("id = ?", id).Updates(map[string]any{
				"sort_order": i, "updated_by": actorID, "updated_at": time.Now(),
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
