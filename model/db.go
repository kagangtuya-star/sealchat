package model

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
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
		dialector = postgres.Open(dsn)
	} else if strings.HasPrefix(dsn, "mysql://") || strings.Contains(dsn, "@tcp(") {
		dbDriver = "mysql"
		dsn = strings.TrimLeft(dsn, "mysql://")
		dialector = mysql.Open(dsn)
	} else if strings.HasSuffix(dsn, ".db") || strings.HasPrefix(dsn, "file:") || strings.HasPrefix(dsn, ":memory:") {
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

	if isSQLite {
		applySQLitePragmas(db, sqliteCfg)
		applySQLiteConnPool(db, sqliteCfg)
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
	db.AutoMigrate(&MessageDiceRollModel{})
	db.AutoMigrate(&MessageEditHistoryModel{})
	db.AutoMigrate(&MessageArchiveLogModel{})
	db.AutoMigrate(&UserModel{})
	db.AutoMigrate(&AccessTokenModel{})
	db.AutoMigrate(&MemberModel{})
	db.AutoMigrate(&AttachmentModel{})
	db.AutoMigrate(&MentionModel{})
	db.AutoMigrate(&TimelineModel{})
	db.AutoMigrate(&TimelineUserLastRecordModel{})
	db.AutoMigrate(&UserEmojiModel{})
	db.AutoMigrate(&BotTokenModel{})
	db.AutoMigrate(&ChannelLatestReadModel{})
	db.AutoMigrate(&ChannelIdentityModel{})
	db.AutoMigrate(&ChannelIdentityFolderModel{}, &ChannelIdentityFolderMemberModel{}, &ChannelIdentityFolderFavoriteModel{})
	db.AutoMigrate(&GalleryCollection{}, &GalleryItem{})
	db.AutoMigrate(&AudioAsset{}, &AudioFolder{}, &AudioScene{}, &AudioPlaybackState{})
	db.AutoMigrate(&DiceMacroModel{})

	db.AutoMigrate(&SystemRoleModel{}, &ChannelRoleModel{}, &RolePermissionModel{}, &UserRoleMappingModel{})
	db.AutoMigrate(&FriendModel{}, &FriendRequestModel{})
	db.AutoMigrate(&MessageExportJobModel{})
	db.AutoMigrate(&ChannelIFormModel{})
	db.AutoMigrate(&WorldModel{}, &WorldMemberModel{}, &WorldInviteModel{}, &WorldFavoriteModel{}, &WorldKeywordModel{})
	db.AutoMigrate(&ServiceMetricSample{})

	if err := db.Model(&ChannelModel{}).
		Where("default_dice_expr = '' OR default_dice_expr IS NULL").
		Update("default_dice_expr", "d20").Error; err != nil {
		log.Printf("初始化频道默认骰失败: %v", err)
	}

	if err := BackfillMessageDisplayOrder(); err != nil {
		log.Printf("补齐消息 display_order 失败: %v", err)
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
	switch db.Dialector.(type) {
	case *sqlite.Dialector: // SQLite 数据库，进行落盘
	default:
		return
	}

	_ = db.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
	_ = db.Exec("PRAGMA shrink_memory")
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
