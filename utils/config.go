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

type OneBotForwardWSConfig struct {
	Host          string `json:"host" yaml:"host"`
	Port          int    `json:"port" yaml:"port"`
	APIPath       string `json:"apiPath" yaml:"apiPath"`
	EventPath     string `json:"eventPath" yaml:"eventPath"`
	UniversalPath string `json:"universalPath" yaml:"universalPath"`
}

type OneBotReverseWSConfig struct {
	Enabled                   bool     `json:"enabled" yaml:"enabled"`
	APIEndpoints              []string `json:"apiEndpoints" yaml:"apiEndpoints"`
	EventEndpoints            []string `json:"eventEndpoints" yaml:"eventEndpoints"`
	UniversalEndpoints        []string `json:"universalEndpoints" yaml:"universalEndpoints"`
	UseUniversalEndpointFirst bool     `json:"useUniversalEndpointFirst" yaml:"useUniversalEndpointFirst"`
	ReconnectIntervalSeconds  int      `json:"reconnectIntervalSeconds" yaml:"reconnectIntervalSeconds"`
}

type OneBotAuthConfig struct {
	AccessToken string `json:"accessToken" yaml:"accessToken"`
}

type OneBotConfig struct {
	Enabled         bool                   `json:"enabled" yaml:"enabled"`
	Version         string                 `json:"version" yaml:"version"`
	DefaultConnMode string                 `json:"defaultConnMode" yaml:"defaultConnMode"`
	Auth            OneBotAuthConfig       `json:"auth" yaml:"auth"`
	WS              OneBotForwardWSConfig  `json:"ws" yaml:"ws"`
	WSReverse       OneBotReverseWSConfig  `json:"wsReverse" yaml:"wsReverse"`
	Metadata        map[string]interface{} `json:"metadata,omitempty" yaml:"metadata"`
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
	OneBot                    OneBotConfig    `json:"oneBot" yaml:"oneBot"`
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
		OneBot: OneBotConfig{
			Enabled:         false,
			Version:         "v11",
			DefaultConnMode: "forward_ws",
			Auth: OneBotAuthConfig{
				AccessToken: "",
			},
			WS: OneBotForwardWSConfig{
				Host:          "0.0.0.0",
				Port:          33212,
				APIPath:       "/onebot/ws/api",
				EventPath:     "/onebot/ws/event",
				UniversalPath: "/onebot/ws/",
			},
			WSReverse: OneBotReverseWSConfig{
				Enabled:                  false,
				ReconnectIntervalSeconds: 10,
			},
			Metadata: map[string]interface{}{
				"description": "OneBot v11 基础配置，wsReverse 可按需启用",
			},
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
		_ = k.Set("oneBot.enabled", config.OneBot.Enabled)
		_ = k.Set("oneBot.version", config.OneBot.Version)
		_ = k.Set("oneBot.defaultConnMode", config.OneBot.DefaultConnMode)
		_ = k.Set("oneBot.auth.accessToken", config.OneBot.Auth.AccessToken)
		_ = k.Set("oneBot.ws.host", config.OneBot.WS.Host)
		_ = k.Set("oneBot.ws.port", config.OneBot.WS.Port)
		_ = k.Set("oneBot.ws.apiPath", config.OneBot.WS.APIPath)
		_ = k.Set("oneBot.ws.eventPath", config.OneBot.WS.EventPath)
		_ = k.Set("oneBot.ws.universalPath", config.OneBot.WS.UniversalPath)
		_ = k.Set("oneBot.wsReverse.enabled", config.OneBot.WSReverse.Enabled)
		_ = k.Set("oneBot.wsReverse.apiEndpoints", config.OneBot.WSReverse.APIEndpoints)
		_ = k.Set("oneBot.wsReverse.eventEndpoints", config.OneBot.WSReverse.EventEndpoints)
		_ = k.Set("oneBot.wsReverse.universalEndpoints", config.OneBot.WSReverse.UniversalEndpoints)
		_ = k.Set("oneBot.wsReverse.useUniversalEndpointFirst", config.OneBot.WSReverse.UseUniversalEndpointFirst)
		_ = k.Set("oneBot.wsReverse.reconnectIntervalSeconds", config.OneBot.WSReverse.ReconnectIntervalSeconds)
		_ = k.Set("oneBot.metadata", config.OneBot.Metadata)

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
