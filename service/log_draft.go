package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sealchat/model"
)

type LogSourceSpec struct {
	ChannelID        string     `json:"channelId"`
	FromTime         *time.Time `json:"fromTime"`
	FromExclusive    bool       `json:"fromExclusive,omitempty"`
	ToTime           time.Time  `json:"toTime"`
	IncludeOOC       bool       `json:"includeOoc"`
	IncludeArchived  bool       `json:"includeArchived"`
	ShowQuoteContent bool       `json:"showQuoteContent"`
	IdentityIDs      []string   `json:"identityIds"`
}

// LogSection is the smallest source unit: one channel and one linear message range.
// Queries, cursors, exclusions, anchors and release nodes stay keyed by section ID.
// In chapter mode Title is only an arrangement label; the visual title is the chapter's.
type LogSection struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	Source    LogSourceSpec `json:"source"`
	Order     int           `json:"order"`
	ChapterID string        `json:"chapterId,omitempty"`
}

// LogChapter groups consecutive sections into one visual/narrative chapter. It is never a query unit.
// Drafts without chapters (legacy) treat every section as its own single-source chapter.
type LogChapter struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Order int    `json:"order"`
}
type LogPageSettings struct {
	Preset    string  `json:"preset"`
	WidthMM   float64 `json:"widthMm"`
	HeightMM  float64 `json:"heightMm"`
	MarginsMM struct {
		Top    float64 `json:"top"`
		Right  float64 `json:"right"`
		Bottom float64 `json:"bottom"`
		Left   float64 `json:"left"`
	} `json:"marginsMm"`
	GutterMM          float64 `json:"gutterMm"`
	Columns           int     `json:"columns"`
	BackgroundColor   string  `json:"backgroundColor"`
	BackgroundAssetID *string `json:"backgroundAssetId"`
	FontFamily        string  `json:"fontFamily"`
	FontSizePT        float64 `json:"fontSizePt"`
	LineHeight        float64 `json:"lineHeight"`
	HeaderTemplateID  *string `json:"headerTemplateId"`
	FooterTemplateID  *string `json:"footerTemplateId"`
	ShowPageNumbers   bool    `json:"showPageNumbers"`
	MergeNeighbors    bool    `json:"mergeNeighbors"`

	// BackgroundProjectAssetID references the LOG's own asset registry; backgroundAssetId stays a style asset.
	BackgroundProjectAssetID *string `json:"backgroundProjectAssetId,omitempty"`
}
type LogDisplayMatch struct {
	MessageType string `json:"messageType,omitempty"`
	IdentityID  string `json:"identityId,omitempty"`
	UserID      string `json:"userId,omitempty"`
	VariantID   string `json:"variantId,omitempty"`
}
type LogDisplayRule struct {
	ID         string          `json:"id"`
	Match      LogDisplayMatch `json:"match"`
	TemplateID string          `json:"templateId"`
}
type LogDraft struct {
	SchemaVersion       int               `json:"schemaVersion"`
	Title               string            `json:"title"`
	Sections            []LogSection      `json:"sections"`
	Chapters            []LogChapter      `json:"chapters,omitempty"`
	Elements            []json.RawMessage `json:"elements"`
	Page                LogPageSettings   `json:"page"`
	StylePackageID      string            `json:"stylePackageId"`
	StylePackageVersion string            `json:"stylePackageVersion"`
	ThemeTokens         map[string]any    `json:"themeTokens"`
	// Assets registers ordinary LOG images (attachments) independently of the style package.
	Assets                   []LogProjectAsset  `json:"assets,omitempty"`
	ImageWidths              map[string]float64 `json:"imageWidths,omitempty"`
	DisplayRules             []LogDisplayRule   `json:"displayRules,omitempty"`
	MessageTemplateOverrides map[string]string  `json:"messageTemplateOverrides,omitempty"`
	// MessageContentOverrides is the LOG's own copy of a message body (e.g. batch proofreading);
	// it never writes back to the chat message and is applied only where it keeps the source's assets.
	MessageContentOverrides map[string]string `json:"messageContentOverrides,omitempty"`
	// Replay is legacy/extension draft data. Preserve it losslessly across edits;
	// it is never interpreted by LOG services and remains under the draft size cap.
	Replay json.RawMessage `json:"replay,omitempty"`
}
type LogDraftUpdate struct {
	ExpectedVersion int64           `json:"expectedVersion"`
	Draft           json.RawMessage `json:"draft"`
}
type LogDraftResult struct {
	DraftVersion int64    `json:"draftVersion"`
	Draft        LogDraft `json:"draft"`
}

// logMaxContentOverrideBytes bounds one project-local message body; the whole draft stays under 2MB.
const logMaxContentOverrideBytes = 64 << 10

var logUUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var logColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$`)
var logPlatformFontPattern = regexp.MustCompile(`^platform:[a-zA-Z0-9_-]{1,100}$`)
var logMessageImagePattern = regexp.MustCompile(`^message:([A-Za-z0-9_-]{1,100}):image:(0|[1-9][0-9]{0,8})$`)

func validLogImageWidth(width float64) bool {
	return !math.IsNaN(width) && !math.IsInf(width, 0) && width >= 0.15 && width <= 1
}

// Snapshot image indices without exposing source message IDs in public nodes.
func logMessageImageWidths(d *LogDraft) map[string]map[string]float64 {
	out := map[string]map[string]float64{}
	for key, width := range d.ImageWidths {
		match := logMessageImagePattern.FindStringSubmatch(key)
		if match == nil || !validLogImageWidth(width) {
			continue
		}
		if out[match[1]] == nil {
			out[match[1]] = map[string]float64{}
		}
		out[match[1]][match[2]] = width
	}
	return out
}

func isLogPlatformFont(font string) bool {
	return logPlatformFontPattern.MatchString(font)
}

func parseLogDraft(raw []byte) (LogDraft, error) {
	var draft LogDraft
	if len(raw) > 2<<20 {
		return draft, logError("TOO_LARGE", "草稿超过 2MB")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&draft); err != nil {
		// 严格字段检查仍保留；指出不兼容字段，避免客户端只能反复重试整份草稿。
		if strings.HasPrefix(err.Error(), "json: unknown field ") {
			return draft, logError("INVALID_INPUT", "草稿包含未识别字段："+strings.TrimPrefix(err.Error(), "json: unknown field "))
		}
		return draft, logError("INVALID_INPUT", "草稿格式无效")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return draft, logError("INVALID_INPUT", "草稿格式无效")
	}
	if draft.SchemaVersion != 1 || strings.TrimSpace(draft.Title) == "" || len([]rune(draft.Title)) > 200 || draft.Sections == nil || draft.Elements == nil || len(draft.Sections) > 200 || len(draft.Elements) > 10000 {
		return draft, logError("INVALID_INPUT", "草稿字段或数量无效")
	}
	for key, width := range draft.ImageWidths {
		if !logMessageImagePattern.MatchString(key) || !validLogImageWidth(width) {
			return draft, logError("INVALID_INPUT", "图片尺寸无效")
		}
	}
	// Template references deliberately need not exist in the current package: switching
	// styles preserves them and the shared renderer skips unavailable message templates.
	if len(draft.DisplayRules) > 1000 || len(draft.MessageTemplateOverrides) > 100000 {
		return draft, logError("INVALID_INPUT", "显示规则或消息模板覆盖数量超出上限")
	}
	ruleIDs := map[string]bool{}
	for _, rule := range draft.DisplayRules {
		m := rule.Match
		if !logUUIDPattern.MatchString(rule.ID) || ruleIDs[rule.ID] || !logStyleKey.MatchString(rule.TemplateID) || m == (LogDisplayMatch{}) {
			return draft, logError("INVALID_INPUT", "显示规则无效或没有匹配条件")
		}
		ruleIDs[rule.ID] = true
		switch m.MessageType {
		case "", "ic", "ooc", "narration", "dice", "bot", "image", "unknown":
		default:
			return draft, logError("INVALID_INPUT", "显示规则消息类型无效")
		}
		for _, id := range []string{m.IdentityID, m.UserID, m.VariantID} {
			if len(id) > 100 || (id != "" && strings.TrimSpace(id) != id) {
				return draft, logError("INVALID_INPUT", "显示规则匹配标识无效")
			}
		}
	}
	for id, template := range draft.MessageTemplateOverrides {
		if id == "" || len(id) > 100 || strings.TrimSpace(id) != id || !logStyleKey.MatchString(template) {
			return draft, logError("INVALID_INPUT", "消息模板覆盖无效")
		}
	}
	if len(draft.MessageContentOverrides) > 100000 {
		return draft, logError("INVALID_INPUT", "消息正文覆盖数量超出上限")
	}
	for id, content := range draft.MessageContentOverrides {
		if id == "" || len(id) > 100 || strings.TrimSpace(id) != id || strings.TrimSpace(content) == "" || len(content) > logMaxContentOverrideBytes || !utf8.ValidString(content) {
			return draft, logError("INVALID_INPUT", "消息正文覆盖无效")
		}
	}
	seen := map[string]bool{}
	for _, section := range draft.Sections {
		if !logUUIDPattern.MatchString(section.ID) || seen[section.ID] || len([]rune(section.Title)) > 200 || section.Order < 0 || int64(section.Order) > LogMaxVersion {
			return draft, logError("INVALID_INPUT", "章节标识或顺序无效")
		}
		seen[section.ID] = true
		s := section.Source
		if s.ChannelID == "" || len(s.ChannelID) > 100 || s.ToTime.IsZero() || (s.FromTime != nil && s.FromTime.After(s.ToTime)) || (s.FromExclusive && s.FromTime == nil) || s.IdentityIDs == nil || len(s.IdentityIDs) > 200 {
			return draft, logError("INVALID_INPUT", "来源范围无效")
		}
		identities := map[string]bool{}
		for _, id := range s.IdentityIDs {
			if id == "" || len(id) > 100 || identities[id] {
				return draft, logError("INVALID_INPUT", "角色筛选无效")
			}
			identities[id] = true
		}
	}
	if err := validateLogChapters(&draft); err != nil {
		return draft, err
	}
	if err := validateLogProjectAssetRegistry(&draft); err != nil {
		return draft, err
	}
	p := draft.Page
	m := p.MarginsMM
	for _, n := range []float64{p.WidthMM, p.HeightMM, m.Top, m.Right, m.Bottom, m.Left, p.GutterMM, p.FontSizePT, p.LineHeight} {
		if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
			return draft, logError("INVALID_INPUT", "页面数值无效")
		}
	}
	if (p.Preset != "a4" && p.Preset != "a5" && p.Preset != "custom") || p.WidthMM < 50 || p.WidthMM > 1000 || p.HeightMM < 50 || p.HeightMM > 1000 || m.Top+m.Bottom >= p.HeightMM || m.Left+m.Right+p.GutterMM >= p.WidthMM || (p.Columns != 1 && p.Columns != 2) || p.FontSizePT < 6 || p.FontSizePT > 72 || p.LineHeight < 1 || p.LineHeight > 3 || !logColorPattern.MatchString(p.BackgroundColor) {
		return draft, logError("INVALID_INPUT", "页面设置超出允许范围")
	}
	if draft.StylePackageID == "" && p.FontFamily != "sans-serif" && p.FontFamily != "serif" && p.FontFamily != "monospace" && !isLogPlatformFont(p.FontFamily) {
		return draft, logError("INVALID_INPUT", "字体未登记")
	}
	return draft, nil
}

// validateLogChapters keeps the chapter layer consistent with the global section order, so that
// publishing by section.order alone yields every chapter as one contiguous run in chapter order.
func validateLogChapters(draft *LogDraft) error {
	if len(draft.Chapters) == 0 {
		draft.Chapters = nil
		for _, section := range draft.Sections {
			if section.ChapterID != "" {
				return logError("INVALID_INPUT", "来源片段引用了不存在的章节")
			}
		}
		return nil
	}
	if len(draft.Chapters) > 200 {
		return logError("INVALID_INPUT", "章节数量超出上限")
	}
	sectionIDs := map[string]bool{}
	for _, section := range draft.Sections {
		sectionIDs[section.ID] = true
	}
	chapters := map[string]LogChapter{}
	chapterOrders := map[int]bool{}
	for _, c := range draft.Chapters {
		if !logUUIDPattern.MatchString(c.ID) || chapters[c.ID].ID != "" || sectionIDs[c.ID] || len([]rune(c.Title)) > 200 || c.Order < 0 || int64(c.Order) > LogMaxVersion || chapterOrders[c.Order] {
			return logError("INVALID_INPUT", "章节标识或顺序无效")
		}
		chapters[c.ID] = c
		chapterOrders[c.Order] = true
	}
	sectionOrders := map[int]bool{}
	for _, section := range draft.Sections {
		if _, ok := chapters[section.ChapterID]; !ok {
			return logError("INVALID_INPUT", "来源片段引用了不存在的章节")
		}
		if sectionOrders[section.Order] {
			return logError("INVALID_INPUT", "来源片段顺序重复")
		}
		sectionOrders[section.Order] = true
	}
	ordered := append([]LogSection(nil), draft.Sections...)
	sort.SliceStable(ordered, func(i, k int) bool { return ordered[i].Order < ordered[k].Order })
	finished := map[string]bool{}
	current := ""
	for _, section := range ordered {
		if section.ChapterID == current {
			continue
		}
		if finished[section.ChapterID] || (current != "" && chapters[section.ChapterID].Order < chapters[current].Order) {
			return logError("INVALID_INPUT", "章节内的来源片段必须连续且与章节顺序一致")
		}
		if current != "" {
			finished[current] = true
		}
		current = section.ChapterID
	}
	if current != "" {
		finished[current] = true
	}
	if len(finished) != len(chapters) {
		return logError("INVALID_INPUT", "章节至少需要一个来源片段")
	}
	return nil
}

// logSectionArrangement maps each section to its chapter and rank inside that chapter.
// Legacy sections are single-source chapters keyed by their own ID.
func logSectionArrangement(d *LogDraft) map[string][2]string {
	ordered := append([]LogSection(nil), d.Sections...)
	sort.SliceStable(ordered, func(i, k int) bool { return ordered[i].Order < ordered[k].Order })
	ranks := map[string]int{}
	out := map[string][2]string{}
	for _, s := range ordered {
		chapter := s.ChapterID
		if len(d.Chapters) == 0 {
			chapter = "section:" + s.ID
		}
		out[s.ID] = [2]string{chapter, strconv.Itoa(ranks[chapter])}
		ranks[chapter]++
	}
	return out
}

func loadLogProjectForDesign(id, userID string) (*model.LogProjectModel, bool, error) {
	return loadLogProjectForDesignAccess(id, userID, false)
}

func loadLogProjectForDraftSave(id, userID string) (*model.LogProjectModel, bool, error) {
	return loadLogProjectForDesignAccess(id, userID, true)
}

func loadLogProjectForDesignAccess(id, userID string, allowInvalidAdminSources bool) (*model.LogProjectModel, bool, error) {
	var p model.LogProjectModel
	if err := model.GetDB().Where("id = ? AND enabled = ?", id, true).Take(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, logError("NOT_FOUND", "工程不存在")
		}
		return nil, false, err
	}
	_, admin, err := logWorldAccess(p.WorldID, userID, false)
	if err != nil {
		return nil, false, err
	}
	if !allowInvalidAdminSources || !admin {
		if err := logCheckSources(&p, userID); err != nil {
			return nil, false, err
		}
	}
	return &p, admin, nil
}

func SaveLogDraft(id, userID string, input LogDraftUpdate) (*LogDraftResult, error) {
	p, admin, err := loadLogProjectForDraftSave(id, userID)
	if err != nil {
		return nil, err
	}
	if input.ExpectedVersion < 1 || input.ExpectedVersion >= LogMaxVersion {
		return nil, logError("INVALID_INPUT", "版本无效")
	}
	if p.DraftVersion != input.ExpectedVersion {
		return nil, logConflict(p.DraftVersion)
	}
	draft, err := parseLogDraft(input.Draft)
	if err != nil {
		return nil, err
	}
	if err := validateLogDraftStyle(&draft, userID); err != nil {
		return nil, err
	}
	var old LogDraft
	if err := json.Unmarshal([]byte(p.DraftJSON), &old); err != nil {
		return nil, err
	}
	if !admin {
		if len(old.Sections) != len(draft.Sections) {
			return nil, logError("PERMISSION_DENIED", "仅管理者可以更改来源")
		}
		oldSources := map[string]LogSourceSpec{}
		for _, s := range old.Sections {
			oldSources[s.ID] = s.Source
		}
		for _, s := range draft.Sections {
			source, ok := oldSources[s.ID]
			if !ok || !reflect.DeepEqual(source, s.Source) {
				return nil, logError("PERMISSION_DENIED", "仅管理者可以更改来源")
			}
		}
		// Members may rename chapters and move whole chapters, but not regroup or reorder sources inside them.
		if (len(old.Chapters) == 0) != (len(draft.Chapters) == 0) || !reflect.DeepEqual(logSectionArrangement(&old), logSectionArrangement(&draft)) {
			return nil, logError("PERMISSION_DENIED", "仅管理者可以编排来源片段")
		}
	}
	encoded, err := json.Marshal(draft)
	if err != nil {
		return nil, err
	}
	next := *p
	next.DraftJSON = string(encoded)
	if err := logCheckSources(&next, userID); err != nil {
		return nil, err
	}
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		r := tx.Model(&model.LogProjectModel{}).Where("id = ? AND enabled = ? AND draft_version = ?", id, true, input.ExpectedVersion).Updates(map[string]any{"draft_json": string(encoded), "draft_version": gorm.Expr("draft_version + 1")})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return logConflict(p.DraftVersion)
		}
		if err := claimLogProjectAssetRegistrations(tx, &old, &draft, id, userID); err != nil {
			return err
		}
		binding := model.LogProjectStyleBindingModel{ProjectID: id, PackageID: draft.StylePackageID, Version: draft.StylePackageVersion}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "project_id"}}, DoUpdates: clause.AssignmentColumns([]string{"package_id", "version"})}).Create(&binding).Error
	})
	if err != nil {
		return nil, err
	}
	logAudit("draft.save", userID, id, "", "OK", p.DraftVersion+1, int64(len(draft.Elements)))
	return &LogDraftResult{DraftVersion: p.DraftVersion + 1, Draft: draft}, nil
}
