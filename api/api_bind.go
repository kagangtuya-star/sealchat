package api

import (
	_ "embed"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/spf13/afero"

	"sealchat/pm"
	"sealchat/utils"
)

var appConfig *utils.AppConfig
var appFs afero.Fs

func Init(config *utils.AppConfig, uiStatic fs.FS) {
	appConfig = config
	corsConfig := cors.New(cors.Config{
		AllowMethods:     "GET, POST, PUT, DELETE",
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
	v1.Get("/worlds", WorldList)
	v1.Get("/worlds/:slug", WorldDetailBySlug)
	v1.Get("/invites/:code", InvitePreview)

	v1.Get("/config", func(c *fiber.Ctx) error {
		ret := *appConfig
		ret.LogUpload.Token = ""
		u := getCurUser(c)
		if u == nil || !pm.CanWithSystemRole(u.ID, pm.PermModAdmin) {
			ret.ServeAt = ""
		}
		return c.Status(http.StatusOK).JSON(ret)
	})

	v1.Get("/attachment/:id", AttachmentGet)

	v1Auth := v1.Group("")
	v1Auth.Use(SignCheckMiddleware)
	v1Auth.Post("/user-password-change", UserChangePassword)
	v1Auth.Get("/user-info", UserInfo)
	v1Auth.Post("/user-info-update", UserInfoUpdate)
	v1Auth.Post("/worlds", WorldCreate)
	v1Auth.Put("/worlds/:worldId", WorldUpdate)
	v1Auth.Delete("/worlds/:worldId", WorldDelete)
	v1Auth.Get("/worlds/:worldId/channels", WorldChannelList)
	v1Auth.Post("/worlds/:worldId/channels", WorldChannelCreate)
	v1Auth.Post("/worlds/:worldId/invites", WorldInviteCreate)
	v1Auth.Get("/worlds/:worldId/invites", WorldInviteList)
	v1Auth.Get("/worlds/:worldId/members", WorldMemberList)
	v1Auth.Delete("/worlds/:worldId/members/:userId", WorldMemberRemove)
	v1Auth.Post("/invites/:code/accept", InviteAccept)
	v1Auth.Post("/user-emoji-add", UserEmojiAdd)
	v1Auth.Get("/user-emoji-list", UserEmojiList)
	v1Auth.Post("/user-emoji-delete", UserEmojiDelete)
	v1Auth.Patch("/user-emoji/:id", UserEmojiUpdate)

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
	v1Auth.Static("/gallery/thumbs", "./data/gallery/thumbs")

	audio := v1Auth.Group("/audio")
	audio.Get("/assets", AudioAssetList)
	audio.Get("/assets/:id", AudioAssetGet)
	audio.Get("/folders", AudioFolderList)
	audio.Get("/scenes", AudioSceneList)
	audio.Get("/stream/:id", AudioAssetStream)
	audio.Get("/state", AudioPlaybackStateGet)
	audioAdmin := audio.Group("", UserRoleAdminMiddleware)
	audioAdmin.Post("/assets/upload", AudioAssetUpload)
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
	v1Auth.Post("/channel-info-edit", ChannelInfoEdit)
	v1Auth.Get("/channel-info", ChannelInfoGet)
	v1Auth.Get("/channel-perm-tree", ChannelPermTree)
	v1Auth.Get("/channel-role-perms", ChannelRolePermGet)
	v1Auth.Post("/role-perms-apply", RolePermApply)
	v1Auth.Get("/channel-presence", ChannelPresence)
	v1Auth.Post("/chat/export", ChatExportCreate)
	v1Auth.Get("/chat/export/:taskId", ChatExportGet)
	v1Auth.Post("/chat/export/test", ChatExportTest)
	v1Auth.Post("/chat/export/:taskId/upload", ChatExportUpload)

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

	v1AuthAdmin.Put("/config", func(ctx *fiber.Ctx) error {
		var newConfig utils.AppConfig
		err := ctx.BodyParser(&newConfig)
		if err != nil {
			return err
		}
		appConfig = &newConfig
		utils.WriteConfig(appConfig)
		return nil
	})

	// Default /test
	app.Use(config.WebUrl, filesystem.New(filesystem.Config{
		Root:       http.FS(uiStatic),
		PathPrefix: "ui/dist",
		MaxAge:     5 * 60,
	}))

	websocketWorks(app)

	// Default :3212
	log.Fatal(app.Listen(config.ServeAt))
}
