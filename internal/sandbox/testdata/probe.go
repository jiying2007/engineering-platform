// Compiled statically into a scratch image only by integration tests.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

func require(ok bool, what string) {
	if !ok {
		fmt.Fprintln(os.Stderr, "FAILED:", what)
		os.Exit(2)
	}
}
func main() {
	if len(os.Args) > 1 && os.Args[1] == "sleep" {
		time.Sleep(2 * time.Minute)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "flood" {
		for i := 0; i < 1024; i++ {
			fmt.Println(strings.Repeat("x", 1024))
		}
		return
	}
	require(os.Getuid() != 0, "non-root")
	status, err := os.ReadFile("/proc/self/status")
	require(err == nil, "process status")
	require(strings.Contains(string(status), "NoNewPrivs:\t1"), "no-new-privileges")
	require(strings.Contains(string(status), "CapEff:\t0000000000000000"), "no capabilities")
	require(strings.Contains(string(status), "Seccomp:\t2"), "seccomp filter")
	for _, key := range []string{"CONTROL_CLIENT_KEY_FILE", "DATABASE_URL", "OPENAI_API_KEY", "DOCKER_HOST"} {
		require(os.Getenv(key) == "", "no inherited credentials")
	}
	_, err = os.Stat("/var/run/docker.sock")
	require(os.IsNotExist(err), "no engine socket")
	data, err := os.ReadFile("/workspace/hello.txt")
	require(err == nil, "source readable")
	err = os.WriteFile("/workspace/hello.txt", []byte("must not write"), 0o600)
	require(err != nil, "source read-only")
	_, err = os.ReadFile("/context/manifest.json")
	require(err == nil, "approved context readable")
	err = os.WriteFile("/context/injected", []byte("no"), 0o600)
	require(err != nil, "context read-only")
	err = os.WriteFile("/rootfs-write", []byte("no"), 0o600)
	require(err != nil, "root filesystem read-only")
	err = os.WriteFile("/tmp/scratch", []byte("private"), 0o600)
	require(err == nil, "bounded private scratch")
	conn, err := net.DialTimeout("tcp", "1.1.1.1:443", 200*time.Millisecond)
	if conn != nil {
		conn.Close()
	}
	require(err != nil, "network disabled")
	if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "/") {
		_, err = os.ReadFile(os.Args[1])
		require(os.IsNotExist(err), "host secret not mounted")
	}
	h := sha256.Sum256(data)
	fmt.Printf("OFFLINE_PROBE_PASS source_sha256=%s\n", hex.EncodeToString(h[:]))
}
