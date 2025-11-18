package migrations

import "embed"

// Files contains the embedded SQL migrations.
//
//go:embed *.sql
var Files embed.FS
