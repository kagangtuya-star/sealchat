package model

// StorageType 表示文件或媒体资源的存储后端类型
type StorageType string

const (
	StorageLocal StorageType = "local"
	StorageS3    StorageType = "s3"
	StorageFontLocal StorageType = "font_local"
	StorageFontS3    StorageType = "font_s3"
)
