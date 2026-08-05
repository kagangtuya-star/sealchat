package model

import "time"

const updateJobStateID = "update-job"

// UpdateJobState records an update across the process restart boundary.
type UpdateJobState struct {
	StringPKBaseModel
	Status          string `json:"status"`
	Channel         string `json:"channel"`
	TargetVersion   string `json:"targetVersion"`
	ReleaseID       int64  `json:"releaseId"`
	AssetID         int64  `json:"assetId"`
	AssetName       string `json:"assetName"`
	Progress        int    `json:"progress"`
	Message         string `json:"message"`
	Error           string `json:"error"`
	StartedAt       int64  `json:"startedAt"`
	FinishedAt      int64  `json:"finishedAt"`
	PreviousVersion string `json:"previousVersion"`
}

func (*UpdateJobState) TableName() string {
	return "update_job_state"
}

func UpdateJobStateGet() (*UpdateJobState, error) {
	var item UpdateJobState
	if err := db.Where("id = ?", updateJobStateID).Limit(1).Find(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, nil
	}
	return &item, nil
}

func UpdateJobStateUpsert(item *UpdateJobState) error {
	if item == nil {
		return nil
	}
	if item.ID == "" {
		item.ID = updateJobStateID
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now()
	}
	item.UpdatedAt = time.Now()
	return db.Save(item).Error
}
