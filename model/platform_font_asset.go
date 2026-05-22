package model

import "time"

type PlatformFontStatus string

const (
	PlatformFontStatusProcessing PlatformFontStatus = "processing"
	PlatformFontStatusReady      PlatformFontStatus = "ready"
	PlatformFontStatusFailed     PlatformFontStatus = "failed"
	PlatformFontStatusDisabled   PlatformFontStatus = "disabled"
)

type PlatformFontDeliveryMode string

const (
	PlatformFontDeliverySingle PlatformFontDeliveryMode = "single"
	PlatformFontDeliverySubset PlatformFontDeliveryMode = "subset"
)

type PlatformFontAsset struct {
	StringPKBaseModel
	DisplayName         string                   `json:"displayName" gorm:"index"`
	Family              string                   `json:"family" gorm:"index"`
	Weight              string                   `json:"weight"`
	Style               string                   `json:"style"`
	Status              PlatformFontStatus       `json:"status" gorm:"type:varchar(16);index"`
	DeliveryMode        PlatformFontDeliveryMode `json:"deliveryMode" gorm:"type:varchar(16)"`
	OriginalStorageType StorageType              `json:"originalStorageType" gorm:"type:varchar(16)"`
	OriginalObjectKey   string                   `json:"originalObjectKey"`
	SubsetStorageType   StorageType              `json:"subsetStorageType" gorm:"type:varchar(16)"`
	SubsetObjectKey     string                   `json:"subsetObjectKey"`
	ManifestStorageType StorageType              `json:"manifestStorageType" gorm:"type:varchar(16)"`
	ManifestObjectKey   string                   `json:"manifestObjectKey"`
	PreviewText         string                   `json:"previewText"`
	SourceFileName      string                   `json:"sourceFileName"`
	SourceMimeType      string                   `json:"sourceMimeType"`
	SourceSize          int64                    `json:"sourceSize"`
	SubsetCount         int                      `json:"subsetCount"`
	LastError           string                   `json:"lastError"`
	CreatedBy           string                   `json:"createdBy" gorm:"index"`
	UpdatedBy           string                   `json:"updatedBy"`
	LastPublishedAt     *time.Time               `json:"lastPublishedAt"`
}

func (*PlatformFontAsset) TableName() string { return "platform_font_assets" }

