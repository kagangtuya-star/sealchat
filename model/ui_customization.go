package model

// UICustomizationPackageModel is shared by platform and world packages. Code is
// deliberately separate from AppConfig and WorldModel. JS is only returned by
// the runtime resolver after server-side policy filtering.
type UICustomizationPackageModel struct {
	StringPKBaseModel
	ScopeType   string `gorm:"size:16;not null;index:idx_ui_customization_scope,priority:1"`
	ScopeID     string `gorm:"size:100;not null;default:'';index:idx_ui_customization_scope,priority:2"`
	Name        string `gorm:"size:100;not null"`
	Description string `gorm:"size:500"`
	Enabled     bool   `gorm:"not null"`
	SortOrder   int    `gorm:"not null;default:0;index:idx_ui_customization_scope,priority:3"`
	// BuiltinKey is a stable builtin:<key> reference. Builtin package source
	// stays in the shipped resource directory; only enablement/order is mutable.
	BuiltinKey              string `gorm:"size:100;index"`
	BuiltinVersion          string `gorm:"size:80"`
	SurfacesJSON            string `gorm:"type:text;not null"`
	ChannelCapabilitiesJSON string `gorm:"type:text"`
	// Size lets GORM choose mediumtext on MySQL (TEXT only holds 64 KiB),
	// while retaining portable string columns on PostgreSQL and SQLite.
	CSS       string `gorm:"size:262144"`
	JS        string `gorm:"size:262144" json:"-"`
	CreatedBy string `gorm:"size:100;not null"`
	UpdatedBy string `gorm:"size:100;not null"`
}

func (UICustomizationPackageModel) TableName() string { return "ui_customization_packages" }

// WorldUICustomizationPlatformRefModel links a world to a platform-owned
// customization package without copying package content.
type WorldUICustomizationPlatformRefModel struct {
	WorldID           string `gorm:"size:100;not null;uniqueIndex:idx_world_ui_platform_ref,priority:1;index"`
	PlatformPackageID string `gorm:"size:100;not null;uniqueIndex:idx_world_ui_platform_ref,priority:2;index"`
}

func (WorldUICustomizationPlatformRefModel) TableName() string {
	return "world_ui_customization_platform_refs"
}
