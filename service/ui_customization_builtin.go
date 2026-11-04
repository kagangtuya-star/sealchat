package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	uicustomizationpackage "sealchat/doc/ui-customization-package"
	"sealchat/model"
	"sealchat/utils"
)

// Builtin UI packages are referenced like channel embeds: the durable value is
// builtin:<key>, while the package document remains a shipped server resource.
// This keeps enablement/order in the database and lets a newer server refresh
// the CSS/JS without asking the browser to carry a copy of the theme.
const BuiltinUICustomizationRefPrefix = "builtin:"

type BuiltinUICustomizationRegistration struct {
	Key      string
	FileName string
}

var builtinUICustomizationRegistrations = []BuiltinUICustomizationRegistration{
	{Key: "rlyeh-archive", FileName: "sealchat-ui-package-rlyeh-archive.json"},
	{Key: "triangle-agency", FileName: "sealchat-ui-package-triangle-agency.json"},
}

var (
	builtinUICustomizationCacheMu sync.RWMutex
	builtinUICustomizationCache   = map[string]*builtinUICustomizationPackage{}
)

type builtinUICustomizationDocument struct {
	Format            string `json:"format"`
	FormatVersion     int    `json:"formatVersion"`
	RuntimeAPIVersion int    `json:"runtimeApiVersion"`
	Manifest          struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Version     string `json:"version"`
	} `json:"manifest"`
	Targets []UISurface `json:"targets"`
	CSS     string      `json:"css"`
	JS      string      `json:"js"`
}

type builtinUICustomizationPackage struct {
	Key         string
	Ref         string
	Version     string
	Name        string
	Description string
	Targets     []UISurface
	CSS         string
	JS          string
}

func BuiltinUICustomizationRegistrations() []BuiltinUICustomizationRegistration {
	return append([]BuiltinUICustomizationRegistration(nil), builtinUICustomizationRegistrations...)
}

func ParseBuiltinUICustomizationRef(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, BuiltinUICustomizationRefPrefix) {
		return "", false
	}
	key := strings.TrimPrefix(value, BuiltinUICustomizationRefPrefix)
	if key == "" || strings.ContainsAny(key, `/\\`) || strings.Contains(key, "..") {
		return "", false
	}
	return key, true
}

// BuiltinUICustomizationRoot returns an explicit development override only.
// Normal production builds use the embedded package documents so moving or
// copying the SealChat binary cannot make startup depend on doc files.
func BuiltinUICustomizationRoot() string {
	if configured := strings.TrimSpace(os.Getenv("SEALCHAT_UI_CUSTOMIZATION_ROOT")); configured != "" {
		return filepath.Clean(configured)
	}
	return ""
}

func readBuiltinUICustomizationDocument(fileName string) ([]byte, error) {
	if root := BuiltinUICustomizationRoot(); root != "" {
		return os.ReadFile(filepath.Join(root, fileName))
	}
	return uicustomizationpackage.ReadFile(fileName)
}

func currentUICustomizationPolicy() *utils.UICustomizationPolicyConfig {
	if cfg := utils.GetConfig(); cfg != nil {
		return cfg.UICustomization
	}
	return nil
}

func validateBuiltinUICustomizationCodeSize(key, css, js string) error {
	if err := validateUICustomizationCodeSize(css, js, currentUICustomizationPolicy()); err != nil {
		return fmt.Errorf("内置界面主题 %s 超出代码大小限制：%s", key, strings.TrimPrefix(err.Error(), ErrUICustomizationInvalid.Error()+": "))
	}
	return nil
}

func loadBuiltinUICustomizationPackage(registration BuiltinUICustomizationRegistration) (*builtinUICustomizationPackage, error) {
	data, err := readBuiltinUICustomizationDocument(registration.FileName)
	if err != nil {
		return nil, fmt.Errorf("读取内置界面主题 %s: %w", registration.Key, err)
	}
	var document builtinUICustomizationDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("解析内置界面主题 %s: %w", registration.Key, err)
	}
	if document.Format != UICustomizationPackageFormat || document.FormatVersion != UICustomizationFormatVersion || document.RuntimeAPIVersion < 1 || document.RuntimeAPIVersion > UICustomizationRuntimeAPIVersion {
		return nil, fmt.Errorf("内置界面主题 %s 的包格式不受支持", registration.Key)
	}
	name := strings.TrimSpace(document.Manifest.Name)
	if name == "" {
		return nil, fmt.Errorf("内置界面主题 %s 缺少名称", registration.Key)
	}
	if len(document.Targets) == 0 || len(document.Targets) > 32 {
		return nil, fmt.Errorf("内置界面主题 %s 的 Surface 无效", registration.Key)
	}
	seen := map[UISurface]bool{}
	targets := make([]UISurface, 0, len(document.Targets))
	for _, surface := range document.Targets {
		if !surface.Valid() || !seen[surface] {
			if !surface.Valid() {
				return nil, fmt.Errorf("内置界面主题 %s 的 Surface 无效", registration.Key)
			}
			targets = append(targets, surface)
			seen[surface] = true
		}
	}
	if err := validateBuiltinUICustomizationCodeSize(registration.Key, document.CSS, document.JS); err != nil {
		return nil, err
	}
	version := strings.TrimSpace(document.Manifest.Version)
	if version == "" {
		sum := sha256.Sum256(data)
		version = hex.EncodeToString(sum[:])
	}
	return &builtinUICustomizationPackage{
		Key: registration.Key, Ref: BuiltinUICustomizationRefPrefix + registration.Key,
		Version: version, Name: name, Description: strings.TrimSpace(document.Manifest.Description),
		Targets: targets, CSS: document.CSS, JS: document.JS,
	}, nil
}

func builtinUICustomizationForKey(key string) (*builtinUICustomizationPackage, error) {
	key = strings.TrimSpace(key)
	builtinUICustomizationCacheMu.RLock()
	item := builtinUICustomizationCache[key]
	builtinUICustomizationCacheMu.RUnlock()
	if item != nil {
		if err := validateBuiltinUICustomizationCodeSize(item.Key, item.CSS, item.JS); err != nil {
			return nil, err
		}
		return item, nil
	}
	for _, registration := range builtinUICustomizationRegistrations {
		if registration.Key != key {
			continue
		}
		loaded, err := loadBuiltinUICustomizationPackage(registration)
		if err != nil {
			return nil, err
		}
		builtinUICustomizationCacheMu.Lock()
		builtinUICustomizationCache[key] = loaded
		builtinUICustomizationCacheMu.Unlock()
		return loaded, nil
	}
	return nil, fmt.Errorf("未知的内置界面主题: %s", key)
}

func loadBuiltinUICustomizationPackages() ([]*builtinUICustomizationPackage, error) {
	packages := make([]*builtinUICustomizationPackage, 0, len(builtinUICustomizationRegistrations))
	for _, registration := range builtinUICustomizationRegistrations {
		item, err := builtinUICustomizationForKey(registration.Key)
		if err != nil {
			return nil, err
		}
		packages = append(packages, item)
	}
	return packages, nil
}

// builtinUICustomizationRowID keeps platform and world references distinct
// while leaving the public Ref (builtin:<key>) stable. The short world hash
// avoids putting a potentially long world ID into a primary key.
func builtinUICustomizationRowID(scope UICustomizationScope, key string) string {
	if scope.Type == "platform" {
		return BuiltinUICustomizationRefPrefix + key
	}
	sum := sha256.Sum256([]byte(scope.ID))
	return fmt.Sprintf("%sworld-%x-%s", BuiltinUICustomizationRefPrefix, sum[:8], key)
}

func registerBuiltinUICustomizationsForScope(tx *gorm.DB, scope UICustomizationScope, packages []*builtinUICustomizationPackage) error {
	rows, err := uiCustomizationRows(tx, scope)
	if err != nil {
		return err
	}
	nextSortOrder := 0
	if len(rows) > 0 {
		nextSortOrder = rows[len(rows)-1].SortOrder + 1
	}
	for _, item := range packages {
		var row model.UICustomizationPackageModel
		err := uiCustomizationQuery(tx, scope).Where("builtin_key = ?", item.Key).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			row = model.UICustomizationPackageModel{StringPKBaseModel: model.StringPKBaseModel{ID: builtinUICustomizationRowID(scope, item.Key)},
				ScopeType: scope.Type, ScopeID: scope.ID, Name: item.Name, Description: item.Description, Enabled: false,
				SortOrder: nextSortOrder, BuiltinKey: item.Key, BuiltinVersion: item.Version,
				CreatedBy: "builtin", UpdatedBy: "builtin"}
			nextSortOrder++
			data, marshalErr := json.Marshal(item.Targets)
			if marshalErr != nil {
				return marshalErr
			}
			row.SurfacesJSON = string(data)
			row.CreatedAt = time.Now()
			row.UpdatedAt = row.CreatedAt
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if row.BuiltinKey != item.Key {
			return fmt.Errorf("内置界面主题引用冲突: %s", item.Ref)
		}
		data, marshalErr := json.Marshal(item.Targets)
		if marshalErr != nil {
			return marshalErr
		}
		if row.BuiltinVersion == item.Version && row.Name == item.Name && row.Description == item.Description && row.SurfacesJSON == string(data) && row.CSS == "" && row.JS == "" {
			continue
		}
		if err := tx.Model(&model.UICustomizationPackageModel{}).Where("id = ?", row.ID).Updates(map[string]any{
			"name": item.Name, "description": item.Description, "surfaces_json": string(data),
			"css": "", "js": "", "builtin_version": item.Version,
			"updated_by": "builtin", "updated_at": time.Now(),
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

// ensureBuiltinUICustomizations makes builtins available in a scope without
// copying their CSS/JS into the database. It is also called lazily for worlds
// created after process startup.
func ensureBuiltinUICustomizations(scope UICustomizationScope) error {
	packages, err := loadBuiltinUICustomizationPackages()
	if err != nil {
		return err
	}
	uiCustomizationMutationMu.Lock()
	defer uiCustomizationMutationMu.Unlock()
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		return registerBuiltinUICustomizationsForScope(tx, scope, packages)
	})
}

// EnsureBuiltinUICustomizations authorizes and materializes world references
// for management endpoints. It is intentionally separate from List so service
// callers that only inspect user-created packages do not get an implicit write.
func EnsureBuiltinUICustomizations(scope UICustomizationScope, actorID string) error {
	if err := uiCustomizationManage(scope, actorID); err != nil {
		return err
	}
	if scope.Type != "world" {
		return nil
	}
	return ensureBuiltinUICustomizations(scope)
}

// RegisterBuiltinUICustomizations creates the stable reference rows once and
// refreshes their canonical content on every startup. The same reference rows
// are created for active worlds, so world administrators can opt into a theme
// without copying its source. Existing Enabled and SortOrder values are
// deliberately preserved; a shipped theme never becomes active merely because
// the server was upgraded.
func RegisterBuiltinUICustomizations() error {
	packages := make([]*builtinUICustomizationPackage, 0, len(builtinUICustomizationRegistrations))
	for _, registration := range builtinUICustomizationRegistrations {
		item, err := loadBuiltinUICustomizationPackage(registration)
		if err != nil {
			return err
		}
		packages = append(packages, item)
	}

	uiCustomizationMutationMu.Lock()
	defer uiCustomizationMutationMu.Unlock()
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := registerBuiltinUICustomizationsForScope(tx, UICustomizationScope{Type: "platform"}, packages); err != nil {
			return err
		}
		var worldIDs []string
		if err := tx.Model(&model.WorldModel{}).Where("status = ?", "active").Pluck("id", &worldIDs).Error; err != nil {
			return err
		}
		for _, worldID := range worldIDs {
			if err := registerBuiltinUICustomizationsForScope(tx, UICustomizationScope{Type: "world", ID: worldID}, packages); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	builtinUICustomizationCacheMu.Lock()
	for _, item := range packages {
		builtinUICustomizationCache[item.Key] = item
	}
	builtinUICustomizationCacheMu.Unlock()
	return nil
}
