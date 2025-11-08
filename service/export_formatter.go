package service

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	htmltemplate "html/template"
	"io"
	"strings"
	"time"

	"sealchat/model"

	htmlnode "golang.org/x/net/html"
	"html"
)

type exportFormatter interface {
	Ext() string
	ContentType() string
	Build(payload *ExportPayload) ([]byte, error)
}

type ExportMessage struct {
	ID             string    `json:"id"`
	SenderID       string    `json:"sender_id"`
	SenderName     string    `json:"sender_name"`
	SenderColor    string    `json:"sender_color"`
	IcMode         string    `json:"ic_mode"`
	IsWhisper      bool      `json:"is_whisper"`
	IsArchived     bool      `json:"is_archived"`
	CreatedAt      time.Time `json:"created_at"`
	Content        string    `json:"content"`
	WhisperTargets []string  `json:"whisper_targets"`
}

type ExportPayload struct {
	ChannelID        string          `json:"channel_id"`
	ChannelName      string          `json:"channel_name"`
	GeneratedAt      time.Time       `json:"generated_at"`
	StartTime        *time.Time      `json:"start_time,omitempty"`
	EndTime          *time.Time      `json:"end_time,omitempty"`
	Messages         []ExportMessage `json:"messages"`
	Meta             map[string]bool `json:"meta"`
	Count            int             `json:"count"`
	WithoutTimestamp bool            `json:"without_timestamp"`
}

var formatterRegistry = map[string]exportFormatter{
	"json": jsonFormatter{},
	"txt":  textFormatter{},
	"html": htmlFormatter{},
	"docx": docxFormatter{},
}

func getFormatter(name string) (exportFormatter, bool) {
	f, ok := formatterRegistry[name]
	return f, ok
}

func buildExportPayload(job *model.MessageExportJobModel, channelName string, messages []*model.MessageModel) *ExportPayload {
	identityResolver := newIdentityResolver(job.ChannelID)
	exportMessages := make([]ExportMessage, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		exportMessages = append(exportMessages, ExportMessage{
			ID:             msg.ID,
			SenderID:       msg.UserID,
			SenderName:     resolveSenderName(msg),
			SenderColor:    msg.SenderIdentityColor,
			IcMode:         fallbackIcMode(msg.ICMode),
			IsWhisper:      msg.IsWhisper,
			IsArchived:     msg.IsArchived,
			CreatedAt:      msg.CreatedAt,
			Content:        msg.Content,
			WhisperTargets: extractWhisperTargets(msg, job.ChannelID, identityResolver),
		})
	}

	return &ExportPayload{
		ChannelID:        job.ChannelID,
		ChannelName:      channelName,
		GeneratedAt:      time.Now(),
		StartTime:        job.StartTime,
		EndTime:          job.EndTime,
		Messages:         exportMessages,
		Count:            len(exportMessages),
		WithoutTimestamp: job.WithoutTimestamp,
		Meta: map[string]bool{
			"include_ooc":       job.IncludeOOC,
			"include_archived":  job.IncludeArchived,
			"merge_messages":    job.MergeMessages,
			"without_timestamp": job.WithoutTimestamp,
		},
	}
}

func extractWhisperTargets(msg *model.MessageModel, channelID string, resolver *identityResolver) []string {
	if msg == nil || !msg.IsWhisper {
		return nil
	}
	var targets []string
	seen := map[string]struct{}{}
	addName := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		targets = append(targets, name)
	}
	if msg.WhisperTarget != nil {
		addName(resolveMemberDisplayName(channelID, msg.WhisperTarget.ID, resolver))
	}
	if strings.TrimSpace(msg.WhisperTargetMemberName) != "" {
		addName(msg.WhisperTargetMemberName)
	}
	if strings.TrimSpace(msg.WhisperTargetUserNick) != "" && len(targets) == 0 {
		addName(msg.WhisperTargetUserNick)
	}
	for _, id := range parseWhisperIDs(msg.WhisperTo) {
		if resolver != nil {
			if name := resolver.resolveIdentityName(id); name != "" {
				addName(name)
				continue
			}
		}
		addName(resolveMemberDisplayName(channelID, id, resolver))
	}
	return targets
}

func parseWhisperIDs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var ids []string
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			ids = append(ids, trimmed)
		}
	}
	return ids
}

type identityResolver struct {
	channelID string
	byID      map[string]string
}

func newIdentityResolver(channelID string) *identityResolver {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil
	}
	items, err := model.ChannelIdentityList(channelID, "")
	if err != nil {
		return nil
	}
	m := make(map[string]string, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		name := strings.TrimSpace(item.DisplayName)
		if name == "" {
			continue
		}
		m[strings.TrimSpace(item.ID)] = name
	}
	return &identityResolver{channelID: channelID, byID: m}
}

func (r *identityResolver) resolveIdentityName(identityID string) string {
	if r == nil {
		return ""
	}
	identityID = strings.TrimSpace(identityID)
	if identityID == "" {
		return ""
	}
	if name, ok := r.byID[identityID]; ok {
		return name
	}
	return ""
}

func stripRichText(input string) string {
	if input == "" {
		return ""
	}
	s := strings.TrimSpace(input)
	if s == "" {
		return ""
	}
	if !strings.ContainsAny(s, "<>&") {
		return normalizePlainText(s)
	}
	tokenizer := htmlnode.NewTokenizer(strings.NewReader(s))
	var sb strings.Builder
	lastWasNewline := false
	writeText := func(text string) {
		if text == "" {
			return
		}
		text = html.UnescapeString(text)
		text = strings.ReplaceAll(text, "\u00a0", " ")
		sb.WriteString(text)
		lastWasNewline = strings.HasSuffix(text, "\n")
	}
	writeNewline := func() {
		if sb.Len() == 0 || lastWasNewline {
			return
		}
		sb.WriteByte('\n')
		lastWasNewline = true
	}
	for {
		switch tokenizer.Next() {
		case htmlnode.ErrorToken:
			return normalizePlainText(sb.String())
		case htmlnode.TextToken:
			writeText(string(tokenizer.Text()))
		case htmlnode.StartTagToken:
			name, _ := tokenizer.TagName()
			tag := strings.ToLower(string(name))
			if tag == "img" {
				continue
			}
			if shouldInsertLineBreak(tag) {
				writeNewline()
			}
		case htmlnode.EndTagToken:
			name, _ := tokenizer.TagName()
			tag := strings.ToLower(string(name))
			if shouldInsertLineBreak(tag) {
				writeNewline()
			}
		case htmlnode.SelfClosingTagToken:
			name, _ := tokenizer.TagName()
			tag := strings.ToLower(string(name))
			if tag == "img" {
				continue
			}
			if shouldInsertLineBreak(tag) {
				writeNewline()
			}
		}
	}
}

func shouldInsertLineBreak(tag string) bool {
	switch tag {
	case "br", "p", "div", "li":
		return true
	default:
		return false
	}
}

func normalizePlainText(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimSpace(s)
}

func resolveSenderName(msg *model.MessageModel) string {
	if msg == nil {
		return "未知用户"
	}
	if v := strings.TrimSpace(msg.SenderIdentityName); v != "" {
		return v
	}
	if v := strings.TrimSpace(msg.SenderMemberName); v != "" {
		return v
	}
	if msg.Member != nil && strings.TrimSpace(msg.Member.Nickname) != "" {
		return msg.Member.Nickname
	}
	if msg.User != nil {
		if strings.TrimSpace(msg.User.Nickname) != "" {
			return msg.User.Nickname
		}
		if strings.TrimSpace(msg.User.Username) != "" {
			return msg.User.Username
		}
	}
	if strings.TrimSpace(msg.UserID) != "" {
		return msg.UserID
	}
	return "匿名"
}

func resolveUserDisplayName(u *model.UserModel) string {
	if u == nil {
		return ""
	}
	if v := strings.TrimSpace(u.Nickname); v != "" {
		return v
	}
	if v := strings.TrimSpace(u.Username); v != "" {
		return v
	}
	return strings.TrimSpace(u.ID)
}

func resolveMemberDisplayName(channelID, userID string, resolver *identityResolver) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ""
	}
	if resolver != nil {
		if name := resolver.resolveIdentityName(userID); name != "" {
			return name
		}
	}
	if member, _ := model.MemberGetByUserIDAndChannelIDBase(userID, channelID, "", false); member != nil {
		if v := strings.TrimSpace(member.Nickname); v != "" {
			return v
		}
	}
	if user := model.UserGet(userID); user != nil {
		return resolveUserDisplayName(user)
	}
	return userID
}

func fallbackIcMode(value string) string {
	if strings.TrimSpace(value) == "" {
		return "ic"
	}
	return strings.ToLower(value)
}

type jsonFormatter struct{}

func (jsonFormatter) Ext() string {
	return "json"
}

func (jsonFormatter) ContentType() string {
	return "application/json"
}

func (jsonFormatter) Build(payload *ExportPayload) ([]byte, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload 为空")
	}
	return json.MarshalIndent(payload, "", "  ")
}

type textFormatter struct{}

func (textFormatter) Ext() string {
	return "txt"
}

func (textFormatter) ContentType() string {
	return "text/plain; charset=utf-8"
}

func (textFormatter) Build(payload *ExportPayload) ([]byte, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload 为空")
	}
	var sb strings.Builder
	header := fmt.Sprintf("频道: %s (%s)\n导出时间: %s\n消息数量: %d\n---\n",
		payload.ChannelName,
		payload.ChannelID,
		payload.GeneratedAt.Format(time.RFC3339),
		len(payload.Messages),
	)
	sb.WriteString(header)
	for _, msg := range payload.Messages {
		var prefixParts []string
		if !payload.WithoutTimestamp {
			prefixParts = append(prefixParts, fmt.Sprintf("[%s]", msg.CreatedAt.Format("2006-01-02 15:04:05")))
		}
		var header string
		if len(prefixParts) > 0 {
			header = strings.Join(prefixParts, " ")
		}
		namePart := fmt.Sprintf("<%s>", msg.SenderName)
		cleanContent := stripRichText(msg.Content)
		content := wrapOOCContent(msg.IcMode, cleanContent)
		parts := []string{}
		if header != "" {
			parts = append(parts, header)
		}
		parts = append(parts, namePart)
		if msg.IsWhisper {
			if label := formatWhisperTargets(msg.WhisperTargets); label != "" {
				parts = append(parts, label)
			}
		}
		parts = append(parts, content)
		sb.WriteString(strings.Join(parts, " ") + "\n")
	}
	return []byte(sb.String()), nil
}

func wrapOOCContent(icMode string, content string) string {
	if strings.EqualFold(strings.TrimSpace(icMode), "ooc") {
		trimmed := strings.TrimSpace(content)
		if (strings.HasPrefix(trimmed, "（") && strings.HasSuffix(trimmed, "）")) ||
			(strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")")) {
			return content
		}
		return fmt.Sprintf("（%s）", content)
	}
	return content
}

func formatWhisperTargets(targets []string) string {
	if len(targets) == 0 {
		return ""
	}
	return fmt.Sprintf("[对%s]", strings.Join(targets, "、"))
}

type htmlFormatter struct{}

func (htmlFormatter) Ext() string {
	return "html"
}

func (htmlFormatter) ContentType() string {
	return "text/html; charset=utf-8"
}

var exportHTMLTemplate = htmltemplate.Must(htmltemplate.New("export_html").Funcs(htmltemplate.FuncMap{
	"formatTime": func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.Format("2006-01-02 15:04:05")
	},
}).Parse(`<!DOCTYPE html>
<html lang="zh">
<head>
  <meta charset="UTF-8">
  <title>频道导出 - {{.ChannelName}}</title>
  <style>
    body { font-family: -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Hiragino Sans GB",sans-serif; margin: 2rem; background: #f7f7f7; }
    .meta { margin-bottom: 1.5rem; color: #555; }
    .message { padding: 12px 16px; margin-bottom: 8px; background: #fff; border-radius: 6px; box-shadow: 0 1px 2px rgba(0,0,0,0.05); }
    .sender { font-weight: 600; color: #222; margin-right: 4px; }
    .timestamp { color: #888; font-size: 0.9rem; }
    .timestamp.hidden { visibility: hidden; height: 0; margin: 0; }
    .ooc { border-left: 3px solid #eab308; }
    .whisper { border-left: 3px solid #6366f1; }
    .content { margin-top: 4px; white-space: pre-wrap; line-height: 1.5; }
  </style>
</head>
<body>
  <section class="meta">
    <div><strong>频道：</strong>{{.ChannelName}} ({{.ChannelID}})</div>
    <div><strong>导出时间：</strong>{{formatTime .GeneratedAt}}</div>
    <div><strong>消息数量：</strong>{{.Count}}</div>
  </section>
  {{range .Messages}}
    <article class="message {{if eq .IcMode "ooc"}}ooc{{end}} {{if .IsWhisper}}whisper{{end}}">
      {{if not $.WithoutTimestamp}}<div class="timestamp">{{formatTime .CreatedAt}}</div>{{end}}
      <div class="content"><span class="sender">&lt;{{.SenderName}}&gt;</span>{{.Content}}</div>
    </article>
  {{end}}
</body>
</html>`))

func (htmlFormatter) Build(payload *ExportPayload) ([]byte, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload 为空")
	}
	buf := &bytes.Buffer{}
	if err := exportHTMLTemplate.Execute(buf, payload); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type docxFormatter struct{}

func (docxFormatter) Ext() string {
	return "docx"
}

func (docxFormatter) ContentType() string {
	return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
}

func (docxFormatter) Build(payload *ExportPayload) ([]byte, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload 为空")
	}
	documentXML := buildDocxDocumentXML(payload)
	return packageDocx(documentXML)
}

func buildDocxDocumentXML(payload *ExportPayload) []byte {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sb.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`)
	sb.WriteString(`<w:body>`)
	header := fmt.Sprintf("频道: %s (%s) 导出时间: %s", payload.ChannelName, payload.ChannelID, payload.GeneratedAt.Format(time.RFC3339))
	sb.WriteString(wParagraph(header))
	for _, msg := range payload.Messages {
		timePrefix := ""
		if !payload.WithoutTimestamp {
			timePrefix = fmt.Sprintf("[%s] ", msg.CreatedAt.Format("2006-01-02 15:04:05"))
		}
		line := fmt.Sprintf("%s<%s> %s", timePrefix, msg.SenderName, msg.Content)
		sb.WriteString(wParagraph(line))
	}
	sb.WriteString(`<w:sectPr/>`)
	sb.WriteString(`</w:body></w:document>`)
	return []byte(sb.String())
}

func wParagraph(text string) string {
	var esc strings.Builder
	_ = xml.EscapeText(&esc, []byte(text))
	return fmt.Sprintf(`<w:p><w:r><w:t xml:space="preserve">%s</w:t></w:r></w:p>`, esc.String())
}

func packageDocx(documentXML []byte) ([]byte, error) {
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)

	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="R1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`,
	}

	for name, content := range files {
		if err := writeZipFile(zw, name, []byte(content)); err != nil {
			return nil, err
		}
	}
	if err := writeZipFile(zw, "word/document.xml", documentXML); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeZipFile(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, bytes.NewReader(data))
	return err
}
