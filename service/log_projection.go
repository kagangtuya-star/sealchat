package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sealchat/model"
	"sealchat/utils"
)

func logPublicKey(release, domain, id string) string {
	h := sha256.Sum256([]byte(release + "\x00" + domain + "\x00" + id))
	return hex.EncodeToString(h[:16])
}
func logAssetRef(release, key, node string) error {
	r := model.LogAssetReferenceModel{ReleaseID: release, AssetKey: key, NodeID: node}
	return model.GetDB().Clauses(clause.OnConflict{DoNothing: true}).Create(&r).Error
}
func logBuildStyleAssets(r *model.LogReleaseModel, d *LogDraft) error {
	if d.StylePackageID == "" {
		return nil
	}
	var assets []model.LogStyleAssetModel
	if err := model.GetDB().Where("package_id = ? AND version = ?", d.StylePackageID, d.StylePackageVersion).Find(&assets).Error; err != nil {
		return err
	}
	total := int64(0)
	for _, a := range assets {
		total += int64(len(a.Data))
		key := logPublicKey(r.ID, "style", a.AssetID)
		row := model.LogReleaseAssetModel{ReleaseID: r.ID, AssetKey: key, MIME: a.MIME, Hash: a.Hash, Data: a.Data}
		if err := model.GetDB().Create(&row).Error; err != nil {
			return err
		}
		if err := logAssetRef(r.ID, key, ""); err != nil {
			return err
		}
	}
	return model.GetDB().Model(r).Updates(map[string]any{"asset_bytes": total, "asset_count": len(assets)}).Error
}
func logSnapshotBodyAsset(ctx context.Context, r *model.LogReleaseModel, display *messageDisplayResolver, m *model.MessageModel, nodeID string, ref LogBodyAssetRef) (string, error) {
	a := logResolveBodyAsset(display, m, ref)
	if a == nil {
		return "", nil
	}
	return logSnapshotAttachment(ctx, r, a, logPublicKey(r.ID, "attachment", a.ID), nodeID)
}

// logSnapshotAttachment copies a validated stored image into the release once per key and
// references it from nodeID ("" = publisher-approved page/element resource). Unreadable or
// invalid images return an empty key so each caller decides whether that is fatal.
func logSnapshotAttachment(ctx context.Context, r *model.LogReleaseModel, a *model.AttachmentModel, key, nodeID string) (string, error) {
	var existing int64
	if err := model.GetDB().Model(&model.LogReleaseAssetModel{}).Where("release_id = ? AND asset_key = ?", r.ID, key).Count(&existing).Error; err != nil {
		return "", err
	}
	if existing > 0 {
		return key, logAssetRef(r.ID, key, nodeID)
	}
	asset, err := logReadBodyAsset(ctx, a)
	if err != nil {
		return "", nil
	}
	b := asset.Data
	p := LogStylePackage{SchemaVersion: 1, ID: "asset-check", Version: "1.0.0", MinRendererVersion: "1.0.0", Name: "asset", Tokens: map[string]any{}, Rules: []LogStyleRule{}, Templates: map[string]string{"empty": "<div></div>"}, SettingsSchema: map[string]any{}, Assets: []LogStyleAsset{{ID: "asset", Path: "assets/item", MIME: asset.MIME}}}
	if ValidateLogStyle(&p, map[string][]byte{"asset": b}) != nil || !strings.HasPrefix(p.Assets[0].MIME, "image/") {
		return "", nil
	}
	row := model.LogReleaseAssetModel{ReleaseID: r.ID, AssetKey: key, AttachmentID: a.ID, MIME: p.Assets[0].MIME, Hash: p.Assets[0].SHA256, Data: b}
	if err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		reserve := tx.Model(&model.LogReleaseModel{}).Where("id = ? AND asset_bytes <= ? AND asset_count < ?", r.ID, (512<<20)-len(b), 10000).Updates(map[string]any{"asset_bytes": gorm.Expr("asset_bytes + ?", len(b)), "asset_count": gorm.Expr("asset_count + 1")})
		if reserve.Error != nil {
			return reserve.Error
		}
		if reserve.RowsAffected != 1 {
			return logError("TOO_LARGE", "发布资源超过 512MB 或一万项")
		}
		return tx.Create(&row).Error
	}); err != nil {
		return "", err
	}
	if err := logAssetRef(r.ID, key, nodeID); err != nil {
		return "", err
	}
	return key, nil
}

// logApplyContentOverride returns the message carrying the LOG draft's project-local body.
// The override only rewrites text: it is ignored when either body is an embed or when it references
// an attachment the source message does not, so a draft can never publish assets the source lacks.
func logApplyContentOverride(display *messageDisplayResolver, m *model.MessageModel, override string) *model.MessageModel {
	if override == "" || override == m.Content || m.IsDeleted || m.IsRevoked {
		return m
	}
	next := *m
	next.Content = override
	source, target := logMessageBodyOf(display, m), logMessageBodyOf(display, &next)
	if source.EmbedKind != "" || target.EmbedKind != "" {
		return m
	}
	allowed := map[string]bool{}
	for _, ref := range source.Assets {
		allowed[ref.Kind+"\x00"+ref.AttachmentID] = true
	}
	for _, ref := range target.Assets {
		if !allowed[ref.Kind+"\x00"+ref.AttachmentID] {
			return m
		}
	}
	return &next
}

func logBuildMessage(ctx context.Context, r *model.LogReleaseModel, display *messageDisplayResolver, section string, m *model.MessageModel, ordinal int64, imageWidths map[string]float64, showQuoteContent bool, templateOverride string) error {
	id := utils.NewID()
	body := logMessageBodyOf(display, m)
	sourceImageIndex, publicImageIndex := 0, 0
	resolvedWidths := map[string]float64{}
	err := logResolveBody(&body, func(ref LogBodyAssetRef) (string, error) {
		width, hasWidth := imageWidths[strconv.Itoa(sourceImageIndex)]
		sourceImageIndex++
		key, err := logSnapshotBodyAsset(ctx, r, display, m, id, ref)
		if key == "" || err != nil {
			return "", err
		}
		if hasWidth {
			resolvedWidths[strconv.Itoa(publicImageIndex)] = width
		}
		publicImageIndex++
		return "/__log_asset/" + key, nil
	})
	if err != nil {
		return err
	}
	avatar, err := logSnapshotBodyAsset(ctx, r, display, m, id, logAvatarAsset(m))
	if err != nil {
		return err
	}
	speakerID := m.SenderIdentityID
	if speakerID == "" {
		speakerID = m.UserID
	}
	var color, avatarValue, variant any
	if logColorPattern.MatchString(m.SenderIdentityColor) {
		color = m.SenderIdentityColor
	}
	if avatar != "" {
		avatarValue = avatar
	}
	if m.SenderIdentityVariantID != "" {
		variant = logPublicKey(r.ID, "variant", m.SenderIdentityVariantID)
	}
	plain := body.plainText()
	node := map[string]any{"id": id, "kind": "message", "sectionId": section, "messageType": logMessageType(m), "speaker": map[string]any{"key": logPublicKey(r.ID, "speaker", speakerID), "name": logSpeakerName(m), "color": color, "avatarAssetKey": avatarValue, "variantKey": variant}, "bodyHtml": body.html(), "plainText": plain, "createdAt": m.CreatedAt.UTC(), "quoteNodeId": nil}
	// Independent release-scoped domains preserve user matching across characters.
	// Never publish the source message ID, identity ID, user ID, or draft override map.
	if m.SenderIdentityID != "" {
		node["identityKey"] = logPublicKey(r.ID, "identity", m.SenderIdentityID)
	}
	if m.UserID != "" {
		node["userKey"] = logPublicKey(r.ID, "user", m.UserID)
	}
	if templateOverride != "" {
		node["templateOverride"] = templateOverride
	}
	if len(resolvedWidths) > 0 {
		node["imageWidths"] = resolvedWidths
	}
	if body.EmbedKind != "" {
		node["embed"] = body.EmbedKind
	}
	if clues := body.worldClues(); len(clues) > 0 {
		node["clues"] = clues
	}
	// Releases only take non-whisper messages; should that change, recipients still never leave as raw IDs.
	if m.IsWhisper {
		targets := []MessageWhisperTarget{}
		for _, t := range display.forChannel(m.ChannelID).whisperTargets(m) {
			targets = append(targets, MessageWhisperTarget{Key: logPublicKey(r.ID, "whisper", t.Key), Name: t.Name})
		}
		node["whisper"] = map[string]any{"targets": targets}
	}
	b, err := json.Marshal(node)
	if err != nil {
		return err
	}
	quoteMessageID := ""
	if showQuoteContent {
		quoteMessageID = m.QuoteID
	}
	row := model.LogReleaseNodeModel{ID: id, ReleaseID: r.ID, SectionID: section, Ordinal: ordinal, MessageID: m.ID, ChannelID: m.ChannelID, SourceVersion: m.Version, NodeJSON: string(b), SearchText: plain, QuoteMessageID: quoteMessageID}
	return model.GetDB().Create(&row).Error
}
func logBuildElement(r *model.LogReleaseModel, e LogElement, ordinal int64, projectKeys map[string]string) error {
	id := utils.NewID()
	content := e.Content
	if asset, ok := content["assetId"].(string); ok {
		delete(content, "assetId")
		content["assetKey"] = logPublicKey(r.ID, "style", asset)
	}
	// Readers only see assetKey; whether it came from the style or the project stays internal.
	if asset, ok := content["projectAssetId"].(string); ok {
		delete(content, "projectAssetId")
		key := projectKeys[asset]
		if key == "" {
			return logProjectAssetsUnavailable([]string{asset})
		}
		content["assetKey"] = key
	}
	if body, ok := content["body"].(string); ok {
		doc := buildLogRichDocument(body, false)
		content["body"] = renderLogRichDocumentHTML(doc, false)
	}
	b, err := json.Marshal(map[string]any{"id": id, "kind": "element", "sectionId": e.Anchor.SectionID, "type": e.Type, "templateId": e.TemplateID, "content": content, "style": e.Style})
	if err != nil {
		return err
	}
	row := model.LogReleaseNodeModel{ID: id, ReleaseID: r.ID, SectionID: e.Anchor.SectionID, Ordinal: ordinal, NodeJSON: string(b)}
	return model.GetDB().Create(&row).Error
}
func logBuildPublicMetadata(r *model.LogReleaseModel, d *LogDraft, projectKeys map[string]string) (map[string]any, error) {
	// d.Sections is already in publish order. In chapter mode a section's public title is its chapter's
	// visual title (labels are editor-only); readers that predate chapters still get one entry per section.
	chapterTitles := map[string]string{}
	for _, c := range d.Chapters {
		chapterTitles[c.ID] = c.Title
	}
	sections := []map[string]string{}
	chapters := []map[string]any{}
	for _, s := range d.Sections {
		if s.ChapterID == "" {
			sections = append(sections, map[string]string{"id": s.ID, "title": s.Title})
			continue
		}
		sections = append(sections, map[string]string{"id": s.ID, "title": chapterTitles[s.ChapterID], "chapterId": s.ChapterID})
		if n := len(chapters); n > 0 && chapters[n-1]["id"] == s.ChapterID {
			chapters[n-1]["sectionIds"] = append(chapters[n-1]["sectionIds"].([]string), s.ID)
			continue
		}
		chapters = append(chapters, map[string]any{"id": s.ChapterID, "title": chapterTitles[s.ChapterID], "sectionIds": []string{s.ID}})
	}
	b, _ := json.Marshal(d.Page)
	page := map[string]any{}
	if err := json.Unmarshal(b, &page); err != nil {
		return nil, err
	}
	if d.Page.BackgroundAssetID != nil {
		page["backgroundAssetId"] = logPublicKey(r.ID, "style", *d.Page.BackgroundAssetID)
	}
	// A project background is published under the same public field as a style background.
	delete(page, "backgroundProjectAssetId")
	if id := d.Page.BackgroundProjectAssetID; id != nil {
		key := projectKeys[*id]
		if key == "" {
			return nil, logProjectAssetsUnavailable([]string{*id})
		}
		page["backgroundAssetId"] = key
	}
	var style any
	if d.StylePackageID != "" {
		p, err := logLoadStyle(d.StylePackageID, d.StylePackageVersion)
		if err != nil {
			return nil, err
		}
		rules := []map[string]any{}
		for _, rule := range p.Rules {
			item := map[string]any{"templateId": rule.TemplateID}
			if rule.MessageType != "" {
				item["messageType"] = rule.MessageType
			}
			if rule.IdentityKey != "" {
				item["identityKey"] = logPublicKey(r.ID, "speaker", rule.IdentityKey)
			}
			if rule.VariantKey != "" {
				item["variantKey"] = logPublicKey(r.ID, "variant", rule.VariantKey)
			}
			rules = append(rules, item)
		}
		assets := []map[string]string{}
		cssText := logPublicCSS(r.ID, p)
		for _, a := range p.Assets {
			key := logPublicKey(r.ID, "style", a.ID)
			assets = append(assets, map[string]string{"assetKey": key, "mime": a.MIME, "sha256": a.SHA256})
			if strings.HasPrefix(a.MIME, "font/") && d.Page.FontFamily == a.ID {
				page["fontFamily"] = "log-font-" + key
			}
		}
		tokens := p.Tokens
		for k, v := range d.ThemeTokens {
			tokens[k] = v
		}
		style = map[string]any{"schemaVersion": 1, "version": p.Version, "name": p.Name, "license": p.License, "minRendererVersion": p.MinRendererVersion, "tokens": tokens, "templates": p.Templates, "css": cssText, "rules": rules, "settingsSchema": p.SettingsSchema, "assets": assets}
	}
	out := map[string]any{"schemaVersion": 1, "title": d.Title, "releaseId": r.ID, "sections": sections, "page": page, "style": style}
	if len(d.DisplayRules) > 0 {
		out["displayRules"] = logPublicDisplayRules(r.ID, d.DisplayRules)
	}
	if len(chapters) > 0 {
		out["chapters"] = chapters
	}
	return out, nil
}

// Project display rules use the same safe mapping as public nodes. Selection stays
// in the shared frontend renderer; no second template resolver is maintained here.
func logPublicDisplayRules(release string, rules []LogDisplayRule) []LogDisplayRule {
	out := make([]LogDisplayRule, 0, len(rules))
	for _, rule := range rules {
		rule.ID = logPublicKey(release, "displayRule", rule.ID)
		if rule.Match.IdentityID != "" {
			rule.Match.IdentityID = logPublicKey(release, "identity", rule.Match.IdentityID)
		}
		if rule.Match.UserID != "" {
			rule.Match.UserID = logPublicKey(release, "user", rule.Match.UserID)
		}
		if rule.Match.VariantID != "" {
			rule.Match.VariantID = logPublicKey(release, "variant", rule.Match.VariantID)
		}
		out = append(out, rule)
	}
	return out
}

func logPublicCSS(release string, p *LogStylePackage) string {
	paths := map[string]string{}
	fonts := map[string]string{}
	for _, a := range p.Assets {
		key := logPublicKey(release, "style", a.ID)
		paths[a.Path] = "/__log_asset/" + key
		if strings.HasPrefix(a.MIME, "font/") {
			fonts[a.ID] = "log-font-" + key
		}
	}
	lexer := css.NewLexer(parse.NewInputString(p.CSS))
	var out strings.Builder
	property := ""
	fontDeclaration := false
	depth := 0
	for {
		tt, data := lexer.Next()
		if tt == css.ErrorToken {
			break
		}
		value := string(data)
		switch tt {
		case css.LeftBraceToken:
			depth++
			property = ""
			fontDeclaration = false
		case css.RightBraceToken:
			depth--
			property = ""
			fontDeclaration = false
		case css.SemicolonToken:
			property = ""
			fontDeclaration = false
		case css.ColonToken:
			fontDeclaration = depth > 0 && strings.EqualFold(property, "font-family")
		case css.IdentToken:
			if fontDeclaration {
				if name, ok := fonts[value]; ok {
					value = name
				}
			} else {
				property = value
			}
		case css.StringToken:
			if fontDeclaration {
				if name, ok := fonts[strings.Trim(value, "\"'")]; ok {
					value = "\"" + name + "\""
				}
			}
		case css.URLToken:
			if len(value) >= 5 {
				path := strings.Trim(strings.TrimSpace(value[4:len(value)-1]), "\"'")
				if mapped, ok := paths[path]; ok {
					value = "url(\"" + mapped + "\")"
				}
			}
		}
		out.WriteString(value)
	}
	return out.String()
}
func logResolvePublicURLs(raw, slug string) string {
	return strings.ReplaceAll(raw, "/__log_asset/", "/api/v1/public/log/"+url.PathEscape(slug)+"/assets/")
}
