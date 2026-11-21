package service

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// TipTap JSON uses numeric spans and one pixel width per spanned column.
func tipTapTableNumber(value any, max int, fallback int) int {
	number, ok := value.(float64)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) || number < 1 || number > float64(max) || math.Trunc(number) != number {
		return fallback
	}
	return int(number)
}

func renderTipTapTableStart(buf *strings.Builder, node *tiptapNode) {
	var columns strings.Builder
	total, fixed := 0.0, true
	if len(node.Content) > 0 && node.Content[0] != nil {
		for _, cell := range node.Content[0].Content {
			if cell == nil {
				continue
			}
			span := tipTapTableNumber(cell.Attrs["colspan"], 1000, 1)
			widths, _ := cell.Attrs["colwidth"].([]any)
			for index := 0; index < span; index++ {
				width := 0.0
				if index < len(widths) {
					if value, ok := widths[index].(float64); ok && !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 1 && value <= 10000 {
						width = value
					}
				}
				property := "width"
				if width == 0 {
					width, property, fixed = 80, "min-width", false
				}
				total += width
				fmt.Fprintf(&columns, `<col style="%s: %spx" />`, property, strconv.FormatFloat(width, 'f', -1, 64))
			}
		}
	}
	buf.WriteString(`<div class="tiptap-table-wrapper"><table class="tiptap-table"`)
	if total > 0 {
		property := "min-width"
		if fixed {
			property = "width"
		}
		fmt.Fprintf(buf, ` style="%s: %spx"`, property, strconv.FormatFloat(total, 'f', -1, 64))
	}
	buf.WriteString(">")
	if columns.Len() > 0 {
		buf.WriteString("<colgroup>" + columns.String() + "</colgroup>")
	}
	buf.WriteString("<tbody>")
}

func renderTipTapTableHTML(buf *strings.Builder, node *tiptapNode) {
	renderTipTapTableStart(buf, node)
	for _, row := range node.Content {
		renderTipTapHTML(buf, row)
	}
	buf.WriteString("</tbody></table></div>")
}

// The existing LOG projection has string-valued attrs; decode only native table attrs.
func logTableNodeAttrs(node AgentRichNode) map[string]any {
	attrs := map[string]any{}
	for _, key := range []string{"colspan", "rowspan", "colwidth"} {
		var value any
		if json.Unmarshal([]byte(node.Attrs[key]), &value) == nil {
			attrs[key] = value
		}
	}
	return attrs
}

func renderLogTableStart(buf *strings.Builder, node AgentRichNode) {
	layout := &tiptapNode{}
	if len(node.Children) > 0 {
		row := &tiptapNode{}
		for _, cell := range node.Children[0].Children {
			row.Content = append(row.Content, &tiptapNode{Attrs: logTableNodeAttrs(cell)})
		}
		layout.Content = []*tiptapNode{row}
	}
	renderTipTapTableStart(buf, layout)
}
