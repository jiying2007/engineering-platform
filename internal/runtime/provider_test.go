package runtime

import (
	"context"
	"testing"
)

func TestProviderRequiresExecutable(t *testing.T) {
	p := NewLocalProcessProvider("codex")
	if _, err := p.Command(context.Background(), LaunchSpec{}); err == nil {
		t.Fatal("expected missing executable error")
	}
	cmd, err := p.Command(context.Background(), LaunchSpec{Executable: "codex", Args: []string{"--version"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := cmd.Args[0]; got != "codex" {
		t.Fatalf("unexpected executable %q", got)
	}
}
