package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"sealchat/model"
	"sealchat/utils"

	"github.com/knadh/koanf/parsers/yaml"
)

// sensitiveFields 敏感字段列表（小写）
var sensitiveFields = []string{
	"password", "secret", "secretkey", "accesskey", "token",
	"dsn", "dburl", "sessiontoken",
}

// maskSensitiveFields 递归遮罩敏感字段
func maskSensitiveFields(data map[string]interface{}) {
	for key, value := range data {
		keyLower := strings.ToLower(key)

		// 检查是否为敏感字段
		isSensitive := false
		for _, sf := range sensitiveFields {
			if strings.Contains(keyLower, sf) {
				isSensitive = true
				break
			}
		}

		if isSensitive {
			if str, ok := value.(string); ok && str != "" {
				data[key] = "******"
			}
		} else if nested, ok := value.(map[string]interface{}); ok {
			maskSensitiveFields(nested)
		}
	}
}

// initConfigWithDB 初始化配置并同步到数据库
// 实现双路径启动逻辑：
// - 配置文件存在 → 读取 → 同步到 DB
// - 配置文件不存在 → 尝试从 DB 恢复
func initConfigWithDB() *utils.AppConfig {
	configExists := utils.ConfigFileExists()

	if configExists {
		// 路径A：配置文件存在
		config := utils.ReadConfig()
		model.DBInit(config)
		syncConfigToDB(config, "file")
		return config
	}

	// 路径B：配置文件不存在
	log.Println("[配置] 配置文件不存在，尝试从数据库恢复...")

	dsn := utils.GetMinimalDSN()
	if err := model.DBInitMinimal(dsn); err != nil {
		log.Printf("[配置] 初始化数据库失败: %v，使用默认配置", err)
		config := utils.ReadConfig()
		model.DBInit(config)
		syncConfigToDB(config, "init")
		return config
	}

	// 尝试从数据库读取配置
	dbConfig, err := model.GetCurrentConfig()
	if err != nil {
		log.Printf("[配置] 读取数据库配置失败: %v，使用默认配置", err)
		config := utils.ReadConfig()
		model.DBInit(config)
		syncConfigToDB(config, "init")
		return config
	}

	if dbConfig != nil && dbConfig.ConfigJSON != "" {
		// 从数据库恢复配置
		var config utils.AppConfig
		if err := json.Unmarshal([]byte(dbConfig.ConfigJSON), &config); err != nil {
			log.Printf("[配置] 解析数据库配置失败: %v，使用默认配置", err)
			config := utils.ReadConfig()
			model.DBInit(config)
			syncConfigToDB(config, "init")
			return config
		}

		// 确保 DSN 不丢失（JSON 中 DSN 标记为 "-" 不会序列化）
		if config.DSN == "" {
			config.DSN = dsn
		}

		log.Println("[配置] 从数据库恢复配置成功")
		// 写入配置文件
		utils.WriteConfig(&config)
		// 重新完整初始化数据库
		model.DBInit(&config)
		return &config
	}

	// 数据库中也没有配置，全新安装
	log.Println("[配置] 数据库中无配置记录，创建默认配置")
	config := utils.ReadConfig()
	model.DBInit(config)
	syncConfigToDB(config, "init")
	return config
}

// syncConfigToDB 将配置同步到数据库
func syncConfigToDB(config *utils.AppConfig, source string) {
	if config == nil {
		return
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		log.Printf("[配置] 序列化配置失败: %v", err)
		return
	}

	note := ""
	switch source {
	case "file":
		note = "从配置文件同步"
	case "init":
		note = "初始安装"
	case "api":
		note = "通过 API 修改"
	}

	if err := model.SaveConfigVersion(string(configJSON), source, note); err != nil {
		log.Printf("[配置] 保存配置版本失败: %v", err)
	}
}

// handleConfigList 列出配置历史版本
func handleConfigList() {
	list, err := model.GetConfigHistoryList()
	if err != nil {
		log.Fatalf("获取配置历史失败: %v", err)
	}

	if len(list) == 0 {
		fmt.Println("暂无配置历史记录")
		return
	}

	fmt.Println("配置版本历史：")
	fmt.Println("────────────────────────────────────────────────────────────")
	fmt.Printf("%-8s %-20s %-12s %s\n", "版本", "时间", "来源", "说明")
	fmt.Println("────────────────────────────────────────────────────────────")

	for _, item := range list {
		fmt.Printf("%-8d %-20s %-12s %s\n",
			item.Version,
			item.CreatedAt.Format("2006-01-02 15:04:05"),
			item.Source,
			item.Note,
		)
	}

	// 显示当前版本
	current, _ := model.GetCurrentConfig()
	if current != nil {
		fmt.Println("────────────────────────────────────────────────────────────")
		fmt.Printf("当前版本 ID: %s\n", current.CurrentVersionID)
	}
}

// handleConfigShow 显示指定版本配置详情
func handleConfigShow(version int64) {
	history, err := model.GetConfigHistoryByVersion(version)
	if err != nil {
		log.Fatalf("获取配置版本失败: %v", err)
	}
	if history == nil {
		fmt.Printf("版本 %d 不存在\n", version)
		return
	}

	fmt.Printf("版本: %d\n", history.Version)
	fmt.Printf("时间: %s\n", history.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("来源: %s\n", history.Source)
	fmt.Printf("说明: %s\n", history.Note)
	fmt.Printf("哈希: %s\n", history.ConfigHash)
	fmt.Println("────────────────────────────────────────────────────────────")
	fmt.Println("注意: 敏感字段（密码、密钥等）已遮罩显示")
	fmt.Println("────────────────────────────────────────────────────────────")

	// 美化输出 JSON，遮罩敏感字段
	var prettyJSON map[string]interface{}
	if err := json.Unmarshal([]byte(history.ConfigJSON), &prettyJSON); err == nil {
		maskSensitiveFields(prettyJSON)
		output, _ := json.MarshalIndent(prettyJSON, "", "  ")
		fmt.Println(string(output))
	} else {
		fmt.Println(history.ConfigJSON)
	}
}

// handleConfigRollback 回滚到指定版本
func handleConfigRollback(version int64) {
	history, err := model.GetConfigHistoryByVersion(version)
	if err != nil {
		log.Fatalf("获取配置版本失败: %v", err)
	}
	if history == nil {
		fmt.Printf("版本 %d 不存在\n", version)
		return
	}

	fmt.Printf("即将回滚到版本 %d（%s）\n", history.Version, history.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("来源: %s\n", history.Source)
	fmt.Printf("说明: %s\n", history.Note)
	fmt.Println()
	fmt.Print("确认回滚？(y/N): ")

	var confirm string
	fmt.Scanln(&confirm)

	if confirm != "y" && confirm != "Y" {
		fmt.Println("已取消")
		return
	}

	// 解析配置
	var config utils.AppConfig
	if err := json.Unmarshal([]byte(history.ConfigJSON), &config); err != nil {
		log.Fatalf("解析配置失败: %v", err)
	}

	// 保存为新版本
	note := fmt.Sprintf("从版本 %d 回滚", version)
	if err := model.SaveConfigVersion(history.ConfigJSON, "rollback", note); err != nil {
		log.Fatalf("保存回滚版本失败: %v", err)
	}

	// 写入配置文件
	utils.WriteConfig(&config)

	fmt.Printf("已成功回滚到版本 %d，配置文件已更新\n", version)
}

// handleConfigExport 导出指定版本配置到文件
func handleConfigExport(version int64, output string) {
	history, err := model.GetConfigHistoryByVersion(version)
	if err != nil {
		log.Fatalf("获取配置版本失败: %v", err)
	}
	if history == nil {
		fmt.Printf("版本 %d 不存在\n", version)
		return
	}

	if output == "" {
		output = fmt.Sprintf("config.v%d.yaml", version)
	}

	// 解析 JSON 并转换为 YAML
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(history.ConfigJSON), &config); err != nil {
		log.Fatalf("解析配置失败: %v", err)
	}

	yamlData, err := yaml.Parser().Marshal(config)
	if err != nil {
		log.Fatalf("转换 YAML 失败: %v", err)
	}

	if err := os.WriteFile(output, yamlData, 0644); err != nil {
		log.Fatalf("写入文件失败: %v", err)
	}

	fmt.Printf("已导出版本 %d 到 %s\n", version, output)
}

// handleSQLiteVacuum 手动执行 SQLite VACUUM 空间整理
func handleSQLiteVacuum() error {
	if !model.IsSQLite() {
		return fmt.Errorf("当前数据库不是 SQLite，无法执行 VACUUM")
	}
	if err := model.VacuumSQLite(); err != nil {
		return err
	}
	fmt.Println("SQLite VACUUM 执行完成")
	return nil
}
