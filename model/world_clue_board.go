package model

// WorldClueBoardModel stores personal member boards and shared world boards.
// The document is deliberately opaque to the persistence layer; validation and
// protocol semantics live in the board service.
type WorldClueBoardModel struct {
	StringPKBaseModel
	WorldID     string `json:"-" gorm:"size:100;not null;uniqueIndex:idx_world_clue_board_scope,priority:1;index"`
	BoardKey    string `json:"-" gorm:"size:64;not null;uniqueIndex:idx_world_clue_board_scope,priority:2"`
	Scope       string `json:"-" gorm:"size:16;not null;uniqueIndex:idx_world_clue_board_scope,priority:3"`
	OwnerUserID string `json:"-" gorm:"size:100;not null;uniqueIndex:idx_world_clue_board_scope,priority:4;index"`
	Revision    int64  `json:"-" gorm:"not null;default:0"`
	// Keep this untyped (rather than gorm:"type:text") like the Theater
	// state fields. GORM maps it to the dialect's large string type, avoiding
	// MySQL's small VARCHAR default while remaining portable to SQLite/Postgres.
	DocumentJSON string `json:"-" gorm:"not null"`
	UpdatedBy    string `json:"-" gorm:"size:100;not null"`
}

func (*WorldClueBoardModel) TableName() string { return "world_clue_boards" }
