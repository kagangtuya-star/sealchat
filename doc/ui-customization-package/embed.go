package uicustomizationpackage

import "embed"

// Files contains the built-in themes that must remain available when SealChat
// is deployed as a single binary. The JSON files in this directory remain the
// canonical source and developer-facing examples.
//
//go:embed sealchat-ui-package-rlyeh-archive.json sealchat-ui-package-triangle-agency.json
var files embed.FS

// ReadFile returns one embedded UI customization package by file name.
func ReadFile(name string) ([]byte, error) {
	return files.ReadFile(name)
}
