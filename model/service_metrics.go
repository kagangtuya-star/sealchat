package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// ServiceMetricSample 用于持久化系统运行状态快照。
type ServiceMetricSample struct {
	ID                    uint      `gorm:"primaryKey" json:"id"`
	TimestampMs           int64     `gorm:"uniqueIndex;not null" json:"timestampMs"`
	ConcurrentConnections int64     `json:"concurrentConnections"`
	OnlineUsers           int64     `json:"onlineUsers"`
	MessagesPerMinute     int64     `json:"messagesPerMinute"`
	RegisteredUsers       int64     `json:"registeredUsers"`
	WorldCount            int64     `json:"worldCount"`
	ChannelCount          int64     `json:"channelCount"`
	PrivateChannelCount   int64     `json:"privateChannelCount"`
	MessageCount          int64     `json:"messageCount"`
	AttachmentCount       int64     `json:"attachmentCount"`
	AttachmentBytes       int64     `json:"attachmentBytes"`
	CreatedAt             time.Time `json:"createdAt"`
}

func (*ServiceMetricSample) TableName() string {
	return "service_metrics"
}

// InsertServiceMetricSample 写入一条采样数据。
func InsertServiceMetricSample(sample *ServiceMetricSample) error {
	if sample == nil {
		return nil
	}
	return db.Create(sample).Error
}

// GetLatestServiceMetricSample 返回数据库中最新的采样记录。
func GetLatestServiceMetricSample() (*ServiceMetricSample, error) {
	var sample ServiceMetricSample
	err := db.Order("timestamp_ms desc").Limit(1).Take(&sample).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if sample.TimestampMs == 0 {
		return nil, nil
	}
	return &sample, nil
}

// QueryServiceMetricSamples 根据时间范围获取历史采样数据。
func QueryServiceMetricSamples(startMs, endMs int64) ([]ServiceMetricSample, error) {
	query := db.Model(&ServiceMetricSample{}).Order("timestamp_ms asc")
	if startMs > 0 {
		query = query.Where("timestamp_ms >= ?", startMs)
	}
	if endMs > 0 {
		query = query.Where("timestamp_ms <= ?", endMs)
	}
	var samples []ServiceMetricSample
	if err := query.Find(&samples).Error; err != nil {
		return nil, err
	}
	return samples, nil
}

// DeleteServiceMetricBefore 删除早于指定时间戳的记录。
func DeleteServiceMetricBefore(cutoffMs int64) error {
	if cutoffMs <= 0 {
		return nil
	}
	return db.Where("timestamp_ms < ?", cutoffMs).Delete(&ServiceMetricSample{}).Error
}
