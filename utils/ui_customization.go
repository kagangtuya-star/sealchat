package utils

import (
	"errors"
	"fmt"
)

const DefaultUICustomizationMaxCodeSizeKB = 1024

const (
	MinUICustomizationMaxCodeSizeKB = 64
	MaxUICustomizationMaxCodeSizeKB = 16384
)

type UICustomizationPolicyConfig struct {
	AllowWorldCSS       bool `json:"allowWorldCss" yaml:"allowWorldCss"`
	AllowWorldJS        bool `json:"allowWorldJs" yaml:"allowWorldJs"`
	AllowPublicWorldCSS bool `json:"allowPublicWorldCss" yaml:"allowPublicWorldCss"`
	MaxCodeSizeKB       int  `json:"maxCodeSizeKB" yaml:"maxCodeSizeKB"`
}

func NormalizeUICustomizationPolicy(cfg *UICustomizationPolicyConfig) *UICustomizationPolicyConfig {
	if cfg == nil {
		// World scripts are opt-in: a missing allowWorldJs key stays false.
		return &UICustomizationPolicyConfig{
			AllowWorldCSS:       true,
			AllowPublicWorldCSS: true,
			MaxCodeSizeKB:       DefaultUICustomizationMaxCodeSizeKB,
		}
	}
	copy := *cfg
	if copy.MaxCodeSizeKB == 0 {
		copy.MaxCodeSizeKB = DefaultUICustomizationMaxCodeSizeKB
	}
	return &copy
}

func ValidateUICustomizationPolicy(cfg *UICustomizationPolicyConfig) error {
	if cfg == nil {
		return errors.New("UI customization policy must be normalized")
	}
	// All switches are booleans: false is an explicit policy, never a default.
	if cfg.MaxCodeSizeKB < MinUICustomizationMaxCodeSizeKB || cfg.MaxCodeSizeKB > MaxUICustomizationMaxCodeSizeKB {
		return fmt.Errorf("界面自定义代码大小上限必须在 %d 到 %d KiB 之间", MinUICustomizationMaxCodeSizeKB, MaxUICustomizationMaxCodeSizeKB)
	}
	return nil
}
