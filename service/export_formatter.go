package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"io"
	"regexp"
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
	IsBot          bool      `json:"is_bot"`
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

const diceLogVersion = 105

var formatterRegistry = map[string]exportFormatter{
	"json": jsonFormatter{},
	"txt":  textFormatter{},
	"html": htmlFormatter{},
}

type diceLogPayload struct {
	Version int           `json:"version"`
	Items   []diceLogItem `json:"items"`
}

type diceLogItem struct {
	Nickname    string           `json:"nickname"`
	ImUserID    string           `json:"imUserId"`
	UniformID   string           `json:"uniformId"`
	Time        int64            `json:"time"`
	Message     string           `json:"message"`
	IsDice      bool             `json:"isDice"`
	CommandID   string           `json:"commandId"`
	CommandInfo *diceCommandInfo `json:"commandInfo"`
	RawMsgID    string           `json:"rawMsgId"`
}

type diceCommandInfo struct {
	Cmd    string `json:"cmd"`
	Result string `json:"result"`
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
			IsBot:          msg.User != nil && msg.User.IsBot,
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

	if plain, ok := extractTipTapPlainText(input); ok {
		return normalizePlainText(plain)
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

func extractTipTapPlainText(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
		return "", false
	}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	var fragments []string
	for {
		var node tiptapNode
		if err := decoder.Decode(&node); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", false
		}
		if strings.ToLower(strings.TrimSpace(node.Type)) != "doc" {
			return "", false
		}
		writer := newPlainTextWriter()
		writeTipTapNode(writer, &node)
		fragments = append(fragments, writer.String())
	}
	if len(fragments) == 0 {
		return "", false
	}
	return strings.Join(fragments, "\n"), true
}

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
	if value, ok := n.Attrs[key]; ok {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

var tiptapBlockNodes = map[string]struct{}{
	"paragraph":      {},
	"heading":        {},
	"blockquote":     {},
	"codeblock":      {},
	"bulletlist":     {},
	"orderedlist":    {},
	"listitem":       {},
	"tasklist":       {},
	"taskitem":       {},
	"horizontalrule": {},
	"table":          {},
	"tablerow":       {},
	"tablecell":      {},
}

func isTipTapBlockNode(nodeType string) bool {
	_, ok := tiptapBlockNodes[nodeType]
	return ok
}

type plainTextWriter struct {
	sb             strings.Builder
	lastWasNewline bool
}

func newPlainTextWriter() *plainTextWriter {
	return &plainTextWriter{}
}

func (w *plainTextWriter) write(text string) {
	if text == "" {
		return
	}
	text = strings.ReplaceAll(text, "\u00a0", " ")
	w.sb.WriteString(text)
	w.lastWasNewline = strings.HasSuffix(text, "\n")
}

func (w *plainTextWriter) newline() {
	if w.sb.Len() == 0 || w.lastWasNewline {
		return
	}
	w.sb.WriteByte('\n')
	w.lastWasNewline = true
}

func (w *plainTextWriter) String() string {
	return w.sb.String()
}

func writeTipTapNode(w *plainTextWriter, node *tiptapNode) {
	if node == nil || w == nil {
		return
	}
	nodeType := strings.ToLower(strings.TrimSpace(node.Type))
	switch nodeType {
	case "doc":
		for _, child := range node.Content {
			writeTipTapNode(w, child)
		}
		return
	case "text":
		w.write(node.Text)
		return
	case "hardbreak":
		w.newline()
		return
	case "mention":
		if label := node.attrString("label"); label != "" {
			w.write(label)
		} else if node.Text != "" {
			w.write(node.Text)
		} else if name := node.attrString("name"); name != "" {
			w.write(name)
		} else if text := node.attrString("text"); text != "" {
			w.write(text)
		}
		return
	}
	if len(node.Content) > 0 {
		for _, child := range node.Content {
			writeTipTapNode(w, child)
		}
	} else if node.Text != "" {
		w.write(node.Text)
	} else if text := node.attrString("text"); text != "" {
		w.write(text)
	}
	if isTipTapBlockNode(nodeType) {
		w.newline()
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
	dicePayload := buildDiceLogPayload(payload)
	return json.MarshalIndent(dicePayload, "", "  ")
}

func buildDiceLogPayload(payload *ExportPayload) *diceLogPayload {
	if payload == nil {
		return &diceLogPayload{Version: diceLogVersion, Items: nil}
	}
	items := make([]diceLogItem, 0, len(payload.Messages))
	for i := range payload.Messages {
		msg := &payload.Messages[i]
		body := buildContentBody(msg)
		isDice, info := detectDiceCommand(msg)
		items = append(items, diceLogItem{
			Nickname:    msg.SenderName,
			ImUserID:    fallbackIMUserID(msg.SenderID),
			UniformID:   buildUniformID(msg.SenderID),
			Time:        safeUnix(msg.CreatedAt),
			Message:     body,
			IsDice:      isDice,
			CommandID:   msg.ID,
			CommandInfo: info,
			RawMsgID:    msg.ID,
		})
	}
	return &diceLogPayload{Version: diceLogVersion, Items: items}
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
		content := buildContentBody(&msg)
		parts := []string{}
		if header != "" {
			parts = append(parts, header)
		}
		parts = append(parts, namePart)
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

var diceRollPattern = regexp.MustCompile(`(?i)\b(\d+d\d+(?:[+\-x×*/]\d+)?[^=]*)=\s*([^\s]+.*)`)

func detectDiceCommand(msg *ExportMessage) (bool, *diceCommandInfo) {
	if msg == nil || !msg.IsBot {
		return false, nil
	}
	clean := stripRichText(msg.Content)
	if clean == "" {
		return false, nil
	}
	lines := strings.Split(clean, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		matches := diceRollPattern.FindStringSubmatch(line)
		if len(matches) < 3 {
			continue
		}
		cmd := strings.TrimSpace(matches[1])
		result := strings.TrimSpace(matches[2])
		if cmd == "" || result == "" {
			continue
		}
		return true, &diceCommandInfo{Cmd: cmd, Result: result}
	}
	return false, nil
}

func buildContentBody(msg *ExportMessage) string {
	if msg == nil {
		return ""
	}
	clean := stripRichText(msg.Content)
	clean = wrapOOCContent(msg.IcMode, clean)
	var parts []string
	if msg.IsArchived {
		parts = append(parts, "[已归档]")
	}
	if msg.IsWhisper {
		if label := formatWhisperTargets(msg.WhisperTargets); label != "" {
			parts = append(parts, label)
		}
	}
	parts = append(parts, clean)
	return strings.TrimSpace(strings.Join(parts, " "))
}

func safeUnix(t time.Time) int64 {
	if t.IsZero() {
		return time.Now().Unix()
	}
	return t.Unix()
}

func fallbackIMUserID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "anonymous"
	}
	return id
}

func buildUniformID(id string) string {
	base := fallbackIMUserID(id)
	return "Seal:" + base
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
