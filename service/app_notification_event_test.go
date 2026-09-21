package service

import (
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/utils"
)

func initAppNotificationEventTestDB(t *testing.T) {
	t.Helper()
	model.DBInit(&utils.AppConfig{
		DSN: fmt.Sprintf("file:service-app-notification-%s?mode=memory&cache=shared", utils.NewID()),
		SQLite: utils.SQLiteConfig{
			EnableWAL:       false,
			TxLockImmediate: false,
			ReadConnections: 1,
			OptimizeOnInit:  false,
		},
	})

	deadline := time.Now().Add(5 * time.Second)
	for {
		status := model.SQLiteFTSStatus()
		if status.Status == "ready" || status.Status == "error" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("wait for SQLite FTS initialization: status=%s action=%s error=%s", status.Status, status.LastAction, status.LastError)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestEnqueueAppNotificationForMessageReturnsAfterConsumerQueries(t *testing.T) {
	initAppNotificationEventTestDB(t)

	queriedTables := map[string]int{}
	callbackName := "test:app-notification-fast-path"
	db := model.GetDB()
	if err := db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		queriedTables[tx.Statement.Table]++
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove(callbackName)
	})

	if err := EnqueueAppNotificationForMessageWithMeow("missing-message", "", "", ""); err != nil {
		t.Fatalf("enqueue without notification consumers: %v", err)
	}

	for _, table := range []string{"app_notification_devices", "app_notification_preferences"} {
		if queriedTables[table] != 1 {
			t.Errorf("query count for %s = %d, want 1", table, queriedTables[table])
		}
	}
	for _, table := range []string{"messages", "users", "channels", "worlds", "message_whisper_recipients", "app_notification_instance"} {
		if queriedTables[table] != 0 {
			t.Errorf("fast path unexpectedly queried %s %d times", table, queriedTables[table])
		}
	}
}
