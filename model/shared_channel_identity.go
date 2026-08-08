package model

import (
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/protocol"
	"sealchat/utils"
)

type SharedChannelIdentityModel struct {
	StringPKBaseModel
	WorldID             string                        `json:"worldId" gorm:"size:100;index"`
	UserID              string                        `json:"userId" gorm:"size:100;not null;index"`
	SourceChannelID     string                        `json:"sourceChannelId" gorm:"size:100"`
	SourceIdentityID    string                        `json:"sourceIdentityId" gorm:"size:100"`
	DisplayName         string                        `json:"displayName"`
	Color               string                        `json:"color"`
	AvatarAttachmentID  string                        `json:"avatarAttachmentId"`
	AvatarDecorations   protocol.AvatarDecorationList `json:"avatarDecorations,omitempty" gorm:"serializer:json;column:avatar_decoration"`
	TheaterPresentation *protocol.TheaterPresentation `json:"theaterPresentation,omitempty" gorm:"serializer:json;column:theater_presentation"`
	// SharedDataJSON is canonical, versioned shared appearance document. Unknown keys survive
	// older server versions so later shared fields can be added without a schema migration.
	SharedDataJSON string `json:"sharedData,omitempty" gorm:"column:shared_data_json;type:text"`
	Revision       int64  `json:"revision" gorm:"not null;default:0"`
}

type SharedChannelIdentityWorldPresentationModel struct {
	StringPKBaseModel
	SharedIdentityID    string                        `json:"sharedIdentityId" gorm:"size:100;not null;uniqueIndex:udx_shared_identity_world_presentation,priority:1"`
	WorldID             string                        `json:"worldId" gorm:"size:100;not null;uniqueIndex:udx_shared_identity_world_presentation,priority:2"`
	SourceChannelID     string                        `json:"sourceChannelId" gorm:"size:100;not null"`
	SourceIdentityID    string                        `json:"sourceIdentityId" gorm:"size:100;not null"`
	TheaterPresentation *protocol.TheaterPresentation `json:"theaterPresentation,omitempty" gorm:"serializer:json;column:theater_presentation"`
	Revision            int64                         `json:"revision" gorm:"not null;default:0"`
}

type SharedChannelIdentitySyncRetryModel struct {
	StringPKBaseModel
	CopyID           string    `json:"copyId" gorm:"size:100;not null;uniqueIndex"`
	SharedIdentityID string    `json:"sharedIdentityId" gorm:"size:100;not null;index"`
	AttemptCount     int       `json:"attemptCount" gorm:"not null;default:0"`
	NextAttemptAt    time.Time `json:"nextAttemptAt" gorm:"not null;index"`
	LastError        string    `json:"lastError" gorm:"size:2048"`
}

func (*SharedChannelIdentityModel) TableName() string {
	return "shared_channel_identities"
}

func (*SharedChannelIdentityWorldPresentationModel) TableName() string {
	return "shared_channel_identity_world_presentations"
}

func (*SharedChannelIdentitySyncRetryModel) TableName() string {
	return "shared_channel_identity_sync_retries"
}

func SharedChannelIdentityGetByID(id string) (*SharedChannelIdentityModel, error) {
	return SharedChannelIdentityGetByIDTx(db, id)
}

func SharedChannelIdentityGetByIDTx(conn *gorm.DB, id string) (*SharedChannelIdentityModel, error) {
	if conn == nil {
		conn = db
	}
	var item SharedChannelIdentityModel
	if err := conn.Where("id = ?", id).Take(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func SharedChannelIdentityCopies(id string) ([]*ChannelIdentityModel, error) {
	var items []*ChannelIdentityModel
	err := db.Where("shared_identity_id = ?", id).
		Order("channel_id ASC, created_at ASC").
		Find(&items).Error
	return items, err
}

// MigrateSharedChannelIdentityWorldScope splits legacy cross-world shared identities.
// It is idempotent and keeps every existing channel identity ID intact.
func MigrateSharedChannelIdentityWorldScope() error {
	var templates []*SharedChannelIdentityModel
	if err := db.Where("world_id = '' OR world_id IS NULL").Find(&templates).Error; err != nil {
		return err
	}
	for _, template := range templates {
		if err := db.Transaction(func(tx *gorm.DB) error {
			var copies []*ChannelIdentityModel
			if err := tx.Where("shared_identity_id = ?", template.ID).Order("created_at ASC").Find(&copies).Error; err != nil {
				return err
			}
			groups := make(map[string][]*ChannelIdentityModel)
			for _, copy := range copies {
				var channel ChannelModel
				if err := tx.Select("id", "world_id").Where("id = ?", copy.ChannelID).Take(&channel).Error; err != nil {
					return err
				}
				if worldID := strings.TrimSpace(channel.WorldID); worldID != "" {
					groups[worldID] = append(groups[worldID], copy)
				}
			}
			first := true
			for worldID, group := range groups {
				source := group[0]
				if first {
					first = false
					if err := tx.Model(&SharedChannelIdentityModel{}).Where("id = ?", template.ID).Updates(map[string]any{
						"world_id": worldID, "source_channel_id": source.ChannelID, "source_identity_id": source.ID,
					}).Error; err != nil {
						return err
					}
					continue
				}
				clone := *template
				clone.StringPKBaseModel = StringPKBaseModel{ID: utils.NewID()}
				clone.WorldID, clone.SourceChannelID, clone.SourceIdentityID = worldID, source.ChannelID, source.ID
				clone.TheaterPresentation = source.TheaterPresentation
				if err := tx.Create(&clone).Error; err != nil {
					return err
				}
				ids := make([]string, 0, len(group))
				for _, copy := range group {
					ids = append(ids, copy.ID)
				}
				if err := tx.Model(&ChannelIdentityModel{}).Where("id IN ?", ids).Update("shared_identity_id", clone.ID).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}
