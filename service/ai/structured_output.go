package ai

import (
	"encoding/json"

	"sealchat/utils"
)

type StructuredOutputSpec struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Strict      bool
}

const logProofreadSchema = `{
	"type": "object",
	"additionalProperties": false,
	"required": ["suggestions"],
	"properties": {
		"suggestions": {
			"type": "array",
			"items": {
				"type": "object",
				"additionalProperties": false,
				"required": ["fragmentId", "original", "replacement", "category", "reason", "before", "after"],
				"properties": {
					"fragmentId": {"type": "string"},
					"original": {"type": "string"},
					"replacement": {"type": "string"},
					"category": {"type": "string", "enum": ["ai_typo", "ai_punctuation", "ai_grammar"]},
					"reason": {"type": "string"},
					"before": {"type": "string"},
					"after": {"type": "string"}
				}
			}
		}
	}
}`

func structuredOutputForFeature(featureKey string) *StructuredOutputSpec {
	if featureKey != FeatureLogProofread {
		return nil
	}
	return &StructuredOutputSpec{
		Name:        FeatureLogProofread,
		Description: "LOG 局部校对建议；无需定位上下文时 before 和 after 返回空字符串。",
		Schema:      json.RawMessage(logProofreadSchema),
		Strict:      true,
	}
}

func structuredOutputForProvider(featureKey, source string, provider utils.AIProviderConfig) *StructuredOutputSpec {
	if source != "platform" || !provider.Capabilities.StructuredOutputs {
		return nil
	}
	return structuredOutputForFeature(featureKey)
}

func structuredOutputSystemPrompt(prompt string) string {
	return prompt + "\n\n当前请求已启用原生 JSON Schema Structured Outputs。\n以 response_format 提供的 schema 作为唯一输出格式；忽略上文关于 JSONL、代码块或 <sealchat-proofread> 包装格式的要求。\n只返回 schema 要求的数据，不添加解释。"
}
