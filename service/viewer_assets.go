package service

import _ "embed"

type viewerAssets struct {
	CSS string
	JS  string
}

//go:embed embed/export_viewer.css
var embeddedViewerCSS string

//go:embed embed/tiptap-table.css
var embeddedTableCSS string

//go:embed embed/export_viewer.js
var embeddedViewerJS string

func getViewerAssets() viewerAssets {
	return viewerAssets{
		CSS: embeddedViewerCSS + "\n" + embeddedTableCSS,
		JS:  embeddedViewerJS,
	}
}
