package service

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	mcpChatGPTCIMDTTL     = time.Hour
	mcpChatGPTCIMDMaxBody = 64 << 10
)

// Only the capabilities used by our fixed public client are retained.
type mcpChatGPTMetadata struct {
	ClientID      string   `json:"client_id"`
	RedirectURIs  []string `json:"redirect_uris"`
	ResponseTypes []string `json:"response_types"`
	GrantTypes    []string `json:"grant_types"`
	AuthMethods   []string `json:"token_endpoint_auth_methods_supported"`
}

func (m mcpChatGPTMetadata) valid() bool {
	contains := func(values []string, want string) bool {
		for _, value := range values {
			if value == want {
				return true
			}
		}
		return false
	}
	return m.ClientID == MCPChatGPTClientID && contains(m.RedirectURIs, MCPChatGPTRedirectURI) && contains(m.ResponseTypes, "code") && contains(m.GrantTypes, "authorization_code") && contains(m.AuthMethods, "none")
}

type mcpChatGPTCIMDResolver struct {
	mu        sync.Mutex
	fetching  chan struct{}
	client    *http.Client
	metadata  mcpChatGPTMetadata
	expiresAt time.Time
	now       func() time.Time
}

func newMCPChatGPTCIMDResolver(transport http.RoundTripper) *mcpChatGPTCIMDResolver {
	return &mcpChatGPTCIMDResolver{client: &http.Client{
		Transport:     transport,
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, now: time.Now}
}

var mcpChatGPTCIMD = newMCPChatGPTCIMDResolver(nil)

func (r *mcpChatGPTCIMDResolver) cached() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.expiresAt.After(r.now()) && r.metadata.valid()
}

func (r *mcpChatGPTCIMDResolver) validate() error {
	r.mu.Lock()
	if r.expiresAt.After(r.now()) && r.metadata.valid() {
		r.mu.Unlock()
		return nil
	}
	if done := r.fetching; done != nil {
		r.mu.Unlock()
		// Concurrent callers share even a failed fetch; they do not each queue
		// another five-second network request behind a mutex.
		<-done
		if r.cached() {
			return nil
		}
		return oauthError("temporarily_unavailable")
	}
	done := make(chan struct{})
	r.fetching = done
	r.mu.Unlock()
	metadata, err := r.fetch()
	r.mu.Lock()
	if err == nil {
		r.metadata = metadata
		r.expiresAt = r.now().Add(mcpChatGPTCIMDTTL)
	}
	valid := r.expiresAt.After(r.now()) && r.metadata.valid()
	r.fetching = nil
	close(done)
	r.mu.Unlock()
	if valid {
		return nil
	}
	return oauthError("temporarily_unavailable")
}

func (r *mcpChatGPTCIMDResolver) fetch() (mcpChatGPTMetadata, error) {
	invalid := func() (mcpChatGPTMetadata, error) { return mcpChatGPTMetadata{}, oauthError("temporarily_unavailable") }
	// No authorize parameter can influence this URL, method, or redirect policy.
	req, err := http.NewRequest(http.MethodGet, MCPChatGPTClientID, nil)
	if err != nil {
		return invalid()
	}
	req.Header.Set("Accept", "application/json")
	res, err := r.client.Do(req)
	if err != nil {
		return invalid()
	}
	defer res.Body.Close()
	contentType, _, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if res.StatusCode < 200 || res.StatusCode >= 300 || err != nil || (contentType != "application/json" && !(strings.HasPrefix(contentType, "application/") && strings.HasSuffix(contentType, "+json"))) {
		return invalid()
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, mcpChatGPTCIMDMaxBody+1))
	var metadata mcpChatGPTMetadata
	if err != nil || len(raw) > mcpChatGPTCIMDMaxBody || json.Unmarshal(raw, &metadata) != nil || !metadata.valid() {
		return invalid()
	}
	return metadata, nil
}

// NewMCPOAuthAuthorizationStoreWithCIMDTransport exposes only the HTTP transport
// seam; even tests cannot change the fixed URL, timeout, or redirect policy.
func NewMCPOAuthAuthorizationStoreWithCIMDTransport(transport http.RoundTripper) *MCPOAuthAuthorizationStore {
	s := NewMCPOAuthAuthorizationStore()
	s.cimd = newMCPChatGPTCIMDResolver(transport)
	return s
}
