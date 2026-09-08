package service

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	docx "github.com/mmonterroca/docxgo/v2"
	"github.com/mmonterroca/docxgo/v2/domain"
)

const docxContentMaxWidthPx = 605 // approximately 6.3in at 96 DPI

type docxFormatter struct{}

func (docxFormatter) Ext() string { return "docx" }

func (docxFormatter) ContentType() string {
	return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
}

func (docxFormatter) Build(payload *ExportPayload) ([]byte, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload 为空")
	}
	doc := docx.NewDocument()
	resolver, err := newDocxImageResolver(payload.ChannelID)
	if err != nil {
		return nil, fmt.Errorf("创建 DOCX 图片临时目录失败: %w", err)
	}
	defer resolver.Close()
	for i := range payload.Messages {
		if err := renderDocxMessage(doc, payload, &payload.Messages[i], resolver); err != nil {
			return nil, err
		}
	}
	var out bytes.Buffer
	if _, err := doc.WriteTo(&out); err != nil {
		return nil, fmt.Errorf("生成 DOCX 失败: %w", err)
	}
	return out.Bytes(), nil
}

func renderDocxMessage(doc domain.Document, payload *ExportPayload, msg *ExportMessage, resolver *docxImageResolver) error {
	if doc == nil || payload == nil || msg == nil {
		return nil
	}
	para, err := doc.AddParagraph()
	if err != nil {
		return err
	}
	roleColor := resolveDocxMessageColor(payload, msg)
	roleName := resolveBBCodeSenderName(payload, msg)
	if !payload.WithoutTimestamp {
		run, err := para.AddRun()
		if err != nil {
			return err
		}
		if err := run.SetText("[" + msg.CreatedAt.Format("2006-01-02 15:04:05") + "] "); err != nil {
			return err
		}
		_ = run.SetColor(domain.Color{R: 150, G: 150, B: 150})
		_ = run.SetSize(18) // 9pt
	}
	nameRun, err := para.AddRun()
	if err != nil {
		return err
	}
	if err := nameRun.SetText("<" + roleName + "> "); err != nil {
		return err
	}
	if err := nameRun.SetBold(true); err != nil {
		return err
	}
	if err := nameRun.SetColor(roleColor); err != nil {
		return err
	}

	if msg.IsArchived {
		if err := addDocxRun(para, "[已归档] ", nil, roleColor); err != nil {
			return err
		}
	}
	if msg.IsWhisper {
		if label := formatWhisperTargets(msg.WhisperTargets); label != "" {
			if err := addDocxRun(para, label+" ", nil, roleColor); err != nil {
				return err
			}
		}
	}
	docContent := msg.ContentRich
	if docContent == nil {
		built := buildRichDocumentForExport(msg.Content, payload.IncludeImages)
		docContent = &built
	}
	blocks := cloneRichBlocks(docContent.Blocks)
	applyDocxOOCWrapper(msg, blocks)
	for _, block := range blocks {
		var bodyPara domain.Paragraph
		if block.Type != "bullet_list" && block.Type != "ordered_list" {
			bodyPara, err = doc.AddParagraph()
			if err != nil {
				return err
			}
		}
		if err := renderDocxBlock(doc, bodyPara, block, resolver, roleColor, 0); err != nil {
			return err
		}
	}
	return nil
}

func addDocxRun(para domain.Paragraph, text string, marks []AgentRichMark, fallback domain.Color) error {
	parts := strings.Split(text, "\n")
	for index, part := range parts {
		if part != "" {
			run, err := para.AddRun()
			if err != nil {
				return err
			}
			if err := run.SetText(part); err != nil {
				return err
			}
			if err := applyDocxRunMarks(run, marks, fallback); err != nil {
				return err
			}
		}
		if index < len(parts)-1 {
			run, err := para.AddRun()
			if err != nil {
				return err
			}
			if err := run.AddBreak(domain.BreakTypeLine); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderDocxBlock(doc domain.Document, para domain.Paragraph, node AgentRichNode, resolver *docxImageResolver, fallback domain.Color, depth int) error {
	switch node.Type {
	case "paragraph", "heading", "blockquote":
		if node.Type == "heading" {
			level := clampInt(parseInt(node.Attrs["level"]), 1, 6)
			_ = para.SetStyle("Heading" + strconv.Itoa(level))
		}
		if node.Type == "blockquote" {
			_ = para.SetIndentLeft(360 + depth*180)
		}
		for _, child := range node.Children {
			if err := renderDocxInline(para, child, resolver, fallback, depth); err != nil {
				return err
			}
		}
	case "list_item":
		return renderDocxListItem(doc, para, node, resolver, fallback, depth, "• ")
	case "bullet_list":
		for _, child := range node.Children {
			itemPara, err := doc.AddParagraph()
			if err != nil {
				return err
			}
			if err := renderDocxListItem(doc, itemPara, child, resolver, fallback, depth, "• "); err != nil {
				return err
			}
		}
	case "ordered_list":
		for index, child := range node.Children {
			itemPara, err := doc.AddParagraph()
			if err != nil {
				return err
			}
			if err := renderDocxListItem(doc, itemPara, child, resolver, fallback, depth, fmt.Sprintf("%d. ", index+1)); err != nil {
				return err
			}
		}
	case "code_block":
		if node.Text != "" {
			return addDocxRun(para, node.Text, []AgentRichMark{{Type: "code"}}, fallback)
		}
	case "horizontal_rule":
		return addDocxRun(para, "────────────────", nil, fallback)
	default:
		return renderDocxInline(para, node, resolver, fallback, depth)
	}
	return nil
}

func renderDocxListItem(doc domain.Document, para domain.Paragraph, node AgentRichNode, resolver *docxImageResolver, fallback domain.Color, depth int, prefix string) error {
	if node.Type != "list_item" {
		return renderDocxBlock(doc, para, node, resolver, fallback, depth+1)
	}
	if depth > 0 {
		prefix = strings.Repeat("  ", depth) + prefix
		_ = para.SetIndentLeft(360 + depth*180)
	}
	if err := addDocxRun(para, prefix, nil, fallback); err != nil {
		return err
	}
	for _, child := range node.Children {
		switch child.Type {
		case "bullet_list", "ordered_list":
			if err := renderDocxBlock(doc, nil, child, resolver, fallback, depth+1); err != nil {
				return err
			}
		default:
			if err := renderDocxInline(para, child, resolver, fallback, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderDocxInline(para domain.Paragraph, node AgentRichNode, resolver *docxImageResolver, fallback domain.Color, depth int) error {
	switch node.Type {
	case "text", "mention":
		if node.Text == "" {
			return nil
		}
		if href := richLinkMark(node.Marks); href != "" {
			run, err := para.AddHyperlink(href, node.Text)
			if err != nil {
				return err
			}
			return applyDocxRunMarks(run, appendAgentMarks(node.Marks, AgentRichMark{Type: "underline"}), fallback)
		}
		return addDocxRun(para, node.Text, node.Marks, fallback)
	case "hard_break":
		run, err := para.AddRun()
		if err != nil {
			return err
		}
		return run.AddBreak(domain.BreakTypeLine)
	case "image":
		if resolver == nil {
			return nil
		}
		img, err := resolver.Resolve(node)
		if err != nil {
			return err
		}
		if img.Placeholder != "" {
			return addDocxRun(para, img.Placeholder, nil, fallback)
		}
		layout := exportImageLayout{Width: img.LayoutWidth, Height: img.LayoutHeight}
		width, height := imageDisplaySize(node, img.Width, img.Height, layout)
		_, err = para.AddImageFromBytesWithSize(img.Data, img.Format, domain.NewImageSize(width, height))
		if err != nil {
			return fmt.Errorf("docx image embed failed: %w", err)
		}
	case "paragraph", "heading", "blockquote", "list_item":
		for _, child := range node.Children {
			if err := renderDocxInline(para, child, resolver, fallback, depth); err != nil {
				return err
			}
		}
	default:
		for _, child := range node.Children {
			if err := renderDocxInline(para, child, resolver, fallback, depth); err != nil {
				return err
			}
		}
	}
	return nil
}

func applyDocxRunMarks(run domain.Run, marks []AgentRichMark, fallback domain.Color) error {
	if run == nil {
		return nil
	}
	if err := run.SetColor(fallback); err != nil {
		return err
	}
	for _, mark := range marks {
		switch mark.Type {
		case "bold":
			if err := run.SetBold(true); err != nil {
				return err
			}
		case "italic":
			if err := run.SetItalic(true); err != nil {
				return err
			}
		case "underline", "link":
			if err := run.SetUnderline(domain.UnderlineSingle); err != nil {
				return err
			}
		case "strike":
			if err := run.SetStrike(true); err != nil {
				return err
			}
		case "color":
			if color, ok := parseDocxColor(mark.Value); ok {
				if err := run.SetColor(color); err != nil {
					return err
				}
			}
		case "fontFamily":
			font := domain.Font{Name: mark.Value, EastAsia: mark.Value, CS: mark.Value}
			if err := run.SetFont(font); err != nil {
				return err
			}
		case "fontSize":
			if half := docxHalfPoints(mark.Value); half > 0 {
				if err := run.SetSize(half); err != nil {
					return err
				}
			}
		case "code":
			font := domain.Font{Name: "Consolas", EastAsia: "等线", CS: "Consolas"}
			_ = run.SetFont(font)
		}
	}
	return nil
}

func resolveDocxMessageColor(payload *ExportPayload, msg *ExportMessage) domain.Color {
	if msg == nil {
		return domain.Color{R: 17, G: 17, B: 17}
	}
	for _, value := range []string{lookupBBCodeColorOverride(payload, msg), msg.SenderColor} {
		if color, ok := parseDocxColor(value); ok {
			return color
		}
	}
	return domain.Color{R: 17, G: 17, B: 17}
}

func parseDocxColor(value string) (domain.Color, bool) {
	normalized := normalizeAgentColor(value)
	if normalized == "" {
		normalized = sanitizeBBCodeColor(value, "")
	}
	if strings.HasPrefix(normalized, "#") {
		normalized = strings.TrimPrefix(normalized, "#")
	}
	if len(normalized) != 6 {
		return domain.Color{}, false
	}
	r, e1 := strconv.ParseUint(normalized[0:2], 16, 8)
	g, e2 := strconv.ParseUint(normalized[2:4], 16, 8)
	b, e3 := strconv.ParseUint(normalized[4:6], 16, 8)
	return domain.Color{R: uint8(r), G: uint8(g), B: uint8(b)}, e1 == nil && e2 == nil && e3 == nil
}

func docxHalfPoints(raw string) int {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 0
	}
	value := strings.TrimSuffix(strings.TrimSuffix(raw, "px"), "pt")
	f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || f <= 0 {
		return 0
	}
	if strings.HasSuffix(raw, "px") {
		f = f * 72 / 96
	}
	return int(f*2 + 0.5)
}

func parseInt(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}

func richLinkMark(marks []AgentRichMark) string {
	for _, mark := range marks {
		if mark.Type == "link" {
			return mark.Value
		}
	}
	return ""
}

func imageDisplaySize(node AgentRichNode, width, height int, layouts ...exportImageLayout) (int, int) {
	if width <= 0 || height <= 0 {
		return 1, 1
	}
	requestedWidth := 0
	requestedHeight := 0
	if len(layouts) > 0 {
		requestedWidth = layouts[0].Width
		requestedHeight = layouts[0].Height
	}
	if requestedWidth <= 0 {
		requestedWidth = parseInt(node.Attrs["width"])
	}
	if requestedHeight <= 0 {
		requestedHeight = parseInt(node.Attrs["height"])
	}
	if requestedWidth > 0 && requestedHeight > 0 {
		width, height = requestedWidth, requestedHeight
	} else if requestedWidth > 0 {
		ratio := float64(height) / float64(width)
		width = requestedWidth
		height = int(float64(width)*ratio + 0.5)
	} else if requestedHeight > 0 {
		ratio := float64(width) / float64(height)
		height = requestedHeight
		width = int(float64(height)*ratio + 0.5)
	}
	if width > docxContentMaxWidthPx {
		height = int(float64(height) * float64(docxContentMaxWidthPx) / float64(width))
		width = docxContentMaxWidthPx
	}
	if height < 1 {
		height = 1
	}
	return width, height
}

func cloneRichBlocks(nodes []AgentRichNode) []AgentRichNode {
	result := make([]AgentRichNode, len(nodes))
	for i, node := range nodes {
		result[i] = node
		result[i].Attrs = cloneStringMap(node.Attrs)
		result[i].Marks = cloneAgentMarks(node.Marks)
		result[i].Children = cloneRichBlocks(node.Children)
	}
	return result
}

func applyDocxOOCWrapper(msg *ExportMessage, blocks []AgentRichNode) {
	if msg == nil || msg.WithoutOOCParentheses || !strings.EqualFold(msg.IcMode, "ooc") || len(blocks) == 0 {
		return
	}
	text := strings.TrimSpace(richNodePlainText(blocks))
	if strings.HasPrefix(text, "（") && strings.HasSuffix(text, "）") || strings.HasPrefix(text, "(") && strings.HasSuffix(text, ")") {
		return
	}
	for i := range blocks {
		if richBlockHasContent(blocks[i]) {
			prependRichBlockText(&blocks[i], "（")
			break
		}
	}
	for i := len(blocks) - 1; i >= 0; i-- {
		if richBlockHasContent(blocks[i]) {
			appendRichBlockText(&blocks[i], "）")
			break
		}
	}
}

func richBlockHasContent(node AgentRichNode) bool {
	return strings.TrimSpace(node.Text) != "" || len(node.Children) > 0
}

func prependRichBlockText(node *AgentRichNode, text string) {
	if node == nil {
		return
	}
	switch node.Type {
	case "code_block":
		node.Text = text + node.Text
		return
	case "bullet_list", "ordered_list":
		for i := range node.Children {
			child := &node.Children[i]
			if child.Type == "list_item" && richBlockHasContent(*child) {
				prependRichBlockText(child, text)
				return
			}
		}
		return
	case "list_item":
		for i := range node.Children {
			child := &node.Children[i]
			if richBlockHasContent(*child) {
				prependRichBlockText(child, text)
				return
			}
		}
		return
	}
	node.Children = append([]AgentRichNode{{Type: "text", Text: text}}, node.Children...)
}

func appendRichBlockText(node *AgentRichNode, text string) {
	if node == nil {
		return
	}
	switch node.Type {
	case "code_block":
		node.Text += text
		return
	case "bullet_list", "ordered_list":
		for i := len(node.Children) - 1; i >= 0; i-- {
			child := &node.Children[i]
			if child.Type == "list_item" && richBlockHasContent(*child) {
				appendRichBlockText(child, text)
				return
			}
		}
		return
	case "list_item":
		for i := len(node.Children) - 1; i >= 0; i-- {
			child := &node.Children[i]
			if richBlockHasContent(*child) {
				appendRichBlockText(child, text)
				return
			}
		}
		return
	}
	node.Children = append(node.Children, AgentRichNode{Type: "text", Text: text})
}
