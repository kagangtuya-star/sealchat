package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/samber/lo"

	"sealchat/api"
	"sealchat/model"
	"sealchat/pm"
	"sealchat/service"
	"sealchat/service/metrics"
	"sealchat/utils"
)

//go:embed ui/dist
var embedDirStatic embed.FS

//go:generate go run ./pm/generator/

func main() {
	var opts struct {
		Install                  bool     `short:"i" long:"install" description:"安装为系统服务"`
		Uninstall                bool     `long:"uninstall" description:"删除系统服务"`
		Download                 bool     `short:"d" long:"download" description:"从github下载最新的压缩包"`
		ConfigList               bool     `long:"config-list" description:"列出配置历史版本"`
		ConfigShow               int64    `long:"config-show" description:"显示指定版本配置详情"`
		ConfigRollback           int64    `long:"config-rollback" description:"回滚到指定配置版本"`
		ConfigExport             int64    `long:"config-export" description:"导出指定版本配置到文件"`
		SQLiteVacuum             bool     `long:"sqlite-vacuum" description:"手动执行 SQLite 数据库空间整理（VACUUM）"`
		SQLiteFTSRebuild         bool     `long:"sqlite-fts-rebuild" description:"手动全量重建 SQLite FTS 索引"`
		CleanupWebhookBotFriends bool     `long:"cleanup-webhook-bot-friends" description:"清理 webhook BOT 历史遗留的好友关系与私聊频道（物理删除）"`
		UserSecret               string   `long:"user-secret" description:"用户秘密工具：list 列出平台管理员，reset 按用户名重置密码为123456" choice:"list" choice:"reset"`
		Username                 []string `long:"username" description:"目标用户名，可重复指定"`
		AdminOnly                bool     `long:"admin-only" description:"仅允许重置平台管理员"`
		Yes                      bool     `long:"yes" description:"执行重置时跳过交互确认"`
		Output                   string   `long:"output" description:"导出配置的输出文件路径"`
	}
	_, err := flags.ParseArgs(&opts, os.Args)
	if err != nil {
		return
	}

	if opts.Install {
		serviceInstall(true)
		return
	}

	if opts.Uninstall {
		serviceInstall(false)
		return
	}

	if opts.Download {
		err = downloadLatestRelease()
		if err != nil {
			fmt.Println(err.Error())
		}
		return
	}

	if opts.UserSecret != "" && (opts.ConfigList || opts.ConfigShow > 0 || opts.ConfigRollback > 0 || opts.ConfigExport > 0 || opts.SQLiteVacuum || opts.SQLiteFTSRebuild || opts.CleanupWebhookBotFriends) {
		log.Fatal("--user-secret 不能与配置版本管理/数据库维护参数同时使用")
	}
	if opts.SQLiteFTSRebuild && (opts.ConfigList || opts.ConfigShow > 0 || opts.ConfigRollback > 0 || opts.ConfigExport > 0 || opts.SQLiteVacuum || opts.CleanupWebhookBotFriends) {
		log.Fatal("--sqlite-fts-rebuild 不能与配置版本管理/数据库维护参数同时使用")
	}

	// 配置管理命令需要先初始化数据库
	if opts.ConfigList || opts.ConfigShow > 0 || opts.ConfigRollback > 0 || opts.ConfigExport > 0 || opts.SQLiteVacuum || opts.CleanupWebhookBotFriends || opts.UserSecret != "" {
		lo.Must0(os.MkdirAll("./data", 0755))
		// 优先从配置文件读取 DSN，否则使用默认值
		dsn := utils.GetDSNForCLI()
		if err := model.DBInitMinimal(dsn); err != nil {
			log.Fatalf("初始化数据库失败: %v", err)
		}

		if opts.ConfigList {
			handleConfigList()
			return
		}
		if opts.ConfigShow > 0 {
			handleConfigShow(opts.ConfigShow)
			return
		}
		if opts.ConfigRollback > 0 {
			handleConfigRollback(opts.ConfigRollback)
			return
		}
		if opts.ConfigExport > 0 {
			handleConfigExport(opts.ConfigExport, opts.Output)
			return
		}
		if opts.SQLiteVacuum {
			if err := handleSQLiteVacuum(); err != nil {
				log.Fatalf("SQLite 空间整理失败: %v", err)
			}
			return
		}
		if opts.CleanupWebhookBotFriends {
			if err := handleCleanupWebhookBotFriends(); err != nil {
				log.Fatalf("Webhook BOT 历史好友数据清理失败: %v", err)
			}
			return
		}
		if opts.UserSecret != "" {
			if err := handleUserSecret(opts.UserSecret, opts.Username, opts.AdminOnly, opts.Yes); err != nil {
				log.Fatalf("用户秘密命令执行失败: %v", err)
			}
			return
		}
	}

	if opts.SQLiteFTSRebuild {
		lo.Must0(os.MkdirAll("./data", 0755))
		configInit := initConfigWithDB()
		config := configInit.Config
		model.DBInit(config)
		if configInit.ShouldSync {
			syncConfigToDB(config, configInit.SyncSource)
		}
		if err := model.ForceRebuildSQLiteFTS(); err != nil {
			log.Fatalf("SQLite FTS 全量重建失败: %v", err)
		}
		log.Println("SQLite FTS 全量重建完成")
		return
	}

	lo.Must0(os.MkdirAll("./data", 0755))
	configInit := initConfigWithDB()
	config := configInit.Config
	utils.EnsureDataDirs(config)

	if err := utils.VerifyBundledWebPToolsWithLog(log.Printf); err != nil {
		log.Fatalf("启动自检失败：WebP 编码工具不可用（请检查 bin/ 目录是否完整、与当前平台匹配且可执行）：%v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model.DBInit(config)
	if configInit.ShouldSync {
		syncConfigToDB(config, configInit.SyncSource)
	}
	cleanUp := func() {
		if db := model.GetDB(); db != nil {
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
	}
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		_ = <-c
		cancel()
		cleanUp()
		os.Exit(0)
	}()

	collector := metrics.Init(metrics.Config{
		Interval:  2 * time.Minute,
		Retention: 7 * 24 * time.Hour,
		OnlineTTL: 2 * time.Minute,
	})
	if collector != nil {
		collector.Start(ctx)
	}

	pm.Init()

	service.SyncUpdateCurrentVersion(utils.BuildVersion)

	storageManager, err := service.InitStorageManager(config.Storage)
	if err != nil {
		log.Fatalf("初始化存储系统失败: %v", err)
	}

	if err := service.InitAudioService(config.Audio, storageManager); err != nil {
		log.Fatalf("初始化音频子系统失败: %v", err)
	}

	// 输出 FFmpeg 检测结果
	if svc := service.GetAudioService(); svc != nil {
		info := svc.PlatformInfo()
		if svc.FFmpegAvailable() {
			log.Printf("[音频] FFmpeg 已检测到: %s", info["ffmpeg"])
			if info["ffprobe"] != "" {
				log.Printf("[音频] FFprobe 已检测到: %s", info["ffprobe"])
			}
		} else {
			log.Printf("[音频] 警告: FFmpeg 未检测到，音频工作台的转码功能将不可用")
			log.Printf("[音频] 如需启用音频转码，请下载 FFmpeg: https://github.com/BtbN/FFmpeg-Builds/releases")
		}
	}

	service.InitExportLimiter(service.ExportLimiterConfig{
		BandwidthKBps: config.Export.DownloadBandwidthKBps,
		BurstKB:       config.Export.DownloadBurstKB,
	})
	service.StartMessageExportWorker(service.MessageExportWorkerConfig{
		StorageDir:          config.Export.StorageDir,
		HTMLPageSizeDefault: config.Export.HTMLPageSizeDefault,
		HTMLPageSizeMax:     config.Export.HTMLPageSizeMax,
		HTMLMaxConcurrency:  config.Export.HTMLMaxConcurrency,
	})

	// 未读提醒取代旧未读邮件提醒主链路；旧代码保留但不再默认启动。
	service.StartDigestPushWorker()

	// 启动更新检测 Worker
	if config.UpdateCheck.Enabled {
		service.StartUpdateCheckWorker(service.UpdateCheckWorkerConfig{
			IntervalSec:    config.UpdateCheck.IntervalSec,
			GithubRepo:     config.UpdateCheck.GithubRepo,
			GithubToken:    config.UpdateCheck.GithubToken,
			CurrentVersion: utils.BuildVersion,
		})
	}

	// 启动 SQLite 备份 Worker
	if config.Backup.Enabled {
		service.StartBackupWorker(config)
	}

	autoSave := func() {
		walTicker := time.NewTicker(3 * 60 * time.Second)
		vacuumTicker := time.NewTicker(15 * time.Minute)
		defer walTicker.Stop()
		defer vacuumTicker.Stop()

		idleSince := time.Now()
		lastVacuumAt := time.Time{}
		const minIdleDuration = 30 * time.Minute
		const writeActivityWindow = 10 * time.Minute

		for {
			select {
			case <-ctx.Done():
				return
			case <-walTicker.C:
				model.FlushWAL()
			case <-vacuumTicker.C:
				if !model.IsSQLite() {
					continue
				}
				cfg := utils.GetConfig()
				if cfg == nil || !cfg.SQLite.AutoVacuumEnabled {
					continue
				}
				if collector != nil && collector.CurrentConnectionCount() > 0 {
					idleSince = time.Now()
					continue
				}
				if model.HasRecentSQLiteWriteActivity(writeActivityWindow) {
					log.Printf("SQLite 空闲维护: 检测到近期写入活动，跳过 VACUUM")
					continue
				}
				intervalHours := cfg.SQLite.AutoVacuumIntervalHours
				if intervalHours <= 0 {
					intervalHours = 168
				}
				minVacuumInterval := time.Duration(intervalHours) * time.Hour
				now := time.Now()
				if now.Sub(idleSince) < minIdleDuration {
					continue
				}
				if !lastVacuumAt.IsZero() && now.Sub(lastVacuumAt) < minVacuumInterval {
					continue
				}
				if err := model.VacuumSQLite(); err != nil {
					log.Printf("SQLite 空闲 VACUUM 失败: %v", err)
					continue
				}
				lastVacuumAt = now
				log.Printf("SQLite 空闲维护: VACUUM 执行完成")
			}
		}
	}
	go autoSave()

	api.Init(config, embedDirStatic)
}
