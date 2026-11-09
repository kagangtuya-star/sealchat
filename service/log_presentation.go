package service

import (
	"encoding/json"
	"net/url"
	"strings"

	"sealchat/model"
	"sealchat/protocol"
)

// LOG reads messages through the shared display semantics. The clue itself keeps
// the public audience boundary, while the archival projection may include
// member-private clue sections for an authorized viewer. The raw message.content
// stays the only editable source; nothing here is written back.

// LogMessagePresentation is the read-only projection a design window adds to a raw message.
type LogMessagePresentation struct {
	// Embed replaces the body when the whole message is internal embed links.
	Embed   *LogMessageEmbed   `json:"embed,omitempty"`
	Whisper *LogMessageWhisper `json:"whisper,omitempty"`
	Quote   *LogMessageQuote   `json:"quote,omitempty"`
	// A present empty token explicitly suppresses an unsafe/unavailable avatar.
	AvatarToken *string `json:"avatarToken,omitempty"`
}

type LogMessageEmbed struct {
	Kind      string `json:"kind"`
	BodyHTML  string `json:"bodyHtml"`
	PlainText string `json:"plainText"`
	// Clues lists the expanded world clues in body order (worldClue embeds only).
	Clues []LogWorldClue `json:"clues,omitempty"`
}

// LogWorldClue is the stable bindable display semantics of one expanded world
// clue. Member-private sections are rendered inside bodyHtml instead of being
// flattened into template fields; manager notes, access, folders, read state and
// the clue box's own presentation settings never reach LOG.
type LogWorldClue struct {
	Title string `json:"title"`
	// Kind is text, image or iframe (web clue).
	Kind      string `json:"kind"`
	KindLabel string `json:"kindLabel"`
	// Source is a web clue's http(s) address, for display only.
	Source string `json:"source,omitempty"`
}

// LogMessageWhisper is orthogonal to the message type: an IC, OOC or dice message may be a whisper.
type LogMessageWhisper struct {
	Targets []MessageWhisperTarget `json:"targets"`
}

type LogMessageQuote struct {
	SpeakerName string `json:"speakerName"`
	Text        string `json:"text"`
}

func (p *LogMessagePresentation) empty() bool {
	return p.Embed == nil && p.Whisper == nil && p.Quote == nil && p.AvatarToken == nil
}

// logMessageBody is a message's LOG body before its image references are resolved.
type logMessageBody struct {
	Doc       AgentRichDocument
	EmbedKind string
	Assets    map[string]LogBodyAssetRef
	// Clues keeps each expanded world clue as display parts so styles can target
	// its title, kind, media, body, source and archival private sections; Doc stays empty for them.
	Clues []worldClueDisplay
}

func logMessageBodyOf(display *messageDisplayResolver, m *model.MessageModel) logMessageBody {
	body := logMessageBody{}
	if m.IsDeleted || m.IsRevoked {
		return body
	}
	if expanded, ok := display.forChannel(m.ChannelID).expandEmbed(m.Content, true, true); ok {
		body.EmbedKind = expanded.Kind
		switch expanded.Kind {
		case messageEmbedWorldClue:
			for _, block := range expanded.WorldClues {
				source := LogBodyAssetRef{Kind: logBodyWorldClue, WorldID: block.WorldID, ClueID: block.ClueID}
				clue := worldClueDisplayOf(block, true)
				clue.Media = body.markDocument(m, clue.Media, source)
				clue.Body = body.markDocument(m, clue.Body, source)
				for _, section := range block.Private {
					if !richDocumentHasContent(&section.Content) {
						continue
					}
					section.Content = body.markDocument(m, section.Content, source)
					clue.Private = append(clue.Private, section)
				}
				body.Clues = append(body.Clues, clue)
			}
		case messageEmbedStickyNote:
			for _, part := range expanded.StickyNotes {
				body.appendDocument(m, part.Doc, LogBodyAssetRef{Kind: logBodyStickyNote, NoteID: part.NoteID})
			}
		}
		return body
	}
	body.appendDocument(m, buildLogRichDocument(m.Content, true), LogBodyAssetRef{Kind: logBodyMessage})
	return body
}

// html renders the body; an expanded embed gets a wrapper that styles can target.
func (b logMessageBody) html() string {
	switch b.EmbedKind {
	case messageEmbedStickyNote:
		return `<div class="log-embed log-embed--sticky-note">` + renderLogRichDocumentHTML(b.Doc, true) + `</div>`
	case messageEmbedWorldClue:
		var out strings.Builder
		out.WriteString(`<div class="log-embed log-embed--world-clue">`)
		for _, clue := range b.Clues {
			out.WriteString(logWorldClueHTML(clue))
		}
		out.WriteString(`</div>`)
		return out.String()
	}
	return renderLogRichDocumentHTML(b.Doc, true)
}

// document is the flattened body; clues keep the export's text form.
func (b logMessageBody) document() AgentRichDocument {
	if len(b.Clues) == 0 {
		return b.Doc
	}
	doc := AgentRichDocument{Type: "document"}
	for _, clue := range b.Clues {
		part := clue.richDocument()
		doc.Blocks = append(doc.Blocks, part.Blocks...)
		for _, section := range clue.Private {
			if !richDocumentHasContent(&section.Content) {
				continue
			}
			name := strings.TrimSpace(section.MemberName)
			if name == "" {
				name = "成员"
			}
			doc.Blocks = append(doc.Blocks, AgentRichNode{Type: "paragraph", Children: []AgentRichNode{{Type: "text", Text: "[专属信息: " + name + "]", Marks: []AgentRichMark{{Type: "bold"}}}}})
			doc.Blocks = append(doc.Blocks, section.Content.Blocks...)
		}
	}
	return doc
}

func (b logMessageBody) plainText() string {
	return AgentRichDocumentPlainText(b.document())
}

// worldClues is the clue metadata a style may bind; nil for other bodies.
func (b logMessageBody) worldClues() []LogWorldClue {
	if b.EmbedKind != messageEmbedWorldClue || len(b.Clues) == 0 {
		return nil
	}
	out := make([]LogWorldClue, 0, len(b.Clues))
	for _, clue := range b.Clues {
		kind := logWorldClueKind(clue.Kind)
		out = append(out, LogWorldClue{Title: clue.Title, Kind: kind, KindLabel: worldClueExportKindLabel(kind), Source: clue.SourceURL})
	}
	return out
}

// logWorldClueKind keeps the class/attribute vocabulary closed to the known clue kinds.
func logWorldClueKind(kind string) string {
	switch kind = strings.ToLower(strings.TrimSpace(kind)); kind {
	case model.WorldClueKindImage, model.WorldClueKindIframe:
		return kind
	}
	return model.WorldClueKindText
}

// logWorldClueHTML renders one clue with stable classes inside the body sanitizer's
// whitelist (div/span/strong/p/a/img + class): .log-clue--<kind>, __head, __label,
// __title, __kind, __media, __body, __source and __private*. A web clue's source
// is a plain link that the LOG frame never navigates or embeds.
func logWorldClueHTML(clue worldClueDisplay) string {
	kind := logWorldClueKind(clue.Kind)
	var out strings.Builder
	out.WriteString(`<div class="log-clue log-clue--` + kind + `">`)
	out.WriteString(`<div class="log-clue__head"><span class="log-clue__label">线索</span><strong class="log-clue__title">` + htmlEscape(clue.Title) + `</strong><span class="log-clue__kind">` + htmlEscape(worldClueExportKindLabel(kind)) + `</span></div>`)
	if media := RenderAgentRichDocumentHTML(clue.Media, true); strings.TrimSpace(media) != "" {
		class := "log-clue__media"
		if !logDocumentHasImage(clue.Media.Blocks) {
			class += " log-clue__media--missing"
		}
		out.WriteString(`<div class="` + class + `">` + media + `</div>`)
	}
	if clue.Empty {
		out.WriteString(`<div class="log-clue__body log-clue__body--empty"><p>（空线索）</p></div>`)
	} else if body := RenderAgentRichDocumentHTML(clue.Body, true); strings.TrimSpace(body) != "" {
		out.WriteString(`<div class="log-clue__body">` + body + `</div>`)
	}
	source := func(modifier, label, href string) {
		if href == "" {
			return
		}
		out.WriteString(`<div class="log-clue__source` + modifier + `"><span class="log-clue__source-label">` + label + `</span><a href="` + htmlEscape(href) + `">` + htmlEscape(href) + `</a></div>`)
	}
	source("", "网页来源", clue.SourceURL)
	source(" log-clue__source--image", "图片来源", clue.ImageSourceURL)
	if len(clue.Private) > 0 {
		out.WriteString(`<div class="log-clue__body log-clue__private">`)
		for _, section := range clue.Private {
			if !richDocumentHasContent(&section.Content) {
				continue
			}
			name := strings.TrimSpace(section.MemberName)
			if name == "" {
				name = "成员"
			}
			out.WriteString(`<section class="log-clue__private-section"><div class="log-clue__private-head"><span class="log-clue__private-label">专属信息</span><strong class="log-clue__private-member">` + htmlEscape(name) + `</strong></div><div class="log-clue__private-body">` + RenderAgentRichDocumentHTML(section.Content, true) + `</div></section>`)
		}
		out.WriteString(`</div>`)
	}
	out.WriteString(`</div>`)
	return out.String()
}

func logDocumentHasImage(nodes []AgentRichNode) bool {
	for _, n := range nodes {
		if n.Type == "image" || logDocumentHasImage(n.Children) {
			return true
		}
	}
	return false
}

// logResolveBody keeps only http(s) links and points every image at resolve's
// source; unavailable and external images become plain placeholders. Internal
// source attributes are cleared before any HTML leaves the service.
func logResolveBody(body *logMessageBody, resolve func(LogBodyAssetRef) (string, error)) error {
	count := 0
	var walk func([]AgentRichNode, int) error
	walk = func(nodes []AgentRichNode, depth int) error {
		if depth > 32 {
			return logError("TOO_LARGE", "正文嵌套超限")
		}
		for i := range nodes {
			n := &nodes[i]
			marks := n.Marks[:0]
			for _, mark := range n.Marks {
				if mark.Type == "link" {
					u, err := url.Parse(mark.Value)
					if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
						continue
					}
				}
				marks = append(marks, mark)
			}
			n.Marks = marks
			count++
			if count > 10000 {
				return logError("TOO_LARGE", "正文节点超限")
			}
			if n.Type == "image" {
				src := ""
				missingText := "外链图片未嵌入"
				if ref, ok := body.Assets[n.Attrs[logBodyAssetAttr]]; ok {
					missingText = "图片不可用"
					var err error
					if src, err = resolve(ref); err != nil {
						return err
					}
				}
				n.Attrs = map[string]string{}
				if src != "" {
					n.Attrs["src"] = src
				} else {
					n.Type, n.Text = "text", missingText
				}
			}
			if err := walk(n.Children, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(body.Doc.Blocks, 0); err != nil {
		return err
	}
	for i := range body.Clues {
		if err := walk(body.Clues[i].Media.Blocks, 0); err != nil {
			return err
		}
		if err := walk(body.Clues[i].Body.Blocks, 0); err != nil {
			return err
		}
		for j := range body.Clues[i].Private {
			if err := walk(body.Clues[i].Private[j].Content.Blocks, 0); err != nil {
				return err
			}
		}
	}
	return nil
}

// logImageAttachmentID accepts only SealChat attachment references (explicit attribute,
// id:<ID> or an attachment URL); external images never become LOG assets.
func logImageAttachmentID(n AgentRichNode) string {
	if id := strings.TrimSpace(n.Attrs["attachment_id"]); id != "" {
		return id
	}
	src := strings.TrimSpace(n.Attrs["src"])
	if strings.HasPrefix(src, "id:") || strings.Contains(src, "/api/v1/attachment/") {
		return extractAttachmentToken(src)
	}
	return ""
}

// logPublishableAttachment is the attachment a release may snapshot for the message, or nil.
func logPublishableAttachment(m *model.MessageModel, attachmentID string) *model.AttachmentModel {
	a, err := ResolveAttachmentAccessible(m.UserID, m.UserID, m.ChannelID, attachmentID)
	if err != nil || a == nil || a.RootIDType == "world_clue" || a.IsTTSManaged() || a.Size > 20<<20 {
		return nil
	}
	return a
}

func logMessageType(m *model.MessageModel) string {
	if strings.TrimSpace(m.DiceVisualJSON) != "" {
		var payload protocol.DiceVisualPayload
		if json.Unmarshal([]byte(m.DiceVisualJSON), &payload) == nil && len(payload.Groups) > 0 {
			return "dice"
		}
	}
	if m.User != nil && m.User.IsBot {
		return "bot"
	}
	if m.ICMode == "ooc" {
		return "ooc"
	}
	return "ic"
}

func logSpeakerName(m *model.MessageModel) string {
	if m.SenderIdentityName != "" {
		return m.SenderIdentityName
	}
	if m.SenderMemberName != "" {
		return m.SenderMemberName
	}
	return "匿名"
}

func logQuoteOf(speakerName, plainText string) *LogMessageQuote {
	text := strings.Join(strings.Fields(plainText), " ")
	if runes := []rune(text); len(runes) > 60 {
		text = string(runes[:60]) + "…"
	}
	if text == "" {
		return nil
	}
	return &LogMessageQuote{SpeakerName: speakerName, Text: text}
}

// logDesignPresentations projects one design window. The window was loaded through the
// section query, so the viewer may already read every whisper and quote in it.
func logDesignPresentations(viewerID, sectionID string, messages []*model.MessageModel) (map[string]*LogMessagePresentation, error) {
	if err := hydrateWhisperTargetsForExport(messages); err != nil {
		return nil, err
	}
	display := newMessageDisplayResolver("", viewerID, messageDisplayLogArchive)
	out := map[string]*LogMessagePresentation{}
	for _, m := range messages {
		display = display.forChannel(m.ChannelID)
		p := &LogMessagePresentation{}
		body := logMessageBodyOf(display, m)
		// Only server-expanded embeds replace the design body. Ordinary messages keep
		// the chat frontend's full TipTap renderer; otherwise merely containing an
		// image would downgrade unrelated marks/nodes to AgentRich's export subset.
		if body.EmbedKind != "" {
			err := logResolveBody(&body, func(ref LogBodyAssetRef) (string, error) {
				if logResolveBodyAsset(display, m, ref) != nil {
					return "/__log_body_asset/" + sectionID + "/" + url.PathEscape(m.ID) + "/" + ref.Key, nil
				}
				return "", nil
			})
			if err != nil {
				return nil, err
			}
			p.Embed = &LogMessageEmbed{Kind: body.EmbedKind, BodyHTML: body.html(), PlainText: body.plainText(), Clues: body.worldClues()}
		}
		if m.SenderIdentityAvatarID != "" {
			token := ""
			ref := body.addAsset(m, logAvatarAsset(m))
			if logResolveBodyAsset(display, m, ref) != nil {
				token = "body:" + sectionID + "/" + m.ID + "/" + ref.Key
			}
			p.AvatarToken = &token
		}
		if m.IsWhisper {
			if targets := display.whisperTargets(m); len(targets) > 0 {
				p.Whisper = &LogMessageWhisper{Targets: targets}
			}
		}
		// A release never shows a quoted whisper; the design window only does inside whispers.
		if q := m.Quote; q != nil && (!q.IsWhisper || m.IsWhisper) {
			p.Quote = logQuoteOf(logSpeakerName(q), logMessageBodyOf(display, q).plainText())
		}
		if !p.empty() {
			out[m.ID] = p
		}
	}
	return out, nil
}
