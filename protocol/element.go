package protocol

import (
	"encoding/xml"
	"fmt"
	"html"
	"regexp"
	"strings"
)

type Element struct {
	//KElement bool      `json:"kElement"`
	Type     string     `json:"type"`
	Attrs    Dict       `json:"attrs"`
	Children []*Element `json:"children"`
	//Source   string    `json:"source"` // js版本自己也不写source，所以姑且注释掉
}

func (el *Element) Traverse(fn func(el *Element)) {
	fn(el)
	for _, child := range el.Children {
		child.Traverse(fn)
	}
}

func (el *Element) Traverse2(fn, fn2 func(el *Element)) {
	fn(el)
	for _, child := range el.Children {
		child.Traverse2(fn, fn2)
	}
	fn2(el)
}

func (el *Element) ToString() string {
	var sb strings.Builder
	el.Traverse2(func(el *Element) {
		switch el.Type {
		case "root":
			break
		case "text":
			if content, ok := el.Attrs["content"].(string); ok {
				sb.WriteString(html.EscapeString(content))
			} else if el.Attrs["content"] != nil {
				sb.WriteString(html.EscapeString(fmt.Sprint(el.Attrs["content"])))
			}
		default:
			sb.WriteString(fmt.Sprintf("<%s", el.Type))
			for k, v := range el.Attrs {
				sb.WriteString(fmt.Sprintf(" %s=\"%s\"", k, html.EscapeString(fmt.Sprint(v))))
			}
			sb.WriteString(">")
		}
	}, func(el *Element) {
		switch el.Type {
		case "root", "text":
			break
		default:
			sb.WriteString(fmt.Sprintf("</%s>", el.Type))
		}
	})
	return sb.String()
}

type Dict map[string]interface{}

func ElementParse(text string) *Element {
	decoder := xml.NewDecoder(strings.NewReader(text))

	var elStack []*Element
	// 添加一个临时的根节点，这样逻辑上更容易处理
	elStack = append(elStack, &Element{Type: "root"})

	appendToChild := func(el *Element) *Element {
		top := elStack[len(elStack)-1]
		top.Children = append(top.Children, el)
		return el
	}

	appendToChildAndPush := func(el *Element) {
		appendToChild(el)
		elStack = append(elStack, el)
	}

	popElement := func() {
		elStack = elStack[:len(elStack)-1]
	}

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch se := token.(type) {
		case xml.StartElement:
			// 对tag弄一下白名单？
			attrs := Dict{}
			for _, attr := range se.Attr {
				attrs[attr.Name.Local] = attr.Value
			}

			appendToChildAndPush(&Element{
				Type:  se.Name.Local,
				Attrs: attrs,
			})
		case xml.EndElement:
			popElement()
		case xml.CharData:
			appendToChild(&Element{
				Type:  "text",
				Attrs: Dict{"content": string(se)},
			})
			// case xml.Comment:
			//	fmt.Printf("Comment: %s\n", se)
			// case xml.ProcInst:
			//	fmt.Printf("ProcInst: %s %s\n", se.Target, se.Inst)
			// case xml.Directive:
			//	fmt.Printf("Directive: %s\n", se)
			// default:
			//	fmt.Printf("Unknown element\n")
		}
	}

	return elStack[0]
}

// satoriTags 是 Satori 协议支持的标签列表
var satoriTags = []string{
	"at", "sharp", "a", "img", "audio", "video", "file",
	"b", "strong", "i", "em", "u", "ins", "s", "del", "spl", "code", "sup", "sub",
	"br", "p", "message", "quote", "author", "button",
}

var satoriTagSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(satoriTags))
	for _, tag := range satoriTags {
		set[tag] = struct{}{}
	}
	return set
}()

var satoriTagRegexp = regexp.MustCompile(`</?([a-zA-Z][a-zA-Z0-9_-]*)(\s[^<>]*?)?/?>`)
var nestedEntityRegexp = regexp.MustCompile(`(?i)&amp;((?:amp|lt|gt|quot|apos)|#\d+|#x[0-9a-fA-F]+);`)

// ContainsSatoriTags 检查内容是否包含 Satori 协议标签
func ContainsSatoriTags(content string) bool {
	for _, tag := range satoriTags {
		// 检查开始标签 <tag> 或 <tag ...> 或自闭合 <tag/>
		if strings.Contains(content, "<"+tag+">") ||
			strings.Contains(content, "<"+tag+" ") ||
			strings.Contains(content, "<"+tag+"/>") {
			return true
		}
	}
	return false
}

// EscapeText 对纯文本进行 HTML 转义
func EscapeText(content string) string {
	return html.EscapeString(content)
}

// EscapeSatoriText 转义普通文本，但保留 Satori 标签
func EscapeSatoriText(content string) string {
	if content == "" {
		return content
	}
	content = normalizeNestedEntities(content)
	var sb strings.Builder
	sb.Grow(len(content))
	hasAllowedTag := false
	last := 0
	matches := satoriTagRegexp.FindAllStringSubmatchIndex(content, -1)
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		start, end := match[0], match[1]
		nameStart, nameEnd := match[2], match[3]
		if start < last || nameStart < 0 || nameEnd < 0 {
			continue
		}
		tagName := strings.ToLower(content[nameStart:nameEnd])
		if _, ok := satoriTagSet[tagName]; !ok {
			continue
		}
		hasAllowedTag = true
		if start > last {
			sb.WriteString(escapeXMLText(content[last:start]))
		}
		sb.WriteString(content[start:end])
		last = end
	}
	if !hasAllowedTag {
		return escapeXMLText(content)
	}
	if last < len(content) {
		sb.WriteString(escapeXMLText(content[last:]))
	}
	return sb.String()
}

const maxEntityLength = 32

func escapeXMLText(content string) string {
	if content == "" {
		return content
	}
	var sb strings.Builder
	sb.Grow(len(content))
	for i := 0; i < len(content); i++ {
		switch content[i] {
		case '&':
			if isSafeEntityAt(content, i) {
				sb.WriteByte('&')
			} else {
				sb.WriteString("&amp;")
			}
		case '<':
			sb.WriteString("&lt;")
		case '>':
			sb.WriteString("&gt;")
		default:
			sb.WriteByte(content[i])
		}
	}
	return sb.String()
}

func isSafeEntityAt(content string, pos int) bool {
	if pos < 0 || pos+1 >= len(content) {
		return false
	}
	limit := pos + 1 + maxEntityLength
	if limit > len(content) {
		limit = len(content)
	}
	for i := pos + 1; i < limit; i++ {
		ch := content[i]
		if ch == ';' {
			return isSafeEntityName(content[pos+1 : i])
		}
		switch ch {
		case ' ', '\t', '\n', '\r', '<', '&':
			return false
		}
	}
	return false
}

func isSafeEntityName(entity string) bool {
	switch entity {
	case "amp", "lt", "gt", "quot", "apos":
		return true
	}
	if len(entity) < 2 || entity[0] != '#' {
		return false
	}
	if entity[1] == 'x' || entity[1] == 'X' {
		if len(entity) == 2 {
			return false
		}
		for i := 2; i < len(entity); i++ {
			if !isHexDigit(entity[i]) {
				return false
			}
		}
		return true
	}
	for i := 1; i < len(entity); i++ {
		if entity[i] < '0' || entity[i] > '9' {
			return false
		}
	}
	return true
}

func isHexDigit(ch byte) bool {
	return (ch >= '0' && ch <= '9') ||
		(ch >= 'a' && ch <= 'f') ||
		(ch >= 'A' && ch <= 'F')
}

func normalizeNestedEntities(content string) string {
	if !strings.Contains(content, "&amp;") {
		return content
	}
	return nestedEntityRegexp.ReplaceAllString(content, "&$1;")
}
