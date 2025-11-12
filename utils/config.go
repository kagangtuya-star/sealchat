package utils

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"github.com/samber/lo"
)

type LogUploadConfig struct {
	Enabled        bool   `json:"enabled" yaml:"enabled"`
	Endpoint       string `json:"endpoint" yaml:"endpoint"`
	Token          string `json:"token,omitempty" yaml:"token"`
	TimeoutSeconds int    `json:"timeoutSeconds" yaml:"timeoutSeconds"`
	Client         string `json:"client" yaml:"client"`
	UniformID      string `json:"uniformId" yaml:"uniformId"`
	Version        int    `json:"version" yaml:"version"`
	Note           string `json:"note" yaml:"note"`
}

type AudioConfig struct {
	StorageDir         string   `json:"storageDir" yaml:"storageDir"`
	TempDir            string   `json:"tempDir" yaml:"tempDir"`
	MaxUploadSizeMB    int64    `json:"maxUploadSizeMB" yaml:"maxUploadSizeMB"`
	AllowedMimeTypes   []string `json:"allowedMimeTypes" yaml:"allowedMimeTypes"`
	EnableTranscode    bool     `json:"enableTranscode" yaml:"enableTranscode"`
	DefaultBitrateKbps int      `json:"defaultBitrateKbps" yaml:"defaultBitrateKbps"`
	AlternateBitrates  []int    `json:"alternateBitrates" yaml:"alternateBitrates"`
	FFmpegPath         string   `json:"ffmpegPath" yaml:"ffmpegPath"`
}

type AppConfig struct {
	ServeAt                   string          `json:"serveAt" yaml:"serveAt"`
	Domain                    string          `json:"domain" yaml:"domain"`
	ImageBaseURL              string          `json:"imageBaseUrl" yaml:"imageBaseUrl"`
	RegisterOpen              bool            `json:"registerOpen" yaml:"registerOpen"`
	WebUrl                    string          `json:"webUrl" yaml:"webUrl"`
	ChatHistoryPersistentDays int64           `json:"chatHistoryPersistentDays" yaml:"chatHistoryPersistentDays"`
	ImageSizeLimit            int64           `json:"imageSizeLimit" yaml:"imageSizeLimit"` // in kb
	ImageCompress             bool            `json:"imageCompress" yaml:"imageCompress"`
	ImageCompressQuality      int             `json:"imageCompressQuality" yaml:"imageCompressQuality"`
	DSN                       string          `json:"-" yaml:"dbUrl" koanf:"dbUrl"`
	BuiltInSealBotEnable      bool            `json:"builtInSealBotEnable" yaml:"builtInSealBotEnable"` // 内置小海豹启用
	Version                   int             `json:"version" yaml:"version"`
	GalleryQuotaMB            int64           `json:"galleryQuotaMB" yaml:"galleryQuotaMB"`
	LogUpload                 LogUploadConfig `json:"logUpload" yaml:"logUpload"`
	Audio                     AudioConfig     `json:"audio" yaml:"audio"`
}

// 注: 实验型使用koanf，其实从需求上讲目前并无必要
var (
	k             = koanf.New(".")
	currentConfig *AppConfig
)

func GetConfig() *AppConfig {
	return currentConfig
}

func ReadConfig() *AppConfig {
	config := AppConfig{
		ServeAt:                   ":3212",
		Domain:                    "127.0.0.1:3212",
		RegisterOpen:              true,
		WebUrl:                    "/",
		ChatHistoryPersistentDays: -1,
		ImageSizeLimit:            8192,
		ImageCompress:             true,
		ImageCompressQuality:      85,
		DSN:                       "./data/chat.db",
		BuiltInSealBotEnable:      true,
		Version:                   1,
		GalleryQuotaMB:            100,
		LogUpload: LogUploadConfig{
			Enabled:        true,
			Endpoint:       "https://dice.weizaima.com/dice/api/log",
			TimeoutSeconds: 15,
			Client:         "Others",
			UniformID:      "Sealchat",
			Version:        105,
			Note:           "默认上传到 DicePP 云端获取海豹染色器 BBcode/Docx",
		},
		Audio: AudioConfig{
			StorageDir:         "./static/audio",
			TempDir:            "./data/audio-temp",
			MaxUploadSizeMB:    80,
			AllowedMimeTypes:   []string{"audio/mpeg", "audio/ogg", "audio/wav", "audio/x-wav", "audio/webm", "audio/aac", "audio/flac"},
			EnableTranscode:    true,
			DefaultBitrateKbps: 96,
			AlternateBitrates:  []int{64, 128},
			FFmpegPath:         "",
		},
	}

	if strings.TrimSpace(config.ImageBaseURL) == "" {
		config.ImageBaseURL = defaultImageBaseURL(config.ServeAt)
	}

	lo.Must0(k.Load(structs.Provider(&config, "yaml"), nil))

	f := file.Provider("config.yaml")
	// _ = f.Watch(func(event interface{}, err error) {
	// 	if err != nil {
	// 		log.Printf("watch error: %v", err)
	// 		return
	// 	}
	//
	// 	log.Println("config changed. Reloading ...")
	// 	k = koanf.New(".")
	// 	lo.Must0(k.Load(structs.Provider(&config, "yaml"), nil))
	// 	lo.Must0(k.Load(f, yaml.Parser()))
	// 	k.Print()
	// })

	isNotExist := false
	if err := k.Load(f, yaml.Parser()); err != nil {
		fmt.Printf("配置读取失败: %v\n", err)

		if os.IsNotExist(err) {
			isNotExist = true
		} else {
			os.Exit(-1)
		}
	}

	if isNotExist {
		WriteConfig(nil)
	} else {
		if err := k.Unmarshal("", &config); err != nil {
			fmt.Printf("配置解析失败: %v\n", err)
			os.Exit(-1)
		}
	}

	if strings.TrimSpace(config.ImageBaseURL) == "" {
		config.ImageBaseURL = defaultImageBaseURL(config.ServeAt)
	}

	config.ImageCompressQuality = normalizeImageCompressQuality(config.ImageCompressQuality)

	k.Print()
	currentConfig = &config
	return currentConfig
}

func WriteConfig(config *AppConfig) {
	if config != nil {
		config.ImageCompressQuality = normalizeImageCompressQuality(config.ImageCompressQuality)
		if config.ServeAt != "" {
			_ = k.Set("serveAt", config.ServeAt)
		}
		if config.Domain != "" {
			_ = k.Set("domain", config.Domain)
		}
		_ = k.Set("registerOpen", config.RegisterOpen)
		_ = k.Set("webUrl", config.WebUrl)
		_ = k.Set("chatHistoryPersistentDays", config.ChatHistoryPersistentDays)
		_ = k.Set("imageSizeLimit", config.ImageSizeLimit)
		_ = k.Set("imageCompress", config.ImageCompress)
		_ = k.Set("imageCompressQuality", config.ImageCompressQuality)
		_ = k.Set("builtInSealBotEnable", config.BuiltInSealBotEnable)
		_ = k.Set("galleryQuotaMB", config.GalleryQuotaMB)
		_ = k.Set("imageBaseUrl", config.ImageBaseURL)
		_ = k.Set("logUpload.enabled", config.LogUpload.Enabled)
		_ = k.Set("logUpload.endpoint", config.LogUpload.Endpoint)
		_ = k.Set("logUpload.token", config.LogUpload.Token)
		_ = k.Set("logUpload.timeoutSeconds", config.LogUpload.TimeoutSeconds)
		_ = k.Set("logUpload.client", config.LogUpload.Client)
		_ = k.Set("logUpload.uniformId", config.LogUpload.UniformID)
		_ = k.Set("logUpload.version", config.LogUpload.Version)
		_ = k.Set("logUpload.note", config.LogUpload.Note)
		_ = k.Set("audio.storageDir", config.Audio.StorageDir)
		_ = k.Set("audio.tempDir", config.Audio.TempDir)
		_ = k.Set("audio.maxUploadSizeMB", config.Audio.MaxUploadSizeMB)
		_ = k.Set("audio.allowedMimeTypes", config.Audio.AllowedMimeTypes)
		_ = k.Set("audio.enableTranscode", config.Audio.EnableTranscode)
		_ = k.Set("audio.defaultBitrateKbps", config.Audio.DefaultBitrateKbps)
		_ = k.Set("audio.alternateBitrates", config.Audio.AlternateBitrates)
		_ = k.Set("audio.ffmpegPath", config.Audio.FFmpegPath)

		if err := k.Unmarshal("", config); err != nil {
			fmt.Printf("配置解析失败: %v\n", err)
			os.Exit(-1)
		}
		currentConfig = config
	}

	content, err := yaml.Parser().Marshal(k.Raw())
	if err != nil {
		fmt.Println("错误: 配置文件序列化失败")
		return
	}
	err = os.WriteFile("./config.yaml", content, 0644)
	if err != nil {
		fmt.Println("错误: 配置文件写入失败")
	}
}

func defaultImageBaseURL(serveAt string) string {
	host, port := splitHostPort(serveAt)
	if port == "" {
		port = "3212"
	}
	ip := host
	if ip == "" || ip == "0.0.0.0" || ip == "::" || ip == "[::]" {
		if detected := detectLocalIPv4(); detected != "" {
			ip = detected
		} else {
			ip = "127.0.0.1"
		}
	}
	return fmt.Sprintf("%s:%s", ip, port)
}

func splitHostPort(addr string) (string, string) {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return "", ""
	}
	if !strings.Contains(trimmed, ":") {
		return trimmed, ""
	}
	host, port, err := net.SplitHostPort(trimmed)
	if err != nil {
		return trimmed, ""
	}
	return host, port
}

func detectLocalIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if (iface.Flags&net.FlagUp) == 0 || (iface.Flags&net.FlagLoopback) != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				if ip := v.IP.To4(); ip != nil {
					return ip.String()
				}
			case *net.IPAddr:
				if ip := v.IP.To4(); ip != nil {
					return ip.String()
				}
			}
		}
	}
	return ""
}

func normalizeImageCompressQuality(val int) int {
	if val < 1 || val > 100 {
		return 85
	}
	return val
}
