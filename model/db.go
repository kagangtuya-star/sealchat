package model

import (
	"log"
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

func DBInit(dsn string) {
	resetSQLiteFTSState()
	var err error
	var dialector gorm.Dialector

	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		dbDriver = "postgres"
		dialector = postgres.Open(dsn)
	} else if strings.HasPrefix(dsn, "mysql://") || strings.Contains(dsn, "@tcp(") {
		dbDriver = "mysql"
		dsn = strings.TrimLeft(dsn, "mysql://")
		dialector = mysql.Open(dsn)
	} else if strings.HasSuffix(dsn, ".db") || strings.HasPrefix(dsn, "file:") || strings.HasPrefix(dsn, ":memory:") {
		dbDriver = "sqlite"
		dialector = sqlite.Open(dsn)
	} else {
		panic("无法识别的数据库类型，请检查DSN格式")
	}

	db, err = gorm.Open(dialector, &gorm.Config{})

	switch dialector.(type) {
	case *sqlite.Dialector: // SQLite 数据库
		dbDriver = "sqlite"
		db.Exec("PRAGMA journal_mode=WAL")
	}

	if err != nil {
		panic("连接数据库失败")
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
	db.AutoMigrate(&GalleryCollection{}, &GalleryItem{})
	db.AutoMigrate(&AudioAsset{}, &AudioFolder{}, &AudioScene{}, &AudioPlaybackState{})

	db.AutoMigrate(&SystemRoleModel{}, &ChannelRoleModel{}, &RolePermissionModel{}, &UserRoleMappingModel{})
	db.AutoMigrate(&FriendModel{}, &FriendRequestModel{})
	db.AutoMigrate(&MessageExportJobModel{})

	if err := BackfillMessageDisplayOrder(); err != nil {
		log.Printf("补齐消息 display_order 失败: %v", err)
	}

	if IsSQLite() {
		go func() {
			if err := ensureSQLiteFTSManager(db); err != nil {
				log.Printf("初始化消息全文索引失败: %v", err)
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
