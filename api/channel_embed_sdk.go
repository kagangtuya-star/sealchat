package api

import (
	_ "embed"

	"github.com/gofiber/fiber/v2"
)

//go:embed embed/channel-embed-sdk.js
var channelEmbedSDK []byte

// ChannelEmbedSDKHandler serves the browser SDK without login. The SDK itself
// only starts a scoped MessageChannel session after the iframe handshake.
func ChannelEmbedSDKHandler(c *fiber.Ctx) error {
	c.Set(fiber.HeaderContentType, "application/javascript; charset=utf-8")
	c.Set(fiber.HeaderCacheControl, "public, max-age=300")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Referrer-Policy", "no-referrer")
	return c.Send(channelEmbedSDK)
}
