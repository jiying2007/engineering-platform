package main

import "testing"

func TestBootstrapListenerDefaultsToLoopback(t *testing.T) {
	for _, tc := range []struct{ host, port, want string }{
		{"", "", "127.0.0.1:8080"},
		{"", "9090", "127.0.0.1:9090"},
		{"::1", "9090", "[::1]:9090"},
		{"0.0.0.0", "8081", "0.0.0.0:8081"},
	} {
		if got := controlPlaneAddress(tc.host, tc.port); got != tc.want {
			t.Fatalf("address=%s want=%s", got, tc.want)
		}
	}
}
