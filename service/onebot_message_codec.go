package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"sealchat/protocol"
)

type OneBotDecodedMessage struct {
	Content string
	QuoteID string
}

type OneBotMessageCodecHooks struct {
	ResolveUserID          func(numericID int64) (string, error)
	ResolveMessageID       func(numericID int64) (string, error)
	ResolveUserOneBotID    func(userID string) (int64, error)
	ResolveMessageOneBotID func(messageID string) (int64, error)
	ResolveAttachmentURL   func(token string) (string, error)
}

type oneBotMessageSegment struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

func DecodeOneBotMessageRaw(raw json.RawMessage, autoEscape bool, hooks OneBotMessageCodecHooks) (*OneBotDecodedMessage, error) {
	if len(raw) == 0 {
		return &OneBotDecodedMessage{}, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return DecodeOneBotMessageValue(value, autoEscape, hooks)
}

func DecodeOneBotMessageValue(value any, autoEscape bool, hooks OneBotMessageCodecHooks) (*OneBotDecodedMessage, error) {
	segments, err := normalizeOneBotMessageSegments(value, autoEscape)
	if err != nil {
		return nil, err
	}
	return decodeOneBotSegments(segments, hooks)
}

func EncodeOneBotMessage(content string, quoteID string, hooks OneBotMessageCodecHooks) (string, error) {
	root := protocol.ElementParse(content)
	var sb strings.Builder

	if strings.TrimSpace(quoteID) != "" && hooks.ResolveMessageOneBotID != nil {
		numericID, err := hooks.ResolveMessageOneBotID(strings.TrimSpace(quoteID))
		if err != nil {
			return "", err
		}
		sb.WriteString(fmt.Sprintf("[CQ:reply,id=%d]", numericID))
	}

	if root == nil || len(root.Children) == 0 {
		sb.WriteString(content)
		return sb.String(), nil
	}

	for _, child := range root.Children {
		if err := encodeOneBotElement(&sb, child, hooks); err != nil {
			return "", err
		}
	}

	return sb.String(), nil
}

func normalizeOneBotMessageSegments(value any, autoEscape bool) ([]oneBotMessageSegment, error) {
	if value == nil {
		return nil, nil
	}

	if autoEscape {
		return []oneBotMessageSegment{{
			Type: "text",
			Data: map[string]any{"text": fmt.Sprint(value)},
		}}, nil
	}

	switch v := value.(type) {
	case string:
		return parseStringMessageToSegments(v), nil
	case map[string]any:
		seg := normalizeOneBotArraySegment(v)
		if seg.Type == "" {
			return nil, fmt.Errorf("invalid onebot message segment")
		}
		return []oneBotMessageSegment{seg}, nil
	case []any:
		result := make([]oneBotMessageSegment, 0, len(v))
		for _, item := range v {
			raw, ok := item.(map[string]any)
			if !ok {
				result = append(result, oneBotMessageSegment{
					Type: "text",
					Data: map[string]any{"text": fmt.Sprint(item)},
				})
				continue
			}
			result = append(result, normalizeOneBotArraySegment(raw))
		}
		return result, nil
	default:
		return []oneBotMessageSegment{{
			Type: "text",
			Data: map[string]any{"text": fmt.Sprint(value)},
		}}, nil
	}
}

func parseStringMessageToSegments(content string) []oneBotMessageSegment {
	elements := ParseCQCode(content)
	if len(elements) == 0 {
		return nil
	}
	result := make([]oneBotMessageSegment, 0, len(elements))
	for _, element := range elements {
		if element == nil {
			continue
		}
		switch element.Type {
		case "text":
			result = append(result, oneBotMessageSegment{
				Type: "text",
				Data: map[string]any{"text": getStringAttr(element.Attrs, "content")},
			})
		case "at":
			result = append(result, oneBotMessageSegment{
				Type: "at",
				Data: map[string]any{
					"qq":   getStringAttr(element.Attrs, "id"),
					"name": getStringAttr(element.Attrs, "name"),
				},
			})
		case "img", "image":
			result = append(result, oneBotMessageSegment{
				Type: "image",
				Data: map[string]any{
					"file": getStringAttr(element.Attrs, "src"),
					"url":  getStringAttr(element.Attrs, "src"),
				},
			})
		case "quote":
			result = append(result, oneBotMessageSegment{
				Type: "reply",
				Data: map[string]any{"id": getStringAttr(element.Attrs, "id")},
			})
		case "br":
			result = append(result, oneBotMessageSegment{
				Type: "text",
				Data: map[string]any{"text": "\n"},
			})
		default:
			result = append(result, oneBotMessageSegment{
				Type: "text",
				Data: map[string]any{"text": element.ToString()},
			})
		}
	}
	return result
}

func normalizeOneBotArraySegment(raw map[string]any) oneBotMessageSegment {
	segmentType := strings.TrimSpace(strings.ToLower(fmt.Sprint(raw["type"])))
	data, _ := raw["data"].(map[string]any)
	if data == nil {
		data = map[string]any{}
	}
	if segmentType == "" {
		return oneBotMessageSegment{
			Type: "text",
			Data: map[string]any{"text": fmt.Sprint(raw)},
		}
	}
	return oneBotMessageSegment{
		Type: segmentType,
		Data: data,
	}
}

func decodeOneBotSegments(segments []oneBotMessageSegment, hooks OneBotMessageCodecHooks) (*OneBotDecodedMessage, error) {
	result := &OneBotDecodedMessage{}
	var sb strings.Builder

	for _, segment := range segments {
		switch segment.Type {
		case "text":
			sb.WriteString(stringFromOneBotData(segment.Data, "text", "content"))
		case "at":
			id := stringFromOneBotData(segment.Data, "qq", "id")
			if strings.EqualFold(strings.TrimSpace(id), "all") {
				sb.WriteString(`<at id="all" name="全体成员" />`)
				continue
			}
			numericID, err := parseOneBotInt64(id)
			if err != nil || hooks.ResolveUserID == nil {
				sb.WriteString(formatOneBotSegmentAsLiteral(segment))
				continue
			}
			internalID, err := hooks.ResolveUserID(numericID)
			if err != nil || strings.TrimSpace(internalID) == "" {
				sb.WriteString(formatOneBotSegmentAsLiteral(segment))
				continue
			}
			name := stringFromOneBotData(segment.Data, "name")
			if strings.TrimSpace(name) != "" {
				sb.WriteString(fmt.Sprintf(`<at id="%s" name="%s" />`, internalID, name))
			} else {
				sb.WriteString(fmt.Sprintf(`<at id="%s" />`, internalID))
			}
		case "image":
			src := resolveOneBotImageSource(segment.Data)
			if src == "" {
				continue
			}
			if strings.HasPrefix(src, "base64://") {
				src = "data:;base64," + strings.TrimPrefix(src, "base64://")
			}
			lower := strings.ToLower(src)
			if strings.HasPrefix(lower, "file://") {
				return nil, fmt.Errorf("unsupported image source: %s", src)
			}
			sb.WriteString(fmt.Sprintf(`<img src="%s" />`, src))
		case "reply":
			if result.QuoteID != "" {
				continue
			}
			if hooks.ResolveMessageID == nil {
				continue
			}
			numericID, err := parseOneBotInt64(stringFromOneBotData(segment.Data, "id"))
			if err != nil {
				return nil, err
			}
			quoteID, err := hooks.ResolveMessageID(numericID)
			if err != nil || strings.TrimSpace(quoteID) == "" {
				return nil, err
			}
			result.QuoteID = quoteID
		default:
			sb.WriteString(formatOneBotSegmentAsLiteral(segment))
		}
	}

	result.Content = sb.String()
	return result, nil
}

func encodeOneBotElement(sb *strings.Builder, el *protocol.Element, hooks OneBotMessageCodecHooks) error {
	if el == nil {
		return nil
	}
	switch el.Type {
	case "text":
		sb.WriteString(getStringAttr(el.Attrs, "content"))
	case "br":
		sb.WriteString("\n")
	case "at":
		id := strings.TrimSpace(getStringAttr(el.Attrs, "id"))
		if id == "" {
			return nil
		}
		if strings.EqualFold(id, "all") {
			sb.WriteString("[CQ:at,qq=all]")
			return nil
		}
		if hooks.ResolveUserOneBotID == nil {
			sb.WriteString("@" + id)
			return nil
		}
		numericID, err := hooks.ResolveUserOneBotID(id)
		if err != nil {
			name := strings.TrimSpace(getStringAttr(el.Attrs, "name"))
			if name != "" {
				sb.WriteString("@" + name)
			} else {
				sb.WriteString("@" + id)
			}
			return nil
		}
		sb.WriteString("[CQ:at,qq=")
		sb.WriteString(strconv.FormatInt(numericID, 10))
		if name := strings.TrimSpace(getStringAttr(el.Attrs, "name")); name != "" {
			sb.WriteString(",name=")
			sb.WriteString(escapeCQ(name))
		}
		sb.WriteString("]")
	case "img", "image", "file":
		src := strings.TrimSpace(getStringAttr(el.Attrs, "src"))
		if src == "" {
			return nil
		}
		url := src
		if strings.HasPrefix(src, "id:") && hooks.ResolveAttachmentURL != nil {
			resolved, err := hooks.ResolveAttachmentURL(strings.TrimPrefix(src, "id:"))
			if err != nil {
				return err
			}
			if strings.TrimSpace(resolved) != "" {
				url = resolved
			}
		}
		sb.WriteString("[CQ:image,file=")
		sb.WriteString(escapeCQ(url))
		sb.WriteString(",url=")
		sb.WriteString(escapeCQ(url))
		sb.WriteString("]")
	case "quote":
		// 引用由外部 quoteID 参数统一处理，避免重复输出。
	default:
		for _, child := range el.Children {
			if err := encodeOneBotElement(sb, child, hooks); err != nil {
				return err
			}
		}
	}
	return nil
}

func resolveOneBotImageSource(data map[string]any) string {
	candidates := []string{
		stringFromOneBotData(data, "url"),
		stringFromOneBotData(data, "file"),
	}
	for _, item := range candidates {
		item = strings.TrimSpace(item)
		if item != "" {
			return item
		}
	}
	return ""
}

func formatOneBotSegmentAsLiteral(segment oneBotMessageSegment) string {
	switch segment.Type {
	case "text":
		return stringFromOneBotData(segment.Data, "text", "content")
	case "reply":
		return ""
	default:
		return encodeOneBotLiteralSegment(segment.Type, segment.Data)
	}
}

func encodeOneBotLiteralSegment(segmentType string, data map[string]any) string {
	if segmentType == "" {
		return ""
	}
	var parts []string
	for key, value := range data {
		parts = append(parts, fmt.Sprintf("%s=%s", key, escapeCQ(fmt.Sprint(value))))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("[CQ:%s]", segmentType)
	}
	return fmt.Sprintf("[CQ:%s,%s]", segmentType, strings.Join(parts, ","))
}

func stringFromOneBotData(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			return fmt.Sprint(value)
		}
	}
	return ""
}

func parseOneBotInt64(input string) (int64, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0, fmt.Errorf("empty numeric value")
	}
	return strconv.ParseInt(input, 10, 64)
}
