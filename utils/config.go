package utils

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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
	StorageDir               string   `json:"storageDir" yaml:"storageDir"`
	TempDir                  string   `json:"tempDir" yaml:"tempDir"`
	ImportDir                string   `json:"importDir" yaml:"importDir"`
	MaxUploadSizeMB          int64    `json:"maxUploadSizeMB" yaml:"maxUploadSizeMB"`
	AllowedMimeTypes         []string `json:"allowedMimeTypes" yaml:"allowedMimeTypes"`
	EnableTranscode          bool     `json:"enableTranscode" yaml:"enableTranscode"`
	DefaultBitrateKbps       int      `json:"defaultBitrateKbps" yaml:"defaultBitrateKbps"`
	AlternateBitrates        []int    `json:"alternateBitrates" yaml:"alternateBitrates"`
	FFmpegPath               string   `json:"ffmpegPath" yaml:"ffmpegPath"`
	AllowWorldAudioWorkbench bool     `json:"allowWorldAudioWorkbench" yaml:"allowWorldAudioWorkbench"`
	AllowNonAdminCreateWorld bool     `json:"allowNonAdminCreateWorld" yaml:"allowNonAdminCreateWorld"`
}

type StorageMode string

const (
	StorageModeAuto  StorageMode = "auto"
	StorageModeLocal StorageMode = "local"
	StorageModeS3    StorageMode = "s3"
)

const (
	defaultPageTitle                = "海豹尬聊 SealChat"
	defaultExportStorageDir         = "./data/exports"
	defaultExportHTMLPageSize       = 100
	defaultExportHTMLPageSizeMax    = 500
	defaultExportHTMLMaxConcurrency = 2
	defaultBackupPath               = "./backups"
	defaultBackupIntervalHours      = 12
	defaultBackupRetentionCount     = 5
)

type CaptchaMode string

const (
	CaptchaModeOff       CaptchaMode = "off"
	CaptchaModeLocal     CaptchaMode = "local"
	CaptchaModeTurnstile CaptchaMode = "turnstile"
)

type TurnstileConfig struct {
	SiteKey   string `json:"siteKey" yaml:"siteKey"`
	SecretKey string `json:"secretKey" yaml:"secretKey"`
}

type CaptchaTargetConfig struct {
	Mode      CaptchaMode     `json:"mode" yaml:"mode"`
	Turnstile TurnstileConfig `json:"turnstile" yaml:"turnstile"`
}

type CaptchaScene string

const (
	CaptchaSceneSignup        CaptchaScene = "signup"
	CaptchaSceneSignin        CaptchaScene = "signin"
	CaptchaScenePasswordReset CaptchaScene = "passwordReset"
)

type CaptchaConfig struct {
	Mode          CaptchaMode         `json:"mode,omitempty" yaml:"mode,omitempty"`
	Turnstile     TurnstileConfig     `json:"turnstile,omitempty" yaml:"turnstile,omitempty"`
	Signup        CaptchaTargetConfig `json:"signup" yaml:"signup"`
	Signin        CaptchaTargetConfig `json:"signin" yaml:"signin"`
	PasswordReset CaptchaTargetConfig `json:"passwordReset" yaml:"passwordReset"`
}

type StorageConfig struct {
	Mode       StorageMode        `json:"mode" yaml:"mode"`
	BaseURL    string             `json:"baseUrl" yaml:"baseUrl"`
	PresignTTL int                `json:"presignTTL" yaml:"presignTTL"`
	MaxSizeMB  int64              `json:"maxSizeMB" yaml:"maxSizeMB"`
	LogLevel   string             `json:"logLevel" yaml:"logLevel"`
	Local      LocalStorageConfig `json:"local" yaml:"local"`
	S3         S3StorageConfig    `json:"s3" yaml:"s3"`
}

type LocalStorageConfig struct {
	UploadDir string `json:"uploadDir" yaml:"uploadDir"`
	AudioDir  string `json:"audioDir" yaml:"audioDir"`
	TempDir   string `json:"tempDir" yaml:"tempDir"`
	BaseURL   string `json:"baseUrl" yaml:"baseUrl"`
}

type S3StorageConfig struct {
	Enabled            bool   `json:"enabled" yaml:"enabled"`
	AttachmentsEnabled *bool  `json:"attachmentsEnabled" yaml:"attachmentsEnabled"`
	AudioEnabled       *bool  `json:"audioEnabled" yaml:"audioEnabled"`
	Endpoint           string `json:"endpoint" yaml:"endpoint"`
	Region             string `json:"region" yaml:"region"`
	Bucket             string `json:"bucket" yaml:"bucket"`
	AccessKey          string `json:"accessKey" yaml:"accessKey"`
	SecretKey          string `json:"secretKey" yaml:"secret" koanf:"secret"`
	SessionToken       string `json:"sessionToken" yaml:"sessionToken"`
	ForcePathStyle     bool   `json:"forcePathStyle" yaml:"pathStyle"`
	BaseURL            string `json:"baseUrl" yaml:"baseUrl"`
	PublicBaseURL      string `json:"publicBaseUrl" yaml:"publicBaseUrl"`
	UseSSL             bool   `json:"useSSL" yaml:"useSSL"`
	PresignTTL         int    `json:"presignTTL" yaml:"presignTTL"`
	MaxSizeMB          int64  `json:"maxSizeMB" yaml:"maxSizeMB"`
	LogLevel           string `json:"logLevel" yaml:"logLevel"`
}

// SMTPConfig SMTP 邮件服务配置
type SMTPConfig struct {
	Host        string `json:"host" yaml:"host"`
	Port        int    `json:"port" yaml:"port"`
	Username    string `json:"username" yaml:"username"`
	Password    string `json:"-" yaml:"password"` // 禁止通过 JSON 输出
	FromAddress string `json:"fromAddress" yaml:"fromAddress"`
	FromName    string `json:"fromName" yaml:"fromName"`
	UseTLS      bool   `json:"useTLS" yaml:"useTLS"`
	SkipVerify  bool   `json:"skipVerify" yaml:"skipVerify"`
}

// EmailNotificationConfig 邮件通知功能配置
type EmailNotificationConfig struct {
	Enabled          bool       `json:"enabled" yaml:"enabled"`
	CheckIntervalSec int        `json:"-" yaml:"checkIntervalSec"` // 禁止前端获取
	MaxPerHour       int        `json:"-" yaml:"maxPerHour"`       // 禁止前端获取
	MinDelayMinutes  int        `json:"minDelayMinutes" yaml:"minDelayMinutes"`
	MaxDelayMinutes  int        `json:"maxDelayMinutes" yaml:"maxDelayMinutes"`
	SMTP             SMTPConfig `json:"-" yaml:"smtp"` // 禁止前端获取
}

// UpdateCheckConfig 更新检测配置
type UpdateCheckConfig struct {
	Enabled     bool   `json:"-" yaml:"enabled"`
	IntervalSec int    `json:"-" yaml:"intervalSec"`
	GithubRepo  string `json:"-" yaml:"githubRepo"`
	GithubToken string `json:"-" yaml:"githubToken"`
}

// EmailAuthConfig 邮箱认证配置（SMTP 配置复用 emailNotification.smtp）
type EmailAuthConfig struct {
	Enabled        bool `json:"enabled" yaml:"enabled"`
	CodeLength     int  `json:"-" yaml:"codeLength"`
	CodeTTLSeconds int  `json:"-" yaml:"codeTTLSeconds"`
	MaxAttempts    int  `json:"-" yaml:"maxAttempts"`
	RateLimitPerIP int  `json:"-" yaml:"rateLimitPerIP"`
}

// BackupConfig SQLite 备份配置
type BackupConfig struct {
	Enabled        bool   `json:"enabled" yaml:"enabled"`
	IntervalHours  int    `json:"intervalHours" yaml:"intervalHours"`
	RetentionCount int    `json:"retentionCount" yaml:"retentionCount"`
	Path           string `json:"path" yaml:"path"`
}

// LoginBackgroundConfig 登录页背景配置
type LoginBackgroundConfig struct {
	AttachmentId   string `json:"attachmentId" yaml:"attachmentId"`
	Mode           string `json:"mode" yaml:"mode"`
	Opacity        int    `json:"opacity" yaml:"opacity"`
	Blur           int    `json:"blur" yaml:"blur"`
	Brightness     int    `json:"brightness" yaml:"brightness"`
	OverlayColor   string `json:"overlayColor" yaml:"overlayColor"`
	OverlayOpacity int    `json:"overlayOpacity" yaml:"overlayOpacity"`
}

type AppConfig struct {
	ServeAt                   string                  `json:"serveAt" yaml:"serveAt"`
	Domain                    string                  `json:"domain" yaml:"domain"`
	ImageBaseURL              string                  `json:"imageBaseUrl" yaml:"imageBaseUrl"`
	RegisterOpen              bool                    `json:"registerOpen" yaml:"registerOpen"`
	WebUrl                    string                  `json:"webUrl" yaml:"webUrl"`
	PageTitle                 string                  `json:"pageTitle" yaml:"pageTitle"`
	ChatHistoryPersistentDays int64                   `json:"chatHistoryPersistentDays" yaml:"chatHistoryPersistentDays"`
	TypingOrderWindowMs       int64                   `json:"typingOrderWindowMs" yaml:"typingOrderWindowMs"`
	ImageSizeLimit            int64                   `json:"imageSizeLimit" yaml:"imageSizeLimit"` // in kb
	ImageCompress             bool                    `json:"imageCompress" yaml:"imageCompress"`
	ImageCompressQuality      int                     `json:"imageCompressQuality" yaml:"imageCompressQuality"`
	KeywordMaxLength          int64                   `json:"keywordMaxLength" yaml:"keywordMaxLength"` // 术语最大字数
	DSN                       string                  `json:"-" yaml:"dbUrl" koanf:"dbUrl"`
	BuiltInSealBotEnable      bool                    `json:"builtInSealBotEnable" yaml:"builtInSealBotEnable"` // 内置小海豹启用
	Version                   int                     `json:"version" yaml:"version"`
	GalleryQuotaMB            int64                   `json:"galleryQuotaMB" yaml:"galleryQuotaMB"`
	LogUpload                 LogUploadConfig         `json:"logUpload" yaml:"logUpload"`
	Audio                     AudioConfig             `json:"audio" yaml:"audio"`
	Export                    ExportConfig            `json:"export" yaml:"export"`
	Storage                   StorageConfig           `json:"storage" yaml:"storage"`
	SQLite                    SQLiteConfig            `json:"sqlite" yaml:"sqlite"`
	Captcha                   CaptchaConfig           `json:"captcha" yaml:"captcha"`
	EmailNotification         EmailNotificationConfig `json:"emailNotification" yaml:"emailNotification"`
	EmailAuth                 EmailAuthConfig         `json:"emailAuth" yaml:"emailAuth"`
	UpdateCheck               UpdateCheckConfig       `json:"updateCheck" yaml:"updateCheck"`
	Backup                    BackupConfig            `json:"backup" yaml:"backup"`
	LoginBackground           LoginBackgroundConfig   `json:"loginBackground" yaml:"loginBackground"`
}

type ExportConfig struct {
	StorageDir            string `json:"storageDir" yaml:"storageDir"`
	DownloadBandwidthKBps int    `json:"downloadBandwidthKBps" yaml:"downloadBandwidthKBps"`
	DownloadBurstKB       int    `json:"downloadBurstKB" yaml:"downloadBurstKB"`
	HTMLPageSizeDefault   int    `json:"htmlPageSizeDefault" yaml:"htmlPageSizeDefault"`
	HTMLPageSizeMax       int    `json:"htmlPageSizeMax" yaml:"htmlPageSizeMax"`
	HTMLMaxConcurrency    int    `json:"htmlMaxConcurrency" yaml:"htmlMaxConcurrency"`
}

// SQLiteConfig 用于细化 SQLite 数据库的运行参数
type SQLiteConfig struct {
	// 是否启用 WAL（默认开启）
	EnableWAL bool `json:"enableWAL" yaml:"wal"`
	// busy_timeout 毫秒，避免高并发下频繁数据库锁冲突
	BusyTimeoutMS int `json:"busyTimeoutMS" yaml:"busyTimeout"`
	// cache_size 以 KB 计，负值代表 KB，默认 512MB
	CacheSizeKB int `json:"cacheSizeKB" yaml:"cacheSizeKB"`
	// synchronous 模式：OFF/NORMAL/FULL，默认 NORMAL
	Synchronous string `json:"synchronous" yaml:"synchronous"`
	// 是否在连接串追加 _txlock=immediate，提前写锁，默认开启
	TxLockImmediate bool `json:"txLockImmediate" yaml:"txLockImmediate"`
	// 读取连接池大小，默认按 CPU 数；写连接默认 1（共用池）
	ReadConnections int `json:"readConnections" yaml:"readConnections"`
	// 初始化时是否执行 PRAGMA optimize
	OptimizeOnInit bool `json:"optimizeOnInit" yaml:"optimizeOnInit"`
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
		PageTitle:                 defaultPageTitle,
	ChatHistoryPersistentDays: -1,
	TypingOrderWindowMs:       1000,
		ImageSizeLimit:            8192,
		ImageCompress:             true,
		ImageCompressQuality:      85,
		KeywordMaxLength:          2000,
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
			StorageDir:               "./static/audio",
			TempDir:                  "./data/audio-temp",
			ImportDir:                "./static/audio/import",
			MaxUploadSizeMB:          80,
			AllowedMimeTypes:         []string{"audio/mpeg", "audio/ogg", "audio/wav", "audio/x-wav", "audio/webm", "audio/aac", "audio/flac"},
			EnableTranscode:          true,
			DefaultBitrateKbps:       96,
			AlternateBitrates:        []int{64, 128},
			FFmpegPath:               "",
			AllowWorldAudioWorkbench: false,
			AllowNonAdminCreateWorld: true,
		},
		Export: ExportConfig{
			StorageDir:            defaultExportStorageDir,
			DownloadBandwidthKBps: 0,
			DownloadBurstKB:       0,
			HTMLPageSizeDefault:   defaultExportHTMLPageSize,
			HTMLPageSizeMax:       defaultExportHTMLPageSizeMax,
			HTMLMaxConcurrency:    defaultExportHTMLMaxConcurrency,
		},
		Storage: StorageConfig{
			Mode:       StorageModeLocal,
			PresignTTL: 900,
			MaxSizeMB:  64,
			Local: LocalStorageConfig{
				UploadDir: "./data/upload",
				AudioDir:  "./static/audio",
				TempDir:   "./data/temp",
			},
			S3: S3StorageConfig{
				UseSSL:     true,
				BaseURL:    "",
				MaxSizeMB:  64,
				PresignTTL: 900,
			},
		},
		SQLite: SQLiteConfig{
			EnableWAL:       true,
			BusyTimeoutMS:   10000,
			CacheSizeKB:     512000,
			Synchronous:     "NORMAL",
			TxLockImmediate: true,
			ReadConnections: runtime.NumCPU(),
			OptimizeOnInit:  true,
		},
		Captcha: CaptchaConfig{
			Signup: CaptchaTargetConfig{Mode: CaptchaModeLocal},
			Signin: CaptchaTargetConfig{Mode: CaptchaModeOff},
		},
		EmailNotification: EmailNotificationConfig{
			Enabled:          false,
			CheckIntervalSec: 60,
			MaxPerHour:       5,
			MinDelayMinutes:  10,
			MaxDelayMinutes:  30,
			SMTP: SMTPConfig{
				Port:     587,
				UseTLS:   true,
				FromName: "SealChat",
			},
		},
		EmailAuth: EmailAuthConfig{
			Enabled:        false,
			CodeLength:     6,
			CodeTTLSeconds: 300,
			MaxAttempts:    5,
			RateLimitPerIP: 10,
		},
		UpdateCheck: UpdateCheckConfig{
			Enabled:     true,
			IntervalSec: 6 * 60 * 60,
			GithubRepo:  "kagangtuya-star/sealchat",
		},
		Backup: BackupConfig{
			Enabled:        true,
			IntervalHours:  defaultBackupIntervalHours,
			RetentionCount: defaultBackupRetentionCount,
			Path:           defaultBackupPath,
		},
		LoginBackground: LoginBackgroundConfig{
			Mode:       "cover",
			Opacity:    30,
			Blur:       0,
			Brightness: 100,
		},
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

	if strings.TrimSpace(config.PageTitle) == "" {
		config.PageTitle = defaultPageTitle
	}
	normalizedServeAt, serveAtChanged := NormalizeServeAt(config.ServeAt)
	if serveAtChanged {
		config.ServeAt = normalizedServeAt
		_ = k.Set("serveAt", config.ServeAt)
	}
	normalizedDomain, domainChanged := NormalizeDomain(config.Domain)
	if domainChanged {
		config.Domain = normalizedDomain
		_ = k.Set("domain", config.Domain)
	}

	config.ImageCompressQuality = normalizeImageCompressQuality(config.ImageCompressQuality)
	config.Storage.normalize()
		applyStorageEnvOverrides(&config.Storage)
		if strings.TrimSpace(config.Storage.Local.AudioDir) == "" {
			config.Storage.Local.AudioDir = config.Audio.StorageDir
		}
		if strings.TrimSpace(config.Audio.ImportDir) == "" && strings.TrimSpace(config.Audio.StorageDir) != "" {
			config.Audio.ImportDir = filepath.Join(config.Audio.StorageDir, "import")
		}
		applyImageBaseURLFallback(&config)
		applySQLiteDefaults(&config.SQLite)
		applyExportDefaults(&config.Export)
	config.Captcha.normalize()
	applyEmailNotificationDefaults(&config.EmailNotification)
	applyEmailAuthDefaults(&config.EmailAuth)
	applyUpdateCheckDefaults(&config.UpdateCheck)
	applyBackupDefaults(&config.Backup)

	k.Print()
	currentConfig = &config
	return currentConfig
}

func applySQLiteDefaults(cfg *SQLiteConfig) {
	if cfg == nil {
		return
	}
	if cfg.BusyTimeoutMS <= 0 {
		cfg.BusyTimeoutMS = 10000
	}
	if cfg.CacheSizeKB == 0 {
		cfg.CacheSizeKB = 512000
	}
	if cfg.Synchronous == "" {
		cfg.Synchronous = "NORMAL"
	}
	if cfg.ReadConnections <= 0 {
		cfg.ReadConnections = runtime.NumCPU()
	}
	cfg.Synchronous = strings.ToUpper(cfg.Synchronous)
	if cfg.Synchronous != "OFF" && cfg.Synchronous != "NORMAL" && cfg.Synchronous != "FULL" {
		cfg.Synchronous = "NORMAL"
	}
}

func applyExportDefaults(cfg *ExportConfig) {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(cfg.StorageDir) == "" {
		cfg.StorageDir = defaultExportStorageDir
	}
	if cfg.HTMLPageSizeDefault <= 0 {
		cfg.HTMLPageSizeDefault = defaultExportHTMLPageSize
	}
	if cfg.HTMLPageSizeMax <= 0 {
		cfg.HTMLPageSizeMax = defaultExportHTMLPageSizeMax
	}
	if cfg.HTMLPageSizeDefault > cfg.HTMLPageSizeMax {
		cfg.HTMLPageSizeDefault = cfg.HTMLPageSizeMax
	}
	if cfg.HTMLMaxConcurrency <= 0 {
		cfg.HTMLMaxConcurrency = defaultExportHTMLMaxConcurrency
	}
	if cfg.DownloadBandwidthKBps < 0 {
		cfg.DownloadBandwidthKBps = 0
	}
	if cfg.DownloadBurstKB < 0 {
		cfg.DownloadBurstKB = 0
	}
}

func applyEmailNotificationDefaults(cfg *EmailNotificationConfig) {
	if cfg == nil {
		return
	}
	if cfg.CheckIntervalSec <= 0 {
		cfg.CheckIntervalSec = 60
	}
	if cfg.MaxPerHour <= 0 {
		cfg.MaxPerHour = 5
	}
	if cfg.MinDelayMinutes <= 0 {
		cfg.MinDelayMinutes = 10
	}
	if cfg.MaxDelayMinutes <= 0 {
		cfg.MaxDelayMinutes = 30
	}
	if cfg.MinDelayMinutes > cfg.MaxDelayMinutes {
		cfg.MinDelayMinutes = cfg.MaxDelayMinutes
	}
	if cfg.SMTP.Port <= 0 {
		cfg.SMTP.Port = 587
	}
	if strings.TrimSpace(cfg.SMTP.FromName) == "" {
		cfg.SMTP.FromName = "SealChat"
	}
	// SMTP 密码支持环境变量覆盖
	if pw := strings.TrimSpace(os.Getenv("SEALCHAT_SMTP_PASSWORD")); pw != "" {
		cfg.SMTP.Password = pw
	}
}

func applyEmailAuthDefaults(cfg *EmailAuthConfig) {
	if cfg == nil {
		return
	}
	if cfg.CodeLength <= 0 {
		cfg.CodeLength = 6
	}
	if cfg.CodeTTLSeconds <= 0 {
		cfg.CodeTTLSeconds = 300
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 5
	}
	if cfg.RateLimitPerIP <= 0 {
		cfg.RateLimitPerIP = 10
	}
}

func applyUpdateCheckDefaults(cfg *UpdateCheckConfig) {
	if cfg == nil {
		return
	}
	if cfg.IntervalSec <= 0 {
		cfg.IntervalSec = 6 * 60 * 60
	}
	if strings.TrimSpace(cfg.GithubRepo) == "" {
		cfg.GithubRepo = "kagangtuya-star/sealchat"
	}
	if token := strings.TrimSpace(os.Getenv("SEALCHAT_GITHUB_TOKEN")); token != "" {
		cfg.GithubToken = token
	}
}

func applyBackupDefaults(cfg *BackupConfig) {
	if cfg == nil {
		return
	}
	if cfg.IntervalHours <= 0 {
		cfg.IntervalHours = defaultBackupIntervalHours
	}
	if cfg.RetentionCount <= 0 {
		cfg.RetentionCount = defaultBackupRetentionCount
	}
	if strings.TrimSpace(cfg.Path) == "" {
		cfg.Path = defaultBackupPath
	}
}

func (cfg *CaptchaConfig) normalize() {
	if cfg == nil {
		return
	}
	applyCaptchaTargetDefaults(&cfg.Signup, cfg.Mode, CaptchaModeLocal, cfg.Turnstile)
	applyCaptchaTargetDefaults(&cfg.Signin, cfg.Mode, CaptchaModeOff, cfg.Turnstile)
	applyCaptchaTargetDefaults(&cfg.PasswordReset, cfg.Mode, CaptchaModeLocal, cfg.Turnstile)
}

func applyCaptchaTargetDefaults(target *CaptchaTargetConfig, globalMode, fallbackMode CaptchaMode, fallbackTurnstile TurnstileConfig) {
	if target == nil {
		return
	}
	if target.Mode == "" {
		if globalMode != "" {
			target.Mode = globalMode
		} else {
			target.Mode = fallbackMode
		}
	}
	if target.Turnstile.SiteKey == "" {
		target.Turnstile.SiteKey = fallbackTurnstile.SiteKey
	}
	if target.Turnstile.SecretKey == "" {
		target.Turnstile.SecretKey = fallbackTurnstile.SecretKey
	}
}

func (cfg *CaptchaConfig) Target(scene CaptchaScene) CaptchaTargetConfig {
	if cfg == nil {
		return CaptchaTargetConfig{}
	}
	switch scene {
	case CaptchaSceneSignin:
		return cfg.Signin
	case CaptchaScenePasswordReset:
		return cfg.PasswordReset
	default:
		return cfg.Signup
	}
}

func (cfg *CaptchaConfig) HasLocalEnabled() bool {
	if cfg == nil {
		return false
	}
	return cfg.Signup.Mode == CaptchaModeLocal || cfg.Signin.Mode == CaptchaModeLocal || cfg.PasswordReset.Mode == CaptchaModeLocal
}

func WriteConfig(config *AppConfig) {
	if config != nil {
		config.Captcha.normalize()
		config.Storage.normalize()
		config.ImageCompressQuality = normalizeImageCompressQuality(config.ImageCompressQuality)
		if strings.TrimSpace(config.PageTitle) == "" {
			config.PageTitle = defaultPageTitle
		}
		normalizedServeAt, serveAtChanged := NormalizeServeAt(config.ServeAt)
		if serveAtChanged {
			config.ServeAt = normalizedServeAt
		}
		normalizedDomain, domainChanged := NormalizeDomain(config.Domain)
		if domainChanged {
			config.Domain = normalizedDomain
		}
		if config.ServeAt != "" {
			_ = k.Set("serveAt", config.ServeAt)
		}
		if config.Domain != "" {
			_ = k.Set("domain", config.Domain)
		}
		_ = k.Set("registerOpen", config.RegisterOpen)
		_ = k.Set("webUrl", config.WebUrl)
		_ = k.Set("pageTitle", config.PageTitle)
		_ = k.Set("chatHistoryPersistentDays", config.ChatHistoryPersistentDays)
		_ = k.Set("imageSizeLimit", config.ImageSizeLimit)
		_ = k.Set("imageCompress", config.ImageCompress)
		_ = k.Set("imageCompressQuality", config.ImageCompressQuality)
		_ = k.Set("keywordMaxLength", config.KeywordMaxLength)
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
		_ = k.Set("audio.importDir", config.Audio.ImportDir)
		_ = k.Set("audio.maxUploadSizeMB", config.Audio.MaxUploadSizeMB)
		_ = k.Set("audio.allowedMimeTypes", config.Audio.AllowedMimeTypes)
		_ = k.Set("audio.enableTranscode", config.Audio.EnableTranscode)
		_ = k.Set("audio.defaultBitrateKbps", config.Audio.DefaultBitrateKbps)
		_ = k.Set("audio.alternateBitrates", config.Audio.AlternateBitrates)
		_ = k.Set("audio.ffmpegPath", config.Audio.FFmpegPath)
		_ = k.Set("audio.allowWorldAudioWorkbench", config.Audio.AllowWorldAudioWorkbench)
		_ = k.Set("audio.allowNonAdminCreateWorld", config.Audio.AllowNonAdminCreateWorld)
		_ = k.Set("export.storageDir", config.Export.StorageDir)
		_ = k.Set("export.downloadBandwidthKBps", config.Export.DownloadBandwidthKBps)
		_ = k.Set("export.downloadBurstKB", config.Export.DownloadBurstKB)
		_ = k.Set("export.htmlPageSizeDefault", config.Export.HTMLPageSizeDefault)
		_ = k.Set("export.htmlPageSizeMax", config.Export.HTMLPageSizeMax)
		_ = k.Set("export.htmlMaxConcurrency", config.Export.HTMLMaxConcurrency)
		_ = k.Set("storage.mode", config.Storage.Mode)
		_ = k.Set("storage.baseUrl", config.Storage.BaseURL)
		_ = k.Set("storage.presignTTL", config.Storage.PresignTTL)
		_ = k.Set("storage.maxSizeMB", config.Storage.MaxSizeMB)
		_ = k.Set("storage.logLevel", config.Storage.LogLevel)
		_ = k.Set("storage.local.uploadDir", config.Storage.Local.UploadDir)
		_ = k.Set("storage.local.audioDir", config.Storage.Local.AudioDir)
		_ = k.Set("storage.local.tempDir", config.Storage.Local.TempDir)
		_ = k.Set("storage.local.baseUrl", config.Storage.Local.BaseURL)
		_ = k.Set("storage.s3.enabled", config.Storage.S3.Enabled)
		if config.Storage.S3.AttachmentsEnabled != nil {
			_ = k.Set("storage.s3.attachmentsEnabled", *config.Storage.S3.AttachmentsEnabled)
		}
		if config.Storage.S3.AudioEnabled != nil {
			_ = k.Set("storage.s3.audioEnabled", *config.Storage.S3.AudioEnabled)
		}
		_ = k.Set("storage.s3.endpoint", config.Storage.S3.Endpoint)
		_ = k.Set("storage.s3.region", config.Storage.S3.Region)
		_ = k.Set("storage.s3.bucket", config.Storage.S3.Bucket)
		_ = k.Set("storage.s3.accessKey", config.Storage.S3.AccessKey)
		_ = k.Set("storage.s3.secret", config.Storage.S3.SecretKey)
		_ = k.Set("storage.s3.sessionToken", config.Storage.S3.SessionToken)
		_ = k.Set("storage.s3.pathStyle", config.Storage.S3.ForcePathStyle)
		_ = k.Set("storage.s3.baseUrl", config.Storage.S3.BaseURL)
		_ = k.Set("storage.s3.publicBaseUrl", config.Storage.S3.PublicBaseURL)
		_ = k.Set("storage.s3.useSSL", config.Storage.S3.UseSSL)
		_ = k.Set("storage.s3.presignTTL", config.Storage.S3.PresignTTL)
		_ = k.Set("storage.s3.maxSizeMB", config.Storage.S3.MaxSizeMB)
		_ = k.Set("storage.s3.logLevel", config.Storage.S3.LogLevel)
		_ = k.Set("captcha.mode", string(config.Captcha.Mode))
		_ = k.Set("captcha.turnstile.siteKey", config.Captcha.Turnstile.SiteKey)
		_ = k.Set("captcha.turnstile.secretKey", config.Captcha.Turnstile.SecretKey)
		_ = k.Set("captcha.signup.mode", string(config.Captcha.Signup.Mode))
		_ = k.Set("captcha.signup.turnstile.siteKey", config.Captcha.Signup.Turnstile.SiteKey)
		_ = k.Set("captcha.signup.turnstile.secretKey", config.Captcha.Signup.Turnstile.SecretKey)
		_ = k.Set("captcha.signin.mode", string(config.Captcha.Signin.Mode))
		_ = k.Set("captcha.signin.turnstile.siteKey", config.Captcha.Signin.Turnstile.SiteKey)
		_ = k.Set("captcha.signin.turnstile.secretKey", config.Captcha.Signin.Turnstile.SecretKey)

		// 邮件通知配置
		_ = k.Set("emailNotification.enabled", config.EmailNotification.Enabled)
		_ = k.Set("emailNotification.minDelayMinutes", config.EmailNotification.MinDelayMinutes)
		_ = k.Set("emailNotification.maxDelayMinutes", config.EmailNotification.MaxDelayMinutes)

		// 备份配置
		_ = k.Set("backup.enabled", config.Backup.Enabled)
		_ = k.Set("backup.intervalHours", config.Backup.IntervalHours)
		_ = k.Set("backup.retentionCount", config.Backup.RetentionCount)
		_ = k.Set("backup.path", config.Backup.Path)

		// 登录页背景配置
		_ = k.Set("loginBackground.attachmentId", config.LoginBackground.AttachmentId)
		_ = k.Set("loginBackground.mode", config.LoginBackground.Mode)
		_ = k.Set("loginBackground.opacity", config.LoginBackground.Opacity)
		_ = k.Set("loginBackground.blur", config.LoginBackground.Blur)
		_ = k.Set("loginBackground.brightness", config.LoginBackground.Brightness)
		_ = k.Set("loginBackground.overlayColor", config.LoginBackground.OverlayColor)
		_ = k.Set("loginBackground.overlayOpacity", config.LoginBackground.OverlayOpacity)

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

func applyImageBaseURLFallback(config *AppConfig) {
	if config == nil {
		return
	}
	if strings.TrimSpace(config.ImageBaseURL) == "" && strings.TrimSpace(config.Domain) == "" {
		config.ImageBaseURL = defaultImageBaseURL(config.ServeAt)
	}
}

func defaultImageBaseURL(serveAt string) string {
	host, port := splitHostPort(serveAt)
	if port == "" {
		port = "3212"
	}
	ip := host
	if ip == "" || ip == "0.0.0.0" || ip == "::" {
		if detected := detectLocalIPv4(); detected != "" {
			ip = detected
		} else if detected := detectLocalIPv6(); detected != "" {
			ip = detected
		} else {
			ip = "127.0.0.1"
		}
	}
	return FormatHostPort(ip, port)
}

func FormatHostPort(host, port string) string {
	formattedHost := EnsureIPv6Bracket(host)
	if port == "" {
		return formattedHost
	}
	if formattedHost == "" {
		return ":" + port
	}
	return fmt.Sprintf("%s:%s", formattedHost, port)
}

func FormatListenHostPort(host, port string) string {
	formattedHost := EnsureIPv6BracketForListen(host)
	if port == "" {
		return formattedHost
	}
	if formattedHost == "" {
		return ":" + port
	}
	return fmt.Sprintf("%s:%s", formattedHost, port)
}

// IsPortAvailable checks if a TCP port is available for binding
func IsPortAvailable(addr string) bool {
	return IsPortAvailableWithNetwork("tcp", addr)
}

// IsPortAvailableWithNetwork checks if a TCP port is available for binding with the given network.
func IsPortAvailableWithNetwork(network, addr string) bool {
	ln, err := net.Listen(network, addr)
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// FindAvailablePort tries the given address first, then searches nearby ports if occupied.
// Returns the available address and a boolean indicating if a fallback port was used.
func FindAvailablePort(addr string) (string, bool) {
	return FindAvailablePortWithNetwork("tcp", addr)
}

// FindAvailablePortWithNetwork tries the given address first, then searches nearby ports if occupied.
// Returns the available address and a boolean indicating if a fallback port was used.
func FindAvailablePortWithNetwork(network, addr string) (string, bool) {
	if IsPortAvailableWithNetwork(network, addr) {
		return addr, false
	}

	host, portStr := splitHostPort(addr)
	port := 3212
	if portStr != "" {
		if p, err := net.LookupPort("tcp", portStr); err == nil {
			port = p
		} else if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
			port = 3212
		}
	}

	// Try nearby ports: +1 to +100
	for offset := 1; offset <= 100; offset++ {
		candidate := FormatListenHostPort(host, fmt.Sprintf("%d", port+offset))
		if IsPortAvailableWithNetwork(network, candidate) {
			return candidate, true
		}
	}

	// If no port found, return original (will fail at Listen)
	return addr, false
}

func EnsureIPv6Bracket(host string) string {
	return ensureIPv6Bracket(host, true)
}

func EnsureIPv6BracketForListen(host string) string {
	return ensureIPv6Bracket(host, false)
}

func ensureIPv6Bracket(host string, encodeZone bool) string {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "]") {
		return trimmed
	}
	base, zone := normalizeIPv6Reference(trimmed)
	if base == "" {
		return trimmed
	}
	if ip := net.ParseIP(base); ip != nil && ip.To4() == nil {
		if encodeZone {
			zone = encodeIPv6Zone(zone)
		}
		return fmt.Sprintf("[%s%s]", base, zone)
	}
	return trimmed
}

func normalizeIPv6Reference(host string) (string, string) {
	withoutBrackets := strings.TrimSpace(host)
	withoutBrackets = strings.TrimPrefix(withoutBrackets, "[")
	if idx := strings.Index(withoutBrackets, "]"); idx >= 0 {
		withoutBrackets = withoutBrackets[:idx]
	}
	return splitIPv6Zone(withoutBrackets)
}

func splitIPv6Zone(host string) (string, string) {
	if host == "" {
		return "", ""
	}
	if idx := strings.LastIndex(host, "%"); idx >= 0 {
		return host[:idx], host[idx:]
	}
	return host, ""
}

func encodeIPv6Zone(zone string) string {
	if zone == "" {
		return ""
	}
	if strings.HasPrefix(zone, "%25") || !strings.HasPrefix(zone, "%") {
		return zone
	}
	return "%25" + zone[1:]
}

func NormalizeServeAt(addr string) (string, bool) {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return "", false
	}
	host, port := splitHostPort(trimmed)
	if port == "" {
		port = "3212"
	}
	normalized := FormatListenHostPort(host, port)
	return normalized, normalized != trimmed
}

func NormalizeDomain(domain string) (string, bool) {
	trimmed := strings.TrimSpace(domain)
	if trimmed == "" {
		return "", false
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		parsed, err := url.Parse(trimmed)
		if err != nil || parsed.Host == "" {
			return trimmed, false
		}
		host := parsed.Hostname()
		if host == "" {
			return trimmed, false
		}
		port := parsed.Port()
		if port != "" {
			parsed.Host = FormatHostPort(host, port)
		} else {
			parsed.Host = EnsureIPv6Bracket(host)
		}
		normalized := parsed.String()
		return normalized, normalized != trimmed
	}

	host, port := splitHostPort(trimmed)
	if host == "" {
		return trimmed, false
	}
	if port == "" {
		normalized := EnsureIPv6Bracket(host)
		return normalized, normalized != trimmed
	}
	normalized := FormatHostPort(host, port)
	return normalized, normalized != trimmed
}

func splitHostPort(addr string) (string, string) {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return "", ""
	}
	if strings.HasPrefix(trimmed, "[") {
		host, port, err := net.SplitHostPort(trimmed)
		if err == nil {
			return host, port
		}
		base, _ := normalizeIPv6Reference(trimmed)
		if base != "" {
			return base, ""
		}
		return trimmed, ""
	}
	if !strings.Contains(trimmed, ":") {
		return trimmed, ""
	}
	host, port, err := net.SplitHostPort(trimmed)
	if err != nil {
		if strings.Count(trimmed, ":") >= 2 {
			lastColon := strings.LastIndex(trimmed, ":")
			if lastColon > 0 && lastColon < len(trimmed)-1 {
				hostPart := strings.TrimSpace(trimmed[:lastColon])
				portPart := strings.TrimSpace(trimmed[lastColon+1:])
				if hostPart != "" && portPart != "" && isAllDigits(portPart) {
					base, _ := splitIPv6Zone(hostPart)
					if ip := net.ParseIP(base); ip != nil && ip.To4() == nil {
						return hostPart, portPart
					}
				}
			}
		}
		return trimmed, ""
	}
	return host, port
}

func isAllDigits(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
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

func detectLocalIPv6() string {
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
				ip := v.IP
				if ip == nil || ip.To4() != nil {
					continue
				}
				if ip.IsLinkLocalUnicast() {
					continue
				}
				return ip.String()
			case *net.IPAddr:
				ip := v.IP
				if ip == nil || ip.To4() != nil {
					continue
				}
				if ip.IsLinkLocalUnicast() {
					continue
				}
				return ip.String()
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

func (cfg *StorageConfig) normalize() {
	if cfg == nil {
		return
	}
	if cfg.Mode == "" {
		cfg.Mode = StorageModeLocal
	}
	if cfg.S3.AttachmentsEnabled == nil {
		v := true
		cfg.S3.AttachmentsEnabled = &v
	}
	if cfg.S3.AudioEnabled == nil {
		v := true
		cfg.S3.AudioEnabled = &v
	}
	if strings.TrimSpace(cfg.Local.UploadDir) == "" {
		cfg.Local.UploadDir = "./data/upload"
	}
	if strings.TrimSpace(cfg.Local.AudioDir) == "" {
		cfg.Local.AudioDir = "./static/audio"
	}
	if strings.TrimSpace(cfg.Local.TempDir) == "" {
		cfg.Local.TempDir = "./data/temp"
	}
	if cfg.S3.PublicBaseURL == "" {
		cfg.S3.PublicBaseURL = cfg.S3.BaseURL
	}
	if !cfg.S3.Enabled && strings.TrimSpace(cfg.S3.Bucket) != "" {
		cfg.S3.Enabled = true
	}
	if cfg.PresignTTL <= 0 {
		cfg.PresignTTL = 900
	}
	if cfg.S3.PresignTTL <= 0 {
		cfg.S3.PresignTTL = cfg.PresignTTL
	}
	if cfg.MaxSizeMB <= 0 {
		cfg.MaxSizeMB = 64
	}
	if cfg.S3.MaxSizeMB <= 0 {
		cfg.S3.MaxSizeMB = cfg.MaxSizeMB
	}
}

func applyStorageEnvOverrides(cfg *StorageConfig) {
	if cfg == nil {
		return
	}
	if ak := strings.TrimSpace(os.Getenv("SEALCHAT_S3_ACCESS_KEY")); ak != "" {
		cfg.S3.AccessKey = ak
	}
	if sk := strings.TrimSpace(os.Getenv("SEALCHAT_S3_SECRET_KEY")); sk != "" {
		cfg.S3.SecretKey = sk
	}
	if st := strings.TrimSpace(os.Getenv("SEALCHAT_S3_SESSION_TOKEN")); st != "" {
		cfg.S3.SessionToken = st
	}
}

// EnsureDataDirs 确保所有必要的数据目录存在
// 在启动时调用，避免目录不存在导致的错误（尤其是 Docker 环境）
func EnsureDataDirs(cfg *AppConfig) {
	if cfg == nil {
		return
	}

	// 基础数据目录
	dirs := []string{
		"./data",
		"./static",
	}

	// 根据配置添加目录
	if cfg.Storage.Local.UploadDir != "" {
		dirs = append(dirs, cfg.Storage.Local.UploadDir)
	}
	if cfg.Storage.Local.AudioDir != "" {
		dirs = append(dirs, cfg.Storage.Local.AudioDir)
	}
	if cfg.Storage.Local.TempDir != "" {
		dirs = append(dirs, cfg.Storage.Local.TempDir)
	}
	if cfg.Audio.StorageDir != "" {
		dirs = append(dirs, cfg.Audio.StorageDir)
	}
	if cfg.Audio.TempDir != "" {
		dirs = append(dirs, cfg.Audio.TempDir)
	}
	if cfg.Audio.ImportDir != "" {
		dirs = append(dirs, cfg.Audio.ImportDir)
	}
	if cfg.Export.StorageDir != "" {
		dirs = append(dirs, cfg.Export.StorageDir)
	}

	// 创建所有目录
	for _, dir := range dirs {
		if dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				fmt.Printf("创建目录 %s 失败: %v\n", dir, err)
			}
		}
	}
}
