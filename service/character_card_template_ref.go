package service

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"sealchat/model"
)

const PlatformCharacterCardTemplateRefPrefix = "platform:"

type CharacterCardTemplateRefSource string

const (
	CharacterCardTemplateRefSourceUser     CharacterCardTemplateRefSource = "user"
	CharacterCardTemplateRefSourcePlatform CharacterCardTemplateRefSource = "platform"
)

type CharacterCardTemplateRef struct {
	Source CharacterCardTemplateRefSource
	ID     string
	Ref    string
}

type CharacterCardTemplateResolution struct {
	Ref                        string
	Source                     CharacterCardTemplateRefSource
	ID                         string
	Exists                     bool
	Enabled                    bool
	Name                       string
	SheetType                  string
	Content                    string
	BadgeTemplateOverride      string
	TheaterOverlayTemplateJSON string
	UserTemplate               *model.CharacterCardTemplateModel
	PlatformTemplate           *model.PlatformCharacterCardTemplateModel
}

func IsPlatformCharacterCardTemplateRef(value string) bool {
	parsed, ok := ParseCharacterCardTemplateRef(value)
	return ok && parsed.Source == CharacterCardTemplateRefSourcePlatform
}

func ParseCharacterCardTemplateRef(value string) (CharacterCardTemplateRef, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return CharacterCardTemplateRef{}, false
	}
	if strings.HasPrefix(trimmed, PlatformCharacterCardTemplateRefPrefix) {
		id := strings.TrimSpace(strings.TrimPrefix(trimmed, PlatformCharacterCardTemplateRefPrefix))
		if id == "" || strings.Contains(id, ":") {
			return CharacterCardTemplateRef{}, false
		}
		return CharacterCardTemplateRef{Source: CharacterCardTemplateRefSourcePlatform, ID: id, Ref: PlatformCharacterCardTemplateRefPrefix + id}, true
	}
	return CharacterCardTemplateRef{Source: CharacterCardTemplateRefSourceUser, ID: trimmed, Ref: trimmed}, true
}

func ResolveCharacterCardTemplateRef(ref string) (*CharacterCardTemplateResolution, error) {
	parsed, ok := ParseCharacterCardTemplateRef(ref)
	if !ok {
		return &CharacterCardTemplateResolution{Ref: strings.TrimSpace(ref)}, nil
	}
	resolution := &CharacterCardTemplateResolution{Ref: parsed.Ref, Source: parsed.Source, ID: parsed.ID}
	if parsed.Source == CharacterCardTemplateRefSourcePlatform {
		platformTemplate, err := model.PlatformCharacterCardTemplateGetByID(parsed.ID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return resolution, nil
			}
			return nil, err
		}
		resolution.Exists = true
		resolution.Enabled = platformTemplate.Enabled
		resolution.Name = platformTemplate.Name
		resolution.SheetType = platformTemplate.SheetType
		resolution.Content = platformTemplate.Content
		resolution.BadgeTemplateOverride = platformTemplate.BadgeTemplateOverride
		resolution.TheaterOverlayTemplateJSON = platformTemplate.TheaterOverlayTemplateJSON
		resolution.PlatformTemplate = platformTemplate
		return resolution, nil
	}
	userTemplate, err := model.CharacterCardTemplateGetByID(parsed.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return resolution, nil
		}
		return nil, err
	}
	resolution.Exists = true
	resolution.Enabled = true
	resolution.Name = userTemplate.Name
	resolution.SheetType = userTemplate.SheetType
	resolution.Content = userTemplate.Content
	resolution.BadgeTemplateOverride = userTemplate.DefaultBadgeTemplate
	resolution.UserTemplate = userTemplate
	return resolution, nil
}

func ResolveCharacterCardTemplateContent(ref string) (string, *CharacterCardTemplateResolution, error) {
	resolution, err := ResolveCharacterCardTemplateRef(ref)
	if err != nil {
		return "", nil, err
	}
	if resolution == nil || !resolution.Exists {
		return "", resolution, nil
	}
	return resolution.Content, resolution, nil
}
