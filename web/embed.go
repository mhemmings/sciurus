// Package web contains the compiled, offline frontend shipped in the Go binary.
// Run yarn build in this directory after editing frontend source.
package web

import "embed"

// Assets holds the complete production frontend; no separate files or network
// downloads are required when running a compiled Sciurus binary.
//
//go:embed dist
var Assets embed.FS
