package model

import (
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	pgFTSVersionCurrent = 1
	pgFTSDefaultConfig  = "simple"
)

var (
	pgFTSInitialized atomic.Bool
	pgFTSRebuilding  atomic.Bool
	pgFTSReady       atomic.Bool
	pgLastError      atomic.Value
)

func init() {
	pgLastError.Store("")
}

func resetPostgresFTSState() {
	pgFTSInitialized.Store(false)
	pgFTSReady.Store(false)
	pgFTSRebuilding.Store(false)
	pgLastError.Store("")
}

func PostgresFTSReady() bool {
	return pgFTSReady.Load()
}

func LastPostgresFTSError() string {
	if val, ok := pgLastError.Load().(string); ok {
		return val
	}
	return ""
}

func ReportPostgresFTSFailure(err error) {
	if err == nil {
		return
	}
	pgLastError.Store(strings.TrimSpace(err.Error()))
	pgFTSReady.Store(false)
}

func PostgresTextSearchConfig() string {
	return pgFTSDefaultConfig
}

func ensurePostgresFTSManager(conn *gorm.DB) error {
	if !pgFTSInitialized.CompareAndSwap(false, true) {
		return nil
	}
	if conn == nil {
		return fmt.Errorf("nil db connection")
	}
	if err := conn.AutoMigrate(&ftsVersionRecord{}); err != nil {
		return err
	}
	rec, err := getPGFTSVersion(conn)
	if err != nil {
		return err
	}
	if rec.Version >= pgFTSVersionCurrent && rec.Status == "ready" {
		if ok, schemaErr := postgresFTSSchemaValid(conn); schemaErr == nil && ok {
			pgFTSReady.Store(true)
			return nil
		} else if schemaErr != nil {
			log.Printf("检测到 Postgres FTS schema 校验错误，准备重建: %v", schemaErr)
		} else {
			log.Printf("检测到 Postgres FTS schema 缺失，准备重建")
		}
	}
	go rebuildPostgresFTSInBackground(conn)
	return nil
}

func postgresFTSSchemaValid(conn *gorm.DB) (bool, error) {
	type existsRow struct{ Exists bool }
	var col existsRow
	if err := conn.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name='messages' AND column_name='content_tsv'
		)`).Scan(&col).Error; err != nil {
		return false, err
	}
	if !col.Exists {
		return false, nil
	}
	var idx existsRow
	if err := conn.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes WHERE tablename='messages' AND indexname='messages_content_tsv_idx'
		)`).Scan(&idx).Error; err != nil {
		return false, err
	}
	if !idx.Exists {
		return false, nil
	}
	var trg existsRow
	if err := conn.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM pg_trigger WHERE tgname='messages_content_tsv_update'
		)`).Scan(&trg).Error; err != nil {
		return false, err
	}
	return trg.Exists, nil
}

func getPGFTSVersion(conn *gorm.DB) (*ftsVersionRecord, error) {
	rec := &ftsVersionRecord{}
	err := conn.Where("key = ?", "messages_pg_fts").
		First(rec).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			rec.Key = "messages_pg_fts"
			rec.Version = 0
			rec.Status = "unknown"
			return rec, nil
		}
		return rec, err
	}
	return rec, nil
}

func rebuildPostgresFTSInBackground(conn *gorm.DB) {
	if !pgFTSRebuilding.CompareAndSwap(false, true) {
		return
	}
	defer pgFTSRebuilding.Store(false)

	start := time.Now()
	if err := markPGFTSStatus(conn, "building", ""); err != nil {
		log.Printf("记录 Postgres FTS 状态失败: %v", err)
	}
	if err := rebuildPostgresFTS(conn); err != nil {
		pgLastError.Store(err.Error())
		_ = markPGFTSStatus(conn, "error", err.Error())
		log.Printf("重建 Postgres FTS 失败: %v", err)
		return
	}
	pgLastError.Store("")
	pgFTSReady.Store(true)
	_ = markPGFTSStatus(conn, "ready", fmt.Sprintf("rebuilt in %s", time.Since(start)))
	log.Printf("Postgres FTS 重建完成，用时 %s", time.Since(start))
}

func rebuildPostgresFTS(conn *gorm.DB) error {
	config := pgFTSDefaultConfig
	statements := []string{
		`ALTER TABLE messages ADD COLUMN IF NOT EXISTS content_tsv tsvector;`,
		fmt.Sprintf(`UPDATE messages SET content_tsv = to_tsvector('%s', COALESCE(content, '')) WHERE content_tsv IS NULL;`, config),
		`CREATE INDEX IF NOT EXISTS messages_content_tsv_idx ON messages USING GIN(content_tsv);`,
		fmt.Sprintf(`CREATE OR REPLACE FUNCTION messages_content_tsv_trigger() RETURNS trigger AS $$
		BEGIN
			NEW.content_tsv := to_tsvector('%s', COALESCE(NEW.content, ''));
			RETURN NEW;
		END$$ LANGUAGE plpgsql;`, config),
		`DROP TRIGGER IF EXISTS messages_content_tsv_update ON messages;`,
		`CREATE TRIGGER messages_content_tsv_update BEFORE INSERT OR UPDATE ON messages
			FOR EACH ROW EXECUTE FUNCTION messages_content_tsv_trigger();`,
	}
	for _, stmt := range statements {
		if err := conn.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

func markPGFTSStatus(conn *gorm.DB, status, message string) error {
	record := ftsVersionRecord{
		Key: "messages_pg_fts",
		Version: func() int {
			if status == "ready" {
				return pgFTSVersionCurrent
			}
			return 0
		}(),
		Status:    status,
		Message:   strings.TrimSpace(message),
		UpdatedAt: time.Now(),
	}
	return conn.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Create(&record).Error
}
