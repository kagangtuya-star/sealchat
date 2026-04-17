package contentstats

import (
	"encoding/json"
	"html"
	"strconv"
	"strings"
	"unicode/utf8"

	htmlparser "golang.org/x/net/html"
	htmlatom "golang.org/x/net/html/atom"
)

type tiptapNode struct {
	Type    string         `json:"type"`
	Text    string         `json:"text"`
	Content []*tiptapNode  `json:"content"`
	Attrs   map[string]any `json:"attrs"`
}

func (n *tiptapNode) attrString(key string) string {
	if n == nil || n.Attrs == nil {
		return ""
	}
	value, ok := n.Attrs[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return ""
	}
}

type visibleTextWriter struct {
	sb             strings.Builder
	lastWasNewline bool
}

func (w *visibleTextWriter) write(text string) {
	if text == "" {
		return
	}
	text = strings.ReplaceAll(text, "\u00a0", " ")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	w.sb.WriteString(text)
	w.lastWasNewline = strings.HasSuffix(text, "\n")
}

func (w *visibleTextWriter) newline() {
	if w.sb.Len() == 0 || w.lastWasNewline {
		return
	}
	w.sb.WriteByte('\n')
	w.lastWasNewline = true
}

func (w *visibleTextWriter) String() string {
	return w.sb.String()
}

// CountVisibleTextChars returns count of user-visible text characters in stored message content.
func CountVisibleTextChars(input string) int {
	if input == "" {
		return 0
	}
	if count, ok := countVisibleTipTapText(input); ok {
		return count
	}
	if looksLikeStructuredHTML(input) {
		return countVisibleHTMLText(input)
	}
	return countVisiblePlainText(input)
}

func countVisiblePlainText(input string) int {
	normalized := strings.ReplaceAll(input, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return utf8.RuneCountInString(normalized)
}

func looksLikeStructuredHTML(input string) bool {
	return strings.ContainsAny(input, "<>&")
}

func countVisibleHTMLText(input string) int {
	wrapper := &htmlparser.Node{Type: htmlparser.ElementNode, DataAtom: htmlatom.Div, Data: "div"}
	nodes, err := htmlparser.ParseFragment(strings.NewReader(input), wrapper)
	if err != nil {
		return countVisiblePlainText(html.UnescapeString(input))
	}
	writer := &visibleTextWriter{}
	for _, node := range nodes {
		writeHTMLVisibleText(writer, node)
	}
	return utf8.RuneCountInString(strings.TrimRight(writer.String(), "\n"))
}

func writeHTMLVisibleText(writer *visibleTextWriter, node *htmlparser.Node) {
	if writer == nil || node == nil {
		return
	}
	switch node.Type {
	case htmlparser.TextNode:
		writer.write(html.UnescapeString(node.Data))
	case htmlparser.ElementNode:
		tag := strings.ToLower(strings.TrimSpace(node.Data))
		switch tag {
		case "img", "image":
			return
		case "br":
			writer.newline()
			return
		case "at":
			writer.write(renderAtNode(node))
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			writeHTMLVisibleText(writer, child)
		}
		if shouldInsertStructuredNewline(tag) {
			writer.newline()
		}
	}
}

func renderAtNode(node *htmlparser.Node) string {
	if node == nil {
		return ""
	}
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
	display := strings.TrimSpace(name)
	if display == "" {
		display = strings.TrimSpace(id)
	}
	if display == "" {
		return ""
	}
	if display == "all" {
		display = "全体成员"
	}
	return "@" + display
}

func countVisibleTipTapText(input string) (int, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || !looksLikeTipTapJSON(trimmed) {
		return 0, false
	}
	var doc tiptapNode
	if err := json.Unmarshal([]byte(trimmed), &doc); err != nil {
		return 0, false
	}
	if strings.ToLower(strings.TrimSpace(doc.Type)) != "doc" {
		return 0, false
	}
	writer := &visibleTextWriter{}
	writeTipTapVisibleText(writer, &doc)
	return utf8.RuneCountInString(strings.TrimRight(writer.String(), "\n")), true
}

func looksLikeTipTapJSON(input string) bool {
	trimmed := strings.TrimSpace(input)
	return strings.HasPrefix(trimmed, "{") && strings.Contains(trimmed, `"type":"doc"`)
}

func writeTipTapVisibleText(writer *visibleTextWriter, node *tiptapNode) {
	if writer == nil || node == nil {
		return
	}
	nodeType := strings.ToLower(strings.TrimSpace(node.Type))
	switch nodeType {
	case "doc":
		for _, child := range node.Content {
			writeTipTapVisibleText(writer, child)
		}
		return
	case "text":
		writer.write(node.Text)
		return
	case "hardbreak":
		writer.newline()
		return
	case "image":
		return
	case "satorimention":
		writer.write(renderTipTapMention(node.attrString("name"), node.attrString("id")))
		return
	case "mention":
		writer.write(renderTipTapMention(node.attrString("label"), node.attrString("text"), node.attrString("name"), node.attrString("id")))
		return
	}
	for _, child := range node.Content {
		writeTipTapVisibleText(writer, child)
	}
	if shouldInsertStructuredNewline(nodeType) {
		writer.newline()
	}
}

func renderTipTapMention(parts ...string) string {
	for _, part := range parts {
		display := strings.TrimSpace(part)
		if display == "" {
			continue
		}
		if display == "all" {
			display = "全体成员"
		}
		if strings.HasPrefix(display, "@") {
			return display
		}
		return "@" + display
	}
	return ""
}

func shouldInsertStructuredNewline(tag string) bool {
	switch tag {
	case "p", "div", "blockquote", "li",
		"paragraph", "heading", "listitem", "codeblock", "table", "tablerow", "tablecell":
		return true
	default:
		return false
	}
}
