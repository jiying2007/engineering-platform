package dbschema

import _ "embed"

//go:embed migrations/0001_core.sql
var coreMigration string

//go:embed migrations/0002_outbox_authority.sql
var outboxAuthorityMigration string

func CoreMigration() string { return coreMigration + "\n" + outboxAuthorityMigration }
