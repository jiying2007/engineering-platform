package main

import (
	"os"
	"os/exec"
	"testing"
)

func TestCLIProcess(t *testing.T) {
	if os.Getenv("EP_TEST_CLI_CHILD") != "1" {
		return
	}
	i := 0
	for i < len(os.Args) && os.Args[i] != "--" {
		i++
	}
	os.Args = append([]string{"eng"}, os.Args[i+1:]...)
	main()
	os.Exit(0)
}
func TestCLIExitStatus(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
	}{{nil, 2}, {[]string{"--audit-invalid-command"}, 2}, {[]string{"help", "extra"}, 2}, {[]string{"--help"}, 0}, {[]string{"help"}, 0}, {[]string{"distribution-verify"}, 1}} {
		cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestCLIProcess$", "--"}, tc.args...)...)
		cmd.Env = append(os.Environ(), "EP_TEST_CLI_CHILD=1")
		err := cmd.Run()
		got := 0
		if err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				got = exit.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		if got != tc.code {
			t.Fatalf("args %v: exit %d want %d", tc.args, got, tc.code)
		}
	}
}
