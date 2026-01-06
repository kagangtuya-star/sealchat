package model

import "time"

const updateCheckStateID = "update-check"

type UpdateCheckState struct {
	StringPKBaseModel
	CurrentVersion   string `json:"currentVersion"`
	LatestTag        string `json:"latestTag"`
	LatestName       string `json:"latestName"`
	LatestBody       string `json:"latestBody"`
	LatestPublishedAt int64  `json:"latestPublishedAt"`
	LatestHtmlURL    string `json:"latestHtmlUrl"`
	LastCheckedAt    int64  `json:"lastCheckedAt"`
	LastNotifiedTag  string `json:"lastNotifiedTag"`
	ETag             string `json:"-"`
	LastModified     string `json:"-"`
}

func (*UpdateCheckState) TableName() string {
	return "update_check_state"
}

func UpdateCheckStateGet() (*UpdateCheckState, error) {
	var item UpdateCheckState
	if err := db.Where("id = ?", updateCheckStateID).Limit(1).Find(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, nil
	}
	return &item, nil
}

func UpdateCheckStateUpsert(item *UpdateCheckState) error {
	if item == nil {
		return nil
	}
	if item.ID == "" {
		item.ID = updateCheckStateID
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now()
	}
	item.UpdatedAt = time.Now()
	return db.Save(item).Error
}
