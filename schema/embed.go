// Package schemafs embeds the JSON schema files under schema/<layer>/<major>/
// (contracts section 11) so the binary validates its own records without
// a working tree.
package schemafs

import "embed"

// FS holds every schema file, addressed as "<layer>/<major>/<name>.json".
//
//go:embed trace/1/*.json doctor/1/*.json
var FS embed.FS
