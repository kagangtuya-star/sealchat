package service

import (
	"regexp"
	"sealchat/protocol"
	"strings"
)

var (
	// CQ 码正则：匹配 [CQ:type,key=value,key=value...]
	cqCodePattern = regexp.MustCompile(`\[CQ:([a-zA-Z]+)(?:,([^\]]*))?\]`)
)

// ParseCQCode 解析 CQ 码为 Element 数组
// 支持格式：[CQ:at,qq=userId,name=displayName] 或 [CQ:at,qq=all]
func ParseCQCode(content string) []*protocol.Element {
	if content == "" {
		return nil
	}

	var elements []*protocol.Element
	lastIndex := 0

	matches := cqCodePattern.FindAllStringSubmatchIndex(content, -1)
	for _, match := range matches {
		// 添加 CQ 码之前的文本
		if match[0] > lastIndex {
			textContent := content[lastIndex:match[0]]
			if textContent != "" {
				elements = append(elements, &protocol.Element{
					Type:  "text",
					Attrs: protocol.Dict{"content": textContent},
				})
			}
		}

		// 解析 CQ 码
		fullMatch := content[match[0]:match[1]]
		cqType := content[match[2]:match[3]]
		var params string
		if match[4] != -1 && match[5] != -1 {
			params = content[match[4]:match[5]]
		}

		// 解析参数
		paramMap := parseCQParams(params)

		// 根据类型创建 Element
		switch cqType {
		case "at":
			attrs := protocol.Dict{}
			qq := paramMap["qq"]
			name := paramMap["name"]

			if qq == "all" {
				attrs["id"] = "all"
				attrs["name"] = "全体成员"
			} else {
				attrs["id"] = qq
				if name != "" {
					attrs["name"] = unescapeCQ(name)
				}
			}
			elements = append(elements, &protocol.Element{
				Type:  "at",
				Attrs: attrs,
			})
		default:
			// 不支持的 CQ 类型，保留原文
			elements = append(elements, &protocol.Element{
				Type:  "text",
				Attrs: protocol.Dict{"content": fullMatch},
			})
		}

		lastIndex = match[1]
	}

	// 添加剩余文本
	if lastIndex < len(content) {
		remaining := content[lastIndex:]
		if remaining != "" {
			elements = append(elements, &protocol.Element{
				Type:  "text",
				Attrs: protocol.Dict{"content": remaining},
			})
		}
	}

	return elements
}

// EncodeCQCode 将 Element 数组编码为 CQ 码格式字符串
func EncodeCQCode(elements []*protocol.Element) string {
	if len(elements) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, el := range elements {
		switch el.Type {
		case "text":
			if content, ok := el.Attrs["content"].(string); ok {
				sb.WriteString(content)
			}
		case "at":
			id := getStringAttr(el.Attrs, "id")
			name := getStringAttr(el.Attrs, "name")

			if id == "all" {
				sb.WriteString("[CQ:at,qq=all]")
			} else if id != "" {
				sb.WriteString("[CQ:at,qq=")
				sb.WriteString(escapeCQ(id))
				if name != "" {
					sb.WriteString(",name=")
					sb.WriteString(escapeCQ(name))
				}
				sb.WriteString("]")
			}
		default:
			// 其他类型使用默认的 ToString
			sb.WriteString(el.ToString())
		}
	}

	return sb.String()
}

// ElementsToSatoriXML 将 Element 数组转换为 Satori XML 字符串
func ElementsToSatoriXML(elements []*protocol.Element) string {
	if len(elements) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, el := range elements {
		sb.WriteString(el.ToString())
	}
	return sb.String()
}

// ConvertCQToSatori 将包含 CQ 码的消息转换为 Satori XML 格式
func ConvertCQToSatori(content string) string {
	if content == "" || !strings.Contains(content, "[CQ:") {
		return content
	}
	elements := ParseCQCode(content)
	return ElementsToSatoriXML(elements)
}

// ConvertSatoriToCQ 将 Satori XML 格式消息转换为 CQ 码格式
func ConvertSatoriToCQ(content string) string {
	if content == "" {
		return content
	}
	root := protocol.ElementParse(content)
	if root == nil || len(root.Children) == 0 {
		return content
	}
	return EncodeCQCode(root.Children)
}

// parseCQParams 解析 CQ 码参数
func parseCQParams(params string) map[string]string {
	result := make(map[string]string)
	if params == "" {
		return result
	}

	pairs := strings.Split(params, ",")
	for _, pair := range pairs {
		idx := strings.Index(pair, "=")
		if idx > 0 {
			key := strings.TrimSpace(pair[:idx])
			value := ""
			if idx+1 < len(pair) {
				value = strings.TrimSpace(pair[idx+1:])
			}
			result[key] = value
		}
	}
	return result
}

// escapeCQ 转义 CQ 码特殊字符
func escapeCQ(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "[", "&#91;")
	s = strings.ReplaceAll(s, "]", "&#93;")
	s = strings.ReplaceAll(s, ",", "&#44;")
	return s
}

// unescapeCQ 反转义 CQ 码特殊字符
func unescapeCQ(s string) string {
	s = strings.ReplaceAll(s, "&#44;", ",")
	s = strings.ReplaceAll(s, "&#93;", "]")
	s = strings.ReplaceAll(s, "&#91;", "[")
	s = strings.ReplaceAll(s, "&amp;", "&")
	return s
}

// getStringAttr 安全获取字符串属性
func getStringAttr(attrs protocol.Dict, key string) string {
	if v, ok := attrs[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
