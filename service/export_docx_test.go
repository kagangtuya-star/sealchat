package service

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"image"
	"image/jpeg"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mmonterroca/docxgo/v2/domain"
	"sealchat/model"
)

func TestDocxFormatterBuildContainsDocumentParts(t *testing.T) {
	color := "#336699"
	payload := &ExportPayload{
		ChannelName: "测试频道",
		GeneratedAt: time.Unix(1700000000, 0),
		Messages: []ExportMessage{{
			SenderIdentityID: "role-a",
			SenderName:       "角色",
			SenderColor:      color,
			CreatedAt:        time.Unix(1700000000, 0),
			ContentRich: &AgentRichDocument{Type: "document", Blocks: []AgentRichNode{{
				Type: "paragraph", Children: []AgentRichNode{{Type: "text", Text: "粗体", Marks: []AgentRichMark{{Type: "bold"}}}},
			}}},
		}},
	}
	payload.ExtraMeta = map[string]interface{}{"text_colorize_bbcode_map": map[string]string{"identity:role-a": color}}
	data, err := (docxFormatter{}).Build(payload)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]string{}
	for _, file := range zr.File {
		body, readErr := file.Open()
		if readErr != nil {
			t.Fatal(readErr)
		}
		value, readErr := io.ReadAll(body)
		_ = body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		parts[file.Name] = string(value)
	}
	if parts["[Content_Types].xml"] == "" || parts["word/document.xml"] == "" {
		t.Fatalf("docx missing required parts: %v", parts)
	}
	xml := parts["word/document.xml"]
	if !strings.Contains(xml, "粗体") || !strings.Contains(xml, "w:b") || !strings.Contains(xml, "336699") {
		t.Fatalf("document.xml missing text/style: %s", xml)
	}
}

func TestBuildRichDocumentForExportTipTapAndQuickText(t *testing.T) {
	content := `prefix {"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"加粗","marks":[{"type":"bold"},{"type":"italic"}]}]}]} suffix`
	doc := buildRichDocumentForExport(content, true)
	plain := richNodePlainText(doc.Blocks)
	if !strings.Contains(plain, "prefix") || !strings.Contains(plain, "加粗") || !strings.Contains(plain, "suffix") {
		t.Fatalf("unexpected rich plain text: %q", plain)
	}
	if len(doc.Blocks) < 2 || len(doc.Blocks[1].Children) == 0 {
		t.Fatalf("expected multiple rich blocks: %#v", doc.Blocks)
	}
	marks := doc.Blocks[1].Children[0].Marks
	if len(marks) < 2 {
		t.Fatalf("expected bold/italic marks: %#v", marks)
	}
	quick := buildRichDocumentForExport("**bold** *italic* `code` [link](https://example.com)", true)
	if got := richNodePlainText(quick.Blocks); got != "bold italic code link" {
		t.Fatalf("quick text plain mismatch: %q", got)
	}
}

func TestBuildRichDocumentForExportTable(t *testing.T) {
	input := strings.Replace(exportTable2x2TipTap, `"type":"tableHeader"`, `"type":"tableHeader","attrs":{"colspan":1,"rowspan":1,"colwidth":[120.5],"onclick":"bad","textAlign":"right","level":2,"language":"go"}`, 1)
	for _, build := range []func(string, bool) AgentRichDocument{buildRichDocumentForExport, buildLogRichDocument} {
		doc := build(input, false)
		if len(doc.Blocks) != 1 || doc.Blocks[0].Type != "table" {
			t.Fatalf("table was flattened: %#v", doc)
		}
		rows := doc.Blocks[0].Children
		if len(rows) != 2 || rows[0].Type != "tablerow" || rows[1].Type != "tablerow" || len(rows[0].Children) != 2 || len(rows[1].Children) != 2 {
			t.Fatalf("table rows/cells changed: %#v", rows)
		}
		cell := rows[0].Children[0]
		if cell.Type != "tableheader" || rows[1].Children[0].Type != "tablecell" || cell.Attrs["colspan"] != "1" || cell.Attrs["rowspan"] != "1" || cell.Attrs["colwidth"] != "[120.5]" {
			t.Fatalf("cell structure/attrs changed: %#v", cell)
		}
		if len(cell.Attrs) != 3 {
			t.Fatal("arbitrary TipTap attrs copied")
		}
	}
}

type docxTestTable struct {
	Rows []struct {
		Cells []struct {
			Properties struct {
				Span struct {
					Value int `xml:"val,attr"`
				} `xml:"gridSpan"`
				Merge *struct {
					Value string `xml:"val,attr"`
				} `xml:"vMerge"`
				Width struct {
					Value int `xml:"w,attr"`
				} `xml:"tcW"`
			} `xml:"tcPr"`
			Paragraphs []struct {
				XML string `xml:",innerxml"`
			} `xml:"p"`
			XML string `xml:",innerxml"`
		} `xml:"tc"`
	} `xml:"tr"`
}

func readDocxTestTables(t *testing.T, data []byte) ([]docxTestTable, string) {
	t.Helper()
	content := readDocxDocumentXML(t, data)
	var document struct {
		Body struct {
			Tables     []docxTestTable `xml:"tbl"`
			Paragraphs []struct {
				XML string `xml:",innerxml"`
			} `xml:"p"`
		} `xml:"body"`
	}
	if err := xml.Unmarshal([]byte(content), &document); err != nil {
		t.Fatal(err)
	}
	for _, para := range document.Body.Paragraphs {
		for _, text := range []string{"甲", "乙", "丙", "丁", "第二段"} {
			if strings.Contains(para.XML, text) {
				t.Fatalf("table content escaped into document paragraph: %s", para.XML)
			}
		}
	}
	return document.Body.Tables, content
}

func TestDocxTipTapNativeTable(t *testing.T) {
	input := strings.Replace(exportTable2x2TipTap, `"marks":[{"type":"bold"}]`, `"marks":[{"type":"bold"},{"type":"italic"},{"type":"underline"},{"type":"strike"},{"type":"textStyle","attrs":{"color":"#ff0000","fontFamily":"Arial","fontSize":"16px"}},{"type":"link","attrs":{"href":"https://example.com"}}]`, 1)
	input = strings.Replace(input, `"type":"text","text":"乙"`, `"type":"hardBreak"},{"type":"text","text":"乙"`, 1)
	input = strings.Replace(input, `"text":"丙"}]}]`, `"text":"丙"}]},{"type":"paragraph","content":[{"type":"text","text":"第二段"}]}]`, 1)
	for _, ooc := range []bool{false, true} {
		mode := "ic"
		if ooc {
			mode = "ooc"
		}
		data, err := (docxFormatter{}).Build(&ExportPayload{WithoutTimestamp: true, Messages: []ExportMessage{{SenderName: "角色", IcMode: mode, Content: input}}})
		if err != nil {
			t.Fatal(err)
		}
		tables, content := readDocxTestTables(t, data)
		if len(tables) != 1 || !strings.Contains(content, "<w:tbl>") || len(tables[0].Rows) != 2 {
			t.Fatalf("native 2x2 table missing: %s", content)
		}
		if !strings.Contains(content, `w:tblStyle w:val="TableGrid"`) {
			t.Fatalf("table grid style missing: %s", content)
		}
		for row, labels := range [][]string{{"甲", "乙"}, {"丙", "丁"}} {
			if len(tables[0].Rows[row].Cells) != 2 {
				t.Fatalf("wrong cell count: %s", content)
			}
			for col, label := range labels {
				if cell := tables[0].Rows[row].Cells[col]; !strings.Contains(cell.XML, label) || strings.Count(content, label) != 1 {
					t.Fatalf("cell %d,%d text lost/repeated: %s", row, col, content)
				}
			}
		}
		first := tables[0].Rows[0].Cells[0].XML
		for _, mark := range []string{"<w:b", "<w:i", "<w:u", "<w:strike", "FF0000", "Arial", `w:val="24"`, "<w:hyperlink"} {
			if !strings.Contains(first, mark) {
				t.Fatalf("cell mark %q missing: %s", mark, first)
			}
		}
		if !strings.Contains(tables[0].Rows[0].Cells[1].XML, "<w:br") {
			t.Fatal("cell hard break missing")
		}
		paragraphs := tables[0].Rows[1].Cells[0].Paragraphs
		if len(paragraphs) != 2 || !strings.Contains(paragraphs[0].XML, "丙") || !strings.Contains(paragraphs[1].XML, "第二段") {
			t.Fatalf("cell paragraphs flattened: %#v", paragraphs)
		}
		if ooc && (!strings.Contains(tables[0].Rows[0].Cells[0].XML, "（") || !strings.Contains(tables[0].Rows[1].Cells[1].XML, "）")) {
			t.Fatalf("table OOC wrapper missing: %s", content)
		}
	}
}

func TestDocxTipTapTableMergesAndWidths(t *testing.T) {
	input := `{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableHeader","attrs":{"colspan":2,"rowspan":2,"colwidth":[400,600]},"content":[{"type":"paragraph","content":[{"type":"text","text":"甲"}]}]},{"type":"tableCell","attrs":{"colwidth":[1000]},"content":[{"type":"paragraph","content":[{"type":"text","text":"乙"}]}]}]},{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"丙"}]}]}]},{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"丁"}]}]},{"type":"tableCell"},{"type":"tableCell"}]}]}]}`
	data, err := (docxFormatter{}).Build(&ExportPayload{Messages: []ExportMessage{{Content: input}}})
	if err != nil {
		t.Fatal(err)
	}
	tables, content := readDocxTestTables(t, data)
	if len(tables) != 1 || len(tables[0].Rows) != 3 {
		t.Fatalf("merged table missing: %s", content)
	}
	rows := tables[0].Rows
	if len(rows[0].Cells) != 2 || len(rows[1].Cells) != 2 || len(rows[2].Cells) != 3 {
		t.Fatalf("merge cell structure: %s", content)
	}
	start, continuation := rows[0].Cells[0], rows[1].Cells[0]
	if start.Properties.Span.Value != 2 || continuation.Properties.Span.Value != 2 || start.Properties.Merge == nil || start.Properties.Merge.Value != "restart" || continuation.Properties.Merge == nil || continuation.Properties.Merge.Value != "" {
		t.Fatalf("native gridSpan/vMerge missing: %s", content)
	}
	for _, cell := range []struct{ got, want string }{{start.XML, "甲"}, {rows[0].Cells[1].XML, "乙"}, {rows[1].Cells[1].XML, "丙"}, {rows[2].Cells[0].XML, "丁"}} {
		if !strings.Contains(cell.got, cell.want) || strings.Count(content, cell.want) != 1 {
			t.Fatalf("merged cell text lost/repeated: %s", content)
		}
	}
	if start.Properties.Width.Value != 4538 || rows[0].Cells[1].Properties.Width.Value != 4538 || continuation.Properties.Width.Value != start.Properties.Width.Value {
		t.Fatalf("expected widths scaled into 605px area: %s", content)
	}
}

func TestDocxTipTapTableCellListsAndOOCEmptyEdges(t *testing.T) {
	input := `{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph"}]},{"type":"tableCell","content":[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"甲"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"乙"}]}]}]}]},{"type":"tableCell","content":[{"type":"orderedList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"丙"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"丁"}]}]}]}]},{"type":"tableCell","content":[{"type":"paragraph"}]}]}]}]}`
	data, err := (docxFormatter{}).Build(&ExportPayload{WithoutTimestamp: true, Messages: []ExportMessage{{IcMode: "ooc", Content: input}}})
	if err != nil {
		t.Fatal(err)
	}
	tables, content := readDocxTestTables(t, data)
	if len(tables) != 1 || len(tables[0].Rows) != 1 || len(tables[0].Rows[0].Cells) != 4 {
		t.Fatalf("list table structure missing: %s", content)
	}
	cells := tables[0].Rows[0].Cells
	if strings.Contains(cells[0].XML, "（") || strings.Contains(cells[3].XML, "）") {
		t.Fatalf("OOC wrapper landed in empty edge cell: %s", content)
	}
	if !strings.Contains(cells[1].XML, "（") || !strings.Contains(cells[2].XML, "）") {
		t.Fatalf("OOC wrapper did not reach first/last real cell content: %s", content)
	}
	for _, want := range []string{"• ", "1. ", "2. ", "甲", "乙", "丙", "丁"} {
		if !strings.Contains(content, want) {
			t.Fatalf("cell list content %q missing: %s", want, content)
		}
	}
	if len(cells[1].Paragraphs) != 2 || len(cells[2].Paragraphs) != 2 {
		t.Fatalf("cell list items were flattened: %#v %#v", cells[1].Paragraphs, cells[2].Paragraphs)
	}
}

func TestDocxTipTapTableInvalidWidthsAndCellImage(t *testing.T) {
	for _, width := range []string{`[-1]`, `[0]`, `["NaN"]`, `[10001]`, `["Infinity"]`, `[120.5]`} {
		input := strings.Replace(exportTable2x2TipTap, `"type":"tableHeader"`, `"type":"tableHeader","attrs":{"colspan":-1,"rowspan":"bad","colwidth":`+width+`}`, 1)
		data, err := (docxFormatter{}).Build(&ExportPayload{Messages: []ExportMessage{{Content: input}}})
		if err != nil {
			t.Fatal(err)
		}
		tables, content := readDocxTestTables(t, data)
		if len(tables) != 1 || len(tables[0].Rows[0].Cells) != 2 {
			t.Fatalf("invalid attrs damaged table: %s", content)
		}
		got := tables[0].Rows[0].Cells[0].Properties.Width.Value
		if width == `[120.5]` && got != 1808 || width != `[120.5]` && got != 0 {
			t.Fatalf("unsafe/unexpected cell width %d: %s", got, content)
		}
	}
	input := strings.Replace(exportTable2x2TipTap, `"type":"text","text":"甲"`, `"type":"image","attrs":{"src":"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="}},{"type":"text","text":"甲"`, 1)
	for _, include := range []bool{false, true} {
		data, err := (docxFormatter{}).Build(&ExportPayload{IncludeImages: include, Messages: []ExportMessage{{Content: input}}})
		if err != nil {
			t.Fatal(err)
		}
		tables, _ := readDocxTestTables(t, data)
		cell := tables[0].Rows[0].Cells[0].XML
		if !strings.Contains(cell, "甲") || strings.Contains(cell, "r:embed") != include {
			t.Fatalf("cell text/image rendering failed: %s", cell)
		}
	}
}

func TestDocxHalfPointsAndColor(t *testing.T) {
	if got := docxHalfPoints("16px"); got != 24 {
		t.Fatalf("16px half-points = %d, want 24", got)
	}
	if got, ok := parseDocxColor("#abc"); !ok || got != (domain.Color{R: 170, G: 187, B: 204}) {
		t.Fatalf("unexpected color: %#v, %v", got, ok)
	}
}

func TestDocxTipTapInlineMarks(t *testing.T) {
	payload := &ExportPayload{WithoutTimestamp: true, Messages: []ExportMessage{{
		SenderName: "角色", SenderColor: "#111111",
		ContentRich: &AgentRichDocument{Type: "document", Blocks: []AgentRichNode{{
			Type: "paragraph", Children: []AgentRichNode{{Type: "text", Text: "格式", Marks: []AgentRichMark{
				{Type: "bold"}, {Type: "italic"}, {Type: "color", Value: "#ff0000"}, {Type: "link", Value: "https://example.com"},
			}}},
		}}},
	}}}
	data, err := (docxFormatter{}).Build(payload)
	if err != nil {
		t.Fatal(err)
	}
	xml := readDocxDocumentXML(t, data)
	if !strings.Contains(xml, "w:b") || !strings.Contains(xml, "w:i") || !strings.Contains(xml, "FF0000") || !strings.Contains(xml, "<w:hyperlink") {
		t.Fatalf("TipTap marks missing: %s", xml)
	}
}

func TestDocxFormatterEmbedsDataURLImage(t *testing.T) {
	pngData, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngData)
	payload := &ExportPayload{IncludeImages: true, Messages: []ExportMessage{{SenderName: "图", SenderColor: "#111111", ContentRich: &AgentRichDocument{Type: "document", Blocks: []AgentRichNode{{Type: "paragraph", Children: []AgentRichNode{{Type: "image", Attrs: map[string]string{"src": dataURL}}}}}}}}}
	data, err := (docxFormatter{}).Build(payload)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range zr.File {
		if strings.HasPrefix(file.Name, "word/media/") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected embedded media")
	}
}

func TestDocxFormatterEmbedsJPEGWithoutReencoding(t *testing.T) {
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 1, 1)), &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	dataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
	payload := &ExportPayload{IncludeImages: true, Messages: []ExportMessage{{
		SenderName: "图", SenderColor: "#111111",
		ContentRich: &AgentRichDocument{Type: "document", Blocks: []AgentRichNode{{Type: "paragraph", Children: []AgentRichNode{{Type: "image", Attrs: map[string]string{"src": dataURL}}}}}},
	}}}
	data, err := (docxFormatter{}).Build(payload)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range zr.File {
		if !strings.HasPrefix(file.Name, "word/media/") || !strings.HasSuffix(file.Name, ".jpeg") {
			continue
		}
		reader, openErr := file.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		media, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !bytes.HasPrefix(media, []byte{0xff, 0xd8, 0xff}) {
			t.Fatalf("JPEG media was re-encoded unexpectedly: %x", media[:min(len(media), 4)])
		}
		return
	}
	t.Fatal("expected JPEG media entry")
}

func TestDocxFormatterTimestampAndOOCOptions(t *testing.T) {
	created := time.Unix(1700000000, 0)
	makePayload := func(withoutTimestamp bool) *ExportPayload {
		return &ExportPayload{
			WithoutTimestamp: withoutTimestamp,
			Messages: []ExportMessage{{
				SenderName: "角色", SenderColor: "#123456", CreatedAt: created, IcMode: "ooc",
				ContentRich: &AgentRichDocument{Type: "document", Blocks: []AgentRichNode{{Type: "paragraph", Children: []AgentRichNode{{Type: "text", Text: "场外"}}}}},
			}},
		}
	}
	withTimestamp, err := (docxFormatter{}).Build(makePayload(false))
	if err != nil {
		t.Fatal(err)
	}
	withXML := readDocxDocumentXML(t, withTimestamp)
	if !strings.Contains(withXML, created.Format("2006-01-02 15:04:05")) || !strings.Contains(withXML, "（") || !strings.Contains(withXML, "）") {
		t.Fatalf("timestamp/OOC missing: %s", withXML)
	}
	withoutTimestamp, err := (docxFormatter{}).Build(makePayload(true))
	if err != nil {
		t.Fatal(err)
	}
	withoutXML := readDocxDocumentXML(t, withoutTimestamp)
	if strings.Contains(withoutXML, created.Format("2006-01-02 15:04:05")) {
		t.Fatalf("timestamp unexpectedly present: %s", withoutXML)
	}
}

func TestDocxListsUseSeparateParagraphs(t *testing.T) {
	payload := &ExportPayload{WithoutTimestamp: true, Messages: []ExportMessage{{
		SenderName: "角色", SenderColor: "#123456",
		ContentRich: &AgentRichDocument{Type: "document", Blocks: []AgentRichNode{
			{Type: "bullet_list", Children: []AgentRichNode{
				{Type: "list_item", Children: []AgentRichNode{{Type: "paragraph", Children: []AgentRichNode{{Type: "text", Text: "item1"}}}}},
				{Type: "list_item", Children: []AgentRichNode{{Type: "paragraph", Children: []AgentRichNode{{Type: "text", Text: "item2"}}}}},
			}},
			{Type: "ordered_list", Children: []AgentRichNode{
				{Type: "list_item", Children: []AgentRichNode{{Type: "paragraph", Children: []AgentRichNode{{Type: "text", Text: "first"}}}}},
				{Type: "list_item", Children: []AgentRichNode{{Type: "paragraph", Children: []AgentRichNode{{Type: "text", Text: "second"}}}}},
			}},
		}},
	}}}
	data, err := (docxFormatter{}).Build(payload)
	if err != nil {
		t.Fatal(err)
	}
	xml := readDocxDocumentXML(t, data)
	if strings.Count(xml, "item1") != 1 || strings.Count(xml, "item2") != 1 {
		t.Fatalf("bullet items missing: %s", xml)
	}
	if strings.Count(xml, "<w:p>") < 5 {
		t.Fatalf("expected header plus four list paragraphs, got %d: %s", strings.Count(xml, "<w:p>"), xml)
	}
	if !strings.Contains(xml, "• ") || !strings.Contains(xml, "1. ") || !strings.Contains(xml, "2. ") {
		t.Fatalf("list prefixes missing: %s", xml)
	}
	if strings.Index(xml, "item1") > strings.Index(xml, "item2") || strings.Index(xml, "first") > strings.Index(xml, "second") {
		t.Fatalf("list item order changed: %s", xml)
	}
}

func TestDocxOOCWrapperOnOrderedList(t *testing.T) {
	payload := &ExportPayload{WithoutTimestamp: true, Messages: []ExportMessage{{
		SenderName: "角色", SenderColor: "#123456", IcMode: "ooc",
		ContentRich: &AgentRichDocument{Type: "document", Blocks: []AgentRichNode{{
			Type: "ordered_list", Children: []AgentRichNode{
				{Type: "list_item", Children: []AgentRichNode{{Type: "paragraph", Children: []AgentRichNode{{Type: "text", Text: "第一项"}}}}},
				{Type: "list_item", Children: []AgentRichNode{{Type: "paragraph", Children: []AgentRichNode{{Type: "text", Text: "第二项"}}}}},
			},
		}}},
	}}}
	data, err := (docxFormatter{}).Build(payload)
	if err != nil {
		t.Fatal(err)
	}
	xml := readDocxDocumentXML(t, data)
	if !strings.Contains(xml, "1. ") || !strings.Contains(xml, "2. ") {
		t.Fatalf("ordered list prefixes missing: %s", xml)
	}
	if strings.Contains(xml, "3. ") {
		t.Fatalf("unexpected third list item: %s", xml)
	}
	if strings.Index(xml, "（") == -1 || strings.Index(xml, "第一项") == -1 || strings.Index(xml, "（") > strings.Index(xml, "第一项") {
		t.Fatalf("opening OOC wrapper is not before first item: %s", xml)
	}
	if strings.Index(xml, "）") == -1 || strings.Index(xml, "第二项") == -1 || strings.Index(xml, "）") < strings.Index(xml, "第二项") {
		t.Fatalf("closing OOC wrapper is not after second item: %s", xml)
	}
}

func TestBuildExportPayloadSkipsRichASTForNonDocx(t *testing.T) {
	job := &model.MessageExportJobModel{Format: "txt", ChannelID: "non-docx-rich-path"}
	msg := &model.MessageModel{Content: `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"内容"}]}]}`}
	payload := buildExportPayload(job, "频道", []*model.MessageModel{msg}, nil, &exportExtraOptions{IncludeImages: true, IncludeDiceCommand: true})
	if payload == nil || len(payload.Messages) != 1 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if payload.Messages[0].ContentRich != nil {
		t.Fatal("non-DOCX payload unexpectedly built a rich AST")
	}
}

func readDocxDocumentXML(t *testing.T, data []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range zr.File {
		if filepath.ToSlash(file.Name) != "word/document.xml" {
			continue
		}
		reader, openErr := file.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		body, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		return string(body)
	}
	t.Fatal("word/document.xml missing")
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
