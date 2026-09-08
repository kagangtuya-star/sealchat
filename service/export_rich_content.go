package service

import (
	"encoding/json"
	"regexp"
	"strings"
)

// buildRichDocumentForExport converts the persisted message content directly
// into the neutral AgentRich AST. It intentionally does not use HTML as an
// intermediate representation; legacy HTML is handled by the existing
// controlled parser only when the content itself is HTML.
func buildRichDocumentForExport(content string, includeImages bool) AgentRichDocument {
	content = strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n")
	if strings.TrimSpace(content) == "" {
		return AgentRichDocument{Type: "document", Blocks: []AgentRichNode{}}
	}
	if isLikelyHTMLContent(content) && findTipTapJSONStart(content) < 0 {
		return ParseAgentRichHTML(content, "supported", imageModeForExport(includeImages))
	}

	var blocks []AgentRichNode
	remaining := content
	for {
		start := findTipTapJSONStart(remaining)
		if start < 0 {
			blocks = appendRichPlain(blocks, remaining, includeImages)
			break
		}
		blocks = appendRichPlain(blocks, remaining[:start], includeImages)
		part := remaining[start:]
		end := findJSONEnd(part)
		if end < 0 {
			// Keep malformed data visible as ordinary text; the existing TXT/HTML
			// scanners use the same non-fatal behavior.
			blocks = appendRichPlain(blocks, part, includeImages)
			break
		}
		var node tiptapNode
		if err := json.Unmarshal([]byte(part[:end]), &node); err != nil || !strings.EqualFold(strings.TrimSpace(node.Type), "doc") {
			blocks = appendRichPlain(blocks, part[:end], includeImages)
			remaining = part[end:]
			continue
		}
		blocks = append(blocks, richBlocksFromTipTap(&node, includeImages)...)
		remaining = part[end:]
	}
	return AgentRichDocument{Type: "document", Blocks: normalizeAgentRichBlocks(blocks)}
}

func imageModeForExport(include bool) string {
	if include {
		return "meta"
	}
	return "omit"
}

func appendRichPlain(blocks []AgentRichNode, raw string, includeImages bool) []AgentRichNode {
	if raw == "" {
		return blocks
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return blocks
	}
	lines := strings.Split(raw, "\n")
	children := make([]AgentRichNode, 0, len(lines))
	for index, line := range lines {
		children = append(children, parseQuickRichInline(line, includeImages)...)
		if index < len(lines)-1 {
			children = append(children, AgentRichNode{Type: "hard_break"})
		}
	}
	if len(children) > 0 {
		blocks = append(blocks, AgentRichNode{Type: "paragraph", Children: children})
	}
	return blocks
}

var (
	quickRichCodeRE   = regexp.MustCompile("`([^`\\n]+)`")
	quickRichLinkRE   = regexp.MustCompile(`\[([^\]\n]+)\]\((https?://[^\s)]+)\)`)
	quickRichBoldRE   = regexp.MustCompile(`\*\*([^\n*][^*\n]*?)\*\*`)
	quickRichItalicRE = regexp.MustCompile(`(^|[^*])\*([^*\n]+)\*`)
	quickRichAtRE     = regexp.MustCompile(`<at\s+id="([^"]+)"(?:\s+name="([^"]*)")?\s*/>`)
)

func parseQuickRichInline(raw string, includeImages bool) []AgentRichNode {
	if shouldDisableInlineCodeForBotCommand(raw) {
		return parseRichTextAndImages(raw, nil, includeImages)
	}
	return parseRichSpecialTokens(raw, nil, includeImages)
}

func parseRichSpecialTokens(raw string, inherited []AgentRichMark, includeImages bool) []AgentRichNode {
	if raw == "" {
		return nil
	}
	var out []AgentRichNode
	for len(raw) > 0 {
		bestStart := len(raw)
		bestEnd := -1
		bestKind := ""
		patterns := []struct {
			name string
			re   *regexp.Regexp
		}{
			{"at", quickRichAtRE},
			{"code", quickRichCodeRE},
			{"link", quickRichLinkRE},
			{"bold", quickRichBoldRE},
			{"italic", quickRichItalicRE},
		}
		for _, item := range patterns {
			loc := item.re.FindStringIndex(raw)
			if loc == nil || loc[0] >= bestStart {
				continue
			}
			bestStart, bestEnd, bestKind = loc[0], loc[1], item.name
		}
		if bestKind == "" {
			out = append(out, parseRichTextAndImages(raw, inherited, includeImages)...)
			break
		}
		if bestStart > 0 {
			out = append(out, parseRichTextAndImages(raw[:bestStart], inherited, includeImages)...)
		}
		segment := raw[bestStart:bestEnd]
		switch bestKind {
		case "at":
			m := quickRichAtRE.FindStringSubmatch(segment)
			label := "@" + m[1]
			if len(m) > 2 && strings.TrimSpace(m[2]) != "" {
				label = "@" + strings.TrimSpace(m[2])
			}
			out = append(out, AgentRichNode{Type: "mention", Text: label, Attrs: map[string]string{"id": m[1]}, Marks: cloneAgentMarks(inherited)})
		case "code":
			m := quickRichCodeRE.FindStringSubmatch(segment)
			out = append(out, AgentRichNode{Type: "text", Text: m[1], Marks: appendAgentMarks(inherited, AgentRichMark{Type: "code"})})
		case "link":
			m := quickRichLinkRE.FindStringSubmatch(segment)
			out = append(out, AgentRichNode{Type: "text", Text: m[1], Marks: appendAgentMarks(inherited, AgentRichMark{Type: "link", Value: m[2]})})
		case "bold":
			m := quickRichBoldRE.FindStringSubmatch(segment)
			out = append(out, parseRichSpecialTokens(m[1], appendAgentMarks(inherited, AgentRichMark{Type: "bold"}), includeImages)...)
		case "italic":
			m := quickRichItalicRE.FindStringSubmatch(segment)
			if len(m) > 2 {
				if m[1] != "" {
					out = append(out, parseRichTextAndImages(m[1], inherited, includeImages)...)
				}
				out = append(out, parseRichSpecialTokens(m[2], appendAgentMarks(inherited, AgentRichMark{Type: "italic"}), includeImages)...)
			}
		}
		raw = raw[bestEnd:]
	}
	return out
}

func parseRichTextAndImages(raw string, marks []AgentRichMark, includeImages bool) []AgentRichNode {
	if raw == "" {
		return nil
	}
	if !includeImages {
		raw = cqImageTokenPattern.ReplaceAllString(raw, "")
	}
	var out []AgentRichNode
	for len(raw) > 0 {
		loc := cqImageTokenPattern.FindStringIndex(raw)
		if loc == nil {
			if raw != "" {
				out = append(out, AgentRichNode{Type: "text", Text: htmlUnescapeDeep(raw), Marks: cloneAgentMarks(marks)})
			}
			break
		}
		if loc[0] > 0 {
			out = append(out, AgentRichNode{Type: "text", Text: htmlUnescapeDeep(raw[:loc[0]]), Marks: cloneAgentMarks(marks)})
		}
		if includeImages {
			segment := raw[loc[0]:loc[1]]
			attrs := map[string]string{"src": segment}
			out = append(out, AgentRichNode{Type: "image", Attrs: attrs})
		}
		raw = raw[loc[1]:]
	}
	return out
}

func richBlocksFromTipTap(root *tiptapNode, includeImages bool) []AgentRichNode {
	if root == nil {
		return nil
	}
	var out []AgentRichNode
	for _, child := range root.Content {
		if node := richNodeFromTipTap(child, includeImages); node != nil {
			out = append(out, *node)
		}
	}
	return normalizeAgentRichBlocks(out)
}

func richNodeFromTipTap(node *tiptapNode, includeImages bool) *AgentRichNode {
	if node == nil {
		return nil
	}
	typ := strings.ToLower(strings.TrimSpace(node.Type))
	if typ == "text" {
		text := node.Text
		marks := richMarksFromTipTap(node.Marks)
		for _, mark := range node.Marks {
			if mark != nil && strings.EqualFold(mark.Type, "ruby") {
				if ruby := strings.TrimSpace(mark.attrString("rubyText")); ruby != "" {
					text += "（" + ruby + "）"
				}
				break
			}
		}
		return &AgentRichNode{Type: "text", Text: text, Marks: marks}
	}
	if typ == "image" {
		if !includeImages {
			return nil
		}
		attrs := map[string]string{}
		for _, key := range []string{"src", "dataSrc", "data-src", "attachmentId", "attachment_id", "dataAttachmentId", "data-attachment-id", "alt", "title", "width", "height"} {
			if value := strings.TrimSpace(node.attrString(key)); value != "" {
				attrs[key] = value
			}
		}
		if id := firstNonEmpty(node.attrString("attachment_id"), node.attrString("attachmentId"), node.attrString("dataAttachmentId"), node.attrString("data-attachment-id")); id != "" {
			attrs["attachment_id"] = id
		}
		return &AgentRichNode{Type: "image", Attrs: attrs}
	}
	if typ == "hardbreak" {
		return &AgentRichNode{Type: "hard_break"}
	}
	if typ == "horizontalrule" {
		return &AgentRichNode{Type: "horizontal_rule"}
	}
	if typ == "mention" {
		label := firstNonEmpty(node.attrString("label"), node.Text, node.attrString("name"), node.attrString("text"))
		if label == "" {
			label = firstNonEmpty(node.attrString("id"), node.attrString("userId"))
		}
		if label != "" && !strings.HasPrefix(label, "@") {
			label = "@" + label
		}
		return &AgentRichNode{Type: "mention", Text: label, Attrs: map[string]string{"id": firstNonEmpty(node.attrString("id"), node.attrString("userId"))}}
	}
	result := &AgentRichNode{Type: mapTipTapRichNodeType(typ), Attrs: map[string]string{}}
	if level := node.attrString("level"); level != "" {
		result.Attrs["level"] = level
	}
	if lang := firstNonEmpty(node.attrString("language"), node.attrString("lang")); lang != "" {
		result.Attrs["language"] = lang
	}
	for _, child := range node.Content {
		if converted := richNodeFromTipTap(child, includeImages); converted != nil {
			result.Children = append(result.Children, *converted)
		}
	}
	if typ == "codeblock" {
		result.Type = "code_block"
		result.Text = richNodePlainText(result.Children)
		result.Children = nil
	}
	if result.Type == "text" && result.Text == "" && len(result.Children) == 0 {
		result.Text = node.Text
	}
	if len(result.Attrs) == 0 {
		result.Attrs = nil
	}
	return result
}

func mapTipTapRichNodeType(typ string) string {
	switch typ {
	case "bulletlist", "bullet_list", "tasklist":
		return "bullet_list"
	case "orderedlist", "ordered_list":
		return "ordered_list"
	case "listitem", "list_item", "taskitem":
		return "list_item"
	case "codeblock", "code_block":
		return "code_block"
	case "blockquote":
		return "blockquote"
	case "heading":
		return "heading"
	case "paragraph":
		return "paragraph"
	default:
		return "paragraph"
	}
}

func richMarksFromTipTap(marks []*tiptapMark) []AgentRichMark {
	var out []AgentRichMark
	for _, mark := range marks {
		if mark == nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(mark.Type)) {
		case "bold":
			out = append(out, AgentRichMark{Type: "bold"})
		case "italic":
			out = append(out, AgentRichMark{Type: "italic"})
		case "underline":
			out = append(out, AgentRichMark{Type: "underline"})
		case "strike":
			out = append(out, AgentRichMark{Type: "strike"})
		case "code":
			out = append(out, AgentRichMark{Type: "code"})
		case "link":
			if href := strings.TrimSpace(mark.attrString("href")); href != "" && isSafeQuickLink(href) {
				out = append(out, AgentRichMark{Type: "link", Value: href})
			}
		case "textstyle":
			for _, key := range []string{"color", "fontFamily", "fontSize"} {
				if value := strings.TrimSpace(mark.attrString(key)); value != "" {
					out = append(out, AgentRichMark{Type: key, Value: value})
				}
			}
		case "color":
			if value := strings.TrimSpace(mark.attrString("color")); value != "" {
				out = append(out, AgentRichMark{Type: "color", Value: value})
			}
		}
	}
	return deduplicateAgentMarks(out)
}

func richNodePlainText(nodes []AgentRichNode) string {
	var b strings.Builder
	for _, node := range nodes {
		switch node.Type {
		case "text", "mention":
			b.WriteString(node.Text)
		case "hard_break":
			b.WriteByte('\n')
		default:
			b.WriteString(richNodePlainText(node.Children))
		}
	}
	return b.String()
}

func richDocumentHasContent(doc *AgentRichDocument) bool {
	if doc == nil {
		return false
	}
	var hasContent func([]AgentRichNode) bool
	hasContent = func(nodes []AgentRichNode) bool {
		for _, node := range nodes {
			if node.Type == "image" || node.Type == "horizontal_rule" || strings.TrimSpace(node.Text) != "" {
				return true
			}
			if hasContent(node.Children) {
				return true
			}
		}
		return false
	}
	return hasContent(doc.Blocks)
}
