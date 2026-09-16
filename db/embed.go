package migrations

import "embed"

// Files contains the versioned PostgreSQL migrations.
//
//go:embed migrations/*.up.sql
var Files embed.FS
