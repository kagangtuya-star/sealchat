package service

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sealchat/model"
	"sealchat/pm"
	aiService "sealchat/service/ai"
	"sealchat/utils"
)

var ErrTTSWorldDenied = errors.New("此世界尚未启用 AI 语音")

type TTSWorldAccess struct {
	Enabled     bool                          `json:"enabled"`
	WorldID     string                        `json:"worldId"`
	Allowed     bool                          `json:"allowed"`
	Reason      string                        `json:"reason"`
	CanActivate bool                          `json:"canActivate"`
	Policy      model.TTSWorldPolicy          `json:"policy"`
	Usage       *aiService.QuotaUsageSnapshot `json:"usage"`
}

func ttsChannelWorld(db *gorm.DB, channelID string) (string, error) {
	var channel model.ChannelModel
	if err := db.Where("id = ? AND deleted_at IS NULL AND status <> ?", channelID, model.ChannelStatusDeleted).First(&channel).Error; err != nil {
		return "", err
	}
	if channel.WorldID == "" {
		return "", TTSValidationError("语音请求需要有效的世界归属")
	}
	var world model.WorldModel
	if err := db.Select("id").Where("id = ? AND status = ? AND deleted_at IS NULL", channel.WorldID, "active").First(&world).Error; err != nil {
		return "", err
	}
	return world.ID, nil
}

func ttsReadWorldPolicy(db *gorm.DB, worldID string) (model.TTSWorldPolicy, error) {
	p := model.TTSWorldPolicy{WorldID: worldID}
	err := db.Where("world_id = ?", worldID).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	return p, err
}

// Called after the user lock and before the capacity lock. An actual UPDATE
// locks the row on every supported backend, including SQLite's writer lock.
func ttsLockWorldPolicy(tx *gorm.DB, worldID string) (model.TTSWorldPolicy, error) {
	p := model.TTSWorldPolicy{WorldID: worldID}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "world_id"}}, DoNothing: true}).Create(&p).Error; err != nil {
		return p, err
	}
	if err := tx.Model(&p).UpdateColumn("revision", gorm.Expr("revision + 1")).Error; err != nil {
		return p, err
	}
	return ttsReadWorldPolicy(tx, worldID)
}

func ttsWorldAllowed(cfg *utils.SpeechConfig, policy model.TTSWorldPolicy) bool {
	return cfg != nil && (cfg.WorldAccessMode != "whitelist" || policy.Allowlisted)
}

func ttsCanManageWorld(actorID string, world *model.WorldModel) bool {
	return actorID != "" && (world.OwnerID == actorID || IsWorldAdmin(world.ID, actorID))
}

func ResolveTTSWorldAccess(actorID, channelID string) (*TTSWorldAccess, error) {
	channelID = strings.TrimSpace(channelID)
	if !pm.CanWithChannelRole(actorID, channelID, pm.PermFuncChannelRead, pm.PermFuncChannelReadAll) {
		return nil, ErrTTSDenied
	}
	return resolveTTSWorldAccess(actorID, channelID)
}

// Role auditions already authorize their identity through TTSRoleConfig.
func resolveTTSWorldAccess(actorID, channelID string) (*TTSWorldAccess, error) {
	db := model.GetDB()
	worldID, err := ttsChannelWorld(db, channelID)
	if err != nil {
		return nil, err
	}
	world, err := GetWorldByID(worldID)
	if err != nil {
		return nil, err
	}
	p, err := ttsReadWorldPolicy(db, worldID)
	if err != nil {
		return nil, err
	}
	usage, err := ttsWorldUsage(db, []string{worldID}, time.Now())
	if err != nil {
		return nil, err
	}
	access := &TTSWorldAccess{WorldID: worldID, Policy: p, Usage: usage[worldID]}
	cfg, err := ttsConfig()
	if errors.Is(err, ErrTTSDisabled) {
		access.Reason = "platform_disabled"
		return access, nil
	}
	if err != nil {
		return nil, err
	}
	access.Allowed = ttsWorldAllowed(cfg, p)
	access.Enabled = access.Allowed
	if !access.Allowed {
		access.Reason = "world_not_allowlisted"
		access.CanActivate = cfg.WorldActivationCode != "" && ttsCanManageWorld(actorID, world)
	}
	return access, nil
}

func TTSQuotaForChannel(userID, channelID string) (TTSQuota, error) {
	q, err := TTSQuotaForUser(userID)
	if err != nil || strings.TrimSpace(channelID) == "" {
		return q, err
	}
	q.WorldAccess, err = ResolveTTSWorldAccess(userID, channelID)
	if err == nil {
		q.Enabled = q.Enabled && q.WorldAccess.Enabled
	}
	return q, err
}

// Speech reservations do not expire implicitly: unknown upstream usage remains
// occupied until the existing explicit settlement/release flow resolves it.
func ttsWorldUsage(db *gorm.DB, worldIDs []string, now time.Time) (map[string]*aiService.QuotaUsageSnapshot, error) {
	result := make(map[string]*aiService.QuotaUsageSnapshot, len(worldIDs))
	for _, id := range worldIDs {
		result[id] = &aiService.QuotaUsageSnapshot{}
	}
	if len(worldIDs) == 0 {
		return result, nil
	}
	var settled []struct {
		WorldID string
		aiService.QuotaUsageSnapshot
	}
	err := db.Model(&model.AIUsageLedgerModel{}).
		Where("quota_kind = ? AND world_id IN ?", model.QuotaKindSpeech, worldIDs).
		Select("world_id, COALESCE(SUM(CASE WHEN billing_day = ? THEN total_cost ELSE 0 END), 0) AS daily_settled, COALESCE(SUM(CASE WHEN billing_month = ? THEN total_cost ELSE 0 END), 0) AS monthly_settled, COALESCE(SUM(total_cost), 0) AS lifetime_settled", now.Format("2006-01-02"), now.Format("2006-01")).
		Group("world_id").Scan(&settled).Error
	if err != nil {
		return nil, err
	}
	for _, row := range settled {
		*result[row.WorldID] = row.QuotaUsageSnapshot
	}
	var reserved []struct {
		WorldID string
		Cost    float64
	}
	if err := db.Model(&model.AIQuotaReservationModel{}).
		Where("quota_kind = ? AND world_id IN ? AND status = ?", model.QuotaKindSpeech, worldIDs, "active").
		Select("world_id, COALESCE(SUM(reserved_cost), 0) AS cost").Group("world_id").Scan(&reserved).Error; err != nil {
		return nil, err
	}
	for _, row := range reserved {
		result[row.WorldID].ActiveReserved = row.Cost
	}
	return result, nil
}

func ttsWorldQuotaAvailable(tx *gorm.DB, p model.TTSWorldPolicy, cost float64, now time.Time) error {
	if !p.QuotaOverrideEnabled {
		return nil
	}
	usage, err := ttsWorldUsage(tx, []string{p.WorldID}, now)
	if err != nil {
		return err
	}
	u := usage[p.WorldID]
	if p.DailyLimit != nil && u.DailySettled+u.ActiveReserved+cost > *p.DailyLimit ||
		p.MonthlyLimit != nil && u.MonthlySettled+u.ActiveReserved+cost > *p.MonthlyLimit ||
		p.LifetimeLimit != nil && u.LifetimeSettled+u.ActiveReserved+cost > *p.LifetimeLimit {
		return TTSValidationError("世界语音用量不足")
	}
	return nil
}

func ActivateTTSWorld(actorID, worldID, code string) error {
	world, err := GetWorldByID(strings.TrimSpace(worldID))
	if errors.Is(err, ErrWorldNotFound) {
		return gorm.ErrRecordNotFound
	}
	if err != nil {
		return err
	}
	if world.Status != "active" || world.DeletedAt != nil {
		return gorm.ErrRecordNotFound
	}
	if !ttsCanManageWorld(actorID, world) {
		return ErrTTSDenied
	}
	cfg, err := ttsConfig()
	if err != nil {
		return err
	}
	if cfg.WorldActivationCode == "" {
		return TTSValidationError("未开放世界语音自助激活")
	}
	expected, supplied := sha256.Sum256([]byte(cfg.WorldActivationCode)), sha256.Sum256([]byte(code))
	if subtle.ConstantTimeCompare(expected[:], supplied[:]) != 1 {
		return TTSValidationError("激活码无效")
	}
	now := time.Now()
	p := model.TTSWorldPolicy{WorldID: world.ID, Allowlisted: true, ActivatedBy: actorID, ActivatedAt: &now}
	return model.GetDB().Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "world_id"}}, DoUpdates: clause.Assignments(map[string]any{
		"allowlisted": true, "activated_by": actorID, "activated_at": now, "updated_at": now,
	})}).Create(&p).Error
}

type TTSWorldPatch struct {
	Allowlisted          *bool    `json:"allowlisted"`
	QuotaOverrideEnabled *bool    `json:"quotaOverrideEnabled"`
	DailyLimit           *float64 `json:"dailyLimit"`
	MonthlyLimit         *float64 `json:"monthlyLimit"`
	LifetimeLimit        *float64 `json:"lifetimeLimit"`
}

// present distinguishes an omitted limit from an explicit null (unlimited).
func TTSPatchWorldPolicy(worldID string, patch TTSWorldPatch, present map[string]bool) error {
	world, err := GetWorldByID(worldID)
	if errors.Is(err, ErrWorldNotFound) {
		return gorm.ErrRecordNotFound
	}
	if err != nil {
		return err
	}
	if world.Status != "active" || world.DeletedAt != nil {
		return gorm.ErrRecordNotFound
	}
	updates := map[string]any{}
	for key, limit := range map[string]*float64{"dailyLimit": patch.DailyLimit, "monthlyLimit": patch.MonthlyLimit, "lifetimeLimit": patch.LifetimeLimit} {
		if limit != nil && (*limit < 0 || math.IsNaN(*limit) || math.IsInf(*limit, 0)) {
			return TTSValidationError("世界语音限额无效")
		}
		if present[key] {
			column := map[string]string{"dailyLimit": "daily_limit", "monthlyLimit": "monthly_limit", "lifetimeLimit": "lifetime_limit"}[key]
			updates[column] = limit
		}
	}
	if patch.Allowlisted != nil {
		updates["allowlisted"] = *patch.Allowlisted
	}
	if patch.QuotaOverrideEnabled != nil {
		updates["quota_override_enabled"] = *patch.QuotaOverrideEnabled
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		if _, err := ttsLockWorldPolicy(tx, worldID); err != nil {
			return err
		}
		return tx.Model(&model.TTSWorldPolicy{}).Where("world_id = ?", worldID).Updates(updates).Error
	})
}

type AdminTTSWorld struct {
	WorldID       string                        `json:"worldId"`
	Name          string                        `json:"name"`
	OwnerID       string                        `json:"ownerId"`
	OwnerUsername string                        `json:"ownerUsername"`
	OwnerNickname string                        `json:"ownerNickname"`
	Policy        model.TTSWorldPolicy          `json:"policy" gorm:"-"`
	Usage         *aiService.QuotaUsageSnapshot `json:"usage" gorm:"-"`
	LastUsedAt    *time.Time                    `json:"lastUsedAt" gorm:"-"`
}

type AdminTTSWorldList struct {
	Items    []AdminTTSWorld `json:"items"`
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"pageSize"`
}

func AdminListTTSWorlds(page, size int, search, worldID string) (*AdminTTSWorldList, error) {
	page, size = max(page, 1), max(1, min(size, 100))
	db := model.GetDB()
	q := db.Model(&model.WorldModel{}).Where("worlds.status = ? AND worlds.deleted_at IS NULL", "active")
	if worldID != "" {
		q = q.Where("worlds.id = ?", worldID)
	}
	if search = strings.TrimSpace(search); search != "" {
		q = q.Where("worlds.name LIKE ? OR worlds.id LIKE ?", "%"+search+"%", "%"+search+"%")
	}
	result := &AdminTTSWorldList{Items: []AdminTTSWorld{}, Page: page, PageSize: size}
	if err := q.Count(&result.Total).Error; err != nil {
		return nil, err
	}
	if err := q.Joins("LEFT JOIN users ON users.id = worlds.owner_id").
		Select("worlds.id AS world_id, worlds.name, worlds.owner_id, users.username AS owner_username, users.nickname AS owner_nickname").
		Order("worlds.created_at DESC, worlds.id DESC").Offset((page - 1) * size).Limit(size).Scan(&result.Items).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		ids = append(ids, item.WorldID)
	}
	if len(ids) == 0 {
		return result, nil
	}
	var policies []model.TTSWorldPolicy
	if err := db.Where("world_id IN ?", ids).Find(&policies).Error; err != nil {
		return nil, err
	}
	byWorld := map[string]model.TTSWorldPolicy{}
	for _, p := range policies {
		byWorld[p.WorldID] = p
	}
	usage, err := ttsWorldUsage(db, ids, time.Now())
	if err != nil {
		return nil, err
	}
	// Fetch only the most recent job per listed world, retaining typed timestamps.
	var latest []model.TTSJob
	if err := db.Select("world_id, created_at").Where("world_id IN ? AND id = (SELECT j.id FROM tts_jobs j WHERE j.world_id = tts_jobs.world_id ORDER BY j.created_at DESC, j.id DESC LIMIT 1)", ids).Find(&latest).Error; err != nil {
		return nil, err
	}
	lastUsed := map[string]time.Time{}
	for _, job := range latest {
		lastUsed[job.WorldID] = job.CreatedAt
	}
	for i := range result.Items {
		item := &result.Items[i]
		item.Policy = byWorld[item.WorldID]
		item.Policy.WorldID = item.WorldID
		item.Usage = usage[item.WorldID]
		if at, ok := lastUsed[item.WorldID]; ok {
			item.LastUsedAt = &at
		}
	}
	return result, nil
}
