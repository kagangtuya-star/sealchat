package model

// CountActiveUsers 返回未禁用的注册用户数量。
func CountActiveUsers() (int64, error) {
	var count int64
	err := db.Model(&UserModel{}).Where("disabled = ?", false).Count(&count).Error
	return count, err
}

// CountWorlds 返回处于激活状态的世界数量。
func CountWorlds() (int64, error) {
	var count int64
	err := db.Model(&WorldModel{}).Where("status <> ?", "deleted").Count(&count).Error
	return count, err
}

// CountChannels 返回正常状态的公共频道数量（不含私聊）。
func CountChannels() (int64, error) {
	var count int64
	err := db.Model(&ChannelModel{}).Where("status <> ? AND is_private = ?", "deleted", false).Count(&count).Error
	return count, err
}

// CountPrivateChannels 返回正常状态的私聊频道数量。
func CountPrivateChannels() (int64, error) {
	var count int64
	err := db.Model(&ChannelModel{}).Where("status <> ? AND is_private = ?", "deleted", true).Count(&count).Error
	return count, err
}

// CountMessages 返回未删除的消息数量。
func CountMessages() (int64, error) {
	var count int64
	err := db.Model(&MessageModel{}).Where("is_deleted = ?", false).Count(&count).Error
	return count, err
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
