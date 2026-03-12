package wraithui

import "embed"

// StaticFiles embeds the frontend web assets.
// The web/static/ directory contains the compiled frontend (HTML, CSS, JS).
//
//go:embed all:web/static
var StaticFiles embed.FS
