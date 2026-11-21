package service

import (
	"encoding/json"
	"sort"
	"strings"

	"sealchat/model"
)

// LogExportInput contains output options only; source selection belongs to the saved draft.
type LogExportInput struct {
	ExpectedDraftVersion   int64  `json:"expectedDraftVersion"`
	Format                 string `json:"format"`
	DisplayName            string `json:"displayName"`
	IncludeImages          *bool  `json:"includeImages"`
	IncludeDiceCommands    *bool  `json:"includeDiceCommands"`
	WithoutTimestamp       bool   `json:"withoutTimestamp"`
	WithoutOOCParentheses  bool   `json:"withoutOocParentheses"`
	MergeMessages          *bool  `json:"mergeMessages"`
	AutoCorrectPunctuation *bool  `json:"autoCorrectPunctuation"`
	TextBBCodeColorize     bool   `json:"textBbcodeColorize"`
}

func logExportConflict(version int64) error {
	return &LogError{Code: "VERSION_CONFLICT", Message: "LOG 排版已变化，请重新导出", Details: map[string]any{"draftVersion": version}}
}

func loadLogProjectForExport(userID string, extra *exportExtraOptions) (*model.LogProjectModel, error) {
	if extra == nil || extra.LogProjectID == "" || extra.LogDraftVersion < 1 || extra.LogDraftVersion > LogMaxVersion {
		return nil, logError("INVALID_INPUT", "LOG 导出来源无效")
	}
	p, _, err := loadLogProjectForDesign(extra.LogProjectID, userID)
	if err != nil {
		return nil, err
	}
	if p.DraftVersion != extra.LogDraftVersion {
		return nil, logExportConflict(p.DraftVersion)
	}
	return p, nil
}

func IsLogMessageExportJob(job *model.MessageExportJobModel) bool {
	return job != nil && parseExportExtraOptions(job.ExtraOptions).SourceKind == "log"
}

// CreateLogMessageExportJob reuses the traditional export queue without assigning a channel.
func CreateLogMessageExportJob(projectID, userID string, input LogExportInput) (*model.MessageExportJobModel, error) {
	format := strings.ToLower(strings.TrimSpace(input.Format))
	if format != "txt" && format != "docx" && format != "html" && format != "json" && format != exportFormatPDF {
		return nil, logError("INVALID_INPUT", "导出格式仅支持 txt / docx / html / json / pdf")
	}
	extra := &exportExtraOptions{SourceKind: "log", LogProjectID: projectID, LogDraftVersion: input.ExpectedDraftVersion}
	p, err := loadLogProjectForExport(userID, extra)
	if err != nil {
		return nil, err
	}
	name := input.DisplayName
	if strings.TrimSpace(name) == "" {
		var draft LogDraft
		if err := json.Unmarshal([]byte(p.DraftJSON), &draft); err != nil {
			return nil, err
		}
		name = draft.Title
	}
	return CreateMessageExportJob(&ExportJobOptions{
		UserID: userID, SourceKind: "log", LogProjectID: projectID, LogDraftVersion: p.DraftVersion,
		Format: format, DisplayName: name,
		IncludeOOC: true, IncludeArchived: true,
		IncludeImages:      input.IncludeImages != nil && *input.IncludeImages,
		IncludeDiceCommand: input.IncludeDiceCommands != nil && *input.IncludeDiceCommands,
		WithoutTimestamp:   input.WithoutTimestamp, WithoutOOCParentheses: input.WithoutOOCParentheses,
		MergeMessages:          input.MergeMessages == nil || *input.MergeMessages,
		AutoCorrectPunctuation: input.AutoCorrectPunctuation == nil || *input.AutoCorrectPunctuation,
		TextColorizeBBCode:     format == "txt" && input.TextBBCodeColorize,
	})
}

func loadLogMessagesForExport(job *model.MessageExportJobModel, extra *exportExtraOptions) ([]*model.MessageModel, error) {
	if job == nil {
		return nil, logError("INVALID_INPUT", "任务不存在")
	}
	p, err := loadLogProjectForExport(job.UserID, extra)
	if err != nil {
		return nil, err
	}
	var draft LogDraft
	if err := json.Unmarshal([]byte(p.DraftJSON), &draft); err != nil {
		return nil, err
	}
	sort.SliceStable(draft.Sections, func(i, j int) bool { return draft.Sections[i].Order < draft.Sections[j].Order })
	// Section queries own IC/OOC and archive selection. The output stage must not filter them again.
	outputJob := *job
	outputJob.IncludeOOC = true
	outputJob.IncludeArchived = true
	remaining := messageExportLimit
	result := make([]*model.MessageModel, 0)
	// 与 LOG 预览和发布使用相同的正文覆盖保护：不引入嵌入或新附件。
	var display *messageDisplayResolver
	if len(draft.MessageContentOverrides) > 0 {
		display = newMessageDisplayResolver("", job.UserID, messageDisplayLogArchive)
	}
	for _, section := range draft.Sections {
		if remaining == 0 {
			break
		}
		var messages []*model.MessageModel
		if err := logSourceQuery(p, section, job.UserID).
			Where("is_revoked = ?", false).
			Preload("User").Preload("Member").
			Order("display_order ASC, created_at ASC, id ASC").Limit(remaining).Find(&messages).Error; err != nil {
			return nil, err
		}
		remaining -= len(messages)
		if err := hydrateWhisperTargetsForExport(messages); err != nil {
			return nil, err
		}
		if display != nil {
			for i, m := range messages {
				if override := draft.MessageContentOverrides[m.ID]; override != "" {
					messages[i] = logApplyContentOverride(display.forChannel(m.ChannelID), m, override)
				}
			}
		}
		// Each call resets the merge accumulator at the narrative boundary.
		result = append(result, prepareMessagesForExport(messages, &outputJob, extra)...)
	}
	// Reject edits saved while the section queries were running as well.
	var current model.LogProjectModel
	if err := model.GetDB().Select("id, draft_version, enabled").Where("id = ?", p.ID).Take(&current).Error; err != nil {
		return nil, err
	}
	if current.DraftVersion != extra.LogDraftVersion || !current.Enabled {
		return nil, logExportConflict(current.DraftVersion)
	}
	return result, nil
}
