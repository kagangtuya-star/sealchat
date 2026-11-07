package service

import (
	"bytes"
	"encoding/json"
	"fmt"

	"sealchat/model"
	"sealchat/utils"
)

// Versioned UI customization transfer documents. Exports are produced by the
// client from management data; imports are validated here so they go through
// the same permission, quota, Surface and size rules as manual creation.
const (
	UICustomizationPackageFormat     = "sealchat.ui-package"
	UICustomizationBundleFormat      = "sealchat.ui-bundle"
	UICustomizationFormatVersion     = 1
	UICustomizationRuntimeAPIVersion = 2
)

type uiCustomizationTransferHeader struct {
	Format            string `json:"format"`
	FormatVersion     *int   `json:"formatVersion"`
	RuntimeAPIVersion *int   `json:"runtimeApiVersion"`
}

type uiCustomizationTransferPackage struct {
	uiCustomizationTransferHeader
	Manifest struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"manifest"`
	Targets             []UISurface `json:"targets"`
	CSS                 string      `json:"css"`
	JS                  string      `json:"js"`
	ChannelCapabilities []string    `json:"channelCapabilities"`
}

type uiCustomizationTransferBundle struct {
	uiCustomizationTransferHeader
	Packages []json.RawMessage `json:"packages"`
}

func checkUICustomizationVersions(h uiCustomizationTransferHeader, inheritedFormat *int) error {
	version := h.FormatVersion
	if version == nil {
		version = inheritedFormat
	}
	if version == nil {
		return uiCustomizationInvalid("缺少 formatVersion")
	}
	if *version != UICustomizationFormatVersion {
		return uiCustomizationInvalid(fmt.Sprintf("不支持的 formatVersion %d，当前仅支持 %d", *version, UICustomizationFormatVersion))
	}
	if h.RuntimeAPIVersion != nil && (*h.RuntimeAPIVersion < 1 || *h.RuntimeAPIVersion > UICustomizationRuntimeAPIVersion) {
		return uiCustomizationInvalid(fmt.Sprintf("包需要脚本 API 版本 %d，当前 SealChat 仅支持 %d", *h.RuntimeAPIVersion, UICustomizationRuntimeAPIVersion))
	}
	return nil
}

func decodeUICustomizationTransfer(raw []byte, v any) error {
	if err := json.Unmarshal(raw, v); err != nil {
		return uiCustomizationInvalid("文件不是有效的界面自定义 JSON")
	}
	return nil
}

func parseUICustomizationImport(raw []byte) ([]uiCustomizationTransferPackage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return nil, uiCustomizationInvalid("文件不是有效的界面自定义 JSON")
	}
	var header uiCustomizationTransferHeader
	if err := decodeUICustomizationTransfer(raw, &header); err != nil {
		return nil, err
	}
	switch header.Format {
	case UICustomizationPackageFormat:
		var item uiCustomizationTransferPackage
		if err := decodeUICustomizationTransfer(raw, &item); err != nil {
			return nil, err
		}
		if err := checkUICustomizationVersions(item.uiCustomizationTransferHeader, nil); err != nil {
			return nil, err
		}
		return []uiCustomizationTransferPackage{item}, nil
	case UICustomizationBundleFormat:
		var bundle uiCustomizationTransferBundle
		if err := decodeUICustomizationTransfer(raw, &bundle); err != nil {
			return nil, err
		}
		if err := checkUICustomizationVersions(bundle.uiCustomizationTransferHeader, nil); err != nil {
			return nil, err
		}
		if len(bundle.Packages) == 0 {
			return nil, uiCustomizationInvalid("Bundle 中没有包")
		}
		if len(bundle.Packages) > UICustomizationMaxPackages {
			return nil, ErrUICustomizationLimit
		}
		items := make([]uiCustomizationTransferPackage, 0, len(bundle.Packages))
		for i, data := range bundle.Packages {
			var item uiCustomizationTransferPackage
			if err := json.Unmarshal(data, &item); err != nil {
				return nil, uiCustomizationInvalid(fmt.Sprintf("第 %d 个包不是有效的 JSON 对象", i+1))
			}
			if item.Format != "" && item.Format != UICustomizationPackageFormat {
				return nil, uiCustomizationInvalid(fmt.Sprintf("第 %d 个包的 format 应为 %s", i+1, UICustomizationPackageFormat))
			}
			if item.RuntimeAPIVersion == nil {
				item.RuntimeAPIVersion = bundle.RuntimeAPIVersion
			}
			if err := checkUICustomizationVersions(item.uiCustomizationTransferHeader, bundle.FormatVersion); err != nil {
				return nil, fmt.Errorf("第 %d 个包：%w", i+1, err)
			}
			items = append(items, item)
		}
		return items, nil
	case "":
		return nil, uiCustomizationInvalid("缺少 format 字段，不是 SealChat 界面自定义包")
	default:
		return nil, uiCustomizationInvalid(fmt.Sprintf("不支持的 format %q，应为 %s 或 %s", header.Format, UICustomizationPackageFormat, UICustomizationBundleFormat))
	}
}

// ReplacePlatformUICustomizationFromImport replaces one platform package in
// place. The database ID and enablement stay unchanged, so world references
// keep following the same package after an uploaded update.
func ReplacePlatformUICustomizationFromImport(actorID, id string, raw []byte, policy *utils.UICustomizationPolicyConfig) (*UICustomizationPackage, error) {
	if err := uiCustomizationManage(UICustomizationScope{Type: "platform"}, actorID); err != nil {
		return nil, err
	}
	items, err := parseUICustomizationImport(raw)
	if err != nil {
		return nil, err
	}
	if len(items) != 1 {
		return nil, uiCustomizationInvalid("覆盖更新仅支持单个 sealchat.ui-package")
	}
	item := &items[0]
	return UpdateUICustomization(UICustomizationScope{Type: "platform"}, actorID, id, UICustomizationPatch{
		Name: &item.Manifest.Name, Description: &item.Manifest.Description,
		Surfaces: &item.Targets, CSS: &item.CSS, JS: &item.JS, ChannelCapabilities: &item.ChannelCapabilities,
	}, policy)
}

// ImportUICustomizations creates new copies in the target scope. File IDs are
// never used to overwrite existing packages. Imported packages start disabled
// so administrators can review CSS/JS before it reaches any page.
func ImportUICustomizations(scope UICustomizationScope, actorID string, raw []byte, policy *utils.UICustomizationPolicyConfig) ([]*UICustomizationPackage, error) {
	if err := uiCustomizationManage(scope, actorID); err != nil {
		return nil, err
	}
	items, err := parseUICustomizationImport(raw)
	if err != nil {
		return nil, err
	}
	models := make([]*model.UICustomizationPackageModel, 0, len(items))
	disabled := false
	for i := range items {
		item := &items[i]
		m := &model.UICustomizationPackageModel{ScopeType: scope.Type, ScopeID: scope.ID, CreatedBy: actorID, UpdatedBy: actorID}
		patch := UICustomizationPatch{
			Name: &item.Manifest.Name, Description: &item.Manifest.Description, Enabled: &disabled,
			Surfaces: &item.Targets, CSS: &item.CSS, JS: &item.JS, ChannelCapabilities: &item.ChannelCapabilities,
		}
		if err := applyUICustomizationPatch(m, patch, policy); err != nil {
			if len(items) > 1 {
				return nil, fmt.Errorf("第 %d 个包：%w", i+1, err)
			}
			return nil, err
		}
		models = append(models, m)
	}
	return createUICustomizationModels(scope, models)
}
