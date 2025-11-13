package model

// StorageType 表示文件或媒体资源的存储后端类型
type StorageType string

const (
	StorageLocal StorageType = "local"
	StorageS3    StorageType = "s3"
)
