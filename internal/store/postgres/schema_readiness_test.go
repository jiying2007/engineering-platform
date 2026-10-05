package postgres

import (
	"context"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	dbschema "github.com/jiying2007/engineering-platform/db"
)

func currentMigrationVersions() []int {
	var versions []int
	for i := 2; i <= dbschema.CurrentSchemaVersion; i++ {
		versions = append(versions, i)
	}
	return versions
}

func TestCoreSchemaVersionFence(t *testing.T) {
	good := currentMigrationVersions()
	if err := checkCoreMigrationVersions(good); err != nil {
		t.Fatal(err)
	}
	for name, versions := range map[string][]int{
		"empty": nil, "inbox-only": {2, 3}, "highest-only": {dbschema.CurrentSchemaVersion},
		"future":        append(append([]int{}, good...), dbschema.CurrentSchemaVersion+1),
		"untracked-one": append([]int{1}, good...), "partial": good[:len(good)-1],
		"zero": append([]int{0}, good[1:]...), "duplicate": append([]int{3}, good[1:]...),
	} {
		t.Run(name, func(t *testing.T) {
			if err := checkCoreMigrationVersions(versions); err == nil {
				t.Fatalf("incompatible ledger accepted: %v", versions)
			}
		})
	}
	for i := range good {
		missing := append(append([]int{}, good[:i]...), good[i+1:]...)
		if checkCoreMigrationVersions(missing) == nil {
			t.Fatalf("missing migration %d accepted", good[i])
		}
	}
	var s *Store
	if s.CheckWorkerSchema(context.Background()) == nil || (&Store{}).CheckWorkerSchema(context.Background()) == nil {
		t.Fatal("unconfigured store accepted")
	}
}

// Only a regression test parses SQL. The runtime never guesses a schema from
// arbitrary SQL and migrations are not rewritten to satisfy the new checker.
func TestCoreStartupInventoryMatchesEmbeddedMigrations(t *testing.T) {
	sql := dbschema.CoreMigration()
	tables := regexp.MustCompile(`(?im)^\s*CREATE TABLE (?:IF NOT EXISTS )?([a-z_]+)\s*\(`).FindAllStringSubmatch(sql, -1)
	var expected []string
	for _, match := range tables {
		expected = append(expected, match[1])
	}
	sort.Strings(expected)
	if !reflect.DeepEqual(expected, startupSchemaTables) {
		t.Fatalf("startup table inventory drift: migration=%v startup=%v", expected, startupSchemaTables)
	}
	markers := regexp.MustCompile(`INSERT INTO core_schema_migrations\(version\) VALUES\(([0-9]+)\)`).FindAllStringSubmatch(sql, -1)
	var versions []int
	for _, match := range markers {
		n, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatal(err)
		}
		versions = append(versions, n)
	}
	if !reflect.DeepEqual(versions, currentMigrationVersions()) {
		t.Fatalf("binary version fence and embedded ledger drift: %v", versions)
	}
	added := regexp.MustCompile(`(?i)ALTER TABLE ([a-z_]+) ADD COLUMN ([a-z_]+)`).FindAllStringSubmatch(sql, -1)
	for _, match := range added {
		if !strings.Contains(" "+strings.Join(startupSchemaColumns[match[1]], " ")+" ", " "+match[2]+" ") {
			t.Fatalf("startup probe missing added column %s.%s", match[1], match[2])
		}
	}
}
