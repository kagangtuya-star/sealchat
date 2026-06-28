package model

import "gorm.io/gorm"

func BackfillAudioAssetSortOrder() error {
	conn := GetDB()
	if conn == nil {
		return nil
	}
	var assets []AudioAsset
	if err := conn.
		Where("deleted_at IS NULL AND COALESCE(sort_order, 0) = 0").
		Order("updated_at DESC").
		Order("created_at DESC").
		Order("id ASC").
		Find(&assets).Error; err != nil {
		return err
	}
	if len(assets) == 0 {
		return nil
	}
	return conn.Transaction(func(tx *gorm.DB) error {
		for index := range assets {
			if err := tx.Model(&AudioAsset{}).
				Where("id = ?", assets[index].ID).
				Update("sort_order", (index+1)*1000).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
