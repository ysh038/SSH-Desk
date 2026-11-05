// Package assets embeds bundled data files into the binary so the single
// static executable needs nothing else on a closed-network server.
package assets

import "embed"

// Snippets holds the bundled snippet files (SPEC 7.4).
//
//go:embed snippets/*.toml
var Snippets embed.FS
