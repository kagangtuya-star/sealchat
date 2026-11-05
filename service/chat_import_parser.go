package service

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata"

	"sealchat/model"
)

// 内置正则模板
var chatImportTemplates = []*model.ChatImportTemplate{
	{
		ID:          "timestamp_angle",
		Name:        "带时间戳尖括号格式",
		Description: "[时间戳]<角色名>内容 或 [时间戳]<角色名>：内容",
		Pattern:     `\[(\d{4}-\d{2}-\d{2}[T\s]+\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2}|[ \t]+[+-]\d{2}:?\d{2})?)\]\s*<([^>]+)>\s*[:：]?\s*(.*)`,
		Example:     "[2025-12-07 21:02:49] <星尘> 你好世界",
	},
	{
		ID:          "time_angle",
		Name:        "仅时间尖括号格式",
		Description: "HH:mm:ss<角色名>内容 或 HH:mm:ss<角色名>：内容",
		Pattern:     `(\d{2}:\d{2}:\d{2})\s*<([^>]+)>\s*[:：]?\s*(.*)`,
		Example:     "19:05:05<海豹一号机>:新的故事开始了",
	},
	{
		ID:          "angle_only",
		Name:        "仅尖括号格式",
		Description: "<角色名>内容 或 <角色名>：内容（无时间信息）",
		Pattern:     `<([^>]+)>\s*[:：]?\s*(.*)`,
		Example:     "<星尘>：从前有一座房子",
	},
	{
		ID:          "bracket_name",
		Name:        "方括号角色名格式",
		Description: "[角色名] 内容",
		Pattern:     `\[([^\]]+)\]\s*[:：]?\s*(.*)`,
		Example:     "[星尘] 你好世界",
	},
	{
		ID:          "colon_name",
		Name:        "冒号分隔格式",
		Description: "角色名：内容 或 角色名: 内容",
		Pattern:     `^([^:：\s]+)\s*[:：]\s*(.+)`,
		Example:     "星尘：你好世界",
	},
	{
		ID:          "ccfolia",
		Name:        "CCFOLIA格式（冒号分隔格式二）",
		Description: "角色名：内容（消息之间以空行分隔）",
		Pattern:     `(?ms)^[ \t]*([^\r\n:：]{1,40}?)[ \t]+[:：][ \t]+(.*?)(?:\r?\n[ \t]*\r?\n+|\z)`,
		Example:     "星尘：你好世界\n\n海豹一号机：新的故事开始了",
	},
}

// GetChatImportTemplates 获取所有内置模板
func GetChatImportTemplates() []*model.ChatImportTemplate {
	return chatImportTemplates
}

// GetChatImportTemplateByID 根据ID获取模板
func GetChatImportTemplateByID(id string) *model.ChatImportTemplate {
	for _, t := range chatImportTemplates {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// ChatLogParser 日志解析器
type ChatLogParser struct {
	pattern          *regexp.Regexp
	templateID       string
	baseTime         time.Time
	location         *time.Location
	explicitBaseTime bool
	timeIncrement    int64 // 毫秒
	mergeUnmatched   bool
	strictOOC        bool
	hasTimeGroup     bool // 正则是否包含时间捕获组
}

// NewChatLogParser 创建解析器
func NewChatLogParser(config *model.ChatImportConfig) (*ChatLogParser, error) {
	return newChatLogParserAt(config, time.Now().UTC())
}

// 同一次任务只捕获一次时间，所有自动时间戳在分批写入之前确定。
func newChatLogParserAt(config *model.ChatImportConfig, now time.Time) (*ChatLogParser, error) {
	if config == nil {
		config = &model.ChatImportConfig{}
	}
	location := time.UTC
	if config.TimeZone != "" {
		var err error
		if config.TimeZone == "Local" {
			return nil, fmt.Errorf("无效的 IANA 时区: %s", config.TimeZone)
		}
		location, err = time.LoadLocation(config.TimeZone)
		if err != nil {
			return nil, fmt.Errorf("无效的 IANA 时区: %s", config.TimeZone)
		}
	}
	var pattern string
	var templateID string

	if config.RegexPattern != "" {
		pattern = config.RegexPattern
	} else if config.TemplateID != "" {
		tmpl := GetChatImportTemplateByID(config.TemplateID)
		if tmpl != nil {
			pattern = tmpl.Pattern
			templateID = tmpl.ID
		}
	}

	if pattern == "" {
		// 默认使用带时间戳尖括号格式
		pattern = chatImportTemplates[0].Pattern
		templateID = chatImportTemplates[0].ID
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	baseTime := now.UTC()
	if config.BaseTime != nil {
		baseTime = *config.BaseTime
	}
	if config.TimeZone != "" {
		baseTime = baseTime.In(location)
	} else if config.BaseTime != nil {
		// RFC3339 解码可能复用 time.Local；旧客户端只沿用输入的固定偏移，不能取服务器的夏令时规则。
		_, offset := baseTime.Zone()
		baseTime = baseTime.In(time.FixedZone("", offset))
	}

	timeIncrement := config.TimeIncrement
	if timeIncrement <= 0 {
		timeIncrement = 1000 // 默认1秒
	}

	// 检测可选时间捕获组，缺少原始时间时仍使用后面的角色、内容组。
	hasTimeGroup := strings.Contains(pattern, `\d{2}:\d{2}:\d{2}`)

	return &ChatLogParser{
		pattern:          re,
		templateID:       templateID,
		baseTime:         baseTime,
		location:         location,
		explicitBaseTime: config.BaseTime != nil,
		timeIncrement:    timeIncrement,
		mergeUnmatched:   config.MergeUnmatched,
		strictOOC:        config.StrictOOC,
		hasTimeGroup:     hasTimeGroup,
	}, nil
}

// ParseLogContent 解析日志内容
func (p *ChatLogParser) ParseLogContent(content string) ([]*model.ParsedLogEntry, int, int) {
	lines := strings.Split(content, "\n")
	var entries []*model.ParsedLogEntry
	var currentEntry *model.ParsedLogEntry
	skippedCount := 0

	for lineNum, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			// 空行：停止向当前条目追加内容
			currentEntry = nil
			continue
		}

		matches := p.pattern.FindStringSubmatch(line)
		if matches != nil {
			// 匹配成功，创建新条目
			entry := p.parseMatchedLine(matches, line, lineNum+1)
			if entry != nil && entry.RoleName != "" {
				entries = append(entries, entry)
				currentEntry = entry
			} else {
				// 空角色不会被创建身份或入库，也不应占用自动时间序列。
				skippedCount++
				currentEntry = nil
			}
		} else if p.mergeUnmatched && currentEntry != nil {
			// 不匹配行追加到上一条消息（只在没遇到空行时）
			currentEntry.Content += "\n" + strings.TrimSpace(line)
			currentEntry.RawLine += "\n" + line
		} else {
			skippedCount++
		}
	}

	// 只为缺少可解析原始时间的消息生成序列，合并行、空行及原始时间均不占用增量。
	count := 0
	for _, entry := range entries {
		if entry.Timestamp == nil {
			count++
		}
	}
	currentTime := p.baseTime
	increment := time.Duration(p.timeIncrement) * time.Millisecond
	if !p.explicitBaseTime && count > 0 {
		currentTime = currentTime.Add(-time.Duration(count-1) * increment)
	}
	for _, entry := range entries {
		if entry.Timestamp == nil {
			t := currentTime
			entry.Timestamp = &t
			currentTime = currentTime.Add(increment)
		}
	}

	// 处理OOC标记
	p.processOOCMarking(entries)

	return entries, len(lines), skippedCount
}

// parseMatchedLine 解析匹配的行
func (p *ChatLogParser) parseMatchedLine(matches []string, rawLine string, lineNum int) *model.ParsedLogEntry {
	entry := &model.ParsedLogEntry{
		RawLine:    rawLine,
		LineNumber: lineNum,
	}

	numGroups := len(matches) - 1

	switch p.templateID {
	case "timestamp_angle":
		// 组1=时间戳, 组2=角色名, 组3=内容
		if numGroups >= 3 {
			entry.Timestamp = p.parseTimestamp(matches[1])
			entry.RoleName = strings.TrimSpace(matches[2])
			entry.Content = strings.TrimSpace(matches[3])
		}
	case "time_angle":
		// 组1=时间, 组2=角色名, 组3=内容
		if numGroups >= 3 {
			entry.Timestamp = p.parseTimestamp(matches[1])
			entry.RoleName = strings.TrimSpace(matches[2])
			entry.Content = strings.TrimSpace(matches[3])
		}
	case "angle_only":
		// 组1=角色名, 组2=内容
		if numGroups >= 2 {
			entry.RoleName = strings.TrimSpace(matches[1])
			entry.Content = strings.TrimSpace(matches[2])
		}
	case "bracket_name":
		// 组1=角色名, 组2=内容
		if numGroups >= 2 {
			entry.RoleName = strings.TrimSpace(matches[1])
			entry.Content = strings.TrimSpace(matches[2])
		}
	case "colon_name":
		// 组1=角色名, 组2=内容
		if numGroups >= 2 {
			entry.RoleName = strings.TrimSpace(matches[1])
			entry.Content = strings.TrimSpace(matches[2])
		}
	default:
		// 自定义正则：尝试智能解析
		// 假设：如果有3+组且第一组像时间戳，则 组1=时间, 组2=角色, 组3=内容
		// 否则：组1=角色, 组2=内容
		if numGroups >= 3 && (p.looksLikeTimestamp(matches[1]) || (p.hasTimeGroup && strings.TrimSpace(matches[1]) == "")) {
			entry.Timestamp = p.parseTimestamp(matches[1])
			entry.RoleName = strings.TrimSpace(matches[2])
			entry.Content = strings.TrimSpace(matches[3])
		} else if numGroups >= 2 {
			entry.RoleName = strings.TrimSpace(matches[1])
			entry.Content = strings.TrimSpace(matches[2])
		}
	}

	return entry
}

// looksLikeTimestamp 检查字符串是否像时间戳
func (p *ChatLogParser) looksLikeTimestamp(s string) bool {
	// 检查是否包含时间相关格式
	timePatterns := []string{
		`\d{2}:\d{2}:\d{2}`,
		`\d{4}-\d{2}-\d{2}`,
		`\d{2}/\d{2}/\d{4}`,
	}
	for _, pattern := range timePatterns {
		if matched, _ := regexp.MatchString(pattern, s); matched {
			return true
		}
	}
	return false
}

// parseTimestamp 解析时间戳
func (p *ChatLogParser) parseTimestamp(s string) *time.Time {
	var t time.Time
	var err error

	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05 -07:00",
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		"2006-01-02T15:04:05",
		"15:04:05",
		"15:04",
	}

	for _, layout := range layouts {
		t, err = time.ParseInLocation(layout, strings.TrimSpace(s), p.location)
		if err == nil {
			// 如果只解析到时间没有日期，使用baseTime的日期
			if t.Year() == 0 {
				t = time.Date(
					p.baseTime.Year(), p.baseTime.Month(), p.baseTime.Day(),
					t.Hour(), t.Minute(), t.Second(), t.Nanosecond(),
					p.baseTime.Location(),
				)
			}
			return &t
		}
	}

	return nil
}

// processOOCMarking 处理OOC标记
func (p *ChatLogParser) processOOCMarking(entries []*model.ParsedLogEntry) {
	if p.strictOOC {
		// 严格模式：只看首字符
		for _, entry := range entries {
			content := strings.TrimSpace(entry.Content)
			if strings.HasPrefix(content, "(") || strings.HasPrefix(content, "（") {
				entry.IsOOC = true
			}
		}
	} else {
		// 宽松模式：检查内容是否被括号完全包裹
		for _, entry := range entries {
			content := strings.TrimSpace(entry.Content)
			if content == "" {
				continue
			}

			// 检查是否以括号开始和结束
			startsWithParen := strings.HasPrefix(content, "(") || strings.HasPrefix(content, "（")
			endsWithParen := strings.HasSuffix(content, ")") || strings.HasSuffix(content, "）")

			// 只有当内容以括号开始并以括号结束时才标记为OOC
			if startsWithParen && endsWithParen && len(content) > 1 {
				entry.IsOOC = true
			}
		}
	}
}

// ExtractRoleNames 从解析结果中提取角色名列表
func ExtractRoleNames(entries []*model.ParsedLogEntry) []string {
	roleSet := make(map[string]struct{})
	var roles []string

	for _, entry := range entries {
		if entry.RoleName != "" {
			if _, exists := roleSet[entry.RoleName]; !exists {
				roleSet[entry.RoleName] = struct{}{}
				roles = append(roles, entry.RoleName)
			}
		}
	}

	return roles
}

// ParsePreview 解析预览
func ParsePreview(req *model.ChatImportPreviewRequest) (*model.ChatImportPreviewResponse, error) {
	config := &model.ChatImportConfig{
		RegexPattern:   req.RegexPattern,
		TemplateID:     req.TemplateID,
		MergeUnmatched: req.MergeUnmatched,
		BaseTime:       req.BaseTime,
		TimeZone:       req.TimeZone,
		TimeIncrement:  req.TimeIncrement,
		StrictOOC:      req.StrictOOC,
	}

	parser, err := NewChatLogParser(config)
	if err != nil {
		return nil, err
	}

	entries, totalLines, skippedCount := parser.ParseLogContent(req.Content)

	// 限制预览数量
	limit := req.PreviewLimit
	if limit <= 0 {
		limit = 20
	}
	previewEntries := entries
	if len(entries) > limit {
		previewEntries = entries[:limit]
	}

	// 获取使用的模板名称
	usedTemplateName := ""
	if parser.templateID != "" {
		tmpl := GetChatImportTemplateByID(parser.templateID)
		if tmpl != nil {
			usedTemplateName = tmpl.Name
		}
	}

	return &model.ChatImportPreviewResponse{
		Entries:          previewEntries,
		TotalLines:       totalLines,
		ParsedCount:      len(entries),
		SkippedCount:     skippedCount,
		DetectedRoles:    ExtractRoleNames(entries),
		UsedPattern:      parser.pattern.String(),
		UsedTemplateName: usedTemplateName,
	}, nil
}
