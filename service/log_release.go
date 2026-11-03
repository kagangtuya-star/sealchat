package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sealchat/model"
	"sealchat/utils"
)

type LogReleaseInput struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	ClientOpID      string `json:"clientOpId"`
}
type logReleaseBuild struct {
	Draft           LogDraft `json:"draft"`
	ExpectedVersion int64    `json:"expectedVersion"`
}

func QueueLogRelease(projectID, userID string, input LogReleaseInput) (map[string]any, error) {
	p, admin, err := loadLogProjectForDesign(projectID, userID)
	if err != nil {
		return nil, err
	}
	if !admin {
		return nil, logError("PERMISSION_DENIED", "仅管理者可以发布")
	}
	if input.ExpectedVersion < 1 || input.ExpectedVersion >= LogMaxVersion || strings.TrimSpace(input.ClientOpID) == "" || len(input.ClientOpID) > 100 {
		return nil, logError("INVALID_INPUT", "发布参数无效")
	}
	b, _ := json.Marshal(struct {
		Project string
		Version int64
	}{projectID, input.ExpectedVersion})
	h := sha256.Sum256(b)
	hash := hex.EncodeToString(h[:])
	var existing model.LogJobModel
	if err := model.GetDB().Where("actor_id = ? AND client_op_id = ?", userID, input.ClientOpID).Take(&existing).Error; err == nil {
		if existing.PayloadHash != hash || existing.Kind != "release" {
			return nil, logError("VERSION_CONFLICT", "操作标识已用于其他请求")
		}
		return map[string]any{"jobId": existing.ID, "status": existing.Status}, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if p.DraftVersion != input.ExpectedVersion {
		return nil, logConflict(p.DraftVersion)
	}
	d, err := parseLogDraft([]byte(p.DraftJSON))
	if err != nil {
		return nil, err
	}
	if d.StylePackageID == "" && d.StylePackageVersion == "" {
		d.StylePackageID = logDefaultStyleID
		d.StylePackageVersion = logDefaultStyleVersion
	}
	if err := validateLogDraftStyle(&d, userID); err != nil {
		return nil, err
	}
	if err := validateLogPublishAnchors(p, &d, userID); err != nil {
		return nil, err
	}
	if err := validateLogProjectAssetsReadable(&d); err != nil {
		return nil, err
	}
	inputJSON, _ := json.Marshal(logReleaseBuild{Draft: d, ExpectedVersion: input.ExpectedVersion})
	job := model.LogJobModel{Kind: "release", ActorID: userID, ClientOpID: input.ClientOpID, ProjectID: projectID, PayloadHash: hash, InputJSON: string(inputJSON), Status: "queued"}
	job.Init()
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		// CAS acquires a write lock without advancing the draft until publication.
		r := tx.Model(&model.LogProjectModel{}).Where("id = ? AND draft_version = ? AND enabled = ?", projectID, input.ExpectedVersion, true).Update("updated_at", time.Now().UTC())
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return logConflict(p.DraftVersion)
		}
		return tx.Create(&job).Error
	})
	if err != nil {
		if model.GetDB().Where("actor_id = ? AND client_op_id = ?", userID, input.ClientOpID).Take(&existing).Error == nil && existing.Kind == "release" && existing.PayloadHash == hash {
			return map[string]any{"jobId": existing.ID, "status": existing.Status}, nil
		}
		return nil, err
	}
	logAudit("release.queue", userID, projectID, job.ID, "OK", input.ExpectedVersion, 0)
	return map[string]any{"jobId": job.ID, "status": "queued"}, nil
}
func GetLogJob(projectID, jobID, userID string) (map[string]any, error) {
	_, admin, err := loadLogProjectForDesign(projectID, userID)
	if err != nil {
		return nil, err
	}
	if !admin {
		return nil, logError("PERMISSION_DENIED", "仅管理者可以读取发布任务")
	}
	var j model.LogJobModel
	if err := model.GetDB().Where("id = ? AND project_id = ? AND kind = ?", jobID, projectID, "release").Take(&j).Error; err != nil {
		return nil, logError("NOT_FOUND", "任务不存在")
	}
	out := map[string]any{"jobId": j.ID, "status": j.Status, "progress": map[string]int64{"done": j.Progress, "total": j.Total}}
	if j.ReleaseID != "" {
		out["releaseId"] = j.ReleaseID
	}
	if j.ErrorCode != "" {
		out["error"] = map[string]string{"code": j.ErrorCode, "message": "发布未完成，请检查工程并重试"}
	}
	return out, nil
}
func deleteLogReleaseSnapshots(tx *gorm.DB, releaseIDs ...string) error {
	ids := make([]string, 0, len(releaseIDs))
	for _, id := range releaseIDs {
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	if err := tx.Where("release_id IN ?", ids).Delete(&model.LogAssetReferenceModel{}).Error; err != nil {
		return err
	}
	if err := tx.Where("release_id IN ?", ids).Delete(&model.LogReleaseAssetModel{}).Error; err != nil {
		return err
	}
	if err := tx.Where("release_id IN ?", ids).Delete(&model.LogReleaseNodeModel{}).Error; err != nil {
		return err
	}
	return tx.Unscoped().Where("id IN ?", ids).Delete(&model.LogReleaseModel{}).Error
}

func WithdrawLogPublication(projectID, userID string, expected int64) error {
	p, admin, err := loadLogProjectForDesign(projectID, userID)
	if err != nil {
		return err
	}
	if !admin {
		return logError("PERMISSION_DENIED", "仅管理者可以下架")
	}
	if expected < 1 || expected >= LogMaxVersion {
		return logError("INVALID_INPUT", "版本无效")
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		r := tx.Model(&model.LogProjectModel{}).Where("id = ? AND draft_version = ?", projectID, expected).Updates(map[string]any{"active_release_id": "", "draft_version": gorm.Expr("draft_version + 1")})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return logConflict(p.DraftVersion)
		}
		var staleIDs []string
		if err := tx.Model(&model.LogReleaseModel{}).Where("project_id = ? AND status <> ?", projectID, "building").Pluck("id", &staleIDs).Error; err != nil {
			return err
		}
		return deleteLogReleaseSnapshots(tx, staleIDs...)
	})
}

var logWorkerOnce sync.Once

func StartLogWorker(ctx context.Context, broadcast func(string, string)) {
	logWorkerOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_ = runLogReleaseJob(ctx)
					_ = runMessageAppearanceJob(ctx, broadcast)
				}
			}
		}()
	})
}
func runLogReleaseJob(ctx context.Context) error {
	var job model.LogJobModel
	now := time.Now().UTC()
	query := model.GetDB().Where("kind = ? AND (status = ? OR (status = ? AND lease_until < ?))", "release", "queued", "running", now).Order("created_at, id").Limit(1).Find(&job)
	if query.Error != nil {
		return query.Error
	}
	if query.RowsAffected == 0 {
		return nil
	}
	token := utils.NewID()
	r := model.GetDB().Model(&model.LogJobModel{}).Where("id = ? AND (status = ? OR (status = ? AND lease_until < ?))", job.ID, "queued", "running", now).Updates(map[string]any{"status": "running", "lease_token": token, "lease_until": now.Add(2 * time.Minute)})
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return nil
	}
	job.LeaseToken = token
	err := buildLogRelease(ctx, &job)
	if err != nil && ctx.Err() == nil {
		code := "INTERNAL_ERROR"
		var domain *LogError
		if errors.As(err, &domain) {
			code = domain.Code
		}
		logAudit("release.build", job.ActorID, job.ProjectID, job.ID, code, 0, job.Progress)
		return model.GetDB().Model(&model.LogJobModel{}).Where("id = ? AND lease_token = ? AND status = ?", job.ID, token, "running").Updates(map[string]any{"status": "failed", "error_code": code}).Error
	}
	if err == nil {
		logAudit("release.build", job.ActorID, job.ProjectID, job.ID, "OK", 0, 0)
	}
	return err
}
func logRenewLease(ctx context.Context, j *model.LogJobModel, done int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r := model.GetDB().Model(&model.LogJobModel{}).Where("id = ? AND lease_token = ? AND status = ? AND lease_until > ?", j.ID, j.LeaseToken, "running", time.Now().UTC()).Updates(map[string]any{"lease_until": time.Now().UTC().Add(2 * time.Minute), "progress": done})
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return logError("VERSION_CONFLICT", "任务租约已失效")
	}
	return nil
}

func buildLogRelease(ctx context.Context, j *model.LogJobModel) (resultErr error) {
	var input logReleaseBuild
	if err := json.Unmarshal([]byte(j.InputJSON), &input); err != nil {
		return err
	}
	p, admin, err := loadLogProjectForDesign(j.ProjectID, j.ActorID)
	if err != nil {
		return err
	}
	if !admin {
		return logError("PERMISSION_DENIED", "发布权限已撤销")
	}
	if p.DraftVersion != input.ExpectedVersion {
		return logConflict(p.DraftVersion)
	}
	if err := validateLogDraftStyle(&input.Draft, j.ActorID); err != nil {
		return err
	}
	d := input.Draft
	sort.SliceStable(d.Sections, func(i, k int) bool { return d.Sections[i].Order < d.Sections[k].Order })
	versions := map[string]int64{}
	total := int64(len(d.Elements))
	for _, s := range d.Sections {
		row := model.LogChannelVersionModel{ChannelID: s.Source.ChannelID}
		if err := model.GetDB().Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		v, err := logSourceVersion(s.Source.ChannelID)
		if err != nil {
			return err
		}
		versions[s.Source.ChannelID] = v
		var count int64
		if err := logSourceQuery(p, s, j.ActorID).Where("is_whisper = ? AND is_revoked = ?", false, false).Count(&count).Error; err != nil {
			return err
		}
		total += count
		if total > 1000000 {
			return logError("TOO_LARGE", "发布节点超过一百万项")
		}
	}
	if err := model.GetDB().Model(&model.LogJobModel{}).Where("id = ? AND lease_token = ?", j.ID, j.LeaseToken).Updates(map[string]any{"total": total, "progress": 0}).Error; err != nil {
		return err
	}
	release := model.LogReleaseModel{ProjectID: p.ID, DraftVersion: p.DraftVersion, Status: "building", DraftJSON: p.DraftJSON}
	release.Init()
	if err := model.GetDB().Create(&release).Error; err != nil {
		return err
	}
	defer func() {
		if resultErr != nil {
			_ = model.GetDB().Transaction(func(tx *gorm.DB) error {
				return deleteLogReleaseSnapshots(tx, release.ID)
			})
		}
	}()
	if err := logBuildStyleAssets(&release, &d); err != nil {
		return err
	}
	projectKeys, err := logBuildProjectAssets(ctx, &release, &d)
	if err != nil {
		return err
	}
	elements, err := logDraftElements(&d)
	if err != nil {
		return err
	}
	sort.SliceStable(elements, func(i, k int) bool { return elements[i].Order < elements[k].Order })
	ordinal := int64(0)
	used := map[string]bool{}
	// Keep the public clue boundary, but archive member-private clue sections that
	// the publishing actor is authorized to read into the release snapshot.
	display := newMessageDisplayResolver("", j.ActorID, messageDisplayLogArchive)
	imageWidths := logMessageImageWidths(&d)
	for _, section := range d.Sections {
		addElements := func(kind, messageID, side string) error {
			for _, e := range elements {
				if e.Anchor.SectionID != section.ID || e.Anchor.Kind != kind || e.Anchor.MessageID != messageID || e.Anchor.Side != side {
					continue
				}
				ordinal++
				if err := logBuildElement(&release, e, ordinal, projectKeys); err != nil {
					return err
				}
				used[e.ID] = true
			}
			return nil
		}
		if err := addElements("sectionStart", "", ""); err != nil {
			return err
		}
		var last *model.MessageModel
		for {
			if err := logRenewLease(ctx, j, ordinal); err != nil {
				return err
			}
			q := logSourceQuery(p, section, j.ActorID).Where("is_whisper = ? AND is_revoked = ?", false, false)
			if last != nil {
				q = logOrderAfter(q, last.DisplayOrder, last.CreatedAt, last.ID, true)
			}
			var rows []model.MessageModel
			if err := q.Preload("User", func(tx *gorm.DB) *gorm.DB { return tx.Select("id, is_bot") }).Order("display_order, created_at, id").Limit(100).Find(&rows).Error; err != nil {
				return err
			}
			for i := range rows {
				m := &rows[i]
				if err := addElements("message", m.ID, "before"); err != nil {
					return err
				}
				ordinal++
				if ordinal > 1000000 {
					return logError("TOO_LARGE", "发布节点超过上限")
				}
				display = display.forChannel(m.ChannelID)
				if err := logBuildMessage(ctx, &release, display, section.ID, m, ordinal, imageWidths[m.ID], section.Source.ShowQuoteContent); err != nil {
					return err
				}
				if err := addElements("message", m.ID, "after"); err != nil {
					return err
				}
			}
			if len(rows) < 100 {
				break
			}
			last = &rows[len(rows)-1]
		}
		if err := addElements("sectionEnd", "", ""); err != nil {
			return err
		}
	}
	if len(used) != len(elements) {
		ids := []string{}
		for _, e := range elements {
			if !used[e.ID] && len(ids) < 200 {
				ids = append(ids, e.ID)
			}
		}
		return &LogError{Code: "INVALID_TEMPLATE", Message: "存在不可发布的孤立元素锚点", Details: map[string]any{"elementIds": ids, "total": len(elements) - len(used)}}
	}
	for channel, v := range versions {
		current, err := logSourceVersion(channel)
		if err != nil {
			return err
		}
		if current != v {
			return logError("SOURCE_CHANGED", "构建期间来源已变化")
		}
	}
	if _, admin, err := loadLogProjectForDesign(j.ProjectID, j.ActorID); err != nil {
		return err
	} else if !admin {
		return logError("PERMISSION_DENIED", "发布权限已撤销")
	}
	metadata, err := logBuildPublicMetadata(&release, &d, projectKeys)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(metadata)
	v, _ := json.Marshal(versions)
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		claim := tx.Model(&model.LogJobModel{}).Where("id = ? AND lease_token = ? AND status = ? AND lease_until > ?", j.ID, j.LeaseToken, "running", time.Now().UTC()).Updates(map[string]any{"status": "done", "release_id": release.ID, "progress": ordinal, "total": ordinal})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return logError("VERSION_CONFLICT", "任务租约已失效")
		}
		for channel, version := range versions {
			var row model.LogChannelVersionModel
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("channel_id = ?", channel).Take(&row).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if row.Version != version {
				return logError("SOURCE_CHANGED", "构建期间来源已变化")
			}
		}
		r := tx.Model(&model.LogProjectModel{}).Where("id = ? AND enabled = ? AND draft_version = ?", p.ID, true, input.ExpectedVersion).Updates(map[string]any{"active_release_id": release.ID, "draft_version": gorm.Expr("draft_version + 1")})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return logConflict(p.DraftVersion)
		}
		if err := tx.Model(&release).Updates(map[string]any{"status": "active", "public_json": string(b), "source_versions_json": string(v)}).Error; err != nil {
			return err
		}
		var staleIDs []string
		if err := tx.Model(&model.LogReleaseModel{}).Where("project_id = ? AND id <> ?", p.ID, release.ID).Pluck("id", &staleIDs).Error; err != nil {
			return err
		}
		return deleteLogReleaseSnapshots(tx, staleIDs...)
	})
}
