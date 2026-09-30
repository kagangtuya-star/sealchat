package service

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sealchat/model"
	aiService "sealchat/service/ai"
	"sealchat/utils"
)

var ErrTTSConflict = errors.New("语音状态已变化，请刷新")
var ErrTTSDenied = errors.New("无权使用此语音资源")
var ErrTTSDisabled = errors.New("平台语音已禁用或未配置")

// Only explicit business validation messages are safe to return to a client.
type TTSValidationError string

func (e TTSValidationError) Error() string { return string(e) }

// The first actual write serializes quota and voice-slot operations across
// processes and SQL backends. No provider or storage I/O belongs in this callback.
func withTTSUserPolicy(db *gorm.DB, userID string, fn func(*gorm.DB, *model.TTSUserPolicy) error) error {
	if strings.TrimSpace(userID) == "" {
		return ErrTTSDenied
	}
	return db.Transaction(func(tx *gorm.DB) error {
		p := model.TTSUserPolicy{UserID: userID}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoNothing: true}).Create(&p).Error; err != nil {
			return err
		}
		r := tx.Model(&model.TTSUserPolicy{}).Where("user_id = ? AND deleted_at IS NULL", userID).UpdateColumn("revision", gorm.Expr("revision + 1"))
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrTTSConflict
		}
		p = model.TTSUserPolicy{}
		if err := tx.Where("user_id = ? AND deleted_at IS NULL", userID).First(&p).Error; err != nil {
			return err
		}
		return fn(tx, &p)
	})
}

func ttsCost(units int64, price float64) (float64, error) {
	if units < 0 || price < 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return 0, TTSValidationError("语音用量无效")
	}
	cost := math.Round(float64(units)*price*1e6) / 1e6
	if math.IsInf(cost, 0) || cost > 1e12 {
		return 0, TTSValidationError("语音用量超限")
	}
	return cost, nil
}

func ttsReserveJob(db *gorm.DB, cfg *utils.SpeechConfig, job *model.TTSJob, provider utils.SpeechProviderConfig, price float64, now time.Time) error {
	if cfg == nil || job == nil || strings.TrimSpace(job.RequestKey) == "" || len(job.RequestKey) > 128 {
		return ErrTTSConflict
	}
	switch job.Operation {
	case "audition", "message_synthesis", "design", "clone":
	default:
		return ErrTTSConflict
	}
	cost, err := ttsCost(job.EstimatedUnits, price)
	if err != nil {
		return err
	}
	return withTTSUserPolicy(db, job.PayerUserID, func(tx *gorm.DB, p *model.TTSUserPolicy) error {
		var existing model.TTSJob
		if err := tx.Where("request_key = ?", job.RequestKey).First(&existing).Error; err == nil {
			if existing.PayerUserID != job.PayerUserID || existing.Operation != job.Operation || existing.Snapshot != job.Snapshot {
				return ErrTTSConflict
			}
			*job = existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := ttsCapacityLock(tx); err != nil {
			return err
		}
		var capacity model.TTSUserPolicy
		if err := tx.Where("user_id = ?", "__tts_capacity__").First(&capacity).Error; err != nil {
			return err
		}
		job.QueueOrder = capacity.Revision
		var queued int64
		q := tx.Model(&model.TTSJob{}).Where("status IN ?", []string{"queued", "running", "storage_pending", "archiving", "usage_unknown"})
		if err := q.Count(&queued).Error; err != nil {
			return err
		}
		if queued >= 128 {
			return TTSValidationError("语音任务队列已满")
		}
		if job.ChannelID != "" {
			if err := tx.Model(&model.TTSJob{}).Where("channel_id = ? AND status IN ?", job.ChannelID, []string{"queued", "running"}).Count(&queued).Error; err != nil {
				return err
			}
			if queued >= int64(cfg.ChannelQueueLimit) {
				return TTSValidationError("频道语音队列已满")
			}
		}
		if job.UsageStatus == "cached" && job.ResourceID != "" && job.Operation == "message_synthesis" {
			job.Status = "queued"
			return tx.Create(job).Error
		}
		policy := cfg.QuotaDefault
		if p.OverrideEnabled {
			policy = utils.AIQuotaPolicyConfig{DailyLimit: p.DailyLimit, MonthlyLimit: p.MonthlyLimit, LifetimeLimit: p.LifetimeLimit}
		}
		usage, err := aiService.QueryQuotaUsageSnapshotForKind(tx, model.QuotaKindSpeech, job.PayerUserID, now)
		if err != nil {
			return err
		}
		if policy.DailyLimit != nil && usage.DailySettled+usage.ActiveReserved+cost > *policy.DailyLimit || policy.MonthlyLimit != nil && usage.MonthlySettled+usage.ActiveReserved+cost > *policy.MonthlyLimit || policy.LifetimeLimit != nil && usage.LifetimeSettled+usage.ActiveReserved+cost > *policy.LifetimeLimit {
			return TTSValidationError("语音用量不足")
		}
		if job.Operation == "design" || job.Operation == "clone" {
			if provider.AccountVoiceLimit != nil {
				var used int64
				if err := tx.Model(&model.TTSVoice{}).Where("credential_scope = ? AND region = ? AND workspace = ? AND lifecycle <> ?", provider.CredentialScope, provider.Region, provider.Workspace, "deleted").Count(&used).Error; err != nil {
					return err
				}
				if used >= int64(*provider.AccountVoiceLimit) {
					return TTSValidationError("供应商账号音色容量已满")
				}
			}
			var count int64
			if err := tx.Model(&model.TTSVoice{}).Where("owner_user_id = ? AND deleted_at IS NULL AND lifecycle IN ?", job.PayerUserID, []string{"creating", "preview"}).Count(&count).Error; err != nil {
				return err
			}
			if count >= int64(cfg.PreviewLimit) {
				return TTSValidationError("临时音色名额已满")
			}
			expires := now.Add(time.Duration(cfg.PreviewTTLMinutes) * time.Minute)
			voice := model.TTSVoice{OwnerUserID: job.PayerUserID, ProviderID: provider.ID, CredentialScope: provider.CredentialScope, Region: provider.Region, Workspace: provider.Workspace, TargetModel: provider.Model, Kind: job.Operation, Lifecycle: "creating", ProviderStatus: "DEPLOYING", Revision: 1, PreviewExpiresAt: &expires}
			var snapshot TTSSnapshot
			if job.Snapshot != "" {
				if err := json.Unmarshal([]byte(job.Snapshot), &snapshot); err != nil {
					return err
				}
				voice.Name, voice.Description, voice.SourceResourceID = snapshot.Name, snapshot.Description, snapshot.SourceResourceID
			}
			if err := tx.Create(&voice).Error; err != nil {
				return err
			}
			job.VoiceID = voice.ID
		}
		r := model.AIQuotaReservationModel{QuotaKind: model.QuotaKindSpeech, UserID: job.PayerUserID, FeatureKey: job.Operation, ProviderID: provider.ID, Model: provider.Model, ReservedCost: cost, Status: "active", ExpiresAt: now.Add(24 * time.Hour)}
		if err := tx.Create(&r).Error; err != nil {
			return err
		}
		job.ReservationID = r.ID
		job.UnitPrice = price
		job.Status = "queued"
		job.UsageStatus = "reserved"
		return tx.Create(job).Error
	})
}

// Storage completion is deliberately separate from provider usage settlement.
func ttsSettleJob(db *gorm.DB, jobID string, units int64, now time.Time) error {
	var owner model.TTSJob
	if err := db.Select("payer_user_id").Where("id = ?", jobID).First(&owner).Error; err != nil {
		return err
	}
	// Settlement moves money from reservations to the ledger. It must take the
	// same user lock as reservation snapshots, including on READ COMMITTED DBs.
	return withTTSUserPolicy(db, owner.PayerUserID, func(tx *gorm.DB, _ *model.TTSUserPolicy) error {
		var job model.TTSJob
		if err := tx.Where("id = ?", jobID).First(&job).Error; err != nil {
			return err
		}
		price := job.UnitPrice
		cost, err := ttsCost(units, price)
		if err != nil {
			return err
		}
		r := tx.Model(&model.TTSJob{}).Where("id = ? AND usage_status IN ?", jobID, []string{"reserved", "unknown"}).Updates(map[string]any{"usage_status": "settled", "actual_units": units})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			if job.UsageStatus == "settled" {
				return nil
			}
			return ErrTTSConflict
		}
		var reservation model.AIQuotaReservationModel
		if err := tx.Where("id = ? AND quota_kind = ? AND user_id = ?", job.ReservationID, model.QuotaKindSpeech, job.PayerUserID).First(&reservation).Error; err != nil {
			return err
		}
		r = tx.Model(&model.AIQuotaReservationModel{}).Where("id = ? AND quota_kind = ? AND status = ?", reservation.ID, model.QuotaKindSpeech, "active").Update("status", "settled")
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrTTSConflict
		}
		log := model.AIUsageLogModel{QuotaKind: model.QuotaKindSpeech, UserID: job.PayerUserID, FeatureKey: job.Operation, ProviderID: reservation.ProviderID, Model: reservation.Model, Source: "platform", Status: "success", BillingUnits: units, UnitPrice: price, TotalCost: cost, StartedAt: job.CreatedAt, FinishedAt: now}
		if err := tx.Create(&log).Error; err != nil {
			return err
		}
		key := "speech:" + job.ID
		return tx.Create(&model.AIUsageLedgerModel{QuotaKind: model.QuotaKindSpeech, OperationKey: &key, UserID: job.PayerUserID, FeatureKey: job.Operation, ProviderID: reservation.ProviderID, Model: reservation.Model, BillingDay: now.Format("2006-01-02"), BillingMonth: now.Format("2006-01"), TotalCost: cost, LogID: log.ID}).Error
	})
}

func ttsReleaseJob(db *gorm.DB, jobID string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var job model.TTSJob
		if err := tx.Where("id = ?", jobID).First(&job).Error; err != nil {
			return err
		}
		if job.UsageStatus == "cached" {
			return tx.Model(&model.TTSJob{}).Where("id = ? AND status = ?", job.ID, "queued").Update("status", "cancelled").Error
		}
		r := tx.Model(&model.TTSJob{}).Where("id = ? AND status = ? AND usage_status = ?", jobID, "queued", "reserved").Updates(map[string]any{"status": "cancelled", "usage_status": "released"})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrTTSConflict
		}
		return tx.Model(&model.AIQuotaReservationModel{}).Where("id = ? AND quota_kind = ? AND status = ?", job.ReservationID, model.QuotaKindSpeech, "active").Update("status", "released").Error
	})
}
