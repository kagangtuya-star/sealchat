package utils

import (
	"os"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const (
	EnvDSN        = "SEALCHAT_DSN"
	DefaultSQLite = "./data/chat.db"
)

// ConfigFilePath preserves the legacy path unless explicitly overridden.
func ConfigFilePath() string {
	if path := strings.TrimSpace(os.Getenv("SEALCHAT_CONFIG_PATH")); path != "" {
		return path
	}
	return "config.yaml"
}

// GetMinimalDSN 获取最小 DSN 用于引导启动
// 优先级：环境变量 > 默认 SQLite
func GetMinimalDSN() string {
	if dsn := strings.TrimSpace(os.Getenv(EnvDSN)); dsn != "" {
		return dsn
	}
	return DefaultSQLite
}

// GetDSNForCLI 获取 CLI 命令使用的 DSN
// 优先级：环境变量 > 配置文件 > 默认 SQLite
func GetDSNForCLI() string {
	// 1. 环境变量优先
	if dsn := strings.TrimSpace(os.Getenv(EnvDSN)); dsn != "" {
		return dsn
	}

	// 2. 尝试从配置文件读取
	if ConfigFileExists() {
		k := koanf.New(".")
		if err := k.Load(file.Provider(ConfigFilePath()), yaml.Parser()); err == nil {
			if dsn := strings.TrimSpace(k.String("dbUrl")); dsn != "" {
				return dsn
			}
		}
	}

	// 3. 回退到默认值
	return DefaultSQLite
}

// ConfigFileExists 检查配置文件是否存在
func ConfigFileExists() bool {
	_, err := os.Stat(ConfigFilePath())
	return err == nil
}
