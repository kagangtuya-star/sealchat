package service

import (
	"regexp"
	"strings"
	"time"

	"sealchat/model"
)

// 内置正则模板
var chatImportTemplates = []*model.ChatImportTemplate{
	{
		ID:          "timestamp_angle",
		Name:        "带时间戳尖括号格式",
		Description: "[时间戳]<角色名>内容 或 [时间戳]<角色名>：内容",
		Pattern:     `\[(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})\]\s*<([^>]+)>\s*[:：]?\s*(.*)`,
		Example:     "[2025-12-07 21:02:49] <木落> 你好世界",
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
		Example:     "<木落>：从前有一座房子",
	},
	{
		ID:          "bracket_name",
		Name:        "方括号角色名格式",
		Description: "[角色名] 内容",
		Pattern:     `\[([^\]]+)\]\s*[:：]?\s*(.*)`,
		Example:     "[木落] 你好世界",
	},
	{
		ID:          "colon_name",
		Name:        "冒号分隔格式",
		Description: "角色名：内容 或 角色名: 内容",
		Pattern:     `^([^:：\s]+)\s*[:：]\s*(.+)`,
		Example:     "木落：你好世界",
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
	pattern        *regexp.Regexp
	templateID     string
	baseTime       time.Time
	timeIncrement  int64 // 毫秒
	mergeUnmatched bool
	strictOOC      bool
	hasTimeGroup   bool // 正则是否包含时间捕获组
	hasDateGroup   bool // 正则是否包含日期捕获组
}

// NewChatLogParser 创建解析器
func NewChatLogParser(config *model.ChatImportConfig) (*ChatLogParser, error) {
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

	baseTime := time.Now()
	if config.BaseTime != nil {
		baseTime = *config.BaseTime
	}

	timeIncrement := config.TimeIncrement
	if timeIncrement <= 0 {
		timeIncrement = 1000 // 默认1秒
	}

	// 检测正则是否包含时间/日期组
	hasTimeGroup := strings.Contains(pattern, `\d{2}:\d{2}:\d{2}`)
	hasDateGroup := strings.Contains(pattern, `\d{4}-\d{2}-\d{2}`)

	return &ChatLogParser{
		pattern:        re,
		templateID:     templateID,
		baseTime:       baseTime,
		timeIncrement:  timeIncrement,
		mergeUnmatched: config.MergeUnmatched,
		strictOOC:      config.StrictOOC,
		hasTimeGroup:   hasTimeGroup,
		hasDateGroup:   hasDateGroup,
	}, nil
}

// ParseLogContent 解析日志内容
func (p *ChatLogParser) ParseLogContent(content string) ([]*model.ParsedLogEntry, int, int) {
	lines := strings.Split(content, "\n")
	var entries []*model.ParsedLogEntry
	var currentEntry *model.ParsedLogEntry
	skippedCount := 0
	currentTime := p.baseTime

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
			entry := p.parseMatchedLine(matches, line, lineNum+1, &currentTime)
			if entry != nil {
				entries = append(entries, entry)
				currentEntry = entry
			}
		} else if p.mergeUnmatched && currentEntry != nil {
			// 不匹配行追加到上一条消息（只在没遇到空行时）
			currentEntry.Content += "\n" + strings.TrimSpace(line)
			currentEntry.RawLine += "\n" + line
		} else {
			skippedCount++
		}
	}

	// 处理OOC标记
	p.processOOCMarking(entries)

	return entries, len(lines), skippedCount
}

// parseMatchedLine 解析匹配的行
func (p *ChatLogParser) parseMatchedLine(matches []string, rawLine string, lineNum int, currentTime *time.Time) *model.ParsedLogEntry {
	entry := &model.ParsedLogEntry{
		RawLine:    rawLine,
		LineNumber: lineNum,
	}

	numGroups := len(matches) - 1

	switch p.templateID {
	case "timestamp_angle":
		// 组1=时间戳, 组2=角色名, 组3=内容
		if numGroups >= 3 {
			entry.Timestamp = p.parseTimestamp(matches[1], true)
			entry.RoleName = strings.TrimSpace(matches[2])
			entry.Content = strings.TrimSpace(matches[3])
		}
	case "time_angle":
		// 组1=时间, 组2=角色名, 组3=内容
		if numGroups >= 3 {
			entry.Timestamp = p.parseTimestamp(matches[1], false)
			entry.RoleName = strings.TrimSpace(matches[2])
			entry.Content = strings.TrimSpace(matches[3])
		}
	case "angle_only":
		// 组1=角色名, 组2=内容
		if numGroups >= 2 {
			t := *currentTime
			entry.Timestamp = &t
			*currentTime = currentTime.Add(time.Duration(p.timeIncrement) * time.Millisecond)
			entry.RoleName = strings.TrimSpace(matches[1])
			entry.Content = strings.TrimSpace(matches[2])
		}
	case "bracket_name":
		// 组1=角色名, 组2=内容
		if numGroups >= 2 {
			t := *currentTime
			entry.Timestamp = &t
			*currentTime = currentTime.Add(time.Duration(p.timeIncrement) * time.Millisecond)
			entry.RoleName = strings.TrimSpace(matches[1])
			entry.Content = strings.TrimSpace(matches[2])
		}
	case "colon_name":
		// 组1=角色名, 组2=内容
		if numGroups >= 2 {
			t := *currentTime
			entry.Timestamp = &t
			*currentTime = currentTime.Add(time.Duration(p.timeIncrement) * time.Millisecond)
			entry.RoleName = strings.TrimSpace(matches[1])
			entry.Content = strings.TrimSpace(matches[2])
		}
	default:
		// 自定义正则：尝试智能解析
		// 假设：如果有3+组且第一组像时间戳，则 组1=时间, 组2=角色, 组3=内容
		// 否则：组1=角色, 组2=内容
		if numGroups >= 3 && p.looksLikeTimestamp(matches[1]) {
			entry.Timestamp = p.parseTimestamp(matches[1], p.hasDateGroup)
			entry.RoleName = strings.TrimSpace(matches[2])
			entry.Content = strings.TrimSpace(matches[3])
		} else if numGroups >= 2 {
			t := *currentTime
			entry.Timestamp = &t
			*currentTime = currentTime.Add(time.Duration(p.timeIncrement) * time.Millisecond)
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
func (p *ChatLogParser) parseTimestamp(s string, hasDate bool) *time.Time {
	var t time.Time
	var err error

	layouts := []string{
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		"2006-01-02T15:04:05",
		"15:04:05",
		"15:04",
	}

	for _, layout := range layouts {
		t, err = time.Parse(layout, strings.TrimSpace(s))
		if err == nil {
			// 如果只解析到时间没有日期，使用baseTime的日期
			if !hasDate && t.Year() == 0 {
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
