package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ChatSkin 是频道角色的聊天装扮文档（气泡 / 紧凑两套表面 + 装饰图层）。
//
// 前端 ui/src/features/chat-skin/chatSkinSchema.ts 负责完整的语义规范化与默认值；
// 服务端只做结构白名单、数值范围、字符串安全与资源引用格式校验，保证存储与快照中
// 不会出现任意 HTML/CSS/URL 或二进制数据，同时保留文档的前向兼容形态。
type ChatSkin map[string]any

// ChatSkinPatch 是角色差分对基础装扮的局部覆写，存放在差分 appearance.chatSkin。
type ChatSkinPatch struct {
	Mode string   `json:"mode"`
	Skin ChatSkin `json:"skin,omitempty"`
}

const (
	ChatSkinPatchModeMerge   = "merge"
	ChatSkinPatchModeReplace = "replace"
	ChatSkinPatchModeNone    = "none"

	ChatSkinMaxEncodedBytes = 16 * 1024
	ChatSkinMaxLayers       = 8
	ChatSkinMaxVars         = 24
	ChatSkinMaxAdvancedCSS  = 600
	ChatSkinMaxLayerAxis    = 480
)

var (
	ErrChatSkinTooLarge = errors.New("聊天装扮数据过大")

	chatSkinAttachmentRef = regexp.MustCompile(`^id:[A-Za-z0-9_-]{1,96}$`)
	chatSkinBuiltinRef    = regexp.MustCompile(`^builtin:[a-z0-9-]{1,40}$`)
	chatSkinColor         = regexp.MustCompile(`^(#([0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})|transparent)$`)
	chatSkinLayerID       = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	chatSkinPresetID      = regexp.MustCompile(`^[A-Za-z0-9:_-]{1,64}$`)
	chatSkinVarName       = regexp.MustCompile(`^--(chat|quote|message|sc-skin)-[a-z0-9-]{1,48}$`)
	chatSkinForbiddenCSS  = regexp.MustCompile(`(?i)url\s*\(|expression\s*\(|javascript:|@import|[<>{}\\]|/\*|image-set|image\s*\(|-moz-binding|behavior\s*:`)
	chatSkinImportantCSS  = regexp.MustCompile(`(?i)!important`)
)

// 与 Vue 和静态 Viewer 使用相同的声明白名单；保留视觉定制，排除定位与外部资源。
const chatSkinCSSProperties = `
background background-color background-image background-size background-position
background-repeat background-blend-mode border border-color border-style border-width border-radius
border-top border-right border-bottom border-left box-shadow outline outline-offset filter backdrop-filter
opacity mix-blend-mode color text-shadow letter-spacing word-spacing line-height font-style font-weight
font-variant text-decoration text-decoration-color text-decoration-style text-transform text-align
padding padding-top padding-right padding-bottom padding-left min-height min-width
`

func sanitizeChatSkinCSS(value string) string {
	allowed := map[string]bool{}
	for _, property := range strings.Fields(chatSkinCSSProperties) {
		allowed[property] = true
	}
	result := ""
	for _, chunk := range strings.Split(value, ";") {
		property, value, ok := strings.Cut(chunk, ":")
		property = strings.ToLower(strings.TrimSpace(property))
		value = strings.TrimSpace(chatSkinImportantCSS.ReplaceAllString(value, ""))
		if !ok || !allowed[property] || value == "" || len(value) > 200 || chatSkinForbiddenCSS.MatchString(value) {
			continue
		}
		declaration := property + ": " + value
		if result != "" {
			declaration = "; " + declaration
		}
		if len(result)+len(declaration) > ChatSkinMaxAdvancedCSS {
			break
		}
		result += declaration
	}
	return result
}

type chatSkinFieldKind int

const (
	chatSkinNumber chatSkinFieldKind = iota
	chatSkinBool
	chatSkinColorField
	chatSkinEnum
	chatSkinRef
	chatSkinText
	chatSkinCSS
	chatSkinObject
	chatSkinRadius
)

type chatSkinField struct {
	kind     chatSkinFieldKind
	min, max float64
	enum     []string
	maxLen   int
	object   map[string]chatSkinField
}

func numberField(min, max float64) chatSkinField {
	return chatSkinField{kind: chatSkinNumber, min: min, max: max}
}

func enumField(values ...string) chatSkinField {
	return chatSkinField{kind: chatSkinEnum, enum: values}
}

func objectField(fields map[string]chatSkinField) chatSkinField {
	return chatSkinField{kind: chatSkinObject, object: fields}
}

var (
	chatSkinColorF = chatSkinField{kind: chatSkinColorField}
	chatSkinBoolF  = chatSkinField{kind: chatSkinBool}
	chatSkinFonts  = []string{"inherit", "sans", "serif", "kai", "mono", "rounded"}

	chatSkinStyleFields = map[string]chatSkinField{
		"background": objectField(map[string]chatSkinField{
			"mode":          enumField("color", "gradient", "none"),
			"color":         chatSkinColorF,
			"color2":        chatSkinColorF,
			"angle":         numberField(0, 360),
			"imageRef":      {kind: chatSkinRef},
			"imageFit":      enumField("cover", "contain", "tile", "stretch"),
			"imagePosition": enumField("center", "top", "bottom", "left", "right"),
		}),
		"border": objectField(map[string]chatSkinField{
			"width": numberField(0, 8),
			"style": enumField("solid", "dashed", "dotted", "double"),
			"color": chatSkinColorF,
		}),
		"radius": {kind: chatSkinRadius},
		"shadow": objectField(map[string]chatSkinField{
			"mode":   enumField("none", "custom"),
			"color":  chatSkinColorF,
			"blur":   numberField(0, 48),
			"x":      numberField(-24, 24),
			"y":      numberField(-24, 24),
			"spread": numberField(-12, 12),
		}),
		"padding": objectField(map[string]chatSkinField{"x": numberField(0, 48), "y": numberField(0, 40)}),
		"size": objectField(map[string]chatSkinField{
			"minWidth": numberField(0, 480), "minHeight": numberField(0, 240), "maxWidth": numberField(0, 960),
		}),
		"spacing": objectField(map[string]chatSkinField{"top": numberField(0, 64), "bottom": numberField(0, 64)}),
		"accent": objectField(map[string]chatSkinField{
			"color": chatSkinColorF,
			"width": numberField(1, 12),
			"side":  enumField("left", "right", "top", "bottom"),
		}),
		"blur": numberField(0, 20),
		"text": objectField(map[string]chatSkinField{
			"color":  chatSkinColorF,
			"font":   enumField(chatSkinFonts...),
			"scale":  numberField(0.8, 1.4),
			"shadow": chatSkinColorF,
		}),
		"name": objectField(map[string]chatSkinField{
			"color":  chatSkinColorF,
			"font":   enumField(chatSkinFonts...),
			"weight": enumField("normal", "bold", "black"),
			"italic": chatSkinBoolF,
			"chip":   chatSkinColorF,
			"shadow": chatSkinColorF,
		}),
	}

	chatSkinLayerFields = map[string]chatSkinField{
		"id":      {kind: chatSkinText, maxLen: 64},
		"label":   {kind: chatSkinText, maxLen: 32},
		"enabled": chatSkinBoolF,
		"ref":     {kind: chatSkinRef},
		"tint":    chatSkinColorF,
		"anchor": enumField("top-left", "top", "top-right", "left", "center", "right",
			"bottom-left", "bottom", "bottom-right"),
		"x":       numberField(-240, 240),
		"y":       numberField(-240, 240),
		"size":    numberField(8, 480),
		"aspect":  numberField(0.05, 20),
		"rotate":  numberField(-180, 180),
		"opacity": numberField(0, 1),
		"z":       enumField("front", "back"),
		"flipX":   chatSkinBoolF,
		"flipY":   chatSkinBoolF,
		"mirror":  chatSkinBoolF,
		"blend":   enumField("normal", "multiply", "screen", "overlay", "soft-light"),
	}
)

type chatSkinSanitizer struct {
	// partial 允许出现 null（差分中表示删除字段）以及不完整的分组。
	partial bool
}

func (s chatSkinSanitizer) value(field chatSkinField, raw any, path string) (any, error) {
	if raw == nil {
		if s.partial {
			return nil, nil
		}
		return nil, fmt.Errorf("%s 不能为空", path)
	}
	switch field.kind {
	case chatSkinNumber:
		num, ok := raw.(float64)
		if !ok || math.IsNaN(num) || math.IsInf(num, 0) {
			return nil, fmt.Errorf("%s 需为数字", path)
		}
		return math.Min(field.max, math.Max(field.min, num)), nil
	case chatSkinBool:
		value, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("%s 需为布尔值", path)
		}
		return value, nil
	case chatSkinColorField:
		value, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("%s 需为颜色", path)
		}
		value = strings.TrimSpace(value)
		if value != "" && !chatSkinColor.MatchString(value) {
			return nil, fmt.Errorf("%s 颜色格式无效", path)
		}
		return strings.ToLower(value), nil
	case chatSkinEnum:
		value, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("%s 取值无效", path)
		}
		for _, allowed := range field.enum {
			if value == allowed {
				return value, nil
			}
		}
		return nil, fmt.Errorf("%s 取值无效", path)
	case chatSkinRef:
		value, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("%s 资源引用无效", path)
		}
		value = strings.TrimSpace(value)
		if value != "" && !chatSkinAttachmentRef.MatchString(value) && !chatSkinBuiltinRef.MatchString(value) {
			return nil, fmt.Errorf("%s 只能引用附件或内置纹样", path)
		}
		return value, nil
	case chatSkinText:
		value, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("%s 需为文本", path)
		}
		value = strings.TrimSpace(value)
		if utf8.RuneCountInString(value) > field.maxLen || strings.ContainsAny(value, "<>") || hasControlRune(value) {
			return nil, fmt.Errorf("%s 文本无效", path)
		}
		return value, nil
	case chatSkinCSS:
		value, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("%s 需为文本", path)
		}
		if len(value) > ChatSkinMaxAdvancedCSS*2 {
			return nil, fmt.Errorf("%s 包含不允许的 CSS", path)
		}
		return sanitizeChatSkinCSS(value), nil
	case chatSkinRadius:
		if _, ok := raw.(float64); ok {
			value, err := s.value(numberField(0, 48), raw, path)
			return map[string]any{"tl": value, "tr": value, "br": value, "bl": value}, err
		}
		radius, err := s.object(map[string]chatSkinField{
			"tl": numberField(0, 48), "tr": numberField(0, 48), "br": numberField(0, 48), "bl": numberField(0, 48),
		}, raw, path)
		if err == nil && !s.partial {
			for _, corner := range []string{"tl", "tr", "br", "bl"} {
				if _, exists := radius[corner]; !exists {
					radius[corner] = float64(12)
				}
			}
		}
		return radius, err
	case chatSkinObject:
		return s.object(field.object, raw, path)
	}
	return nil, fmt.Errorf("%s 无法识别", path)
}

func (s chatSkinSanitizer) object(fields map[string]chatSkinField, raw any, path string) (map[string]any, error) {
	source, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s 需为对象", path)
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		field, known := fields[key]
		if !known {
			continue
		}
		cleaned, err := s.value(field, value, path+"."+key)
		if err != nil {
			return nil, err
		}
		result[key] = cleaned
	}
	return result, nil
}

func (s chatSkinSanitizer) style(raw any, path string) (any, error) {
	if raw == nil {
		return nil, nil
	}
	return s.object(chatSkinStyleFields, raw, path)
}

func (s chatSkinSanitizer) layers(raw any, path string) ([]any, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s 需为数组", path)
	}
	if len(items) > ChatSkinMaxLayers {
		return nil, fmt.Errorf("每种布局最多 %d 个装饰图层", ChatSkinMaxLayers)
	}
	result := make([]any, 0, len(items))
	strict := chatSkinSanitizer{}
	for index, item := range items {
		layer, err := strict.object(chatSkinLayerFields, item, fmt.Sprintf("%s[%d]", path, index))
		if err != nil {
			return nil, err
		}
		ref, _ := layer["ref"].(string)
		if ref == "" {
			return nil, fmt.Errorf("%s[%d] 缺少资源引用", path, index)
		}
		if id, _ := layer["id"].(string); id != "" && !chatSkinLayerID.MatchString(id) {
			return nil, fmt.Errorf("%s[%d] 图层 ID 无效", path, index)
		}
		size, ok := layer["size"].(float64)
		if !ok {
			size = 40
		}
		aspect, ok := layer["aspect"].(float64)
		if !ok {
			aspect = 1
		}
		size = math.Round(size*1000) / 1000
		aspect = math.Round(aspect*1000) / 1000
		layer["size"] = math.Min(size, math.Floor(ChatSkinMaxLayerAxis/aspect*1000)/1000)
		layer["aspect"] = aspect
		result = append(result, layer)
	}
	return result, nil
}

func (s chatSkinSanitizer) advanced(raw any, path string) (any, error) {
	if raw == nil {
		return nil, nil
	}
	source, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s 需为对象", path)
	}
	result := map[string]any{}
	for _, key := range []string{"surfaceCss", "nameCss", "textCss"} {
		value, exists := source[key]
		if !exists {
			continue
		}
		cleaned, err := s.value(chatSkinField{kind: chatSkinCSS}, value, path+"."+key)
		if err != nil {
			return nil, err
		}
		result[key] = cleaned
	}
	if rawVars, exists := source["vars"]; exists && rawVars != nil {
		vars, ok := rawVars.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s.vars 需为对象", path)
		}
		if len(vars) > ChatSkinMaxVars {
			return nil, fmt.Errorf("高级变量最多 %d 个", ChatSkinMaxVars)
		}
		cleanedVars := make(map[string]any, len(vars))
		for name, value := range vars {
			text, ok := value.(string)
			text = strings.TrimSpace(chatSkinImportantCSS.ReplaceAllString(text, ""))
			if !chatSkinVarName.MatchString(name) || !ok || len(text) > 200 || strings.Contains(text, ";") || chatSkinForbiddenCSS.MatchString(text) {
				return nil, fmt.Errorf("%s.vars.%s 无效", path, name)
			}
			cleanedVars[name] = text
		}
		result["vars"] = cleanedVars
	} else if exists && s.partial {
		result["vars"] = nil
	}
	return result, nil
}

func (s chatSkinSanitizer) surface(raw any, path string) (any, error) {
	if raw == nil {
		return nil, nil
	}
	source, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s 需为对象", path)
	}
	result := map[string]any{}
	for key, value := range source {
		var (
			cleaned any
			err     error
		)
		switch key {
		case "enabled", "mirrorOnSelf", "reserveSpace":
			cleaned, err = s.value(chatSkinBoolF, value, path+"."+key)
		case "style", "night":
			cleaned, err = s.style(value, path+"."+key)
		case "layers":
			if value == nil && s.partial {
				cleaned = nil
			} else {
				cleaned, err = s.layers(value, path+"."+key)
			}
		case "advanced":
			cleaned, err = s.advanced(value, path+"."+key)
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		result[key] = cleaned
	}
	return result, nil
}

func (s chatSkinSanitizer) document(raw map[string]any) (ChatSkin, error) {
	result := ChatSkin{}
	for key, value := range raw {
		var (
			cleaned any
			err     error
		)
		switch key {
		case "version":
			cleaned, err = s.value(numberField(1, 1), value, key)
		case "scope":
			cleaned, err = s.value(enumField("all", "ic"), value, key)
		case "bubble", "compact":
			cleaned, err = s.surface(value, key)
		case "meta":
			if value == nil {
				cleaned = nil
				break
			}
			var meta map[string]any
			meta, err = s.object(map[string]chatSkinField{
				"name":     {kind: chatSkinText, maxLen: 40},
				"presetId": {kind: chatSkinText, maxLen: 64},
			}, value, key)
			if err == nil {
				if presetID, _ := meta["presetId"].(string); presetID != "" && !chatSkinPresetID.MatchString(presetID) {
					meta["presetId"] = ""
				}
				cleaned = meta
			}
		default:
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("聊天装扮无效: %w", err)
		}
		result[key] = cleaned
	}
	return result, nil
}

func hasControlRune(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func chatSkinNormalizeJSON(value any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(raw) > ChatSkinMaxEncodedBytes {
		return nil, ErrChatSkinTooLarge
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, errors.New("聊天装扮需为对象")
	}
	return document, nil
}

// SanitizeChatSkin 校验并清理完整装扮。nil / 空表面的装扮返回 nil，表示「无装扮」。
func SanitizeChatSkin(skin ChatSkin) (ChatSkin, error) {
	if skin == nil {
		return nil, nil
	}
	document, err := chatSkinNormalizeJSON(skin)
	if err != nil || document == nil {
		return nil, err
	}
	cleaned, err := chatSkinSanitizer{}.document(document)
	if err != nil {
		return nil, err
	}
	if !chatSkinHasSurface(cleaned) {
		return nil, nil
	}
	cleaned["version"] = float64(1)
	return cleaned, nil
}

// SanitizeChatSkinPatch 校验差分覆写；merge 模式允许局部字段与 null 删除。
func SanitizeChatSkinPatch(patch *ChatSkinPatch) (*ChatSkinPatch, error) {
	if patch == nil {
		return nil, nil
	}
	mode := strings.TrimSpace(patch.Mode)
	switch mode {
	case ChatSkinPatchModeNone:
		return &ChatSkinPatch{Mode: mode}, nil
	case ChatSkinPatchModeReplace:
		skin, err := SanitizeChatSkin(patch.Skin)
		if err != nil {
			return nil, err
		}
		if skin == nil {
			return &ChatSkinPatch{Mode: ChatSkinPatchModeNone}, nil
		}
		return &ChatSkinPatch{Mode: mode, Skin: skin}, nil
	case ChatSkinPatchModeMerge, "":
		if patch.Skin == nil {
			return nil, nil
		}
		document, err := chatSkinNormalizeJSON(patch.Skin)
		if err != nil {
			return nil, err
		}
		cleaned, err := chatSkinSanitizer{partial: true}.document(document)
		if err != nil {
			return nil, err
		}
		if len(cleaned) == 0 {
			return nil, nil
		}
		return &ChatSkinPatch{Mode: ChatSkinPatchModeMerge, Skin: cleaned}, nil
	default:
		return nil, errors.New("聊天装扮差分模式无效")
	}
}

func chatSkinHasSurface(skin ChatSkin) bool {
	if skin == nil {
		return false
	}
	_, bubble := skin["bubble"].(map[string]any)
	_, compact := skin["compact"].(map[string]any)
	return bubble || compact
}

// MergeChatSkinDocument 与前端 deepMergeChatSkinDocument 语义一致：
// 对象递归合并、数组整体替换、null 删除字段。
func MergeChatSkinDocument(base any, patch any) any {
	patchMap, ok := patch.(map[string]any)
	if !ok {
		if patch == nil {
			return base
		}
		return cloneChatSkinValue(patch)
	}
	result := map[string]any{}
	if baseMap, ok := base.(map[string]any); ok {
		for key, value := range baseMap {
			result[key] = cloneChatSkinValue(value)
		}
	}
	for key, value := range patchMap {
		if value == nil {
			delete(result, key)
			continue
		}
		if _, isMap := value.(map[string]any); isMap {
			result[key] = MergeChatSkinDocument(result[key], value)
			continue
		}
		result[key] = cloneChatSkinValue(value)
	}
	return result
}

func cloneChatSkinValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		clone := make(map[string]any, len(typed))
		for key, item := range typed {
			clone[key] = cloneChatSkinValue(item)
		}
		return clone
	case ChatSkin:
		return cloneChatSkinValue(map[string]any(typed))
	case []any:
		clone := make([]any, len(typed))
		for index, item := range typed {
			clone[index] = cloneChatSkinValue(item)
		}
		return clone
	default:
		return typed
	}
}

// CloneChatSkin 深拷贝装扮，避免快照与角色配置共享底层 map。
func CloneChatSkin(skin ChatSkin) ChatSkin {
	if skin == nil {
		return nil
	}
	clone, _ := cloneChatSkinValue(map[string]any(skin)).(map[string]any)
	return clone
}

// ResolveChatSkin 计算「基础装扮 + 差分覆写」的最终装扮，即发送时写入消息快照的内容。
func ResolveChatSkin(base ChatSkin, patch *ChatSkinPatch) ChatSkin {
	base = canonicalizeChatSkinRadius(base, false)
	if patch == nil {
		return CloneChatSkin(base)
	}
	switch patch.Mode {
	case ChatSkinPatchModeNone:
		return nil
	case ChatSkinPatchModeReplace:
		return canonicalizeChatSkinRadius(patch.Skin, false)
	}
	if patch.Skin == nil {
		return CloneChatSkin(base)
	}
	var baseValue any = map[string]any{}
	if base != nil {
		baseValue = map[string]any(base)
	}
	merged, _ := MergeChatSkinDocument(baseValue, map[string]any(canonicalizeChatSkinRadius(patch.Skin, true))).(map[string]any)
	if !chatSkinHasSurface(merged) {
		return nil
	}
	return canonicalizeChatSkinRadius(merged, false)
}

// 历史完整文档可能仍存 scalar radius。仅在领域层规范化，不改变通用 merge 规则；
// partial object 不补角，scalar patch 则明确覆写四角。
func canonicalizeChatSkinRadius(skin ChatSkin, partial bool) ChatSkin {
	result := CloneChatSkin(skin)
	for _, layout := range []string{"bubble", "compact"} {
		surface, _ := result[layout].(map[string]any)
		for _, key := range []string{"style", "night"} {
			style, _ := surface[key].(map[string]any)
			switch radius := style["radius"].(type) {
			case float64:
				style["radius"] = map[string]any{"tl": radius, "tr": radius, "br": radius, "bl": radius}
			case map[string]any:
				if !partial {
					for _, corner := range []string{"tl", "tr", "br", "bl"} {
						if radius[corner] == nil {
							radius[corner] = float64(12)
						}
					}
				}
			}
		}
	}
	return result
}

// ChatSkinAttachmentIDs 返回装扮中引用的附件 ID（去掉 id: 前缀，去重）。
func ChatSkinAttachmentIDs(skin ChatSkin) []string {
	if skin == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var result []string
	var visit func(value any, key string)
	visit = func(value any, key string) {
		switch typed := value.(type) {
		case map[string]any:
			for childKey, child := range typed {
				visit(child, childKey)
			}
		case []any:
			for _, child := range typed {
				visit(child, key)
			}
		case string:
			if (key == "ref" || key == "imageRef") && chatSkinAttachmentRef.MatchString(typed) {
				id := strings.TrimPrefix(typed, "id:")
				if _, exists := seen[id]; !exists {
					seen[id] = struct{}{}
					result = append(result, id)
				}
			}
		}
	}
	visit(map[string]any(skin), "")
	return result
}

// ChatSkinPatchFromAppearance 从差分 appearance 文档中读取 chatSkin 覆写。
func ChatSkinPatchFromAppearance(appearance map[string]any) *ChatSkinPatch {
	raw, exists := appearance["chatSkin"]
	if !exists || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var patch ChatSkinPatch
	if json.Unmarshal(encoded, &patch) != nil {
		return nil
	}
	if patch.Mode == "" {
		patch.Mode = ChatSkinPatchModeMerge
	}
	return &patch
}

// MapChatSkinAttachmentRefs 按映射替换附件引用（参数与返回值均为不带前缀的附件 ID）。
// 映射为空的图层会被移除，背景图引用会被清空；可用于完整装扮与差分文档。
func MapChatSkinAttachmentRefs(document map[string]any, mapID func(id string) string) map[string]any {
	if document == nil {
		return nil
	}
	mapRef := func(ref string) string {
		if !chatSkinAttachmentRef.MatchString(ref) {
			return ref
		}
		mapped := strings.TrimSpace(mapID(strings.TrimPrefix(ref, "id:")))
		if mapped == "" {
			return ""
		}
		return "id:" + strings.TrimPrefix(mapped, "id:")
	}
	var visit func(value any) any
	visit = func(value any) any {
		switch typed := value.(type) {
		case map[string]any:
			result := make(map[string]any, len(typed))
			for key, child := range typed {
				if key == "imageRef" {
					if ref, ok := child.(string); ok {
						result[key] = mapRef(ref)
						continue
					}
				}
				if key == "layers" {
					if items, ok := child.([]any); ok {
						layers := make([]any, 0, len(items))
						for _, item := range items {
							layer, ok := visit(item).(map[string]any)
							if !ok {
								continue
							}
							if ref, ok := layer["ref"].(string); ok {
								layer["ref"] = mapRef(ref)
								if layer["ref"] == "" {
									continue
								}
							}
							layers = append(layers, layer)
						}
						result[key] = layers
						continue
					}
				}
				result[key] = visit(child)
			}
			return result
		case ChatSkin:
			return visit(map[string]any(typed))
		case []any:
			result := make([]any, len(typed))
			for index, child := range typed {
				result[index] = visit(child)
			}
			return result
		default:
			return typed
		}
	}
	mapped, _ := visit(document).(map[string]any)
	return mapped
}
