package scripts

import "embed"

// FS contains the compiled-in LinkedIn browser extractor scripts.
//
//go:embed *.js
var FS embed.FS
