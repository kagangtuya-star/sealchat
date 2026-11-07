package service

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/service/textfmt"
)

const (
	messageExportLimit = 65535
	defaultExportTZ    = "2006-01-02 15:04"

	// 导出切片与并发控制的全局限制，需与前端/接口保持一致
	DefaultExportSliceLimit = 5000
	MinExportSliceLimit     = 1000
	MaxExportSliceLimit     = 20000

	DefaultExportConcurrency = 2
	MinExportConcurrency     = 1
	MaxExportConcurrency     = 8

	exportListDefaultLimit = 20
	exportListMaxLimit     = 100
)

var supportedExportFormats = map[string]struct{}{
	"json": {},
	"txt":  {},
	"html": {},
	"docx": {},
}

// ExportJobOptions 聚合创建导出任务所需的信息。
type ExportJobOptions struct {
	UserID                    string
	ChannelID                 string
	SourceKind                string
	LogProjectID              string
	LogDraftVersion           int64
	Format                    string
	DisplayName               string
	IncludeOOC                bool
	IncludeArchived           bool
	IncludeImages             bool
	IncludeDiceCommand        bool
	WithoutTimestamp          bool
	WithoutOOCParentheses     bool
	MergeMessages             bool
	AutoCorrectPunctuation    bool
	StartTime                 *time.Time
	EndTime                   *time.Time
	DisplaySettings           map[string]any
	SliceLimit                int
	MaxConcurrency            int
	TextColorizeBBCode        bool
	TextColorizeBBCodeMap     map[string]string
	TextColorizeBBCodeNameMap map[string]string
	visibilityByChannel       map[string]bool
}

type exportExtraOptions struct {
	SourceKind                string            `json:"source_kind,omitempty"`
	LogProjectID              string            `json:"log_project_id,omitempty"`
	LogDraftVersion           int64             `json:"log_draft_version,omitempty"`
	DisplaySettings           map[string]any    `json:"display,omitempty"`
	SliceLimit                int               `json:"slice_limit,omitempty"`
	MaxConcurrency            int               `json:"max_concurrency,omitempty"`
	TextColorizeBBCode        bool              `json:"text_colorize_bbcode,omitempty"`
	TextColorizeBBCodeMap     map[string]string `json:"text_colorize_bbcode_map,omitempty"`
	TextColorizeBBCodeNameMap map[string]string `json:"text_colorize_bbcode_name_map,omitempty"`
	AutoCorrectPunctuation    bool              `json:"auto_correct_punctuation"`
	IncludeImages             bool              `json:"include_images"`
	IncludeDiceCommand        bool              `json:"include_dice_commands"`
	WithoutOOCParentheses     bool              `json:"without_ooc_parentheses,omitempty"`
	BatchChannelIDs           []string          `json:"batch_channel_ids,omitempty"`
	BatchFormat               string            `json:"batch_format,omitempty"`
	VisibilityByChannel       map[string]bool   `json:"visibility_by_channel,omitempty"`
}

// exportFormatPDF is only valid for LOG sources (background Chromium render);
// it is intentionally absent from supportedExportFormats so channel and batch
// exports cannot request it.
const exportFormatPDF = "pdf"

func normalizeExportFormat(format string) (string, bool) {
	f := strings.ToLower(strings.TrimSpace(format))
	_, ok := supportedExportFormats[f]
	return f, ok
}

// CreateMessageExportJob 持久化导出任务并返回记录。
func CreateMessageExportJob(opts *ExportJobOptions) (*model.MessageExportJobModel, error) {
	if opts == nil {
		return nil, fmt.Errorf("导出参数不能为空")
	}
	format, ok := normalizeExportFormat(opts.Format)
	if !ok && opts.SourceKind == "log" && strings.EqualFold(strings.TrimSpace(opts.Format), exportFormatPDF) {
		if c := GetLogPDFCapability(); !c.Available {
			return nil, logError("PDF_UNAVAILABLE", firstNonEmpty(c.Reason, "服务器未检测到 PDF 渲染器")+"，请改用浏览器打印")
		}
		format, ok = exportFormatPDF, true
	}
	if !ok {
		return nil, fmt.Errorf("不支持的导出格式: %s", opts.Format)
	}

	if err := snapshotExportVisibility(opts, []string{opts.ChannelID}); err != nil {
		return nil, err
	}
	opts.SliceLimit = NormalizeExportSliceLimit(opts.SliceLimit)
	opts.MaxConcurrency = NormalizeExportConcurrency(opts.MaxConcurrency)
	extraOptions, err := buildExportExtraOptions(opts)
	if err != nil {
		return nil, err
	}

	job := &model.MessageExportJobModel{
		UserID:           opts.UserID,
		ChannelID:        opts.ChannelID,
		Format:           format,
		DisplayName:      normalizeExportDisplayName(opts.DisplayName),
		IncludeOOC:       opts.IncludeOOC,
		IncludeArchived:  opts.IncludeArchived,
		WithoutTimestamp: opts.WithoutTimestamp,
		MergeMessages:    opts.MergeMessages,
		StartTime:        opts.StartTime,
		EndTime:          opts.EndTime,
		Status:           model.MessageExportStatusPending,
		ExtraOptions:     extraOptions,
	}

	if err := model.GetDB().Create(job).Error; err != nil {
		return nil, err
	}
	return job, nil
}

// CreateBatchMessageExportJob 创建汇总多个频道结果的 ZIP 导出任务。
func CreateBatchMessageExportJob(opts *ExportJobOptions, channelIDs []string) (*model.MessageExportJobModel, error) {
	if opts == nil {
		return nil, fmt.Errorf("导出参数不能为空")
	}
	format, ok := normalizeExportFormat(opts.Format)
	if !ok {
		return nil, fmt.Errorf("不支持的导出格式: %s", opts.Format)
	}
	seen := make(map[string]struct{}, len(channelIDs))
	normalizedIDs := make([]string, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		channelID = strings.TrimSpace(channelID)
		if channelID == "" {
			continue
		}
		if _, exists := seen[channelID]; exists {
			continue
		}
		seen[channelID] = struct{}{}
		normalizedIDs = append(normalizedIDs, channelID)
	}
	if len(normalizedIDs) == 0 {
		return nil, fmt.Errorf("至少选择一个频道")
	}
	if err := snapshotExportVisibility(opts, append([]string{opts.ChannelID}, normalizedIDs...)); err != nil {
		return nil, err
	}

	opts.SliceLimit = NormalizeExportSliceLimit(opts.SliceLimit)
	opts.MaxConcurrency = NormalizeExportConcurrency(opts.MaxConcurrency)
	extraOptions, err := buildExportExtraOptions(opts)
	if err != nil {
		return nil, err
	}
	extra := parseExportExtraOptions(extraOptions)
	extra.BatchChannelIDs = normalizedIDs
	extra.BatchFormat = format
	encodedExtraOptions, err := json.Marshal(extra)
	if err != nil {
		return nil, fmt.Errorf("导出附加参数序列化失败: %w", err)
	}
	job := &model.MessageExportJobModel{
		UserID:           opts.UserID,
		ChannelID:        opts.ChannelID,
		Format:           "zip",
		DisplayName:      normalizeExportDisplayName(opts.DisplayName),
		IncludeOOC:       opts.IncludeOOC,
		IncludeArchived:  opts.IncludeArchived,
		WithoutTimestamp: opts.WithoutTimestamp,
		MergeMessages:    opts.MergeMessages,
		StartTime:        opts.StartTime,
		EndTime:          opts.EndTime,
		Status:           model.MessageExportStatusPending,
		ExtraOptions:     string(encodedExtraOptions),
	}
	if err := model.GetDB().Create(job).Error; err != nil {
		return nil, err
	}
	return job, nil
}

// GetMessageExportJob 获取任务详情。
func GetMessageExportJob(jobID string) (*model.MessageExportJobModel, error) {
	if strings.TrimSpace(jobID) == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var job model.MessageExportJobModel
	if err := model.GetDB().Where("id = ?", jobID).Limit(1).Find(&job).Error; err != nil {
		return nil, err
	}
	if job.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &job, nil
}

func loadMessagesForExport(job *model.MessageExportJobModel) ([]*model.MessageModel, error) {
	if job == nil {
		return nil, fmt.Errorf("任务不存在")
	}
	db := model.GetDB()
	query := db.Model(&model.MessageModel{}).
		Where("channel_id = ?", job.ChannelID).
		Where("is_revoked = ?", false).
		Where("is_deleted = ?", false).
		Preload("Member").
		Preload("User")
	extra := parseExportExtraOptions(job.ExtraOptions)
	filter, hasSnapshot := extra.VisibilityByChannel[job.ChannelID]
	if !hasSnapshot {
		filter = len(job.ChannelID) < 30
	}
	query = ApplyWhisperVisibilityFilterWithReadAll(query, job.UserID, !filter)

	if job.StartTime != nil {
		query = query.Where("created_at >= ?", *job.StartTime)
	}
	if job.EndTime != nil {
		query = query.Where("created_at <= ?", *job.EndTime)
	}
	if !job.IncludeArchived {
		query = query.Where("is_archived = ?", false)
	}
	if !job.IncludeOOC && !job.MergeMessages {
		query = query.Where("COALESCE(ic_mode, 'ic') != ?", "ooc")
	}

	query = query.Order("display_order asc").Order("created_at asc").Limit(messageExportLimit)

	var messages []*model.MessageModel
	if err := query.Find(&messages).Error; err != nil {
		return nil, err
	}
	if err := hydrateWhisperTargetsForExport(messages); err != nil {
		return nil, err
	}
	return prepareMessagesForExport(messages, job, extra), nil
}

func prepareMessagesForExport(messages []*model.MessageModel, job *model.MessageExportJobModel, extra *exportExtraOptions) []*model.MessageModel {
	messages = normalizeMessagesForExportPunctuation(messages, extra.AutoCorrectPunctuation)
	if job.MergeMessages {
		return mergeSequentialMessagesForExport(messages, extra, job.IncludeOOC)
	}
	return filterMessagesForExport(messages, extra, job.IncludeOOC)
}

func normalizeMessagesForExportPunctuation(messages []*model.MessageModel, enabled bool) []*model.MessageModel {
	if !enabled || len(messages) == 0 {
		return messages
	}
	normalized := make([]*model.MessageModel, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			normalized = append(normalized, nil)
			continue
		}
		clone := cloneMessage(msg)
		if shouldNormalizeExportPunctuation(msg) {
			clone.Content = textfmt.NormalizeChinesePunctuation(msg.Content)
		}
		normalized = append(normalized, clone)
	}
	return normalized
}

func shouldNormalizeExportPunctuation(msg *model.MessageModel) bool {
	// MessageModel has no explicit type. Plain user text whitelist: persisted user
	// sender, no widget/dice payload, no TipTap document, no command syntax.
	if msg == nil || strings.TrimSpace(msg.UserID) == "" || (msg.User != nil && msg.User.IsBot) ||
		strings.TrimSpace(msg.WidgetData) != "" || strings.TrimSpace(msg.DiceVisualJSON) != "" || LooksLikeTipTapJSON(msg.Content) {
		return false
	}
	plainContent := buildFilteredPlainContent(msg.Content, true)
	return !shouldDisableInlineCodeForBotCommand(msg.Content) && !isSingleLineDiceCommand(plainContent)
}

func filterMessagesForExport(messages []*model.MessageModel, extra *exportExtraOptions, includeOOC bool) []*model.MessageModel {
	if len(messages) == 0 {
		return messages
	}
	includeImages := true
	includeDiceCommand := true
	if extra != nil {
		includeImages = extra.IncludeImages
		includeDiceCommand = extra.IncludeDiceCommand
	}
	filtered := make([]*model.MessageModel, 0, len(messages))
	for _, msg := range messages {
		if classifyExportMessage(msg, includeImages, includeDiceCommand, includeOOC).Skip {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

type exportMessageFilterResult struct {
	Skip       bool
	SkippedOOC bool
}

func classifyExportMessage(msg *model.MessageModel, includeImages, includeDiceCommand, includeOOC bool) exportMessageFilterResult {
	if msg == nil {
		return exportMessageFilterResult{Skip: true}
	}
	if !includeOOC && strings.EqualFold(normalizeIcMode(msg.ICMode), "ooc") {
		return exportMessageFilterResult{Skip: true, SkippedOOC: true}
	}
	plainContent := buildFilteredPlainContent(msg.Content, includeImages)
	if plainContent == "" {
		return exportMessageFilterResult{Skip: true}
	}
	isBotMessage := msg.User != nil && msg.User.IsBot
	if !includeDiceCommand && !isBotMessage && isSingleLineDiceCommand(plainContent) {
		return exportMessageFilterResult{Skip: true}
	}
	return exportMessageFilterResult{}
}

func hydrateWhisperTargetsForExport(messages []*model.MessageModel) error {
	if len(messages) == 0 {
		return nil
	}

	whisperMsgIDs := make([]string, 0, len(messages))
	for _, msg := range messages {
		if msg == nil || !msg.IsWhisper {
			continue
		}
		if id := strings.TrimSpace(msg.ID); id != "" {
			whisperMsgIDs = append(whisperMsgIDs, id)
		}
	}
	if len(whisperMsgIDs) == 0 {
		return nil
	}

	recipientMap := model.GetWhisperRecipientIDsBatch(whisperMsgIDs)
	userIDSet := make(map[string]struct{})
	addUserID := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		userIDSet[id] = struct{}{}
	}
	for _, msg := range messages {
		if msg == nil || !msg.IsWhisper {
			continue
		}
		for _, id := range splitWhisperIDs(msg.WhisperTo) {
			addUserID(id)
		}
		for _, id := range recipientMap[msg.ID] {
			addUserID(id)
		}
	}

	userIDs := make([]string, 0, len(userIDSet))
	for id := range userIDSet {
		userIDs = append(userIDs, id)
	}

	userMap := map[string]*model.UserModel{}
	if len(userIDs) > 0 {
		var users []*model.UserModel
		if err := model.GetDB().Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return err
		}
		for _, user := range users {
			if user == nil || strings.TrimSpace(user.ID) == "" {
				continue
			}
			userMap[user.ID] = user
		}
	}

	for _, msg := range messages {
		if msg == nil || !msg.IsWhisper {
			continue
		}
		recipientIDs := normalizeWhisperRecipientIDs(recipientMap[msg.ID])
		if len(recipientIDs) == 0 {
			recipientIDs = splitWhisperIDs(msg.WhisperTo)
		}
		if len(recipientIDs) > 0 {
			targets := make([]*model.UserModel, 0, len(recipientIDs))
			for _, id := range recipientIDs {
				if user, ok := userMap[id]; ok && user != nil {
					targets = append(targets, user)
					continue
				}
				targets = append(targets, &model.UserModel{
					StringPKBaseModel: model.StringPKBaseModel{ID: id},
				})
			}
			msg.WhisperTargets = targets
			if msg.WhisperTarget == nil && len(targets) > 0 {
				msg.WhisperTarget = targets[0]
			}
		}
		if msg.WhisperTarget == nil {
			ids := splitWhisperIDs(msg.WhisperTo)
			if len(ids) > 0 {
				if user, ok := userMap[ids[0]]; ok && user != nil {
					msg.WhisperTarget = user
				}
			}
		}
	}

	return nil
}

func normalizeWhisperRecipientIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func splitWhisperIDs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	return normalizeWhisperRecipientIDs(parts)
}

func mergeSequentialMessages(messages []*model.MessageModel) []*model.MessageModel {
	if len(messages) == 0 {
		return messages
	}
	const mergeWindow = 60 * time.Second
	var result []*model.MessageModel
	var current *model.MessageModel
	var lastTime time.Time
	var currentIcMode string
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		formatted := formatContentForMerge(msg)
		if current == nil {
			current = cloneMessage(msg)
			current.Content = formatted
			current.MergedMessages = 1
			lastTime = msg.CreatedAt
			currentIcMode = normalizeIcMode(msg.ICMode)
			result = append(result, current)
			continue
		}
		if canMerge(current, currentIcMode, lastTime, msg, mergeWindow, false) {
			nextContent := strings.TrimLeft(formatted, "\r\n")
			trimmed := strings.TrimRight(current.Content, " \r\n\t")
			if trimmed == "" {
				current.Content = nextContent
			} else {
				current.Content = trimmed + "\n" + nextContent
			}
			if current.MergedMessages <= 0 {
				current.MergedMessages = 1
			}
			current.MergedMessages++
			lastTime = msg.CreatedAt
			continue
		}
		current = cloneMessage(msg)
		current.Content = formatted
		current.MergedMessages = 1
		lastTime = msg.CreatedAt
		currentIcMode = normalizeIcMode(msg.ICMode)
		result = append(result, current)
	}
	return result
}

func mergeSequentialMessagesForExport(messages []*model.MessageModel, extra *exportExtraOptions, includeOOC bool) []*model.MessageModel {
	if len(messages) == 0 {
		return messages
	}
	includeImages := true
	includeDiceCommand := true
	if extra != nil {
		includeImages = extra.IncludeImages
		includeDiceCommand = extra.IncludeDiceCommand
	}
	const mergeWindow = 60 * time.Second
	var result []*model.MessageModel
	var current *model.MessageModel
	var lastTime time.Time
	var currentIcMode string
	var sawFilteredOOCGap bool
	var sawOtherFilteredGap bool

	for _, msg := range messages {
		filter := classifyExportMessage(msg, includeImages, includeDiceCommand, includeOOC)
		if filter.Skip {
			if current == nil {
				continue
			}
			if filter.SkippedOOC {
				sawFilteredOOCGap = true
			} else {
				sawOtherFilteredGap = true
			}
			continue
		}

		formatted := formatContentForMergeWithOption(msg, extra != nil && extra.WithoutOOCParentheses)
		if current == nil {
			current = cloneMessage(msg)
			current.Content = formatted
			current.MergedMessages = 1
			lastTime = msg.CreatedAt
			currentIcMode = normalizeIcMode(msg.ICMode)
			result = append(result, current)
			sawFilteredOOCGap = false
			sawOtherFilteredGap = false
			continue
		}

		allowFilteredOOCGapMerge := sawFilteredOOCGap && !sawOtherFilteredGap
		if !isStandaloneExportEmbedMessage(current.Content) && !isStandaloneExportEmbedMessage(msg.Content) &&
			canMerge(current, currentIcMode, lastTime, msg, mergeWindow, allowFilteredOOCGapMerge) {
			nextContent := strings.TrimLeft(formatted, "\r\n")
			trimmed := strings.TrimRight(current.Content, " \r\n\t")
			if trimmed == "" {
				current.Content = nextContent
			} else {
				current.Content = trimmed + "\n" + nextContent
			}
			if current.MergedMessages <= 0 {
				current.MergedMessages = 1
			}
			current.MergedMessages++
			lastTime = msg.CreatedAt
			sawFilteredOOCGap = false
			sawOtherFilteredGap = false
			continue
		}

		current = cloneMessage(msg)
		current.Content = formatted
		current.MergedMessages = 1
		lastTime = msg.CreatedAt
		currentIcMode = normalizeIcMode(msg.ICMode)
		result = append(result, current)
		sawFilteredOOCGap = false
		sawOtherFilteredGap = false
	}

	return result
}

func canMerge(base *model.MessageModel, currentIcMode string, last time.Time, next *model.MessageModel, window time.Duration, allowFilteredGap bool) bool {
	if base == nil || next == nil {
		return false
	}
	if !sameSenderIdentity(base, next) {
		return false
	}
	if currentIcMode != normalizeIcMode(next.ICMode) {
		return false
	}
	if base.IsWhisper != next.IsWhisper {
		return false
	}
	if whisperMergeContextKey(base) != whisperMergeContextKey(next) {
		return false
	}
	if base.IsArchived != next.IsArchived {
		return false
	}
	if allowFilteredGap {
		return true
	}
	diff := next.CreatedAt.Sub(last)
	if diff < 0 {
		diff = -diff
	}
	return diff <= window
}

func sameSenderIdentity(a, b *model.MessageModel) bool {
	return senderKey(a) != "" && senderKey(a) == senderKey(b)
}

func senderKey(msg *model.MessageModel) string {
	if msg == nil {
		return ""
	}
	if id := strings.TrimSpace(msg.SenderIdentityID); id != "" {
		return "identity:" + id
	}
	if user := strings.TrimSpace(msg.UserID); user != "" {
		return "user:" + user
	}
	return ""
}

func whisperMergeContextKey(msg *model.MessageModel) string {
	if msg == nil || !msg.IsWhisper {
		return "public"
	}
	recipientIDs := make([]string, 0, len(msg.WhisperTargets)+1)
	for _, target := range msg.WhisperTargets {
		if target == nil {
			continue
		}
		if id := strings.TrimSpace(target.ID); id != "" {
			recipientIDs = append(recipientIDs, id)
		}
	}
	recipientIDs = append(recipientIDs, splitWhisperIDs(msg.WhisperTo)...)
	recipientIDs = normalizeWhisperRecipientIDs(recipientIDs)
	if len(recipientIDs) > 0 {
		return "whisper:" + strings.Join(recipientIDs, ",")
	}
	targetNames := msg.ResolveWhisperTargetDisplayNames()
	if len(targetNames) > 0 {
		return "whisper-fallback:" + strings.Join(targetNames, "|")
	}
	return "whisper:unknown"
}

func normalizeIcMode(mode string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		return "ic"
	}
	return mode
}

func buildExportExtraOptions(opts *ExportJobOptions) (string, error) {
	if opts == nil {
		return "", nil
	}
	extra := exportExtraOptions{
		SourceKind:                opts.SourceKind,
		LogProjectID:              opts.LogProjectID,
		LogDraftVersion:           opts.LogDraftVersion,
		SliceLimit:                opts.SliceLimit,
		MaxConcurrency:            opts.MaxConcurrency,
		TextColorizeBBCode:        opts.TextColorizeBBCode,
		TextColorizeBBCodeMap:     cloneStringMap(opts.TextColorizeBBCodeMap),
		TextColorizeBBCodeNameMap: cloneStringMap(opts.TextColorizeBBCodeNameMap),
		AutoCorrectPunctuation:    opts.AutoCorrectPunctuation,
		IncludeImages:             opts.IncludeImages,
		IncludeDiceCommand:        opts.IncludeDiceCommand,
		WithoutOOCParentheses:     opts.WithoutOOCParentheses,
		VisibilityByChannel:       opts.visibilityByChannel,
	}
	if len(opts.DisplaySettings) > 0 {
		extra.DisplaySettings = opts.DisplaySettings
	}
	if len(extra.VisibilityByChannel) == 0 && extra.SourceKind == "" && extra.DisplaySettings == nil &&
		extra.SliceLimit == 0 &&
		extra.MaxConcurrency == 0 &&
		!extra.TextColorizeBBCode &&
		len(extra.TextColorizeBBCodeMap) == 0 &&
		len(extra.TextColorizeBBCodeNameMap) == 0 &&
		!extra.WithoutOOCParentheses &&
		extra.AutoCorrectPunctuation &&
		extra.IncludeImages &&
		extra.IncludeDiceCommand {
		return "", nil
	}
	data, err := json.Marshal(extra)
	if err != nil {
		return "", fmt.Errorf("导出附加参数序列化失败: %w", err)
	}
	return string(data), nil
}

func parseExportExtraOptions(raw string) *exportExtraOptions {
	extra := &exportExtraOptions{
		SliceLimit:             DefaultExportSliceLimit,
		MaxConcurrency:         DefaultExportConcurrency,
		AutoCorrectPunctuation: true,
		IncludeImages:          true,
		IncludeDiceCommand:     true,
	}
	if strings.TrimSpace(raw) == "" {
		return extra
	}
	if err := json.Unmarshal([]byte(raw), extra); err != nil {
		// 格式异常时退回默认值
		extra.SliceLimit = DefaultExportSliceLimit
		extra.MaxConcurrency = DefaultExportConcurrency
		extra.AutoCorrectPunctuation = true
		return extra
	}
	extra.SliceLimit = NormalizeExportSliceLimit(extra.SliceLimit)
	extra.MaxConcurrency = NormalizeExportConcurrency(extra.MaxConcurrency)
	if extra.DisplaySettings != nil && len(extra.DisplaySettings) == 0 {
		extra.DisplaySettings = nil
	}
	if len(extra.TextColorizeBBCodeMap) == 0 {
		extra.TextColorizeBBCodeMap = nil
	}
	if len(extra.TextColorizeBBCodeNameMap) == 0 {
		extra.TextColorizeBBCodeNameMap = nil
	}
	return extra
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

// loadExportColorOverrides reads the existing profile format for DOCX batch
// children. It is deliberately read-only and keeps the API package out of the
// service layer.
func loadExportColorOverrides(userID, channelID string) (map[string]string, map[string]string) {
	if model.GetDB() == nil {
		return nil, nil
	}
	colors := map[string]string{}
	names := map[string]string{}
	profiles, _, _, err := LoadExportColorProfile(userID, channelID)
	if err != nil {
		return nil, nil
	}
	for key, profile := range profiles {
		if profile.Color != "" {
			colors[key] = profile.Color
		}
		if profile.Name != "" {
			names[key] = profile.Name
		}
	}
	return colors, names
}

func cloneMessage(msg *model.MessageModel) *model.MessageModel {
	if msg == nil {
		return nil
	}
	clone := *msg
	return &clone
}

func formatContentForMerge(msg *model.MessageModel) string {
	return formatContentForMergeWithOption(msg, false)
}

func formatContentForMergeWithOption(msg *model.MessageModel, withoutOOCParentheses bool) string {
	if msg == nil {
		return ""
	}
	if !withoutOOCParentheses && strings.EqualFold(normalizeIcMode(msg.ICMode), "ooc") {
		return ensureOOCWrapped(msg.Content)
	}
	return msg.Content
}

func ensureOOCWrapped(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "（）"
	}
	if (strings.HasPrefix(trimmed, "（") && strings.HasSuffix(trimmed, "）")) ||
		(strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")")) {
		return trimmed
	}
	return fmt.Sprintf("（%s）", trimmed)
}
func NormalizeExportSliceLimit(value int) int {
	if value <= 0 {
		value = DefaultExportSliceLimit
	}
	if value < MinExportSliceLimit {
		return MinExportSliceLimit
	}
	if value > MaxExportSliceLimit {
		return MaxExportSliceLimit
	}
	return value
}

func NormalizeExportConcurrency(value int) int {
	if value <= 0 {
		value = DefaultExportConcurrency
	}
	if value < MinExportConcurrency {
		return MinExportConcurrency
	}
	if value > MaxExportConcurrency {
		return MaxExportConcurrency
	}
	return value
}

// ListMessageExportJobs 返回指定频道的导出任务及统计信息。
func ListMessageExportJobs(channelID string, statuses []string, limit, offset int, keyword, userID string) ([]*model.MessageExportJobModel, int64, int64, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, 0, 0, fmt.Errorf("channel_id 不能为空")
	}
	if limit <= 0 {
		limit = exportListDefaultLimit
	}
	if limit > exportListMaxLimit {
		limit = exportListMaxLimit
	}
	if offset < 0 {
		offset = 0
	}

	base := model.GetDB().Model(&model.MessageExportJobModel{}).Where("channel_id = ?", channelID)
	if len(statuses) > 0 {
		base = base.Where("status IN ?", statuses)
	}
	if trimmed := strings.TrimSpace(keyword); trimmed != "" {
		pattern := fmt.Sprintf("%%%s%%", trimmed)
		base = base.Where("(display_name LIKE ? OR file_name LIKE ?)", pattern, pattern)
	}
	// Restrict before counting/pagination: upload URLs are also sensitive.
	probe := &model.MessageExportJobModel{ChannelID: channelID}
	if !CanManageMessageExportJob(userID, probe, true) {
		base = base.Where("user_id = ?", userID)
	} else {
		// Batch tasks can span worlds. Check all their sources, not just the anchor.
		var candidates []*model.MessageExportJobModel
		if err := base.Session(&gorm.Session{}).Select("id, user_id, channel_id, extra_options").
			Where("user_id <> ? AND format = ?", userID, "zip").Find(&candidates).Error; err != nil {
			return nil, 0, 0, err
		}
		denied := make([]string, 0)
		for _, candidate := range candidates {
			if !CanManageMessageExportJob(userID, candidate, true) {
				denied = append(denied, candidate.ID)
			}
		}
		if len(denied) > 0 {
			base = base.Where("id NOT IN ?", denied)
		}
	}
	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, 0, err
	}

	var jobs []*model.MessageExportJobModel
	query := base.Session(&gorm.Session{}).Order("finished_at DESC, created_at DESC").Limit(limit).Offset(offset)
	if err := query.Find(&jobs).Error; err != nil {
		return nil, 0, 0, err
	}

	totalSize := int64(0)
	for _, job := range jobs {
		if exportJobFileExists(job) {
			totalSize += job.FileSize
		}
	}

	return jobs, total, totalSize, nil
}

// UpdateExportJobErrorMessage 只更新任务的错误描述，用于下载阶段反馈。
func UpdateExportJobErrorMessage(jobID, message string) error {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return fmt.Errorf("job_id 不能为空")
	}
	updates := map[string]any{
		"error_msg":  strings.TrimSpace(message),
		"updated_at": time.Now(),
	}
	return model.GetDB().Model(&model.MessageExportJobModel{}).
		Where("id = ?", jobID).
		Updates(updates).Error
}

func normalizeExportDisplayName(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	const maxLen = 120
	runes := []rune(trimmed)
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return trimmed
}

func exportJobFileExists(job *model.MessageExportJobModel) bool {
	if job == nil {
		return false
	}
	path := strings.TrimSpace(job.FilePath)
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// RetryMessageExportJob 根据既有任务重新创建一条导出任务。
func RetryMessageExportJob(job *model.MessageExportJobModel) (*model.MessageExportJobModel, error) {
	if job == nil {
		return nil, fmt.Errorf("任务不存在")
	}
	extra := parseExportExtraOptions(job.ExtraOptions)
	if extra.SourceKind == "log" {
		if _, err := loadLogProjectForExport(job.UserID, extra); err != nil {
			return nil, err
		}
	}
	opts := &ExportJobOptions{
		UserID:                 job.UserID,
		ChannelID:              job.ChannelID,
		SourceKind:             extra.SourceKind,
		LogProjectID:           extra.LogProjectID,
		LogDraftVersion:        extra.LogDraftVersion,
		Format:                 job.Format,
		DisplayName:            job.DisplayName,
		IncludeOOC:             job.IncludeOOC,
		IncludeArchived:        job.IncludeArchived,
		WithoutTimestamp:       job.WithoutTimestamp,
		WithoutOOCParentheses:  extra.WithoutOOCParentheses,
		MergeMessages:          job.MergeMessages,
		AutoCorrectPunctuation: extra.AutoCorrectPunctuation,
		StartTime:              job.StartTime,
		EndTime:                job.EndTime,
		visibilityByChannel:    extra.VisibilityByChannel,
	}
	if extra.SourceKind != "log" && opts.visibilityByChannel == nil {
		opts.visibilityByChannel = map[string]bool{}
	}
	opts.DisplaySettings = extra.DisplaySettings
	opts.SliceLimit = extra.SliceLimit
	opts.MaxConcurrency = extra.MaxConcurrency
	opts.IncludeImages = extra.IncludeImages
	opts.IncludeDiceCommand = extra.IncludeDiceCommand
	opts.TextColorizeBBCode = extra.TextColorizeBBCode
	opts.TextColorizeBBCodeMap = cloneStringMap(extra.TextColorizeBBCodeMap)
	opts.TextColorizeBBCodeNameMap = cloneStringMap(extra.TextColorizeBBCodeNameMap)
	if len(extra.BatchChannelIDs) > 0 {
		opts.Format = extra.BatchFormat
		return CreateBatchMessageExportJob(opts, extra.BatchChannelIDs)
	}
	return CreateMessageExportJob(opts)
}

// DeleteMessageExportJob 删除导出任务记录并清理本地文件。
func DeleteMessageExportJob(job *model.MessageExportJobModel) (bool, error) {
	if job == nil {
		return false, fmt.Errorf("任务不存在")
	}
	fileDeleted := false
	path := strings.TrimSpace(job.FilePath)
	if path != "" {
		info, err := os.Stat(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return false, err
			}
		} else if info.IsDir() {
			return false, fmt.Errorf("导出路径异常")
		} else if err := os.Remove(path); err != nil {
			if !os.IsNotExist(err) {
				return false, err
			}
		} else {
			fileDeleted = true
		}
	}
	if err := model.GetDB().Delete(&model.MessageExportJobModel{}, "id = ?", job.ID).Error; err != nil {
		return fileDeleted, err
	}
	return fileDeleted, nil
}

// ExportJobRuntimeSettings 返回任务当前的分页与并发配置。
func ExportJobRuntimeSettings(job *model.MessageExportJobModel) (int, int) {
	extra := parseExportExtraOptions("")
	if job != nil {
		extra = parseExportExtraOptions(job.ExtraOptions)
	}
	return extra.SliceLimit, extra.MaxConcurrency
}
