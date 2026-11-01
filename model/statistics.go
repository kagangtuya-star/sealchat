package model

import "time"

type MessageStatusStats struct {
	TotalMessages int64 `json:"totalMessages"`
	ICMessages    int64 `json:"icMessages"`
	OOCMessages   int64 `json:"oocMessages"`
	TotalChars    int64 `json:"totalChars"`
	ICChars       int64 `json:"icChars"`
	OOCChars      int64 `json:"oocChars"`
}

type AttachmentStatusStats struct {
	TotalCount int64 `json:"totalCount"`
	TotalBytes int64 `json:"totalBytes"`
	ImageCount int64 `json:"imageCount"`
	ImageBytes int64 `json:"imageBytes"`
	FontCount  int64 `json:"fontCount"`
	FontBytes  int64 `json:"fontBytes"`
}

// CountActiveUsers 返回未禁用的非 BOT 注册用户数量。
func CountActiveUsers() (int64, error) {
	var count int64
	err := db.Model(&UserModel{}).
		Where("disabled = ? AND is_bot = ?", false, false).
		Count(&count).Error
	return count, err
}

// CountWorlds 返回处于激活状态的世界数量。
func CountWorlds() (int64, error) {
	var count int64
	err := db.Model(&WorldModel{}).Where("status NOT IN ?", []string{"deleted", "archived"}).Count(&count).Error
	return count, err
}

// CountChannels 返回正常状态的公共频道数量（不含私聊）。
func CountChannels() (int64, error) {
	var count int64
	err := db.Model(&ChannelModel{}).Where("status NOT IN ? AND is_private = ?", []string{ChannelStatusDeleted, ChannelStatusArchived}, false).Count(&count).Error
	return count, err
}

// CountPrivateChannels 返回正常状态的私聊频道数量。
func CountPrivateChannels() (int64, error) {
	var count int64
	err := db.Model(&ChannelModel{}).Where("status NOT IN ? AND is_private = ?", []string{ChannelStatusDeleted, ChannelStatusArchived}, true).Count(&count).Error
	return count, err
}

// CountMessages 返回未删除的消息数量。
func CountMessages() (int64, error) {
	var count int64
	err := db.Model(&MessageModel{}).Where("is_deleted = ?", false).Count(&count).Error
	return count, err
}

// CountMessageStatusStats 返回未删除消息的总数/总字数，以及 IC/OOC 拆分。
func CountMessageStatusStats() (*MessageStatusStats, error) {
	type rawResult struct {
		TotalMessages int64 `gorm:"column:total_messages"`
		ICMessages    int64 `gorm:"column:ic_messages"`
		OOCMessages   int64 `gorm:"column:ooc_messages"`
		TotalChars    int64 `gorm:"column:total_chars"`
		ICChars       int64 `gorm:"column:ic_chars"`
		OOCChars      int64 `gorm:"column:ooc_chars"`
	}

	lenExpr := visibleCharCountExpr("visible_char_count")
	modeExpr := "LOWER(COALESCE(NULLIF(ic_mode, ''), 'ic'))"

	var result rawResult
	err := db.Model(&MessageModel{}).
		Where("is_deleted = ?", false).
		Select(
			"COUNT(*) AS total_messages, " +
				"SUM(CASE WHEN " + modeExpr + " = 'ooc' THEN 0 ELSE 1 END) AS ic_messages, " +
				"SUM(CASE WHEN " + modeExpr + " = 'ooc' THEN 1 ELSE 0 END) AS ooc_messages, " +
				"COALESCE(SUM(" + lenExpr + "), 0) AS total_chars, " +
				"COALESCE(SUM(CASE WHEN " + modeExpr + " = 'ooc' THEN 0 ELSE " + lenExpr + " END), 0) AS ic_chars, " +
				"COALESCE(SUM(CASE WHEN " + modeExpr + " = 'ooc' THEN " + lenExpr + " ELSE 0 END), 0) AS ooc_chars",
		).
		Scan(&result).Error
	if err != nil {
		return nil, err
	}

	return &MessageStatusStats{
		TotalMessages: result.TotalMessages,
		ICMessages:    result.ICMessages,
		OOCMessages:   result.OOCMessages,
		TotalChars:    result.TotalChars,
		ICChars:       result.ICChars,
		OOCChars:      result.OOCChars,
	}, nil
}

// WorldMessageCharStats 是单个世界内未删除消息的字数与最近发言统计。
type WorldMessageCharStats struct {
	WorldID      string `gorm:"column:world_id"`
	Chars        int64  `gorm:"column:total_chars"`
	CharsIC      int64  `gorm:"column:ic_chars"`
	CharsOOC     int64  `gorm:"column:ooc_chars"`
	LastActiveAt int64  `gorm:"column:last_active_at"`
}

// CountWorldMessageChars 汇总正常世界中未删除消息的可见字数和最近发言时间，包含归档频道。
func CountWorldMessageChars(worldIDs []string) ([]WorldMessageCharStats, error) {
	if len(worldIDs) == 0 {
		return nil, nil
	}
	lenExpr := visibleCharCountExpr("m.visible_char_count")
	modeExpr := "LOWER(COALESCE(NULLIF(m.ic_mode, ''), 'ic'))"
	lastActiveExpr := "MAX(m.created_at) AS last_message_at"
	if IsSQLite() {
		// 先转换再取最大值，避免历史消息的时区和日期字符串格式影响排序。
		lastActiveExpr = "COALESCE(CAST(ROUND((MAX(julianday(m.created_at)) - 2440587.5) * 86400000) AS BIGINT), 0) AS last_active_at"
	}

	var messageRows []struct {
		WorldMessageCharStats
		LastMessageAt *time.Time `gorm:"column:last_message_at"`
	}
	err := db.Table("messages AS m").
		Select(
			"c.world_id AS world_id, "+
				"COALESCE(SUM("+lenExpr+"), 0) AS total_chars, "+
				"COALESCE(SUM(CASE WHEN "+modeExpr+" = 'ooc' THEN 0 ELSE "+lenExpr+" END), 0) AS ic_chars, "+
				"COALESCE(SUM(CASE WHEN "+modeExpr+" = 'ooc' THEN "+lenExpr+" ELSE 0 END), 0) AS ooc_chars, "+
				lastActiveExpr,
		).
		Joins("JOIN channels c ON c.id = m.channel_id").
		Joins("JOIN worlds w ON w.id = c.world_id").
		Where("c.world_id IN ? AND m.is_deleted = ? AND c.status <> ? AND w.status NOT IN ?", worldIDs, false, ChannelStatusDeleted, []string{"deleted", "archived"}).
		Group("c.world_id").
		Scan(&messageRows).Error
	if err != nil {
		return nil, err
	}
	rows := make([]WorldMessageCharStats, 0, len(messageRows))
	for _, row := range messageRows {
		// 沿用驱动解析的时区，避免 MySQL 会话时区与连接 loc 不一致时发生偏移。
		if row.LastMessageAt != nil && !row.LastMessageAt.IsZero() {
			row.LastActiveAt = row.LastMessageAt.UnixMilli()
		}
		rows = append(rows, row.WorldMessageCharStats)
	}
	return rows, nil
}

// CountAttachments 返回正式附件数量（不含临时附件）。
func CountAttachments() (int64, error) {
	var count int64
	err := db.Model(&AttachmentModel{}).Where("is_temp = ?", false).Count(&count).Error
	return count, err
}

// SumAttachmentSizes 返回正式附件总大小（不含临时附件）。
func SumAttachmentSizes() (int64, error) {
	var total int64
	err := db.Model(&AttachmentModel{}).
		Where("is_temp = ?", false).
		Select("COALESCE(SUM(size), 0)").
		Scan(&total).Error
	return total, err
}

// CountAttachmentStatusStats 返回正式图片附件与平台字体资源的数量/大小统计。
func CountAttachmentStatusStats() (*AttachmentStatusStats, error) {
	type attachmentResult struct {
		ImageCount int64 `gorm:"column:image_count"`
		ImageBytes int64 `gorm:"column:image_bytes"`
	}
	type fontResult struct {
		FontCount int64 `gorm:"column:font_count"`
		FontBytes int64 `gorm:"column:font_bytes"`
	}

	var attachment attachmentResult
	if err := db.Model(&AttachmentModel{}).
		Where("is_temp = ?", false).
		Select(
			"COALESCE(SUM(CASE WHEN LOWER(COALESCE(mime_type, '')) LIKE 'image/%' THEN 1 ELSE 0 END), 0) AS image_count, " +
				"COALESCE(SUM(CASE WHEN LOWER(COALESCE(mime_type, '')) LIKE 'image/%' THEN size ELSE 0 END), 0) AS image_bytes",
		).
		Scan(&attachment).Error; err != nil {
		return nil, err
	}

	var font fontResult
	if err := db.Model(&PlatformFontAsset{}).
		Where("status = ?", PlatformFontStatusReady).
		Select(
			"COALESCE(SUM(CASE WHEN storage_file_count > 0 THEN storage_file_count ELSE 1 END), 0) AS font_count, " +
				"COALESCE(SUM(CASE WHEN storage_total_bytes > 0 THEN storage_total_bytes ELSE source_size END), 0) AS font_bytes",
		).
		Scan(&font).Error; err != nil {
		return nil, err
	}

	return &AttachmentStatusStats{
		TotalCount: attachment.ImageCount + font.FontCount,
		TotalBytes: attachment.ImageBytes + font.FontBytes,
		ImageCount: attachment.ImageCount,
		ImageBytes: attachment.ImageBytes,
		FontCount:  font.FontCount,
		FontBytes:  font.FontBytes,
	}, nil
}
