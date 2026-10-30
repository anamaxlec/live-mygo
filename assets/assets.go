package assets

import "embed"

// Files contains the static assets used by the app and packaging tools.
//
//go:embed app-icon.png app-icon-dark.png icons/*.svg
var Files embed.FS
