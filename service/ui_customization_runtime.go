package service

import (
	"encoding/json"
	"strings"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/utils"
)

type UICustomizationContext struct {
	Surface   UISurface `json:"surface"`
	WorldID   string    `json:"worldId"`
	ChannelID string    `json:"channelId"`
	Slug      string    `json:"-"`
}

type UICustomizationRuntimePackage struct {
	ID        string `json:"id"`
	Ref       string `json:"ref"`
	Origin    string `json:"origin"`
	Version   string `json:"version,omitempty"`
	ScopeType string `json:"scopeType"`
	Name      string `json:"name"`
	SortOrder int    `json:"sortOrder"`
	CreatedAt int64  `json:"createdAt"`
	CSS       string `json:"css"`
	// JS is empty whenever the script may not run in this context. World
	// scripts additionally require per-browser trust of packageId + runtime fingerprint.
	JS                  string   `json:"js"`
	JSHash              string   `json:"jsHash"`
	RequiresTrust       bool     `json:"requiresTrust"`
	ChannelCapabilities []string `json:"channelCapabilities"`
	RuntimeFingerprint  string   `json:"runtimeFingerprint"`
}

// uiSurfaceCoveredByGlobal lists the ordinary surfaces a global target
// reaches. Public surfaces (observer, embed, log-public) must be targeted
// explicitly, and internal is never customizable.
func uiSurfaceCoveredByGlobal(s UISurface) bool {
	switch s {
	case UISurfaceObserver, UISurfaceEmbed, UISurfaceLogPublic, UISurfaceInternal, UISurfaceGlobal:
		return false
	}
	return s.Valid()
}

func uiSurfaceTargeted(surfaces []UISurface, current UISurface) bool {
	for _, s := range surfaces {
		if s == current || s == UISurfaceGlobal && uiSurfaceCoveredByGlobal(current) {
			return true
		}
	}
	return false
}

type UICustomizationRuntime struct {
	UICustomizationContext
	Packages []UICustomizationRuntimePackage `json:"packages"`
}

// resolveUICustomizationWorld validates context on the server. Public slugs
// resolve through the same access checks as their original public endpoints;
// a caller-provided worldId never authorizes a private world.
func resolveUICustomizationWorld(ctx UICustomizationContext, actorID string) (string, bool, error) {
	// Public-surface policy controls CSS/JS exposure, while access mode controls
	// how the world/channel itself is authorized. In particular, an authenticated
	// member using the embed surface must keep normal channel permissions instead
	// of being downgraded to observer access.
	publicSurface := ctx.Surface == UISurfaceEmbed || ctx.Surface == UISurfaceObserver || ctx.Surface == UISurfaceLogPublic
	switch ctx.Surface {
	case UISurfaceAuth, UISurfaceLobby, UISurfaceStatus, UISurfaceInternal:
		return "", publicSurface, nil
	case UISurfaceLogPublic:
		if ctx.Slug == "" {
			return "", true, nil
		}
		s, err := logLoadPublic(ctx.Slug)
		if err != nil {
			return "", true, err
		}
		return s.Project.WorldID, true, nil
	case UISurfaceLogLayout:
		if ctx.Slug == "" {
			return "", false, nil
		}
		p, err := ResolveLogLayout(ctx.Slug, actorID)
		if err != nil {
			return "", false, err
		}
		return p.WorldID, false, nil
	case UISurfaceObserver:
		if ctx.Slug != "" {
			w, _, err := ResolveWorldObserverLink(ctx.Slug)
			if err != nil {
				return "", true, err
			}
			if ctx.ChannelID != "" {
				if _, err := CanObserverAccessChannel(ctx.ChannelID, w.ID); err != nil {
					return "", true, ErrUICustomizationDenied
				}
			}
			return w.ID, true, nil
		}
	}
	if ctx.WorldID == "" {
		return "", publicSurface, nil
	}
	w, err := GetWorldByID(ctx.WorldID)
	if err != nil {
		return "", publicSurface, err
	}
	if w.Status != "active" {
		return "", publicSurface, ErrWorldNotFound
	}
	member := actorID != "" && IsWorldMember(w.ID, actorID)
	admin := actorID != "" && pm.CanWithSystemRole(actorID, pm.PermModAdmin)
	if ctx.Surface == UISurfaceInvite && !member {
		return "", false, ErrUICustomizationDenied
	}
	externalViewer := !member && !admin
	if externalViewer {
		if w.Visibility != model.WorldVisibilityPublic && !(actorID != "" && w.Visibility == model.WorldVisibilityUnlisted) {
			return "", publicSurface, ErrUICustomizationDenied
		}
	}
	publicContext := publicSurface || externalViewer
	if ctx.ChannelID != "" {
		if externalViewer {
			if _, err := CanObserverAccessChannel(ctx.ChannelID, w.ID); err != nil {
				return "", publicContext, ErrUICustomizationDenied
			}
		} else {
			var channel model.ChannelModel
			if err := model.GetDB().Where("id = ? AND world_id = ? AND status = ?", ctx.ChannelID, w.ID, model.ChannelStatusActive).Take(&channel).Error; err != nil {
				return "", publicContext, ErrUICustomizationDenied
			}
			if !CanReadChannelByUserId(actorID, channel.ID) {
				return "", publicContext, ErrUICustomizationDenied
			}
		}
	}
	return w.ID, publicContext, nil
}

func ResolveUICustomizations(ctx UICustomizationContext, actorID string, policy *utils.UICustomizationPolicyConfig) (*UICustomizationRuntime, error) {
	if !ctx.Surface.Valid() || ctx.Surface == UISurfaceGlobal {
		return nil, ErrUICustomizationInvalid
	}
	ctx.WorldID = strings.TrimSpace(ctx.WorldID)
	ctx.ChannelID = strings.TrimSpace(ctx.ChannelID)
	ctx.Slug = strings.TrimSpace(ctx.Slug)
	result := &UICustomizationRuntime{UICustomizationContext: ctx, Packages: []UICustomizationRuntimePackage{}}
	if ctx.Surface == UISurfaceInternal {
		result.WorldID = ""
		result.ChannelID = ""
		return result, nil
	}
	worldID, public, err := resolveUICustomizationWorld(ctx, actorID)
	if err != nil {
		return nil, err
	}
	result.WorldID = worldID
	// Public LOG uses the publication's world only for server-side resolution.
	if ctx.Surface == UISurfaceLogPublic {
		result.WorldID = ""
	}
	if result.WorldID == "" {
		result.ChannelID = ""
	}
	policy = utils.NormalizeUICustomizationPolicy(policy)
	// World scripts never reach anonymous or public contexts (OB, embed, public
	// LOG, non-member reads of a public world), regardless of policy.
	worldJS := policy.AllowWorldJS && !public && actorID != ""
	scopes := []UICustomizationScope{{Type: "platform"}}
	if worldID != "" && policy.AllowWorldCSS && (!public || policy.AllowPublicWorldCSS) {
		scopes = append(scopes, UICustomizationScope{Type: "world", ID: worldID})
	}
	seenBuiltinRefs := map[string]bool{}
	seenPlatformPackages := map[string]bool{}
	for _, scope := range scopes {
		var rows []model.UICustomizationPackageModel
		if err := uiCustomizationQuery(model.GetDB(), scope).Where("enabled = ?", true).
			Order("sort_order ASC, created_at ASC, id ASC").Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, m := range rows {
			var surfaces []UISurface
			css, js := m.CSS, m.JS
			version := m.BuiltinVersion
			builtinRef := ""
			if m.BuiltinKey != "" {
				builtin, err := builtinUICustomizationForKey(m.BuiltinKey)
				if err != nil {
					return nil, err
				}
				surfaces = builtin.Targets
				css, js = builtin.CSS, builtin.JS
				version = builtin.Version
				builtinRef = builtin.Ref
			} else if err := json.Unmarshal([]byte(m.SurfacesJSON), &surfaces); err != nil {
				return nil, err
			}
			if !uiSurfaceTargeted(surfaces, ctx.Surface) {
				continue
			}
			if builtinRef != "" && seenBuiltinRefs[builtinRef] {
				continue
			}
			item := UICustomizationRuntimePackage{
				ID: m.ID, Ref: m.ID, Origin: "user", ScopeType: m.ScopeType, Name: m.Name,
				SortOrder: m.SortOrder, CreatedAt: m.CreatedAt.UnixMilli(), CSS: css,
				ChannelCapabilities: uiCustomizationChannelCapabilities(&m),
			}
			if m.BuiltinKey != "" {
				item.Ref = BuiltinUICustomizationRefPrefix + m.BuiltinKey
				item.Origin = "builtin"
				item.Version = version
			}
			if scope.Type == "platform" || worldJS {
				item.JS = js
				item.JSHash = UICustomizationJSHash(js)
				item.RuntimeFingerprint = UICustomizationRuntimeFingerprint(js, item.ChannelCapabilities)
				item.RequiresTrust = scope.Type == "world" && js != ""
			}
			result.Packages = append(result.Packages, item)
			if scope.Type == "platform" {
				seenPlatformPackages[m.ID] = true
			}
			if builtinRef != "" {
				seenBuiltinRefs[builtinRef] = true
			}
		}
		if scope.Type == "platform" && worldID != "" && policy.AllowWorldCSS && (!public || policy.AllowPublicWorldCSS) {
			var refs []model.WorldUICustomizationPlatformRefModel
			if err := model.GetDB().Where("world_id = ?", worldID).Find(&refs).Error; err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(refs))
			for _, ref := range refs {
				if !seenPlatformPackages[ref.PlatformPackageID] {
					ids = append(ids, ref.PlatformPackageID)
				}
			}
			if len(ids) > 0 {
				var referenced []model.UICustomizationPackageModel
				if err := uiCustomizationNonBuiltinQuery(model.GetDB(), UICustomizationScope{Type: "platform"}).
					Where("id IN ?", ids).
					Order("sort_order ASC, created_at ASC, id ASC").Find(&referenced).Error; err != nil {
					return nil, err
				}
				for _, m := range referenced {
					var surfaces []UISurface
					if err := json.Unmarshal([]byte(m.SurfacesJSON), &surfaces); err != nil {
						return nil, err
					}
					if !uiSurfaceTargeted(surfaces, ctx.Surface) {
						continue
					}
					result.Packages = append(result.Packages, UICustomizationRuntimePackage{
						ID: m.ID, Ref: m.ID, Origin: "user", ScopeType: "platform", Name: m.Name,
						SortOrder: m.SortOrder, CreatedAt: m.CreatedAt.UnixMilli(), CSS: m.CSS,
						JS: m.JS, JSHash: UICustomizationJSHash(m.JS),
						ChannelCapabilities: uiCustomizationChannelCapabilities(&m), RuntimeFingerprint: UICustomizationRuntimeFingerprint(m.JS, uiCustomizationChannelCapabilities(&m)),
					})
					seenPlatformPackages[m.ID] = true
				}
			}
		}
	}
	return result, nil
}
