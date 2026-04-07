package model

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"sealchat/protocol"
)

const displayOrderBaseGap = 1024.0

type MessageModel struct {
	StringPKBaseModel
	Content          string  `json:"content"`
	WidgetData       string  `json:"widget_data" gorm:"type:text;not null;default:''"`
	ChannelID        string  `json:"channel_id" gorm:"size:100;index:idx_msg_channel_order,priority:1;uniqueIndex:idx_msg_client_dedupe,priority:1"`
	GuildID          string  `json:"guild_id" gorm:"null;size:100"`
	MemberID         string  `json:"member_id" gorm:"null;size:100"`
	UserID           string  `json:"user_id" gorm:"null;size:100;uniqueIndex:idx_msg_client_dedupe,priority:2"`
	QuoteID          string  `json:"quote_id" gorm:"null;size:100"`
	DisplayOrder     float64 `json:"display_order" gorm:"type:decimal(24,8);index:idx_msg_channel_order,priority:2"`
	ClientID         *string `json:"client_id,omitempty" gorm:"size:100;uniqueIndex:idx_msg_client_dedupe,priority:3"`
	IsRevoked        bool    `json:"is_revoked" gorm:"null"` // 被撤回。这样实现可能不很严肃，但是能填补窗口中空白
	IsWhisper        bool    `json:"is_whisper" gorm:"default:false"`
	WhisperTo        string  `json:"whisper_to" gorm:"size:100"`
	IsEdited         bool    `json:"is_edited" gorm:"default:false"`
	EditCount        int     `json:"edit_count" gorm:"default:0"`
	EditedByUserID   string  `json:"edited_by_user_id" gorm:"size:100"`   // 编辑者用户ID（管理员编辑时记录）
	EditedByUserName string  `json:"edited_by_user_name" gorm:"size:100"` // 编辑者用户名
	// Whisper 元数据持久化
	WhisperSenderMemberID   string `json:"whisper_sender_member_id" gorm:"size:100"`
	WhisperSenderMemberName string `json:"whisper_sender_member_name"`
	WhisperSenderUserName   string `json:"whisper_sender_user_name"`
	WhisperSenderUserNick   string `json:"whisper_sender_user_nick"`
	WhisperTargetMemberID   string `json:"whisper_target_member_id" gorm:"size:100"`
	WhisperTargetMemberName string `json:"whisper_target_member_name"`
	WhisperTargetUserName   string `json:"whisper_target_user_name"`
	WhisperTargetUserNick   string `json:"whisper_target_user_nick"`

	ICMode        string     `json:"ic_mode" gorm:"size:8;default:'ic';index:idx_msg_ic_archive"`
	IsArchived    bool       `json:"is_archived" gorm:"default:false;index:idx_msg_ic_archive"`
	ArchivedAt    *time.Time `json:"archived_at"`
	ArchivedBy    string     `json:"archived_by" gorm:"size:100"`
	ArchiveReason string     `json:"archive_reason" gorm:"size:255"`
	IsPinned      bool       `json:"is_pinned" gorm:"default:false;index:idx_msg_pinned"`
	PinnedAt      *time.Time `json:"pinned_at"`
	PinnedBy      string     `json:"pinned_by" gorm:"size:100"`
	IsDeleted     bool       `json:"is_deleted" gorm:"default:false;index:idx_msg_deleted"` // 删除后不再展示
	DeletedAt     *time.Time `json:"deleted_at"`
	DeletedBy     string     `json:"deleted_by" gorm:"size:100"`
	IsImported    bool       `json:"isImported" gorm:"default:false;index:idx_msg_imported"`
	ImportJobID   string     `json:"importJobId" gorm:"size:100;index:idx_msg_import_job_id"`

	SenderMemberName          string                        `json:"sender_member_name"` // 用户在当时的名字
	SenderIdentityID          string                        `json:"sender_identity_id" gorm:"size:100"`
	SenderIdentityVariantID   string                        `json:"sender_identity_variant_id" gorm:"size:100"`
	SenderIdentityName        string                        `json:"sender_identity_name"`
	SenderIdentityColor       string                        `json:"sender_identity_color"`
	SenderIdentityAvatarID    string                        `json:"sender_identity_avatar_id"`
	SenderIdentityDecorations protocol.AvatarDecorationList `json:"sender_identity_decorations,omitempty" gorm:"serializer:json;column:sender_identity_decoration"`
	SenderIdentityIsTemporary bool                          `json:"sender_identity_is_temporary" gorm:"default:false"`
	SenderRoleID              string                        `json:"sender_role_id" gorm:"size:100"`
	MergedMessages            int                           `json:"-" gorm:"-"`

	User   *UserModel    `json:"user"`           // 嵌套 User 结构体
	Member *MemberModel  `json:"member"`         // 嵌套 Member 结构体
	Quote  *MessageModel `json:"quote" gorm:"-"` // 嵌套 Message 结构体
	// WhisperTarget 为前端展示提供冗余
	WhisperTarget             *UserModel                `json:"whisper_target" gorm:"-"`
	WhisperTargets            []*UserModel              `json:"whisper_targets" gorm:"-"`
	WhisperTargetDisplayNames []string                  `json:"whisper_target_display_names,omitempty" gorm:"-"`
	WhisperMeta               *protocol.WhisperMeta     `json:"whisper_meta,omitempty" gorm:"-"`
	Reactions                 []MessageReactionListItem `json:"reactions" gorm:"-"`
}

func (*MessageModel) TableName() string {
	return "messages"
}

func MessageUpdate(id string, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	if rawDecoration, ok := values["sender_identity_decoration"]; ok {
		switch value := rawDecoration.(type) {
		case nil:
			values["sender_identity_decoration"] = nil
		case protocol.AvatarDecorationList:
			encoded, err := json.Marshal(value)
			if err != nil {
				return err
			}
			values["sender_identity_decoration"] = string(encoded)
		case []protocol.AvatarDecoration:
			encoded, err := json.Marshal(value)
			if err != nil {
				return err
			}
			values["sender_identity_decoration"] = string(encoded)
		case *protocol.AvatarDecoration:
			encoded, err := json.Marshal(protocol.AvatarDecorationList{*value})
			if err != nil {
				return err
			}
			values["sender_identity_decoration"] = string(encoded)
		case protocol.AvatarDecoration:
			encoded, err := json.Marshal(protocol.AvatarDecorationList{value})
			if err != nil {
				return err
			}
			values["sender_identity_decoration"] = string(encoded)
		}
	}
	return db.Model(&MessageModel{}).Where("id = ?", id).Updates(values).Error
}

func (m *MessageModel) ToProtocolType2(channelData *protocol.Channel) *protocol.Message {
	var updatedAt int64
	if !m.UpdatedAt.IsZero() {
		updatedAt = m.UpdatedAt.UnixMilli()
	}
	icMode := m.ICMode
	if icMode == "" {
		icMode = "ic"
	}
	var archivedAt int64
	if m.ArchivedAt != nil {
		archivedAt = m.ArchivedAt.UnixMilli()
	}
	var deletedAt int64
	if m.DeletedAt != nil {
		deletedAt = m.DeletedAt.UnixMilli()
	}
	var pinnedAt int64
	if m.PinnedAt != nil {
		pinnedAt = m.PinnedAt.UnixMilli()
	}
	msg := &protocol.Message{
		ID:               m.ID,
		Content:          m.Content,
		Channel:          channelData,
		CreatedAt:        m.CreatedAt.UnixMilli(),
		UpdatedAt:        updatedAt,
		DisplayOrder:     m.DisplayOrder,
		IsWhisper:        m.IsWhisper,
		IsEdited:         m.IsEdited,
		EditCount:        m.EditCount,
		EditedByUserId:   m.EditedByUserID,
		EditedByUserName: m.EditedByUserName,
		IcMode:           icMode,
		IsArchived:       m.IsArchived,
		ArchivedAt:       archivedAt,
		ArchivedBy:       m.ArchivedBy,
		ArchiveReason:    m.ArchiveReason,
		IsPinned:         m.IsPinned,
		PinnedAt:         pinnedAt,
		PinnedBy:         m.PinnedBy,
		IsDeleted:        m.IsDeleted,
		DeletedAt:        deletedAt,
		DeletedBy:        m.DeletedBy,
		WidgetData:       m.WidgetData,
		WhisperTo: func() *protocol.User {
			if m.WhisperTarget != nil {
				return m.WhisperTarget.ToProtocolType()
			}
			return nil
		}(),
	}
	if m.ClientID != nil {
		msg.ClientID = *m.ClientID
	}
	if len(m.WhisperTargets) > 0 {
		msg.WhisperToIds = make([]*protocol.User, 0, len(m.WhisperTargets))
		for _, target := range m.WhisperTargets {
			if target == nil {
				continue
			}
			msg.WhisperToIds = append(msg.WhisperToIds, target.ToProtocolType())
		}
	}
	var legacyDecoration *protocol.AvatarDecoration
	if len(m.SenderIdentityDecorations) > 0 {
		first := m.SenderIdentityDecorations[0]
		legacyDecoration = &first
	}
	if m.SenderIdentityID != "" || m.SenderIdentityColor != "" || m.SenderIdentityAvatarID != "" || m.SenderIdentityName != "" || m.SenderIdentityIsTemporary || len(m.SenderIdentityDecorations) > 0 {
		msg.Identity = &protocol.MessageIdentity{
			ID:                m.SenderIdentityID,
			VariantID:         m.SenderIdentityVariantID,
			DisplayName:       m.SenderIdentityName,
			Color:             m.SenderIdentityColor,
			AvatarAttachment:  m.SenderIdentityAvatarID,
			AvatarDecoration:  legacyDecoration,
			AvatarDecorations: m.SenderIdentityDecorations,
			IsTemporary:       m.SenderIdentityIsTemporary,
		}
	}
	if meta := m.buildWhisperMeta(); meta != nil {
		msg.WhisperMeta = meta
	}
	if m.SenderRoleID != "" {
		msg.SenderRoleID = m.SenderRoleID
	}
	return msg
}

func (m *MessageModel) buildWhisperMeta() *protocol.WhisperMeta {
	if !m.IsWhisper {
		return nil
	}
	meta := &protocol.WhisperMeta{
		SenderMemberID:   m.WhisperSenderMemberID,
		SenderMemberName: m.WhisperSenderMemberName,
		SenderUserID:     m.UserID,
		SenderUserNick:   m.WhisperSenderUserNick,
		SenderUserName:   m.WhisperSenderUserName,
		TargetMemberID:   m.WhisperTargetMemberID,
		TargetMemberName: m.WhisperTargetMemberName,
		TargetUserID:     m.WhisperTo,
		TargetUserNick:   m.WhisperTargetUserNick,
		TargetUserName:   m.WhisperTargetUserName,
	}
	if len(m.WhisperTargets) > 0 {
		targetIDs := make([]string, 0, len(m.WhisperTargets))
		seen := map[string]struct{}{}
		for _, target := range m.WhisperTargets {
			if target == nil || target.ID == "" {
				continue
			}
			if _, ok := seen[target.ID]; ok {
				continue
			}
			seen[target.ID] = struct{}{}
			targetIDs = append(targetIDs, target.ID)
		}
		if len(targetIDs) > 0 {
			meta.TargetUserIds = targetIDs
		}
	}
	if displayNames := m.ResolveWhisperTargetDisplayNames(); len(displayNames) > 0 {
		meta.TargetDisplayNames = append([]string{}, displayNames...)
	}
	if meta.SenderMemberID == "" {
		meta.SenderMemberID = m.MemberID
	}
	if meta.SenderMemberName == "" {
		meta.SenderMemberName = m.SenderMemberName
	}
	if meta.SenderUserNick == "" {
		if m.User != nil && m.User.Nickname != "" {
			meta.SenderUserNick = m.User.Nickname
		} else {
			meta.SenderUserNick = m.WhisperSenderUserNick
		}
	}
	if meta.SenderUserName == "" && m.User != nil {
		meta.SenderUserName = m.User.Username
	}
	if meta.TargetMemberName == "" && m.WhisperTarget != nil {
		meta.TargetMemberName = m.WhisperTarget.Nickname
	}
	if meta.TargetUserNick == "" && m.WhisperTarget != nil {
		meta.TargetUserNick = m.WhisperTarget.Nickname
	}
	if meta.TargetUserName == "" && m.WhisperTarget != nil {
		meta.TargetUserName = m.WhisperTarget.Username
	}
	if meta.TargetMemberName == "" && len(meta.TargetDisplayNames) > 0 {
		meta.TargetMemberName = meta.TargetDisplayNames[0]
	}
	// 如果目标 meta 仍全部为空，并且没有 WhisperTo，视为无效
	if meta.TargetUserID == "" {
		meta.TargetUserID = m.WhisperTo
	}
	if meta.SenderUserID == "" && m.UserID != "" {
		meta.SenderUserID = m.UserID
	}
	return meta
}

func (m *MessageModel) ResolveWhisperTargetDisplayNames() []string {
	if m == nil || !m.IsWhisper {
		return nil
	}
	if len(m.WhisperTargetDisplayNames) > 0 {
		return append([]string{}, normalizeWhisperDisplayNames(m.WhisperTargetDisplayNames)...)
	}

	userMap := make(map[string]*UserModel, len(m.WhisperTargets)+1)
	orderedIDs := make([]string, 0, len(m.WhisperTargets)+1)
	seenIDs := map[string]struct{}{}
	addID := func(id string, user *UserModel) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if user != nil {
			userMap[id] = user
		}
		if _, ok := seenIDs[id]; ok {
			return
		}
		seenIDs[id] = struct{}{}
		orderedIDs = append(orderedIDs, id)
	}

	for _, target := range m.WhisperTargets {
		if target == nil {
			continue
		}
		addID(target.ID, target)
	}
	if m.WhisperTarget != nil {
		addID(m.WhisperTarget.ID, m.WhisperTarget)
	}
	for _, id := range splitWhisperTargetIDs(m.WhisperTo) {
		addID(id, nil)
	}

	displayNames := make([]string, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		name := ResolveChannelMappedIdentityDisplayName(m.ChannelID, id, m.ICMode)
		if name == "" && id == strings.TrimSpace(m.WhisperTo) {
			name = strings.TrimSpace(m.WhisperTargetMemberName)
		}
		if name == "" {
			if user := userMap[id]; user != nil {
				name = resolveMessageUserDisplayName(user)
			}
		}
		if name == "" {
			if member, _ := MemberGetByUserIDAndChannelIDBase(id, m.ChannelID, "", false); member != nil {
				name = strings.TrimSpace(member.Nickname)
			}
		}
		if name == "" {
			if user := UserGet(id); user != nil {
				name = resolveMessageUserDisplayName(user)
			}
		}
		if name == "" {
			name = id
		}
		if name != "" {
			displayNames = append(displayNames, name)
		}
	}

	if len(displayNames) == 0 {
		fallback := strings.TrimSpace(m.WhisperTargetMemberName)
		if fallback == "" && m.WhisperTarget != nil {
			fallback = resolveMessageUserDisplayName(m.WhisperTarget)
		}
		if fallback != "" {
			displayNames = append(displayNames, fallback)
		}
	}

	m.WhisperTargetDisplayNames = normalizeWhisperDisplayNames(displayNames)
	return append([]string{}, m.WhisperTargetDisplayNames...)
}

func splitWhisperTargetIDs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func normalizeWhisperDisplayNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	result := make([]string, 0, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func resolveMessageUserDisplayName(user *UserModel) string {
	if user == nil {
		return ""
	}
	if name := strings.TrimSpace(user.Nickname); name != "" {
		return name
	}
	if name := strings.TrimSpace(user.Username); name != "" {
		return name
	}
	return strings.TrimSpace(user.ID)
}

func (m *MessageModel) EnsureWhisperMeta() {
	if !m.IsWhisper {
		m.WhisperMeta = nil
		return
	}
	if m.WhisperMeta == nil {
		m.WhisperMeta = m.buildWhisperMeta()
		return
	}
	if len(m.WhisperMeta.TargetDisplayNames) == 0 {
		m.WhisperMeta.TargetDisplayNames = append([]string{}, m.ResolveWhisperTargetDisplayNames()...)
	}
	if m.WhisperMeta.TargetMemberName == "" && len(m.WhisperMeta.TargetDisplayNames) > 0 {
		m.WhisperMeta.TargetMemberName = m.WhisperMeta.TargetDisplayNames[0]
	}
	if len(m.WhisperMeta.TargetUserIds) == 0 && len(m.WhisperTargets) > 0 {
		targetIDs := make([]string, 0, len(m.WhisperTargets))
		seen := map[string]struct{}{}
		for _, target := range m.WhisperTargets {
			if target == nil || strings.TrimSpace(target.ID) == "" {
				continue
			}
			if _, ok := seen[target.ID]; ok {
				continue
			}
			seen[target.ID] = struct{}{}
			targetIDs = append(targetIDs, target.ID)
		}
		m.WhisperMeta.TargetUserIds = targetIDs
	}
}

func BackfillMessageDisplayOrder() error {
	const batchSize = 500
	for {
		var msgs []MessageModel
		err := db.
			Where("display_order IS NULL OR display_order = 0").
			Order("created_at asc").
			Limit(batchSize).
			Find(&msgs).Error
		if err != nil {
			return err
		}
		if len(msgs) == 0 {
			break
		}
		for _, msg := range msgs {
			order := float64(msg.CreatedAt.UnixMilli())
			if order == 0 {
				order = float64(time.Now().UnixMilli())
			}
			if err := db.Model(&MessageModel{}).
				Where("id = ?", msg.ID).
				UpdateColumn("display_order", order).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func RebalanceChannelDisplayOrder(channelID string) error {
	const batchSize = 500
	offset := 0
	for {
		var msgs []MessageModel
		err := db.Where("channel_id = ?", channelID).
			Order("display_order asc").
			Order("created_at asc").
			Order("id asc").
			Limit(batchSize).
			Offset(offset).
			Find(&msgs).Error
		if err != nil {
			return err
		}
		if len(msgs) == 0 {
			break
		}
		for i, msg := range msgs {
			order := float64(offset+i+1) * displayOrderBaseGap
			if err := db.Model(&MessageModel{}).
				Where("id = ?", msg.ID).
				UpdateColumn("display_order", order).Error; err != nil {
				return err
			}
		}
		offset += len(msgs)
	}
	return nil
}

type MessageEditHistoryModel struct {
	StringPKBaseModel
	MessageID    string `json:"message_id" gorm:"index"`
	EditorID     string `json:"editor_id" gorm:"index"`
	PrevContent  string `json:"prev_content"`
	ChannelID    string `json:"channel_id" gorm:"index"`
	EditedUserID string `json:"edited_user_id" gorm:"index"`
}

func (*MessageEditHistoryModel) TableName() string {
	return "message_edit_histories"
}

func MessagesCountByChannelIDsAfterTime(channelIDs []string, updateTimes []time.Time, userID string) (map[string]int64, error) {
	// updateTimes []int64
	if len(channelIDs) != len(updateTimes) {
		return nil, errors.New("channelIDs和updateTimes长度不匹配")
	}

	var results []struct {
		ChannelID string
		Count     int64
	}

	query := db.Model(&MessageModel{}).
		Select("channel_id, count(*) as count").
		Where("user_id <> ?", userID)

	// 使用gorm的条件构建器
	conditions := db.Where("1 = 0") // 初始为false的条件
	for i, channelID := range channelIDs {
		conditions = conditions.Or(db.Where("channel_id = ? AND created_at > ?", channelID, updateTimes[i]))
	}

	err := query.Where(conditions).
		Group("channel_id").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	// 转换为map
	countMap := make(map[string]int64)
	for _, result := range results {
		countMap[result.ChannelID] = result.Count
	}

	return countMap, nil
}
