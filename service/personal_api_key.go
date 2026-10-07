package service

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"sealchat/model"
	"sealchat/utils"
)

var (
	ErrPersonalKeyInvalid  = errors.New("invalid personal API key")
	ErrMCPDisabled         = errors.New("MCP disabled")
	ErrMCPScopeDenied      = errors.New("MCP scope denied")
	ErrPersonalKeyInput    = errors.New("invalid personal API key input")
	ErrPersonalKeyLimit    = errors.New("personal API key limit reached")
	ErrPersonalKeyConflict = errors.New("personal API key changed or revoked")
)

// GORM error/slow-query logging must never print secret digests from inserts or
// conditional rotation/usage updates. Callers report only sanitized errors.
func personalKeyDB() *gorm.DB {
	db := model.GetDB()
	return db.Session(&gorm.Session{Logger: db.Logger.LogMode(logger.Silent)})
}

type PersonalKeyInput struct {
	Name         string     `json:"name"`
	Scopes       []string   `json:"scopes"`
	NeverExpires bool       `json:"neverExpires"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
}
type PersonalKeyPatch struct {
	Name   *string   `json:"name,omitempty"`
	Scopes *[]string `json:"scopes,omitempty"`
}
type MCPActor struct {
	Key            model.PersonalAPIKeyModel
	User           *model.UserModel
	GrantedScopes  []string
	Scopes         []string
	CredentialID   string
	CredentialType string
}

func (a *MCPActor) Allows(scopes ...string) bool {
	if a == nil {
		return false
	}
	for _, s := range scopes {
		found := false
		for _, granted := range a.Scopes {
			if s == granted {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func validatePersonalKeyName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return "", ErrPersonalKeyInput
	}
	return name, nil
}
func normalizePersonalKeyScopes(scopes []string) ([]string, error) {
	known := map[string]bool{}
	for _, s := range utils.MCPScopeCatalog {
		known[s.ID] = true
	}
	seen := map[string]bool{}
	ret := []string{}
	for _, s := range scopes {
		if !known[s] {
			return nil, ErrPersonalKeyInput
		}
		if !seen[s] {
			ret = append(ret, s)
			seen[s] = true
		}
	}
	return ret, nil
}
func randomPersonalKey(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func personalKeyHash(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

func ListPersonalAPIKeys(userID string) ([]model.PersonalAPIKeyModel, error) {
	items := []model.PersonalAPIKeyModel{}
	err := personalKeyDB().Where("user_id = ?", userID).Order("created_at DESC, id DESC").Find(&items).Error
	return items, err
}

func CreatePersonalAPIKey(userID string, input PersonalKeyInput, cfg utils.MCPConfig) (*model.PersonalAPIKeyModel, string, error) {
	if !cfg.Enabled {
		return nil, "", ErrMCPDisabled
	}
	name, err := validatePersonalKeyName(input.Name)
	if err != nil {
		return nil, "", err
	}
	scopes, err := normalizePersonalKeyScopes(input.Scopes)
	if err != nil {
		return nil, "", err
	}
	allowed := &MCPActor{Scopes: cfg.AllowedScopes()}
	if !allowed.Allows(scopes...) {
		return nil, "", ErrMCPScopeDenied
	}
	now := time.Now().UTC()
	expiry := input.ExpiresAt
	if input.NeverExpires && expiry != nil {
		return nil, "", ErrPersonalKeyInput
	}
	if !input.NeverExpires && expiry == nil {
		v := now.Add(90 * 24 * time.Hour)
		expiry = &v
	}
	if expiry != nil && !expiry.After(now) {
		return nil, "", ErrPersonalKeyInput
	}
	public, err := randomPersonalKey(12)
	if err != nil {
		return nil, "", err
	}
	secret, err := randomPersonalKey(32)
	if err != nil {
		return nil, "", err
	}
	key := &model.PersonalAPIKeyModel{ID: utils.NewID(), UserID: userID, Name: name, PublicID: public, SecretHash: personalKeyHash(secret), Tail: secret[len(secret)-8:], Scopes: scopes, CreatedAt: now, UpdatedAt: now, ExpiresAt: expiry}
	err = personalKeyDB().Transaction(func(tx *gorm.DB) error {
		// Serialize quota decisions on the existing user row, also across processes.
		var user model.UserModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND disabled = ? AND deleted_at IS NULL", userID, false).First(&user).Error; err != nil {
			return ErrPersonalKeyInvalid
		}
		// SQLite ignores FOR UPDATE; acquiring a write lock before counting closes that race.
		if user.IsBot {
			return ErrPersonalKeyInvalid
		}
		if err := tx.Model(&model.UserModel{}).Where("id = ? AND disabled = ?", userID, false).UpdateColumn("username", gorm.Expr("username")).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&model.PersonalAPIKeyModel{}).Where("user_id = ? AND revoked_at IS NULL", userID).Count(&count).Error; err != nil {
			return err
		}
		if count >= 10 {
			return ErrPersonalKeyLimit
		}
		return tx.Create(key).Error
	})
	if err != nil {
		return nil, "", err
	}
	return key, "sc_mcp_" + public + "." + secret, nil
}

func UpdatePersonalAPIKey(userID, keyID string, p PersonalKeyPatch, cfg utils.MCPConfig) error {
	if !cfg.Enabled {
		return ErrMCPDisabled
	}
	updates := map[string]any{"updated_at": time.Now().UTC()}
	query := personalKeyDB().Model(&model.PersonalAPIKeyModel{}).Where("id = ? AND user_id = ? AND revoked_at IS NULL", keyID, userID)
	if p.Name != nil {
		n, err := validatePersonalKeyName(*p.Name)
		if err != nil {
			return err
		}
		updates["name"] = n
	}
	if p.Scopes != nil {
		s, err := normalizePersonalKeyScopes(*p.Scopes)
		if err != nil {
			return err
		}
		var stored struct{ Scopes string }
		if err := query.Session(&gorm.Session{}).Select("scopes").Take(&stored).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPersonalKeyConflict
			}
			return err
		}
		var oldScopes []string
		if err := json.Unmarshal([]byte(stored.Scopes), &oldScopes); err != nil {
			return err
		}
		previous := &MCPActor{Scopes: oldScopes}
		allowed := &MCPActor{Scopes: cfg.AllowedScopes()}
		for _, scope := range s {
			if !previous.Allows(scope) && !allowed.Allows(scope) {
				return ErrMCPScopeDenied
			}
		}
		raw, err := json.Marshal(s)
		if err != nil {
			return err
		}
		updates["scopes"] = string(raw)
		// Do not restore scopes removed by another update after this validation.
		query = query.Where("scopes = ?", stored.Scopes)
	}
	r := query.Updates(updates)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return ErrPersonalKeyConflict
	}
	return nil
}
func RevokePersonalAPIKey(userID, keyID string) error {
	r := personalKeyDB().Model(&model.PersonalAPIKeyModel{}).Where("id = ? AND user_id = ? AND revoked_at IS NULL", keyID, userID).Updates(map[string]any{"revoked_at": time.Now().UTC(), "updated_at": time.Now().UTC()})
	return r.Error
}
func RotatePersonalAPIKey(userID, keyID string, cfg utils.MCPConfig) (*model.PersonalAPIKeyModel, string, error) {
	if !cfg.Enabled {
		return nil, "", ErrMCPDisabled
	}
	var key model.PersonalAPIKeyModel
	if err := personalKeyDB().Where("id = ? AND user_id = ? AND revoked_at IS NULL", keyID, userID).First(&key).Error; err != nil {
		return nil, "", ErrPersonalKeyConflict
	}
	secret, err := randomPersonalKey(32)
	if err != nil {
		return nil, "", err
	}
	oldHash := key.SecretHash
	key.SecretHash = personalKeyHash(secret)
	key.Tail = secret[len(secret)-8:]
	key.UpdatedAt = time.Now().UTC()
	r := personalKeyDB().Model(&model.PersonalAPIKeyModel{}).Where("id = ? AND user_id = ? AND revoked_at IS NULL AND secret_hash = ?", keyID, userID, oldHash).Updates(map[string]any{"secret_hash": key.SecretHash, "tail": key.Tail, "updated_at": key.UpdatedAt})
	if r.Error != nil {
		return nil, "", r.Error
	}
	if r.RowsAffected != 1 {
		return nil, "", ErrPersonalKeyConflict
	}
	return &key, "sc_mcp_" + key.PublicID + "." + secret, nil
}

func AuthenticatePersonalAPIKey(token string, cfg utils.MCPConfig) (*MCPActor, error) {
	if !cfg.Enabled {
		return nil, ErrMCPDisabled
	}
	if !strings.HasPrefix(token, "sc_mcp_") {
		return nil, ErrPersonalKeyInvalid
	}
	parts := strings.Split(strings.TrimPrefix(token, "sc_mcp_"), ".")
	if len(parts) != 2 || len(parts[0]) != 16 || len(parts[1]) != 43 {
		return nil, ErrPersonalKeyInvalid
	}
	for _, p := range parts {
		if _, err := base64.RawURLEncoding.DecodeString(p); err != nil {
			return nil, ErrPersonalKeyInvalid
		}
	}
	var key model.PersonalAPIKeyModel
	if err := personalKeyDB().Where("public_id = ? AND revoked_at IS NULL", parts[0]).First(&key).Error; err != nil {
		return nil, ErrPersonalKeyInvalid
	}
	if key.ExpiresAt != nil && !key.ExpiresAt.After(time.Now()) {
		return nil, ErrPersonalKeyInvalid
	}
	if subtle.ConstantTimeCompare([]byte(personalKeyHash(parts[1])), []byte(key.SecretHash)) != 1 {
		return nil, ErrPersonalKeyInvalid
	}
	var user model.UserModel
	if err := personalKeyDB().Where("id = ? AND disabled = ? AND deleted_at IS NULL", key.UserID, false).First(&user).Error; err != nil || user.IsBot {
		return nil, ErrPersonalKeyInvalid
	}
	allowed := &MCPActor{Scopes: cfg.AllowedScopes()}
	effective := []string{}
	for _, s := range key.Scopes {
		if allowed.Allows(s) {
			effective = append(effective, s)
		}
	}
	return &MCPActor{Key: key, User: &user, GrantedScopes: append([]string{}, key.Scopes...), Scopes: effective, CredentialID: key.ID, CredentialType: "pat"}, nil
}

func TouchPersonalAPIKey(key *model.PersonalAPIKeyModel) {
	now := time.Now().UTC()
	personalKeyDB().Model(&model.PersonalAPIKeyModel{}).Where("id = ? AND revoked_at IS NULL AND secret_hash = ? AND (last_used_at IS NULL OR last_used_at < ?)", key.ID, key.SecretHash, now.Add(-time.Minute)).UpdateColumn("last_used_at", now)
}
