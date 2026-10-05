// Compiled statically into a scratch image only by integration tests.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

func require(ok bool, what string) {
	if !ok {
		fmt.Fprintln(os.Stderr, "FAILED:", what)
		os.Exit(2)
	}
}
func main() {
	if len(os.Args) > 1 && os.Args[1] == "build-child" {
		// A detached writer survives command-group exit unless PID1 kills/reaps it.
		for {
			_ = os.WriteFile("/tmp/background-marker", []byte("alive"), 0600)
			time.Sleep(5 * time.Millisecond)
		}
	}
	if len(os.Args) > 2 && os.Args[1] == "build-output" {
		buildOutput(os.Args[2])
		return
	}

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

func buildOutput(mode string) {
	root := os.Getenv("EP_OUTPUT_DIR")
	require(root == "/tmp/ep-output", "fixed output root")
	require(os.Getuid() != 0, "build nonroot")
	a, b := root+"/app.bin", root+"/app.map"
	require(os.WriteFile(a, []byte("actual output\x00\xff"), 0600) == nil, "output bin")
	require(os.WriteFile(b, []byte("map bytes"), 0600) == nil, "output map")
	switch mode {
	case "missing":
		require(os.Remove(b) == nil, "remove fixture")
	case "extra":
		require(os.WriteFile(root+"/undeclared", nil, 0600) == nil, "extra fixture")
	case "symlink":
		require(os.Remove(a) == nil, "remove fixture")
		require(os.Symlink("/proc/1/environ", a) == nil, "link fixture")
	case "hardlink":
		require(os.Link(a, "/tmp/alias") == nil, "hardlink fixture")
	case "fifo":
		require(os.Remove(a) == nil, "remove fixture")
		require(syscall.Mkfifo(a, 0600) == nil, "fifo fixture")
	case "large":
		require(os.WriteFile(a, make([]byte, 65), 0600) == nil, "large fixture")
	case "directory":
		require(os.Remove(a) == nil, "remove fixture")
		require(os.Mkdir(a, 0700) == nil, "dir fixture")
	case "forge-log":
		f, err := os.OpenFile("/proc/1/fd/1", os.O_WRONLY, 0)
		require(err == nil, "raw log fixture")
		_, err = f.WriteString("{\"version\":1}\n")
		require(err == nil, "raw log injection")
		require(f.Close() == nil, "close")
	case "failed":
		os.Exit(7)
	case "background":
		c := exec.Command("/probe", "build-child")
		c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		require(c.Start() == nil, "detached writer")
	}
	fmt.Println("OFFLINE_BUILD_PASS")
}
