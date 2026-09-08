package service

import (
	"fmt"
	"net/url"
	"strings"

	"sealchat/model"
	"sealchat/protocol"
)

// WorldClueExportBlock is export-only metadata. It is deliberately excluded
// from ExportMessage JSON so the existing export contracts remain unchanged.
type WorldClueExportBlock struct {
	Title             string
	Kind              string
	Public            AgentRichDocument
	Private           []WorldClueExportPrivateSection
	ImageAttachmentID string
	ImageURL          string
	IframeSourceURL   string
}

type WorldClueExportPrivateSection struct {
	MemberName string
	Content    AgentRichDocument
}

type worldClueExportRender struct {
	Plain  string
	HTML   string
	Blocks []WorldClueExportBlock
}

type worldClueExportResolver struct {
	channelID string
	userID    string
	worldID   string
	cache     map[string]*WorldClueExportBlock
	userNames map[string]string
}

type worldClueEmbedTarget struct {
	WorldID   string
	ChannelID string
	ClueID    string
	RawLink   string
}

const worldClueMediaAttr = "world_clue_media"

func isStandaloneExportEmbedMessage(content string) bool {
	if _, ok := parseOnlyStickyNoteEmbedTargets(content); ok {
		return true
	}
	if _, ok := parseOnlyWorldClueEmbedTargets(content); ok {
		return true
	}
	return false
}

func newWorldClueExportResolver(channelID, userID string) *worldClueExportResolver {
	resolver := &worldClueExportResolver{
		channelID: strings.TrimSpace(channelID),
		userID:    strings.TrimSpace(userID),
		cache:     make(map[string]*WorldClueExportBlock),
		userNames: make(map[string]string),
	}
	if resolver.channelID == "" || model.GetDB() == nil {
		return resolver
	}
	channel, err := model.ChannelGet(resolver.channelID)
	if err == nil && channel != nil && strings.TrimSpace(channel.ID) != "" {
		resolver.worldID = strings.TrimSpace(channel.WorldID)
	}
	return resolver
}

func (r *worldClueExportResolver) render(content string, includeImages bool) (worldClueExportRender, bool) {
	if r == nil || r.channelID == "" || r.userID == "" || r.worldID == "" {
		return worldClueExportRender{}, false
	}
	targets, ok := parseOnlyWorldClueEmbedTargets(content)
	if !ok || len(targets) == 0 {
		return worldClueExportRender{}, false
	}
	blocks := make([]WorldClueExportBlock, 0, len(targets))
	for _, target := range targets {
		if target.ChannelID != r.channelID || target.WorldID != r.worldID {
			return worldClueExportRender{}, false
		}
		block := r.load(target, includeImages)
		if block == nil {
			return worldClueExportRender{}, false
		}
		blocks = append(blocks, *block)
	}
	plainParts := make([]string, 0, len(blocks))
	htmlParts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		plainParts = append(plainParts, renderWorldCluePlain(block, includeImages))
		htmlParts = append(htmlParts, renderWorldClueHTML(block, includeImages))
	}
	return worldClueExportRender{
		Plain:  strings.TrimSpace(strings.Join(plainParts, "\n\n")),
		HTML:   strings.Join(htmlParts, ""),
		Blocks: blocks,
	}, true
}

func (r *worldClueExportResolver) load(target worldClueEmbedTarget, includeImages bool) *WorldClueExportBlock {
	if r == nil || model.GetDB() == nil {
		return nil
	}
	if r.cache == nil {
		r.cache = make(map[string]*WorldClueExportBlock)
	}
	key := target.WorldID + ":" + target.ClueID
	if includeImages {
		key += ":images"
	} else {
		key += ":no-images"
	}
	if block, ok := r.cache[key]; ok {
		return block
	}
	r.cache[key] = nil
	detail, err := WorldClueGet(target.WorldID, target.ClueID, r.userID)
	if err != nil || detail == nil {
		return nil
	}

	content := strings.TrimSpace(detail.Content)
	if content == "" {
		content = strings.TrimSpace(detail.ContentText)
	}
	public := buildRichDocumentForExport(content, includeImages)
	iframeSourceURL := ""
	if strings.TrimSpace(detail.Kind) == model.WorldClueKindIframe {
		iframeSourceURL = safeExportHTTPURL(detail.EmbedURL)
	}
	block := &WorldClueExportBlock{
		Title:             strings.TrimSpace(detail.Title),
		Kind:              strings.TrimSpace(detail.Kind),
		Public:            public,
		ImageAttachmentID: strings.TrimPrefix(strings.TrimSpace(detail.ImageAttachmentID), "id:"),
		ImageURL:          strings.TrimSpace(detail.ImageURL),
		IframeSourceURL:   iframeSourceURL,
	}
	if block.Title == "" {
		block.Title = "未命名线索"
	}
	if block.Kind == "" {
		block.Kind = model.WorldClueKindText
	}
	if includeImages {
		appendWorldClueImage(&block.Public, block.ImageAttachmentID, block.ImageURL)
	}

	db := model.GetDB()
	role, roleErr := worldClueRole(db, target.WorldID, r.userID)
	if roleErr == nil && worldClueIsAdminRole(role) {
		r.loadWorldCluePrivateSections(block, target.WorldID, target.ClueID, includeImages)
	} else if privateContent := r.memberPrivateContent(target, detail); privateContent != "" {
		name := r.userDisplayName(r.userID)
		block.Private = append(block.Private, WorldClueExportPrivateSection{
			MemberName: name,
			Content:    buildRichDocumentForExport(privateContent, includeImages),
		})
	}

	r.cache[key] = block
	return block
}

func (r *worldClueExportResolver) loadWorldCluePrivateSections(block *WorldClueExportBlock, worldID, clueID string, includeImages bool) {
	if r == nil || block == nil || model.GetDB() == nil {
		return
	}
	var rows []model.WorldClueAccessModel
	if err := model.GetDB().Where("world_id = ? AND clue_id = ?", worldID, clueID).Order("user_id asc").Find(&rows).Error; err != nil {
		return
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if worldClueAccessPrivateExportContent(&row) != "" {
			ids = append(ids, row.UserID)
		}
	}
	names := r.userDisplayNames(ids)
	for _, row := range rows {
		privateContent := worldClueAccessPrivateExportContent(&row)
		if privateContent == "" {
			continue
		}
		memberName := names[strings.TrimSpace(row.UserID)]
		if memberName == "" {
			memberName = strings.TrimSpace(row.UserID)
		}
		block.Private = append(block.Private, WorldClueExportPrivateSection{
			MemberName: memberName,
			Content:    buildRichDocumentForExport(privateContent, includeImages),
		})
	}
}

func (r *worldClueExportResolver) memberPrivateContent(target worldClueEmbedTarget, detail *protocol.WorldClueDetail) string {
	// WorldClueGet is the authority for member visibility. The DTO normally
	// carries PrivateContent; the text column is only consulted as a legacy
	// fallback when a row was persisted without its rich source value.
	if detail == nil || detail.Status != model.WorldClueStatusPublished {
		return ""
	}
	if content := strings.TrimSpace(detail.PrivateContent); content != "" {
		return content
	}
	if r == nil || model.GetDB() == nil {
		return ""
	}
	var row model.WorldClueAccessModel
	if err := model.GetDB().Where("world_id = ? AND clue_id = ? AND user_id = ?", target.WorldID, target.ClueID, r.userID).Limit(1).Find(&row).Error; err != nil {
		return ""
	}
	return worldClueAccessPrivateExportContent(&row)
}

func worldClueAccessPrivateExportContent(row *model.WorldClueAccessModel) string {
	if row == nil {
		return ""
	}
	if content := strings.TrimSpace(row.PrivateContent); content != "" {
		return content
	}
	return strings.TrimSpace(row.PrivateContentText)
}

func (r *worldClueExportResolver) userDisplayName(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ""
	}
	return r.userDisplayNames([]string{userID})[userID]
}

func (r *worldClueExportResolver) userDisplayNames(ids []string) map[string]string {
	result := make(map[string]string, len(ids))
	if r == nil {
		return result
	}
	if r.userNames == nil {
		r.userNames = make(map[string]string)
	}
	missing := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, rawID := range ids {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if name, ok := r.userNames[id]; ok {
			result[id] = name
			continue
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 && model.GetDB() != nil {
		var users []model.UserModel
		if err := model.GetDB().Where("id IN ?", missing).Find(&users).Error; err == nil {
			for i := range users {
				id := strings.TrimSpace(users[i].ID)
				if id == "" {
					continue
				}
				name := resolveUserDisplayName(&users[i])
				if name == "" {
					name = id
				}
				r.userNames[id] = name
			}
		}
	}
	for _, id := range missing {
		if _, ok := r.userNames[id]; !ok {
			r.userNames[id] = id
		}
		result[id] = r.userNames[id]
	}
	for id := range result {
		if result[id] == "" {
			result[id] = id
		}
	}
	return result
}

func parseOnlyWorldClueEmbedTargets(content string) ([]worldClueEmbedTarget, bool) {
	candidate := normalizeStickyNoteEmbedCandidate(content)
	if candidate == "" {
		return nil, false
	}
	parts := splitStickyNoteEmbedOnlyParts(candidate)
	if len(parts) == 0 {
		return nil, false
	}
	targets := make([]worldClueEmbedTarget, 0, len(parts))
	for _, part := range parts {
		target, ok := parseWorldClueEmbedTargetPart(part)
		if !ok {
			return nil, false
		}
		targets = append(targets, target)
	}
	return targets, true
}

func parseWorldClueEmbedTargetPart(candidate string) (worldClueEmbedTarget, bool) {
	candidate = trimStickyNoteLinkWrapper(candidate)
	if candidate == "" || containsUnicodeSpace(candidate) {
		return worldClueEmbedTarget{}, false
	}
	match := stickyNoteEmbedURLPattern.FindStringSubmatch(candidate)
	if len(match) != 4 {
		return worldClueEmbedTarget{}, false
	}
	query, err := url.ParseQuery(strings.ReplaceAll(match[3], "&amp;", "&"))
	if err != nil {
		return worldClueEmbedTarget{}, false
	}
	clueID := strings.TrimSpace(query.Get("clue"))
	if clueID == "" {
		return worldClueEmbedTarget{}, false
	}
	return worldClueEmbedTarget{WorldID: match[1], ChannelID: match[2], ClueID: clueID, RawLink: candidate}, true
}

func appendWorldClueImage(doc *AgentRichDocument, attachmentID, imageURL string) {
	if doc == nil {
		return
	}
	attachmentID = strings.TrimPrefix(strings.TrimSpace(attachmentID), "id:")
	imageURL = strings.TrimSpace(imageURL)
	if attachmentID == "" {
		attachmentID = extractAttachmentToken(imageURL)
	}
	if attachmentID == "" {
		imageURL = safeExportHTTPURL(imageURL)
	}
	if attachmentID == "" && imageURL == "" {
		return
	}
	src := imageURL
	attrs := map[string]string{"alt": "线索图片"}
	if attachmentID != "" {
		attrs["attachment_id"] = attachmentID
		attrs[worldClueMediaAttr] = "true"
		src = resolveImageURL("id:" + attachmentID)
	} else {
		attrs[worldClueMediaAttr] = "true"
	}
	if src == "" {
		return
	}
	attrs["src"] = src
	doc.Blocks = append(doc.Blocks, AgentRichNode{Type: "paragraph", Children: []AgentRichNode{{Type: "image", Attrs: attrs}}})
}

func worldClueImageAttachmentToken(block WorldClueExportBlock) string {
	if attachmentID := strings.TrimPrefix(strings.TrimSpace(block.ImageAttachmentID), "id:"); attachmentID != "" {
		return attachmentID
	}
	return extractAttachmentToken(block.ImageURL)
}

func splitWorldCluePublicDocument(doc AgentRichDocument) (AgentRichDocument, AgentRichDocument) {
	body := doc
	body.Blocks = make([]AgentRichNode, 0, len(doc.Blocks))
	media := AgentRichDocument{Type: doc.Type, Blocks: make([]AgentRichNode, 0, 1)}
	for _, block := range doc.Blocks {
		if isWorldClueMediaBlock(block) {
			media.Blocks = append(media.Blocks, block)
			continue
		}
		body.Blocks = append(body.Blocks, block)
	}
	return body, media
}

func isWorldClueMediaBlock(block AgentRichNode) bool {
	if block.Type != "paragraph" || len(block.Children) != 1 {
		return false
	}
	image := block.Children[0]
	return image.Type == "image" && strings.EqualFold(strings.TrimSpace(image.Attrs[worldClueMediaAttr]), "true")
}

func renderWorldCluePlain(block WorldClueExportBlock, includeImages bool) string {
	title := strings.TrimSpace(block.Title)
	if title == "" {
		title = "未命名线索"
	}
	kind := strings.TrimSpace(block.Kind)
	if kind == "" {
		kind = model.WorldClueKindText
	}
	lines := []string{fmt.Sprintf("[线索: %s]", title), "类型: " + kind}
	body := strings.TrimSpace(AgentRichDocumentPlainText(block.Public))
	if body == "" {
		body = "（空线索）"
	}
	lines = append(lines, body)
	if kind == model.WorldClueKindIframe && block.IframeSourceURL != "" {
		lines = append(lines, "网页线索/来源: "+block.IframeSourceURL)
	}
	if includeImages && worldClueImageAttachmentToken(block) == "" && block.ImageURL != "" {
		if imageURL := safeExportHTTPURL(block.ImageURL); imageURL != "" {
			lines = append(lines, "图片来源: "+imageURL)
		}
	}
	for _, section := range block.Private {
		name := strings.TrimSpace(section.MemberName)
		if name == "" {
			name = "成员"
		}
		privateBody := strings.TrimSpace(AgentRichDocumentPlainText(section.Content))
		if privateBody == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("[专属信息: %s]", name), privateBody)
	}
	return normalizePlainText(strings.Join(lines, "\n"))
}

func renderWorldClueHTML(block WorldClueExportBlock, includeImages bool) string {
	title := strings.TrimSpace(block.Title)
	if title == "" {
		title = "未命名线索"
	}
	kind := strings.TrimSpace(block.Kind)
	if kind == "" {
		kind = model.WorldClueKindText
	}
	bodyDocument, mediaDocument := splitWorldCluePublicDocument(block.Public)
	body := RenderAgentRichDocumentHTML(bodyDocument, includeImages)
	media := RenderAgentRichDocumentHTML(mediaDocument, includeImages)
	if strings.TrimSpace(body) == "" && strings.TrimSpace(media) == "" {
		body = `<p class="export-world-clue__empty">（空线索）</p>`
	}
	var builder strings.Builder
	builder.WriteString(`<section class="export-world-clue export-world-clue--` + htmlEscape(kind) + `">`)
	builder.WriteString(`<header class="export-world-clue__header"><span class="export-world-clue__label">线索</span><span class="export-world-clue__title">` + htmlEscape(title) + `</span><span class="export-world-clue__kind">` + htmlEscape(worldClueExportKindLabel(kind)) + `</span></header>`)
	if strings.TrimSpace(media) != "" {
		builder.WriteString(`<div class="export-world-clue__media">` + media + `</div>`)
	}
	builder.WriteString(`<div class="export-world-clue__body">` + body + `</div>`)
	if kind == model.WorldClueKindIframe && block.IframeSourceURL != "" {
		builder.WriteString(`<div class="export-world-clue__source"><span>网页线索/来源：</span><a href="` + htmlEscape(block.IframeSourceURL) + `" target="_blank" rel="noopener noreferrer">` + htmlEscape(block.IframeSourceURL) + `</a></div>`)
	}
	if includeImages && worldClueImageAttachmentToken(block) == "" && block.ImageURL != "" {
		if imageURL := safeExportHTTPURL(block.ImageURL); imageURL != "" {
			builder.WriteString(`<div class="export-world-clue__source"><span>图片来源：</span><a href="` + htmlEscape(imageURL) + `" target="_blank" rel="noopener noreferrer">` + htmlEscape(imageURL) + `</a></div>`)
		}
	}
	for _, section := range block.Private {
		name := strings.TrimSpace(section.MemberName)
		if name == "" {
			name = "成员"
		}
		privateBody := RenderAgentRichDocumentHTML(section.Content, includeImages)
		if strings.TrimSpace(privateBody) == "" {
			continue
		}
		builder.WriteString(`<section class="export-world-clue__private"><div class="export-world-clue__private-title">专属信息 · ` + htmlEscape(name) + `</div><div class="export-world-clue__private-body">` + privateBody + `</div></section>`)
	}
	builder.WriteString(`</section>`)
	return builder.String()
}

func worldClueExportKindLabel(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case model.WorldClueKindText:
		return "文本"
	case model.WorldClueKindImage:
		return "图片"
	case model.WorldClueKindIframe:
		return "网页"
	default:
		return strings.TrimSpace(kind)
	}
}

func safeExportHTTPURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return ""
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return ""
	}
	return parsed.String()
}
