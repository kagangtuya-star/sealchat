package service

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service/pdfruntime"
)

// Background LOG PDF export orchestrator. It reuses the existing MessageExportJob
// lifecycle (pending -> processing -> done/failed) and the LOG design projection;
// the browser only prints the shared renderer's output through a Render Session.

const (
	logPDFChunkAttempts = 2
	logPDFMaxJobTime    = 6 * time.Hour
	logPDFMergeTimeout  = 15 * time.Minute
	logPDFWindow        = 200
)

var platformFontFamilyRe = regexp.MustCompile(`^platform:([A-Za-z0-9_-]{1,100})$`)

// logPDFDeps are the external boundaries of one PDF job; tests replace them.
type logPDFDeps struct {
	renderer pdfruntime.PDFRenderer
	merger   pdfruntime.PDFMerger
	server   *logPDFRenderServer
	timeout  time.Duration
}

func logPDFFail(format string, args ...any) error {
	return &LogError{Code: "PDF_EXPORT_FAILED", Message: fmt.Sprintf(format, args...)}
}

// logPDFItem is one message as the design API delivers it (LogDesignMessage).
type logPDFItem struct {
	Message      *protocol.Message       `json:"message"`
	Version      int64                   `json:"version"`
	Capabilities map[string]bool         `json:"capabilities"`
	Presentation *LogMessagePresentation `json:"presentation,omitempty"`
}

type logPDFPayloadPart struct {
	SectionID         string       `json:"sectionId"`
	Slice             int          `json:"slice"`
	SliceCount        int          `json:"sliceCount"`
	Messages          []logPDFItem `json:"messages"`
	SectionMessageIDs []string     `json:"sectionMessageIds,omitempty"`
}

type logPDFPayloadChapterSection struct {
	SectionID  string   `json:"sectionId"`
	MessageIDs []string `json:"messageIds"`
}

type logPDFPayloadFont struct {
	ID       string                          `json:"id"`
	Meta     *model.PlatformFontAsset        `json:"meta,omitempty"`
	Manifest *PlatformFontSubsetManifestData `json:"manifest,omitempty"`
}

type logPDFPayload struct {
	Version         int                           `json:"version"`
	Chunk           logPDFPayloadChunk            `json:"chunk"`
	Draft           json.RawMessage               `json:"draft"`
	Style           *LogStylePackage              `json:"style"`
	Parts           []logPDFPayloadPart           `json:"parts"`
	ChapterSections []logPDFPayloadChapterSection `json:"chapterSections,omitempty"`
	TocPages        map[string]int                `json:"tocPages,omitempty"`
	Font            *logPDFPayloadFont            `json:"font,omitempty"`
}

type logPDFPayloadChunk struct {
	Index     int `json:"index"`
	Count     int `json:"count"`
	PageStart int `json:"pageStart"`
}

// logPDFReport is the surface's ready state.
type logPDFReport struct {
	Status        string          `json:"status"`
	PageCount     int             `json:"pageCount"`
	NodePages     map[string]int  `json:"nodePages"`
	UnresolvedToc []string        `json:"unresolvedToc"`
	MissingAssets int             `json:"missingAssets"`
	Warnings      []logPDFWarning `json:"warnings"`
}

type logPDFWarning struct {
	Code string `json:"code"`
}

type logPDFChunkState struct {
	payloadPath string
	pdfPath     string
	scope       *logPDFAssetScope
	pageStart   int
	pages       int
	unresolved  []string
	warnings    []logPDFWarning
}

// LogPDFExportWarnings returns the anchor pagination warnings for a completed PDF job.
func LogPDFExportWarnings(job *model.MessageExportJobModel) map[string]int {
	if job == nil || job.Format != exportFormatPDF || job.Status != model.MessageExportStatusDone {
		return nil
	}
	extra := parseExportExtraOptions(job.ExtraOptions)
	if extra.SourceKind != "log" {
		return nil
	}
	return extra.LogPDFWarnings
}

// logPDFSectionStream reads one section in design-window order through
// QueryLogMessages, so cursor validation, whisper visibility, quotes and
// presentations are exactly those of the workbench.
type logPDFSectionStream struct {
	projectID, sectionID, userID string
	expectVersion                int64
	cursor                       string
	done                         bool
	buf                          []logPDFStreamItem
}

type logPDFStreamItem struct {
	item    logPDFItem
	content string
	avatar  string
}

func (s *logPDFSectionStream) fill() error {
	if s.done {
		return nil
	}
	window, err := QueryLogMessages(s.projectID, s.sectionID, s.userID, s.cursor, "", logPDFWindow)
	if err != nil {
		return err
	}
	if window.SourceVersion != s.expectVersion {
		return logError("SOURCE_CHANGED", "来源或排版范围已变化，请重新导出")
	}
	channelData := window.Channel.ToProtocolType()
	for _, m := range window.Messages {
		s.buf = append(s.buf, logPDFStreamItem{
			item: logPDFItem{
				Message: LogDesignMessageData(m, channelData),
				Version: m.Version,
				// Print output never shows design controls; capabilities stay closed.
				Capabilities: map[string]bool{"edit": false, "changeIdentity": false, "reorder": false, "revoke": false, "remove": false, "editAppearance": false},
				Presentation: window.Presentations[m.ID],
			},
			content: m.Content,
			avatar:  m.SenderIdentityAvatarID,
		})
	}
	if window.NextCursor == nil {
		s.done = true
	} else {
		s.cursor = *window.NextCursor
	}
	return nil
}

// take returns the next n items (n < 0: everything left).
func (s *logPDFSectionStream) take(ctx context.Context, n int) ([]logPDFStreamItem, error) {
	for (n < 0 || len(s.buf) < n) && !s.done {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := s.fill(); err != nil {
			return nil, err
		}
	}
	if n < 0 || n > len(s.buf) {
		if n >= 0 && n > len(s.buf) {
			// The section shrank after planning (messages removed or hidden).
			return nil, logError("SOURCE_CHANGED", "来源或排版范围已变化，请重新导出")
		}
		n = len(s.buf)
	}
	out := s.buf[:n]
	s.buf = append([]logPDFStreamItem(nil), s.buf[n:]...)
	return out, nil
}

func logPDFDraftFontID(d *LogDraft) string {
	if m := platformFontFamilyRe.FindStringSubmatch(strings.TrimSpace(d.Page.FontFamily)); m != nil {
		return m[1]
	}
	return ""
}

func loadLogPDFFont(id string) *logPDFPayloadFont {
	if id == "" {
		return nil
	}
	item, err := PlatformFontGet(id)
	if err != nil || item == nil || item.Status != model.PlatformFontStatusReady {
		return &logPDFPayloadFont{ID: id}
	}
	font := &logPDFPayloadFont{ID: id, Meta: item}
	if item.DeliveryMode == model.PlatformFontDeliverySubset && strings.TrimSpace(item.ManifestObjectKey) != "" {
		if raw, _, err := readPlatformFontObject(item.ManifestStorageType, item.ManifestObjectKey, "application/json"); err == nil {
			var manifest PlatformFontSubsetManifestData
			if json.Unmarshal(raw, &manifest) == nil {
				font.Manifest = &manifest
			}
		}
	}
	return font
}

type logPDFSectionPlanInput struct {
	section LogSection
	version int64
	count   int
}

func logPDFSectionChapterKey(d *LogDraft, s LogSection) string {
	if len(d.Chapters) > 0 && s.ChapterID != "" {
		return s.ChapterID
	}
	return "section:" + s.ID
}

func logPDFPartContainsElement(part logPDFPart, e LogElement, fullIDs map[string][]string) bool {
	if e.Anchor.SectionID != part.SectionID {
		return false
	}
	switch e.Anchor.Kind {
	case "sectionStart":
		return part.Slice == 0
	case "sectionEnd":
		return part.Slice == part.SliceCount-1
	case "message":
		ids, ok := fullIDs[part.SectionID]
		if !ok {
			return false
		}
		for i, id := range ids {
			if id == e.Anchor.MessageID {
				return i >= part.Start && i < part.End
			}
		}
	}
	return false
}

func logPDFChunkTOCChapters(d *LogDraft, plan logPDFChunkPlan, inputs map[string]logPDFSectionPlanInput, fullIDs map[string][]string, elements []LogElement) map[string]bool {
	keys := map[string]bool{}
	for _, e := range elements {
		if e.Type != "toc" {
			continue
		}
		for _, part := range plan.Parts {
			if !logPDFPartContainsElement(part, e, fullIDs) {
				continue
			}
			in := inputs[part.SectionID]
			keys[logPDFSectionChapterKey(d, in.section)] = true
			break
		}
	}
	return keys
}

// planLogPDFJob counts sources and builds the chunk plan for a draft.
func planLogPDFJob(ctx context.Context, p *model.LogProjectModel, d *LogDraft, userID string) ([]logPDFChunkPlan, map[string]logPDFSectionPlanInput, map[string][]string, error) {
	elements, err := logDraftElements(d)
	if err != nil {
		return nil, nil, nil, err
	}
	inputs := map[string]logPDFSectionPlanInput{}
	costs := []logPDFSectionCost{}
	fullIDs := map[string][]string{}
	total := 0
	for _, s := range d.Sections {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		version, err := logSourceVersion(s.Source.ChannelID)
		if err != nil {
			return nil, nil, nil, err
		}
		var count int64
		if err := logSourceQuery(p, s, userID).Count(&count).Error; err != nil {
			return nil, nil, nil, err
		}
		inputs[s.ID] = logPDFSectionPlanInput{section: s, version: version, count: int(count)}
		fixed, anchored := 0, map[string]int{}
		for _, e := range elements {
			if e.Anchor.SectionID != s.ID {
				continue
			}
			if e.Anchor.Kind == "message" {
				anchored[e.Anchor.MessageID]++
			} else {
				fixed++
			}
		}
		chapter := logPDFSectionChapterKey(d, s)
		cost := logPDFSectionCost{ID: s.ID, ChapterKey: chapter, Messages: int(count), Fixed: fixed}
		elementsTotal := fixed
		for _, n := range anchored {
			elementsTotal += n
		}
		if int(count)+elementsTotal > logPDFChunkMax {
			// Only oversize sections need per-message metadata for boundary selection.
			var rows []struct {
				ID               string
				UserID           string
				SenderIdentityID string
			}
			if err := logSourceQuery(p, s, userID).Select("id, user_id, sender_identity_id").
				Order("display_order ASC, created_at ASC, id ASC").Find(&rows).Error; err != nil {
				return nil, nil, nil, err
			}
			ids := make([]string, len(rows))
			speakers := make([]string, len(rows))
			weights := make([]int, len(rows))
			for i, r := range rows {
				ids[i] = r.ID
				speakers[i] = r.SenderIdentityID + "|" + r.UserID
				weights[i] = 1 + anchored[r.ID]
			}
			cost.Messages = len(rows)
			cost.Weights = weights
			cost.Scores = logPDFCutScores(speakers, ids, elements, s.ID)
			fullIDs[s.ID] = ids
			in := inputs[s.ID]
			in.count = len(rows)
			inputs[s.ID] = in
		} else {
			cost.Fixed = elementsTotal
		}
		total += cost.total()
		costs = append(costs, cost)
	}
	if total > logPDFMaxNodes {
		return nil, nil, nil, logPDFFail("LOG 内容过多（约 %d 个排版节点，上限 %d），请拆分后分别导出 PDF", total, logPDFMaxNodes)
	}
	chunks := planLogPDFChunks(costs, logPDFChunkTarget, logPDFChunkMax)
	if len(chunks) > logPDFMaxChunks {
		return nil, nil, nil, logPDFFail("LOG 分片过多（%d），请拆分后分别导出 PDF", len(chunks))
	}
	if len(chunks) == 0 {
		return nil, nil, nil, logPDFFail("LOG 没有可导出的内容")
	}
	// A visual chapter can span multiple Chromium chunks. Only chapters that
	// actually contain a TOC need complete message order for cross-chunk entries.
	chapterChunks := map[string]map[int]struct{}{}
	for chunkIndex, chunk := range chunks {
		for _, part := range chunk.Parts {
			in := inputs[part.SectionID]
			key := logPDFSectionChapterKey(d, in.section)
			set := chapterChunks[key]
			if set == nil {
				set = map[int]struct{}{}
				chapterChunks[key] = set
			}
			set[chunkIndex] = struct{}{}
		}
	}
	tocChapters := map[string]bool{}
	for _, e := range elements {
		if e.Type != "toc" {
			continue
		}
		if in, ok := inputs[e.Anchor.SectionID]; ok {
			tocChapters[logPDFSectionChapterKey(d, in.section)] = true
		}
	}
	for _, s := range d.Sections {
		key := logPDFSectionChapterKey(d, s)
		if len(chapterChunks[key]) < 2 || !tocChapters[key] {
			continue
		}
		if _, ok := fullIDs[s.ID]; ok {
			continue
		}
		var rows []struct{ ID string }
		if err := logSourceQuery(p, s, userID).Select("id").Order("display_order ASC, created_at ASC, id ASC").Find(&rows).Error; err != nil {
			return nil, nil, nil, err
		}
		ids := make([]string, len(rows))
		for i, row := range rows {
			ids[i] = row.ID
		}
		fullIDs[s.ID] = ids
	}
	return chunks, inputs, fullIDs, nil
}

func logPDFChunkName(i int) string { return fmt.Sprintf("%04d", i+1) }

// processLogPDFExportJob renders, merges and atomically publishes a LOG PDF.
func processLogPDFExportJob(ctx context.Context, job *model.MessageExportJobModel, cfg MessageExportWorkerConfig, extra *exportExtraOptions) error {
	m := LogPDFRuntime()
	var deps logPDFDeps
	if m != nil {
		if r, _, ok := m.Renderer(); ok {
			deps.renderer = r
		}
		if mg, _, ok := m.Merger(); ok {
			deps.merger = mg
		}
		deps.timeout = m.Config().Timeout()
	}
	if deps.renderer == nil {
		reason := "服务器未检测到 PDF 渲染器"
		if m != nil {
			if s := m.Status().ErrorSummary(); s != "" {
				reason = s
			}
		}
		err := logPDFFail("%s，请改用浏览器打印", reason)
		_ = markJobFailed(job, err)
		return err
	}
	deps.server = logPDFServer
	return runLogPDFExportJob(ctx, job, cfg, extra, deps)
}

func runLogPDFExportJob(parent context.Context, job *model.MessageExportJobModel, cfg MessageExportWorkerConfig, extra *exportExtraOptions, deps logPDFDeps) (retErr error) {
	workDir := filepath.Join(cfg.StorageDir, ".pdf-"+job.ID)
	defer func() {
		deps.server.closeJob(job.ID)
		_ = os.RemoveAll(workDir)
		if retErr != nil {
			_ = markJobFailed(job, retErr)
		}
	}()
	if !logPDFSurfaceAvailable() {
		return logPDFFail("当前程序未包含 PDF 渲染页面，请使用完整构建")
	}
	if deps.timeout <= 0 {
		deps.timeout = 2 * time.Minute
	}
	p, err := loadLogProjectForExport(job.UserID, extra)
	if err != nil {
		return err
	}
	var draft LogDraft
	if err := json.Unmarshal([]byte(p.DraftJSON), &draft); err != nil {
		return err
	}
	sort.SliceStable(draft.Sections, func(i, j int) bool { return draft.Sections[i].Order < draft.Sections[j].Order })
	var style *LogStylePackage
	if draft.StylePackageID != "" {
		if style, err = GetLogStyle(draft.StylePackageID, draft.StylePackageVersion, job.UserID); err != nil {
			return err
		}
	}
	font := loadLogPDFFont(logPDFDraftFontID(&draft))

	_ = os.RemoveAll(workDir)
	chunkDir := filepath.Join(workDir, "chunks")
	profileDir := filepath.Join(workDir, "profiles")
	for _, dir := range []string{chunkDir, profileDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}

	planCtx, cancelPlan := context.WithTimeout(parent, 10*time.Minute)
	chunks, inputs, fullIDs, err := planLogPDFJob(planCtx, p, &draft, job.UserID)
	cancelPlan()
	if err != nil {
		return err
	}
	draftElements, err := logDraftElements(&draft)
	if err != nil {
		return err
	}
	budget := time.Duration(len(chunks)*(logPDFChunkAttempts+1)+2) * deps.timeout
	if budget > logPDFMaxJobTime {
		budget = logPDFMaxJobTime
	}
	ctx, cancel := context.WithTimeout(parent, budget)
	defer cancel()

	streams := map[string]*logPDFSectionStream{}
	states := make([]*logPDFChunkState, len(chunks))
	globalPages := map[string]int{}
	pageStart := 1
	projectAssets := logProjectAssetIDs(&draft)
	for i, plan := range chunks {
		payload := logPDFPayload{
			Version: 1,
			Chunk:   logPDFPayloadChunk{Index: i, Count: len(chunks), PageStart: pageStart},
			Draft:   json.RawMessage(p.DraftJSON),
			Style:   style,
			Font:    font,
		}
		tocChapters := logPDFChunkTOCChapters(&draft, plan, inputs, fullIDs, draftElements)
		for _, section := range draft.Sections {
			if !tocChapters[logPDFSectionChapterKey(&draft, section)] {
				continue
			}
			if ids, ok := fullIDs[section.ID]; ok {
				payload.ChapterSections = append(payload.ChapterSections, logPDFPayloadChapterSection{SectionID: section.ID, MessageIDs: ids})
			}
		}
		scope := &logPDFAssetScope{
			ProjectID: p.ID, UserID: job.UserID, SectionMessages: map[string]map[string]bool{},
			ProjectAssets: projectAssets,
		}
		if style != nil {
			scope.StyleID, scope.StyleVersion = style.ID, style.Version
		}
		if font != nil && font.Meta != nil {
			scope.FontID, scope.FontItem, scope.FontManifest = font.ID, font.Meta, font.Manifest
		}
		for _, part := range plan.Parts {
			in := inputs[part.SectionID]
			stream := streams[part.SectionID]
			if stream == nil {
				stream = &logPDFSectionStream{projectID: p.ID, sectionID: part.SectionID, userID: job.UserID, expectVersion: in.version}
				streams[part.SectionID] = stream
			}
			n := part.End - part.Start
			if part.Slice == part.SliceCount-1 {
				n = -1
			}
			items, err := stream.take(ctx, n)
			if err != nil {
				return err
			}
			if n < 0 && part.SliceCount > 1 && len(items) != part.End-part.Start {
				return logError("SOURCE_CHANGED", "来源或排版范围已变化，请重新导出")
			}
			pp := logPDFPayloadPart{SectionID: part.SectionID, Slice: part.Slice, SliceCount: part.SliceCount, Messages: make([]logPDFItem, 0, len(items))}
			if part.SliceCount > 1 {
				pp.SectionMessageIDs = fullIDs[part.SectionID]
			}
			ids := scope.SectionMessages[part.SectionID]
			if ids == nil {
				ids = map[string]bool{}
				scope.SectionMessages[part.SectionID] = ids
			}
			for _, it := range items {
				pp.Messages = append(pp.Messages, it.item)
				ids[it.item.Message.ID] = true
				scope.MessageText = append(scope.MessageText, it.content)
				if it.avatar != "" {
					scope.MessageText = append(scope.MessageText, it.avatar)
				}
			}
			payload.Parts = append(payload.Parts, pp)
		}
		state := &logPDFChunkState{
			payloadPath: filepath.Join(chunkDir, logPDFChunkName(i)+".json"),
			pdfPath:     filepath.Join(chunkDir, logPDFChunkName(i)+".pdf"),
			scope:       scope,
			pageStart:   pageStart,
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if err := os.WriteFile(state.payloadPath, raw, 0o600); err != nil {
			return err
		}
		report, err := renderLogPDFChunk(ctx, deps, job, extra.LogDraftVersion, i, len(chunks), raw, state, profileDir)
		if err != nil {
			return err
		}
		state.pages = report.PageCount
		state.unresolved = report.UnresolvedToc
		state.warnings = report.Warnings
		for id, page := range report.NodePages {
			if _, ok := globalPages[id]; !ok {
				globalPages[id] = page
			}
		}
		states[i] = state
		pageStart += report.PageCount
	}

	// Second pass: chunks whose TOC points into other chunks are re-rendered with
	// the now-known global page numbers. A page count change would shift every
	// later page, so it fails explicitly instead of publishing wrong numbers.
	for i, state := range states {
		overrides := map[string]int{}
		for _, id := range state.unresolved {
			if page, ok := globalPages[id]; ok {
				overrides[id] = page
			}
		}
		if len(overrides) == 0 {
			continue
		}
		raw, err := os.ReadFile(state.payloadPath)
		if err != nil {
			return err
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(raw, &payload); err != nil {
			return err
		}
		payload["tocPages"], _ = json.Marshal(overrides)
		raw, err = json.Marshal(payload)
		if err != nil {
			return err
		}
		report, err := renderLogPDFChunk(ctx, deps, job, extra.LogDraftVersion, i, len(chunks), raw, state, profileDir)
		if err != nil {
			return err
		}
		if report.PageCount != state.pages {
			return logPDFFail("目录页码回填后第 %d 段页数发生变化，无法保证页码正确", i+1)
		}
		state.warnings = report.Warnings
	}

	// Edits saved while rendering must not be published as this version.
	if err := validateLogPDFSnapshot(ctx, p.ID, extra.LogDraftVersion, inputs); err != nil {
		return err
	}

	ext := "pdf"
	var finalTmp string
	switch {
	case len(states) == 1:
		finalTmp = states[0].pdfPath
	case deps.merger != nil:
		finalTmp = filepath.Join(workDir, "final.pdf.tmp")
		inputs := make([]string, len(states))
		for i, st := range states {
			abs, err := filepath.Abs(st.pdfPath)
			if err != nil {
				return err
			}
			inputs[i] = abs
		}
		absOut, err := filepath.Abs(finalTmp)
		if err != nil {
			return err
		}
		mctx, mcancel := context.WithTimeout(ctx, logPDFMergeTimeout)
		err = deps.merger.Merge(mctx, inputs, absOut)
		mcancel()
		if err != nil {
			return logPDFFail("PDF 合并失败：%s", truncateLogPDFError(err))
		}
	default:
		// No merger: never pretend a complete PDF exists; ship ordered parts instead.
		ext = "zip"
		finalTmp = filepath.Join(workDir, "final.zip.tmp")
		if err := writeLogPDFPartsArchive(finalTmp, job.DisplayName, states); err != nil {
			return err
		}
	}
	if ext == "pdf" {
		if err := pdfruntime.ValidatePDFFile(finalTmp); err != nil {
			return logPDFFail("最终 PDF 校验失败：%v", err)
		}
	}
	extra.LogPDFWarnings = make(map[string]int)
	for _, state := range states {
		for _, warning := range state.warnings {
			switch warning.Code {
			case "ANCHOR_GROUP_BREAK", "ANCHOR_GROUP_OVERSIZE":
				extra.LogPDFWarnings[warning.Code]++
			}
		}
	}
	encodedExtra, err := json.Marshal(extra)
	if err != nil {
		return err
	}
	completedExtra := string(encodedExtra)
	// Merging/archiving can also take time; recheck the whole snapshot immediately before publish.
	if err := validateLogPDFSnapshot(ctx, p.ID, extra.LogDraftVersion, inputs); err != nil {
		return err
	}
	finalPath := filepath.Join(cfg.StorageDir, fmt.Sprintf("%s.%s", job.ID, ext))
	if err := os.Rename(finalTmp, finalPath); err != nil {
		return err
	}
	fileName := BuildExportResultFileName(job.DisplayName, job.ID, ext, time.Now())
	if err := markJobDoneWithExtraOptions(job, finalPath, fileName, &completedExtra); err != nil {
		_ = os.Remove(finalPath)
		return err
	}
	return nil
}

func validateLogPDFSnapshot(ctx context.Context, projectID string, draftVersion int64, inputs map[string]logPDFSectionPlanInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var current model.LogProjectModel
	if err := model.GetDB().Select("id, draft_version, enabled").Where("id = ?", projectID).Take(&current).Error; err != nil {
		return err
	}
	if current.DraftVersion != draftVersion || !current.Enabled {
		return logExportConflict(current.DraftVersion)
	}
	return validateLogPDFSources(ctx, inputs)
}

func validateLogPDFSources(ctx context.Context, inputs map[string]logPDFSectionPlanInput) error {
	versions := map[string]int64{}
	for _, in := range inputs {
		if err := ctx.Err(); err != nil {
			return err
		}
		channelID := in.section.Source.ChannelID
		version, ok := versions[channelID]
		if !ok {
			var err error
			version, err = logSourceVersion(channelID)
			if err != nil {
				return err
			}
			versions[channelID] = version
		}
		if version != in.version {
			return logError("SOURCE_CHANGED", "来源或排版范围已变化，请重新导出")
		}
	}
	return nil
}

func truncateLogPDFError(err error) string {
	msg := err.Error()
	if r := []rune(msg); len(r) > 300 {
		msg = string(r[:300]) + "…"
	}
	return msg
}

// renderLogPDFChunk renders one chunk through a fresh Render Session, retrying a
// limited number of times. Output is written to a .tmp file, validated, then
// renamed into the chunk path.
func renderLogPDFChunk(ctx context.Context, deps logPDFDeps, job *model.MessageExportJobModel, draftVersion int64, index, count int, payload []byte, state *logPDFChunkState, profileDir string) (*logPDFReport, error) {
	var lastErr error
	for attempt := 0; attempt < logPDFChunkAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, logPDFFail("PDF 任务超时")
		}
		report, err := renderLogPDFChunkOnce(ctx, deps, job, draftVersion, index, payload, state, profileDir)
		if err == nil {
			return report, nil
		}
		lastErr = err
		log.Printf("export: PDF 任务 %s 第 %d/%d 段第 %d 次渲染失败: %v", job.ID, index+1, count, attempt+1, err)
	}
	return nil, logPDFFail("PDF 渲染失败（第 %d/%d 段）：%s", index+1, count, truncateLogPDFError(lastErr))
}

func renderLogPDFChunkOnce(ctx context.Context, deps logPDFDeps, job *model.MessageExportJobModel, draftVersion int64, index int, payload []byte, state *logPDFChunkState, profileDir string) (*logPDFReport, error) {
	sess, url, err := deps.server.open(job.ID, index, draftVersion, payload, state.scope, deps.timeout+time.Minute)
	if err != nil {
		return nil, err
	}
	defer deps.server.close(sess)
	tmp := state.pdfPath + ".tmp"
	_ = os.Remove(tmp)
	defer os.Remove(tmp)
	res, err := deps.renderer.Render(ctx, pdfruntime.RenderRequest{URL: url, OutputPath: tmp, WorkDir: profileDir, Timeout: deps.timeout})
	if err != nil {
		return nil, err
	}
	var report logPDFReport
	if res == nil || json.Unmarshal(res.State, &report) != nil || report.Status != "ready" || report.PageCount < 1 {
		return nil, errors.New("渲染页面没有报告有效页数")
	}
	if err := pdfruntime.ValidatePDFFile(tmp); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, state.pdfPath); err != nil {
		return nil, err
	}
	return &report, nil
}

func writeLogPDFPartsArchive(path, displayName string, states []*logPDFChunkState) (retErr error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); retErr == nil {
			retErr = cerr
		}
		if retErr != nil {
			_ = os.Remove(path)
		}
	}()
	base := sanitizeFileName(displayName)
	if base == "" {
		base = "LOG"
	}
	zw := zip.NewWriter(f)
	for i, st := range states {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("%s-第%04d段.pdf", base, i+1), Method: zip.Store, Modified: time.Now()})
		if err != nil {
			return err
		}
		in, err := os.Open(st.pdfPath)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, in)
		in.Close()
		if err != nil {
			return err
		}
	}
	readme, err := zw.Create("README.txt")
	if err != nil {
		return err
	}
	_, _ = readme.Write([]byte("服务器缺少 PDF 合并工具（pdfunite 或 qpdf），因此按顺序输出了分段 PDF。\n页码在各段之间连续；安装合并工具后重新导出即可得到单个 PDF。\n"))
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Sync()
}

// recoverInterruptedExportJobs fails jobs left in processing by a previous
// process (the worker is single-instance) and removes stale PDF work dirs, so
// tasks never stay stuck after a crash or restart.
func recoverInterruptedExportJobs(storageDir string, startedBefore time.Time) {
	res := model.GetDB().Model(&model.MessageExportJobModel{}).
		Where("status = ? AND updated_at < ?", model.MessageExportStatusProcessing, startedBefore).
		Updates(map[string]any{
			"status":      model.MessageExportStatusFailed,
			"error_msg":   "服务重启导致任务中断，请重试",
			"finished_at": time.Now(),
		})
	if res.Error != nil {
		log.Printf("export: 恢复中断任务失败: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("export: 已将 %d 个中断的导出任务标记为失败", res.RowsAffected)
	}
	entries, err := os.ReadDir(storageDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		// .pdf-probe belongs to the runtime prober, which cleans up after itself.
		if e.IsDir() && strings.HasPrefix(e.Name(), ".pdf-") && e.Name() != ".pdf-probe" {
			_ = os.RemoveAll(filepath.Join(storageDir, e.Name()))
		}
	}
}
