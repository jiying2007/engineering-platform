package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/production"
)

// The complete template dry-run must work in both legitimate source states.
// Only private copies are changed; this never performs the terminal task.
func TestProductionTerminalMaintenanceTemplatesAcceptBeforeAndAfter(t *testing.T) {
	plan, err := production.BuildTerminalPlan()
	if err != nil {
		t.Fatal(err)
	}
	source, err := filepath.Abs(filepath.Join("..", "..", "examples", "production", "terminal-maintenance"))
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []struct{ name, marker string }{{"before", plan.Plan.MarkerBefore}, {"after", plan.Plan.MarkerAfter}} {
		t.Run(state.name, func(t *testing.T) {
			root := t.TempDir()
			fixture := filepath.Join(root, "examples", "production", "terminal-maintenance")
			if err := os.MkdirAll(fixture, 0o700); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(source)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				data, err := os.ReadFile(filepath.Join(source, entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(fixture, entry.Name()), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(root, plan.Plan.MarkerPath), []byte(state.marker), 0o600); err != nil {
				t.Fatal(err)
			}
			api := filepath.Join(root, "internal", "api")
			if err := os.MkdirAll(api, 0o700); err != nil {
				t.Fatal(err)
			}
			t.Chdir(api)
			TestProductionTerminalMaintenanceTemplatesDryRunToRunning(t)
		})
	}
}
