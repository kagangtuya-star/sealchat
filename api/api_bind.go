package api

import (
	_ "embed"
	"encoding/json"
	"html"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/spf13/afero"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/service"
	"sealchat/utils"
)

var appConfig *utils.AppConfig
var appFs afero.Fs

// SyncConfigToDB 将配置同步到数据库
func SyncConfigToDB(config *utils.AppConfig, source string) {
	if config == nil {
		return
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		log.Printf("[配置] 序列化配置失败: %v", err)
		return
	}

	note := ""
	switch source {
	case "file":
		note = "从配置文件同步"
	case "init":
		note = "初始安装"
	case "api":
		note = "通过 API 修改"
	}

	if err := model.SaveConfigVersion(string(configJSON), source, note); err != nil {
		log.Printf("[配置] 保存配置版本失败: %v", err)
	}
}

type listenMode int

const (
	listenIPv4 listenMode = iota
	listenIPv6
	listenDual
)

func extractPort(addr string) string {
	_, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return ""
	}
	return port
}

func stripIPv6Zone(host string) string {
	if idx := strings.LastIndex(host, "%"); idx >= 0 {
		return host[:idx]
	}
	return host
}

func classifyListenMode(host string) listenMode {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" || trimmed == "0.0.0.0" {
		return listenDual
	}
	if ip := net.ParseIP(stripIPv6Zone(trimmed)); ip != nil {
		if ip.To4() != nil {
			return listenIPv4
		}
		return listenIPv6
	}
	return listenIPv4
}

func updateDomainPort(domain, newPort string) (string, bool) {
	if strings.TrimSpace(newPort) == "" {
		return domain, false
	}
	trimmed := strings.TrimSpace(domain)
	if trimmed == "" {
		return utils.FormatHostPort("127.0.0.1", newPort), true
	}

	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		parsed, err := url.Parse(trimmed)
		if err != nil || parsed.Host == "" {
			return domain, false
		}
		host := parsed.Hostname()
		if host == "" {
			return domain, false
		}
		parsed.Host = utils.FormatHostPort(host, newPort)
		return parsed.String(), true
	}

	host, _, err := net.SplitHostPort(trimmed)
	if err == nil {
		if host == "" {
			host = "127.0.0.1"
		}
		return utils.FormatHostPort(host, newPort), true
	}

	host = trimmed
	if host == "" {
		host = "127.0.0.1"
	}
	return utils.FormatHostPort(host, newPort), true
}

func buildIndexPaths(webURL string) []string {
	webRoot := strings.TrimSpace(webURL)
	if webRoot == "" {
		return []string{"/", "/index.html"}
	}
	if !strings.HasPrefix(webRoot, "/") {
		webRoot = "/" + webRoot
	}
	webRoot = strings.TrimRight(webRoot, "/")
	if webRoot == "" {
		webRoot = "/"
	}
	paths := []string{webRoot}
	if webRoot != "/" {
		paths = append(paths, webRoot+"/")
	}
	paths = append(paths, path.Join(webRoot, "index.html"))
	return paths
}

func applyPageTitleToIndex(htmlSource string, title string) string {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return htmlSource
	}
	start := strings.Index(htmlSource, "<title>")
	if start == -1 {
		return htmlSource
	}
	end := strings.Index(htmlSource[start:], "</title>")
	if end == -1 {
		return htmlSource
	}
	end += start
	escapedTitle := html.EscapeString(trimmed)
	return htmlSource[:start+len("<title>")] + escapedTitle + htmlSource[end:]
}

func Init(config *utils.AppConfig, uiStatic fs.FS) {
	appConfig = config
	corsConfig := cors.New(cors.Config{
		AllowMethods:     "GET, POST, PUT, DELETE, OPTIONS",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, ObjectId",
		ExposeHeaders:    "Content-Length",
		MaxAge:           3600,
		AllowOrigins:     "",
		AllowCredentials: true,
		AllowOriginsFunc: func(origin string) bool {
			return origin != ""
		},
	})

	appFs = afero.NewOsFs()

	imageLimitBytes := int(config.ImageSizeLimit * 1024)
	audioLimitBytes := int(config.Audio.MaxUploadSizeMB * 1024 * 1024)
	bodyLimit := imageLimitBytes
	if audioLimitBytes > bodyLimit {
		bodyLimit = audioLimitBytes
	}
	if bodyLimit < 32*1024*1024 {
		bodyLimit = 32 * 1024 * 1024
	}

	app := fiber.New(fiber.Config{
		BodyLimit: bodyLimit,
	})
	app.Use(corsConfig)
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(compress.New(compress.Config{
		Next: func(c *fiber.Ctx) bool {
			path := c.Path()
			return strings.HasPrefix(path, "/api/v1/audio/stream")
		},
	}))

	v1 := app.Group("/api/v1")
	v1.Post("/user-signup", UserSignup)
	v1.Post("/user-signin", UserSignin)
	v1.Get("/captcha/new", CaptchaNew)
	v1.Get("/captcha/:id.png", CaptchaImage)
	v1.Get("/captcha/:id/reload", CaptchaReload)

	// Email auth routes (public)
	v1.Post("/email-auth/signup-code", EmailAuthSignupCodeSend)
	v1.Post("/email-auth/signup", EmailAuthSignupWithCode)
	v1.Post("/password-reset/verify", EmailAuthPasswordResetVerify)
	v1.Post("/password-reset/request", EmailAuthPasswordResetRequest)
	v1.Post("/password-reset/confirm", EmailAuthPasswordResetConfirm)

	v1.Get("/config", func(c *fiber.Ctx) error {
		ret := sanitizeConfigForClient(appConfig)
		u := getCurUser(c)
		if u == nil || !pm.CanWithSystemRole(u.ID, pm.PermModAdmin) {
			ret.ServeAt = ""
		}
		ffmpegAvailable := false
		if svc := service.GetAudioService(); svc != nil {
			ffmpegAvailable = svc.FFmpegAvailable()
		}
		audioImportEnabled := false
		if appConfig != nil && strings.TrimSpace(appConfig.Audio.ImportDir) != "" {
			audioImportEnabled = true
		}
		resp := struct {
			utils.AppConfig
			FFmpegAvailable          bool `json:"ffmpegAvailable"`
			AllowWorldAudioWorkbench bool `json:"allowWorldAudioWorkbench"`
			AudioImportEnabled       bool `json:"audioImportEnabled"`
		}{
			AppConfig:                ret,
			FFmpegAvailable:          ffmpegAvailable,
			AllowWorldAudioWorkbench: ret.Audio.AllowWorldAudioWorkbench,
			AudioImportEnabled:       audioImportEnabled,
		}
		return c.Status(http.StatusOK).JSON(resp)
	})
	v1.Get("/public/worlds/:worldId", WorldPublicDetail)
	v1.Get("/public/worlds/:worldId/keywords", WorldKeywordPublicListHandler)
	v1.Get("/public/worlds/:worldId/keywords/categories", WorldKeywordPublicCategoriesHandler)

	v1.Get("/attachment/:id", AttachmentGet)
	v1.Get("/attachment/:id/thumb", AttachmentThumb)

	// External webhook API (channelId + token auth) - 必须在 v1Auth 之前定义
	v1.Get("/webhook/channels/:channelId/changes", WebhookAuthMiddleware, WebhookChanges)
	v1.Post("/webhook/channels/:channelId/messages", WebhookAuthMiddleware, WebhookMessages)

	v1Auth := v1.Group("")
	v1Auth.Use(SignCheckMiddleware)
	v1Auth.Post("/user-password-change", UserChangePassword)
	v1Auth.Get("/user-info", UserInfo)
	v1Auth.Post("/user-info-update", UserInfoUpdate)
	v1Auth.Get("/user-lookup", UserLookup)
	v1Auth.Post("/user-emoji-add", UserEmojiAdd)
	v1Auth.Post("/user-reaction-emoji-add", UserReactionEmojiAdd)
	v1Auth.Get("/user-emoji-list", UserEmojiList)
	v1Auth.Post("/user-emoji-delete", UserEmojiDelete)
	v1Auth.Patch("/user-emoji/:id", UserEmojiUpdate)

	// Email auth routes (authenticated)
	v1Auth.Post("/email-auth/bind-code", EmailAuthBindCodeSend)
	v1Auth.Post("/email-auth/bind-confirm", EmailAuthBindConfirm)

	// User preferences
	v1Auth.Get("/user/preferences", UserPreferencesGet)
	v1Auth.Post("/user/preferences", UserPreferencesUpsert)

	v1Auth.Get("/gallery/collections", GalleryCollectionsList)
	v1Auth.Post("/gallery/collections", GalleryCollectionCreate)
	v1Auth.Patch("/gallery/collections/:id", GalleryCollectionUpdate)
	v1Auth.Delete("/gallery/collections/:id", GalleryCollectionDelete)

	v1Auth.Get("/gallery/items", GalleryItemsList)
	v1Auth.Post("/gallery/items/upload", GalleryItemsUpload)
	v1Auth.Patch("/gallery/items/:id", GalleryItemUpdate)
	v1Auth.Post("/gallery/items/delete", GalleryItemsDelete)

	v1Auth.Get("/gallery/search", GallerySearch)

	v1Auth.Get("/timeline-list", TimelineList)
	v1Auth.Post("/timeline-mark-read", TimelineMarkRead)

	v1Auth.Post("/upload", Upload)
	v1Auth.Post("/upload-quick", UploadQuick)
	v1Auth.Get("/attachments-list", AttachmentList)

	v1Auth.Post("/attachment-upload", AttachmentUploadTempFile)
	v1Auth.Post("/attachment-upload-quick", AttachmentUploadQuick)
	v1Auth.Post("/attachment-confirm", AttachmentSetConfirm)
	v1Auth.Post("/attachments-delete", AttachmentDelete)
	v1Auth.Get("/attachment/:id/meta", AttachmentMeta)

	v1Auth.Get("/channel-identities", ChannelIdentityList)
	v1Auth.Post("/channel-identities", ChannelIdentityCreate)
	v1Auth.Put("/channel-identities/:id", ChannelIdentityUpdate)
	v1Auth.Delete("/channel-identities/:id", ChannelIdentityDelete)
	v1Auth.Post("/channel-identities/:id/bind-character-card", ChannelIdentityBindCharacterCard)
	v1Auth.Post("/channel-identities/:id/unbind-character-card", ChannelIdentityUnbindCharacterCard)

	v1Auth.Get("/character-cards", CharacterCardList)
	v1Auth.Post("/character-cards", CharacterCardCreate)
	v1Auth.Get("/character-cards/:id", CharacterCardGet)
	v1Auth.Put("/character-cards/:id", CharacterCardUpdate)
	v1Auth.Delete("/character-cards/:id", CharacterCardDelete)

	v1Auth.Get("/channel-identity-folders", ChannelIdentityFolderList)
	v1Auth.Post("/channel-identity-folders", ChannelIdentityFolderCreate)
	v1Auth.Put("/channel-identity-folders/:id", ChannelIdentityFolderUpdate)
	v1Auth.Delete("/channel-identity-folders/:id", ChannelIdentityFolderDelete)
	v1Auth.Post("/channel-identity-folders/:id/favorite", ChannelIdentityFolderToggleFavorite)
	v1Auth.Post("/channel-identity-folders/assign", ChannelIdentityFolderAssign)

	diceMacros := v1Auth.Group("/channels/:channelId/dice-macros")
	diceMacros.Get("/", ChannelDiceMacroList)
	diceMacros.Post("/", ChannelDiceMacroCreate)
	diceMacros.Put("/:macroId", ChannelDiceMacroUpdate)
	diceMacros.Delete("/:macroId", ChannelDiceMacroDelete)
	diceMacros.Post("/import", ChannelDiceMacroImport)

	v1Auth.Get("/channels/:channelId/messages/search", ChannelMessageSearch)
	v1Auth.Post("/messages/:messageId/reactions", MessageReactionAdd)
	v1Auth.Delete("/messages/:messageId/reactions", MessageReactionRemove)
	v1Auth.Get("/messages/:messageId/reactions", MessageReactionList)
	v1Auth.Get("/messages/:messageId/reactions/users", MessageReactionUsers)
	v1Auth.Get("/channels/:channelId/images", ChannelImagesList)
	v1Auth.Get("/channels/:channelId/mentionable-members", ChannelMentionableMembers)
	v1Auth.Get("/channels/:channelId/mentionable-members-all", ChannelMentionableMembersAll)

	// Sticky Note routes
	BindStickyNoteRoutes(v1Auth)

	// Channel webhook integrations (admin-only in channel)
	webhookIntegrations := v1Auth.Group("/channels/:channelId/webhook-integrations")
	webhookIntegrations.Get("/", WebhookIntegrationList)
	webhookIntegrations.Post("/", WebhookIntegrationCreate)
	webhookIntegrations.Post("/:id/rotate", WebhookIntegrationRotate)
	webhookIntegrations.Post("/:id/revoke", WebhookIntegrationRevoke)

	// Email notification settings
	v1Auth.Get("/channels/:channelId/email-notification", EmailNotificationSettingsGet)
	v1Auth.Post("/channels/:channelId/email-notification", EmailNotificationSettingsUpsert)
	v1Auth.Delete("/channels/:channelId/email-notification", EmailNotificationSettingsDelete)
	v1Auth.Post("/email-notification/test", EmailNotificationTestSend)

	v1Auth.Get("/commands", func(c *fiber.Ctx) error {
		m := map[string](map[string]string){}
		commandTips.Range(func(key string, value map[string]string) bool {
			m[key] = value
			return true
		})
		return c.Status(http.StatusOK).JSON(m)
	})
	uploadRoot := strings.TrimSpace(config.Storage.Local.UploadDir)
	if uploadRoot == "" {
		uploadRoot = "./data/upload"
	}
	v1Auth.Static("/attachments", uploadRoot)
	v1Auth.Get("/gallery/thumbs/:filename", GalleryThumbServe)

	v1Auth.Get("/status", StatusLatest)
	v1Auth.Get("/status/history", StatusHistory)

	audio := v1Auth.Group("/audio")
	audio.Get("/assets", AudioAssetList)
	audio.Get("/assets/:id", AudioAssetGet)
	audio.Get("/folders", AudioFolderList)
	audio.Get("/scenes", AudioSceneList)
	audio.Get("/stream/:id", AudioAssetStream)
	audio.Get("/state", AudioPlaybackStateGet)
	audioAdmin := audio.Group("", AudioWorkbenchMiddleware)
	audioAdmin.Post("/assets/upload", AudioAssetUpload)
	audioAdmin.Get("/assets/import/preview", AudioAssetImportPreview)
	audioAdmin.Post("/assets/import", AudioAssetImport)
	audioAdmin.Patch("/assets/:id", AudioAssetUpdate)
	audioAdmin.Delete("/assets/:id", AudioAssetDelete)
	audioAdmin.Post("/folders", AudioFolderCreate)
	audioAdmin.Patch("/folders/:id", AudioFolderUpdate)
	audioAdmin.Delete("/folders/:id", AudioFolderDelete)
	audioAdmin.Post("/scenes", AudioSceneCreate)
	audioAdmin.Patch("/scenes/:id", AudioSceneUpdate)
	audioAdmin.Delete("/scenes/:id", AudioSceneDelete)
	audioAdmin.Post("/state", AudioPlaybackStateSet)

	v1Auth.Get("/channel-role-list", ChannelRoles)
	v1Auth.Get("/channel-member-list", ChannelMembers)
	v1Auth.Get("/channels/:channelId/member-options", ChannelMemberOptions)
	v1Auth.Get("/channels/:channelId/speaker-options", ChannelSpeakerOptions)
	v1Auth.Get("/channels/:channelId/speaker-role-options", ChannelSpeakerRoleOptions)
	v1Auth.Post("/channels/:channelId/copy", ChannelCopy)
	v1Auth.Delete("/channels/:channelId", ChannelDissolve)
	v1Auth.Post("/channel-background-edit", ChannelBackgroundEdit)
	v1Auth.Post("/channel-info-edit", ChannelInfoEdit)
	v1Auth.Get("/channel-info", ChannelInfoGet)
	v1Auth.Get("/channel-perm-tree", ChannelPermTree)
	v1Auth.Get("/channel-role-perms", ChannelRolePermGet)
	v1Auth.Post("/role-perms-apply", RolePermApply)

	worldGroup := v1Auth.Group("/worlds")
	worldGroup.Get("/", WorldList)
	worldGroup.Get("", WorldList)
	worldGroup.Post("/", WorldCreateHandler)
	worldGroup.Post("", WorldCreateHandler)
	worldGroup.Get("/:worldId", WorldDetail)
	worldGroup.Patch("/:worldId", WorldUpdateHandler)
	worldGroup.Delete("/:worldId", WorldDeleteHandler)
	worldGroup.Post("/:worldId/join", WorldJoinHandler)
	worldGroup.Post("/:worldId/leave", WorldLeaveHandler)
	worldGroup.Get("/:worldId/sections", WorldSectionsHandler)
	worldGroup.Post("/:worldId/invites", WorldInviteCreateHandler)
	worldGroup.Get("/favorites", WorldFavoriteListHandler)
	worldGroup.Post("/:worldId/favorite", WorldFavoriteToggleHandler)
	worldGroup.Post("/:worldId/ack-edit-notice", WorldAckEditNoticeHandler)
	worldGroup.Get("/:worldId/members", WorldMemberListHandler)
	worldGroup.Delete("/:worldId/members/:userId", WorldMemberRemoveHandler)
	worldGroup.Post("/:worldId/members/:userId/role", WorldMemberRoleHandler)
	worldGroup.Get("/:worldId/keywords", WorldKeywordListHandler)
	worldGroup.Get("/:worldId/keywords/categories", WorldKeywordCategoriesHandler)
	worldGroup.Post("/:worldId/keywords", WorldKeywordCreateHandler)
	worldGroup.Patch("/:worldId/keywords/:keywordId", WorldKeywordUpdateHandler)
	worldGroup.Delete("/:worldId/keywords/:keywordId", WorldKeywordDeleteHandler)
	worldGroup.Post("/:worldId/keywords/bulk-delete", WorldKeywordBulkDeleteHandler)
	worldGroup.Post("/:worldId/keywords/reorder", WorldKeywordReorderHandler)
	worldGroup.Post("/:worldId/keywords/import", WorldKeywordImportHandler)
	worldGroup.Get("/:worldId/keywords/export", WorldKeywordExportHandler)
	worldGroup.Get("/:worldId/archived-channels", ArchivedChannelList)
	v1Auth.Post("/worlds/invites/:slug/consume", WorldInviteConsumeHandler)
	v1Auth.Post("/channels/archive", ChannelArchive)
	v1Auth.Post("/channels/unarchive", ChannelUnarchive)
	v1Auth.Delete("/channels/archived", ChannelPermanentDelete)
	v1Auth.Get("/channel-presence", ChannelPresence)
	v1Auth.Post("/chat/export", ChatExportCreate)
	v1Auth.Get("/chat/export", ChatExportList)
	v1Auth.Get("/chat/export/:taskId", ChatExportGet)
	v1Auth.Delete("/chat/export/:taskId", ChatExportDelete)
	v1Auth.Post("/chat/export/:taskId/retry", ChatExportRetry)
	v1Auth.Post("/chat/export/test", ChatExportTest)
	v1Auth.Post("/chat/export/:taskId/upload", ChatExportUpload)

	// 聊天记录导入
	chatImport := v1Auth.Group("/channels/:channelId/import")
	chatImport.Get("/templates", ChatImportTemplates)
	chatImport.Post("/preview", ChatImportPreview)
	chatImport.Post("/execute", ChatImportExecute)
	chatImport.Get("/jobs/:jobId", ChatImportJobStatus)
	chatImport.Get("/reusable-identities", ChatImportReusableIdentities)

	iform := v1Auth.Group("/channels/:channelId/iforms")
	iform.Get("/", ChannelIFormList)
	iform.Post("/", ChannelIFormCreate)
	iform.Patch("/:formId", ChannelIFormUpdate)
	iform.Delete("/:formId", ChannelIFormDelete)
	iform.Post("/push", ChannelIFormPush)
	iform.Post("/migrate", ChannelIFormMigrate)

	v1Auth.Post("/user-role-link", UserRoleLink)
	v1Auth.Post("/user-role-unlink", UserRoleUnlink)
	v1Auth.Get("/friend-list", FriendList)
	v1Auth.Get("/bot-list", BotList)

	v1AuthAdmin := v1Auth.Group("", UserRoleAdminMiddleware)
	v1AuthAdmin.Get("/admin/bot-token-list", BotTokenList)
	v1AuthAdmin.Post("/admin/bot-token-add", BotTokenAdd)
	v1AuthAdmin.Post("/admin/bot-token-update", BotTokenUpdate)
	v1AuthAdmin.Post("/admin/bot-token-delete", BotTokenDelete)
	v1AuthAdmin.Get("/admin/user-list", AdminUserList)
	v1AuthAdmin.Post("/admin/user-disable", AdminUserDisable)
	v1AuthAdmin.Post("/admin/user-enable", AdminUserEnable)
	v1AuthAdmin.Post("/admin/user-password-reset", AdminUserResetPassword)
	v1AuthAdmin.Post("/admin/user-role-link-by-user-id", AdminUserRoleLinkByUserId)
	v1AuthAdmin.Post("/admin/user-role-unlink-by-user-id", AdminUserRoleUnlinkByUserId)
	v1AuthAdmin.Post("/admin/user-create", AdminUserCreate)
	v1AuthAdmin.Get("/admin/user-check-username", AdminCheckUsername)
	v1AuthAdmin.Get("/admin/user-import-template", AdminUserImportTemplate)
	v1AuthAdmin.Post("/admin/user-batch-create", AdminUserBatchCreate)
	v1AuthAdmin.Get("/admin/update-status", AdminUpdateStatus)
	v1AuthAdmin.Post("/admin/update-check", AdminUpdateCheck)
	v1AuthAdmin.Post("/admin/update-version", AdminUpdateVersion)
	v1AuthAdmin.Get("/admin/backup/list", AdminBackupList)
	v1AuthAdmin.Post("/admin/backup/execute", AdminBackupExecute)
	v1AuthAdmin.Post("/admin/backup/delete", AdminBackupDelete)

	// Image migration routes
	v1AuthAdmin.Get("/admin/image-migration/preview", ImageMigrationPreview)
	v1AuthAdmin.Post("/admin/image-migration/execute", ImageMigrationExecute)
	v1AuthAdmin.Get("/admin/s3-migration/preview", S3MigrationPreview)
	v1AuthAdmin.Post("/admin/s3-migration/execute", S3MigrationExecute)
	v1AuthAdmin.Get("/admin/audio-folder-migration/preview", AudioFolderMigrationPreview)
	v1AuthAdmin.Post("/admin/audio-folder-migration/execute", AudioFolderMigrationExecute)

	// Email notification admin test
	v1AuthAdmin.Post("/admin/email-test", AdminEmailTestSend)

	v1AuthAdmin.Put("/config", func(ctx *fiber.Ctx) error {
		var payload struct {
			utils.AppConfig
			AllowWorldAudioWorkbench *bool `json:"allowWorldAudioWorkbench"`
		}
		err := ctx.BodyParser(&payload)
		if err != nil {
			return err
		}

		newConfig := payload.AppConfig
		if payload.AllowWorldAudioWorkbench != nil {
			newConfig.Audio.AllowWorldAudioWorkbench = *payload.AllowWorldAudioWorkbench
		}

		appConfig = mergeConfigForWrite(appConfig, &newConfig)
		utils.WriteConfig(appConfig)

		// 同步到数据库
		SyncConfigToDB(appConfig, "api")

		return nil
	})

	indexHTML, indexErr := fs.ReadFile(uiStatic, "ui/dist/index.html")
	if indexErr != nil {
		log.Printf("读取内置 index.html 失败: %v", indexErr)
	} else {
		renderIndex := func(c *fiber.Ctx) error {
			page := applyPageTitleToIndex(string(indexHTML), appConfig.PageTitle)
			c.Set(fiber.HeaderContentType, "text/html; charset=utf-8")
			return c.Status(http.StatusOK).SendString(page)
		}
		for _, routePath := range buildIndexPaths(config.WebUrl) {
			pathCopy := routePath
			app.Get(pathCopy, renderIndex)
		}
	}

	// Default /test
	app.Use(config.WebUrl, filesystem.New(filesystem.Config{
		Root:       http.FS(uiStatic),
		PathPrefix: "ui/dist",
		MaxAge:     5 * 60,
	}))

	websocketWorks(app)

	// Check port availability and find fallback if needed
	listenAddr := config.ServeAt
	if normalized, changed := utils.NormalizeServeAt(listenAddr); changed {
		listenAddr = normalized
		config.ServeAt = normalized
	}
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		host = ""
		port = "3212"
	}
	if port == "" {
		port = "3212"
	}
	mode := classifyListenMode(host)
	applyFallback := func(originalAddr, actualAddr string) {
		log.Printf("警告: 端口 %s 被占用，已切换到 %s", originalAddr, actualAddr)
		config.ServeAt = actualAddr
		newPort := extractPort(actualAddr)
		if newDomain, ok := updateDomainPort(config.Domain, newPort); ok {
			config.Domain = newDomain
		}
		utils.WriteConfig(config)
		log.Printf("配置文件已更新: serveAt=%s, domain=%s", config.ServeAt, config.Domain)
	}

	switch mode {
	case listenIPv6:
		actualAddr, usedFallback := utils.FindAvailablePortWithNetwork("tcp6", listenAddr)
		if usedFallback {
			applyFallback(listenAddr, actualAddr)
		}
		ln6, err := net.Listen("tcp6", actualAddr)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("IPv6 listening at %s", actualAddr)
		log.Fatal(app.Listener(ln6))
	case listenDual:
		listenAddr4 := utils.FormatListenHostPort(host, port)
		actualAddr4, usedFallback := utils.FindAvailablePortWithNetwork("tcp4", listenAddr4)
		if usedFallback {
			applyFallback(listenAddr4, actualAddr4)
		}
		port = extractPort(actualAddr4)
		if port == "" {
			port = "3212"
		}
		listenAddr6 := utils.FormatListenHostPort("::", port)
		ln4, err4 := net.Listen("tcp4", actualAddr4)
		if err4 != nil {
			log.Printf("IPv4 listen failed: %v", err4)
		}
		ln6, err6 := net.Listen("tcp6", listenAddr6)
		if err6 != nil {
			log.Printf("IPv6 listen unavailable: %v", err6)
		} else {
			log.Printf("IPv6 listening at %s", listenAddr6)
		}
		if ln4 != nil {
			if ln6 != nil {
				go func() {
					if err := app.Listener(ln6); err != nil {
						log.Printf("IPv6 listener stopped: %v", err)
					}
				}()
			}
			log.Printf("IPv4 listening at %s", actualAddr4)
			log.Fatal(app.Listener(ln4))
		}
		if ln6 != nil {
			log.Fatal(app.Listener(ln6))
		}
		log.Fatal(err4)
	default:
		actualAddr, usedFallback := utils.FindAvailablePortWithNetwork("tcp4", listenAddr)
		if usedFallback {
			applyFallback(listenAddr, actualAddr)
		}
		ln4, err := net.Listen("tcp4", actualAddr)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("IPv4 listening at %s", actualAddr)
		log.Fatal(app.Listener(ln4))
	}
}
