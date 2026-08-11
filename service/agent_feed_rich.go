package service

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	htmlnode "golang.org/x/net/html"
)

// AgentRichDocument is a small, stable rich-text AST used by the public agent
// feed. It deliberately exposes only known nodes and marks; unknown HTML is
// reduced to its text/known descendants instead of being executed.
type AgentRichDocument struct {
	Type   string          `json:"type"`
	Blocks []AgentRichNode `json:"blocks"`
}

type AgentRichNode struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	Attrs    map[string]string `json:"attrs,omitempty"`
	Marks    []AgentRichMark   `json:"marks,omitempty"`
	Children []AgentRichNode   `json:"children,omitempty"`
}

type AgentRichMark struct {
	Type  string `json:"type"`
	Value string `json:"value,omitempty"`
}

var agentRGBColorPattern = regexp.MustCompile(`(?i)^rgba?\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})(?:\s*,\s*(?:0|1|0?\.\d+))?\s*\)$`)

func AgentRichDocumentFromExportMessage(msg *ExportMessage, sanitizeMode string, imagesMode string) AgentRichDocument {
	if msg == nil {
		return AgentRichDocument{Type: "document", Blocks: []AgentRichNode{}}
	}
	input := strings.TrimSpace(msg.ContentHTML)
	if input == "" {
		input = enhancePlainContentForHTMLExport(msg.Content)
	}
	return ParseAgentRichHTML(input, sanitizeMode, imagesMode)
}

func ParseAgentRichHTML(input string, sanitizeMode string, imagesMode string) AgentRichDocument {
	doc := AgentRichDocument{Type: "document", Blocks: []AgentRichNode{}}
	if strings.TrimSpace(input) == "" {
		return doc
	}
	root, err := htmlnode.Parse(strings.NewReader("<!doctype html><html><body><div>" + input + "</div></body></html>"))
	if err != nil || root == nil {
		plain := normalizePlainText(stripRichText(input))
		if plain != "" {
			doc.Blocks = []AgentRichNode{{Type: "paragraph", Children: []AgentRichNode{{Type: "text", Text: plain}}}}
		}
		return doc
	}
	container := findAgentRichContainer(root)
	if container == nil {
		return doc
	}
	mode := strings.ToLower(strings.TrimSpace(sanitizeMode))
	if mode == "" {
		mode = "supported"
	}
	imageMode := strings.ToLower(strings.TrimSpace(imagesMode))
	if imageMode == "" {
		imageMode = "meta"
	}
	var converted []AgentRichNode
	for child := container.FirstChild; child != nil; child = child.NextSibling {
		converted = append(converted, convertAgentHTMLNode(child, nil, mode, imageMode)...)
	}
	doc.Blocks = normalizeAgentRichBlocks(converted)
	return doc
}

func findAgentRichContainer(root *htmlnode.Node) *htmlnode.Node {
	if root == nil {
		return nil
	}
	var body *htmlnode.Node
	var walk func(*htmlnode.Node)
	walk = func(node *htmlnode.Node) {
		if node == nil || body != nil {
			return
		}
		if node.Type == htmlnode.ElementNode && strings.EqualFold(node.Data, "body") {
			body = node
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	if body == nil {
		return root
	}
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == htmlnode.ElementNode && strings.EqualFold(child.Data, "div") {
			return child
		}
	}
	return body
}

func convertAgentHTMLNode(node *htmlnode.Node, inherited []AgentRichMark, sanitizeMode string, imagesMode string) []AgentRichNode {
	if node == nil {
		return nil
	}
	switch node.Type {
	case htmlnode.TextNode:
		if node.Data == "" {
			return nil
		}
		return []AgentRichNode{{Type: "text", Text: node.Data, Marks: cloneAgentMarks(inherited)}}
	case htmlnode.CommentNode, htmlnode.DoctypeNode:
		return nil
	case htmlnode.ElementNode:
	default:
		return convertAgentHTMLChildren(node, inherited, sanitizeMode, imagesMode)
	}

	tag := strings.ToLower(strings.TrimSpace(node.Data))
	attrs := agentHTMLAttributes(node)
	marks := cloneAgentMarks(inherited)
	marks = appendAgentMarks(marks, marksFromAgentHTMLElement(tag, attrs, sanitizeMode)...)

	switch tag {
	case "script", "style", "iframe", "object", "embed", "form", "input", "button", "textarea", "select", "option", "svg", "math":
		return nil
	case "br":
		return []AgentRichNode{{Type: "hard_break"}}
	case "hr":
		return []AgentRichNode{{Type: "horizontal_rule"}}
	case "img":
		if imagesMode == "omit" {
			return nil
		}
		rawSrc := firstNonEmpty(attrs["src"], attrs["data-src"], attrs["data-original"])
		src := sanitizeAgentMediaURL(resolveImageURL(rawSrc))
		attachmentID := firstNonEmpty(attrs["data-attachment-id"], attrs["attachmentid"], attrs["data-id"])
		if src == "" && attachmentID != "" {
			src = sanitizeAgentMediaURL(resolveImageURL(attachmentID))
		}
		if src == "" && attachmentID == "" {
			return nil
		}
		imageAttrs := map[string]string{}
		if src != "" {
			imageAttrs["src"] = src
		}
		if alt := strings.TrimSpace(attrs["alt"]); alt != "" {
			imageAttrs["alt"] = alt
		}
		if title := strings.TrimSpace(attrs["title"]); title != "" {
			imageAttrs["title"] = title
		}
		if attachmentID != "" {
			imageAttrs["attachment_id"] = strings.TrimSpace(attachmentID)
		}
		return []AgentRichNode{{Type: "image", Attrs: imageAttrs}}
	case "pre":
		text := normalizeAgentCodeText(agentHTMLTextContent(node))
		if text == "" {
			return nil
		}
		attrsOut := map[string]string{}
		if lang := firstNonEmpty(attrs["data-language"], attrs["class"]); lang != "" {
			attrsOut["language"] = sanitizeAgentLanguage(lang)
		}
		return []AgentRichNode{{Type: "code_block", Text: text, Attrs: attrsOut}}
	case "ul", "ol":
		children := normalizeAgentListItems(convertAgentHTMLChildren(node, nil, sanitizeMode, imagesMode))
		if len(children) == 0 {
			return nil
		}
		typeName := "bullet_list"
		if tag == "ol" {
			typeName = "ordered_list"
		}
		return []AgentRichNode{{Type: typeName, Children: children}}
	case "li":
		children := normalizeAgentRichBlocks(convertAgentHTMLChildren(node, marks, sanitizeMode, imagesMode))
		if len(children) == 0 {
			return nil
		}
		return []AgentRichNode{{Type: "list_item", Children: children}}
	case "blockquote":
		children := normalizeAgentRichBlocks(convertAgentHTMLChildren(node, marks, sanitizeMode, imagesMode))
		if len(children) == 0 {
			return nil
		}
		return []AgentRichNode{{Type: "blockquote", Children: children}}
	case "p", "div", "section", "article", "header", "footer", "main", "aside":
		children := convertAgentHTMLChildren(node, marks, sanitizeMode, imagesMode)
		if len(children) == 0 {
			return nil
		}
		return []AgentRichNode{{Type: "paragraph", Children: children}}
	case "h1", "h2", "h3", "h4", "h5", "h6":
		children := convertAgentHTMLChildren(node, marks, sanitizeMode, imagesMode)
		if len(children) == 0 {
			return nil
		}
		return []AgentRichNode{{Type: "heading", Attrs: map[string]string{"level": strings.TrimPrefix(tag, "h")}, Children: children}}
	case "at":
		mentionAttrs := map[string]string{}
		if id := firstNonEmpty(attrs["id"], attrs["user-id"], attrs["data-user-id"], attrs["data-id"]); id != "" {
			mentionAttrs["id"] = id
		}
		label := strings.TrimSpace(agentHTMLTextContent(node))
		if label == "" {
			label = firstNonEmpty(attrs["name"], attrs["label"])
		}
		return []AgentRichNode{{Type: "mention", Text: label, Attrs: mentionAttrs, Marks: marks}}
	case "span":
		if isAgentMentionElement(attrs) {
			mentionAttrs := map[string]string{}
			if id := firstNonEmpty(attrs["data-user-id"], attrs["data-id"], attrs["id"]); id != "" {
				mentionAttrs["id"] = id
			}
			return []AgentRichNode{{Type: "mention", Text: strings.TrimSpace(agentHTMLTextContent(node)), Attrs: mentionAttrs, Marks: marks}}
		}
	}
	return convertAgentHTMLChildren(node, marks, sanitizeMode, imagesMode)
}

func convertAgentHTMLChildren(node *htmlnode.Node, marks []AgentRichMark, sanitizeMode string, imagesMode string) []AgentRichNode {
	var out []AgentRichNode
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		out = append(out, convertAgentHTMLNode(child, marks, sanitizeMode, imagesMode)...)
	}
	return out
}

func agentHTMLAttributes(node *htmlnode.Node) map[string]string {
	attrs := map[string]string{}
	if node == nil {
		return attrs
	}
	for _, attr := range node.Attr {
		key := strings.ToLower(strings.TrimSpace(attr.Key))
		if key == "" {
			continue
		}
		attrs[key] = attr.Val
	}
	return attrs
}

func marksFromAgentHTMLElement(tag string, attrs map[string]string, sanitizeMode string) []AgentRichMark {
	var marks []AgentRichMark
	switch tag {
	case "strong", "b":
		marks = append(marks, AgentRichMark{Type: "bold"})
	case "em", "i":
		marks = append(marks, AgentRichMark{Type: "italic"})
	case "u":
		marks = append(marks, AgentRichMark{Type: "underline"})
	case "s", "strike", "del":
		marks = append(marks, AgentRichMark{Type: "strike"})
	case "code":
		marks = append(marks, AgentRichMark{Type: "code"})
	case "a":
		if href := sanitizeAgentLinkURL(attrs["href"]); href != "" {
			marks = append(marks, AgentRichMark{Type: "link", Value: href})
		}
	case "font":
		if sanitizeMode != "strict" {
			if color := normalizeAgentColor(attrs["color"]); color != "" {
				marks = append(marks, AgentRichMark{Type: "color", Value: color})
			}
		}
	}
	style := parseAgentInlineStyle(attrs["style"])
	if weight := strings.ToLower(style["font-weight"]); weight == "bold" || weight == "bolder" || parseAgentInt(weight) >= 600 {
		marks = append(marks, AgentRichMark{Type: "bold"})
	}
	if strings.EqualFold(style["font-style"], "italic") || strings.EqualFold(style["font-style"], "oblique") {
		marks = append(marks, AgentRichMark{Type: "italic"})
	}
	decoration := strings.ToLower(style["text-decoration"] + " " + style["text-decoration-line"])
	if strings.Contains(decoration, "underline") {
		marks = append(marks, AgentRichMark{Type: "underline"})
	}
	if strings.Contains(decoration, "line-through") {
		marks = append(marks, AgentRichMark{Type: "strike"})
	}
	if sanitizeMode != "strict" {
		colorRaw := firstNonEmpty(style["color"], attrs["data-color"], attrs["color"])
		if color := normalizeAgentColor(colorRaw); color != "" {
			marks = append(marks, AgentRichMark{Type: "color", Value: color})
		}
	}
	return deduplicateAgentMarks(marks)
}

func parseAgentInlineStyle(raw string) map[string]string {
	out := map[string]string{}
	for _, item := range strings.Split(raw, ";") {
		key, value, ok := strings.Cut(item, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func normalizeAgentColor(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "#") {
		hex := strings.TrimPrefix(value, "#")
		if (len(hex) == 3 || len(hex) == 4 || len(hex) == 6 || len(hex) == 8) && isAgentHex(hex) {
			if len(hex) == 3 || len(hex) == 4 {
				var expanded strings.Builder
				for _, ch := range hex {
					expanded.WriteRune(ch)
					expanded.WriteRune(ch)
				}
				hex = expanded.String()
			}
			if len(hex) == 8 {
				hex = hex[:6]
			}
			return "#" + hex
		}
		return ""
	}
	match := agentRGBColorPattern.FindStringSubmatch(value)
	if len(match) == 4 {
		parts := make([]int, 3)
		for i := 1; i <= 3; i++ {
			num, err := strconv.Atoi(match[i])
			if err != nil || num < 0 || num > 255 {
				return ""
			}
			parts[i-1] = num
		}
		return fmt.Sprintf("#%02x%02x%02x", parts[0], parts[1], parts[2])
	}
	return ""
}

func isAgentHex(value string) bool {
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func sanitizeAgentLinkURL(raw string) string {
	value := strings.TrimSpace(html.UnescapeString(raw))
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil {
		return ""
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" && scheme != "mailto" {
		return ""
	}
	return value
}

func sanitizeAgentMediaURL(raw string) string {
	value := strings.TrimSpace(html.UnescapeString(raw))
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "//") || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") {
		return ""
	}
	if strings.HasPrefix(value, "/") {
		return value
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "data:") {
		for _, prefix := range []string{
			"data:image/png;",
			"data:image/jpeg;",
			"data:image/jpg;",
			"data:image/gif;",
			"data:image/webp;",
			"data:image/avif;",
			"data:image/bmp;",
		} {
			if strings.HasPrefix(lower, prefix) {
				return value
			}
		}
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil {
		return ""
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme == "http" || scheme == "https" {
		return value
	}
	return ""
}

func normalizeAgentRichBlocks(nodes []AgentRichNode) []AgentRichNode {
	out := make([]AgentRichNode, 0, len(nodes))
	inline := make([]AgentRichNode, 0, 4)
	flush := func() {
		if len(inline) == 0 {
			return
		}
		out = append(out, AgentRichNode{Type: "paragraph", Children: inline})
		inline = nil
	}
	for _, node := range nodes {
		if isAgentBlockNode(node.Type) {
			flush()
			out = append(out, node)
			continue
		}
		inline = append(inline, node)
	}
	flush()
	return trimEmptyAgentRichNodes(out)
}

func normalizeAgentListItems(nodes []AgentRichNode) []AgentRichNode {
	out := make([]AgentRichNode, 0, len(nodes))
	for _, node := range nodes {
		if node.Type == "list_item" {
			out = append(out, node)
			continue
		}
		if isAgentBlockNode(node.Type) {
			out = append(out, AgentRichNode{Type: "list_item", Children: []AgentRichNode{node}})
			continue
		}
		out = append(out, AgentRichNode{Type: "list_item", Children: []AgentRichNode{{Type: "paragraph", Children: []AgentRichNode{node}}}})
	}
	return out
}

func trimEmptyAgentRichNodes(nodes []AgentRichNode) []AgentRichNode {
	out := make([]AgentRichNode, 0, len(nodes))
	for _, node := range nodes {
		if node.Type == "text" && node.Text == "" {
			continue
		}
		if len(node.Children) > 0 {
			node.Children = trimEmptyAgentRichNodes(node.Children)
		}
		if node.Text == "" && len(node.Children) == 0 && node.Type != "hard_break" && node.Type != "horizontal_rule" && node.Type != "image" {
			continue
		}
		out = append(out, node)
	}
	return out
}

func isAgentBlockNode(nodeType string) bool {
	switch nodeType {
	case "paragraph", "heading", "blockquote", "bullet_list", "ordered_list", "list_item", "code_block", "horizontal_rule":
		return true
	default:
		return false
	}
}

func cloneAgentMarks(input []AgentRichMark) []AgentRichMark {
	if len(input) == 0 {
		return nil
	}
	out := make([]AgentRichMark, len(input))
	copy(out, input)
	return out
}

func appendAgentMarks(base []AgentRichMark, additions ...AgentRichMark) []AgentRichMark {
	out := cloneAgentMarks(base)
	out = append(out, additions...)
	return deduplicateAgentMarks(out)
}

func deduplicateAgentMarks(input []AgentRichMark) []AgentRichMark {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]AgentRichMark, 0, len(input))
	for _, mark := range input {
		key := mark.Type + "\x00" + mark.Value
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, mark)
	}
	return out
}

func agentHTMLTextContent(node *htmlnode.Node) string {
	if node == nil {
		return ""
	}
	var builder strings.Builder
	var walk func(*htmlnode.Node)
	walk = func(current *htmlnode.Node) {
		if current == nil {
			return
		}
		if current.Type == htmlnode.TextNode {
			builder.WriteString(current.Data)
		}
		if current.Type == htmlnode.ElementNode && strings.EqualFold(current.Data, "br") {
			builder.WriteByte('\n')
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return builder.String()
}

func normalizeAgentCodeText(input string) string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")
	return strings.Trim(input, "\n")
}

func sanitizeAgentLanguage(input string) string {
	value := strings.TrimSpace(input)
	value = strings.TrimPrefix(value, "language-")
	if len(value) > 32 {
		value = value[:32]
	}
	for _, ch := range value {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '-' && ch != '_' {
			return ""
		}
	}
	return value
}

func parseAgentInt(input string) int {
	value, _ := strconv.Atoi(strings.TrimSpace(input))
	return value
}

func isAgentMentionElement(attrs map[string]string) bool {
	kind := strings.ToLower(firstNonEmpty(attrs["data-type"], attrs["data-node-type"], attrs["class"]))
	return strings.Contains(kind, "mention") || attrs["data-user-id"] != ""
}

func AgentRichDocumentPlainText(doc AgentRichDocument) string {
	var builder strings.Builder
	for index, block := range doc.Blocks {
		if index > 0 {
			builder.WriteByte('\n')
		}
		writeAgentRichPlain(&builder, block, 0)
	}
	return normalizePlainText(builder.String())
}

func writeAgentRichPlain(builder *strings.Builder, node AgentRichNode, depth int) {
	switch node.Type {
	case "text", "mention":
		builder.WriteString(node.Text)
	case "hard_break":
		builder.WriteByte('\n')
	case "image":
		label := node.Attrs["alt"]
		if label == "" {
			label = "图片"
		}
		builder.WriteString("[" + label + "]")
	case "code_block":
		builder.WriteString(node.Text)
	case "bullet_list", "ordered_list":
		for i, child := range node.Children {
			if i > 0 {
				builder.WriteByte('\n')
			}
			if node.Type == "ordered_list" {
				builder.WriteString(strconv.Itoa(i+1) + ". ")
			} else {
				builder.WriteString("- ")
			}
			writeAgentRichPlain(builder, child, depth+1)
		}
	default:
		for _, child := range node.Children {
			writeAgentRichPlain(builder, child, depth)
		}
	}
}

func RenderAgentRichDocumentHTML(doc AgentRichDocument, includeImages bool) string {
	var builder strings.Builder
	for _, block := range doc.Blocks {
		writeAgentRichHTML(&builder, block, includeImages)
	}
	return strings.TrimSpace(builder.String())
}

func writeAgentRichHTML(builder *strings.Builder, node AgentRichNode, includeImages bool) {
	switch node.Type {
	case "text":
		value := html.EscapeString(node.Text)
		value = wrapAgentHTMLMarks(value, node.Marks)
		builder.WriteString(value)
	case "mention":
		value := html.EscapeString(node.Text)
		if value == "" {
			value = "@未知用户"
		}
		builder.WriteString(`<span class="mention">` + wrapAgentHTMLMarks(value, node.Marks) + `</span>`)
	case "hard_break":
		builder.WriteString("<br />")
	case "horizontal_rule":
		builder.WriteString("<hr />")
	case "image":
		if !includeImages {
			return
		}
		src := sanitizeAgentMediaURL(node.Attrs["src"])
		if src == "" {
			return
		}
		builder.WriteString(`<img src="` + html.EscapeString(src) + `"`)
		if alt := node.Attrs["alt"]; alt != "" {
			builder.WriteString(` alt="` + html.EscapeString(alt) + `"`)
		}
		if title := node.Attrs["title"]; title != "" {
			builder.WriteString(` title="` + html.EscapeString(title) + `"`)
		}
		builder.WriteString(" />")
	case "paragraph":
		builder.WriteString("<p>")
		for _, child := range node.Children {
			writeAgentRichHTML(builder, child, includeImages)
		}
		builder.WriteString("</p>")
	case "heading":
		level := parseAgentInt(node.Attrs["level"])
		if level < 1 || level > 6 {
			level = 2
		}
		builder.WriteString(fmt.Sprintf("<h%d>", level))
		for _, child := range node.Children {
			writeAgentRichHTML(builder, child, includeImages)
		}
		builder.WriteString(fmt.Sprintf("</h%d>", level))
	case "blockquote":
		builder.WriteString("<blockquote>")
		for _, child := range node.Children {
			writeAgentRichHTML(builder, child, includeImages)
		}
		builder.WriteString("</blockquote>")
	case "bullet_list", "ordered_list":
		tag := "ul"
		if node.Type == "ordered_list" {
			tag = "ol"
		}
		builder.WriteString("<" + tag + ">")
		for _, child := range node.Children {
			writeAgentRichHTML(builder, child, includeImages)
		}
		builder.WriteString("</" + tag + ">")
	case "list_item":
		builder.WriteString("<li>")
		for _, child := range node.Children {
			writeAgentRichHTML(builder, child, includeImages)
		}
		builder.WriteString("</li>")
	case "code_block":
		builder.WriteString("<pre><code>")
		builder.WriteString(html.EscapeString(node.Text))
		builder.WriteString("</code></pre>")
	default:
		for _, child := range node.Children {
			writeAgentRichHTML(builder, child, includeImages)
		}
	}
}

func wrapAgentHTMLMarks(value string, marks []AgentRichMark) string {
	for index := len(marks) - 1; index >= 0; index-- {
		mark := marks[index]
		switch mark.Type {
		case "bold":
			value = "<strong>" + value + "</strong>"
		case "italic":
			value = "<em>" + value + "</em>"
		case "underline":
			value = "<u>" + value + "</u>"
		case "strike":
			value = "<s>" + value + "</s>"
		case "code":
			value = "<code>" + value + "</code>"
		case "color":
			if color := normalizeAgentColor(mark.Value); color != "" {
				value = `<span style="color:` + color + `">` + value + `</span>`
			}
		case "link":
			if href := sanitizeAgentLinkURL(mark.Value); href != "" {
				value = `<a href="` + html.EscapeString(href) + `" rel="noopener noreferrer">` + value + `</a>`
			}
		}
	}
	return value
}

func RenderAgentRichDocumentBBCode(doc AgentRichDocument, includeImages bool) string {
	var builder strings.Builder
	for index, block := range doc.Blocks {
		if index > 0 {
			builder.WriteByte('\n')
		}
		writeAgentRichBBCode(&builder, block, includeImages)
	}
	return normalizePlainText(builder.String())
}

func writeAgentRichBBCode(builder *strings.Builder, node AgentRichNode, includeImages bool) {
	switch node.Type {
	case "text", "mention":
		value := escapeAgentBBCodeText(node.Text)
		for index := len(node.Marks) - 1; index >= 0; index-- {
			mark := node.Marks[index]
			switch mark.Type {
			case "bold":
				value = "[b]" + value + "[/b]"
			case "italic":
				value = "[i]" + value + "[/i]"
			case "underline":
				value = "[u]" + value + "[/u]"
			case "strike":
				value = "[s]" + value + "[/s]"
			case "code":
				value = "[code]" + value + "[/code]"
			case "color":
				if color := normalizeAgentColor(mark.Value); color != "" {
					value = "[color=" + color + "]" + value + "[/color]"
				}
			case "link":
				if href := sanitizeAgentLinkURL(mark.Value); href != "" {
					value = "[url=" + escapeAgentBBCodeAttribute(href) + "]" + value + "[/url]"
				}
			}
		}
		builder.WriteString(value)
	case "hard_break":
		builder.WriteByte('\n')
	case "horizontal_rule":
		builder.WriteString("----------------")
	case "image":
		if !includeImages {
			return
		}
		if src := sanitizeAgentMediaURL(node.Attrs["src"]); src != "" {
			builder.WriteString("[img]" + escapeAgentBBCodeAttribute(src) + "[/img]")
		}
	case "code_block":
		builder.WriteString("[code]" + escapeAgentBBCodeText(node.Text) + "[/code]")
	case "bullet_list", "ordered_list":
		builder.WriteString("[list")
		if node.Type == "ordered_list" {
			builder.WriteString("=1")
		}
		builder.WriteString("]")
		for _, child := range node.Children {
			writeAgentRichBBCode(builder, child, includeImages)
		}
		builder.WriteString("[/list]")
	case "list_item":
		builder.WriteString("[*]")
		for _, child := range node.Children {
			writeAgentRichBBCode(builder, child, includeImages)
		}
	case "blockquote":
		builder.WriteString("[quote]")
		for _, child := range node.Children {
			writeAgentRichBBCode(builder, child, includeImages)
		}
		builder.WriteString("[/quote]")
	default:
		for _, child := range node.Children {
			writeAgentRichBBCode(builder, child, includeImages)
		}
	}
}

func escapeAgentBBCodeAttribute(input string) string {
	input = strings.ReplaceAll(input, "[", "%5B")
	input = strings.ReplaceAll(input, "]", "%5D")
	input = strings.ReplaceAll(input, "\r", "")
	input = strings.ReplaceAll(input, "\n", "")
	return input
}

func escapeAgentBBCodeText(input string) string {
	// BBCode has no universal escape syntax. Replacing opening brackets prevents
	// user content from injecting formatter directives while keeping readability.
	return strings.ReplaceAll(input, "[", "［")
}

func MergeAgentRichDocuments(documents []AgentRichDocument) AgentRichDocument {
	merged := AgentRichDocument{Type: "document", Blocks: []AgentRichNode{}}
	for index, doc := range documents {
		if index > 0 && len(merged.Blocks) > 0 {
			merged.Blocks = append(merged.Blocks, AgentRichNode{Type: "paragraph", Children: []AgentRichNode{{Type: "hard_break"}}})
		}
		merged.Blocks = append(merged.Blocks, doc.Blocks...)
	}
	return merged
}
