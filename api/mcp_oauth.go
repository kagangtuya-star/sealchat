package api

import (
	"errors"
	"mime"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"sealchat/service"
	"sealchat/utils"
)

var mcpOAuthStore = service.NewMCPOAuthAuthorizationStore()

func mcpOAuthContext(c *fiber.Ctx) (utils.AppConfig, string, string, error) {
	c.Set("Cache-Control", "no-store")
	c.Set("Pragma", "no-cache")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Referrer-Policy", "no-referrer")
	cfg := mcpConfigSnapshot()
	issuer, err := mcpRequestOrigin(c, cfg, true)
	return cfg, issuer, issuer + joinWebPath(cfg.WebUrl, "mcp"), err
}

func mcpOAuthHTTPError(c *fiber.Ctx, err error) error {
	var hostError *mcpError
	if errors.As(err, &hostError) {
		return c.Status(403).JSON(hostError)
	}
	status, code := 400, "invalid_request"
	var protocolError *service.MCPOAuthError
	switch {
	case errors.As(err, &protocolError):
		code = protocolError.Code
	case errors.Is(err, service.ErrMCPDisabled):
		status, code = 503, "temporarily_unavailable"
	case errors.Is(err, service.ErrMCPOAuthExpired):
		status, code = 409, "expired"
	case errors.Is(err, service.ErrPersonalKeyInvalid):
		status, code = 401, "access_denied"
	default:
		status, code = 500, "server_error"
	}
	return c.Status(status).JSON(fiber.Map{"error": code})
}

// Reject duplicate parameters and all client secrets, including empty secrets.
func mcpOAuthParameters(raw string) (url.Values, error) {
	if len(raw) > 8192 {
		return nil, &service.MCPOAuthError{Code: "invalid_request"}
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return nil, &service.MCPOAuthError{Code: "invalid_request"}
	}
	for key, vals := range values {
		if key == "client_secret" || len(vals) != 1 {
			return nil, &service.MCPOAuthError{Code: "invalid_request"}
		}
	}
	return values, nil
}

func bindMCPOAuthPublicRoutes(app *fiber.App, webURL string) {
	publicLimiter := newMCPOAuthPublicLimiter()
	protected := func(c *fiber.Ctx) error {
		cfg, issuer, resource, err := mcpOAuthContext(c)
		if err != nil {
			return mcpOAuthHTTPError(c, err)
		}
		return c.JSON(fiber.Map{"resource": resource, "authorization_servers": []string{issuer}, "scopes_supported": cfg.MCP.AllowedScopes()})
	}
	authorizationServer := func(c *fiber.Ctx) error {
		cfg, issuer, _, err := mcpOAuthContext(c)
		if err != nil {
			return mcpOAuthHTTPError(c, err)
		}
		return c.JSON(fiber.Map{
			"issuer":                                         issuer,
			"authorization_endpoint":                         issuer + joinWebPath(webURL, "oauth/authorize"),
			"token_endpoint":                                 issuer + joinWebPath(webURL, "oauth/token"),
			"code_challenge_methods_supported":               []string{"S256"},
			"token_endpoint_auth_methods_supported":          []string{"none"},
			"scopes_supported":                               cfg.MCP.AllowedScopes(),
			"client_id_metadata_document_supported":          true,
			"authorization_response_iss_parameter_supported": true,
			"response_types_supported":                       []string{"code"},
			"grant_types_supported":                          []string{"authorization_code", "refresh_token"},
		})
	}
	app.Get("/.well-known/oauth-protected-resource", protected)
	app.Get("/.well-known/oauth-authorization-server", authorizationServer)
	if normalizeWebRoot(webURL) != "/" {
		app.Get(joinWebPath(webURL, ".well-known/oauth-protected-resource"), protected)
		app.Get(joinWebPath(webURL, ".well-known/oauth-authorization-server"), authorizationServer)
		// RFC discovery clients may append the resource/issuer path to well-known.
		app.Get("/.well-known/oauth-authorization-server"+normalizeWebRoot(webURL), authorizationServer)
	}
	app.Get("/.well-known/oauth-protected-resource"+joinWebPath(webURL, "mcp"), protected)
	app.Get(joinWebPath(webURL, "oauth/authorize"), mcpOAuthPublicLimit(publicLimiter, false), func(c *fiber.Ctx) error {
		cfg, issuer, resource, err := mcpOAuthContext(c)
		if err != nil {
			return mcpOAuthHTTPError(c, err)
		}
		params, err := mcpOAuthParameters(string(c.Request().URI().QueryString()))
		if err != nil {
			return mcpOAuthHTTPError(c, err)
		}
		input := service.MCPOAuthAuthorizationInput{ResponseType: params.Get("response_type"), ClientID: params.Get("client_id"), RedirectURI: params.Get("redirect_uri"), Scope: params.Get("scope"), State: params.Get("state"), CodeChallenge: params.Get("code_challenge"), CodeChallengeMethod: params.Get("code_challenge_method"), Resource: params.Get("resource")}
		id, err := mcpOAuthStore.Create(input, cfg.MCP, issuer, resource)
		if err != nil {
			// Only redirect errors after validating the complete fixed client/redirect pair.
			if input.ClientID == service.MCPChatGPTClientID && input.RedirectURI == service.MCPChatGPTRedirectURI {
				code := "server_error"
				var protocolError *service.MCPOAuthError
				if errors.As(err, &protocolError) {
					code = protocolError.Code
				}
				if errors.Is(err, service.ErrMCPDisabled) {
					code = "temporarily_unavailable"
				}
				q := url.Values{"error": {code}, "state": {input.State}, "iss": {issuer}}
				return c.Redirect(service.MCPChatGPTRedirectURI+"?"+q.Encode(), 302)
			}
			return mcpOAuthHTTPError(c, err)
		}
		// Vue Router uses hash history; keep the deployment base path outside the hash.
		base := strings.TrimSuffix(normalizeWebRoot(webURL), "/") + "/"
		return c.Redirect(issuer+base+"#/oauth/mcp/authorize?request="+url.QueryEscape(id), 302)
	})
	app.Post(joinWebPath(webURL, "oauth/token"), mcpOAuthPublicLimit(publicLimiter, true), func(c *fiber.Ctx) error {
		cfg, issuer, resource, err := mcpOAuthContext(c)
		if err != nil {
			return mcpOAuthHTTPError(c, err)
		}
		contentType, _, parseErr := mime.ParseMediaType(c.Get("Content-Type"))
		if parseErr != nil || contentType != "application/x-www-form-urlencoded" || c.Get("Authorization") != "" {
			return mcpOAuthHTTPError(c, &service.MCPOAuthError{Code: "invalid_request"})
		}
		if _, err := mcpOAuthParameters(string(c.Request().URI().QueryString())); err != nil {
			return mcpOAuthHTTPError(c, err)
		}
		params, err := mcpOAuthParameters(string(c.Body()))
		if err != nil {
			return mcpOAuthHTTPError(c, err)
		}
		input := service.MCPOAuthTokenInput{GrantType: params.Get("grant_type"), Code: params.Get("code"), ClientID: params.Get("client_id"), RedirectURI: params.Get("redirect_uri"), Resource: params.Get("resource"), CodeVerifier: params.Get("code_verifier"), RefreshToken: params.Get("refresh_token")}
		result, err := mcpOAuthStore.Exchange(input, cfg.MCP, issuer, resource)
		if err != nil {
			return mcpOAuthHTTPError(c, err)
		}
		return c.JSON(result)
	})
}

func bindMCPOAuthConsentRoutes(r fiber.Router) {
	guard := func(c *fiber.Ctx) error {
		_, issuer, _, err := mcpOAuthContext(c)
		if err != nil {
			return mcpOAuthHTTPError(c, err)
		}
		if user := getCurUser(c); user == nil || user.IsBot || user.Disabled {
			return c.SendStatus(401)
		}
		if origin := c.Get("Origin"); origin != "" && origin != issuer {
			return c.SendStatus(403)
		}
		return c.Next()
	}
	r.Get("/mcp/oauth/requests/:requestId", guard, func(c *fiber.Ctx) error {
		_, issuer, resource, _ := mcpOAuthContext(c)
		request, err := mcpOAuthStore.Get(c.Params("requestId"), issuer, resource)
		if err != nil {
			return mcpOAuthHTTPError(c, err)
		}
		scopes := []utils.MCPScope{}
		for _, id := range request.Scopes {
			for _, scope := range utils.MCPScopeCatalog {
				if scope.ID == id {
					scopes = append(scopes, scope)
					break
				}
			}
		}
		return c.JSON(fiber.Map{"client": "ChatGPT", "requestedScopes": scopes, "expiresAt": request.ExpiresAt})
	})
	r.Post("/mcp/oauth/requests/:requestId", guard, func(c *fiber.Ctx) error {
		cfg, issuer, resource, _ := mcpOAuthContext(c)
		var in struct {
			Decision string `json:"decision"`
		}
		contentType, _, _ := mime.ParseMediaType(c.Get("Content-Type"))
		if contentType != "application/json" || decodeMCPJSON(c.Body(), &in) != nil {
			return mcpOAuthHTTPError(c, &service.MCPOAuthError{Code: "invalid_request"})
		}
		redirect, err := mcpOAuthStore.Decide(c.Params("requestId"), getCurUser(c).ID, in.Decision, issuer, resource, cfg.MCP)
		if err != nil {
			return mcpOAuthHTTPError(c, err)
		}
		return c.JSON(fiber.Map{"redirectUrl": redirect})
	})
}
