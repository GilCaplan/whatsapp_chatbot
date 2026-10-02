// Package web embeds the static UI (index.html, ES modules, styles, assets)
// so the server ships as a single binary. The server never serves .go files
// or names beginning with "." or "_".
//
// Note: `//go:embed *` fails to compile if a subdirectory of web/ is empty;
// every folder here must contain at least one file.
package web

import (
	"embed"
	"io/fs"
)

//go:embed *
var files embed.FS

// FS is the UI file tree with index.html at its root.
var FS fs.FS = files
