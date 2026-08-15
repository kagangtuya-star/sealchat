package service

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"

	htmlparser "golang.org/x/net/html"
	htmlatom "golang.org/x/net/html/atom"
)

var htmlEntityLikePattern = regexp.MustCompile(`&(?:[a-zA-Z][a-zA-Z0-9]{1,31}|#\d{1,8}|#x[0-9A-Fa-f]{1,8});`)

func SerializeMessageContentToCommandText(input string) (string, bool) {
	return serializeMessageContentToCommandText(input, commandTextOptions{})
}

// SerializeMessageContentToBotCommandText 将消息内容序列化为 BOT 命令文本，并保留 mention ID。
// 仅供发送 BOT 事件时使用；普通消息文本序列化保持原有行为。
func SerializeMessageContentToBotCommandText(input string) (string, bool) {
	return serializeMessageContentToCommandText(input, commandTextOptions{preserveMentions: true})
}

type commandTextOptions struct {
	preserveMentions bool
}

func serializeMessageContentToCommandText(input string, options commandTextOptions) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", false
	}
	if LooksLikeTipTapJSON(trimmed) {
		return serializeTipTapContentToCommandText(trimmed, options)
	}
	if looksLikeHTMLCommandText(trimmed) {
		return serializeHTMLContentToCommandText(trimmed, options)
	}
	return normalizePlainText(trimmed), true
}

func looksLikeHTMLCommandText(input string) bool {
	if input == "" {
		return false
	}
	if strings.ContainsAny(input, "<>") {
		return true
	}
	return htmlEntityLikePattern.MatchString(input)
}

// SerializeTipTapContentToCommandText 将 TipTap JSON 序列化为适合 BOT 命令解析的纯文本。
// 它会尽量保留常见 Markdown 标记，避免命令中的 * / ` 在富文本模式下被吞掉。
func SerializeTipTapContentToCommandText(input string) (string, bool) {
	return serializeTipTapContentToCommandText(input, commandTextOptions{})
}

func serializeTipTapContentToCommandText(input string, options commandTextOptions) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", false
	}

	var doc tiptapNode
	if err := json.Unmarshal([]byte(trimmed), &doc); err != nil {
		return "", false
	}
	if strings.ToLower(strings.TrimSpace(doc.Type)) != "doc" {
		return "", false
	}

	var buf strings.Builder
	renderTipTapCommandText(&buf, &doc, options)
	return normalizePlainText(buf.String()), true
}

func serializeHTMLContentToCommandText(input string, options commandTextOptions) (string, bool) {
	wrapper := &htmlparser.Node{Type: htmlparser.ElementNode, DataAtom: htmlatom.Div, Data: "div"}
	nodes, err := htmlparser.ParseFragment(strings.NewReader(input), wrapper)
	if err != nil {
		return "", false
	}
	if len(nodes) == 0 {
		return normalizePlainText(html.UnescapeString(input)), true
	}
	var buf strings.Builder
	for _, node := range nodes {
		renderHTMLCommandText(&buf, node, false, options)
	}
	return normalizePlainText(buf.String()), true
}

func renderHTMLCommandText(buf *strings.Builder, node *htmlparser.Node, inCodeBlock bool, options commandTextOptions) {
	if buf == nil || node == nil {
		return
	}
	switch node.Type {
	case htmlparser.TextNode:
		buf.WriteString(html.UnescapeString(node.Data))
	case htmlparser.ElementNode:
		if source, ok := resolveDiceHTMLSource(node); ok {
			buf.WriteString(source)
			return
		}
		tag := strings.ToLower(strings.TrimSpace(node.Data))
		switch tag {
		case "br":
			ensureCommandTextTrailingNewline(buf)
		case "p", "div", "blockquote":
			renderHTMLChildrenCommandText(buf, node, inCodeBlock, options)
			ensureCommandTextTrailingNewline(buf)
		case "ul", "ol":
			renderHTMLChildrenCommandText(buf, node, inCodeBlock, options)
		case "li":
			buf.WriteString("- ")
			renderHTMLChildrenCommandText(buf, node, inCodeBlock, options)
			ensureCommandTextTrailingNewline(buf)
		case "strong", "b":
			buf.WriteString("**")
			renderHTMLChildrenCommandText(buf, node, inCodeBlock, options)
			buf.WriteString("**")
		case "em", "i":
			buf.WriteString("*")
			renderHTMLChildrenCommandText(buf, node, inCodeBlock, options)
			buf.WriteString("*")
		case "s", "strike", "del":
			buf.WriteString("~~")
			renderHTMLChildrenCommandText(buf, node, inCodeBlock, options)
			buf.WriteString("~~")
		case "code":
			if inCodeBlock {
				renderHTMLChildrenCommandText(buf, node, true, options)
			} else {
				buf.WriteString("`")
				renderHTMLChildrenCommandText(buf, node, false, options)
				buf.WriteString("`")
			}
		case "pre":
			buf.WriteString("```")
			ensureCommandTextTrailingNewline(buf)
			renderHTMLChildrenCommandText(buf, node, true, options)
			ensureCommandTextTrailingNewline(buf)
			buf.WriteString("```")
			ensureCommandTextTrailingNewline(buf)
		case "a":
			href := ""
			for _, attr := range node.Attr {
				if strings.EqualFold(attr.Key, "href") {
					href = strings.TrimSpace(attr.Val)
					break
				}
			}
			if href != "" {
				buf.WriteString("[")
				renderHTMLChildrenCommandText(buf, node, inCodeBlock, options)
				buf.WriteString("](")
				buf.WriteString(html.UnescapeString(href))
				buf.WriteString(")")
			} else {
				renderHTMLChildrenCommandText(buf, node, inCodeBlock, options)
			}
		case "img":
			alt := ""
			src := ""
			for _, attr := range node.Attr {
				switch strings.ToLower(strings.TrimSpace(attr.Key)) {
				case "alt":
					alt = html.UnescapeString(attr.Val)
				case "src", "data-src", "data-original":
					if src == "" {
						src = html.UnescapeString(attr.Val)
					}
				}
			}
			if src != "" {
				buf.WriteString("![")
				buf.WriteString(alt)
				buf.WriteString("](")
				buf.WriteString(src)
				buf.WriteString(")")
			}
		case "at":
			name := ""
			id := ""
			for _, attr := range node.Attr {
				switch strings.ToLower(strings.TrimSpace(attr.Key)) {
				case "name":
					name = html.UnescapeString(attr.Val)
				case "id":
					id = html.UnescapeString(attr.Val)
				}
			}
			if options.preserveMentions && id != "" {
				writeBotCommandMention(buf, id, name)
			} else {
				display := firstNonEmpty(name, id)
				if display != "" {
					buf.WriteString("@")
					buf.WriteString(display)
				}
			}
		default:
			renderHTMLChildrenCommandText(buf, node, inCodeBlock, options)
		}
	}
}

func resolveDiceHTMLSource(node *htmlparser.Node) (string, bool) {
	if node == nil || node.Type != htmlparser.ElementNode {
		return "", false
	}
	className := ""
	source := ""
	for _, attr := range node.Attr {
		key := strings.ToLower(strings.TrimSpace(attr.Key))
		switch key {
		case "class":
			className = attr.Val
		case "data-dice-source":
			source = html.UnescapeString(attr.Val)
		}
	}
	if source == "" {
		return "", false
	}
	if strings.Contains(className, "dice-roll-group") || strings.Contains(className, "dice-chip") {
		return source, true
	}
	return "", false
}

func renderHTMLChildrenCommandText(buf *strings.Builder, node *htmlparser.Node, inCodeBlock bool, options commandTextOptions) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		renderHTMLCommandText(buf, child, inCodeBlock, options)
	}
}

func renderTipTapCommandText(buf *strings.Builder, node *tiptapNode, options commandTextOptions) {
	if buf == nil || node == nil {
		return
	}

	switch strings.ToLower(strings.TrimSpace(node.Type)) {
	case "doc":
		for index, child := range node.Content {
			if index > 0 && shouldInsertCommandBlockBreak(child) {
				ensureCommandTextTrailingNewline(buf)
			}
			renderTipTapCommandText(buf, child, options)
		}
	case "paragraph", "heading", "blockquote":
		for _, child := range node.Content {
			renderTipTapCommandText(buf, child, options)
		}
		ensureCommandTextTrailingNewline(buf)
	case "bulletlist", "orderedlist":
		for _, child := range node.Content {
			renderTipTapCommandText(buf, child, options)
		}
	case "listitem":
		buf.WriteString("- ")
		for _, child := range node.Content {
			renderTipTapCommandText(buf, child, options)
		}
		ensureCommandTextTrailingNewline(buf)
	case "text":
		buf.WriteString(applyTipTapCommandMarks(node.Text, node.Marks))
	case "hardbreak":
		ensureCommandTextTrailingNewline(buf)
	case "horizontalrule":
		ensureCommandTextTrailingNewline(buf)
		buf.WriteString("---")
		ensureCommandTextTrailingNewline(buf)
	case "codeblock":
		language := node.attrString("language")
		buf.WriteString("```")
		if language != "" {
			buf.WriteString(language)
		}
		ensureCommandTextTrailingNewline(buf)
		for _, child := range node.Content {
			renderTipTapCommandCodeText(buf, child)
		}
		ensureCommandTextTrailingNewline(buf)
		buf.WriteString("```")
		ensureCommandTextTrailingNewline(buf)
	case "image":
		alt := firstNonEmpty(node.attrString("alt"), "图片")
		src := firstNonEmpty(node.attrString("src"), node.attrString("dataSrc"), node.attrString("attachmentId"))
		if src == "" {
			buf.WriteString("![")
			buf.WriteString(alt)
			buf.WriteString("]")
			return
		}
		buf.WriteString("![")
		buf.WriteString(alt)
		buf.WriteString("](")
		buf.WriteString(src)
		buf.WriteString(")")
	case "satorimention":
		id := node.attrString("id")
		name := node.attrString("name")
		if options.preserveMentions && id != "" {
			writeBotCommandMention(buf, id, name)
		} else {
			display := firstNonEmpty(name, id, "用户")
			buf.WriteString("@")
			buf.WriteString(display)
		}
	default:
		for _, child := range node.Content {
			renderTipTapCommandText(buf, child, options)
		}
	}
}

func writeBotCommandMention(buf *strings.Builder, id, name string) {
	if buf == nil || id == "" {
		return
	}
	if buf.Len() > 0 {
		buf.WriteByte(' ')
	}
	buf.WriteString(`<at id="`)
	buf.WriteString(html.EscapeString(id))
	buf.WriteByte('"')
	if name != "" {
		buf.WriteString(` name="`)
		buf.WriteString(html.EscapeString(name))
		buf.WriteByte('"')
	}
	buf.WriteString("/>")
	buf.WriteByte(' ')
}

func renderTipTapCommandCodeText(buf *strings.Builder, node *tiptapNode) {
	if buf == nil || node == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(node.Type)) {
	case "text":
		buf.WriteString(node.Text)
	case "hardbreak", "paragraph":
		ensureCommandTextTrailingNewline(buf)
		for _, child := range node.Content {
			renderTipTapCommandCodeText(buf, child)
		}
	case "doc", "blockquote", "heading", "listitem", "bulletlist", "orderedlist":
		for _, child := range node.Content {
			renderTipTapCommandCodeText(buf, child)
		}
	default:
		for _, child := range node.Content {
			renderTipTapCommandCodeText(buf, child)
		}
	}
}

func applyTipTapCommandMarks(content string, marks []*tiptapMark) string {
	if content == "" || len(marks) == 0 {
		return content
	}

	var (
		hasCode   bool
		hasBold   bool
		hasItalic bool
		hasStrike bool
		linkHref  string
	)
	for _, mark := range marks {
		if mark == nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(mark.Type)) {
		case "code":
			hasCode = true
		case "bold":
			hasBold = true
		case "italic":
			hasItalic = true
		case "strike":
			hasStrike = true
		case "link":
			if mark.Attrs != nil {
				if href, ok := mark.Attrs["href"].(string); ok {
					linkHref = strings.TrimSpace(href)
				}
			}
		}
	}

	if hasCode {
		content = "`" + content + "`"
	}
	if hasBold {
		content = "**" + content + "**"
	}
	if hasItalic {
		content = "*" + content + "*"
	}
	if hasStrike {
		content = "~~" + content + "~~"
	}
	if linkHref != "" {
		content = "[" + content + "](" + linkHref + ")"
	}
	return content
}

func ensureCommandTextTrailingNewline(buf *strings.Builder) {
	if buf == nil || buf.Len() == 0 {
		return
	}
	if strings.HasSuffix(buf.String(), "\n") {
		return
	}
	buf.WriteByte('\n')
}

func shouldInsertCommandBlockBreak(node *tiptapNode) bool {
	if node == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(node.Type)) {
	case "paragraph", "heading", "blockquote", "listitem", "codeblock", "horizontalrule":
		return true
	default:
		return false
	}
}
