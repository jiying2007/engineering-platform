package main

import "testing"

func TestPublisherRejectsUnsupportedArgumentsBeforeConfiguration(t *testing.T) {
	for _, args := range [][]string{{"--execute"}, {"--production"}, {"--migrate"}, {"--help"}, {"unexpected"}, {"--"}} {
		if err := serve(args); err == nil || err.Error() != "publisher-service accepts no command-line arguments" {
			t.Fatalf("unsupported arguments were not rejected before configuration: %v", err)
		}
	}
}
