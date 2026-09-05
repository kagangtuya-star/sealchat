package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	builtinassets "sealchat/builtin"
	"sealchat/model"
)

var (
	ErrChannelIFormTemplateMissing  = errors.New("频道嵌入模板不存在")
	ErrChannelIFormTemplateInvalid  = errors.New("频道嵌入模板引用无效")
	ErrChannelIFormTemplateArchived = errors.New("频道嵌入模板已归档")
	ErrChannelIFormTemplateDisabled = errors.New("频道嵌入模板未启用")
)

type BuiltinChannelIFormToolRegistration struct {
	Key              string
	Directory        string
	Name             string
	Description      string
	DefaultWidth     int
	DefaultHeight    int
	DefaultCollapsed bool
	DefaultFloating  bool
	AllowPopout      bool
}

// Keep this list in sync with directories under builtin/channel-embed-tools.
var builtinChannelIFormTools = []BuiltinChannelIFormToolRegistration{
	{
		Key: "theater-dialogue-overlay", Directory: "theater-dialogue-overlay", Name: "小剧场透明角色对话框",
		Description: "独立绑定频道角色的透明对话组件", DefaultWidth: 640, DefaultHeight: 240,
		AllowPopout: true,
	},
	{
		Key: "channel-embed-api-demo", Directory: "channel-embed-api-demo", Name: "Demo测试",
		Description: "SealChat Channel Embed API 多端测试工具", DefaultWidth: 960, DefaultHeight: 760,
		AllowPopout: true,
	},
	{
		Key: "shinobigami-plotboard", Directory: "shinobigami-plotboard", Name: "忍神谋位工具",
		Description: "忍神谋位频道嵌入工具", DefaultWidth: 640, DefaultHeight: 360,
		AllowPopout: true,
	},
}

type ChannelIFormTemplate struct {
	Ref              string
	Origin           string
	Name             string
	Description      string
	URL              string
	EmbedCode        string
	DefaultWidth     int
	DefaultHeight    int
	DefaultCollapsed bool
	DefaultFloating  bool
	AllowPopout      bool
	MediaOptions     model.ChannelIFormMediaOptions
	BridgePolicy     model.ChannelIFormBridgePolicy
	Installable      bool
	Archived         bool
	Enabled          bool
	Editable         bool
	ReadOnly         bool
}

type ChannelIFormTemplateMetadata struct {
	Ref             string
	Origin          string
	Name            string
	Description     string
	Installable     bool
	Archived        bool
	TemplateMissing bool
	TemplateURL     string
}

type BuiltinChannelIFormManifest struct {
	Name             string                         `json:"name"`
	Description      string                         `json:"description"`
	Entry            string                         `json:"entry"`
	DefaultWidth     int                            `json:"defaultWidth"`
	DefaultHeight    int                            `json:"defaultHeight"`
	DefaultCollapsed bool                           `json:"defaultCollapsed"`
	DefaultFloating  bool                           `json:"defaultFloating"`
	AllowPopout      bool                           `json:"allowPopout"`
	MediaOptions     model.ChannelIFormMediaOptions `json:"mediaOptions"`
	BridgePolicy     model.ChannelIFormBridgePolicy `json:"bridgePolicy"`
}

func LoadBuiltinChannelIFormManifest(key string) (BuiltinChannelIFormManifest, error) {
	registration, ok := BuiltinChannelIFormTool(key)
	if !ok {
		return BuiltinChannelIFormManifest{}, ErrChannelIFormTemplateMissing
	}
	manifestPath := filepath.Join(BuiltinChannelIFormRoot(), registration.Directory, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if embedded, embeddedErr := builtinassets.ReadChannelEmbedToolAsset(registration.Directory, "manifest.json"); embeddedErr == nil {
			data = embedded
		} else {
			return BuiltinChannelIFormManifest{
				Name: registration.Name, Description: registration.Description, Entry: "index.html",
				DefaultWidth: registration.DefaultWidth, DefaultHeight: registration.DefaultHeight,
				DefaultCollapsed: registration.DefaultCollapsed, DefaultFloating: registration.DefaultFloating,
				AllowPopout: registration.AllowPopout,
			}, nil
		}
	}
	var manifest BuiltinChannelIFormManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return BuiltinChannelIFormManifest{}, err
	}
	if manifest.Name == "" {
		manifest.Name = registration.Name
	}
	if manifest.Description == "" {
		manifest.Description = registration.Description
	}
	if manifest.Entry == "" {
		manifest.Entry = "index.html"
	}
	if manifest.DefaultWidth <= 0 {
		manifest.DefaultWidth = registration.DefaultWidth
	}
	if manifest.DefaultHeight <= 0 {
		manifest.DefaultHeight = registration.DefaultHeight
	}
	return manifest, nil
}

type ResolvedChannelIForm struct {
	Form     *model.ChannelIFormModel
	Metadata ChannelIFormTemplateMetadata
}

func BuiltinChannelIFormTools() []BuiltinChannelIFormToolRegistration {
	return append([]BuiltinChannelIFormToolRegistration(nil), builtinChannelIFormTools...)
}

func BuiltinChannelIFormTool(key string) (BuiltinChannelIFormToolRegistration, bool) {
	key = strings.TrimSpace(key)
	for _, item := range builtinChannelIFormTools {
		if item.Key == key {
			return item, true
		}
	}
	return BuiltinChannelIFormToolRegistration{}, false
}

func BuiltinChannelIFormRoot() string {
	if configured := strings.TrimSpace(os.Getenv("SEALCHAT_BUILTIN_ROOT")); configured != "" {
		return filepath.Clean(configured)
	}
	candidates := []string{
		filepath.Join("builtin", "channel-embed-tools"),
		filepath.Join("/app", "builtin", "channel-embed-tools"),
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), "builtin", "channel-embed-tools"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return filepath.Clean(candidate)
		}
	}
	return filepath.Join("builtin", "channel-embed-tools")
}

func ParseChannelIFormTemplateRef(ref string) (origin, key string, err error) {
	ref = strings.TrimSpace(ref)
	parts := strings.SplitN(ref, ":", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", ErrChannelIFormTemplateInvalid
	}
	origin, key = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	switch origin {
	case "builtin":
		if strings.ContainsAny(key, `/\\`) || key == "." || key == ".." || strings.Contains(key, "..") {
			return "", "", ErrChannelIFormTemplateInvalid
		}
	case "platform":
		if strings.ContainsAny(key, `/\\`) {
			return "", "", ErrChannelIFormTemplateInvalid
		}
	default:
		return "", "", ErrChannelIFormTemplateInvalid
	}
	return origin, key, nil
}

func ResolveChannelIFormTemplate(ref string) (*ChannelIFormTemplate, error) {
	origin, key, err := ParseChannelIFormTemplateRef(ref)
	if err != nil {
		return nil, err
	}
	switch origin {
	case "builtin":
		item, ok := BuiltinChannelIFormTool(key)
		if !ok {
			return nil, ErrChannelIFormTemplateMissing
		}
		manifest, manifestErr := LoadBuiltinChannelIFormManifest(key)
		if manifestErr != nil {
			return nil, manifestErr
		}
		width, height := manifest.DefaultWidth, manifest.DefaultHeight
		if width <= 0 {
			width = item.DefaultWidth
		}
		if height <= 0 {
			height = item.DefaultHeight
		}
		if width <= 0 {
			width = 640
		}
		if height <= 0 {
			height = 360
		}
		name, description := item.Name, item.Description
		if strings.TrimSpace(manifest.Name) != "" {
			name = manifest.Name
		}
		if strings.TrimSpace(manifest.Description) != "" {
			description = manifest.Description
		}
		entry := filepath.ToSlash(strings.TrimSpace(manifest.Entry))
		cleanEntry := filepath.ToSlash(filepath.Clean(filepath.FromSlash(entry)))
		if entry == "" || filepath.IsAbs(filepath.FromSlash(entry)) || cleanEntry == "." || cleanEntry == ".." || strings.HasPrefix(cleanEntry, "../") || strings.Contains(entry, "\\") {
			cleanEntry = "index.html"
		}
		return &ChannelIFormTemplate{
			Ref: ref, Origin: origin, Name: name, Description: description,
			URL:          fmt.Sprintf("/api/v1/channel-embed-tools/builtin/%s/assets/%s", item.Key, cleanEntry),
			DefaultWidth: width, DefaultHeight: height, DefaultCollapsed: manifest.DefaultCollapsed,
			DefaultFloating: manifest.DefaultFloating, AllowPopout: manifest.AllowPopout,
			MediaOptions: manifest.MediaOptions, BridgePolicy: manifest.BridgePolicy,
			Installable: true, Enabled: true, Editable: false, ReadOnly: true,
		}, nil
	case "platform":
		if model.GetDB() == nil {
			return nil, ErrChannelIFormTemplateMissing
		}
		var item model.ChannelIFormTemplateModel
		if err := model.GetDB().Where("id = ?", key).First(&item).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrChannelIFormTemplateMissing
			}
			return nil, err
		}
		item.Normalize()
		return &ChannelIFormTemplate{
			Ref: ref, Origin: origin, Name: item.Name, Description: item.Description,
			URL: item.Url, EmbedCode: item.EmbedCode, DefaultWidth: item.DefaultWidth,
			DefaultHeight: item.DefaultHeight, DefaultCollapsed: item.DefaultCollapsed,
			DefaultFloating: item.DefaultFloating, AllowPopout: item.AllowPopout,
			MediaOptions: item.MediaOptions, BridgePolicy: item.BridgePolicy,
			Installable: item.Enabled && !item.Archived, Archived: item.Archived,
			Enabled: item.Enabled, Editable: true, ReadOnly: false,
		}, nil
	default:
		return nil, ErrChannelIFormTemplateInvalid
	}
}

func applyChannelIFormTemplateOverrides(form *model.ChannelIFormModel, overrides model.ChannelIFormTemplateOverrides) {
	if overrides.Name != nil {
		form.Name = *overrides.Name
	}
	if overrides.DefaultWidth != nil {
		form.DefaultWidth = *overrides.DefaultWidth
	}
	if overrides.DefaultHeight != nil {
		form.DefaultHeight = *overrides.DefaultHeight
	}
	if overrides.DefaultCollapsed != nil {
		form.DefaultCollapsed = *overrides.DefaultCollapsed
	}
	if overrides.DefaultFloating != nil {
		form.DefaultFloating = *overrides.DefaultFloating
	}
	if overrides.AllowPopout != nil {
		form.AllowPopout = *overrides.AllowPopout
	}
	if overrides.MediaOptions != nil {
		form.MediaOptions = *overrides.MediaOptions
	}
	if overrides.BridgePolicy != nil {
		form.BridgePolicy = *overrides.BridgePolicy
	}
}

func resolveStoredChannelIForm(raw *model.ChannelIFormModel, template *ChannelIFormTemplate, missing bool) *model.ChannelIFormModel {
	if raw == nil {
		return nil
	}
	if strings.TrimSpace(raw.TemplateRef) == "" || template == nil {
		result := *raw
		if missing {
			result.Url = ""
			result.EmbedCode = ""
		}
		return &result
	}
	result := *raw
	result.Name = template.Name
	result.Url = template.URL
	result.EmbedCode = template.EmbedCode
	result.DefaultWidth = template.DefaultWidth
	result.DefaultHeight = template.DefaultHeight
	result.DefaultCollapsed = template.DefaultCollapsed
	result.DefaultFloating = template.DefaultFloating
	result.AllowPopout = template.AllowPopout
	result.MediaOptions = template.MediaOptions
	result.BridgePolicy = template.BridgePolicy
	applyChannelIFormTemplateOverrides(&result, raw.TemplateOverrides)
	result.TemplateRef = raw.TemplateRef
	result.TemplateOverrides = raw.TemplateOverrides
	return &result
}

func ResolveChannelIForm(raw *model.ChannelIFormModel) (*ResolvedChannelIForm, error) {
	if raw == nil {
		return nil, nil
	}
	ref := strings.TrimSpace(raw.TemplateRef)
	if ref == "" {
		return &ResolvedChannelIForm{Form: resolveStoredChannelIForm(raw, nil, false)}, nil
	}
	template, err := ResolveChannelIFormTemplate(ref)
	if err != nil {
		if errors.Is(err, ErrChannelIFormTemplateMissing) || errors.Is(err, ErrChannelIFormTemplateInvalid) {
			return &ResolvedChannelIForm{Form: resolveStoredChannelIForm(raw, nil, true), Metadata: ChannelIFormTemplateMetadata{
				Ref: ref, Origin: strings.SplitN(ref, ":", 2)[0], TemplateMissing: true,
			}}, nil
		}
		return nil, err
	}
	return &ResolvedChannelIForm{Form: resolveStoredChannelIForm(raw, template, false), Metadata: ChannelIFormTemplateMetadata{
		Ref: ref, Origin: template.Origin, Name: template.Name, Description: template.Description,
		Installable: template.Installable, Archived: template.Archived,
		TemplateURL: template.URL,
	}}, nil
}

func ResolveChannelIForms(raw []*model.ChannelIFormModel) ([]*ResolvedChannelIForm, error) {
	platformIDs := make([]string, 0)
	seenPlatform := map[string]struct{}{}
	for _, item := range raw {
		if item == nil {
			continue
		}
		origin, key, err := ParseChannelIFormTemplateRef(item.TemplateRef)
		if err != nil || origin != "platform" {
			continue
		}
		if _, ok := seenPlatform[key]; ok {
			continue
		}
		seenPlatform[key] = struct{}{}
		platformIDs = append(platformIDs, key)
	}
	platforms := map[string]ChannelIFormTemplate{}
	builtins := map[string]ChannelIFormTemplate{}
	if len(platformIDs) > 0 {
		if model.GetDB() == nil {
			platformIDs = nil
		}
	}
	if len(platformIDs) > 0 {
		var items []model.ChannelIFormTemplateModel
		if err := model.GetDB().Where("id IN ?", platformIDs).Find(&items).Error; err != nil {
			return nil, err
		}
		for _, item := range items {
			item.Normalize()
			platforms[item.ID] = ChannelIFormTemplate{
				Ref: "platform:" + item.ID, Origin: "platform", Name: item.Name, Description: item.Description,
				URL: item.Url, EmbedCode: item.EmbedCode, DefaultWidth: item.DefaultWidth, DefaultHeight: item.DefaultHeight,
				DefaultCollapsed: item.DefaultCollapsed, DefaultFloating: item.DefaultFloating, AllowPopout: item.AllowPopout,
				MediaOptions: item.MediaOptions, BridgePolicy: item.BridgePolicy,
				Installable: item.Enabled && !item.Archived, Archived: item.Archived, Enabled: item.Enabled,
				Editable: true,
			}
		}
	}
	result := make([]*ResolvedChannelIForm, 0, len(raw))
	for _, item := range raw {
		if item == nil {
			continue
		}
		if strings.TrimSpace(item.TemplateRef) == "" {
			result = append(result, &ResolvedChannelIForm{Form: resolveStoredChannelIForm(item, nil, false)})
			continue
		}
		origin, key, err := ParseChannelIFormTemplateRef(item.TemplateRef)
		if err != nil {
			result = append(result, &ResolvedChannelIForm{Form: resolveStoredChannelIForm(item, nil, true), Metadata: ChannelIFormTemplateMetadata{
				Ref: item.TemplateRef, TemplateMissing: true,
			}})
			continue
		}
		var template *ChannelIFormTemplate
		if origin == "platform" {
			if value, ok := platforms[key]; ok {
				template = &value
			}
		} else {
			if value, ok := builtins[item.TemplateRef]; ok {
				template = &value
			} else {
				value, resolveErr := ResolveChannelIFormTemplate(item.TemplateRef)
				if resolveErr != nil && !errors.Is(resolveErr, ErrChannelIFormTemplateMissing) {
					return nil, resolveErr
				}
				if value != nil {
					builtins[item.TemplateRef] = *value
				}
				template = value
			}
		}
		if template == nil {
			result = append(result, &ResolvedChannelIForm{Form: resolveStoredChannelIForm(item, nil, true), Metadata: ChannelIFormTemplateMetadata{
				Ref: item.TemplateRef, Origin: origin, TemplateMissing: true,
			}})
			continue
		}
		result = append(result, &ResolvedChannelIForm{Form: resolveStoredChannelIForm(item, template, false), Metadata: ChannelIFormTemplateMetadata{
			Ref: item.TemplateRef, Origin: template.Origin, Name: template.Name, Description: template.Description,
			Installable: template.Installable, Archived: template.Archived, TemplateURL: template.URL,
		}})
	}
	return result, nil
}
