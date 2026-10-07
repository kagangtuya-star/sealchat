package service

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"sealchat/model"
	"sealchat/utils"
)

const (
	MCPChatGPTClientID    = "https://chatgpt.com/oauth/client.json"
	MCPChatGPTRedirectURI = "https://chatgpt.com/connector_platform_oauth_redirect"
	mcpOAuthRequestTTL    = 5 * time.Minute
	mcpOAuthCodeTTL       = 2 * time.Minute
	mcpOAuthAccessTTL     = time.Hour
	mcpOAuthRefreshTTL    = 30 * 24 * time.Hour
	mcpOAuthStoreLimit    = 4096
)

var ErrMCPOAuthExpired = errors.New("MCP OAuth request expired or consumed")

// MCPOAuthError contains only public protocol errors, never database errors or secrets.
type MCPOAuthError struct{ Code string }

func (e *MCPOAuthError) Error() string { return e.Code }
func oauthError(code string) error     { return &MCPOAuthError{Code: code} }

type MCPOAuthAuthorizationInput struct {
	ResponseType, ClientID, RedirectURI, Scope, State, CodeChallenge, CodeChallengeMethod, Resource string
}

type MCPOAuthAuthorizationRequest struct {
	UserID, ClientID, RedirectURI, Resource, Issuer, State, CodeChallenge string
	Scopes                                                                []string
	ExpiresAt                                                             time.Time
}

// Temporary requests/codes deliberately live in one process and expire on restart.
type MCPOAuthAuthorizationStore struct {
	mu      sync.Mutex
	pending map[string]MCPOAuthAuthorizationRequest
	codes   map[string]MCPOAuthAuthorizationRequest
	now     func() time.Time
	cimd    *mcpChatGPTCIMDResolver
}

func NewMCPOAuthAuthorizationStore() *MCPOAuthAuthorizationStore {
	return &MCPOAuthAuthorizationStore{pending: map[string]MCPOAuthAuthorizationRequest{}, codes: map[string]MCPOAuthAuthorizationRequest{}, now: time.Now, cimd: mcpChatGPTCIMD}
}

func (s *MCPOAuthAuthorizationStore) cleanupLocked() {
	now := s.now()
	for id, r := range s.pending {
		if !r.ExpiresAt.After(now) {
			delete(s.pending, id)
		}
	}
	for id, r := range s.codes {
		if !r.ExpiresAt.After(now) {
			delete(s.codes, id)
		}
	}
}

func validPKCEValue(value string, challenge bool) bool {
	if challenge {
		if len(value) != 43 {
			return false
		}
		decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
		return err == nil && len(decoded) == sha256.Size
	}
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	for _, ch := range value {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || strings.ContainsRune("-._~", ch)) {
			return false
		}
	}
	return true
}

func ValidateMCPOAuthAuthorization(in MCPOAuthAuthorizationInput, cfg utils.MCPConfig, issuer, resource string) (MCPOAuthAuthorizationRequest, error) {
	r := MCPOAuthAuthorizationRequest{}
	if in.ClientID != MCPChatGPTClientID {
		return r, oauthError("invalid_client")
	}
	if in.RedirectURI != MCPChatGPTRedirectURI {
		return r, oauthError("invalid_request")
	}
	if !cfg.Enabled {
		return r, ErrMCPDisabled
	}
	if in.ResponseType != "code" {
		return r, oauthError("unsupported_response_type")
	}
	if in.CodeChallengeMethod != "S256" || !validPKCEValue(in.CodeChallenge, true) || in.Resource != resource || len(in.State) > 4096 || len(in.Scope) > 2048 {
		return r, oauthError("invalid_request")
	}
	// OAuth scope tokens use ASCII spaces, rather than accepting arbitrary whitespace.
	for _, ch := range in.Scope {
		if ch < 0x20 || ch > 0x7e || ch == '"' || ch == '\\' {
			return r, oauthError("invalid_scope")
		}
	}
	scopes, err := normalizePersonalKeyScopes(strings.Fields(in.Scope))
	if err != nil || !(&MCPActor{Scopes: cfg.AllowedScopes()}).Allows(scopes...) {
		return r, oauthError("invalid_scope")
	}
	return MCPOAuthAuthorizationRequest{ClientID: in.ClientID, RedirectURI: in.RedirectURI, Scopes: scopes, Resource: resource, Issuer: issuer, State: in.State, CodeChallenge: in.CodeChallenge}, nil
}

func (s *MCPOAuthAuthorizationStore) Create(in MCPOAuthAuthorizationInput, cfg utils.MCPConfig, issuer, resource string) (string, error) {
	r, err := ValidateMCPOAuthAuthorization(in, cfg, issuer, resource)
	if err != nil {
		return "", err
	}
	if err := s.cimd.validate(); err != nil {
		return "", err
	}
	id, err := randomPersonalKey(32)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	if len(s.pending)+len(s.codes) >= mcpOAuthStoreLimit {
		return "", oauthError("temporarily_unavailable")
	}
	r.ExpiresAt = s.now().UTC().Add(mcpOAuthRequestTTL)
	s.pending[id] = r
	return id, nil
}

func (s *MCPOAuthAuthorizationStore) Get(id, issuer, resource string) (MCPOAuthAuthorizationRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	r, ok := s.pending[id]
	if !ok || r.Issuer != issuer || r.Resource != resource {
		return MCPOAuthAuthorizationRequest{}, ErrMCPOAuthExpired
	}
	r.Scopes = append([]string{}, r.Scopes...)
	return r, nil
}

func mcpOAuthRedirect(r MCPOAuthAuthorizationRequest, code, errorCode string) string {
	u, _ := url.Parse(r.RedirectURI)
	q := u.Query()
	q.Set("iss", r.Issuer)
	q.Set("state", r.State)
	if code != "" {
		q.Set("code", code)
	} else {
		q.Set("error", errorCode)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// Decide consumes the request atomically. UserID comes only from the login session.
func (s *MCPOAuthAuthorizationStore) Decide(id, userID, decision, issuer, resource string, cfg utils.MCPConfig) (string, error) {
	if decision != "allow" && decision != "deny" {
		return "", oauthError("invalid_request")
	}
	if _, err := mcpOAuthUser(userID); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	r, ok := s.pending[id]
	if !ok || r.Issuer != issuer || r.Resource != resource {
		return "", ErrMCPOAuthExpired
	}
	if decision == "deny" {
		delete(s.pending, id)
		return mcpOAuthRedirect(r, "", "access_denied"), nil
	}
	if !cfg.Enabled {
		return "", ErrMCPDisabled
	}
	if !(&MCPActor{Scopes: cfg.AllowedScopes()}).Allows(r.Scopes...) {
		return "", oauthError("invalid_scope")
	}
	code, err := randomPersonalKey(32)
	if err != nil {
		return "", err
	}
	r.UserID = userID
	r.ExpiresAt = s.now().UTC().Add(mcpOAuthCodeTTL)
	s.codes[code] = r
	delete(s.pending, id)
	return mcpOAuthRedirect(r, code, ""), nil
}

type MCPOAuthTokenInput struct {
	GrantType, Code, ClientID, RedirectURI, Resource, CodeVerifier, RefreshToken string
}

type MCPOAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

func mcpOAuthUser(userID string) (*model.UserModel, error) {
	var user model.UserModel
	if err := personalKeyDB().Where("id = ? AND disabled = ? AND deleted_at IS NULL", userID, false).First(&user).Error; err != nil || user.IsBot {
		return nil, ErrPersonalKeyInvalid
	}
	return &user, nil
}

func mcpOAuthSecrets() (string, string, error) {
	access, err := randomPersonalKey(32)
	if err != nil {
		return "", "", err
	}
	refresh, err := randomPersonalKey(32)
	return access, refresh, err
}

func mcpOAuthResponse(g *model.MCPOAuthGrantModel, access, refresh string, cfg utils.MCPConfig) *MCPOAuthTokenResponse {
	return &MCPOAuthTokenResponse{AccessToken: "sc_oauth_" + g.AccessPublicID + "." + access, TokenType: "Bearer", ExpiresIn: 3600, RefreshToken: "sc_oauth_r_" + g.RefreshPublicID + "." + refresh, Scope: strings.Join(mcpOAuthEffectiveScopes(g.Scopes, cfg), " ")}
}

func (s *MCPOAuthAuthorizationStore) Exchange(in MCPOAuthTokenInput, cfg utils.MCPConfig, issuer, resource string) (*MCPOAuthTokenResponse, error) {
	if !cfg.Enabled {
		return nil, ErrMCPDisabled
	}
	if in.ClientID != MCPChatGPTClientID {
		return nil, oauthError("invalid_client")
	}
	if in.GrantType == "refresh_token" {
		return RefreshMCPOAuthToken(in, cfg, issuer, resource)
	}
	if in.GrantType != "authorization_code" {
		return nil, oauthError("unsupported_grant_type")
	}
	s.mu.Lock()
	s.cleanupLocked()
	r, ok := s.codes[in.Code]
	delete(s.codes, in.Code) // Even a failed redemption cannot replay the code.
	s.mu.Unlock()
	if !ok || r.ClientID != in.ClientID || r.RedirectURI != in.RedirectURI || r.Resource != resource || in.Resource != resource || r.Issuer != issuer || !validPKCEValue(in.CodeVerifier, false) {
		return nil, oauthError("invalid_grant")
	}
	hash := sha256.Sum256([]byte(in.CodeVerifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	if subtle.ConstantTimeCompare([]byte(challenge), []byte(r.CodeChallenge)) != 1 {
		return nil, oauthError("invalid_grant")
	}
	if _, err := mcpOAuthUser(r.UserID); err != nil {
		return nil, oauthError("invalid_grant")
	}
	access, refresh, err := mcpOAuthSecrets()
	if err != nil {
		return nil, err
	}
	ap, err := randomPersonalKey(12)
	if err != nil {
		return nil, err
	}
	rp, err := randomPersonalKey(12)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	g := &model.MCPOAuthGrantModel{ID: utils.NewID(), UserID: r.UserID, ClientID: r.ClientID, Resource: r.Resource, Issuer: r.Issuer, Scopes: r.Scopes, AccessPublicID: ap, AccessSecretHash: personalKeyHash(access), AccessExpiresAt: now.Add(mcpOAuthAccessTTL), RefreshPublicID: rp, RefreshSecretHash: personalKeyHash(refresh), RefreshExpiresAt: now.Add(mcpOAuthRefreshTTL), CreatedAt: now, UpdatedAt: now}
	if err := personalKeyDB().Create(g).Error; err != nil {
		return nil, err
	}
	return mcpOAuthResponse(g, access, refresh, cfg), nil
}

func parseMCPOAuthToken(token, prefix string) (string, string, bool) {
	if !strings.HasPrefix(token, prefix) {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(token, prefix), ".")
	if len(parts) != 2 || len(parts[0]) != 16 || len(parts[1]) != 43 {
		return "", "", false
	}
	for _, p := range parts {
		if _, err := base64.RawURLEncoding.Strict().DecodeString(p); err != nil {
			return "", "", false
		}
	}
	return parts[0], parts[1], true
}

func RefreshMCPOAuthToken(in MCPOAuthTokenInput, cfg utils.MCPConfig, issuer, resource string) (*MCPOAuthTokenResponse, error) {
	if !cfg.Enabled {
		return nil, ErrMCPDisabled
	}
	if in.ClientID != MCPChatGPTClientID {
		return nil, oauthError("invalid_client")
	}
	public, secret, ok := parseMCPOAuthToken(in.RefreshToken, "sc_oauth_r_")
	if !ok || in.Resource != resource {
		return nil, oauthError("invalid_grant")
	}
	var g model.MCPOAuthGrantModel
	now := time.Now().UTC()
	if err := personalKeyDB().Where("refresh_public_id = ? AND revoked_at IS NULL", public).First(&g).Error; err != nil || !g.RefreshExpiresAt.After(now) || g.Resource != resource || g.Issuer != issuer || g.ClientID != in.ClientID || subtle.ConstantTimeCompare([]byte(personalKeyHash(secret)), []byte(g.RefreshSecretHash)) != 1 {
		return nil, oauthError("invalid_grant")
	}
	if _, err := mcpOAuthUser(g.UserID); err != nil {
		return nil, oauthError("invalid_grant")
	}
	access, refresh, err := mcpOAuthSecrets()
	if err != nil {
		return nil, err
	}
	// Database compare-and-set also prevents concurrent rotation across processes.
	result := personalKeyDB().Model(&model.MCPOAuthGrantModel{}).Where("id = ? AND refresh_secret_hash = ? AND revoked_at IS NULL AND refresh_expires_at > ?", g.ID, g.RefreshSecretHash, now).Updates(map[string]any{"access_secret_hash": personalKeyHash(access), "access_expires_at": now.Add(mcpOAuthAccessTTL), "refresh_secret_hash": personalKeyHash(refresh), "updated_at": now})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, oauthError("invalid_grant")
	}
	return mcpOAuthResponse(&g, access, refresh, cfg), nil
}

func mcpOAuthEffectiveScopes(scopes []string, cfg utils.MCPConfig) []string {
	allowed := &MCPActor{Scopes: cfg.AllowedScopes()}
	effective := []string{}
	for _, scope := range scopes {
		if allowed.Allows(scope) {
			effective = append(effective, scope)
		}
	}
	return effective
}

func AuthenticateMCPOAuthToken(token string, cfg utils.MCPConfig, expectedResource string) (*MCPActor, error) {
	if !cfg.Enabled {
		return nil, ErrMCPDisabled
	}
	public, secret, ok := parseMCPOAuthToken(token, "sc_oauth_")
	if !ok {
		return nil, ErrPersonalKeyInvalid
	}
	u, err := url.Parse(expectedResource)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, ErrPersonalKeyInvalid
	}
	issuer := u.Scheme + "://" + u.Host
	var g model.MCPOAuthGrantModel
	if err := personalKeyDB().Where("access_public_id = ? AND revoked_at IS NULL", public).First(&g).Error; err != nil || !g.AccessExpiresAt.After(time.Now()) || g.Resource != expectedResource || g.Issuer != issuer || g.ClientID != MCPChatGPTClientID || subtle.ConstantTimeCompare([]byte(personalKeyHash(secret)), []byte(g.AccessSecretHash)) != 1 {
		return nil, ErrPersonalKeyInvalid
	}
	user, err := mcpOAuthUser(g.UserID)
	if err != nil {
		return nil, err
	}
	return &MCPActor{User: user, GrantedScopes: append([]string{}, g.Scopes...), Scopes: mcpOAuthEffectiveScopes(g.Scopes, cfg), CredentialID: g.ID, CredentialType: "oauth"}, nil
}

func AuthenticateMCPCredential(token string, cfg utils.MCPConfig, expectedResource string) (*MCPActor, error) {
	if !cfg.Enabled {
		return nil, ErrMCPDisabled
	}
	if strings.HasPrefix(token, "sc_mcp_") {
		return AuthenticatePersonalAPIKey(token, cfg)
	}
	if strings.HasPrefix(token, "sc_oauth_") {
		return AuthenticateMCPOAuthToken(token, cfg, expectedResource)
	}
	return nil, ErrPersonalKeyInvalid
}

func TouchMCPCredential(actor *MCPActor) {
	if actor == nil {
		return
	}
	if actor.CredentialType == "pat" {
		TouchPersonalAPIKey(&actor.Key)
		return
	}
	if actor.CredentialType == "oauth" {
		now := time.Now().UTC()
		personalKeyDB().Model(&model.MCPOAuthGrantModel{}).Where("id = ? AND revoked_at IS NULL AND (last_used_at IS NULL OR last_used_at < ?)", actor.CredentialID, now.Add(-time.Minute)).UpdateColumn("last_used_at", now)
	}
}

// MCPOAuthGrantInfo is the complete management response allowlist. In particular
// it contains neither token public IDs nor hashes, issuer, or resource.
type MCPOAuthGrantInfo struct {
	ID               string     `json:"id"`
	Client           string     `json:"client"`
	Scopes           []string   `json:"scopes"`
	CreatedAt        time.Time  `json:"createdAt"`
	LastUsedAt       *time.Time `json:"lastUsedAt"`
	AccessExpiresAt  time.Time  `json:"accessExpiresAt"`
	RefreshExpiresAt time.Time  `json:"refreshExpiresAt"`
	RevokedAt        *time.Time `json:"revokedAt"`
}

func ListMCPOAuthGrants(userID string) ([]MCPOAuthGrantInfo, error) {
	var grants []model.MCPOAuthGrantModel
	err := personalKeyDB().Select("id", "scopes", "created_at", "last_used_at", "access_expires_at", "refresh_expires_at", "revoked_at").Where("user_id = ?", userID).Order("created_at DESC, id DESC").Find(&grants).Error
	items := make([]MCPOAuthGrantInfo, 0, len(grants))
	for _, g := range grants {
		items = append(items, MCPOAuthGrantInfo{ID: g.ID, Client: "ChatGPT", Scopes: append([]string{}, g.Scopes...), CreatedAt: g.CreatedAt, LastUsedAt: g.LastUsedAt, AccessExpiresAt: g.AccessExpiresAt, RefreshExpiresAt: g.RefreshExpiresAt, RevokedAt: g.RevokedAt})
	}
	return items, err
}

// Revoking missing/foreign/already-revoked IDs has the same idempotent response.
// Management deliberately remains available while MCP is disabled.
func RevokeMCPOAuthGrant(userID, grantID string) error {
	now := time.Now().UTC()
	return personalKeyDB().Model(&model.MCPOAuthGrantModel{}).Where("id = ? AND user_id = ? AND revoked_at IS NULL", grantID, userID).Updates(map[string]any{"revoked_at": now, "updated_at": now}).Error
}

func CleanupMCPOAuthGrants(now time.Time) (int64, error) {
	r := personalKeyDB().Where("refresh_expires_at < ?", now.UTC().Add(-7*24*time.Hour)).Delete(&model.MCPOAuthGrantModel{})
	return r.RowsAffected, r.Error
}
