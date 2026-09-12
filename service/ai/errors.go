package ai

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInputTooLong = errors.New("ai input too long")
var ErrUserCustomProviderRequired = errors.New("ai user custom provider required")
var ErrAIPricingNotConfigured = errors.New("ai pricing not configured")

type AIPricingNotConfiguredError struct {
	ProviderID string
	Model      string
}

func (e *AIPricingNotConfiguredError) Error() string {
	if e == nil {
		return "AI pricing 未配置"
	}
	model := strings.TrimSpace(e.Model)
	providerID := strings.TrimSpace(e.ProviderID)
	if providerID == "" {
		return fmt.Sprintf("AI pricing 未配置: %s", model)
	}
	return fmt.Sprintf("AI pricing 未配置: %s / %s", providerID, model)
}

func (e *AIPricingNotConfiguredError) Unwrap() error {
	return ErrAIPricingNotConfigured
}

func FormatAIPricingWarning(model string) string {
	return fmt.Sprintf(
		"AI pricing 未配置: %s，此次服务未扣减配额，请在平台管理-AI功能设定价格。",
		strings.TrimSpace(model),
	)
}

func FormatInputTooLongError(featureKey string, currentChars int, maxChars int) error {
	if featureKey == FeatureBattleSummary && currentChars > 0 && maxChars > 0 {
		return fmt.Errorf(
			"战报总结输入过长（当前 %d 字符，最大 %d 字符），请缩短时间范围或减少来源频道: %w",
			currentChars,
			maxChars,
			ErrInputTooLong,
		)
	}
	return ErrInputTooLong
}

func FormatUserCustomProviderRequiredError(featureKey string) error {
	label := BuiltinFeatures()[featureKey].Label
	if label == "" {
		label = "该 AI 功能"
	}
	return fmt.Errorf("%s仅允许用户自定义调用，请先在个人信息的 AI 设置中配置个人 API: %w", label, ErrUserCustomProviderRequired)
}
