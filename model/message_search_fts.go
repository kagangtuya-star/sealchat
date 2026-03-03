package model

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ftsVersionCurrent      = 1
	ftsRebuildTimeout      = 3 * time.Second
	ftsLeaseDuration       = 5 * time.Minute
	ftsIncrementalLookback = 5 * time.Minute
)

var (
	ftsInitialized      atomic.Bool
	ftsRebuilding       atomic.Bool
	lastFTSError        atomic.Value
	lastSQLiteFTSAction atomic.Value
)

func init() {
	lastFTSError.Store("")
	lastSQLiteFTSAction.Store("none")
}

func resetSQLiteFTSState() {
	sqliteFTSReady = false
	ftsInitialized.Store(false)
	ftsRebuilding.Store(false)
	lastFTSError.Store("")
	lastSQLiteFTSAction.Store("reset")
}

func setLastFTSError(err error) {
	if err == nil {
		lastFTSError.Store("")
		return
	}
	lastFTSError.Store(strings.TrimSpace(err.Error()))
}

func setSQLiteFTSAction(action string) {
	action = strings.TrimSpace(action)
	if action == "" {
		action = "unknown"
	}
	lastSQLiteFTSAction.Store(action)
}

func disableSQLiteFTS(err error) {
	setLastFTSError(err)
	sqliteFTSReady = false
}

func ReportSQLiteFTSFailure(err error) {
	if err == nil {
		return
	}
	disableSQLiteFTS(err)
}

type sqliteFTSSchemaState struct {
	TableExists  bool
	TriggerCount int64
}

func inspectSQLiteFTSSchema(conn *gorm.DB) (sqliteFTSSchemaState, error) {
	state := sqliteFTSSchemaState{}
	if conn == nil {
		return state, errors.New("nil connection for FTS check")
	}
	var tableCount int64
	if err := conn.Raw(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'messages_fts'`,
	).Scan(&tableCount).Error; err != nil {
		return state, err
	}
	state.TableExists = tableCount > 0
	if !state.TableExists {
		return state, nil
	}
	if err := conn.Raw(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger' AND name IN ('messages_ai','messages_ad','messages_au')`,
	).Scan(&state.TriggerCount).Error; err != nil {
		return state, err
	}
	return state, nil
}

func sqliteFTSSchemaValid(conn *gorm.DB) (bool, error) {
	state, err := inspectSQLiteFTSSchema(conn)
	if err != nil {
		return false, err
	}
	return state.TableExists && state.TriggerCount == 3, nil
}

type ftsVersionRecord struct {
	Key             string     `gorm:"primaryKey;size:64"`
	Version         int        `gorm:"not null"`
	UpdatedAt       time.Time  `gorm:"not null"`
	Status          string     `gorm:"size:32"`
	Message         string     `gorm:"size:255"`
	LastIndexedAt   int64      `gorm:"not null;default:0"`
	LastRebuildMode string     `gorm:"size:32"`
	LeaseToken      string     `gorm:"size:64"`
	LeaseExpireAt   *time.Time `gorm:"index"`
}

type SQLiteFTSStatusSnapshot struct {
	Ready           bool   `json:"ready"`
	LastError       string `json:"last_error"`
	Status          string `json:"status"`
	Version         int    `json:"version"`
	LastIndexedAt   int64  `json:"last_indexed_at"`
	LastRebuildMode string `json:"last_rebuild_mode"`
	LeaseExpireAt   int64  `json:"lease_expire_at"`
	Message         string `json:"message"`
	LastAction      string `json:"last_action"`
}

func SQLiteFTSStatus() SQLiteFTSStatusSnapshot {
	snapshot := SQLiteFTSStatusSnapshot{
		Ready:     sqliteFTSReady,
		LastError: LastFTSError(),
		Status:    "unknown",
	}
	if action, ok := lastSQLiteFTSAction.Load().(string); ok {
		snapshot.LastAction = action
	}
	conn := GetDB()
	if conn == nil {
		return snapshot
	}
	rec, err := getFTSVersion(conn)
	if err != nil {
		return snapshot
	}
	snapshot.Status = rec.Status
	snapshot.Version = rec.Version
	snapshot.LastIndexedAt = rec.LastIndexedAt
	snapshot.LastRebuildMode = rec.LastRebuildMode
	snapshot.Message = rec.Message
	if rec.LeaseExpireAt != nil {
		snapshot.LeaseExpireAt = rec.LeaseExpireAt.UnixMilli()
	}
	return snapshot
}

func ensureFTSRecord(conn *gorm.DB) error {
	return conn.Clauses(clause.OnConflict{DoNothing: true}).Create(&ftsVersionRecord{
		Key:             "messages_fts",
		Version:         0,
		Status:          "unknown",
		LastIndexedAt:   0,
		LastRebuildMode: "",
		Message:         "",
		UpdatedAt:       time.Now(),
	}).Error
}

func ensureSQLiteFTSManager(conn *gorm.DB) error {
	if !ftsInitialized.CompareAndSwap(false, true) {
		return nil
	}
	if conn == nil {
		return errors.New("nil db connection")
	}
	if err := conn.AutoMigrate(&ftsVersionRecord{}); err != nil {
		return err
	}
	if err := ensureFTSRecord(conn); err != nil {
		return err
	}
	rec, err := getFTSVersion(conn)
	if err != nil {
		return err
	}
	state, err := inspectSQLiteFTSSchema(conn)
	if err != nil {
		return err
	}
	now := time.Now()
	if rec.Status == "building" && rec.LeaseExpireAt != nil && rec.LeaseExpireAt.After(now) {
		setSQLiteFTSAction("defer_lease_active")
		setLastFTSError(nil)
		sqliteFTSReady = false
		log.Printf("检测到已有 SQLite FTS 构建租约，跳过本次初始化（租约到期: %s）", rec.LeaseExpireAt.Format(time.RFC3339))
		return nil
	}

	if rec.Version >= ftsVersionCurrent && rec.Status == "ready" && state.TableExists && state.TriggerCount == 3 {
		setSQLiteFTSAction("skip")
		setLastFTSError(nil)
		sqliteFTSReady = true
		if rec.LastIndexedAt <= 0 && !rec.UpdatedAt.IsZero() {
			_ = bootstrapSQLiteFTSWatermark(conn, rec.UpdatedAt.UnixMilli())
		}
		return nil
	}

	fullRebuild := rec.Version < ftsVersionCurrent || !state.TableExists
	reason := "schema_repair_or_status_recover"
	if rec.Version < ftsVersionCurrent {
		reason = "version_outdated"
	} else if !state.TableExists {
		reason = "fts_table_missing"
	}

	go rebuildFTSInBackground(conn, *rec, fullRebuild, reason)
	return nil
}

func getFTSVersion(conn *gorm.DB) (*ftsVersionRecord, error) {
	rec := &ftsVersionRecord{}
	err := conn.Where("key = ?", "messages_fts").
		First(rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		rec.Key = "messages_fts"
		rec.Version = 0
		rec.Status = "unknown"
		rec.LastIndexedAt = 0
		return rec, nil
	}
	return rec, err
}

func acquireFTSLease(conn *gorm.DB, token string) (bool, time.Time, error) {
	now := time.Now()
	expires := now.Add(ftsLeaseDuration)
	tx := conn.Model(&ftsVersionRecord{}).
		Where("key = ?", "messages_fts").
		Where("lease_expire_at IS NULL OR lease_expire_at < ? OR lease_token = ?", now, token).
		Updates(map[string]any{
			"lease_token":     token,
			"lease_expire_at": expires,
			"updated_at":      now,
		})
	if tx.Error != nil {
		return false, time.Time{}, tx.Error
	}
	return tx.RowsAffected > 0, expires, nil
}

func releaseFTSLease(conn *gorm.DB, token string) {
	if strings.TrimSpace(token) == "" {
		return
	}
	if err := conn.Model(&ftsVersionRecord{}).
		Where("key = ? AND lease_token = ?", "messages_fts", token).
		Updates(map[string]any{
			"lease_token":     "",
			"lease_expire_at": nil,
			"updated_at":      time.Now(),
		}).Error; err != nil {
		log.Printf("释放 SQLite FTS 构建租约失败: %v", err)
	}
}

func rebuildFTSInBackground(conn *gorm.DB, rec ftsVersionRecord, fullRebuild bool, reason string) {
	if !ftsRebuilding.CompareAndSwap(false, true) {
		return
	}
	defer ftsRebuilding.Store(false)

	leaseToken := fmt.Sprintf("sqlite-fts-%d", time.Now().UnixNano())
	acquired, leaseExpireAt, err := acquireFTSLease(conn, leaseToken)
	if err != nil {
		setSQLiteFTSAction("lease_acquire_failed")
		setLastFTSError(err)
		sqliteFTSReady = false
		log.Printf("获取 SQLite FTS 构建租约失败: %v", err)
		return
	}
	if !acquired {
		setSQLiteFTSAction("defer_lease_busy")
		setLastFTSError(nil)
		sqliteFTSReady = false
		log.Printf("SQLite FTS 构建租约被占用，跳过本次任务")
		return
	}
	defer releaseFTSLease(conn, leaseToken)

	start := time.Now()
	prevWatermark := rec.LastIndexedAt
	if prevWatermark <= 0 && rec.Status == "ready" && !rec.UpdatedAt.IsZero() {
		prevWatermark = rec.UpdatedAt.UnixMilli()
	}

	mode := "incremental"
	status := "repairing"
	if fullRebuild {
		mode = "full"
		status = "building"
		setSQLiteFTSAction("full_rebuild_start")
	} else {
		setSQLiteFTSAction("incremental_repair_start")
	}

	if err := markFTSStatus(
		conn,
		status,
		fmt.Sprintf("%s: %s", mode, strings.TrimSpace(reason)),
		0,
		mode,
		prevWatermark,
		leaseToken,
		&leaseExpireAt,
	); err != nil {
		log.Printf("记录 SQLite FTS 状态失败: %v", err)
	}

	if fullRebuild {
		if err := rebuildFTS(conn); err != nil {
			setSQLiteFTSAction("full_rebuild_failed")
			setLastFTSError(err)
			sqliteFTSReady = false
			_ = markFTSStatus(conn, "error", err.Error(), 0, mode, prevWatermark, "", nil)
			log.Printf("重建 SQLite FTS 失败: %v", err)
			return
		}
	} else {
		if err := repairFTSIncrementally(conn, prevWatermark); err != nil {
			setSQLiteFTSAction("incremental_repair_failed")
			setLastFTSError(err)
			sqliteFTSReady = false
			_ = markFTSStatus(conn, "error", err.Error(), 0, mode, prevWatermark, "", nil)
			log.Printf("增量修复 SQLite FTS 失败: %v", err)
			return
		}
	}

	nextWatermark, err := queryMessagesMaxUpdatedAtMilli(conn)
	if err != nil {
		log.Printf("读取消息更新时间水位失败，使用旧水位: %v", err)
		nextWatermark = prevWatermark
	}
	if nextWatermark < prevWatermark {
		nextWatermark = prevWatermark
	}

	duration := time.Since(start)
	setLastFTSError(nil)
	sqliteFTSReady = true
	if fullRebuild {
		setSQLiteFTSAction("full_rebuild_done")
	} else {
		setSQLiteFTSAction("incremental_repair_done")
	}
	_ = markFTSStatus(
		conn,
		"ready",
		fmt.Sprintf("%s in %s", mode, duration),
		ftsVersionCurrent,
		mode,
		nextWatermark,
		"",
		nil,
	)
	log.Printf("SQLite FTS %s 完成，用时 %s", mode, duration)
}

func bootstrapSQLiteFTSWatermark(conn *gorm.DB, watermark int64) error {
	if watermark <= 0 {
		return nil
	}
	return conn.Model(&ftsVersionRecord{}).
		Where("key = ? AND last_indexed_at = 0", "messages_fts").
		Updates(map[string]any{
			"last_indexed_at": watermark,
			"updated_at":      time.Now(),
		}).Error
}

func repairFTSIncrementally(conn *gorm.DB, watermark int64) error {
	if !conn.Migrator().HasTable("messages_fts") {
		return errors.New("messages_fts table missing")
	}
	if err := recreateFTSTriggers(conn); err != nil {
		return err
	}

	startMilli := int64(0)
	if watermark > 0 {
		startMilli = watermark - int64(ftsIncrementalLookback/time.Millisecond)
		if startMilli < 0 {
			startMilli = 0
		}
	}
	startTime := time.UnixMilli(startMilli)
	if err := conn.Exec(`
		INSERT OR REPLACE INTO messages_fts(message_id, content)
		SELECT id, COALESCE(content, '')
		FROM messages
		WHERE updated_at >= ?;
	`, startTime).Error; err != nil {
		return err
	}
	return nil
}

func rebuildFTS(conn *gorm.DB) error {
	statements := []string{
		`DROP TRIGGER IF EXISTS messages_ai;`,
		`DROP TRIGGER IF EXISTS messages_ad;`,
		`DROP TRIGGER IF EXISTS messages_au;`,
		`DROP TABLE IF EXISTS messages_fts;`,
		`CREATE VIRTUAL TABLE messages_fts USING fts5(
			message_id UNINDEXED,
			content,
			tokenize = 'unicode61 remove_diacritics 0'
		);`,
	}
	for _, stmt := range statements {
		if err := conn.Exec(stmt).Error; err != nil {
			return err
		}
	}
	if err := recreateFTSTriggers(conn); err != nil {
		return err
	}
	if err := conn.Exec(`
		INSERT INTO messages_fts(message_id, content)
		SELECT id, COALESCE(content, '')
		FROM messages;
	`).Error; err != nil {
		return err
	}
	return nil
}

func recreateFTSTriggers(conn *gorm.DB) error {
	statements := []string{
		`DROP TRIGGER IF EXISTS messages_ai;`,
		`DROP TRIGGER IF EXISTS messages_ad;`,
		`DROP TRIGGER IF EXISTS messages_au;`,
		`CREATE TRIGGER messages_ai AFTER INSERT ON messages BEGIN
			INSERT INTO messages_fts(message_id, content) VALUES (new.id, COALESCE(new.content, ''));
		END;`,
		`CREATE TRIGGER messages_ad AFTER DELETE ON messages BEGIN
			DELETE FROM messages_fts WHERE message_id = old.id;
		END;`,
		`CREATE TRIGGER messages_au AFTER UPDATE ON messages BEGIN
			INSERT OR REPLACE INTO messages_fts(message_id, content) VALUES (new.id, COALESCE(new.content, ''));
		END;`,
	}
	for _, stmt := range statements {
		if err := conn.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

func queryMessagesMaxUpdatedAtMilli(conn *gorm.DB) (int64, error) {
	if conn == nil {
		return 0, errors.New("nil connection")
	}
	var latest struct {
		UpdatedAt time.Time
	}
	err := conn.Model(&MessageModel{}).
		Select("updated_at").
		Order("updated_at DESC").
		Limit(1).
		Take(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if latest.UpdatedAt.IsZero() {
		return 0, nil
	}
	return latest.UpdatedAt.UnixMilli(), nil
}

func markFTSStatus(
	conn *gorm.DB,
	status string,
	message string,
	version int,
	mode string,
	lastIndexedAt int64,
	leaseToken string,
	leaseExpireAt *time.Time,
) error {
	record := ftsVersionRecord{
		Key:             "messages_fts",
		Version:         version,
		Status:          strings.TrimSpace(status),
		Message:         strings.TrimSpace(message),
		LastIndexedAt:   lastIndexedAt,
		LastRebuildMode: strings.TrimSpace(mode),
		LeaseToken:      strings.TrimSpace(leaseToken),
		LeaseExpireAt:   leaseExpireAt,
		UpdatedAt:       time.Now(),
	}
	return conn.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Create(&record).Error
}

func ForceRebuildSQLiteFTS() error {
	if !IsSQLite() {
		return errors.New("当前数据库不是 SQLite")
	}
	conn := GetDB()
	if conn == nil {
		return errors.New("数据库尚未初始化")
	}
	if err := conn.AutoMigrate(&ftsVersionRecord{}); err != nil {
		return err
	}
	if err := ensureFTSRecord(conn); err != nil {
		return err
	}
	if !ftsRebuilding.CompareAndSwap(false, true) {
		return errors.New("SQLite FTS 正在构建中")
	}
	defer ftsRebuilding.Store(false)

	leaseToken := fmt.Sprintf("sqlite-fts-manual-%d", time.Now().UnixNano())
	acquired, leaseExpireAt, err := acquireFTSLease(conn, leaseToken)
	if err != nil {
		return err
	}
	if !acquired {
		return errors.New("SQLite FTS 构建租约被占用，请稍后再试")
	}
	defer releaseFTSLease(conn, leaseToken)

	start := time.Now()
	setSQLiteFTSAction("manual_full_rebuild_start")
	setLastFTSError(nil)
	sqliteFTSReady = false
	_ = markFTSStatus(conn, "building", "manual full rebuild", 0, "manual_full", 0, leaseToken, &leaseExpireAt)

	if err := rebuildFTS(conn); err != nil {
		setSQLiteFTSAction("manual_full_rebuild_failed")
		setLastFTSError(err)
		_ = markFTSStatus(conn, "error", err.Error(), 0, "manual_full", 0, "", nil)
		return err
	}
	watermark, err := queryMessagesMaxUpdatedAtMilli(conn)
	if err != nil {
		watermark = 0
	}
	_ = markFTSStatus(
		conn,
		"ready",
		fmt.Sprintf("manual full rebuild in %s", time.Since(start)),
		ftsVersionCurrent,
		"manual_full",
		watermark,
		"",
		nil,
	)
	sqliteFTSReady = true
	setSQLiteFTSAction("manual_full_rebuild_done")
	setLastFTSError(nil)
	return nil
}

func LastFTSError() string {
	val := lastFTSError.Load()
	if val == nil {
		return ""
	}
	if msg, ok := val.(string); ok {
		return msg
	}
	return fmt.Sprintf("%v", val)
}
