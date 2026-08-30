package builtin

import (
	"embed"
	"io/fs"
	"path"
)

// Keep source files beside the binary for external override deployments.
//
//go:embed channel-embed-tools
var channelEmbedTools embed.FS

// ReadChannelEmbedToolAsset reads one asset from a registered tool directory.
func ReadChannelEmbedToolAsset(directory, asset string) ([]byte, error) {
	return fs.ReadFile(channelEmbedTools, path.Join("channel-embed-tools", directory, asset))
}
