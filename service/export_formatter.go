package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	htmltemplate "html/template"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"sealchat/model"
	"sealchat/utils"

	htmlnode "golang.org/x/net/html"
)

type exportFormatter interface {
	Ext() string
	ContentType() string
	Build(payload *ExportPayload) ([]byte, error)
}

type payloadContext struct {
	DisplayOptions map[string]any
	PartIndex      int
	PartTotal      int
	SliceStart     *time.Time
	SliceEnd       *time.Time
	GeneratedAt    *time.Time
}

type ExportMessage struct {
	ID             string    `json:"id"`
	SenderID       string    `json:"sender_id"`
	SenderName     string    `json:"sender_name"`
	SenderColor    string    `json:"sender_color"`
	SenderAvatar   string    `json:"sender_avatar,omitempty"`
	IcMode         string    `json:"ic_mode"`
	IsWhisper      bool      `json:"is_whisper"`
	IsArchived     bool      `json:"is_archived"`
	IsBot          bool      `json:"is_bot"`
	CreatedAt      time.Time `json:"created_at"`
	Content        string    `json:"content"`
	WhisperTargets []string  `json:"whisper_targets"`
}

type ExportPayload struct {
	ChannelID        string                 `json:"channel_id"`
	ChannelName      string                 `json:"channel_name"`
	GeneratedAt      time.Time              `json:"generated_at"`
	StartTime        *time.Time             `json:"start_time,omitempty"`
	EndTime          *time.Time             `json:"end_time,omitempty"`
	SliceStart       *time.Time             `json:"slice_start,omitempty"`
	SliceEnd         *time.Time             `json:"slice_end,omitempty"`
	PartIndex        int                    `json:"part_index,omitempty"`
	PartTotal        int                    `json:"part_total,omitempty"`
	DisplayOptions   map[string]any         `json:"display_options,omitempty"`
	Messages         []ExportMessage        `json:"messages"`
	Meta             map[string]bool        `json:"meta"`
	Count            int                    `json:"count"`
	WithoutTimestamp bool                   `json:"without_timestamp"`
	ExtraMeta        map[string]interface{} `json:"extra_meta,omitempty"`
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

func buildExportPayload(job *model.MessageExportJobModel, channelName string, messages []*model.MessageModel, ctx *payloadContext) *ExportPayload {
	identityResolver := newIdentityResolver(job.ChannelID)
	exportMessages := make([]ExportMessage, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		content := msg.Content
		if html, ok := convertTipTapToHTML(content); ok {
			content = html
		}
		exportMessages = append(exportMessages, ExportMessage{
			ID:             msg.ID,
			SenderID:       msg.UserID,
			SenderName:     resolveSenderName(msg),
			SenderColor:    msg.SenderIdentityColor,
			SenderAvatar:   resolveSenderAvatar(msg),
			IcMode:         fallbackIcMode(msg.ICMode),
			IsWhisper:      msg.IsWhisper,
			IsArchived:     msg.IsArchived,
			IsBot:          msg.User != nil && msg.User.IsBot,
			CreatedAt:      msg.CreatedAt,
			Content:        content,
			WhisperTargets: extractWhisperTargets(msg, job.ChannelID, identityResolver),
		})
	}

	return &ExportPayload{
		ChannelID:        job.ChannelID,
		ChannelName:      channelName,
		GeneratedAt:      resolvePayloadGeneratedAt(ctx),
		StartTime:        job.StartTime,
		EndTime:          job.EndTime,
		SliceStart:       safeCloneTime(ctx, true),
		SliceEnd:         safeCloneTime(ctx, false),
		PartIndex:        safePartIndex(ctx),
		PartTotal:        safePartTotal(ctx),
		DisplayOptions:   cloneDisplayOptions(ctx),
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

func resolvePayloadGeneratedAt(ctx *payloadContext) time.Time {
	if ctx != nil && ctx.GeneratedAt != nil {
		return ctx.GeneratedAt.UTC()
	}
	return time.Now()
}

func safeCloneTime(ctx *payloadContext, isStart bool) *time.Time {
	if ctx == nil {
		return nil
	}
	var source *time.Time
	if isStart {
		source = ctx.SliceStart
	} else {
		source = ctx.SliceEnd
	}
	if source == nil {
		return nil
	}
	value := *source
	return &value
}

func safePartIndex(ctx *payloadContext) int {
	if ctx == nil || ctx.PartIndex <= 0 {
		return 0
	}
	return ctx.PartIndex
}

func safePartTotal(ctx *payloadContext) int {
	if ctx == nil || ctx.PartTotal <= 0 {
		return 0
	}
	return ctx.PartTotal
}

func cloneDisplayOptions(ctx *payloadContext) map[string]any {
	if ctx == nil || len(ctx.DisplayOptions) == 0 {
		return nil
	}
	result := make(map[string]any, len(ctx.DisplayOptions))
	for k, v := range ctx.DisplayOptions {
		result[k] = v
	}
	return result
}

func resolveSenderAvatar(msg *model.MessageModel) string {
	if msg == nil {
		return ""
	}
	if id := strings.TrimSpace(msg.SenderIdentityAvatarID); id != "" {
		return "id:" + id
	}
	if msg.User != nil {
		avatar := strings.TrimSpace(msg.User.Avatar)
		if avatar != "" {
			return avatar
		}
	}
	return ""
}

func convertTipTapToHTML(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || trimmed[0] != '{' {
		return "", false
	}
	var node tiptapNode
	if err := json.Unmarshal([]byte(trimmed), &node); err != nil {
		return "", false
	}
	if strings.ToLower(strings.TrimSpace(node.Type)) != "doc" {
		return "", false
	}
	var buf strings.Builder
	for _, child := range node.Content {
		renderTipTapHTML(&buf, child)
	}
	return buf.String(), true
}

func renderTipTapHTML(buf *strings.Builder, node *tiptapNode) {
	if buf == nil || node == nil {
		return
	}
	nodeType := strings.ToLower(strings.TrimSpace(node.Type))
	switch nodeType {
	case "text":
		buf.WriteString(applyTipTapMarks(htmlEscape(node.Text), node.Marks))
	case "paragraph":
		if align := node.attrString("textAlign"); align != "" {
			buf.WriteString(`<p style="text-align:` + htmlEscape(align) + `">`)
		} else {
			buf.WriteString("<p>")
		}
		if len(node.Content) == 0 {
			buf.WriteString("<br />")
		} else {
			for _, child := range node.Content {
				renderTipTapHTML(buf, child)
			}
		}
		buf.WriteString("</p>")
	case "heading":
		level := clampInt(int(node.attrFloat("level")), 1, 6)
		if level == 0 {
			level = 1
		}
		if align := node.attrString("textAlign"); align != "" {
			buf.WriteString(fmt.Sprintf(`<h%d style="text-align:%s">`, level, htmlEscape(align)))
		} else {
			buf.WriteString(fmt.Sprintf("<h%d>", level))
		}
		for _, child := range node.Content {
			renderTipTapHTML(buf, child)
		}
		buf.WriteString(fmt.Sprintf("</h%d>", level))
	case "bulletlist":
		buf.WriteString("<ul>")
		for _, child := range node.Content {
			renderTipTapHTML(buf, child)
		}
		buf.WriteString("</ul>")
	case "orderedlist":
		buf.WriteString("<ol>")
		for _, child := range node.Content {
			renderTipTapHTML(buf, child)
		}
		buf.WriteString("</ol>")
	case "listitem":
		buf.WriteString("<li>")
		for _, child := range node.Content {
			renderTipTapHTML(buf, child)
		}
		buf.WriteString("</li>")
	case "blockquote":
		buf.WriteString("<blockquote>")
		for _, child := range node.Content {
			renderTipTapHTML(buf, child)
		}
		buf.WriteString("</blockquote>")
	case "codeblock":
		buf.WriteString("<pre><code>")
		for _, child := range node.Content {
			renderTipTapHTML(buf, child)
		}
		buf.WriteString("</code></pre>")
	case "hardbreak":
		buf.WriteString("<br />")
	case "horizontalrule":
		buf.WriteString("<hr />")
	case "image":
		src := firstNonEmpty(
			node.attrString("src"),
			node.attrString("dataSrc"),
			node.attrString("attachmentId"),
		)
		if token := extractAttachmentToken(src); token != "" {
			src = "id:" + token
		}
		alt := node.attrString("alt")
		title := node.attrString("title")
		buf.WriteString(`<img src="` + htmlEscape(src) + `" alt="` + htmlEscape(alt) + `"`)
		if title != "" {
			buf.WriteString(` title="` + htmlEscape(title) + `"`)
		}
		buf.WriteString(` />`)
	default:
		for _, child := range node.Content {
			renderTipTapHTML(buf, child)
		}
	}
}

func applyTipTapMarks(content string, marks []*tiptapMark) string {
	if content == "" || len(marks) == 0 {
		return content
	}
	result := content
	for _, mark := range marks {
		if mark == nil {
			continue
		}
		switch strings.ToLower(mark.Type) {
		case "bold":
			result = "<strong>" + result + "</strong>"
		case "italic":
			result = "<em>" + result + "</em>"
		case "underline":
			result = "<u>" + result + "</u>"
		case "strike":
			result = "<s>" + result + "</s>"
		case "code":
			result = "<code>" + result + "</code>"
		case "highlight":
			color := mark.attrString("color")
			if color == "" {
				color = "#fef08a"
			}
			result = `<mark style="background-color:` + htmlEscape(color) + `">` + result + "</mark>"
		case "link":
			href := htmlEscape(mark.attrString("href"))
			if href == "" {
				href = "#"
			}
			target := mark.attrString("target")
			if target == "" {
				target = "_blank"
			}
			result = `<a href="` + href + `" target="` + htmlEscape(target) + `" rel="noopener noreferrer">` + result + "</a>"
		case "textstyle":
			if color := mark.attrString("color"); color != "" {
				result = `<span style="color:` + htmlEscape(color) + `">` + result + "</span>"
			}
		}
	}
	return result
}

func htmlEscape(input string) string {
	if input == "" {
		return ""
	}
	return html.EscapeString(input)
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
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
				attrs := readTagAttributes(tokenizer)
				if placeholder := buildCQImageMarkup(firstNonEmptyAttr(attrs, "src", "data-src", "data-original")); placeholder != "" {
					writeText(placeholder)
				}
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
				attrs := readTagAttributes(tokenizer)
				if placeholder := buildCQImageMarkup(firstNonEmptyAttr(attrs, "src", "data-src", "data-original")); placeholder != "" {
					writeText(placeholder)
				}
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
		fragments = append(fragments, strings.TrimRight(writer.String(), "\n"))
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
	Marks   []*tiptapMark  `json:"marks"`
}

type tiptapMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs"`
}

func (n *tiptapNode) attrString(key string) string {
	if n == nil || n.Attrs == nil {
		return ""
	}
	if value, ok := n.Attrs[key]; ok {
		if str, ok := value.(string); ok {
			return str
		}
		if num, ok := value.(float64); ok {
			return strconv.FormatFloat(num, 'f', -1, 64)
		}
	}
	return ""
}

func (n *tiptapNode) attrFloat(key string) float64 {
	if n == nil || n.Attrs == nil {
		return 0
	}
	if value, ok := n.Attrs[key]; ok {
		switch typed := value.(type) {
		case float64:
			return typed
		case int:
			return float64(typed)
		case string:
			f, _ := strconv.ParseFloat(typed, 64)
			return f
		}
	}
	return 0
}

func (m *tiptapMark) attrString(key string) string {
	if m == nil || m.Attrs == nil {
		return ""
	}
	if value, ok := m.Attrs[key]; ok {
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
	case "image":
		src := firstNonEmpty(
			node.attrString("src"),
			node.attrString("dataSrc"),
			node.attrString("attachmentId"),
		)
		if placeholder := buildCQImageMarkup(src); placeholder != "" {
			w.write(placeholder)
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

var (
	attachmentTokenPattern    = regexp.MustCompile(`^[0-9A-Za-z_-]+$`)
	attachmentBaseURLOverride string
)

func buildCQImageMarkup(raw string) string {
	url := resolveImageURL(raw)
	if url == "" {
		return ""
	}
	return fmt.Sprintf("[CQ:image,file=image,url=%s]", url)
}

func resolveImageURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return value
	}
	if strings.HasPrefix(value, "//") {
		return "https:" + value
	}
	if len(value) >= 3 && strings.EqualFold(value[:3], "id:") {
		value = value[3:]
	}
	if strings.HasPrefix(strings.ToLower(value), "data:") {
		return value
	}
	if strings.HasPrefix(value, "/") {
		if base := resolveAttachmentBaseURL(); base != "" {
			return base + value
		}
		return value
	}
	if attachmentTokenPattern.MatchString(value) {
		return buildAttachmentDownloadURL(value)
	}
	return value
}

func buildAttachmentDownloadURL(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	path := "/api/v1/attachment/" + token
	if base := resolveAttachmentBaseURL(); base != "" {
		return base + path
	}
	return path
}

func resolveAttachmentBaseURL() string {
	if base := strings.TrimSpace(attachmentBaseURLOverride); base != "" {
		return strings.TrimRight(base, "/")
	}
	if cfg := utils.GetConfig(); cfg != nil {
		if base := strings.TrimSpace(cfg.ImageBaseURL); base != "" {
			return normalizeDomainToURL(base)
		}
		domain := strings.TrimSpace(cfg.Domain)
		if domain != "" {
			return normalizeDomainToURL(domain)
		}
	}
	return ""
}

func normalizeDomainToURL(domain string) string {
	trimmed := strings.TrimSpace(domain)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimRight(trimmed, "/")
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return trimmed
	}
	host, port, hasPort := parseDomainHostPort(trimmed)
	formatted := trimmed
	if hasPort {
		formatted = utils.FormatHostPort(host, port)
	} else if isIPv6LiteralHost(trimmed) {
		formatted = utils.EnsureIPv6Bracket(trimmed)
	}
	hostForScheme := host
	if hostForScheme == "" {
		hostForScheme = trimmed
	}
	scheme := "https"
	if isLikelyLocalDomain(hostForScheme) {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, formatted)
}

func parseDomainHostPort(addr string) (string, string, bool) {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return "", "", false
	}
	host, port, err := net.SplitHostPort(trimmed)
	if err != nil {
		return "", "", false
	}
	return host, port, true
}

func isIPv6LiteralHost(value string) bool {
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		return false
	}
	candidate = strings.Trim(candidate, "[]")
	base := candidate
	if idx := strings.LastIndex(base, "%"); idx >= 0 {
		base = base[:idx]
	}
	ip := net.ParseIP(base)
	return ip != nil && ip.To4() == nil
}

func isLikelyLocalDomain(host string) bool {
	target := strings.TrimSpace(host)
	if target == "" {
		return false
	}
	target = strings.Trim(target, "[]")
	lower := strings.ToLower(target)
	if lower == "localhost" {
		return true
	}
	base := lower
	if idx := strings.LastIndex(base, "%"); idx >= 0 {
		base = base[:idx]
	}
	if ip := net.ParseIP(base); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return true
		}
		return false
	}
	return strings.HasPrefix(lower, "127.") ||
		strings.HasPrefix(lower, "10.") ||
		strings.HasPrefix(lower, "192.168.") ||
		strings.HasPrefix(lower, "172.")
}

func readTagAttributes(tokenizer *htmlnode.Tokenizer) map[string]string {
	attrs := make(map[string]string)
	for {
		key, val, more := tokenizer.TagAttr()
		if len(key) == 0 && len(val) == 0 && !more {
			break
		}
		name := strings.ToLower(string(key))
		attrs[name] = string(val)
		if !more {
			break
		}
	}
	return attrs
}

func firstNonEmptyAttr(attrs map[string]string, keys ...string) string {
	if len(attrs) == 0 {
		return ""
	}
	for _, key := range keys {
		if v := strings.TrimSpace(attrs[strings.ToLower(key)]); v != "" {
			return v
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
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
	useBBCode := shouldApplyBBCodeColor(payload)
	for _, msg := range payload.Messages {
		line := ""
		if useBBCode {
			line = buildBBCodeTextLine(payload, &msg)
		} else {
			line = buildPlainTextLine(payload, &msg)
		}
		sb.WriteString(line + "\n")
	}
	return []byte(sb.String()), nil
}

func buildPlainTextLine(payload *ExportPayload, msg *ExportMessage) string {
	if payload == nil || msg == nil {
		return ""
	}
	var prefixParts []string
	if !payload.WithoutTimestamp {
		prefixParts = append(prefixParts, fmt.Sprintf("[%s]", msg.CreatedAt.Format("2006-01-02 15:04:05")))
	}
	header := strings.Join(prefixParts, " ")
	namePart := fmt.Sprintf("<%s>", msg.SenderName)
	content := buildContentBody(msg)
	var parts []string
	if header != "" {
		parts = append(parts, header)
	}
	parts = append(parts, namePart, content)
	return strings.TrimSpace(strings.Join(parts, " "))
}

func buildBBCodeTextLine(payload *ExportPayload, msg *ExportMessage) string {
	if payload == nil || msg == nil {
		return ""
	}
	var headerParts []string
	if !payload.WithoutTimestamp {
		headerParts = append(headerParts, fmt.Sprintf("[%s]", msg.CreatedAt.Format("2006-01-02 15:04:05")))
	}
	headerParts = append(headerParts, fmt.Sprintf("<%s>", msg.SenderName))
	header := strings.Join(headerParts, " ")
	content := buildContentBody(msg)
	color := sanitizeBBCodeColor(msg.SenderColor, "#111111")
	return fmt.Sprintf("[color=silver]%s[/color][color=%s] %s [/color]", header, color, content)
}

func shouldApplyBBCodeColor(payload *ExportPayload) bool {
	if payload == nil || payload.ExtraMeta == nil {
		return false
	}
	raw, ok := payload.ExtraMeta["text_colorize_bbcode"]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		value := strings.TrimSpace(strings.ToLower(v))
		return value == "1" || value == "true" || value == "yes" || value == "on"
	case float64:
		return v != 0
	case int:
		return v != 0
	default:
		return false
	}
}

func sanitizeBBCodeColor(input string, fallback string) string {
	value := strings.ToLower(strings.TrimSpace(input))
	if value == "" {
		return fallback
	}
	if !strings.HasPrefix(value, "#") {
		return fallback
	}
	hex := strings.TrimPrefix(value, "#")
	normalized, ok := normalizeHexColor(hex)
	if !ok {
		return fallback
	}
	return "#" + normalized
}

func normalizeHexColor(hex string) (string, bool) {
	if len(hex) == 3 && isHexDigits(hex) {
		var builder strings.Builder
		for _, ch := range hex {
			builder.WriteRune(ch)
			builder.WriteRune(ch)
		}
		return builder.String(), true
	}
	if len(hex) == 6 && isHexDigits(hex) {
		return hex, true
	}
	return "", false
}

func isHexDigits(input string) bool {
	for _, ch := range input {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
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
