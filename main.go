package main

import (
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
	"sealchat/utils"
)

//go:embed ui/dist
var embedDirStatic embed.FS

//go:generate go run ./pm/generator/

func main() {
	var opts struct {
		Install   bool `short:"i" long:"install" description:"安装为系统服务"`
		Uninstall bool `long:"uninstall" description:"删除系统服务"`
		Download  bool `short:"d" long:"download" description:"从github下载最新的压缩包"`
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

	lo.Must0(os.MkdirAll("./data", 0755))
	config := utils.ReadConfig()

	model.DBInit(config.DSN)
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
		cleanUp()
		os.Exit(0)
	}()

	pm.Init()

	if err := service.InitWorldContext(); err != nil {
		log.Fatalf("初始化世界上下文失败: %v", err)
	}

	storageManager, err := service.InitStorageManager(config.Storage)
	if err != nil {
		log.Fatalf("初始化存储系统失败: %v", err)
	}

	if err := service.InitAudioService(config.Audio, storageManager); err != nil {
		log.Fatalf("初始化音频子系统失败: %v", err)
	}

	service.StartMessageExportWorker(service.MessageExportWorkerConfig{
		StorageDir: "./data/exports",
	})

	autoSave := func() {
		t := time.NewTicker(3 * 60 * time.Second)
		for {
			<-t.C
			model.FlushWAL()
		}
	}
	go autoSave()

	api.Init(config, embedDirStatic)
}
