package service

import (
	"log"
	"sync"
	"time"

	"sealchat/utils"
)

var backupWorkerOnce sync.Once

// StartBackupWorker 启动备份 Worker
func StartBackupWorker(cfg *utils.AppConfig) {
	backupWorkerOnce.Do(func() {
		if cfg == nil {
			log.Println("backup: config is nil")
			return
		}
		if !cfg.Backup.Enabled {
			log.Println("backup: disabled")
			return
		}
		log.Println("backup: Worker 启动")
		go runBackupWorker(cfg)
	})
}

func runBackupWorker(cfg *utils.AppConfig) {
	interval := cfg.Backup.IntervalHours
	if interval <= 0 {
		interval = 24
	}
	runBackupOnce(cfg)
	ticker := time.NewTicker(time.Duration(interval) * time.Hour)
	defer ticker.Stop()

	for {
		<-ticker.C
		runBackupOnce(cfg)
	}
}

func runBackupOnce(cfg *utils.AppConfig) {
	if cfg == nil {
		return
	}
	if _, err := ExecuteBackup(cfg); err != nil {
		log.Printf("backup: 执行失败: %v", err)
	}
}
