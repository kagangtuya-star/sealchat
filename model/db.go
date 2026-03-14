package model

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	//"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"sealchat/utils"
)

// 注: 所有时间戳使用 time.Now().UnixMilli()

var db *gorm.DB
var dbDriver string
var sqliteFTSReady bool
var sqliteVacuumMu sync.Mutex
var lastDBWriteUnixMilli atomic.Int64
var sqliteDBFilePath atomic.Value

const (
	sqliteAutoVacuumNone        = 0
	sqliteAutoVacuumFull        = 1
	sqliteAutoVacuumIncremental = 2
)

type StringPKBaseModel struct {
	ID        string     `gorm:"primary_key" json:"id"`
	CreatedAt time.Time  `json:"createdAt,omitempty"`
	UpdatedAt time.Time  `json:"updatedAt,omitempty"`
	DeletedAt *time.Time `sql:"index" json:"deletedAt,omitempty"`
}

func (m *StringPKBaseModel) Init() {
	id := utils.NewID()
	m.ID = id
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()
	m.DeletedAt = nil
}

func (m *StringPKBaseModel) GetID() string {
	return m.ID
}

func (m *StringPKBaseModel) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.Init()
	}
	return nil
}

func DBInit(cfg *utils.AppConfig) {
	if cfg == nil {
		panic("配置不可为空")
	}
	dsn := cfg.DSN
	resetSQLiteFTSState()
	resetPostgresFTSState()
	var err error
	var dialector gorm.Dialector
	var isSQLite bool
	sqliteCfg := cfg.SQLite

	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		dbDriver = "postgres"
		sqliteDBFilePath.Store("")
		dialector = postgres.Open(dsn)
	} else if strings.HasPrefix(dsn, "mysql://") || strings.Contains(dsn, "@tcp(") {
		dbDriver = "mysql"
		sqliteDBFilePath.Store("")
		dsn = strings.TrimLeft(dsn, "mysql://")
		dialector = mysql.Open(dsn)
	} else if strings.HasSuffix(dsn, ".db") || strings.HasPrefix(dsn, "file:") || strings.HasPrefix(dsn, ":memory:") {
		sqliteDBFilePath.Store(extractSQLiteFilePath(dsn))
		dsn = ensureSQLiteDSNPath(dsn)
		if sqliteCfg.TxLockImmediate && !strings.Contains(strings.ToLower(dsn), "_txlock=") {
			if strings.Contains(dsn, "?") {
				dsn += "&_txlock=immediate"
			} else {
				dsn += "?_txlock=immediate"
			}
		}
		dbDriver = "sqlite"
		dialector = sqlite.Open(dsn)
		isSQLite = true
	} else {
		panic("无法识别的数据库类型，请检查DSN格式")
	}

	gormCfg := &gorm.Config{}
	if isSQLite {
		gormCfg.SkipDefaultTransaction = true
	}

	db, err = gorm.Open(dialector, gormCfg)
	if err != nil {
		panic("连接数据库失败")
	}
	registerDBWriteActivityCallbacks(db)

	if isSQLite {
		applySQLitePragmas(db, sqliteCfg)
		applySQLiteConnPool(db, sqliteCfg)
		ensureSQLiteAutoVacuum(db)
	}

	if db.Migrator().HasTable(&UserModel{}) {
		_ = UsersDuplicateRemove()
	}

	if db.Migrator().HasTable(&MessageModel{}) {
		// 删除外键约束
		_ = db.Migrator().DropConstraint(&MessageModel{}, "fk_messages_quote")
	}

	db.AutoMigrate(&ChannelModel{})
	db.AutoMigrate(&GuildModel{})
	db.AutoMigrate(&MessageModel{})
	db.AutoMigrate(&MessageWhisperRecipientModel{})
	db.AutoMigrate(&MessageDiceRollModel{})
	db.AutoMigrate(&MessageEditHistoryModel{})
	db.AutoMigrate(&MessageArchiveLogModel{})
	db.AutoMigrate(&MessageReactionModel{}, &MessageReactionCountModel{})
	db.AutoMigrate(&UserModel{})
	db.AutoMigrate(&AccessTokenModel{})
	db.AutoMigrate(&MemberModel{})
	db.AutoMigrate(&AttachmentModel{})
	db.AutoMigrate(&ChannelAttachmentImageLayoutModel{})
	db.AutoMigrate(&MentionModel{})
	db.AutoMigrate(&TimelineModel{})
	db.AutoMigrate(&TimelineUserLastRecordModel{})
	db.AutoMigrate(&UserEmojiModel{})
	db.AutoMigrate(&BotTokenModel{})
	db.AutoMigrate(&ChannelLatestReadModel{})
	db.AutoMigrate(&ChannelIdentityModel{})
	db.AutoMigrate(&CharacterCardModel{})
	db.AutoMigrate(&CharacterCardTemplateModel{})
	db.AutoMigrate(&CharacterCardTemplateBindingModel{})
	db.AutoMigrate(&ChannelIdentityFolderModel{}, &ChannelIdentityFolderMemberModel{}, &ChannelIdentityFolderFavoriteModel{})
	db.AutoMigrate(&GalleryCollection{}, &GalleryItem{})
	db.AutoMigrate(&AudioAsset{}, &AudioFolder{}, &AudioScene{}, &AudioPlaybackState{})
	db.AutoMigrate(&DiceMacroModel{})

	db.AutoMigrate(&SystemRoleModel{}, &ChannelRoleModel{}, &RolePermissionModel{}, &UserRoleMappingModel{})
	db.AutoMigrate(&FriendModel{}, &FriendRequestModel{})
	db.AutoMigrate(&MessageExportJobModel{})
	db.AutoMigrate(&ChannelIFormModel{})
	db.AutoMigrate(&WorldModel{}, &WorldMemberModel{}, &WorldInviteModel{}, &WorldFavoriteModel{}, &WorldKeywordModel{}, &WorldKeywordCategoryModel{})
	db.AutoMigrate(&AnnouncementModel{}, &AnnouncementUserStateModel{})
	db.AutoMigrate(&ServiceMetricSample{})
	db.AutoMigrate(&ChatImportJobModel{})
	db.AutoMigrate(&ChannelWebhookIntegrationModel{}, &MessageExternalRefModel{}, &WebhookEventLogModel{}, &WebhookIdentityBindingModel{})
	db.AutoMigrate(&StickyNoteModel{}, &StickyNoteUserStateModel{}, &StickyNoteFolderModel{})
	db.AutoMigrate(&EmailNotificationSettingsModel{}, &EmailNotificationLogModel{})
	db.AutoMigrate(&EmailVerificationCodeModel{})
	db.AutoMigrate(&UpdateCheckState{})
	db.AutoMigrate(&ConfigCurrentModel{}, &ConfigHistoryModel{})
	db.AutoMigrate(&UserPreferenceModel{})
	db.AutoMigrate(&ExportColorProfileModel{})

	if err := db.Model(&ChannelModel{}).
		Where("default_dice_expr = '' OR default_dice_expr IS NULL").
		Update("default_dice_expr", "d20").Error; err != nil {
		log.Printf("初始化频道默认骰失败: %v", err)
	}

	if err := BackfillMessageDisplayOrder(); err != nil {
		log.Printf("补齐消息 display_order 失败: %v", err)
	}

	if err := BackfillChannelRecentSentAt(); err != nil {
		log.Printf("回填频道最近发言时间失败: %v", err)
	}

	if err := BackfillWorldData(); err != nil {
		log.Printf("初始化世界数据失败: %v", err)
	}

	if IsSQLite() {
		go func() {
			if err := ensureSQLiteFTSManager(db); err != nil {
				log.Printf("初始化消息全文索引失败: %v", err)
			}
		}()
	}
	if IsPostgres() {
		go func() {
			if err := ensurePostgresFTSManager(db); err != nil {
				log.Printf("初始化 Postgres FTS 失败: %v", err)
			}
		}()
	}
}

func GetDB() *gorm.DB {
	return db
}

// DBInitMinimal 仅初始化数据库连接（用于配置恢复场景）
// 只执行连接和配置表迁移，不执行完整迁移
func DBInitMinimal(dsn string) error {
	if db != nil {
		return nil // 已初始化
	}

	resetSQLiteFTSState()
	resetPostgresFTSState()

	var err error
	var dialector gorm.Dialector
	var isSQLite bool

	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		dbDriver = "postgres"
		sqliteDBFilePath.Store("")
		dialector = postgres.Open(dsn)
	} else if strings.HasPrefix(dsn, "mysql://") || strings.Contains(dsn, "@tcp(") {
		dbDriver = "mysql"
		sqliteDBFilePath.Store("")
		dsn = strings.TrimLeft(dsn, "mysql://")
		dialector = mysql.Open(dsn)
	} else {
		sqliteDBFilePath.Store(extractSQLiteFilePath(dsn))
		dsn = ensureSQLiteDSNPath(dsn)
		if !strings.Contains(strings.ToLower(dsn), "_txlock=") {
			if strings.Contains(dsn, "?") {
				dsn += "&_txlock=immediate"
			} else {
				dsn += "?_txlock=immediate"
			}
		}
		dbDriver = "sqlite"
		dialector = sqlite.Open(dsn)
		isSQLite = true
	}

	gormCfg := &gorm.Config{}
	if isSQLite {
		gormCfg.SkipDefaultTransaction = true
	}

	db, err = gorm.Open(dialector, gormCfg)
	if err != nil {
		return err
	}
	registerDBWriteActivityCallbacks(db)

	if isSQLite {
		applySQLitePragmas(db, utils.SQLiteConfig{
			EnableWAL:     true,
			BusyTimeoutMS: 10000,
			CacheSizeKB:   512000,
			Synchronous:   "NORMAL",
		})
		ensureSQLiteAutoVacuum(db)
	}

	// 仅迁移配置表
	if err := db.AutoMigrate(&ConfigCurrentModel{}, &ConfigHistoryModel{}); err != nil {
		return err
	}

	return nil
}

func DBDriver() string {
	return dbDriver
}

func IsSQLite() bool {
	return strings.EqualFold(dbDriver, "sqlite")
}

func IsPostgres() bool {
	return strings.EqualFold(dbDriver, "postgres")
}

func SQLiteFTSReady() bool {
	return sqliteFTSReady
}

func FlushWAL() {
	if db == nil {
		return
	}
	switch db.Dialector.(type) {
	case *sqlite.Dialector: // SQLite 数据库，进行落盘
	default:
		return
	}

	_ = db.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
	_ = db.Exec("PRAGMA shrink_memory")
}

// VacuumSQLite 执行 SQLite VACUUM，通常用于空闲期空间整理。
func VacuumSQLite() error {
	if db == nil || !IsSQLite() {
		return nil
	}
	sqliteVacuumMu.Lock()
	defer sqliteVacuumMu.Unlock()
	return db.Exec("VACUUM").Error
}

// SQLiteFileSizeBytes 返回 SQLite 数据文件大小（字节）。
func SQLiteFileSizeBytes() (int64, error) {
	if !IsSQLite() {
		return 0, fmt.Errorf("当前数据库不是 SQLite")
	}
	path := SQLiteDBFilePath()
	if path == "" {
		return 0, fmt.Errorf("当前 SQLite DSN 非文件路径，无法探测大小")
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func SQLiteDBFilePath() string {
	value := sqliteDBFilePath.Load()
	path, _ := value.(string)
	return path
}

// HasRecentSQLiteWriteActivity 返回在给定窗口内是否检测到写入迹象。
func HasRecentSQLiteWriteActivity(window time.Duration) bool {
	if !IsSQLite() || window <= 0 {
		return false
	}
	last := lastDBWriteUnixMilli.Load()
	if last <= 0 {
		return false
	}
	return time.Since(time.UnixMilli(last)) < window
}

func applySQLitePragmas(conn *gorm.DB, cfg utils.SQLiteConfig) {
	if conn == nil {
		return
	}
	if cfg.EnableWAL {
		conn.Exec("PRAGMA journal_mode=WAL")
	}
	if cfg.BusyTimeoutMS > 0 {
		conn.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d", cfg.BusyTimeoutMS))
	}
	if cfg.CacheSizeKB != 0 {
		size := cfg.CacheSizeKB
		if size < 0 {
			size = -size
		}
		conn.Exec(fmt.Sprintf("PRAGMA cache_size = -%d", size))
	}
	conn.Exec("PRAGMA temp_store = memory")
	if cfg.Synchronous != "" {
		conn.Exec(fmt.Sprintf("PRAGMA synchronous = %s", strings.ToUpper(cfg.Synchronous)))
	}
	if cfg.OptimizeOnInit {
		conn.Exec("PRAGMA optimize")
	}
}

func applySQLiteConnPool(conn *gorm.DB, cfg utils.SQLiteConfig) {
	if conn == nil {
		return
	}
	sqlDB, err := conn.DB()
	if err != nil {
		log.Printf("获取 SQLite 底层连接池失败: %v", err)
		return
	}
	readConns := cfg.ReadConnections
	if readConns <= 0 {
		readConns = 1
	}
	sqlDB.SetMaxOpenConns(readConns)
	sqlDB.SetMaxIdleConns(readConns)
	sqlDB.SetConnMaxIdleTime(0)
	sqlDB.SetConnMaxLifetime(0)
}

func ensureSQLiteAutoVacuum(conn *gorm.DB) {
	if conn == nil {
		return
	}
	mode, err := querySQLiteAutoVacuumMode(conn)
	if err != nil {
		log.Printf("SQLite auto_vacuum 检查失败: %v", err)
		return
	}
	if mode == sqliteAutoVacuumFull {
		return
	}

	log.Printf("SQLite auto_vacuum 当前模式=%d，尝试切换为 FULL", mode)
	if err := conn.Exec("PRAGMA auto_vacuum = FULL").Error; err != nil {
		log.Printf("SQLite 启用 auto_vacuum 失败: %v", err)
		return
	}

	sqliteVacuumMu.Lock()
	vacuumErr := conn.Exec("VACUUM").Error
	sqliteVacuumMu.Unlock()
	if vacuumErr != nil {
		log.Printf("SQLite 激活 auto_vacuum 失败（VACUUM）: %v", vacuumErr)
		return
	}

	mode, err = querySQLiteAutoVacuumMode(conn)
	if err != nil {
		log.Printf("SQLite auto_vacuum 复检失败: %v", err)
		return
	}
	if mode != sqliteAutoVacuumFull {
		log.Printf("SQLite auto_vacuum 复检未生效，当前模式=%d", mode)
		return
	}
	log.Printf("SQLite auto_vacuum 已启用 (FULL)")
}

func querySQLiteAutoVacuumMode(conn *gorm.DB) (int, error) {
	var mode int
	row := conn.Raw("PRAGMA auto_vacuum").Row()
	if row == nil {
		return sqliteAutoVacuumNone, fmt.Errorf("query auto_vacuum returned nil row")
	}
	if err := row.Scan(&mode); err != nil {
		return sqliteAutoVacuumNone, err
	}
	if mode != sqliteAutoVacuumNone && mode != sqliteAutoVacuumFull && mode != sqliteAutoVacuumIncremental {
		return sqliteAutoVacuumNone, fmt.Errorf("unknown auto_vacuum mode: %d", mode)
	}
	return mode, nil
}

func registerDBWriteActivityCallbacks(conn *gorm.DB) {
	if conn == nil {
		return
	}
	register := func(name string, getFn func(string) func(*gorm.DB), registerFn func() error) {
		if getFn(name) != nil {
			return
		}
		if err := registerFn(); err != nil {
			log.Printf("注册数据库写活动回调失败(%s): %v", name, err)
		}
	}
	register("sealchat:write-activity:create", conn.Callback().Create().Get, func() error {
		return conn.Callback().Create().Before("gorm:create").Register("sealchat:write-activity:create", func(tx *gorm.DB) {
			lastDBWriteUnixMilli.Store(time.Now().UnixMilli())
		})
	})
	register("sealchat:write-activity:update", conn.Callback().Update().Get, func() error {
		return conn.Callback().Update().Before("gorm:update").Register("sealchat:write-activity:update", func(tx *gorm.DB) {
			lastDBWriteUnixMilli.Store(time.Now().UnixMilli())
		})
	})
	register("sealchat:write-activity:delete", conn.Callback().Delete().Get, func() error {
		return conn.Callback().Delete().Before("gorm:delete").Register("sealchat:write-activity:delete", func(tx *gorm.DB) {
			lastDBWriteUnixMilli.Store(time.Now().UnixMilli())
		})
	})
}

// ensureSQLiteDSNPath 确保 sqlite DSN 指向文件路径时存在目录
func ensureSQLiteDSNPath(dsn string) string {
	if strings.HasPrefix(dsn, "file:") || strings.HasPrefix(dsn, ":memory:") {
		return dsn
	}
	base := dsn
	if idx := strings.Index(dsn, "?"); idx >= 0 {
		base = dsn[:idx]
	}
	dir := filepath.Dir(base)
	if dir != "." && dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	return dsn
}

func extractSQLiteFilePath(dsn string) string {
	raw := strings.TrimSpace(dsn)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	if lower == ":memory:" || strings.HasPrefix(lower, "file::memory:") {
		return ""
	}
	if idx := strings.Index(lower, "mode=memory"); idx >= 0 {
		return ""
	}

	if strings.HasPrefix(lower, "file:") {
		path := raw[len("file:"):]
		if idx := strings.Index(path, "?"); idx >= 0 {
			path = path[:idx]
		}
		path = strings.TrimSpace(path)
		if path == "" || strings.EqualFold(path, ":memory:") {
			return ""
		}
		if strings.HasPrefix(path, "//") {
			path = strings.TrimPrefix(path, "//")
			if path == "" {
				return ""
			}
			if !strings.HasPrefix(path, "/") {
				if slash := strings.Index(path, "/"); slash >= 0 {
					path = path[slash:]
				}
			}
		}
		if path == "" || strings.EqualFold(path, ":memory:") {
			return ""
		}
		return filepath.Clean(path)
	}

	path := raw
	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}
	path = strings.TrimSpace(path)
	if path == "" || strings.EqualFold(path, ":memory:") {
		return ""
	}
	return filepath.Clean(path)
}
