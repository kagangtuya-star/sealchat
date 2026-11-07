package service

import (
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/utils"
)

type ChannelExtensionScope = model.ChannelExtensionScope

var ChannelExtensionCapabilities = []string{"context.read", "user.read", "members.read", "world.admins.read", "characters.read", "characterCard.read", "characterCard.write", "permissions.read", "storage.read", "storage.write", "events.subscribe", "events.publish", "messages.send", "attachments.upload", "theater.dialogue.subscribe", "theater.character.read"}

func HasChannelExtensionCapability(capabilities []string, expected string) bool {
	for _, capability := range capabilities {
		if strings.TrimSpace(capability) == expected {
			return true
		}
	}
	return false
}

func ChannelExtensionWriteCapability(capability string) bool {
	return capability == "storage.write" || capability == "events.publish" || capability == "messages.send" || capability == "characterCard.write" || capability == "attachments.upload"
}

func ChannelExtensionRequiresTextSendPermission(capability string) bool {
	return capability == "storage.write" || capability == "events.publish" || capability == "messages.send"
}

func uiCustomizationChannelCapabilities(m *model.UICustomizationPackageModel) []string {
	capabilities := []string{}
	if m.ChannelCapabilitiesJSON != "" {
		_ = json.Unmarshal([]byte(m.ChannelCapabilitiesJSON), &capabilities)
	}
	if capabilities == nil {
		return []string{}
	}
	return capabilities
}

type ResolvedChannelExtensionScope struct {
	Scope        ChannelExtensionScope
	ChannelID    string
	WorldID      string
	Capabilities []string
	Form         *model.ChannelIFormModel
}

// GC notifications start with a physical key; only package IDs with a matching
// hash may recover a customization scope. The key itself is never a public ID.
func ChannelExtensionScopeForNamespace(key string) (ChannelExtensionScope, bool) {
	if !strings.HasPrefix(key, "uic_") {
		return ChannelExtensionScope{Type: "iform", ID: key}, true
	}
	var packages []model.UICustomizationPackageModel
	if err := model.GetDB().Select("id").Find(&packages).Error; err != nil {
		return ChannelExtensionScope{}, false
	}
	for _, pkg := range packages {
		scope := ChannelExtensionScope{Type: "ui-customization", ID: pkg.ID}
		if scope.StorageNamespaceKey() == key {
			return scope, true
		}
	}
	return ChannelExtensionScope{}, false
}

// ResolveChannelExtensionScope uses saved policy/package state, never client package metadata.
func ResolveChannelExtensionScope(actorID, channelID string, scope ChannelExtensionScope, capability string, readOnly bool, policy *utils.UICustomizationPolicyConfig) (*ResolvedChannelExtensionScope, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" || len(channelID) > 100 || !scope.Valid() {
		return nil, errors.New("INVALID_PARAMS: missing channel_id or scope")
	}
	if actorID == "" || !CanReadChannelByUserId(actorID, channelID) {
		return nil, errors.New("PERMISSION_DENIED")
	}
	channel, err := model.ChannelGet(channelID)
	if err != nil || channel == nil {
		return nil, errors.New("NOT_FOUND: channel")
	}
	if ChannelExtensionWriteCapability(capability) && readOnly {
		return nil, errors.New("PERMISSION_DENIED")
	}
	if ChannelExtensionRequiresTextSendPermission(capability) && !pm.CanWithChannelRole(actorID, channelID, pm.PermFuncChannelTextSend, pm.PermFuncChannelTextSendAll) {
		return nil, errors.New("PERMISSION_DENIED")
	}
	result := &ResolvedChannelExtensionScope{Scope: scope, ChannelID: channelID, WorldID: channel.WorldID}
	switch scope.Type {
	case "iform":
		local, err := model.ChannelIFormGet(channelID, scope.ID)
		if err != nil {
			return nil, err
		}
		if local != nil {
			resolved, err := ResolveChannelIForm(local)
			if err != nil {
				return nil, err
			}
			result.Form = local
			if resolved != nil {
				if resolved.Metadata.TemplateMissing {
					return nil, errors.New("NOT_FOUND: form template")
				}
				result.Form = resolved.Form
			}
		}
		if result.Form == nil {
			forms, err := ListEffectiveChannelIForms(channelID)
			if err != nil {
				return nil, err
			}
			for _, item := range forms {
				if item != nil && item.ID == scope.ID {
					if item.TemplateMissing {
						return nil, errors.New("NOT_FOUND: form template")
					}
					result.Form = item.ChannelIFormModel
					break
				}
			}
		}
		if result.Form == nil {
			return nil, errors.New("NOT_FOUND: form")
		}
		if !result.Form.BridgePolicy.Enabled {
			return nil, errors.New("CAPABILITY_DENIED")
		}
		result.Capabilities = result.Form.BridgePolicy.Capabilities
	case "ui-customization":
		var pkg model.UICustomizationPackageModel
		if err := model.GetDB().Where("id = ?", scope.ID).Take(&pkg).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("NOT_FOUND: package")
			}
			return nil, err
		}
		if !pkg.Enabled || pkg.JS == "" || pkg.BuiltinKey != "" {
			return nil, errors.New("CAPABILITY_DENIED")
		}
		if channel.WorldID == "" {
			return nil, errors.New("NOT_FOUND: channel")
		}
		if pkg.ScopeType == "world" {
			if pkg.ScopeID != channel.WorldID || !utils.NormalizeUICustomizationPolicy(policy).AllowWorldJS || !utils.NormalizeUICustomizationPolicy(policy).AllowWorldCSS || !IsWorldMember(channel.WorldID, actorID) {
				return nil, errors.New("CAPABILITY_DENIED")
			}
		} else if pkg.ScopeType != "platform" || pkg.ScopeID != "" {
			return nil, errors.New("CAPABILITY_DENIED")
		}
		world, err := GetWorldByID(channel.WorldID)
		if err != nil || world.Status != "active" {
			return nil, errors.New("NOT_FOUND: world")
		}
		result.Capabilities = uiCustomizationChannelCapabilities(&pkg)
	}
	if !HasChannelExtensionCapability(result.Capabilities, capability) {
		return nil, errors.New("CAPABILITY_DENIED")
	}
	return result, nil
}
