package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/utils"
)

var (
	ErrWorldClueNotFound    = errors.New("world clue not found")
	ErrWorldClueDenied      = errors.New("world clue permission denied")
	ErrWorldClueInvalid     = errors.New("world clue invalid")
	ErrWorldClueConflict    = errors.New("world clue conflict")
	ErrWorldClueFolderCycle = errors.New("world clue folder cycle")
)

type WorldClueCreateInput struct {
	Title             string
	Kind              string
	ContentFormat     string
	Content           string
	ImageAttachmentID string
	ImageURL          string
	EmbedURL          string
	Presentation      *protocol.WorldCluePresentation
	SharedFolderID    string
	DefaultAccess     string
	OrderIndex        int
	ManagerNoteFormat string
	ManagerNote       string
}

type WorldClueUpdateInput struct {
	ExpectedRevision  int64
	Title             *string
	Kind              *string
	ContentFormat     *string
	Content           *string
	ImageAttachmentID *string
	ImageURL          *string
	EmbedURL          *string
	Presentation      *protocol.WorldCluePresentation
	SharedFolderID    *string
	DefaultAccess     *string
	OrderIndex        *int
	ManagerNoteFormat *string
	ManagerNote       *string
}

type WorldCluePublishResult struct {
	Clue         *protocol.WorldClueDetail
	RecipientIDs []string
}

type WorldClueTheaterTarget struct {
	UserID string
	Access string
}

type WorldClueTheaterEntryResult struct {
	ClueID       string
	PublishSeq   int64
	RecipientIDs []string
	Revision     int64
	Status       string
}

type WorldClueUserStateInput struct {
	PersonalFolderID *string
	PersonalOrder    *int
	Favorite         *bool
}

type WorldClueFolderInput struct {
	Scope      string
	ParentID   string
	Name       string
	Color      string
	OrderIndex int
}

type WorldClueFolderUpdateInput struct {
	ParentID   *string
	Name       *string
	Color      *string
	OrderIndex *int
}

type WorldClueReorderInput struct {
	Scope      string
	FolderID   string
	OrderedIDs []string
}

func defaultWorldCluePresentation() protocol.WorldCluePresentation {
	return protocol.WorldCluePresentation{
		Version: 1, MediaPlacement: "left", MediaRatio: 0.42, ObjectFit: "contain", MediaType: "image",
		EnterAnimation: "fade-scale", ExitAnimation: "fade", AnimationDurationMS: 280,
		TitleColor:             "#ffffff",
		BackgroundColorEnabled: false, BackgroundColor: "#000000", BackgroundColorOpacity: 45,
		BackgroundMediaEnabled: false, BackgroundMediaAttachmentID: "", BackgroundMediaURL: "", BackgroundMediaType: "image", BackgroundMediaMode: "cover",
		BackgroundMediaOpacity: 100, BackgroundMediaBlur: 0, BackgroundMediaBrightness: 100,
	}
}

func ValidateWorldCluePresentation(value *protocol.WorldCluePresentation) (protocol.WorldCluePresentation, error) {
	if value == nil {
		return defaultWorldCluePresentation(), nil
	}
	result := *value
	if result.Version == 0 {
		result.Version = 1
	}
	if result.Version != 1 {
		return result, fmt.Errorf("%w: unsupported presentation version", ErrWorldClueInvalid)
	}
	if !oneOf(result.MediaPlacement, "left", "right", "top", "bottom") {
		return result, fmt.Errorf("%w: invalid media placement", ErrWorldClueInvalid)
	}
	if !oneOf(result.ObjectFit, "contain", "cover") {
		return result, fmt.Errorf("%w: invalid object fit", ErrWorldClueInvalid)
	}
	if result.MediaType == "" {
		result.MediaType = "image"
	}
	if !oneOf(result.MediaType, "image", "video") {
		return result, fmt.Errorf("%w: invalid media type", ErrWorldClueInvalid)
	}
	if !oneOf(result.EnterAnimation, "none", "fade", "fade-up", "scale", "fade-scale", "slide-left", "slide-right") ||
		!oneOf(result.ExitAnimation, "none", "fade", "fade-up", "scale", "fade-scale", "slide-left", "slide-right") {
		return result, fmt.Errorf("%w: invalid animation", ErrWorldClueInvalid)
	}
	if result.AnimationDurationMS < 0 || result.AnimationDurationMS > 1000 {
		return result, fmt.Errorf("%w: invalid animation duration", ErrWorldClueInvalid)
	}
	if result.MediaRatio < 0.2 {
		result.MediaRatio = 0.2
	}
	if result.MediaRatio > 0.8 {
		result.MediaRatio = 0.8
	}
	result.TitleColor = strings.TrimSpace(result.TitleColor)
	if result.TitleColor == "" {
		result.TitleColor = "#ffffff"
	}
	result.BackgroundColor = strings.TrimSpace(result.BackgroundColor)
	if result.BackgroundColor == "" {
		result.BackgroundColor = "#000000"
	}
	result.BackgroundMediaMode = strings.TrimSpace(result.BackgroundMediaMode)
	if result.BackgroundMediaMode == "" {
		result.BackgroundMediaMode = "cover"
	}
	if !oneOf(result.BackgroundMediaMode, "cover", "contain", "tile", "center") {
		return result, fmt.Errorf("%w: invalid background media mode", ErrWorldClueInvalid)
	}
	if result.BackgroundColorOpacity < 0 || result.BackgroundColorOpacity > 100 {
		return result, fmt.Errorf("%w: invalid background color opacity", ErrWorldClueInvalid)
	}
	if result.BackgroundMediaOpacity < 0 || result.BackgroundMediaOpacity > 100 {
		return result, fmt.Errorf("%w: invalid background media opacity", ErrWorldClueInvalid)
	}
	if result.BackgroundMediaBlur < 0 || result.BackgroundMediaBlur > 20 {
		return result, fmt.Errorf("%w: invalid background media blur", ErrWorldClueInvalid)
	}
	if result.BackgroundMediaBrightness < 50 || result.BackgroundMediaBrightness > 150 {
		return result, fmt.Errorf("%w: invalid background media brightness", ErrWorldClueInvalid)
	}
	result.BackgroundMediaAttachmentID = strings.TrimPrefix(strings.TrimSpace(result.BackgroundMediaAttachmentID), "id:")
	if result.BackgroundMediaType == "" {
		result.BackgroundMediaType = "image"
	}
	if !oneOf(result.BackgroundMediaType, "image", "video") {
		return result, fmt.Errorf("%w: invalid background media type", ErrWorldClueInvalid)
	}
	var err error
	result.BackgroundMediaURL, err = validateHTTPURL(result.BackgroundMediaURL, result.BackgroundMediaEnabled && result.BackgroundMediaAttachmentID == "")
	if err != nil {
		return result, err
	}
	return result, nil
}

func oneOf(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}

func validateHTTPURL(value string, required bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			return "", fmt.Errorf("%w: URL is required", ErrWorldClueInvalid)
		}
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("%w: only http/https URLs are allowed", ErrWorldClueInvalid)
	}
	return parsed.String(), nil
}

func normalizeWorldClueContent(format, content string) (string, string, string, error) {
	format = strings.TrimSpace(format)
	if format == "" {
		format = model.WorldClueContentPlain
	}
	if !oneOf(format, model.WorldClueContentPlain, model.WorldClueContentTipTap) {
		return "", "", "", fmt.Errorf("%w: invalid content format", ErrWorldClueInvalid)
	}
	if format == model.WorldClueContentTipTap {
		text, ok := SerializeTipTapContentToCommandText(content)
		if !ok {
			return "", "", "", fmt.Errorf("%w: invalid TipTap content", ErrWorldClueInvalid)
		}
		return format, content, strings.TrimSpace(text), nil
	}
	return format, content, strings.TrimSpace(content), nil
}

func normalizeWorldClueKind(kind, imageAttachmentID, imageURL, embedURL string) (string, string, string, string, error) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = model.WorldClueKindText
	}
	if !oneOf(kind, model.WorldClueKindText, model.WorldClueKindImage, model.WorldClueKindIframe) {
		return "", "", "", "", fmt.Errorf("%w: invalid clue kind", ErrWorldClueInvalid)
	}
	imageAttachmentID = strings.TrimPrefix(strings.TrimSpace(imageAttachmentID), "id:")
	if imageAttachmentID != "" && strings.TrimSpace(imageURL) != "" {
		return "", "", "", "", fmt.Errorf("%w: image attachment and image URL are mutually exclusive", ErrWorldClueInvalid)
	}
	var err error
	imageURL, err = validateHTTPURL(imageURL, false)
	if err != nil {
		return "", "", "", "", err
	}
	embedURL, err = validateHTTPURL(embedURL, kind == model.WorldClueKindIframe)
	if err != nil {
		return "", "", "", "", err
	}
	if kind != model.WorldClueKindIframe {
		embedURL = ""
	}
	return kind, imageAttachmentID, imageURL, embedURL, nil
}

func worldClueRole(tx *gorm.DB, worldID, userID string) (string, error) {
	if strings.TrimSpace(worldID) == "" || strings.TrimSpace(userID) == "" {
		return "", nil
	}
	var member model.WorldMemberModel
	if err := tx.Where("world_id = ? AND user_id = ?", worldID, userID).Limit(1).Find(&member).Error; err != nil {
		return "", err
	}
	return member.Role, nil
}

func worldClueIsAdminRole(role string) bool {
	return role == model.WorldRoleOwner || role == model.WorldRoleAdmin
}

func effectiveWorldClueAccess(role, defaultAccess, override string) string {
	if worldClueIsAdminRole(role) {
		return model.WorldClueAccessEdit
	}
	if role == "" {
		return model.WorldClueAccessNone
	}
	access := override
	if access == "" || access == model.WorldClueAccessInherit {
		access = defaultAccess
	}
	if !oneOf(access, model.WorldClueAccessNone, model.WorldClueAccessView, model.WorldClueAccessEdit) {
		access = model.WorldClueAccessNone
	}
	if role == model.WorldRoleSpectator && access == model.WorldClueAccessEdit {
		return model.WorldClueAccessView
	}
	return access
}

func worldCluePrivateExcerpt(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > 160 {
		runes = runes[:160]
	}
	return string(runes)
}

func worldClueAccessDTO(userID, role, defaultAccess, override string, row *model.WorldClueAccessModel) protocol.WorldClueAccess {
	if override == "" {
		override = model.WorldClueAccessInherit
	}
	item := protocol.WorldClueAccess{
		UserID: userID, Role: role, AccessOverride: override,
		EffectiveAccess: effectiveWorldClueAccess(role, defaultAccess, override),
	}
	if row != nil {
		item.PrivateRevision = row.PrivateRevision
		item.PrivateExcerpt = worldCluePrivateExcerpt(row.PrivateContentText)
		item.HasPrivateContent = strings.TrimSpace(row.PrivateContent) != ""
	}
	return item
}

func WorldClueRosterList(worldID, actorID string) ([]protocol.WorldClueRosterMember, error) {
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return nil, err
	}
	if !worldClueIsAdminRole(role) {
		return nil, ErrWorldClueDenied
	}
	var rows []struct {
		UserID     string
		Role       string
		Username   string
		Nickname   string
		Avatar     string
		OrderIndex int
	}
	err = db.Model(&model.WorldClueRosterMemberModel{}).
		Select("world_clue_roster_members.user_id, wm.role, u.username, u.nickname, u.avatar, world_clue_roster_members.order_index").
		Joins("JOIN world_members wm ON wm.world_id = world_clue_roster_members.world_id AND wm.user_id = world_clue_roster_members.user_id AND wm.deleted_at IS NULL").
		Joins("LEFT JOIN users u ON u.id = world_clue_roster_members.user_id AND u.deleted_at IS NULL").
		Where("world_clue_roster_members.world_id = ?", worldID).
		Order("world_clue_roster_members.order_index ASC, world_clue_roster_members.created_at ASC, world_clue_roster_members.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	items := make([]protocol.WorldClueRosterMember, 0, len(rows))
	for _, row := range rows {
		items = append(items, protocol.WorldClueRosterMember{UserID: row.UserID, Role: row.Role, Username: row.Username, Nickname: row.Nickname, Avatar: row.Avatar, OrderIndex: row.OrderIndex})
	}
	return items, nil
}

func WorldClueRosterAdd(worldID, actorID, targetUserID string) (*protocol.WorldClueRosterMember, error) {
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return nil, err
	}
	if !worldClueIsAdminRole(role) {
		return nil, ErrWorldClueDenied
	}
	if targetRole, err := worldClueRole(db, worldID, targetUserID); err != nil {
		return nil, err
	} else if targetRole == "" {
		return nil, fmt.Errorf("%w: target is not a world member", ErrWorldClueInvalid)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var existing model.WorldClueRosterMemberModel
		if err := tx.Where("world_id = ? AND user_id = ?", worldID, targetUserID).Limit(1).Find(&existing).Error; err != nil {
			return err
		}
		if existing.ID != "" {
			return nil
		}
		var maxOrder int
		if err := tx.Model(&model.WorldClueRosterMemberModel{}).Where("world_id = ?", worldID).Select("COALESCE(MAX(order_index), -1)").Scan(&maxOrder).Error; err != nil {
			return err
		}
		row := model.WorldClueRosterMemberModel{WorldID: worldID, UserID: targetUserID, OrderIndex: maxOrder + 1}
		row.ID = utils.NewID()
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "world_id"}, {Name: "user_id"}}, DoNothing: true}).Create(&row).Error
	})
	if err != nil {
		return nil, err
	}
	items, err := WorldClueRosterList(worldID, actorID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].UserID == targetUserID {
			return &items[i], nil
		}
	}
	return nil, ErrWorldClueNotFound
}

func WorldClueRosterRemove(worldID, actorID, targetUserID string) error {
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return err
	}
	if !worldClueIsAdminRole(role) {
		return ErrWorldClueDenied
	}
	return db.Unscoped().Where("world_id = ? AND user_id = ?", worldID, targetUserID).Delete(&model.WorldClueRosterMemberModel{}).Error
}

func canViewWorldClue(clue *model.WorldClueModel, role, access string) bool {
	if clue == nil || clue.Status == model.WorldClueStatusArchived {
		return false
	}
	if worldClueIsAdminRole(role) {
		return true
	}
	if access == model.WorldClueAccessEdit {
		return true
	}
	return clue.Status == model.WorldClueStatusPublished && access == model.WorldClueAccessView
}

func getWorldClueAccessRow(tx *gorm.DB, worldID, clueID, userID string) (*model.WorldClueAccessModel, error) {
	var row model.WorldClueAccessModel
	if err := tx.Where("world_id = ? AND clue_id = ? AND user_id = ?", worldID, clueID, userID).Limit(1).Find(&row).Error; err != nil {
		return nil, err
	}
	if row.ID == "" {
		return nil, nil
	}
	return &row, nil
}

func getWorldClueState(tx *gorm.DB, worldID, clueID, userID string) (*model.WorldClueUserStateModel, error) {
	var row model.WorldClueUserStateModel
	if err := tx.Where("world_id = ? AND clue_id = ? AND user_id = ?", worldID, clueID, userID).Limit(1).Find(&row).Error; err != nil {
		return nil, err
	}
	if row.ID == "" {
		return nil, nil
	}
	return &row, nil
}

func validateWorldClueAttachment(tx *gorm.DB, worldID, actorID, attachmentID, mediaType string) error {
	if attachmentID == "" {
		return nil
	}
	var attachment model.AttachmentModel
	if err := tx.Where("id = ?", attachmentID).Limit(1).Find(&attachment).Error; err != nil {
		return err
	}
	mimeType := strings.ToLower(strings.TrimSpace(attachment.MimeType))
	if attachment.ID == "" || (!strings.HasPrefix(mimeType, "image/") && !strings.HasPrefix(mimeType, "video/")) {
		return fmt.Errorf("%w: media attachment is invalid", ErrWorldClueInvalid)
	}
	if (mediaType == "video") != strings.HasPrefix(mimeType, "video/") {
		return fmt.Errorf("%w: media attachment type does not match", ErrWorldClueInvalid)
	}
	if attachment.RootID != worldID || attachment.RootIDType != "world_clue" {
		return fmt.Errorf("%w: media attachment is not owned by this world", ErrWorldClueDenied)
	}
	if attachment.UserID != actorID {
		role, err := worldClueRole(tx, worldID, actorID)
		if err != nil {
			return err
		}
		if !worldClueIsAdminRole(role) {
			return fmt.Errorf("%w: media attachment belongs to another user", ErrWorldClueDenied)
		}
	}
	return nil
}

func validateWorldClueBackgroundAttachment(tx *gorm.DB, worldID, actorID, attachmentID, mediaType string) error {
	if attachmentID == "" {
		return nil
	}
	var attachment model.AttachmentModel
	if err := tx.Where("id = ?", attachmentID).Limit(1).Find(&attachment).Error; err != nil {
		return err
	}
	mimeType := strings.ToLower(strings.TrimSpace(attachment.MimeType))
	if attachment.ID == "" || (!strings.HasPrefix(mimeType, "image/") && !strings.HasPrefix(mimeType, "video/")) {
		return fmt.Errorf("%w: background media attachment is invalid", ErrWorldClueInvalid)
	}
	if (mediaType == "video") != strings.HasPrefix(mimeType, "video/") {
		return fmt.Errorf("%w: background media attachment type does not match", ErrWorldClueInvalid)
	}
	if attachment.RootID != worldID || attachment.RootIDType != "world_clue" {
		return fmt.Errorf("%w: background media attachment is not owned by this world", ErrWorldClueDenied)
	}
	if attachment.UserID != actorID {
		role, err := worldClueRole(tx, worldID, actorID)
		if err != nil {
			return err
		}
		if !worldClueIsAdminRole(role) {
			return fmt.Errorf("%w: background media attachment belongs to another user", ErrWorldClueDenied)
		}
	}
	return nil
}

func confirmWorldClueAttachment(tx *gorm.DB, worldID, attachmentID string) error {
	if attachmentID == "" {
		return nil
	}
	return tx.Model(&model.AttachmentModel{}).Where("id = ? AND root_id = ? AND root_id_type = ?", attachmentID, worldID, "world_clue").Updates(map[string]any{
		"is_temp": false, "root_id": worldID, "root_id_type": "world_clue",
	}).Error
}

func encodeWorldCluePresentation(value *protocol.WorldCluePresentation) (string, protocol.WorldCluePresentation, error) {
	normalized, err := ValidateWorldCluePresentation(value)
	if err != nil {
		return "", normalized, err
	}
	raw, err := json.Marshal(normalized)
	return string(raw), normalized, err
}

func decodeWorldCluePresentation(raw string) protocol.WorldCluePresentation {
	result := defaultWorldCluePresentation()
	if strings.TrimSpace(raw) == "" {
		return result
	}
	parsed := defaultWorldCluePresentation()
	if json.Unmarshal([]byte(raw), &parsed) != nil {
		return result
	}
	validated, err := ValidateWorldCluePresentation(&parsed)
	if err != nil {
		return result
	}
	return validated
}

func WorldClueCreate(worldID, actorID string, input WorldClueCreateInput) (*protocol.WorldClueAdminDetail, error) {
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return nil, err
	}
	if !worldClueIsAdminRole(role) {
		return nil, ErrWorldClueDenied
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return nil, fmt.Errorf("%w: title is required", ErrWorldClueInvalid)
	}
	if input.DefaultAccess == "" {
		input.DefaultAccess = model.WorldClueAccessView
	}
	if !oneOf(input.DefaultAccess, model.WorldClueAccessNone, model.WorldClueAccessView) {
		return nil, fmt.Errorf("%w: invalid default access", ErrWorldClueInvalid)
	}
	format, content, contentText, err := normalizeWorldClueContent(input.ContentFormat, input.Content)
	if err != nil {
		return nil, err
	}
	managerFormat, managerNote, managerText, err := normalizeWorldClueContent(input.ManagerNoteFormat, input.ManagerNote)
	if err != nil {
		return nil, err
	}
	kind, attachmentID, imageURL, embedURL, err := normalizeWorldClueKind(input.Kind, input.ImageAttachmentID, input.ImageURL, input.EmbedURL)
	if err != nil {
		return nil, err
	}
	presentationJSON, presentation, err := encodeWorldCluePresentation(input.Presentation)
	if err != nil {
		return nil, err
	}
	clue := &model.WorldClueModel{
		WorldID: worldID, SharedFolderID: strings.TrimSpace(input.SharedFolderID), Title: input.Title,
		Kind: kind, ContentFormat: format, Content: content, ContentText: contentText,
		ImageAttachmentID: attachmentID, ImageURL: imageURL, EmbedURL: embedURL,
		PresentationJSON: presentationJSON, DefaultAccess: input.DefaultAccess,
		Status: model.WorldClueStatusDraft, Revision: 1, PublishSeq: 0, OrderIndex: input.OrderIndex,
		ManagerNoteFormat: managerFormat, ManagerNote: managerNote, ManagerNoteText: managerText,
		CreatorID: actorID, UpdatedBy: actorID,
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := validateWorldClueFolderForClue(tx, worldID, clue.SharedFolderID); err != nil {
			return err
		}
		if err := validateWorldClueAttachment(tx, worldID, actorID, attachmentID, presentation.MediaType); err != nil {
			return err
		}
		if err := validateWorldClueBackgroundAttachment(tx, worldID, actorID, presentation.BackgroundMediaAttachmentID, presentation.BackgroundMediaType); err != nil {
			return err
		}
		if err := tx.Create(clue).Error; err != nil {
			return err
		}
		if err := confirmWorldClueAttachment(tx, worldID, attachmentID); err != nil {
			return err
		}
		return confirmWorldClueAttachment(tx, worldID, presentation.BackgroundMediaAttachmentID)
	})
	if err != nil {
		return nil, err
	}
	return WorldClueGetAdmin(worldID, clue.ID, actorID)
}

func WorldClueGet(worldID, clueID, actorID string) (*protocol.WorldClueDetail, error) {
	db := model.GetDB()
	clue, role, accessRow, state, access, err := loadWorldClueContext(db, worldID, clueID, actorID)
	if err != nil || !canViewWorldClue(clue, role, access) {
		if err != nil && !errors.Is(err, ErrWorldClueNotFound) {
			return nil, err
		}
		return nil, ErrWorldClueNotFound
	}
	return worldClueDetailDTO(clue, role, accessRow, state, access), nil
}

func WorldClueGetAdmin(worldID, clueID, actorID string) (*protocol.WorldClueAdminDetail, error) {
	db := model.GetDB()
	clue, role, accessRow, state, access, err := loadWorldClueContext(db, worldID, clueID, actorID)
	if err != nil {
		return nil, err
	}
	if !worldClueIsAdminRole(role) {
		return nil, ErrWorldClueDenied
	}
	detail := worldClueDetailDTO(clue, role, accessRow, state, access)
	return &protocol.WorldClueAdminDetail{
		WorldClueDetail: *detail, ManagerNoteFormat: clue.ManagerNoteFormat,
		ManagerNote: clue.ManagerNote, ManagerNoteText: clue.ManagerNoteText,
		UpdatedBy: clue.UpdatedBy, PublishedBy: clue.PublishedBy,
	}, nil
}

func loadWorldClueContext(tx *gorm.DB, worldID, clueID, actorID string) (*model.WorldClueModel, string, *model.WorldClueAccessModel, *model.WorldClueUserStateModel, string, error) {
	role, err := worldClueRole(tx, worldID, actorID)
	if err != nil {
		return nil, "", nil, nil, "", err
	}
	if role == "" {
		return nil, role, nil, nil, model.WorldClueAccessNone, ErrWorldClueNotFound
	}
	var clue model.WorldClueModel
	if err := tx.Where("world_id = ? AND id = ?", worldID, clueID).Limit(1).Find(&clue).Error; err != nil {
		return nil, role, nil, nil, "", err
	}
	if clue.ID == "" {
		return nil, role, nil, nil, "", ErrWorldClueNotFound
	}
	accessRow, err := getWorldClueAccessRow(tx, worldID, clueID, actorID)
	if err != nil {
		return nil, role, nil, nil, "", err
	}
	state, err := getWorldClueState(tx, worldID, clueID, actorID)
	if err != nil {
		return nil, role, nil, nil, "", err
	}
	override := model.WorldClueAccessInherit
	if accessRow != nil {
		override = accessRow.AccessOverride
	}
	access := effectiveWorldClueAccess(role, clue.DefaultAccess, override)
	return &clue, role, accessRow, state, access, nil
}

func worldClueStateDTO(state *model.WorldClueUserStateModel) *protocol.WorldClueUserState {
	if state == nil {
		return &protocol.WorldClueUserState{}
	}
	result := &protocol.WorldClueUserState{
		PersonalFolderID: state.PersonalFolderID, PersonalOrder: state.PersonalOrder, Favorite: state.Favorite,
		SeenRevision: state.SeenRevision, SeenPrivateRevision: state.SeenPrivateRevision,
		AssignedPresentationSeq: state.AssignedPresentationSeq, PresentedPresentationSeq: state.PresentedPresentationSeq,
	}
	if state.LastOpenedAt != nil {
		result.LastOpenedAt = state.LastOpenedAt.UnixMilli()
	}
	return result
}

func worldClueSummaryDTO(clue *model.WorldClueModel, role string, accessRow *model.WorldClueAccessModel, state *model.WorldClueUserStateModel, access string) protocol.WorldClueSummary {
	privateRevision := int64(0)
	hasPrivate := false
	if accessRow != nil && clue.Status == model.WorldClueStatusPublished && access != model.WorldClueAccessNone && strings.TrimSpace(accessRow.PrivateContent) != "" {
		privateRevision = accessRow.PrivateRevision
		hasPrivate = true
	}
	seenRevision, seenPrivateRevision := int64(0), int64(0)
	if state != nil {
		seenRevision, seenPrivateRevision = state.SeenRevision, state.SeenPrivateRevision
	}
	contentText := strings.TrimSpace(clue.ContentText)
	contentRunes := []rune(contentText)
	if len(contentRunes) > 240 {
		contentText = string(contentRunes[:240])
	}
	result := protocol.WorldClueSummary{
		ID: clue.ID, WorldID: clue.WorldID, SharedFolderID: clue.SharedFolderID, Title: clue.Title,
		Kind: clue.Kind, ContentText: contentText, ImageAttachmentID: clue.ImageAttachmentID,
		ImageURL: clue.ImageURL, EffectiveAccess: access, Status: clue.Status, Revision: clue.Revision,
		PublishSeq: clue.PublishSeq, OrderIndex: clue.OrderIndex, PrivateRevision: privateRevision,
		HasPrivateContent: hasPrivate, Unread: clue.Status == model.WorldClueStatusPublished && (clue.Revision > seenRevision || privateRevision > seenPrivateRevision),
		UserState: worldClueStateDTO(state), CreatorID: clue.CreatorID, UpdatedAt: clue.UpdatedAt.UnixMilli(),
	}
	if worldClueIsAdminRole(role) {
		result.DefaultAccess = clue.DefaultAccess
	}
	if clue.PublishedAt != nil {
		result.PublishedAt = clue.PublishedAt.UnixMilli()
	}
	if parsed, err := url.Parse(clue.EmbedURL); err == nil {
		result.EmbedDomain = parsed.Hostname()
	}
	return result
}

func worldClueDetailDTO(clue *model.WorldClueModel, role string, accessRow *model.WorldClueAccessModel, state *model.WorldClueUserStateModel, access string) *protocol.WorldClueDetail {
	presentation := decodeWorldCluePresentation(clue.PresentationJSON)
	attachmentIDs := make([]string, 0, 2)
	if clue.ImageAttachmentID != "" {
		attachmentIDs = append(attachmentIDs, clue.ImageAttachmentID)
	}
	if presentation.BackgroundMediaAttachmentID != "" && presentation.BackgroundMediaAttachmentID != clue.ImageAttachmentID {
		attachmentIDs = append(attachmentIDs, presentation.BackgroundMediaAttachmentID)
	}
	if len(attachmentIDs) > 0 {
		var attachments []model.AttachmentModel
		model.GetDB().Select("id", "mime_type").Where("id IN ?", attachmentIDs).Find(&attachments)
		for _, attachment := range attachments {
			mediaType := "image"
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(attachment.MimeType)), "video/") {
				mediaType = "video"
			}
			if attachment.ID == clue.ImageAttachmentID {
				presentation.MediaType = mediaType
			}
			if attachment.ID == presentation.BackgroundMediaAttachmentID {
				presentation.BackgroundMediaType = mediaType
			}
		}
	}
	result := &protocol.WorldClueDetail{
		WorldClueSummary: worldClueSummaryDTO(clue, role, accessRow, state, access),
		ContentFormat:    clue.ContentFormat, Content: clue.Content, EmbedURL: clue.EmbedURL,
		Presentation: presentation,
	}
	if clue.Status == model.WorldClueStatusPublished && access != model.WorldClueAccessNone && accessRow != nil {
		result.PrivateContentFormat = accessRow.PrivateContentFormat
		result.PrivateContent = accessRow.PrivateContent
	}
	return result
}

func WorldClueList(worldID, actorID, keyword string) ([]protocol.WorldClueSummary, error) {
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return nil, err
	}
	if role == "" {
		return nil, ErrWorldClueDenied
	}
	var clues []model.WorldClueModel
	if err := db.Where("world_id = ? AND status <> ?", worldID, model.WorldClueStatusArchived).
		Order("order_index ASC, updated_at DESC, id ASC").Limit(1000).Find(&clues).Error; err != nil {
		return nil, err
	}
	var accessRows []model.WorldClueAccessModel
	if err := db.Where("world_id = ? AND user_id = ?", worldID, actorID).Find(&accessRows).Error; err != nil {
		return nil, err
	}
	var states []model.WorldClueUserStateModel
	if err := db.Where("world_id = ? AND user_id = ?", worldID, actorID).Find(&states).Error; err != nil {
		return nil, err
	}
	accessByClue := make(map[string]*model.WorldClueAccessModel, len(accessRows))
	for i := range accessRows {
		accessByClue[accessRows[i].ClueID] = &accessRows[i]
	}
	stateByClue := make(map[string]*model.WorldClueUserStateModel, len(states))
	for i := range states {
		stateByClue[states[i].ClueID] = &states[i]
	}
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	result := make([]protocol.WorldClueSummary, 0, len(clues))
	for i := range clues {
		clue := &clues[i]
		accessRow := accessByClue[clue.ID]
		override := model.WorldClueAccessInherit
		if accessRow != nil {
			override = accessRow.AccessOverride
		}
		access := effectiveWorldClueAccess(role, clue.DefaultAccess, override)
		if !canViewWorldClue(clue, role, access) {
			continue
		}
		if keyword != "" {
			haystack := strings.ToLower(clue.Title + "\n" + clue.ContentText)
			if accessRow != nil && clue.Status == model.WorldClueStatusPublished {
				haystack += "\n" + strings.ToLower(accessRow.PrivateContentText)
			}
			if !strings.Contains(haystack, keyword) {
				continue
			}
		}
		result = append(result, worldClueSummaryDTO(clue, role, accessRow, stateByClue[clue.ID], access))
	}
	return result, nil
}

func WorldClueUpdate(worldID, clueID, actorID string, input WorldClueUpdateInput) (*protocol.WorldClueDetail, error) {
	db := model.GetDB()
	clue, role, _, _, access, err := loadWorldClueContext(db, worldID, clueID, actorID)
	if err != nil {
		return nil, err
	}
	if !worldClueIsAdminRole(role) && access != model.WorldClueAccessEdit {
		return nil, ErrWorldClueDenied
	}
	if input.ExpectedRevision <= 0 {
		return nil, fmt.Errorf("%w: expectedRevision is required", ErrWorldClueInvalid)
	}
	updates := map[string]any{"revision": gorm.Expr("revision + 1"), "updated_by": actorID, "updated_at": time.Now()}
	presentation := decodeWorldCluePresentation(clue.PresentationJSON)
	backgroundAttachmentID := presentation.BackgroundMediaAttachmentID
	mediaType := presentation.MediaType
	backgroundMediaType := presentation.BackgroundMediaType
	if input.Title != nil {
		value := strings.TrimSpace(*input.Title)
		if value == "" {
			return nil, fmt.Errorf("%w: title is required", ErrWorldClueInvalid)
		}
		updates["title"] = value
	}
	contentFormat, content := clue.ContentFormat, clue.Content
	if input.ContentFormat != nil {
		contentFormat = *input.ContentFormat
	}
	if input.Content != nil {
		content = *input.Content
	}
	if input.ContentFormat != nil || input.Content != nil {
		format, normalizedContent, text, err := normalizeWorldClueContent(contentFormat, content)
		if err != nil {
			return nil, err
		}
		updates["content_format"], updates["content"], updates["content_text"] = format, normalizedContent, text
	}
	kind, attachmentID, imageURL, embedURL := clue.Kind, clue.ImageAttachmentID, clue.ImageURL, clue.EmbedURL
	if input.Kind != nil {
		kind = *input.Kind
	}
	if input.ImageAttachmentID != nil {
		attachmentID = *input.ImageAttachmentID
	}
	if input.ImageURL != nil {
		imageURL = *input.ImageURL
	}
	if input.EmbedURL != nil {
		embedURL = *input.EmbedURL
	}
	if input.Kind != nil || input.ImageAttachmentID != nil || input.ImageURL != nil || input.EmbedURL != nil {
		kind, attachmentID, imageURL, embedURL, err = normalizeWorldClueKind(kind, attachmentID, imageURL, embedURL)
		if err != nil {
			return nil, err
		}
		updates["kind"], updates["image_attachment_id"], updates["image_url"], updates["embed_url"] = kind, attachmentID, imageURL, embedURL
	}
	if input.Presentation != nil {
		raw, normalized, err := encodeWorldCluePresentation(input.Presentation)
		if err != nil {
			return nil, err
		}
		backgroundAttachmentID = normalized.BackgroundMediaAttachmentID
		mediaType = normalized.MediaType
		backgroundMediaType = normalized.BackgroundMediaType
		updates["presentation_json"] = raw
	}
	if input.OrderIndex != nil {
		if !worldClueIsAdminRole(role) {
			return nil, ErrWorldClueDenied
		}
		updates["order_index"] = *input.OrderIndex
	}
	if input.SharedFolderID != nil {
		if !worldClueIsAdminRole(role) {
			return nil, ErrWorldClueDenied
		}
		updates["shared_folder_id"] = strings.TrimSpace(*input.SharedFolderID)
	}
	if input.DefaultAccess != nil {
		if !worldClueIsAdminRole(role) {
			return nil, ErrWorldClueDenied
		}
		if !oneOf(*input.DefaultAccess, model.WorldClueAccessNone, model.WorldClueAccessView) {
			return nil, fmt.Errorf("%w: invalid default access", ErrWorldClueInvalid)
		}
		updates["default_access"] = *input.DefaultAccess
	}
	if input.ManagerNoteFormat != nil || input.ManagerNote != nil {
		if !worldClueIsAdminRole(role) {
			return nil, ErrWorldClueDenied
		}
		format, note := clue.ManagerNoteFormat, clue.ManagerNote
		if input.ManagerNoteFormat != nil {
			format = *input.ManagerNoteFormat
		}
		if input.ManagerNote != nil {
			note = *input.ManagerNote
		}
		format, note, text, err := normalizeWorldClueContent(format, note)
		if err != nil {
			return nil, err
		}
		updates["manager_note_format"], updates["manager_note"], updates["manager_note_text"] = format, note, text
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if folderValue, ok := updates["shared_folder_id"].(string); ok {
			if err := validateWorldClueFolderForClue(tx, worldID, folderValue); err != nil {
				return err
			}
		}
		if attachmentID != clue.ImageAttachmentID || input.Presentation != nil {
			if err := validateWorldClueAttachment(tx, worldID, actorID, attachmentID, mediaType); err != nil {
				return err
			}
		}
		if input.Presentation != nil {
			if err := validateWorldClueBackgroundAttachment(tx, worldID, actorID, backgroundAttachmentID, backgroundMediaType); err != nil {
				return err
			}
		}
		result := tx.Model(&model.WorldClueModel{}).
			Where("world_id = ? AND id = ? AND revision = ?", worldID, clueID, input.ExpectedRevision).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrWorldClueConflict
		}
		if err := confirmWorldClueAttachment(tx, worldID, attachmentID); err != nil {
			return err
		}
		return confirmWorldClueAttachment(tx, worldID, backgroundAttachmentID)
	})
	if err != nil {
		return nil, err
	}
	return WorldClueGet(worldID, clueID, actorID)
}

func WorldClueDelete(worldID, clueID, actorID string) error {
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return err
	}
	if !worldClueIsAdminRole(role) {
		return ErrWorldClueDenied
	}
	return db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.WorldClueModel{}).Where("world_id = ? AND id = ?", worldID, clueID).
			Updates(map[string]any{"status": model.WorldClueStatusArchived, "updated_by": actorID, "updated_at": time.Now()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrWorldClueNotFound
		}
		return tx.Unscoped().Where("world_id = ? AND clue_id = ?", worldID, clueID).Delete(&model.WorldClueEditLockModel{}).Error
	})
}

func WorldCluePublish(worldID, clueID, actorID string, expectedPublishSeq int64) (*WorldCluePublishResult, error) {
	db := model.GetDB()
	var recipientIDs []string
	var publishSeq int64
	err := db.Transaction(func(tx *gorm.DB) error {
		role, err := worldClueRole(tx, worldID, actorID)
		if err != nil {
			return err
		}
		if !worldClueIsAdminRole(role) {
			return ErrWorldClueDenied
		}
		var clue model.WorldClueModel
		if err := tx.Where("world_id = ? AND id = ? AND status <> ?", worldID, clueID, model.WorldClueStatusArchived).Limit(1).Find(&clue).Error; err != nil {
			return err
		}
		if clue.ID == "" {
			return ErrWorldClueNotFound
		}
		if clue.PublishSeq != expectedPublishSeq {
			return ErrWorldClueConflict
		}
		publishSeq = clue.PublishSeq + 1
		now := time.Now()
		result := tx.Model(&model.WorldClueModel{}).
			Where("world_id = ? AND id = ? AND publish_seq = ? AND status <> ?", worldID, clueID, expectedPublishSeq, model.WorldClueStatusArchived).
			Updates(map[string]any{"status": model.WorldClueStatusPublished, "publish_seq": publishSeq, "published_at": now, "published_by": actorID, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrWorldClueConflict
		}
		var members []model.WorldMemberModel
		if err := tx.Where("world_id = ?", worldID).Find(&members).Error; err != nil {
			return err
		}
		var accessRows []model.WorldClueAccessModel
		if err := tx.Where("world_id = ? AND clue_id = ?", worldID, clueID).Find(&accessRows).Error; err != nil {
			return err
		}
		accessByUser := make(map[string]string, len(accessRows))
		for _, row := range accessRows {
			accessByUser[row.UserID] = row.AccessOverride
		}
		for _, member := range members {
			access := effectiveWorldClueAccess(member.Role, clue.DefaultAccess, accessByUser[member.UserID])
			if access == model.WorldClueAccessNone {
				continue
			}
			recipientIDs = append(recipientIDs, member.UserID)
			state := model.WorldClueUserStateModel{
				WorldID: worldID, ClueID: clueID, UserID: member.UserID, AssignedPresentationSeq: publishSeq,
			}
			state.ID = utils.NewID()
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "world_id"}, {Name: "clue_id"}, {Name: "user_id"}},
				DoUpdates: clause.Assignments(map[string]any{"assigned_presentation_seq": publishSeq, "updated_at": now}),
			}).Create(&state).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	detail, err := WorldClueGet(worldID, clueID, actorID)
	if err != nil {
		return nil, err
	}
	return &WorldCluePublishResult{Clue: detail, RecipientIDs: recipientIDs}, nil
}

func WorldClueReveal(worldID, clueID, actorID string, userIDs []string, expectedPublishSeq int64) (*WorldCluePublishResult, error) {
	db := model.GetDB()
	recipientIDs := make([]string, 0, len(userIDs))
	seenIDs := make(map[string]struct{}, len(userIDs))
	for _, rawID := range userIDs {
		userID := strings.TrimSpace(rawID)
		if userID == "" {
			continue
		}
		if _, ok := seenIDs[userID]; ok {
			continue
		}
		seenIDs[userID] = struct{}{}
		recipientIDs = append(recipientIDs, userID)
	}
	var publishSeq int64
	err := db.Transaction(func(tx *gorm.DB) error {
		role, err := worldClueRole(tx, worldID, actorID)
		if err != nil {
			return err
		}
		if !worldClueIsAdminRole(role) {
			return ErrWorldClueDenied
		}
		var clue model.WorldClueModel
		if err := tx.Where("world_id = ? AND id = ? AND status <> ?", worldID, clueID, model.WorldClueStatusArchived).Limit(1).Find(&clue).Error; err != nil {
			return err
		}
		if clue.ID == "" {
			return ErrWorldClueNotFound
		}
		if clue.PublishSeq != expectedPublishSeq {
			return ErrWorldClueConflict
		}
		if len(recipientIDs) == 0 {
			return fmt.Errorf("%w: at least one recipient is required", ErrWorldClueInvalid)
		}
		var members []model.WorldMemberModel
		if err := tx.Where("world_id = ? AND user_id IN ?", worldID, recipientIDs).Find(&members).Error; err != nil {
			return err
		}
		memberIDs := make(map[string]struct{}, len(members))
		for _, member := range members {
			memberIDs[member.UserID] = struct{}{}
		}
		if len(memberIDs) != len(recipientIDs) {
			return fmt.Errorf("%w: recipient is not a world member", ErrWorldClueInvalid)
		}
		publishSeq = clue.PublishSeq + 1
		now := time.Now()
		result := tx.Model(&model.WorldClueModel{}).
			Where("world_id = ? AND id = ? AND publish_seq = ? AND status <> ?", worldID, clueID, expectedPublishSeq, model.WorldClueStatusArchived).
			Updates(map[string]any{"status": model.WorldClueStatusPublished, "publish_seq": publishSeq, "published_at": now, "published_by": actorID, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrWorldClueConflict
		}
		for _, userID := range recipientIDs {
			row := model.WorldClueAccessModel{
				WorldID: worldID, ClueID: clueID, UserID: userID,
				AccessOverride: model.WorldClueAccessView, PrivateContentFormat: model.WorldClueContentPlain,
				UpdatedBy: actorID,
			}
			row.ID = utils.NewID()
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "world_id"}, {Name: "clue_id"}, {Name: "user_id"}},
				DoUpdates: clause.Assignments(map[string]any{"access_override": model.WorldClueAccessView, "updated_by": actorID, "updated_at": now}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}
		for _, userID := range recipientIDs {
			state := model.WorldClueUserStateModel{WorldID: worldID, ClueID: clueID, UserID: userID, AssignedPresentationSeq: publishSeq}
			state.ID = utils.NewID()
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "world_id"}, {Name: "clue_id"}, {Name: "user_id"}},
				DoUpdates: clause.Assignments(map[string]any{"assigned_presentation_seq": publishSeq, "updated_at": now}),
			}).Create(&state).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	detail, err := WorldClueGet(worldID, clueID, actorID)
	if err != nil {
		return nil, err
	}
	return &WorldCluePublishResult{Clue: detail, RecipientIDs: recipientIDs}, nil
}

func WorldClueUnpublish(worldID, clueID, actorID string, expectedPublishSeq int64) (*protocol.WorldClueDetail, error) {
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return nil, err
	}
	if !worldClueIsAdminRole(role) {
		return nil, ErrWorldClueDenied
	}
	result := db.Model(&model.WorldClueModel{}).
		Where("world_id = ? AND id = ? AND publish_seq = ? AND status = ?", worldID, clueID, expectedPublishSeq, model.WorldClueStatusPublished).
		Updates(map[string]any{"status": model.WorldClueStatusDraft, "updated_at": time.Now(), "updated_by": actorID})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrWorldClueConflict
	}
	return WorldClueGet(worldID, clueID, actorID)
}

func WorldCluePendingPresentations(worldID, actorID string) ([]protocol.WorldClueDetail, error) {
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return nil, err
	}
	if role == "" {
		return nil, ErrWorldClueDenied
	}
	var states []model.WorldClueUserStateModel
	if err := db.Where("world_id = ? AND user_id = ? AND assigned_presentation_seq > presented_presentation_seq", worldID, actorID).Find(&states).Error; err != nil {
		return nil, err
	}
	result := make([]protocol.WorldClueDetail, 0, len(states))
	for i := range states {
		detail, err := WorldClueGet(worldID, states[i].ClueID, actorID)
		if errors.Is(err, ErrWorldClueNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if detail.Status != model.WorldClueStatusPublished || detail.PublishSeq < states[i].AssignedPresentationSeq {
			continue
		}
		detail.PublishSeq = states[i].AssignedPresentationSeq
		result = append(result, *detail)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].PublishedAt == result[j].PublishedAt {
			return result[i].PublishSeq < result[j].PublishSeq
		}
		return result[i].PublishedAt < result[j].PublishedAt
	})
	return result, nil
}

func ensureWorldClueUserState(tx *gorm.DB, worldID, clueID, userID string) error {
	state := model.WorldClueUserStateModel{WorldID: worldID, ClueID: clueID, UserID: userID}
	state.ID = utils.NewID()
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "world_id"}, {Name: "clue_id"}, {Name: "user_id"}}, DoNothing: true}).Create(&state).Error
}

func WorldClueMarkPresented(worldID, clueID, actorID string, publishSeq int64) error {
	db := model.GetDB()
	detail, err := WorldClueGet(worldID, clueID, actorID)
	if err != nil || detail.Status != model.WorldClueStatusPublished {
		return ErrWorldClueNotFound
	}
	if publishSeq <= 0 || publishSeq > detail.PublishSeq {
		return fmt.Errorf("%w: invalid publish sequence", ErrWorldClueInvalid)
	}
	var state model.WorldClueUserStateModel
	if err := db.Where("world_id = ? AND clue_id = ? AND user_id = ?", worldID, clueID, actorID).Limit(1).Find(&state).Error; err != nil {
		return err
	}
	if state.ID == "" || publishSeq > state.AssignedPresentationSeq {
		return fmt.Errorf("%w: presentation was not assigned", ErrWorldClueInvalid)
	}
	return db.Model(&model.WorldClueUserStateModel{}).
		Where("world_id = ? AND clue_id = ? AND user_id = ? AND assigned_presentation_seq >= ? AND presented_presentation_seq < ?", worldID, clueID, actorID, publishSeq, publishSeq).
		Update("presented_presentation_seq", publishSeq).Error
}

func WorldClueMarkSeen(worldID, clueID, actorID string) error {
	db := model.GetDB()
	detail, err := WorldClueGet(worldID, clueID, actorID)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := ensureWorldClueUserState(tx, worldID, clueID, actorID); err != nil {
			return err
		}
		now := time.Now()
		return tx.Model(&model.WorldClueUserStateModel{}).
			Where("world_id = ? AND clue_id = ? AND user_id = ?", worldID, clueID, actorID).
			Updates(map[string]any{
				"seen_revision":         gorm.Expr("CASE WHEN seen_revision < ? THEN ? ELSE seen_revision END", detail.Revision, detail.Revision),
				"seen_private_revision": gorm.Expr("CASE WHEN seen_private_revision < ? THEN ? ELSE seen_private_revision END", detail.PrivateRevision, detail.PrivateRevision),
				"last_opened_at":        now, "updated_at": now,
			}).Error
	})
}

func WorldClueSetAccess(worldID, clueID, actorID, userID, accessOverride string) (*protocol.WorldClueAccess, error) {
	db := model.GetDB()
	actorRole, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return nil, err
	}
	if !worldClueIsAdminRole(actorRole) {
		return nil, ErrWorldClueDenied
	}
	if !oneOf(accessOverride, model.WorldClueAccessInherit, model.WorldClueAccessNone, model.WorldClueAccessView, model.WorldClueAccessEdit) {
		return nil, fmt.Errorf("%w: invalid access override", ErrWorldClueInvalid)
	}
	var clue model.WorldClueModel
	if err := db.Where("world_id = ? AND id = ? AND status <> ?", worldID, clueID, model.WorldClueStatusArchived).Limit(1).Find(&clue).Error; err != nil {
		return nil, err
	}
	if clue.ID == "" {
		return nil, ErrWorldClueNotFound
	}
	targetRole, err := worldClueRole(db, worldID, userID)
	if err != nil {
		return nil, err
	}
	if targetRole == "" {
		return nil, ErrWorldClueInvalid
	}
	row := model.WorldClueAccessModel{WorldID: worldID, ClueID: clueID, UserID: userID, AccessOverride: accessOverride, PrivateContentFormat: model.WorldClueContentPlain, UpdatedBy: actorID}
	row.ID = utils.NewID()
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "world_id"}, {Name: "clue_id"}, {Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{"access_override": accessOverride, "updated_by": actorID, "updated_at": time.Now()}),
	}).Create(&row).Error; err != nil {
		return nil, err
	}
	stored, err := getWorldClueAccessRow(db, worldID, clueID, userID)
	if err != nil {
		return nil, err
	}
	item := worldClueAccessDTO(userID, targetRole, clue.DefaultAccess, accessOverride, stored)
	return &item, nil
}

// WorldClueExecuteTheaterEntry applies one saved theater clue entry atomically.
// It intentionally keeps private-content columns out of the upsert update set.
func WorldClueExecuteTheaterEntry(worldID, clueID, actorID string, targets []WorldClueTheaterTarget, present bool) (*WorldClueTheaterEntryResult, error) {
	db := model.GetDB()
	result := &WorldClueTheaterEntryResult{ClueID: strings.TrimSpace(clueID), RecipientIDs: []string{}}
	err := db.Transaction(func(tx *gorm.DB) error {
		role, err := worldClueRole(tx, worldID, actorID)
		if err != nil {
			return err
		}
		if !worldClueIsAdminRole(role) {
			return ErrWorldClueDenied
		}
		var clue model.WorldClueModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("world_id = ? AND id = ? AND status <> ?", worldID, clueID, model.WorldClueStatusArchived).Take(&clue).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrWorldClueNotFound
			}
			return err
		}
		result.ClueID, result.Revision, result.Status, result.PublishSeq = clue.ID, clue.Revision, clue.Status, clue.PublishSeq
		normalizedTargets := make([]WorldClueTheaterTarget, 0, len(targets))
		seen := make(map[string]struct{}, len(targets))
		for _, target := range targets {
			userID := strings.TrimSpace(target.UserID)
			if userID == "" {
				return fmt.Errorf("%w: target userId is required", ErrWorldClueInvalid)
			}
			if _, exists := seen[userID]; exists {
				return fmt.Errorf("%w: duplicate target userId", ErrWorldClueInvalid)
			}
			seen[userID] = struct{}{}
			if target.Access != "keep" && target.Access != model.WorldClueAccessInherit && target.Access != model.WorldClueAccessNone && target.Access != model.WorldClueAccessView && target.Access != model.WorldClueAccessEdit {
				return fmt.Errorf("%w: invalid access override", ErrWorldClueInvalid)
			}
			memberRole, err := worldClueRole(tx, worldID, userID)
			if err != nil {
				return err
			}
			if memberRole == "" {
				return fmt.Errorf("%w: target is not a world member", ErrWorldClueInvalid)
			}
			normalizedTargets = append(normalizedTargets, WorldClueTheaterTarget{UserID: userID, Access: target.Access})
		}
		var accessRows []model.WorldClueAccessModel
		if err := tx.Where("world_id = ? AND clue_id = ?", worldID, clueID).Find(&accessRows).Error; err != nil {
			return err
		}
		overrides := make(map[string]string, len(accessRows))
		for i := range accessRows {
			overrides[accessRows[i].UserID] = accessRows[i].AccessOverride
		}
		now := time.Now()
		for _, target := range normalizedTargets {
			if target.Access == "keep" {
				continue
			}
			userID := strings.TrimSpace(target.UserID)
			row := model.WorldClueAccessModel{WorldID: worldID, ClueID: clueID, UserID: userID, AccessOverride: target.Access, PrivateContentFormat: model.WorldClueContentPlain, UpdatedBy: actorID}
			row.ID = utils.NewID()
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "world_id"}, {Name: "clue_id"}, {Name: "user_id"}},
				DoUpdates: clause.Assignments(map[string]any{"access_override": target.Access, "updated_by": actorID, "updated_at": now}),
			}).Create(&row).Error; err != nil {
				return err
			}
			overrides[userID] = target.Access
		}
		if !present {
			return nil
		}
		for _, target := range normalizedTargets {
			memberRole, err := worldClueRole(tx, worldID, target.UserID)
			if err != nil {
				return err
			}
			access := overrides[target.UserID]
			if access == "" {
				access = model.WorldClueAccessInherit
			}
			if effective := effectiveWorldClueAccess(memberRole, clue.DefaultAccess, access); effective == model.WorldClueAccessView || effective == model.WorldClueAccessEdit {
				result.RecipientIDs = append(result.RecipientIDs, target.UserID)
			}
		}
		if len(result.RecipientIDs) == 0 {
			return nil
		}
		publishSeq := clue.PublishSeq + 1
		updateResult := tx.Model(&model.WorldClueModel{}).
			Where("world_id = ? AND id = ? AND publish_seq = ? AND status <> ?", worldID, clueID, clue.PublishSeq, model.WorldClueStatusArchived).
			Updates(map[string]any{
				"status":       model.WorldClueStatusPublished,
				"publish_seq":  publishSeq,
				"published_at": now,
				"published_by": actorID,
				"updated_at":   now,
			})
		if updateResult.Error != nil {
			return updateResult.Error
		}
		if updateResult.RowsAffected != 1 {
			return ErrWorldClueConflict
		}
		for _, userID := range result.RecipientIDs {
			state := model.WorldClueUserStateModel{WorldID: worldID, ClueID: clueID, UserID: userID, AssignedPresentationSeq: publishSeq}
			state.ID = utils.NewID()
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "world_id"}, {Name: "clue_id"}, {Name: "user_id"}}, DoUpdates: clause.Assignments(map[string]any{"assigned_presentation_seq": publishSeq, "updated_at": now})}).Create(&state).Error; err != nil {
				return err
			}
		}
		result.PublishSeq, result.Status = publishSeq, model.WorldClueStatusPublished
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func WorldClueListAccess(worldID, clueID, actorID string) ([]protocol.WorldClueAccess, error) {
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return nil, err
	}
	if !worldClueIsAdminRole(role) {
		return nil, ErrWorldClueDenied
	}
	var clue model.WorldClueModel
	if err := db.Where("world_id = ? AND id = ?", worldID, clueID).Limit(1).Find(&clue).Error; err != nil || clue.ID == "" {
		return nil, ErrWorldClueNotFound
	}
	var members []model.WorldMemberModel
	if err := db.Where("world_id = ?", worldID).Order("created_at ASC").Find(&members).Error; err != nil {
		return nil, err
	}
	var rows []model.WorldClueAccessModel
	if err := db.Where("world_id = ? AND clue_id = ?", worldID, clueID).Find(&rows).Error; err != nil {
		return nil, err
	}
	byUser := map[string]*model.WorldClueAccessModel{}
	for i := range rows {
		byUser[rows[i].UserID] = &rows[i]
	}
	items := make([]protocol.WorldClueAccess, 0, len(members))
	for _, member := range members {
		row := byUser[member.UserID]
		override := model.WorldClueAccessInherit
		if row != nil {
			override = row.AccessOverride
		}
		items = append(items, worldClueAccessDTO(member.UserID, member.Role, clue.DefaultAccess, override, row))
	}
	return items, nil
}

func WorldClueGetPrivate(worldID, clueID, actorID, userID string) (*protocol.WorldCluePrivateContent, error) {
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return nil, err
	}
	if !worldClueIsAdminRole(role) {
		return nil, ErrWorldClueDenied
	}
	if targetRole, err := worldClueRole(db, worldID, userID); err != nil || targetRole == "" {
		return nil, ErrWorldClueNotFound
	}
	var clue model.WorldClueModel
	if err := db.Where("world_id = ? AND id = ?", worldID, clueID).Limit(1).Find(&clue).Error; err != nil || clue.ID == "" {
		return nil, ErrWorldClueNotFound
	}
	row, err := getWorldClueAccessRow(db, worldID, clueID, userID)
	if err != nil {
		return nil, err
	}
	result := &protocol.WorldCluePrivateContent{UserID: userID, PrivateContentFormat: model.WorldClueContentPlain}
	if row != nil {
		result.PrivateContentFormat, result.PrivateContent, result.PrivateRevision = row.PrivateContentFormat, row.PrivateContent, row.PrivateRevision
	}
	return result, nil
}

func WorldCluePutPrivate(worldID, clueID, actorID, userID, format, content string, expectedRevision int64) (*protocol.WorldCluePrivateContent, error) {
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return nil, err
	}
	if !worldClueIsAdminRole(role) {
		return nil, ErrWorldClueDenied
	}
	if targetRole, err := worldClueRole(db, worldID, userID); err != nil || targetRole == "" {
		return nil, ErrWorldClueNotFound
	}
	var clue model.WorldClueModel
	if err := db.Where("world_id = ? AND id = ? AND status <> ?", worldID, clueID, model.WorldClueStatusArchived).Limit(1).Find(&clue).Error; err != nil || clue.ID == "" {
		return nil, ErrWorldClueNotFound
	}
	format, content, text, err := normalizeWorldClueContent(format, content)
	if err != nil {
		return nil, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var row model.WorldClueAccessModel
		if err := tx.Where("world_id = ? AND clue_id = ? AND user_id = ?", worldID, clueID, userID).Limit(1).Find(&row).Error; err != nil {
			return err
		}
		if row.ID == "" {
			if expectedRevision != 0 {
				return ErrWorldClueConflict
			}
			row = model.WorldClueAccessModel{WorldID: worldID, ClueID: clueID, UserID: userID, AccessOverride: model.WorldClueAccessInherit, PrivateContentFormat: format, PrivateContent: content, PrivateContentText: text, PrivateRevision: 1, UpdatedBy: actorID}
			result := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "world_id"}, {Name: "clue_id"}, {Name: "user_id"}},
				DoNothing: true,
			}).Create(&row)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrWorldClueConflict
			}
			return nil
		}
		result := tx.Model(&model.WorldClueAccessModel{}).
			Where("world_id = ? AND clue_id = ? AND user_id = ? AND private_revision = ?", worldID, clueID, userID, expectedRevision).
			Updates(map[string]any{"private_content_format": format, "private_content": content, "private_content_text": text, "private_revision": gorm.Expr("private_revision + 1"), "updated_by": actorID, "updated_at": time.Now()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrWorldClueConflict
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return WorldClueGetPrivate(worldID, clueID, actorID, userID)
}

func WorldClueUpdateUserState(worldID, clueID, actorID string, input WorldClueUserStateInput) (*protocol.WorldClueUserState, error) {
	db := model.GetDB()
	if _, err := WorldClueGet(worldID, clueID, actorID); err != nil {
		return nil, err
	}
	updates := map[string]any{"updated_at": time.Now()}
	if input.PersonalFolderID != nil {
		folderID := strings.TrimSpace(*input.PersonalFolderID)
		if err := validatePersonalWorldClueFolder(db, worldID, actorID, folderID); err != nil {
			return nil, err
		}
		updates["personal_folder_id"] = folderID
	}
	if input.PersonalOrder != nil {
		updates["personal_order"] = *input.PersonalOrder
	}
	if input.Favorite != nil {
		updates["favorite"] = *input.Favorite
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := ensureWorldClueUserState(tx, worldID, clueID, actorID); err != nil {
			return err
		}
		return tx.Model(&model.WorldClueUserStateModel{}).Where("world_id = ? AND clue_id = ? AND user_id = ?", worldID, clueID, actorID).Updates(updates).Error
	})
	if err != nil {
		return nil, err
	}
	state, err := getWorldClueState(db, worldID, clueID, actorID)
	if err != nil {
		return nil, err
	}
	return worldClueStateDTO(state), nil
}

func normalizeWorldClueOrderedIDs(ids []string) ([]string, error) {
	if len(ids) > 1000 {
		return nil, fmt.Errorf("%w: too many ordered items", ErrWorldClueInvalid)
	}
	result := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			return nil, fmt.Errorf("%w: empty ordered item", ErrWorldClueInvalid)
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("%w: duplicate ordered item", ErrWorldClueInvalid)
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}

func sameWorldClueIDSet(expected []string, actual []string) bool {
	if len(expected) != len(actual) {
		return false
	}
	set := make(map[string]struct{}, len(expected))
	for _, id := range expected {
		set[id] = struct{}{}
	}
	for _, id := range actual {
		if _, ok := set[id]; !ok {
			return false
		}
	}
	return true
}

func WorldClueReorder(worldID, actorID string, input WorldClueReorderInput) error {
	orderedIDs, err := normalizeWorldClueOrderedIDs(input.OrderedIDs)
	if err != nil {
		return err
	}
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return err
	}
	if role == "" || !oneOf(input.Scope, model.WorldClueFolderShared, model.WorldClueFolderPersonal) {
		return ErrWorldClueDenied
	}
	folderID := strings.TrimSpace(input.FolderID)
	if input.Scope == model.WorldClueFolderShared {
		if !worldClueIsAdminRole(role) {
			return ErrWorldClueDenied
		}
		if err := validateWorldClueFolderForClue(db, worldID, folderID); err != nil {
			return err
		}
		var siblingIDs []string
		if err := db.Model(&model.WorldClueModel{}).
			Where("world_id = ? AND shared_folder_id = ? AND status <> ?", worldID, folderID, model.WorldClueStatusArchived).
			Pluck("id", &siblingIDs).Error; err != nil {
			return err
		}
		if !sameWorldClueIDSet(orderedIDs, siblingIDs) {
			return fmt.Errorf("%w: clue order must contain all siblings", ErrWorldClueInvalid)
		}
		return db.Transaction(func(tx *gorm.DB) error {
			for index, id := range orderedIDs {
				if err := tx.Model(&model.WorldClueModel{}).Where("world_id = ? AND id = ? AND shared_folder_id = ?", worldID, id, folderID).Update("order_index", index).Error; err != nil {
					return err
				}
			}
			return nil
		})
	}
	if err := validatePersonalWorldClueFolder(db, worldID, actorID, folderID); err != nil {
		return err
	}
	visible, err := WorldClueList(worldID, actorID, "")
	if err != nil {
		return err
	}
	siblingIDs := make([]string, 0, len(visible))
	for _, clue := range visible {
		if (clue.UserState != nil && clue.UserState.PersonalFolderID == folderID) ||
			(folderID == "" && (clue.UserState == nil || clue.UserState.PersonalFolderID == "")) {
			siblingIDs = append(siblingIDs, clue.ID)
		}
	}
	if !sameWorldClueIDSet(orderedIDs, siblingIDs) {
		return fmt.Errorf("%w: clue order must contain all visible siblings", ErrWorldClueInvalid)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for index, id := range orderedIDs {
			if err := ensureWorldClueUserState(tx, worldID, id, actorID); err != nil {
				return err
			}
			if err := tx.Model(&model.WorldClueUserStateModel{}).
				Where("world_id = ? AND clue_id = ? AND user_id = ?", worldID, id, actorID).
				Updates(map[string]any{"personal_order": index, "updated_at": time.Now()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func WorldClueResolve(worldID, actorID string, clueIDs []string) (map[string]protocol.WorldClueResolveItem, error) {
	result := make(map[string]protocol.WorldClueResolveItem)
	unique := make([]string, 0, len(clueIDs))
	seen := map[string]struct{}{}
	for _, id := range clueIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		result[id] = protocol.WorldClueResolveItem{Accessible: false}
		if _, ok := seen[id]; !ok && len(unique) < 100 {
			seen[id] = struct{}{}
			unique = append(unique, id)
		}
	}
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return nil, err
	}
	if role == "" || len(unique) == 0 {
		return result, nil
	}
	var clues []model.WorldClueModel
	if err := db.Where("world_id = ? AND id IN ? AND status <> ?", worldID, unique, model.WorldClueStatusArchived).Find(&clues).Error; err != nil {
		return nil, err
	}
	var accessRows []model.WorldClueAccessModel
	if err := db.Where("world_id = ? AND user_id = ? AND clue_id IN ?", worldID, actorID, unique).Find(&accessRows).Error; err != nil {
		return nil, err
	}
	accessByClue := make(map[string]*model.WorldClueAccessModel, len(accessRows))
	for i := range accessRows {
		accessByClue[accessRows[i].ClueID] = &accessRows[i]
	}
	for i := range clues {
		clue := &clues[i]
		row := accessByClue[clue.ID]
		override := model.WorldClueAccessInherit
		if row != nil {
			override = row.AccessOverride
		}
		access := effectiveWorldClueAccess(role, clue.DefaultAccess, override)
		if !canViewWorldClue(clue, role, access) {
			continue
		}
		excerpt := strings.TrimSpace(clue.ContentText)
		if len([]rune(excerpt)) > 180 {
			excerpt = string([]rune(excerpt)[:180])
		}
		embedDomain := ""
		if parsed, err := url.Parse(clue.EmbedURL); err == nil {
			embedDomain = parsed.Hostname()
		}
		hasPrivate := row != nil && clue.Status == model.WorldClueStatusPublished && access != model.WorldClueAccessNone && strings.TrimSpace(row.PrivateContent) != ""
		mediaType := ""
		if clue.Kind == model.WorldClueKindImage {
			mediaType = decodeWorldCluePresentation(clue.PresentationJSON).MediaType
		}
		result[clue.ID] = protocol.WorldClueResolveItem{
			Accessible: true, EffectiveAccess: access, Title: clue.Title, Kind: clue.Kind,
			MediaType:             mediaType,
			ThumbnailAttachmentID: clue.ImageAttachmentID, ThumbnailURL: clue.ImageURL,
			Excerpt: excerpt, EmbedDomain: embedDomain, Revision: clue.Revision,
			UpdatedAt: clue.UpdatedAt.UnixMilli(), HasPrivateContent: hasPrivate,
		}
	}
	return result, nil
}

func WorldClueCanAccessAttachment(worldID, attachmentID, actorID string) (bool, error) {
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil || role == "" {
		return false, err
	}
	var clues []model.WorldClueModel
	if err := db.Where("world_id = ? AND image_attachment_id = ? AND status <> ?", worldID, attachmentID, model.WorldClueStatusArchived).Find(&clues).Error; err != nil {
		return false, err
	}
	if worldClueIsAdminRole(role) && len(clues) > 0 {
		return true, nil
	}
	var accessRows []model.WorldClueAccessModel
	if err := db.Where("world_id = ? AND user_id = ?", worldID, actorID).Find(&accessRows).Error; err != nil {
		return false, err
	}
	accessByClue := make(map[string]string, len(accessRows))
	for _, row := range accessRows {
		accessByClue[row.ClueID] = row.AccessOverride
	}
	for i := range clues {
		access := effectiveWorldClueAccess(role, clues[i].DefaultAccess, accessByClue[clues[i].ID])
		if canViewWorldClue(&clues[i], role, access) {
			return true, nil
		}
	}
	return false, nil
}

func validateWorldClueFolderForClue(tx *gorm.DB, worldID, folderID string) error {
	if folderID == "" {
		return nil
	}
	var folder model.WorldClueFolderModel
	if err := tx.Where("world_id = ? AND id = ? AND scope = ? AND is_deleted = ?", worldID, folderID, model.WorldClueFolderShared, false).Limit(1).Find(&folder).Error; err != nil {
		return err
	}
	if folder.ID == "" {
		return fmt.Errorf("%w: shared folder not found", ErrWorldClueInvalid)
	}
	return nil
}

func validatePersonalWorldClueFolder(tx *gorm.DB, worldID, userID, folderID string) error {
	if folderID == "" {
		return nil
	}
	var folder model.WorldClueFolderModel
	if err := tx.Where("world_id = ? AND id = ? AND scope = ? AND owner_user_id = ? AND is_deleted = ?", worldID, folderID, model.WorldClueFolderPersonal, userID, false).Limit(1).Find(&folder).Error; err != nil {
		return err
	}
	if folder.ID == "" {
		return fmt.Errorf("%w: personal folder not found", ErrWorldClueInvalid)
	}
	return nil
}

func validateWorldClueFolderParent(tx *gorm.DB, worldID, scope, ownerID, parentID, currentID string) error {
	if parentID == "" {
		return nil
	}
	depth := 1
	visited := map[string]struct{}{currentID: {}}
	for parentID != "" {
		if _, ok := visited[parentID]; ok {
			return ErrWorldClueFolderCycle
		}
		visited[parentID] = struct{}{}
		var parent model.WorldClueFolderModel
		query := tx.Where("world_id = ? AND id = ? AND scope = ? AND is_deleted = ?", worldID, parentID, scope, false)
		if scope == model.WorldClueFolderPersonal {
			query = query.Where("owner_user_id = ?", ownerID)
		}
		if err := query.Limit(1).Find(&parent).Error; err != nil {
			return err
		}
		if parent.ID == "" {
			return fmt.Errorf("%w: folder parent not found", ErrWorldClueInvalid)
		}
		depth++
		if depth > 5 {
			return fmt.Errorf("%w: folder depth exceeds 5", ErrWorldClueInvalid)
		}
		parentID = parent.ParentID
	}
	return nil
}

func WorldClueFolderCreate(worldID, actorID string, input WorldClueFolderInput) (*protocol.WorldClueFolder, error) {
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return nil, err
	}
	if role == "" || !oneOf(input.Scope, model.WorldClueFolderShared, model.WorldClueFolderPersonal) {
		return nil, ErrWorldClueDenied
	}
	ownerID := ""
	if input.Scope == model.WorldClueFolderShared {
		if !worldClueIsAdminRole(role) {
			return nil, ErrWorldClueDenied
		}
	} else {
		ownerID = actorID
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return nil, fmt.Errorf("%w: folder name is required", ErrWorldClueInvalid)
	}
	if err := validateWorldClueFolderParent(db, worldID, input.Scope, ownerID, strings.TrimSpace(input.ParentID), ""); err != nil {
		return nil, err
	}
	folder := &model.WorldClueFolderModel{WorldID: worldID, Scope: input.Scope, OwnerUserID: ownerID, ParentID: strings.TrimSpace(input.ParentID), Name: input.Name, Color: strings.TrimSpace(input.Color), OrderIndex: input.OrderIndex}
	if err := db.Create(folder).Error; err != nil {
		return nil, err
	}
	dto := worldClueFolderDTO(folder)
	return &dto, nil
}

func worldClueFolderDTO(folder *model.WorldClueFolderModel) protocol.WorldClueFolder {
	return protocol.WorldClueFolder{ID: folder.ID, WorldID: folder.WorldID, Scope: folder.Scope, OwnerUserID: folder.OwnerUserID, ParentID: folder.ParentID, Name: folder.Name, Color: folder.Color, OrderIndex: folder.OrderIndex, CreatedAt: folder.CreatedAt.UnixMilli(), UpdatedAt: folder.UpdatedAt.UnixMilli()}
}

func WorldClueFolderList(worldID, actorID string) ([]protocol.WorldClueFolder, error) {
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return nil, err
	}
	if role == "" {
		return nil, ErrWorldClueDenied
	}
	var folders []model.WorldClueFolderModel
	if err := db.Where("world_id = ? AND is_deleted = ? AND (scope = ? OR (scope = ? AND owner_user_id = ?))", worldID, false, model.WorldClueFolderShared, model.WorldClueFolderPersonal, actorID).
		Order("scope ASC, order_index ASC, id ASC").Find(&folders).Error; err != nil {
		return nil, err
	}
	allowedShared := map[string]struct{}{}
	if worldClueIsAdminRole(role) {
		for _, folder := range folders {
			if folder.Scope == model.WorldClueFolderShared {
				allowedShared[folder.ID] = struct{}{}
			}
		}
	} else {
		clues, err := WorldClueList(worldID, actorID, "")
		if err != nil {
			return nil, err
		}
		parentByID := map[string]string{}
		for _, folder := range folders {
			if folder.Scope == model.WorldClueFolderShared {
				parentByID[folder.ID] = folder.ParentID
			}
		}
		for _, clue := range clues {
			id := clue.SharedFolderID
			for id != "" {
				allowedShared[id] = struct{}{}
				id = parentByID[id]
			}
		}
	}
	result := make([]protocol.WorldClueFolder, 0, len(folders))
	for i := range folders {
		if folders[i].Scope == model.WorldClueFolderShared {
			if _, ok := allowedShared[folders[i].ID]; !ok {
				continue
			}
		}
		result = append(result, worldClueFolderDTO(&folders[i]))
	}
	return result, nil
}

func WorldClueFolderReorder(worldID, actorID string, input WorldClueReorderInput) error {
	orderedIDs, err := normalizeWorldClueOrderedIDs(input.OrderedIDs)
	if err != nil {
		return err
	}
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return err
	}
	if role == "" || !oneOf(input.Scope, model.WorldClueFolderShared, model.WorldClueFolderPersonal) {
		return ErrWorldClueDenied
	}
	ownerID := ""
	if input.Scope == model.WorldClueFolderShared {
		if !worldClueIsAdminRole(role) {
			return ErrWorldClueDenied
		}
	} else {
		ownerID = actorID
	}
	parentID := strings.TrimSpace(input.FolderID)
	if err := validateWorldClueFolderParent(db, worldID, input.Scope, ownerID, parentID, ""); err != nil {
		return err
	}
	query := db.Model(&model.WorldClueFolderModel{}).
		Where("world_id = ? AND scope = ? AND parent_id = ? AND is_deleted = ?", worldID, input.Scope, parentID, false)
	if input.Scope == model.WorldClueFolderPersonal {
		query = query.Where("owner_user_id = ?", actorID)
	}
	var siblingIDs []string
	if err := query.Pluck("id", &siblingIDs).Error; err != nil {
		return err
	}
	if !sameWorldClueIDSet(orderedIDs, siblingIDs) {
		return fmt.Errorf("%w: folder order must contain all siblings", ErrWorldClueInvalid)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for index, id := range orderedIDs {
			updates := tx.Model(&model.WorldClueFolderModel{}).
				Where("world_id = ? AND id = ? AND scope = ? AND parent_id = ? AND is_deleted = ?", worldID, id, input.Scope, parentID, false)
			if input.Scope == model.WorldClueFolderPersonal {
				updates = updates.Where("owner_user_id = ?", actorID)
			}
			if err := updates.Updates(map[string]any{"order_index": index, "updated_at": time.Now()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func loadManageableWorldClueFolder(tx *gorm.DB, worldID, folderID, actorID string) (*model.WorldClueFolderModel, error) {
	role, err := worldClueRole(tx, worldID, actorID)
	if err != nil {
		return nil, err
	}
	var folder model.WorldClueFolderModel
	if err := tx.Where("world_id = ? AND id = ? AND is_deleted = ?", worldID, folderID, false).Limit(1).Find(&folder).Error; err != nil {
		return nil, err
	}
	if folder.ID == "" {
		return nil, ErrWorldClueNotFound
	}
	if folder.Scope == model.WorldClueFolderShared {
		if !worldClueIsAdminRole(role) {
			return nil, ErrWorldClueDenied
		}
	} else if folder.Scope != model.WorldClueFolderPersonal || folder.OwnerUserID != actorID {
		return nil, ErrWorldClueDenied
	}
	return &folder, nil
}

func WorldClueFolderUpdate(worldID, folderID, actorID string, input WorldClueFolderUpdateInput) (*protocol.WorldClueFolder, error) {
	db := model.GetDB()
	folder, err := loadManageableWorldClueFolder(db, worldID, folderID, actorID)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{"updated_at": time.Now()}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: folder name is required", ErrWorldClueInvalid)
		}
		updates["name"] = name
	}
	if input.Color != nil {
		updates["color"] = strings.TrimSpace(*input.Color)
	}
	if input.OrderIndex != nil {
		updates["order_index"] = *input.OrderIndex
	}
	if input.ParentID != nil {
		parentID := strings.TrimSpace(*input.ParentID)
		if err := validateWorldClueFolderParent(db, worldID, folder.Scope, folder.OwnerUserID, parentID, folder.ID); err != nil {
			return nil, err
		}
		updates["parent_id"] = parentID
	}
	if err := db.Model(&model.WorldClueFolderModel{}).Where("world_id = ? AND id = ? AND is_deleted = ?", worldID, folderID, false).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := db.Where("world_id = ? AND id = ?", worldID, folderID).Limit(1).Find(folder).Error; err != nil {
		return nil, err
	}
	dto := worldClueFolderDTO(folder)
	return &dto, nil
}

func WorldClueFolderDelete(worldID, folderID, actorID string) error {
	db := model.GetDB()
	return db.Transaction(func(tx *gorm.DB) error {
		folder, err := loadManageableWorldClueFolder(tx, worldID, folderID, actorID)
		if err != nil {
			return err
		}
		now := time.Now()
		if folder.Scope == model.WorldClueFolderShared {
			if err := tx.Model(&model.WorldClueFolderModel{}).Where("world_id = ? AND parent_id = ? AND scope = ? AND is_deleted = ?", worldID, folderID, folder.Scope, false).Updates(map[string]any{"parent_id": folder.ParentID, "updated_at": now}).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.WorldClueModel{}).Where("world_id = ? AND shared_folder_id = ?", worldID, folderID).Updates(map[string]any{"shared_folder_id": folder.ParentID, "updated_at": now}).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&model.WorldClueFolderModel{}).Where("world_id = ? AND parent_id = ? AND scope = ? AND owner_user_id = ? AND is_deleted = ?", worldID, folderID, folder.Scope, actorID, false).Updates(map[string]any{"parent_id": folder.ParentID, "updated_at": now}).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.WorldClueUserStateModel{}).Where("world_id = ? AND user_id = ? AND personal_folder_id = ?", worldID, actorID, folderID).Updates(map[string]any{"personal_folder_id": folder.ParentID, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&model.WorldClueFolderModel{}).Where("world_id = ? AND id = ?", worldID, folderID).Updates(map[string]any{"is_deleted": true, "deleted_at": now, "updated_at": now}).Error
	})
}

func cleanupWorldCluesForMember(tx *gorm.DB, worldID, userID string) error {
	if err := tx.Unscoped().Where("world_id = ? AND user_id = ?", worldID, userID).Delete(&model.WorldClueRosterMemberModel{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("world_id = ? AND user_id = ?", worldID, userID).Delete(&model.WorldClueAccessModel{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("world_id = ? AND user_id = ?", worldID, userID).Delete(&model.WorldClueUserStateModel{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("world_id = ? AND user_id = ?", worldID, userID).Delete(&model.WorldClueEditLockModel{}).Error; err != nil {
		return err
	}
	return tx.Unscoped().Where("world_id = ? AND scope = ? AND owner_user_id = ?", worldID, model.WorldClueFolderPersonal, userID).Delete(&model.WorldClueFolderModel{}).Error
}

func archiveWorldClues(tx *gorm.DB, worldID string) error {
	now := time.Now()
	if err := tx.Model(&model.WorldClueModel{}).Where("world_id = ?", worldID).Updates(map[string]any{"status": model.WorldClueStatusArchived, "updated_at": now}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("world_id = ?", worldID).Delete(&model.WorldClueAccessModel{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("world_id = ?", worldID).Delete(&model.WorldClueUserStateModel{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("world_id = ?", worldID).Delete(&model.WorldClueRosterMemberModel{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("world_id = ?", worldID).Delete(&model.WorldClueEditLockModel{}).Error; err != nil {
		return err
	}
	return tx.Model(&model.WorldClueFolderModel{}).Where("world_id = ?", worldID).Updates(map[string]any{"is_deleted": true, "deleted_at": now, "updated_at": now}).Error
}
