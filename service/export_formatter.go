package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	htmltemplate "html/template"
	"net"
	neturl "net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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
	ID               string    `json:"id"`
	SenderID         string    `json:"sender_id"`
	SenderIdentityID string    `json:"sender_identity_id,omitempty"`
	SenderName       string    `json:"sender_name"`
	SenderColor      string    `json:"sender_color"`
	SenderAvatar     string    `json:"sender_avatar,omitempty"`
	IsMerged         bool      `json:"is_merged,omitempty"`
	IcMode           string    `json:"ic_mode"`
	IsWhisper        bool      `json:"is_whisper"`
	IsArchived       bool      `json:"is_archived"`
	IsBot            bool      `json:"is_bot"`
	CreatedAt        time.Time `json:"created_at"`
	Content          string    `json:"content"`
	ContentHTML      string    `json:"content_html,omitempty"` // HTML 渲染结果，用于 HTML 导出
	WhisperTargets   []string  `json:"whisper_targets"`
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
	InlineAssets     map[string]string      `json:"inline_assets,omitempty"`
	Messages         []ExportMessage        `json:"messages"`
	Meta             map[string]bool        `json:"meta"`
	Count            int                    `json:"count"`
	WithoutTimestamp bool                   `json:"without_timestamp"`
	IncludeImages    bool                   `json:"include_images"`
	IncludeDiceCmds  bool                   `json:"include_dice_commands"`
	ExtraMeta        map[string]interface{} `json:"extra_meta,omitempty"`
}

type quickFormatRenderOptions struct {
	DisableInlineCode bool
	DisableAll        bool
}

const diceLogVersion = 105

var stickyNoteEmbedURLPattern = regexp.MustCompile(`^https?://[^\s<>"']*#/([A-Za-z0-9_-]+)/([A-Za-z0-9_-]+)\?([^\s#]+)$`)

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

func buildExportPayload(job *model.MessageExportJobModel, channelName string, messages []*model.MessageModel, ctx *payloadContext, extra *exportExtraOptions) *ExportPayload {
	includeImages := true
	includeDiceCommand := true
	if extra != nil {
		includeImages = extra.IncludeImages
		includeDiceCommand = extra.IncludeDiceCommand
	}
	identityResolver := newIdentityResolver(job.ChannelID)
	imageLayoutResolver := newExportImageLayoutResolver(job.ChannelID)
	stickyNoteResolver := newStickyNoteExportResolver(job.ChannelID)
	exportMessages := make([]ExportMessage, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		originalContent := msg.Content
		exportContent := originalContent
		var htmlContent string
		if expanded, ok := stickyNoteResolver.render(originalContent, includeImages); ok {
			exportContent = expanded.Plain
			htmlContent = expanded.HTML
		}
		plainContent := buildFilteredPlainContent(exportContent, includeImages)
		if plainContent == "" {
			continue
		}
		isBotMessage := msg.User != nil && msg.User.IsBot
		if !includeDiceCommand && !isBotMessage && isSingleLineDiceCommand(plainContent) {
			continue
		}
		if htmlContent == "" {
			if shouldDisableInlineCodeForBotCommand(originalContent) {
				htmlContent = renderBotCommandRawHTML(originalContent)
			} else if html, ok := convertTipTapToHTML(originalContent); ok {
				htmlContent = html
			} else {
				htmlContent = enhancePlainContentForHTMLExport(originalContent)
			}
		}
		// 将 <at> 标签转换为带样式的 HTML
		htmlContent = convertAtTagsToHTML(htmlContent)
		htmlContent = imageLayoutResolver.enhanceHTML(htmlContent)
		if !includeImages {
			htmlContent = stripImageTagsFromHTML(htmlContent)
		}
		exportMessages = append(exportMessages, ExportMessage{
			ID:               msg.ID,
			SenderID:         msg.UserID,
			SenderIdentityID: strings.TrimSpace(msg.SenderIdentityID),
			SenderName:       resolveSenderName(msg),
			SenderColor:      msg.SenderIdentityColor,
			SenderAvatar:     resolveSenderAvatar(msg),
			IsMerged:         msg.MergedMessages > 1,
			IcMode:           fallbackIcMode(msg.ICMode),
			IsWhisper:        msg.IsWhisper,
			IsArchived:       msg.IsArchived,
			IsBot:            msg.User != nil && msg.User.IsBot,
			CreatedAt:        msg.CreatedAt,
			Content:          exportContent,
			ContentHTML:      htmlContent,
			WhisperTargets:   extractWhisperTargets(msg, job.ChannelID, identityResolver),
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
		IncludeImages:    includeImages,
		IncludeDiceCmds:  includeDiceCommand,
		Meta: map[string]bool{
			"include_ooc":           job.IncludeOOC,
			"include_archived":      job.IncludeArchived,
			"include_images":        includeImages,
			"include_dice_commands": includeDiceCommand,
			"merge_messages":        job.MergeMessages,
			"without_timestamp":     job.WithoutTimestamp,
		},
	}
}

type stickyNoteExportRender struct {
	Plain string
	HTML  string
}

type stickyNoteExportResolver struct {
	channelID string
	cache     map[string]*model.StickyNoteModel
}

type stickyNoteEmbedTarget struct {
	WorldID   string
	ChannelID string
	NoteID    string
	RawLink   string
}

type stickyNoteExportAdapter struct {
	TypeLabel string
	Plain     func(note *model.StickyNoteModel, includeImages bool) string
	HTML      func(note *model.StickyNoteModel, includeImages bool) string
}

var stickyNoteExportAdapters = map[model.StickyNoteType]stickyNoteExportAdapter{
	model.StickyNoteTypeText: {
		TypeLabel: "富文本便签",
		Plain:     renderTextStickyNotePlain,
		HTML:      renderTextStickyNoteHTML,
	},
	model.StickyNoteTypeCounter: {
		TypeLabel: "计数器便签",
		Plain:     renderCounterStickyNotePlain,
		HTML:      renderCounterStickyNoteHTML,
	},
	model.StickyNoteTypeList: {
		TypeLabel: "清单便签",
		Plain:     renderListStickyNotePlain,
		HTML:      renderListStickyNoteHTML,
	},
	model.StickyNoteTypeSlider: {
		TypeLabel: "滑条便签",
		Plain:     renderSliderStickyNotePlain,
		HTML:      renderSliderStickyNoteHTML,
	},
	model.StickyNoteTypeChat: {
		TypeLabel: "聊天便签",
		Plain:     renderTextStickyNotePlain,
		HTML:      renderTextStickyNoteHTML,
	},
	model.StickyNoteTypeTimer: {
		TypeLabel: "计时器便签",
		Plain:     renderTimerStickyNotePlain,
		HTML:      renderTimerStickyNoteHTML,
	},
	model.StickyNoteTypeClock: {
		TypeLabel: "时钟便签",
		Plain:     renderClockStickyNotePlain,
		HTML:      renderClockStickyNoteHTML,
	},
	model.StickyNoteTypeRoundCounter: {
		TypeLabel: "回合计数便签",
		Plain:     renderRoundCounterStickyNotePlain,
		HTML:      renderRoundCounterStickyNoteHTML,
	},
}

func newStickyNoteExportResolver(channelID string) *stickyNoteExportResolver {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil
	}
	return &stickyNoteExportResolver{
		channelID: channelID,
		cache:     make(map[string]*model.StickyNoteModel),
	}
}

func (r *stickyNoteExportResolver) render(content string, includeImages bool) (stickyNoteExportRender, bool) {
	if r == nil {
		return stickyNoteExportRender{}, false
	}
	targets, ok := parseOnlyStickyNoteEmbedTargets(content)
	if !ok || len(targets) == 0 {
		return stickyNoteExportRender{}, false
	}
	plainParts := make([]string, 0, len(targets))
	htmlParts := make([]string, 0, len(targets))
	for _, target := range targets {
		if target.ChannelID != r.channelID {
			return stickyNoteExportRender{}, false
		}
		note := r.load(target.NoteID)
		if note == nil || strings.TrimSpace(note.ChannelID) != r.channelID {
			return stickyNoteExportRender{}, false
		}
		adapter := resolveStickyNoteExportAdapter(note.NoteType)
		plain := buildStickyNoteExportPlain(note, adapter, includeImages)
		html := buildStickyNoteExportHTML(note, adapter, includeImages)
		if strings.TrimSpace(plain) == "" && strings.TrimSpace(html) == "" {
			return stickyNoteExportRender{}, false
		}
		plainParts = append(plainParts, plain)
		htmlParts = append(htmlParts, html)
	}
	return stickyNoteExportRender{
		Plain: strings.Join(plainParts, "\n\n"),
		HTML:  strings.Join(htmlParts, ""),
	}, true
}

func (r *stickyNoteExportResolver) load(noteID string) *model.StickyNoteModel {
	noteID = strings.TrimSpace(noteID)
	if noteID == "" {
		return nil
	}
	if note, ok := r.cache[noteID]; ok {
		return note
	}
	note, err := model.StickyNoteGet(noteID)
	if err != nil {
		r.cache[noteID] = nil
		return nil
	}
	r.cache[noteID] = note
	return note
}

func parseSingleStickyNoteEmbedTarget(content string) (stickyNoteEmbedTarget, bool) {
	targets, ok := parseOnlyStickyNoteEmbedTargets(content)
	if !ok || len(targets) != 1 {
		return stickyNoteEmbedTarget{}, false
	}
	return targets[0], true
}

func parseOnlyStickyNoteEmbedTargets(content string) ([]stickyNoteEmbedTarget, bool) {
	candidate := normalizeStickyNoteEmbedCandidate(content)
	if candidate == "" {
		return nil, false
	}
	parts := splitStickyNoteEmbedOnlyParts(candidate)
	if len(parts) == 0 {
		return nil, false
	}
	targets := make([]stickyNoteEmbedTarget, 0, len(parts))
	for _, part := range parts {
		target, ok := parseStickyNoteEmbedTargetPart(part)
		if !ok {
			return nil, false
		}
		targets = append(targets, target)
	}
	return targets, true
}

func parseStickyNoteEmbedTargetPart(candidate string) (stickyNoteEmbedTarget, bool) {
	candidate = trimStickyNoteLinkWrapper(candidate)
	if candidate == "" || containsUnicodeSpace(candidate) {
		return stickyNoteEmbedTarget{}, false
	}
	match := stickyNoteEmbedURLPattern.FindStringSubmatch(candidate)
	if len(match) != 4 {
		return stickyNoteEmbedTarget{}, false
	}
	query, err := neturl.ParseQuery(strings.ReplaceAll(match[3], "&amp;", "&"))
	if err != nil {
		return stickyNoteEmbedTarget{}, false
	}
	noteID := strings.TrimSpace(query.Get("snote"))
	if noteID == "" {
		return stickyNoteEmbedTarget{}, false
	}
	return stickyNoteEmbedTarget{
		WorldID:   match[1],
		ChannelID: match[2],
		NoteID:    noteID,
		RawLink:   candidate,
	}, true
}

func splitStickyNoteEmbedOnlyParts(candidate string) []string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return nil
	}
	fields := strings.Fields(candidate)
	if len(fields) == 0 {
		return nil
	}
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		part := trimStickyNoteLinkWrapper(field)
		if part == "" {
			return nil
		}
		parts = append(parts, part)
	}
	return parts
}

func trimStickyNoteLinkWrapper(input string) string {
	result := strings.TrimSpace(strings.ReplaceAll(input, "&amp;", "&"))
	for {
		trimmed := strings.TrimSpace(result)
		if len(trimmed) >= 2 {
			switch {
			case strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")"):
				result = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
				continue
			case strings.HasPrefix(trimmed, "（") && strings.HasSuffix(trimmed, "）"):
				result = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "（"), "）"))
				continue
			}
		}
		return trimmed
	}
}

func normalizeStickyNoteEmbedCandidate(content string) string {
	candidate := strings.TrimSpace(strings.ReplaceAll(content, "&amp;", "&"))
	if candidate == "" {
		return ""
	}
	if _, ok := parseStickyNoteEmbedURLCandidate(candidate); ok {
		return candidate
	}
	if plain, ok := extractTipTapPlainText(candidate); ok {
		return strings.TrimSpace(strings.ReplaceAll(plain, "&amp;", "&"))
	}
	return candidate
}

func parseStickyNoteEmbedURLCandidate(candidate string) (stickyNoteEmbedTarget, bool) {
	match := stickyNoteEmbedURLPattern.FindStringSubmatch(candidate)
	if len(match) != 4 {
		return stickyNoteEmbedTarget{}, false
	}
	return stickyNoteEmbedTarget{WorldID: match[1], ChannelID: match[2], RawLink: candidate}, true
}

func containsUnicodeSpace(input string) bool {
	for _, r := range input {
		if unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

func resolveStickyNoteExportAdapter(noteType model.StickyNoteType) stickyNoteExportAdapter {
	if adapter, ok := stickyNoteExportAdapters[noteType]; ok {
		return adapter
	}
	return stickyNoteExportAdapter{
		TypeLabel: "便签",
		Plain:     renderTextStickyNotePlain,
		HTML:      renderTextStickyNoteHTML,
	}
}

func buildStickyNoteExportPlain(note *model.StickyNoteModel, adapter stickyNoteExportAdapter, includeImages bool) string {
	if note == nil {
		return ""
	}
	title := strings.TrimSpace(note.Title)
	if title == "" {
		title = "未命名便签"
	}
	body := ""
	if adapter.Plain != nil {
		body = adapter.Plain(note, includeImages)
	}
	body = normalizePlainText(body)
	if body == "" {
		body = "（空便签）"
	}
	return normalizePlainText(fmt.Sprintf("[便签: %s]\n%s", title, body))
}

func buildStickyNoteExportHTML(note *model.StickyNoteModel, adapter stickyNoteExportAdapter, includeImages bool) string {
	if note == nil {
		return ""
	}
	title := strings.TrimSpace(note.Title)
	if title == "" {
		title = "未命名便签"
	}
	label := adapter.TypeLabel
	if label == "" {
		label = "便签"
	}
	body := ""
	if adapter.HTML != nil {
		body = adapter.HTML(note, includeImages)
	}
	if strings.TrimSpace(body) == "" {
		body = `<p class="export-sticky-note__empty">（空便签）</p>`
	}
	color := resolveStickyNoteExportColor(note.Color)
	return `<section class="export-sticky-note export-sticky-note--` + htmlEscape(normalizeStickyNoteColorName(note.Color)) + `" style="--export-sticky-note-accent:` + htmlEscape(color) + `">` +
		`<header class="export-sticky-note__header">` +
		`<span class="export-sticky-note__title">` + htmlEscape(title) + `</span>` +
		`<span class="export-sticky-note__type">` + htmlEscape(label) + `</span>` +
		`</header>` +
		`<div class="export-sticky-note__body">` + body + `</div>` +
		`</section>`
}

func renderTextStickyNotePlain(note *model.StickyNoteModel, includeImages bool) string {
	if note == nil {
		return ""
	}
	if strings.TrimSpace(note.Content) != "" {
		return buildFilteredPlainContent(note.Content, includeImages)
	}
	return buildFilteredPlainContent(note.ContentText, includeImages)
}

func renderTextStickyNoteHTML(note *model.StickyNoteModel, includeImages bool) string {
	if note == nil {
		return ""
	}
	raw := strings.TrimSpace(note.Content)
	if raw == "" {
		raw = strings.TrimSpace(note.ContentText)
	}
	if raw == "" {
		return ""
	}
	var rendered string
	if html, ok := convertTipTapToHTML(raw); ok {
		rendered = html
	} else {
		rendered = enhancePlainContentForHTMLExport(raw)
	}
	if !includeImages {
		rendered = stripImageTagsFromHTML(rendered)
	}
	return rendered
}

func renderDefaultTypedStickyNoteHTML(note *model.StickyNoteModel, includeImages bool) string {
	plain := resolveTypedStickyNotePlain(note, includeImages)
	if plain == "" {
		return renderTextStickyNoteHTML(note, includeImages)
	}
	lines := strings.Split(plain, "\n")
	var buf strings.Builder
	buf.WriteString("<ul>")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		buf.WriteString("<li>")
		buf.WriteString(htmlEscape(line))
		buf.WriteString("</li>")
	}
	buf.WriteString("</ul>")
	return buf.String()
}

func resolveTypedStickyNotePlain(note *model.StickyNoteModel, includeImages bool) string {
	switch note.NoteType {
	case model.StickyNoteTypeCounter:
		return renderCounterStickyNotePlain(note, includeImages)
	case model.StickyNoteTypeList:
		return renderListStickyNotePlain(note, includeImages)
	case model.StickyNoteTypeSlider:
		return renderSliderStickyNotePlain(note, includeImages)
	case model.StickyNoteTypeTimer:
		return renderTimerStickyNotePlain(note, includeImages)
	case model.StickyNoteTypeClock:
		return renderClockStickyNotePlain(note, includeImages)
	case model.StickyNoteTypeRoundCounter:
		return renderRoundCounterStickyNotePlain(note, includeImages)
	default:
		return renderTextStickyNotePlain(note, includeImages)
	}
}

func renderCounterStickyNotePlain(note *model.StickyNoteModel, includeImages bool) string {
	var data struct {
		Value float64 `json:"value"`
		Max   float64 `json:"max"`
	}
	if !decodeStickyNoteTypeData(note, &data) {
		return renderTextStickyNotePlain(note, includeImages)
	}
	if data.Max > 0 {
		return fmt.Sprintf("计数: %s / %s", formatStickyNoteNumber(data.Value), formatStickyNoteNumber(data.Max))
	}
	return "计数: " + formatStickyNoteNumber(data.Value)
}

func renderCounterStickyNoteHTML(note *model.StickyNoteModel, includeImages bool) string {
	var data struct {
		Value float64 `json:"value"`
		Max   float64 `json:"max"`
	}
	if !decodeStickyNoteTypeData(note, &data) {
		return renderTextStickyNoteHTML(note, includeImages)
	}
	display := formatStickyNoteNumber(data.Value)
	if data.Max > 0 {
		display += "/" + formatStickyNoteNumber(data.Max)
	}
	return `<div class="export-sticky-note-counter">` +
		`<span class="export-sticky-note-counter__button">-</span>` +
		`<span class="export-sticky-note-counter__value">` + htmlEscape(display) + `</span>` +
		`<span class="export-sticky-note-counter__button">+</span>` +
		`</div>`
}

func renderListStickyNotePlain(note *model.StickyNoteModel, includeImages bool) string {
	var data struct {
		Items []struct {
			Content string `json:"content"`
			Checked bool   `json:"checked"`
			Indent  int    `json:"indent"`
		} `json:"items"`
	}
	if !decodeStickyNoteTypeData(note, &data) || len(data.Items) == 0 {
		return renderTextStickyNotePlain(note, includeImages)
	}
	lines := make([]string, 0, len(data.Items))
	for _, item := range data.Items {
		text := normalizePlainText(item.Content)
		if text == "" {
			continue
		}
		prefix := "[ ] "
		if item.Checked {
			prefix = "[x] "
		}
		if item.Indent > 0 {
			prefix += strings.Repeat("  ", item.Indent)
		}
		lines = append(lines, prefix+text)
	}
	return strings.Join(lines, "\n")
}

func renderListStickyNoteHTML(note *model.StickyNoteModel, includeImages bool) string {
	plain := renderListStickyNotePlain(note, includeImages)
	if plain == "" {
		return renderTextStickyNoteHTML(note, includeImages)
	}
	var buf strings.Builder
	buf.WriteString(`<ul class="export-sticky-note__list">`)
	for _, line := range strings.Split(plain, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		className := "export-sticky-note-list__item"
		if strings.HasPrefix(line, "[x]") {
			className += " export-sticky-note-list__item--checked"
		}
		buf.WriteString(`<li class="` + className + `">`)
		checked := strings.HasPrefix(line, "[x]")
		text := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "[x]"), "[ ]"))
		if strings.HasPrefix(line, "[ ]") {
			text = strings.TrimSpace(strings.TrimPrefix(line, "[ ]"))
		}
		box := " "
		if checked {
			box = "✓"
		}
		buf.WriteString(`<span class="export-sticky-note-list__checkbox">` + box + `</span>`)
		buf.WriteString(`<span class="export-sticky-note-list__text">` + htmlEscape(text) + `</span>`)
		buf.WriteString("</li>")
	}
	buf.WriteString("</ul>")
	return buf.String()
}

func renderSliderStickyNotePlain(note *model.StickyNoteModel, includeImages bool) string {
	var data struct {
		Value float64 `json:"value"`
		Min   float64 `json:"min"`
		Max   float64 `json:"max"`
		Step  float64 `json:"step"`
	}
	if !decodeStickyNoteTypeData(note, &data) {
		return renderTextStickyNotePlain(note, includeImages)
	}
	return fmt.Sprintf("滑条: %s（范围 %s-%s，步进 %s）", formatStickyNoteNumber(data.Value), formatStickyNoteNumber(data.Min), formatStickyNoteNumber(data.Max), formatStickyNoteNumber(data.Step))
}

func renderSliderStickyNoteHTML(note *model.StickyNoteModel, includeImages bool) string {
	var data struct {
		Value float64 `json:"value"`
		Min   float64 `json:"min"`
		Max   float64 `json:"max"`
		Step  float64 `json:"step"`
	}
	if !decodeStickyNoteTypeData(note, &data) {
		return renderTextStickyNoteHTML(note, includeImages)
	}
	percent := 0.0
	if data.Max > data.Min {
		percent = ((data.Value - data.Min) / (data.Max - data.Min)) * 100
	}
	percent = clampFloat(percent, 0, 100)
	return `<div class="export-sticky-note-slider">` +
		`<div class="export-sticky-note-slider__value">` + htmlEscape(formatStickyNoteNumber(data.Value)) + `</div>` +
		`<div class="export-sticky-note-slider__track"><div class="export-sticky-note-slider__fill" style="width:` + formatStickyNotePercent(percent) + `%"></div></div>` +
		`<div class="export-sticky-note-slider__range"><span>` + htmlEscape(formatStickyNoteNumber(data.Min)) + `</span><span>` + htmlEscape(formatStickyNoteNumber(data.Max)) + `</span></div>` +
		`</div>`
}

func renderTimerStickyNotePlain(note *model.StickyNoteModel, includeImages bool) string {
	var data struct {
		StartTime  float64 `json:"startTime"`
		BaseValue  float64 `json:"baseValue"`
		Direction  string  `json:"direction"`
		Running    bool    `json:"running"`
		ResetValue float64 `json:"resetValue"`
	}
	if !decodeStickyNoteTypeData(note, &data) {
		return renderTextStickyNotePlain(note, includeImages)
	}
	state := "已暂停"
	if data.Running {
		state = "运行中"
	}
	direction := "正计时"
	if data.Direction == "down" {
		direction = "倒计时"
	}
	current := stickyNoteTimerCurrentValue(data.StartTime, data.BaseValue, data.Direction, data.Running)
	return fmt.Sprintf("计时器: %s，%s，当前值 %s，重置值 %s", state, direction, formatStickyNoteDuration(current), formatStickyNoteDuration(data.ResetValue))
}

func renderTimerStickyNoteHTML(note *model.StickyNoteModel, includeImages bool) string {
	var data struct {
		StartTime  float64 `json:"startTime"`
		BaseValue  float64 `json:"baseValue"`
		Direction  string  `json:"direction"`
		Running    bool    `json:"running"`
		ResetValue float64 `json:"resetValue"`
	}
	if !decodeStickyNoteTypeData(note, &data) {
		return renderTextStickyNoteHTML(note, includeImages)
	}
	current := stickyNoteTimerCurrentValue(data.StartTime, data.BaseValue, data.Direction, data.Running)
	status := "已暂停"
	if data.Running {
		status = "运行中"
	}
	direction := "正计时"
	if data.Direction == "down" {
		direction = "倒计时"
	}
	return `<div class="export-sticky-note-timer">` +
		`<div class="export-sticky-note-timer__display">` + htmlEscape(formatStickyNoteDuration(current)) + `</div>` +
		`<div class="export-sticky-note-timer__meta"><span>` + htmlEscape(status) + `</span><span>` + htmlEscape(direction) + `</span><span>重置 ` + htmlEscape(formatStickyNoteDuration(data.ResetValue)) + `</span></div>` +
		`</div>`
}

func renderClockStickyNotePlain(note *model.StickyNoteModel, includeImages bool) string {
	var data struct {
		Segments float64 `json:"segments"`
		Filled   float64 `json:"filled"`
	}
	if !decodeStickyNoteTypeData(note, &data) {
		return renderTextStickyNotePlain(note, includeImages)
	}
	return fmt.Sprintf("时钟: %s / %s", formatStickyNoteNumber(data.Filled), formatStickyNoteNumber(data.Segments))
}

func renderClockStickyNoteHTML(note *model.StickyNoteModel, includeImages bool) string {
	var data struct {
		Segments float64 `json:"segments"`
		Filled   float64 `json:"filled"`
	}
	if !decodeStickyNoteTypeData(note, &data) {
		return renderTextStickyNoteHTML(note, includeImages)
	}
	segments := clampFloat(data.Segments, 1, 120)
	filled := clampFloat(data.Filled, 0, segments)
	filledDeg := 0.0
	if segments > 0 {
		filledDeg = (filled / segments) * 360
	}
	return `<div class="export-sticky-note-clock">` +
		`<div class="export-sticky-note-clock__dial" style="--export-sticky-note-clock-filled:` + formatStickyNotePercent(filledDeg) + `deg;--export-sticky-note-clock-step:` + formatStickyNotePercent(360/segments) + `deg">` +
		`<div class="export-sticky-note-clock__center">` + htmlEscape(formatStickyNoteNumber(filled)) + `/` + htmlEscape(formatStickyNoteNumber(segments)) + `</div>` +
		`</div>` +
		`</div>`
}

func renderRoundCounterStickyNotePlain(note *model.StickyNoteModel, includeImages bool) string {
	var data struct {
		Round     float64 `json:"round"`
		Direction string  `json:"direction"`
		Limit     float64 `json:"limit"`
	}
	if !decodeStickyNoteTypeData(note, &data) {
		return renderTextStickyNotePlain(note, includeImages)
	}
	direction := "递增"
	if data.Direction == "down" {
		direction = "递减"
	}
	if data.Limit > 0 {
		return fmt.Sprintf("回合: %s（%s，限制 %s）", formatStickyNoteNumber(data.Round), direction, formatStickyNoteNumber(data.Limit))
	}
	return fmt.Sprintf("回合: %s（%s）", formatStickyNoteNumber(data.Round), direction)
}

func renderRoundCounterStickyNoteHTML(note *model.StickyNoteModel, includeImages bool) string {
	var data struct {
		Round     float64 `json:"round"`
		Direction string  `json:"direction"`
		Limit     float64 `json:"limit"`
	}
	if !decodeStickyNoteTypeData(note, &data) {
		return renderTextStickyNoteHTML(note, includeImages)
	}
	display := formatStickyNoteNumber(data.Round)
	if data.Limit > 0 {
		display += "/" + formatStickyNoteNumber(data.Limit)
	}
	direction := "递增"
	if data.Direction == "down" {
		direction = "递减"
	}
	return `<div class="export-sticky-note-round">` +
		`<div class="export-sticky-note-round__label">回合</div>` +
		`<div class="export-sticky-note-round__value">` + htmlEscape(display) + `</div>` +
		`<div class="export-sticky-note-round__direction">` + htmlEscape(direction) + `</div>` +
		`</div>`
}

func decodeStickyNoteTypeData(note *model.StickyNoteModel, target any) bool {
	if note == nil || strings.TrimSpace(note.TypeData) == "" || target == nil {
		return false
	}
	return json.Unmarshal([]byte(note.TypeData), target) == nil
}

func formatStickyNoteNumber(value float64) string {
	if value == float64(int64(value)) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func formatStickyNotePercent(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func clampFloat(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func stickyNoteTimerCurrentValue(startTime, baseValue float64, direction string, running bool) float64 {
	if !running || startTime <= 0 {
		return baseValue
	}
	elapsed := float64(time.Now().UnixMilli()-int64(startTime)) / 1000
	if direction == "down" {
		return baseValue - elapsed
	}
	return baseValue + elapsed
}

func formatStickyNoteDuration(seconds float64) string {
	total := int64(seconds)
	if seconds < 0 && seconds != float64(total) {
		total--
	}
	sign := ""
	if total < 0 {
		sign = "-"
		total = -total
	}
	hours := total / 3600
	minutes := (total % 3600) / 60
	secs := total % 60
	return fmt.Sprintf("%s%02d:%02d:%02d", sign, hours, minutes, secs)
}

func normalizeStickyNoteColorName(color string) string {
	color = strings.ToLower(strings.TrimSpace(color))
	if color == "" {
		return "yellow"
	}
	for _, r := range color {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return "custom"
		}
	}
	return color
}

func resolveStickyNoteExportColor(color string) string {
	switch normalizeStickyNoteColorName(color) {
	case "yellow":
		return "#f59e0b"
	case "pink":
		return "#ec4899"
	case "green":
		return "#22c55e"
	case "blue":
		return "#3b82f6"
	case "purple":
		return "#a855f7"
	case "orange":
		return "#f97316"
	default:
		return "#64748b"
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
	if msg.SenderIdentityIsTemporary {
		return ""
	}
	if msg.User != nil {
		avatar := strings.TrimSpace(msg.User.Avatar)
		if avatar != "" {
			return avatar
		}
	}
	return ""
}

type exportImageLayout struct {
	Width  int
	Height int
}

type exportImageLayoutResolver struct {
	channelID string
	cache     map[string]*exportImageLayout
}

func newExportImageLayoutResolver(channelID string) *exportImageLayoutResolver {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil
	}
	return &exportImageLayoutResolver{
		channelID: channelID,
		cache:     make(map[string]*exportImageLayout),
	}
}

func (r *exportImageLayoutResolver) enhanceHTML(content string) string {
	if r == nil || !strings.Contains(strings.ToLower(content), "<img") {
		return content
	}
	nodes, err := htmlnode.ParseFragment(strings.NewReader(content), nil)
	if err != nil {
		return content
	}
	attachmentIDs := make([]string, 0, 4)
	for _, node := range nodes {
		collectImageAttachmentIDsFromHTML(node, &attachmentIDs)
	}
	r.ensureLayouts(attachmentIDs)

	changed := false
	for _, node := range nodes {
		if r.decorateImageNodes(node) {
			changed = true
		}
	}
	if !changed {
		return content
	}

	var buf bytes.Buffer
	for _, node := range nodes {
		if err := htmlnode.Render(&buf, node); err != nil {
			return content
		}
	}
	return buf.String()
}

func collectImageAttachmentIDsFromHTML(node *htmlnode.Node, out *[]string) {
	if node == nil || out == nil {
		return
	}
	if node.Type == htmlnode.ElementNode && strings.EqualFold(node.Data, "img") {
		for _, attr := range node.Attr {
			if !strings.EqualFold(attr.Key, "src") {
				continue
			}
			if token := extractAttachmentToken(attr.Val); token != "" {
				*out = append(*out, token)
			}
			break
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		collectImageAttachmentIDsFromHTML(child, out)
	}
}

func (r *exportImageLayoutResolver) ensureLayouts(attachmentIDs []string) {
	if r == nil || len(attachmentIDs) == 0 {
		return
	}
	missing := make([]string, 0, len(attachmentIDs))
	seen := make(map[string]struct{}, len(attachmentIDs))
	for _, rawID := range attachmentIDs {
		attachmentID := strings.TrimSpace(rawID)
		if attachmentID == "" {
			continue
		}
		if _, ok := seen[attachmentID]; ok {
			continue
		}
		seen[attachmentID] = struct{}{}
		if _, ok := r.cache[attachmentID]; ok {
			continue
		}
		missing = append(missing, attachmentID)
	}
	if len(missing) == 0 {
		return
	}
	layouts, err := model.ChannelAttachmentImageLayoutBatchGet(r.channelID, missing)
	if err != nil {
		for _, attachmentID := range missing {
			r.cache[attachmentID] = nil
		}
		return
	}
	for _, attachmentID := range missing {
		r.cache[attachmentID] = nil
	}
	for _, layout := range layouts {
		if layout == nil {
			continue
		}
		r.cache[strings.TrimSpace(layout.AttachmentID)] = &exportImageLayout{
			Width:  layout.Width,
			Height: layout.Height,
		}
	}
}

func (r *exportImageLayoutResolver) decorateImageNodes(node *htmlnode.Node) bool {
	if node == nil {
		return false
	}
	changed := false
	if node.Type == htmlnode.ElementNode && strings.EqualFold(node.Data, "img") {
		srcIndex := -1
		styleIndex := -1
		dataAttachmentIndex := -1
		attachmentID := ""
		for idx, attr := range node.Attr {
			switch strings.ToLower(strings.TrimSpace(attr.Key)) {
			case "src":
				srcIndex = idx
				attachmentID = extractAttachmentToken(attr.Val)
			case "style":
				styleIndex = idx
			case "data-attachment-id":
				dataAttachmentIndex = idx
			}
		}
		if attachmentID != "" {
			if dataAttachmentIndex >= 0 {
				if node.Attr[dataAttachmentIndex].Val != attachmentID {
					node.Attr[dataAttachmentIndex].Val = attachmentID
					changed = true
				}
			} else {
				node.Attr = append(node.Attr, htmlnode.Attribute{Key: "data-attachment-id", Val: attachmentID})
				changed = true
			}
			if layout := r.cache[attachmentID]; layout != nil && layout.Width > 0 && layout.Height > 0 {
				styleValue := fmt.Sprintf("width:%dpx;height:%dpx;max-width:none;max-height:none;", layout.Width, layout.Height)
				if styleIndex >= 0 {
					merged := mergeInlineStyle(node.Attr[styleIndex].Val, styleValue)
					if merged != node.Attr[styleIndex].Val {
						node.Attr[styleIndex].Val = merged
						changed = true
					}
				} else {
					node.Attr = append(node.Attr, htmlnode.Attribute{Key: "style", Val: styleValue})
					changed = true
				}
			}
		} else if srcIndex >= 0 {
			_ = srcIndex
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if r.decorateImageNodes(child) {
			changed = true
		}
	}
	return changed
}

func mergeInlineStyle(existing string, additions string) string {
	existing = strings.TrimSpace(existing)
	additions = strings.TrimSpace(additions)
	if existing == "" {
		return additions
	}
	if additions == "" {
		return existing
	}
	if !strings.HasSuffix(existing, ";") {
		existing += ";"
	}
	return existing + additions
}

func convertTipTapToHTML(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", false
	}

	// 使用统一的多 JSON 块提取函数
	result, found := extractAllTipTapJSON(trimmed, true)
	if !found {
		return "", false
	}
	return result, true
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
	result := applyTipTapCombinedTextStyle(content, marks)
	for _, mark := range marks {
		if mark == nil {
			continue
		}
		switch strings.ToLower(mark.Type) {
		case "ruby":
			rubyText := htmlEscape(strings.TrimSpace(mark.attrString("rubyText")))
			if rubyText == "" {
				continue
			}
			variables := make([]string, 0, 5)
			dataAttrs := []string{`data-ruby-text="` + rubyText + `"`}
			pushRubyAttr := func(key string, cssVar string) {
				value := htmlEscape(strings.TrimSpace(mark.attrString(key)))
				if value == "" {
					return
				}
				dataKey := camelToDataAttr(key)
				dataAttrs = append(dataAttrs, dataKey+`="`+value+`"`)
				variables = append(variables, cssVar+`: `+value)
			}
			pushRubyAttr("rubyFontFamily", "--ruby-font-family")
			pushRubyAttr("rubyFontSize", "--ruby-font-size")
			pushRubyAttr("rubyColor", "--ruby-color")
			pushRubyAttr("rubyFontWeight", "--ruby-font-weight")
			pushRubyAttr("rubyFontStyle", "--ruby-font-style")
			styleAttr := ""
			if len(variables) > 0 {
				styleAttr = ` style="` + strings.Join(variables, "; ") + `"`
			}
			result = `<ruby class="tiptap-ruby" ` + strings.Join(dataAttrs, " ") + styleAttr + `>` + result + `<rt>` + rubyText + `</rt></ruby>`
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
			continue
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
			continue
		}
	}
	return result
}

func applyTipTapCombinedTextStyle(content string, marks []*tiptapMark) string {
	if content == "" || len(marks) == 0 {
		return content
	}

	var textStyleMark *tiptapMark
	var highlightMark *tiptapMark
	for _, mark := range marks {
		if mark == nil {
			continue
		}
		switch strings.ToLower(mark.Type) {
		case "textstyle":
			if textStyleMark == nil {
				textStyleMark = mark
			}
		case "highlight":
			if highlightMark == nil {
				highlightMark = mark
			}
		}
	}

	if textStyleMark == nil && highlightMark == nil {
		return content
	}

	attrs := make([]string, 0, 4)
	styles := make([]string, 0, 4)
	if textStyleMark != nil {
		if fontAssetID := strings.TrimSpace(textStyleMark.attrString("fontAssetId")); fontAssetID != "" {
			attrs = append(attrs, `data-platform-font-id="`+htmlEscape(fontAssetID)+`"`)
		}
		if platformFontFamily := strings.TrimSpace(textStyleMark.attrString("platformFontFamily")); platformFontFamily != "" {
			attrs = append(attrs, `data-platform-font-family="`+htmlEscape(platformFontFamily)+`"`)
		}
		if fontFamily := strings.TrimSpace(textStyleMark.attrString("fontFamily")); fontFamily != "" {
			styles = append(styles, `font-family:`+htmlEscape(fontFamily))
		}
		if fontSize := strings.TrimSpace(textStyleMark.attrString("fontSize")); fontSize != "" {
			attrs = append(attrs, `data-font-size="`+htmlEscape(fontSize)+`"`)
			styles = append(styles, `font-size:`+htmlEscape(fontSize))
		}
		if color := strings.TrimSpace(textStyleMark.attrString("color")); color != "" {
			styles = append(styles, `color:`+htmlEscape(color))
		}
	}
	if highlightMark != nil {
		color := strings.TrimSpace(highlightMark.attrString("color"))
		if color == "" {
			color = "#fef08a"
		}
		styles = append(styles, `background-color:`+htmlEscape(color))
	}

	if len(attrs) == 0 && len(styles) == 0 {
		return content
	}

	if len(styles) > 0 {
		attrs = append(attrs, `style="`+strings.Join(styles, "; ")+`"`)
	}

	tag := "span"
	if highlightMark != nil {
		tag = "mark"
	}
	return `<` + tag + ` ` + strings.Join(attrs, " ") + `>` + content + `</` + tag + `>`
}

func htmlEscape(input string) string {
	if input == "" {
		return ""
	}
	return html.EscapeString(input)
}

const maxHTMLEntityDecodeDepth = 4

func htmlUnescapeDeep(input string) string {
	if input == "" {
		return ""
	}
	current := input
	for i := 0; i < maxHTMLEntityDecodeDepth; i++ {
		next := html.UnescapeString(current)
		if next == current {
			return next
		}
		current = next
	}
	return current
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
	if names := msg.ResolveWhisperTargetDisplayNames(); len(names) > 0 {
		return names
	}
	primaryTargetID := strings.TrimSpace(msg.WhisperTo)
	if primaryTargetID == "" && msg.WhisperTarget != nil {
		primaryTargetID = strings.TrimSpace(msg.WhisperTarget.ID)
	}
	hasPrimaryRoleName := strings.TrimSpace(msg.WhisperTargetMemberName) != ""

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
	skipPrimaryFallback := func(id string) bool {
		if !hasPrimaryRoleName {
			return false
		}
		id = strings.TrimSpace(id)
		if id == "" {
			return primaryTargetID == ""
		}
		return id == primaryTargetID
	}
	if hasPrimaryRoleName {
		addName(msg.WhisperTargetMemberName)
	}
	if msg.WhisperTarget != nil {
		if !skipPrimaryFallback(msg.WhisperTarget.ID) {
			addName(resolveMemberDisplayName(channelID, msg.WhisperTarget.ID, resolver))
		}
	}
	for _, target := range msg.WhisperTargets {
		if target == nil {
			continue
		}
		if id := strings.TrimSpace(target.ID); id != "" {
			if skipPrimaryFallback(id) {
				continue
			}
			addName(resolveMemberDisplayName(channelID, id, resolver))
			continue
		}
		if skipPrimaryFallback("") {
			continue
		}
		addName(resolveUserDisplayName(target))
	}
	if strings.TrimSpace(msg.WhisperTargetUserNick) != "" && len(targets) == 0 {
		addName(msg.WhisperTargetUserNick)
	}
	for _, id := range parseWhisperIDs(msg.WhisperTo) {
		if skipPrimaryFallback(id) {
			continue
		}
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
	if model.GetDB() == nil {
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

// atTagPattern 匹配 Satori <at> 标签: <at id="xxx" name="角色名"/>
var atTagPattern = regexp.MustCompile(`<at\s+id="([^"]+)"(?:\s+name="([^"]*)")?\s*/>`)

// convertAtTagsToMention 将 <at> 标签转换为 @名字 格式（纯文本）
func convertAtTagsToMention(input string) string {
	if input == "" || !strings.Contains(input, "<at") {
		return input
	}
	return atTagPattern.ReplaceAllStringFunc(input, func(match string) string {
		submatches := atTagPattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		atID := submatches[1]
		atName := ""
		if len(submatches) >= 3 {
			atName = submatches[2]
		}
		// 优先使用 name 属性，若为空则使用 id
		displayName := strings.TrimSpace(atName)
		if displayName == "" {
			if atID == "all" {
				displayName = "全体成员"
			} else {
				displayName = atID
			}
		}
		return "@" + displayName
	})
}

// convertAtTagsToHTML 将 <at> 标签转换为带样式的 HTML span
func convertAtTagsToHTML(input string) string {
	if input == "" || !strings.Contains(input, "<at") {
		return input
	}
	return atTagPattern.ReplaceAllStringFunc(input, func(match string) string {
		submatches := atTagPattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		atID := submatches[1]
		atName := ""
		if len(submatches) >= 3 {
			atName = submatches[2]
		}
		// 优先使用 name 属性，若为空则使用 id
		displayName := strings.TrimSpace(atName)
		if displayName == "" {
			if atID == "all" {
				displayName = "全体成员"
			} else {
				displayName = atID
			}
		}
		// 生成带样式的 HTML span
		className := "mention-capsule"
		if atID == "all" {
			className += " mention-capsule--all"
		}
		return fmt.Sprintf(`<span class="%s">@%s</span>`, className, htmlEscape(displayName))
	})
}

func stripRichText(input string) string {
	if input == "" {
		return ""
	}

	// 先将 <at> 标签转换为 @名字 格式
	input = convertAtTagsToMention(input)

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
		text = htmlUnescapeDeep(text)
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
	if trimmed == "" {
		return "", false
	}

	// 提取所有 TipTap JSON 块及其周围文本
	result, found := extractAllTipTapJSON(trimmed, false)
	if !found {
		return "", false
	}
	return normalizePlainText(result), true
}

// extractAllTipTapJSON 提取并处理所有 TipTap JSON 块
// asHTML 为 true 时返回 HTML，为 false 时返回纯文本
func extractAllTipTapJSON(input string, asHTML bool) (string, bool) {
	if input == "" {
		return "", false
	}

	var result strings.Builder
	remaining := input
	foundAny := false

	for {
		// 查找下一个 TipTap JSON 块
		jsonStart := findTipTapJSONStart(remaining)
		if jsonStart < 0 {
			// 没有更多 JSON，添加剩余文本
			suffix := strings.TrimSpace(remaining)
			if suffix != "" {
				if result.Len() > 0 && !strings.HasSuffix(result.String(), "\n") {
					result.WriteString("\n")
				}
				if asHTML {
					result.WriteString(html.EscapeString(suffix))
				} else {
					result.WriteString(suffix)
				}
			}
			break
		}

		// 检查 JSON 前是否有括号（OOC 包裹）
		prefix := remaining[:jsonStart]
		hasChineseOpen := strings.HasSuffix(strings.TrimSpace(prefix), "（")
		hasEnglishOpen := !hasChineseOpen && strings.HasSuffix(strings.TrimSpace(prefix), "(")

		// 获取括号前的其他文本
		prefixText := strings.TrimSpace(prefix)
		if hasChineseOpen {
			prefixText = strings.TrimSuffix(prefixText, "（")
			prefixText = strings.TrimSpace(prefixText)
		} else if hasEnglishOpen {
			prefixText = strings.TrimSuffix(prefixText, "(")
			prefixText = strings.TrimSpace(prefixText)
		}

		// 添加前缀文本（如果有）
		if prefixText != "" {
			if result.Len() > 0 && !strings.HasSuffix(result.String(), "\n") {
				result.WriteString("\n")
			}
			if asHTML {
				result.WriteString(html.EscapeString(prefixText))
			} else {
				result.WriteString(prefixText)
			}
		}

		// 提取并解析 JSON 块
		jsonPart := remaining[jsonStart:]
		jsonEnd := findJSONEnd(jsonPart)
		if jsonEnd < 0 {
			// 无法找到 JSON 结束位置，跳过
			remaining = remaining[jsonStart+1:]
			continue
		}

		jsonBlock := jsonPart[:jsonEnd]
		var node tiptapNode
		if err := json.Unmarshal([]byte(jsonBlock), &node); err != nil {
			// 解析失败，跳过这个块
			remaining = jsonPart[jsonEnd:]
			continue
		}

		if strings.ToLower(strings.TrimSpace(node.Type)) != "doc" {
			// 不是 doc 类型，跳过
			remaining = jsonPart[jsonEnd:]
			continue
		}

		// 检查 JSON 后是否有闭括号
		afterJSON := jsonPart[jsonEnd:]
		hasChineseClose := strings.HasPrefix(strings.TrimSpace(afterJSON), "）")
		hasEnglishClose := !hasChineseClose && strings.HasPrefix(strings.TrimSpace(afterJSON), ")")

		// 决定使用哪种括号
		useChineseParens := hasChineseOpen && hasChineseClose
		useEnglishParens := hasEnglishOpen && hasEnglishClose

		// 成功解析 - 提取内容
		foundAny = true
		var content string
		if asHTML {
			var buf strings.Builder
			for _, child := range node.Content {
				renderTipTapHTML(&buf, child)
			}
			content = strings.TrimSpace(buf.String())
		} else {
			writer := newPlainTextWriter()
			writeTipTapNode(writer, &node)
			content = strings.TrimSpace(strings.TrimRight(writer.String(), "\n"))
		}

		if content != "" {
			if result.Len() > 0 && !strings.HasSuffix(result.String(), "\n") {
				result.WriteString("\n")
			}
			// 保留括号包裹
			if useChineseParens {
				result.WriteString("（")
				result.WriteString(content)
				result.WriteString("）")
			} else if useEnglishParens {
				result.WriteString("(")
				result.WriteString(content)
				result.WriteString(")")
			} else {
				result.WriteString(content)
			}
		}

		// 更新剩余内容，跳过闭括号
		remaining = afterJSON
		if useChineseParens {
			idx := strings.Index(remaining, "）")
			if idx >= 0 {
				remaining = remaining[idx+len("）"):]
			}
		} else if useEnglishParens {
			idx := strings.Index(remaining, ")")
			if idx >= 0 {
				remaining = remaining[idx+1:]
			}
		}
	}

	if !foundAny {
		return "", false
	}
	return result.String(), true
}

// findTipTapJSONStart 查找 TipTap JSON 的起始位置
func findTipTapJSONStart(s string) int {
	// 查找 {"type":"doc" 模式
	patterns := []string{
		`{"type":"doc"`,
		`{ "type":"doc"`,
		`{"type": "doc"`,
		`{ "type": "doc"`,
	}
	minIndex := -1
	for _, pattern := range patterns {
		if idx := strings.Index(s, pattern); idx >= 0 {
			if minIndex < 0 || idx < minIndex {
				minIndex = idx
			}
		}
	}
	return minIndex
}

// findJSONEnd 找到 JSON 对象的结束位置（匹配的 }）
func findJSONEnd(s string) int {
	if len(s) == 0 || s[0] != '{' {
		return -1
	}
	depth := 0
	inString := false
	escaped := false
	for i, ch := range s {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}

// trimOOCParentheses 去除 OOC 包裹的括号
func trimOOCParentheses(s string) string {
	s = strings.TrimSpace(s)
	// 去除中文括号
	for strings.HasPrefix(s, "（") && strings.HasSuffix(s, "）") {
		s = strings.TrimPrefix(s, "（")
		s = strings.TrimSuffix(s, "）")
		s = strings.TrimSpace(s)
	}
	// 去除英文括号
	for strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		s = strings.TrimPrefix(s, "(")
		s = strings.TrimSuffix(s, ")")
		s = strings.TrimSpace(s)
	}
	return s
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

func camelToDataAttr(key string) string {
	if key == "" {
		return "data-"
	}
	var builder strings.Builder
	builder.WriteString("data-")
	for _, r := range key {
		if unicode.IsUpper(r) {
			builder.WriteByte('-')
			builder.WriteRune(unicode.ToLower(r))
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
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
		text := node.Text
		for _, mark := range node.Marks {
			if mark == nil || !strings.EqualFold(mark.Type, "ruby") {
				continue
			}
			rubyText := strings.TrimSpace(mark.attrString("rubyText"))
			if rubyText != "" {
				text = text + "（" + rubyText + "）"
			}
			break
		}
		w.write(text)
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

type quickFormatFlavor int

const (
	quickFormatFlavorHTML quickFormatFlavor = iota
	quickFormatFlavorBBCode
)

type quickToken struct {
	token string
	html  string
	bb    string
}

var (
	quickCodeFenceLiteralPattern = regexp.MustCompile("```([\\s\\S]*?)```")
	quickInlineCodePattern       = regexp.MustCompile("`([^`\\n]+)`")
	quickLinkPattern             = regexp.MustCompile(`\[([^\]\\n]+)\]\((https?://[^\s)]+)\)`)
	quickBoldPattern             = regexp.MustCompile(`\*\*([^\n*][^*\n]*?)\*\*`)
	quickItalicPattern           = regexp.MustCompile(`(^|[^*])\*([^*\n]+)\*`)
	htmlTagPattern               = regexp.MustCompile(`(?is)<[a-zA-Z][^>]*>`)
	inlineCodeHTMLTagPattern     = regexp.MustCompile(`(?is)<code\b[^>]*>(.*?)</code>`)
	bbcodePrePattern             = regexp.MustCompile(`(?is)<pre\b[^>]*>\s*<code\b[^>]*>(.*?)</code>\s*</pre>`)
	bbcodeInlineCodePattern      = regexp.MustCompile(`(?is)<code\b[^>]*>(.*?)</code>`)
	bbcodeLinkPattern            = regexp.MustCompile(`(?is)<a\b[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	bbcodeStrongPattern          = regexp.MustCompile(`(?is)<\/?strong\b[^>]*>`)
	bbcodeBoldPattern            = regexp.MustCompile(`(?is)<\/?b\b[^>]*>`)
	bbcodeEmPattern              = regexp.MustCompile(`(?is)<\/?em\b[^>]*>`)
	bbcodeItalicPattern          = regexp.MustCompile(`(?is)<\/?i\b[^>]*>`)
	bbcodeBrPattern              = regexp.MustCompile(`(?is)<br\s*/?>`)
	bbcodePBoundaryPattern       = regexp.MustCompile(`(?is)</p>\s*<p\b[^>]*>`)
	bbcodeAnyTagPattern          = regexp.MustCompile(`(?is)</?[^>]+>`)
)

func enhancePlainContentForHTMLExport(content string) string {
	if content == "" {
		return ""
	}
	options := quickFormatRenderOptions{
		DisableInlineCode: shouldDisableInlineCodeForBotCommand(content),
		DisableAll:        shouldDisableInlineCodeForBotCommand(content),
	}
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	if normalized == "" {
		return ""
	}
	if isLikelyHTMLContent(normalized) {
		if options.DisableAll {
			return renderBotCommandRawHTML(content)
		}
		return normalized
	}
	normalized = htmlUnescapeDeep(normalized)

	protected, tokens := protectAtTagsForQuickFormat(normalized)
	converted := convertQuickFormatForFlavor(protected, quickFormatFlavorHTML, options)
	for _, token := range tokens {
		converted = strings.ReplaceAll(converted, token.token, token.html)
	}
	return converted
}

func isLikelyHTMLContent(input string) bool {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "<at") {
		return true
	}
	if strings.Contains(trimmed, "<img") || strings.Contains(trimmed, "<br") {
		return true
	}
	return htmlTagPattern.MatchString(trimmed)
}

func protectAtTagsForQuickFormat(input string) (string, []quickToken) {
	if input == "" || !strings.Contains(input, "<at") {
		return input, nil
	}
	tokens := make([]quickToken, 0, 4)
	index := 0
	result := atTagPattern.ReplaceAllStringFunc(input, func(match string) string {
		token := fmt.Sprintf("__QF_AT_%d__", index)
		index++
		tokens = append(tokens, quickToken{token: token, html: match, bb: convertAtTagsToMention(match)})
		return token
	})
	return result, tokens
}

func convertQuickFormatForFlavor(input string, flavor quickFormatFlavor, options quickFormatRenderOptions) string {
	if input == "" {
		return ""
	}
	if options.DisableAll {
		if flavor == quickFormatFlavorHTML {
			return strings.ReplaceAll(input, "\n", "<br />")
		}
		return input
	}

	fenceTokens := make([]quickToken, 0, 4)
	fenceIndex := 0
	protected := quickCodeFenceLiteralPattern.ReplaceAllStringFunc(input, func(segment string) string {
		token := fmt.Sprintf("__QF_FENCE_%d__", fenceIndex)
		fenceIndex++
		fenceTokens = append(fenceTokens, quickToken{token: token, html: segment, bb: segment})
		return token
	})

	inlined := convertQuickInline(protected, flavor, options)
	for _, token := range fenceTokens {
		replacement := token.html
		if flavor == quickFormatFlavorBBCode {
			replacement = token.bb
		}
		inlined = strings.ReplaceAll(inlined, token.token, replacement)
	}

	if flavor == quickFormatFlavorHTML {
		inlined = strings.ReplaceAll(inlined, "\n", "<br />")
	}

	return inlined
}

func convertQuickInline(input string, flavor quickFormatFlavor, options quickFormatRenderOptions) string {
	if input == "" {
		return ""
	}

	escaped := htmlEscape(input)

	inlineCodes := make([]quickToken, 0, 4)
	if !options.DisableInlineCode {
		codeIndex := 0
		escaped = quickInlineCodePattern.ReplaceAllStringFunc(escaped, func(segment string) string {
			match := quickInlineCodePattern.FindStringSubmatch(segment)
			if len(match) < 2 {
				return segment
			}
			token := fmt.Sprintf("__QF_INLINE_CODE_%d__", codeIndex)
			codeIndex++
			entry := quickToken{token: token}
			switch flavor {
			case quickFormatFlavorBBCode:
				entry.bb = "[code]" + match[1] + "[/code]"
			default:
				entry.html = "<code>" + match[1] + "</code>"
			}
			inlineCodes = append(inlineCodes, entry)
			return token
		})
	}

	links := make([]quickToken, 0, 4)
	linkIndex := 0
	escaped = quickLinkPattern.ReplaceAllStringFunc(escaped, func(segment string) string {
		match := quickLinkPattern.FindStringSubmatch(segment)
		if len(match) < 3 {
			return segment
		}
		label := match[1]
		url := htmlUnescapeDeep(strings.TrimSpace(match[2]))
		if !isSafeQuickLink(url) {
			return segment
		}
		token := fmt.Sprintf("__QF_LINK_%d__", linkIndex)
		linkIndex++
		entry := quickToken{token: token}
		switch flavor {
		case quickFormatFlavorBBCode:
			entry.bb = "[url=" + url + "]" + label + "[/url]"
		default:
			entry.html = "<a href=\"" + htmlEscape(url) + "\" class=\"text-blue-500\" target=\"_blank\" rel=\"noopener noreferrer\">" + label + "</a>"
		}
		links = append(links, entry)
		return token
	})

	escaped = quickBoldPattern.ReplaceAllString(escaped, "<strong>$1</strong>")
	escaped = quickItalicPattern.ReplaceAllString(escaped, "$1<em>$2</em>")

	for _, token := range links {
		replacement := token.html
		if flavor == quickFormatFlavorBBCode {
			replacement = token.bb
		}
		escaped = strings.ReplaceAll(escaped, token.token, replacement)
	}

	for _, token := range inlineCodes {
		replacement := token.html
		if flavor == quickFormatFlavorBBCode {
			replacement = token.bb
		}
		escaped = strings.ReplaceAll(escaped, token.token, replacement)
	}

	if flavor == quickFormatFlavorBBCode {
		escaped = strings.ReplaceAll(escaped, "<strong>", "[b]")
		escaped = strings.ReplaceAll(escaped, "</strong>", "[/b]")
		escaped = strings.ReplaceAll(escaped, "<em>", "[i]")
		escaped = strings.ReplaceAll(escaped, "</em>", "[/i]")
	}

	return escaped
}

func isSafeQuickLink(raw string) bool {
	value := strings.TrimSpace(raw)
	if value == "" {
		return false
	}
	parsed, err := neturl.Parse(value)
	if err != nil || parsed == nil {
		return false
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	return scheme == "http" || scheme == "https"
}

func convertRenderedHTMLToBBCode(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}

	text := convertAtTagsToMention(input)
	codeBlocks := make([]quickToken, 0, 4)
	codeIndex := 0

	text = bbcodePrePattern.ReplaceAllStringFunc(text, func(segment string) string {
		match := bbcodePrePattern.FindStringSubmatch(segment)
		if len(match) < 2 {
			return segment
		}
		body := htmlUnescapeDeep(stripRichText(match[1]))
		token := fmt.Sprintf("__QF_BB_BLOCK_%d__", codeIndex)
		codeIndex++
		codeBlocks = append(codeBlocks, quickToken{token: token, bb: "[code]" + body + "[/code]"})
		return token
	})

	text = bbcodeInlineCodePattern.ReplaceAllStringFunc(text, func(segment string) string {
		match := bbcodeInlineCodePattern.FindStringSubmatch(segment)
		if len(match) < 2 {
			return segment
		}
		body := htmlUnescapeDeep(stripRichText(match[1]))
		return "[code]" + body + "[/code]"
	})

	text = bbcodeLinkPattern.ReplaceAllStringFunc(text, func(segment string) string {
		match := bbcodeLinkPattern.FindStringSubmatch(segment)
		if len(match) < 3 {
			return segment
		}
		href := htmlUnescapeDeep(strings.TrimSpace(match[1]))
		label := htmlUnescapeDeep(stripRichText(match[2]))
		if !isSafeQuickLink(href) {
			return label
		}
		return "[url=" + href + "]" + label + "[/url]"
	})

	text = bbcodeStrongPattern.ReplaceAllStringFunc(text, func(segment string) string {
		if strings.HasPrefix(strings.ToLower(segment), "</") {
			return "[/b]"
		}
		return "[b]"
	})
	text = bbcodeBoldPattern.ReplaceAllStringFunc(text, func(segment string) string {
		if strings.HasPrefix(strings.ToLower(segment), "</") {
			return "[/b]"
		}
		return "[b]"
	})
	text = bbcodeEmPattern.ReplaceAllStringFunc(text, func(segment string) string {
		if strings.HasPrefix(strings.ToLower(segment), "</") {
			return "[/i]"
		}
		return "[i]"
	})
	text = bbcodeItalicPattern.ReplaceAllStringFunc(text, func(segment string) string {
		if strings.HasPrefix(strings.ToLower(segment), "</") {
			return "[/i]"
		}
		return "[i]"
	})

	text = bbcodePBoundaryPattern.ReplaceAllString(text, "\n")
	text = bbcodeBrPattern.ReplaceAllString(text, "\n")
	text = bbcodeAnyTagPattern.ReplaceAllString(text, "")
	text = htmlUnescapeDeep(text)

	for _, token := range codeBlocks {
		text = strings.ReplaceAll(text, token.token, token.bb)
	}

	return normalizePlainText(text)
}

func buildBBCodeBody(msg *ExportMessage, includeImages bool) string {
	if msg == nil {
		return ""
	}
	raw := strings.TrimSpace(msg.Content)
	if raw == "" {
		return ""
	}
	options := quickFormatRenderOptions{
		DisableInlineCode: shouldDisableInlineCodeForBotCommand(raw),
		DisableAll:        shouldDisableInlineCodeForBotCommand(raw),
	}
	if options.DisableAll {
		body := resolveBotCommandRawText(raw)
		body = applyImageVisibilityToPlain(body, includeImages)
		body = wrapOOCContent(msg.IcMode, body)
		parts := make([]string, 0, 3)
		if msg.IsArchived {
			parts = append(parts, "[已归档]")
		}
		if msg.IsWhisper {
			if label := formatWhisperTargets(msg.WhisperTargets); label != "" {
				parts = append(parts, label)
			}
		}
		if body != "" {
			parts = append(parts, body)
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	}

	var body string
	if htmlValue, ok := convertTipTapToHTML(raw); ok {
		htmlValue = convertAtTagsToHTML(htmlValue)
		if options.DisableInlineCode {
			htmlValue = stripInlineCodeTagsFromHTML(htmlValue)
		}
		body = convertRenderedHTMLToBBCode(htmlValue)
	} else if isLikelyHTMLContent(raw) {
		if options.DisableInlineCode {
			raw = stripInlineCodeTagsFromHTML(raw)
		}
		body = convertRenderedHTMLToBBCode(raw)
	} else {
		protected, tokens := protectAtTagsForQuickFormat(raw)
		body = convertQuickFormatForFlavor(protected, quickFormatFlavorBBCode, options)
		for _, token := range tokens {
			body = strings.ReplaceAll(body, token.token, token.bb)
		}
		body = htmlUnescapeDeep(body)
		body = normalizePlainText(body)
	}

	body = applyImageVisibilityToPlain(body, includeImages)
	body = wrapOOCContent(msg.IcMode, body)
	parts := make([]string, 0, 3)
	if msg.IsArchived {
		parts = append(parts, "[已归档]")
	}
	if msg.IsWhisper {
		if label := formatWhisperTargets(msg.WhisperTargets); label != "" {
			parts = append(parts, label)
		}
	}
	if body != "" {
		parts = append(parts, body)
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

var (
	attachmentTokenPattern    = regexp.MustCompile(`^[0-9A-Za-z_-]+$`)
	cqImageTokenPattern       = regexp.MustCompile(`(?i)\[CQ:image,[^\]]*]`)
	htmlImageTagPattern       = regexp.MustCompile(`(?is)<img\b[^>]*>`)
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
		body := buildContentBody(msg, payload.IncludeImages)
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
	content := buildContentBody(msg, payload.IncludeImages)
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
	senderName := resolveBBCodeSenderName(payload, msg)
	var headerParts []string
	if !payload.WithoutTimestamp {
		headerParts = append(headerParts, fmt.Sprintf("[%s]", msg.CreatedAt.Format("2006-01-02 15:04:05")))
	}
	headerParts = append(headerParts, fmt.Sprintf("<%s>", senderName))
	header := strings.Join(headerParts, " ")
	content := buildBBCodeBody(msg, payload.IncludeImages)
	color := resolveBBCodeSenderColor(payload, msg)
	return fmt.Sprintf("[color=silver]%s[/color][color=%s] %s [/color]", header, color, content)
}

func resolveBBCodeSenderName(payload *ExportPayload, msg *ExportMessage) string {
	if override := lookupBBCodeNameOverride(payload, msg); override != "" {
		return override
	}
	return strings.TrimSpace(msg.SenderName)
}

func resolveBBCodeSenderColor(payload *ExportPayload, msg *ExportMessage) string {
	if override := lookupBBCodeColorOverride(payload, msg); override != "" {
		if normalized := sanitizeBBCodeColor(override, ""); normalized != "" {
			return normalized
		}
	}
	return sanitizeBBCodeColor(msg.SenderColor, "#111111")
}

func lookupBBCodeColorOverride(payload *ExportPayload, msg *ExportMessage) string {
	if payload == nil || msg == nil || payload.ExtraMeta == nil {
		return ""
	}
	identityID := strings.TrimSpace(msg.SenderIdentityID)
	if identityID == "" {
		return ""
	}
	rawMap, ok := payload.ExtraMeta["text_colorize_bbcode_map"]
	if !ok {
		return ""
	}
	key := "identity:" + identityID
	switch m := rawMap.(type) {
	case map[string]string:
		return strings.TrimSpace(m[key])
	case map[string]interface{}:
		if raw, exists := m[key]; exists {
			if value, ok := raw.(string); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func lookupBBCodeNameOverride(payload *ExportPayload, msg *ExportMessage) string {
	if payload == nil || msg == nil || payload.ExtraMeta == nil {
		return ""
	}
	identityID := strings.TrimSpace(msg.SenderIdentityID)
	if identityID == "" {
		return ""
	}
	rawMap, ok := payload.ExtraMeta["text_colorize_bbcode_name_map"]
	if !ok {
		return ""
	}
	key := "identity:" + identityID
	switch m := rawMap.(type) {
	case map[string]string:
		return strings.TrimSpace(m[key])
	case map[string]interface{}:
		if raw, exists := m[key]; exists {
			if value, ok := raw.(string); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
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

func formatWhisperMetaText(targets []string) string {
	if len(targets) == 0 {
		return ""
	}
	return "发送给 " + strings.Join(targets, "、")
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

func buildContentBody(msg *ExportMessage, includeImages bool) string {
	if msg == nil {
		return ""
	}
	clean := buildFilteredPlainContent(msg.Content, includeImages)
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
	if clean != "" {
		parts = append(parts, clean)
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func buildFilteredPlainContent(raw string, includeImages bool) string {
	clean := stripRichText(raw)
	return applyImageVisibilityToPlain(clean, includeImages)
}

func applyImageVisibilityToPlain(plain string, includeImages bool) string {
	if includeImages {
		return normalizePlainText(plain)
	}
	withoutImages := cqImageTokenPattern.ReplaceAllString(plain, "")
	return normalizePlainText(withoutImages)
}

func stripImageTagsFromHTML(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}
	return strings.TrimSpace(htmlImageTagPattern.ReplaceAllString(input, ""))
}

func stripInlineCodeTagsFromHTML(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}
	return inlineCodeHTMLTagPattern.ReplaceAllString(input, "`$1`")
}

func resolveBotCommandRawText(content string) string {
	if serialized, ok := SerializeMessageContentToCommandText(content); ok {
		return serialized
	}
	return normalizePlainText(content)
}

func renderBotCommandRawHTML(content string) string {
	raw := htmlEscape(resolveBotCommandRawText(content))
	return strings.ReplaceAll(raw, "\n", "<br />")
}

func shouldDisableInlineCodeForBotCommand(content string) bool {
	leading := strings.TrimLeft(content, " \t\r\n")
	if leading == "" {
		return false
	}
	if serialized, ok := SerializeMessageContentToCommandText(content); ok {
		leading = serialized
	}
	return hasLeadingBotCommandPrefix(leading, utils.GetConfiguredBotCommandPrefixes())
}

func hasLeadingBotCommandPrefix(content string, prefixes []string) bool {
	leading := strings.TrimLeft(content, " \t\r\n")
	if leading == "" {
		return false
	}
	for _, prefix := range prefixes {
		trimmed := strings.TrimSpace(prefix)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(leading, trimmed) {
			return true
		}
	}
	return false
}

func isSingleLineDiceCommand(raw string) bool {
	line := normalizePlainText(raw)
	if line == "" || strings.Contains(line, "\n") {
		return false
	}
	line = strings.TrimSpace(trimOOCParentheses(line))
	if line == "" || strings.Contains(line, "\n") {
		return false
	}
	return isSingleLineDiceCommandWithPrefixes(line, resolveDiceCommandPrefixes())
}

func isSingleLineDiceCommandWithPrefixes(line string, prefixes []string) bool {
	line = strings.TrimSpace(line)
	if line == "" || strings.Contains(line, "\n") {
		return false
	}
	tokens := strings.Fields(line)
	for _, token := range tokens {
		for _, prefix := range prefixes {
			if prefix == "" {
				continue
			}
			searchFrom := 0
			for searchFrom < len(token) {
				idx := strings.Index(token[searchFrom:], prefix)
				if idx < 0 {
					break
				}
				idx += searchFrom
				// 支持两种形态：
				// 1) token 以指令前缀开头，如 ".ra"
				// 2) token 以 @ 开头且内部包含指令前缀，如 "@用户。r"
				if idx != 0 && token[0] != '@' {
					searchFrom = idx + len(prefix)
					continue
				}
				rest := strings.TrimSpace(token[idx+len(prefix):])
				if rest == "" {
					searchFrom = idx + len(prefix)
					continue
				}
				first, _ := utf8.DecodeRuneInString(rest)
				if first != utf8.RuneError && unicode.IsLetter(first) {
					return true
				}
				searchFrom = idx + len(prefix)
			}
		}
	}
	return false
}

func resolveDiceCommandPrefixes() []string {
	if cfg := utils.GetConfig(); cfg != nil && len(cfg.Export.DiceCommandPrefixes) > 0 {
		prefixes := make([]string, 0, len(cfg.Export.DiceCommandPrefixes))
		for _, item := range cfg.Export.DiceCommandPrefixes {
			trimmed := strings.TrimSpace(item)
			if trimmed != "" {
				prefixes = append(prefixes, trimmed)
			}
		}
		if len(prefixes) > 0 {
			return prefixes
		}
	}
	return []string{".", "。"}
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
	"formatWhisperMeta": func(targets []string) string {
		return formatWhisperMetaText(targets)
	},
	"safeHTML": func(s string) htmltemplate.HTML {
		return htmltemplate.HTML(s)
	},
	"safeCSS": func(s string) htmltemplate.CSS {
		return htmltemplate.CSS(s)
	},
}).Parse(`<!DOCTYPE html>
<html lang="zh">
<head>
  <meta charset="UTF-8">
  <title>频道导出 - {{.ChannelName}}</title>
  <style>
    {{if .EmbeddedFontCSS}}{{safeCSS .EmbeddedFontCSS}}{{end}}
    body { font-family: -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Hiragino Sans GB",sans-serif; margin: 2rem; background: #f7f7f7; }
    .meta { margin-bottom: 1.5rem; color: #555; }
    .message { padding: 12px 16px; margin-bottom: 8px; background: #fff; border-radius: 6px; box-shadow: 0 1px 2px rgba(0,0,0,0.05); }
    .sender { font-weight: 600; color: #222; margin-right: 4px; }
    .whisper-meta { margin-left: 4px; color: #666; font-size: 0.82rem; }
    .timestamp { color: #888; font-size: 0.9rem; }
    .timestamp.hidden { visibility: hidden; height: 0; margin: 0; }
    .ooc { border-left: 3px solid #eab308; }
    .whisper { border-left: 3px solid #6366f1; }
    .content { margin-top: 4px; white-space: pre-wrap; line-height: 1.5; }
    .content p { margin: 0.5em 0; }
    .content ul, .content ol { margin: 0.5em 0; padding-left: 1.5em; }
    .content blockquote { margin: 0.5em 0; padding-left: 1em; border-left: 3px solid #ddd; color: #666; }
    .content pre { background: #f4f4f4; padding: 0.5em; border-radius: 4px; overflow-x: auto; }
    .content code { background: #f4f4f4; padding: 0.1em 0.3em; border-radius: 3px; font-family: monospace; }
    .content strong { font-weight: 600; }
    .content em { font-style: italic; }
    .content u { text-decoration: underline; }
    .content s { text-decoration: line-through; }
    .content mark { background-color: #fef08a; }
    .content a { color: #3b82f6; text-decoration: underline; }
    .content img { max-width: 100%; height: auto; border-radius: 4px; }
    .content .tiptap-ruby { ruby-align: center; ruby-position: over; font-family: var(--ruby-font-family, inherit); color: var(--ruby-color, inherit); font-weight: var(--ruby-font-weight, inherit); font-style: var(--ruby-font-style, inherit); }
    .content .tiptap-ruby rt { font-family: var(--ruby-font-family, inherit); color: var(--ruby-color, inherit); font-weight: var(--ruby-font-weight, inherit); font-style: var(--ruby-font-style, inherit); font-size: calc(var(--ruby-font-size, 1em) * 0.72); line-height: 1.05; letter-spacing: 0; }
    .mention-capsule { display: inline; background-color: rgba(59, 130, 246, 0.1); color: #3b82f6; padding: 0 0.35em; border-radius: 4px; font-weight: 500; }
    .mention-capsule--all { background-color: rgba(239, 68, 68, 0.1); color: #ef4444; }
    .export-sticky-note { margin: 0.6em 0; padding: 0.75em 0.9em; border: 1px solid rgba(15,23,42,0.12); border-left: 4px solid var(--export-sticky-note-accent,#64748b); border-radius: 6px; background: color-mix(in srgb, var(--export-sticky-note-accent,#64748b) 9%, #fff); white-space: normal; }
    .export-sticky-note__header { display: flex; align-items: baseline; justify-content: space-between; gap: 0.75em; margin-bottom: 0.4em; }
    .export-sticky-note__title { font-weight: 700; color: #111827; }
    .export-sticky-note__type { font-size: 0.78em; color: #64748b; white-space: nowrap; }
    .export-sticky-note__body { color: #1f2937; }
    .export-sticky-note__body > :first-child { margin-top: 0; }
    .export-sticky-note__body > :last-child { margin-bottom: 0; }
    .export-sticky-note__list { margin: 0; padding-left: 1.35em; }
    .export-sticky-note__list-item--checked { color: #64748b; text-decoration: line-through; }
    .export-sticky-note__empty { color: #64748b; }
    .export-sticky-note-counter { display: flex; align-items: center; justify-content: center; gap: 0.55em; padding: 0.35em 0; }
    .export-sticky-note-counter__button { width: 2em; height: 2em; display: inline-flex; align-items: center; justify-content: center; border-radius: 999px; background: rgba(15,23,42,0.09); color: #334155; }
    .export-sticky-note-counter__value { min-width: 4.8em; padding: 0.35em 0.65em; text-align: center; border: 1px solid rgba(15,23,42,0.18); border-radius: 6px; font-weight: 700; font-size: 1.2em; background: rgba(255,255,255,0.62); }
    .export-sticky-note-slider { padding: 0.3em 0.15em; }
    .export-sticky-note-slider__value { text-align: center; font-weight: 700; margin-bottom: 0.45em; }
    .export-sticky-note-slider__track { height: 0.55em; overflow: hidden; border-radius: 999px; background: rgba(15,23,42,0.12); }
    .export-sticky-note-slider__fill { height: 100%; border-radius: inherit; background: var(--export-sticky-note-accent,#64748b); }
    .export-sticky-note-slider__range { display: flex; justify-content: space-between; margin-top: 0.35em; font-size: 0.82em; color: #64748b; }
    .export-sticky-note-timer { display: grid; justify-items: center; gap: 0.45em; padding: 0.25em 0; }
    .export-sticky-note-timer__display { font-family: ui-monospace,SFMono-Regular,Menlo,Consolas,monospace; font-size: 1.7em; font-weight: 700; letter-spacing: 0.08em; }
    .export-sticky-note-timer__meta { display: flex; flex-wrap: wrap; justify-content: center; gap: 0.45em; font-size: 0.82em; color: #64748b; }
    .export-sticky-note-clock { display: flex; justify-content: center; padding: 0.25em 0; }
    .export-sticky-note-clock__dial { width: 7em; height: 7em; border-radius: 50%; display: grid; place-items: center; background: conic-gradient(var(--export-sticky-note-accent,#64748b) 0deg var(--export-sticky-note-clock-filled), rgba(15,23,42,0.1) var(--export-sticky-note-clock-filled) 360deg); box-shadow: inset 0 0 0 1px rgba(15,23,42,0.16); }
    .export-sticky-note-clock__center { width: 2.4em; height: 2.4em; display: grid; place-items: center; border-radius: 50%; background: rgba(255,255,255,0.88); border: 1px solid rgba(15,23,42,0.18); font-weight: 700; font-size: 0.9em; }
    .export-sticky-note-round { display: grid; justify-items: center; gap: 0.35em; padding: 0.25em 0; }
    .export-sticky-note-round__label, .export-sticky-note-round__direction { font-size: 0.82em; color: #64748b; }
    .export-sticky-note-round__value { font-size: 2em; line-height: 1; font-weight: 800; }
    .export-sticky-note-list { list-style: none; margin: 0; padding: 0; }
    .export-sticky-note-list__item { display: flex; align-items: center; gap: 0.45em; padding: 0.18em 0; }
    .export-sticky-note-list__checkbox { width: 1em; height: 1em; flex: none; display: inline-grid; place-items: center; border-radius: 3px; border: 1px solid rgba(15,23,42,0.28); font-size: 0.78em; line-height: 1; }
    .export-sticky-note-list__item--checked .export-sticky-note-list__text { color: #64748b; text-decoration: line-through; }
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
      <div class="content"><span class="sender">&lt;{{.SenderName}}&gt;</span>{{if and .IsWhisper .WhisperTargets}}<span class="whisper-meta">{{formatWhisperMeta .WhisperTargets}}</span>{{end}}{{if .ContentHTML}}{{safeHTML .ContentHTML}}{{else}}{{.Content}}{{end}}</div>
    </article>
  {{end}}
</body>
</html>`))

func (htmlFormatter) Build(payload *ExportPayload) ([]byte, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload 为空")
	}
	buf := &bytes.Buffer{}
	view := struct {
		*ExportPayload
		EmbeddedFontCSS string
	}{
		ExportPayload:   payload,
		EmbeddedFontCSS: buildEmbeddedPlatformFontCSS(payload),
	}
	if err := exportHTMLTemplate.Execute(buf, view); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
