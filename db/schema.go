package dbschema

import _ "embed"

//go:embed migrations/0001_core.sql
var coreMigration string

//go:embed migrations/0002_outbox_authority.sql
var outboxAuthorityMigration string

//go:embed migrations/0003_worker_inbox.sql
var workerInboxMigration string

func CoreMigration() string {
	return coreMigration + "\n" + outboxAuthorityMigration + "\n" + workerInboxMigration
}
