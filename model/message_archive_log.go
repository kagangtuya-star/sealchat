package model

type MessageArchiveLogModel struct {
	StringPKBaseModel
	MessageID   string `json:"message_id" gorm:"size:100;index"`
	ChannelID   string `json:"channel_id" gorm:"size:100;index"`
	OperatorID  string `json:"operator_id" gorm:"size:100;index"`
	Action      string `json:"action" gorm:"size:32"`
	PayloadJSON string `json:"payload_json" gorm:"type:text"`
}

func (*MessageArchiveLogModel) TableName() string {
	return "message_archive_logs"
}

func MessageArchiveLogBatchCreate(items []*MessageArchiveLogModel) error {
	if len(items) == 0 {
		return nil
	}
	return db.Create(&items).Error
}
