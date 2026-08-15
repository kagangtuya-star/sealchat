package service

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"sealchat/model"
)

var (
	ErrWorldAgentAccessInvalid = errors.New("world agent access link invalid")
	ErrWorldAgentAccessDenied  = errors.New("world agent access denied")
	ErrWorldAgentTokenConflict = errors.New("world agent access token conflict")
)

const (
	worldAgentAccessTokenPrefix  = "agt_"
	worldAgentPublicIDBytes      = 12
	worldAgentSecretBytes        = 32
	worldAgentTouchInterval      = 5 * time.Minute
	WorldAgentRateLimitPerMinute = 120
)

type worldAgentRateBucket struct {
	windowMinute int64
	count        int
}

var worldAgentRateState = struct {
	sync.Mutex
	buckets map[string]worldAgentRateBucket
}{buckets: make(map[string]worldAgentRateBucket)}

// WorldAgentAccessState is the management-facing state of a world's Agent link.
// Token contains the complete bearer credential only when it was created or
// rotated by the current request. The database stores only a SHA-256 digest of
// the secret, so a later GET cannot reveal the existing credential.
type WorldAgentAccessState struct {
	WorldID       string     `json:"worldId"`
	PublicID      string     `json:"publicId,omitempty"`
	HasToken      bool       `json:"hasToken"`
	Token         string     `json:"token,omitempty"`
	TokenTail     string     `json:"tokenTail,omitempty"`
	Enabled       bool       `json:"enabled"`
	RotatedAt     *time.Time `json:"rotatedAt,omitempty"`
	LastAccessAt  *time.Time `json:"lastAccessAt,omitempty"`
	ProfileUserID string     `json:"-"`
}

func generateWorldAgentRandomString(size int) (string, error) {
	if size <= 0 {
		return "", fmt.Errorf("random size must be positive")
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate agent access token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashWorldAgentSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func compareWorldAgentSecret(secret, expectedHash string) bool {
	actual := hashWorldAgentSecret(secret)
	expectedHash = strings.ToLower(strings.TrimSpace(expectedHash))
	if len(actual) != len(expectedHash) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expectedHash)) == 1
}

func formatWorldAgentAccessToken(publicID, secret string) string {
	return worldAgentAccessTokenPrefix + strings.TrimSpace(publicID) + "." + strings.TrimSpace(secret)
}

func parseWorldAgentAccessToken(token string) (string, string, bool) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, worldAgentAccessTokenPrefix) {
		return "", "", false
	}
	body := strings.TrimPrefix(token, worldAgentAccessTokenPrefix)
	parts := strings.SplitN(body, ".", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	publicID := strings.TrimSpace(parts[0])
	secret := strings.TrimSpace(parts[1])
	if len(publicID) != base64.RawURLEncoding.EncodedLen(worldAgentPublicIDBytes) ||
		len(secret) != base64.RawURLEncoding.EncodedLen(worldAgentSecretBytes) ||
		!isWorldAgentTokenSegment(publicID) || !isWorldAgentTokenSegment(secret) {
		return "", "", false
	}
	return publicID, secret, true
}

func isWorldAgentTokenSegment(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '-' && ch != '_' {
			return false
		}
	}
	return true
}

func worldAgentTokenTail(secret string) string {
	secret = strings.TrimSpace(secret)
	if len(secret) <= 8 {
		return secret
	}
	return secret[len(secret)-8:]
}

func canManageWorldAgentAccess(worldID, actorID string) bool {
	worldID = strings.TrimSpace(worldID)
	actorID = strings.TrimSpace(actorID)
	if worldID == "" || actorID == "" {
		return false
	}
	role := ResolveMemberRoleForProtocol(actorID, "", worldID)
	return role == model.WorldRoleOwner || role == model.WorldRoleAdmin
}

func WorldAgentAccessGet(worldID, actorID string) (*WorldAgentAccessState, error) {
	worldID = strings.TrimSpace(worldID)
	actorID = strings.TrimSpace(actorID)
	if worldID == "" {
		return nil, ErrWorldAgentAccessInvalid
	}
	world, err := GetWorldByID(worldID)
	if err != nil {
		return nil, err
	}
	if world == nil || strings.TrimSpace(world.ID) == "" {
		return nil, ErrWorldAgentAccessInvalid
	}
	if !canManageWorldAgentAccess(worldID, actorID) {
		return nil, ErrWorldAgentAccessDenied
	}
	return buildWorldAgentAccessState(world, ""), nil
}

func WorldAgentAccessUpdate(worldID, actorID string, enabled, rotate bool) (*WorldAgentAccessState, error) {
	worldID = strings.TrimSpace(worldID)
	actorID = strings.TrimSpace(actorID)
	if worldID == "" {
		return nil, ErrWorldAgentAccessInvalid
	}
	world, err := GetWorldByID(worldID)
	if err != nil {
		return nil, err
	}
	if world == nil || strings.TrimSpace(world.ID) == "" {
		return nil, ErrWorldAgentAccessInvalid
	}
	if !canManageWorldAgentAccess(worldID, actorID) {
		return nil, ErrWorldAgentAccessDenied
	}

	publicID := ""
	if world.AgentAccessPublicID != nil {
		publicID = strings.TrimSpace(*world.AgentAccessPublicID)
	}
	secretHash := strings.TrimSpace(world.AgentAccessSecretHash)
	// Saving a disabled, not-yet-created link should only persist the switch.
	// Credential generation is needed when the link is enabled or explicitly
	// rotated; otherwise a routine first save would create an unusable token.
	needsCredential := rotate || (enabled && (publicID == "" || secretHash == ""))

	// A normal enable/disable save must never write credential columns from a
	// stale WorldModel. Otherwise a concurrent rotation could be reverted by a
	// second administrator saving only the switch state.
	if !needsCredential {
		updates := map[string]any{"agent_access_enabled": enabled}
		if strings.TrimSpace(world.AgentAccessProfileUserID) == "" {
			updates["agent_access_profile_user_id"] = actorID
		}
		if err := model.GetDB().Model(&model.WorldModel{}).
			Where("id = ?", worldID).
			Updates(updates).Error; err != nil {
			return nil, err
		}
		updated, err := GetWorldByID(worldID)
		if err != nil {
			return nil, err
		}
		if updated == nil || strings.TrimSpace(updated.ID) == "" {
			return nil, ErrWorldAgentAccessInvalid
		}
		return buildWorldAgentAccessState(updated, ""), nil
	}

	publicID, plainToken, secretHash, tokenTail, err := generateUniqueWorldAgentCredential()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	updates := map[string]any{
		"agent_access_public_id":       publicID,
		"agent_access_secret_hash":     secretHash,
		"agent_access_token_tail":      tokenTail,
		"agent_access_enabled":         enabled,
		"agent_access_profile_user_id": actorID,
		"agent_access_rotated_at":      &now,
	}
	if err := model.GetDB().Model(&model.WorldModel{}).
		Where("id = ?", worldID).
		Updates(updates).Error; err != nil {
		return nil, err
	}

	updated, err := GetWorldByID(worldID)
	if err != nil {
		return nil, err
	}
	if updated == nil || strings.TrimSpace(updated.ID) == "" {
		return nil, ErrWorldAgentAccessInvalid
	}
	updatedPublicID := ""
	if updated.AgentAccessPublicID != nil {
		updatedPublicID = strings.TrimSpace(*updated.AgentAccessPublicID)
	}
	if updatedPublicID != publicID || strings.TrimSpace(updated.AgentAccessSecretHash) != secretHash {
		// Another rotation won the race. Do not return a credential that is
		// already invalid; the caller can retry and receive the winning state.
		return nil, ErrWorldAgentTokenConflict
	}
	return buildWorldAgentAccessState(updated, plainToken), nil
}

func generateUniqueWorldAgentCredential() (publicID, token, secretHash, tokenTail string, err error) {
	for attempt := 0; attempt < 8; attempt++ {
		publicID, err = generateWorldAgentRandomString(worldAgentPublicIDBytes)
		if err != nil {
			return "", "", "", "", err
		}
		var count int64
		if err = model.GetDB().Model(&model.WorldModel{}).
			Where("agent_access_public_id = ?", publicID).
			Count(&count).Error; err != nil {
			return "", "", "", "", err
		}
		if count != 0 {
			continue
		}
		secret, secretErr := generateWorldAgentRandomString(worldAgentSecretBytes)
		if secretErr != nil {
			return "", "", "", "", secretErr
		}
		return publicID, formatWorldAgentAccessToken(publicID, secret), hashWorldAgentSecret(secret), worldAgentTokenTail(secret), nil
	}
	return "", "", "", "", ErrWorldAgentTokenConflict
}

func buildWorldAgentAccessState(world *model.WorldModel, token string) *WorldAgentAccessState {
	if world == nil {
		return &WorldAgentAccessState{}
	}
	publicID := ""
	if world.AgentAccessPublicID != nil {
		publicID = strings.TrimSpace(*world.AgentAccessPublicID)
	}
	return &WorldAgentAccessState{
		WorldID:       strings.TrimSpace(world.ID),
		PublicID:      publicID,
		HasToken:      publicID != "" && strings.TrimSpace(world.AgentAccessSecretHash) != "",
		Token:         strings.TrimSpace(token),
		TokenTail:     strings.TrimSpace(world.AgentAccessTokenTail),
		Enabled:       world.AgentAccessEnabled,
		RotatedAt:     world.AgentAccessRotatedAt,
		LastAccessAt:  world.AgentAccessLastAccessAt,
		ProfileUserID: strings.TrimSpace(world.AgentAccessProfileUserID),
	}
}

func ResolveWorldAgentAccess(token string) (*model.WorldModel, error) {
	publicID, secret, ok := parseWorldAgentAccessToken(token)
	if !ok {
		return nil, ErrWorldAgentAccessInvalid
	}
	var world model.WorldModel
	if err := model.GetDB().
		Where("agent_access_public_id = ? AND agent_access_enabled = ? AND status = ?", publicID, true, "active").
		Limit(1).
		Find(&world).Error; err != nil {
		return nil, err
	}
	if strings.TrimSpace(world.ID) == "" || !compareWorldAgentSecret(secret, world.AgentAccessSecretHash) {
		return nil, ErrWorldAgentAccessInvalid
	}
	touchWorldAgentAccess(&world, time.Now().UTC())
	return &world, nil
}

// ConsumeWorldAgentAccessRateLimit applies a process-local fixed-window limit.
// It is intentionally independent of client IP because the URL itself is the
// bearer credential and multiple Agent workers can legitimately share it.
func ConsumeWorldAgentAccessRateLimit(world *model.WorldModel, now time.Time) (remaining int, retryAfterSeconds int, allowed bool) {
	if world == nil {
		return 0, 60, false
	}
	publicID := ""
	if world.AgentAccessPublicID != nil {
		publicID = strings.TrimSpace(*world.AgentAccessPublicID)
	}
	if publicID == "" {
		return 0, 60, false
	}
	now = now.UTC()
	windowMinute := now.Unix() / 60
	worldAgentRateState.Lock()
	defer worldAgentRateState.Unlock()
	bucket := worldAgentRateState.buckets[publicID]
	if bucket.windowMinute != windowMinute {
		bucket = worldAgentRateBucket{windowMinute: windowMinute}
	}
	if bucket.count >= WorldAgentRateLimitPerMinute {
		retry := 60 - int(now.Unix()%60)
		if retry < 1 {
			retry = 1
		}
		worldAgentRateState.buckets[publicID] = bucket
		return 0, retry, false
	}
	bucket.count++
	worldAgentRateState.buckets[publicID] = bucket
	if len(worldAgentRateState.buckets) > 4096 {
		for key, value := range worldAgentRateState.buckets {
			if value.windowMinute+2 < windowMinute {
				delete(worldAgentRateState.buckets, key)
			}
		}
	}
	return WorldAgentRateLimitPerMinute - bucket.count, 0, true
}

func touchWorldAgentAccess(world *model.WorldModel, now time.Time) {
	if world == nil || strings.TrimSpace(world.ID) == "" {
		return
	}
	if world.AgentAccessLastAccessAt != nil && now.Sub(world.AgentAccessLastAccessAt.UTC()) < worldAgentTouchInterval {
		return
	}
	if err := model.GetDB().Model(&model.WorldModel{}).
		Where("id = ?", world.ID).
		UpdateColumn("agent_access_last_access_at", now).Error; err == nil {
		world.AgentAccessLastAccessAt = &now
	}
}
