// Package examples contains self-contained sample models. They are bundled
// so the editor's examples also work entirely offline.
package examples

import "embed"

// Models stores the example documents beside this file in the executable.
//
//go:embed *.json
var Models embed.FS
