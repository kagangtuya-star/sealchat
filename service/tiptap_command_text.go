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
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", false
	}
	if LooksLikeTipTapJSON(trimmed) {
		return SerializeTipTapContentToCommandText(trimmed)
	}
	if looksLikeHTMLCommandText(trimmed) {
		return serializeHTMLContentToCommandText(trimmed)
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
	renderTipTapCommandText(&buf, &doc)
	return normalizePlainText(buf.String()), true
}

func serializeHTMLContentToCommandText(input string) (string, bool) {
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
		renderHTMLCommandText(&buf, node, false)
	}
	return normalizePlainText(buf.String()), true
}

func renderHTMLCommandText(buf *strings.Builder, node *htmlparser.Node, inCodeBlock bool) {
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
			renderHTMLChildrenCommandText(buf, node, inCodeBlock)
			ensureCommandTextTrailingNewline(buf)
		case "ul", "ol":
			renderHTMLChildrenCommandText(buf, node, inCodeBlock)
		case "li":
			buf.WriteString("- ")
			renderHTMLChildrenCommandText(buf, node, inCodeBlock)
			ensureCommandTextTrailingNewline(buf)
		case "strong", "b":
			buf.WriteString("**")
			renderHTMLChildrenCommandText(buf, node, inCodeBlock)
			buf.WriteString("**")
		case "em", "i":
			buf.WriteString("*")
			renderHTMLChildrenCommandText(buf, node, inCodeBlock)
			buf.WriteString("*")
		case "s", "strike", "del":
			buf.WriteString("~~")
			renderHTMLChildrenCommandText(buf, node, inCodeBlock)
			buf.WriteString("~~")
		case "code":
			if inCodeBlock {
				renderHTMLChildrenCommandText(buf, node, true)
			} else {
				buf.WriteString("`")
				renderHTMLChildrenCommandText(buf, node, false)
				buf.WriteString("`")
			}
		case "pre":
			buf.WriteString("```")
			ensureCommandTextTrailingNewline(buf)
			renderHTMLChildrenCommandText(buf, node, true)
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
				renderHTMLChildrenCommandText(buf, node, inCodeBlock)
				buf.WriteString("](")
				buf.WriteString(html.UnescapeString(href))
				buf.WriteString(")")
			} else {
				renderHTMLChildrenCommandText(buf, node, inCodeBlock)
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
			display := firstNonEmpty(name, id)
			if display != "" {
				buf.WriteString("@")
				buf.WriteString(display)
			}
		default:
			renderHTMLChildrenCommandText(buf, node, inCodeBlock)
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

func renderHTMLChildrenCommandText(buf *strings.Builder, node *htmlparser.Node, inCodeBlock bool) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		renderHTMLCommandText(buf, child, inCodeBlock)
	}
}

func renderTipTapCommandText(buf *strings.Builder, node *tiptapNode) {
	if buf == nil || node == nil {
		return
	}

	switch strings.ToLower(strings.TrimSpace(node.Type)) {
	case "doc":
		for index, child := range node.Content {
			if index > 0 && shouldInsertCommandBlockBreak(child) {
				ensureCommandTextTrailingNewline(buf)
			}
			renderTipTapCommandText(buf, child)
		}
	case "paragraph", "heading", "blockquote":
		for _, child := range node.Content {
			renderTipTapCommandText(buf, child)
		}
		ensureCommandTextTrailingNewline(buf)
	case "bulletlist", "orderedlist":
		for _, child := range node.Content {
			renderTipTapCommandText(buf, child)
		}
	case "listitem":
		buf.WriteString("- ")
		for _, child := range node.Content {
			renderTipTapCommandText(buf, child)
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
		display := firstNonEmpty(node.attrString("name"), node.attrString("id"), "用户")
		buf.WriteString("@")
		buf.WriteString(display)
	default:
		for _, child := range node.Content {
			renderTipTapCommandText(buf, child)
		}
	}
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
