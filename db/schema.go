package dbschema

import _ "embed"

//go:embed migrations/0001_core.sql
var coreMigration string

func CoreMigration() string {
	return coreMigration
}
