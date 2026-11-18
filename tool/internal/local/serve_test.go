package local

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func requireUnixSockets(t *testing.T) {
	t.Helper()
	listener, err := net.Listen("unix", filepath.Join(t.TempDir(), "probe.sock"))
	if errors.Is(err, syscall.EPERM) && os.Getenv("ENG_TEST_ALLOW_UNIX_SOCKET_RESTRICTION") == "1" {
		t.Skip("this executor denies Unix sockets; the Linux CI gate must run without ENG_TEST_ALLOW_UNIX_SOCKET_RESTRICTION")
	}
	if err != nil {
		t.Fatalf("Unix sockets are required for this integration test: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
}

func socketProject(t *testing.T, store *Store, root string, argv []string) {
	t.Helper()
	runtime := Runtime{Name: "fixture", Argv: argv, Stdin: "task", Timeout: "10s", Model: false}
	project := Project{Name: "fixture", Root: root, Base: "main", Runtime: runtime.Name, Checks: []Check{{Name: "local", Argv: []string{"/bin/true"}, Timeout: "5s"}}}
	if err := saveRegistration(store.DataDir(), "runtimes", runtime.Name, runtime, false); err != nil {
		t.Fatal(err)
	}
	if err := saveRegistration(store.DataDir(), "projects", project.Name, project, false); err != nil {
		t.Fatal(err)
	}
}

func startSocketServer(t *testing.T, store *Store) {
	t.Helper()
	requireUnixSockets(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, store, io.Discard) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("serve shutdown: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("serve did not stop its local execution")
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for pingServer(store.DataDir()) != nil {
		select {
		case err := <-done:
			done <- err
			t.Fatalf("serve startup: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("socket did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func socketCLI(t *testing.T, store *Store, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	handled, code := ForwardCLI(ctx, store.DataDir(), args, &stdout, &stderr)
	if !handled || code != 0 {
		t.Fatalf("socket command %q: handled=%t exit=%d stdout=%s stderr=%s", args, handled, code, stdout.String(), stderr.String())
	}
	return stdout.String()
}

func TestServePrivateSocketQueuesWhileBusy(t *testing.T) {
	store, root := executionStore(t), executionRepository(t)
	gateDir := t.TempDir()
	started, release := filepath.Join(gateDir, "started"), filepath.Join(gateDir, "release")
	socketProject(t, store, root, []string{"/bin/sh", "-c", `printf ready > "$1"; while [ ! -f "$2" ]; do sleep 0.05; done; cat > source.txt`, "fixture", started, release})
	first, err := newTask(context.Background(), store, "fixture", "after\n")
	if err != nil {
		t.Fatal(err)
	}
	startSocketServer(t, store)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first deterministic runtime did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	info, err := os.Stat(filepath.Join(store.DataDir(), "eng.sock"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("socket is not private: %v %v", info, err)
	}
	var status struct {
		Task     Task      `json:"task"`
		Attempts []Attempt `json:"attempts"`
	}
	if err := json.Unmarshal([]byte(socketCLI(t, store, "status", first.ID)), &status); err != nil || status.Task.State != StateRunning || len(status.Attempts) != 1 {
		t.Fatalf("socket status lost active execution: %+v %v", status, err)
	}
	instruction := filepath.Join(gateDir, "task.md")
	if err := os.WriteFile(instruction, []byte("Queued from another terminal\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var queued []Task
	for _, verb := range []string{"submit", "run"} {
		var task Task
		if err := json.Unmarshal([]byte(socketCLI(t, store, verb, "fixture", "--task", instruction)), &task); err != nil || task.State != StateQueued || task.CurrentAttempt != 0 {
			t.Fatalf("%s did not return the queued task: %+v %v", verb, task, err)
		}
		queued = append(queued, task)
	}
	var stopped Task
	if err := json.Unmarshal([]byte(socketCLI(t, store, "stop", queued[1].ID)), &stopped); err != nil || stopped.State != StateCancelled {
		t.Fatalf("socket stop did not cancel queued work: %+v %v", stopped, err)
	}
	var resumed Task
	if err := json.Unmarshal([]byte(socketCLI(t, store, "resume", stopped.ID)), &resumed); err != nil || resumed.State != StateQueued || resumed.CurrentAttempt != 0 {
		t.Fatalf("socket resume tried to execute synchronously: %+v %v", resumed, err)
	}
	for _, args := range [][]string{{"doctor"}, {"runtime", "list"}, {"project", "list"}, {"logs", first.ID}} {
		_ = socketCLI(t, store, args...)
	}
}

func TestForwardAfterAcceptedRequestDoesNotReplay(t *testing.T) {
	requireUnixSockets(t)
	store := newStoreForTest(t)
	socketProject(t, store, t.TempDir(), []string{"/bin/true"})
	input := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(input, []byte("Exactly one submission\n"), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(store.DataDir(), "eng.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			accepted <- err
			return
		}
		defer conn.Close()
		request, err := http.ReadRequest(bufio.NewReader(conn))
		if err != nil {
			accepted <- err
			return
		}
		defer request.Body.Close()
		var payload commandRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			accepted <- err
			return
		}
		_, err = command(context.Background(), store, payload.Args, io.Discard, io.Discard)
		accepted <- err
		// Simulate a committed task followed by a dropped response.
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	handled, code := ForwardCLI(ctx, store.DataDir(), []string{"submit", "fixture", "--task", input}, &stdout, &stderr)
	if err := <-accepted; err != nil {
		t.Fatal(err)
	}
	if !handled || code == 0 || !strings.Contains(stderr.String(), "unknown") || !strings.Contains(stderr.String(), "status first") {
		t.Fatalf("ambiguous submission permitted fallback: handled=%t code=%d stderr=%s", handled, code, stderr.String())
	}
	tasks, err := store.ListTasks(context.Background())
	if err != nil || len(tasks) != 1 {
		t.Fatalf("submission was lost or duplicated: %+v %v", tasks, err)
	}
}

func TestForwardMissingSocketAndBlockedCommands(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	if handled, _ := ForwardCLI(context.Background(), dir, []string{"status"}, &stdout, &stderr); handled {
		t.Fatal("missing socket did not allow local operation")
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatal("local fallback printed a misleading remote result")
	}
	if err := os.WriteFile(filepath.Join(dir, "eng.sock"), []byte("not a socket"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{"serve", "backup", "restore", "publish", "recover", "version", "unknown"} {
		if handled, _ := ForwardCLI(context.Background(), dir, []string{verb}, &stdout, &stderr); handled {
			t.Fatalf("forwarded disallowed command %s", verb)
		}
	}
	if handled, code := ForwardCLI(context.Background(), dir, []string{"status"}, &stdout, &stderr); !handled || code == 0 {
		t.Fatal("an existing invalid service endpoint permitted local fallback")
	}
}

func TestForwardRefusedSocketUsesLocalCLI(t *testing.T) {
	requireUnixSockets(t)
	dir := t.TempDir()
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: filepath.Join(dir, "eng.sock")})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if handled, _ := ForwardCLI(context.Background(), dir, []string{"status"}, &stdout, &stderr); handled {
		t.Fatal("stale refused socket did not allow local operation")
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatal("local fallback printed a misleading remote result")
	}
}

func TestServeRejectsForbiddenAndOversizedRequests(t *testing.T) {
	store := executionStore(t)
	startSocketServer(t, store)
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(store.DataDir(), "eng.sock"))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	cases := []struct {
		body string
		code int
	}{
		{`{"args":["publish","task","--repo","owner/repo"]}`, http.StatusForbidden},
		{`{"args":["backup","/tmp/must-not-create"]}`, http.StatusForbidden},
		{`{"args":["restore","/tmp/must-not-read"]}`, http.StatusForbidden},
		{`{"args":["serve"]}`, http.StatusForbidden},
		{`{"args":["recover"]}`, http.StatusForbidden},
		{`{"args":["status"],"unexpected":true}`, http.StatusBadRequest},
		{`{"args":["status"]} {"args":["status"]}`, http.StatusBadRequest},
		{`{"args":["status","` + strings.Repeat("x", maxCommandRequest) + `"]}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		response, err := client.Post("http://localhost/command", "application/json", strings.NewReader(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != tc.code {
			t.Fatalf("request rejection: got HTTP %d, want %d", response.StatusCode, tc.code)
		}
	}
}

func TestForwardResolvesInvokingCLIPaths(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	args, err := absoluteCommandPaths([]string{"submit", "fixture", "--task=task.md"})
	if err != nil || args[2] != "--task="+filepath.Join(root, "task.md") {
		t.Fatalf("instruction path would use service cwd: %v %v", args, err)
	}
	args, err = absoluteCommandPaths([]string{"project", "add", "fixture", "repo", "--config", ".eng.yaml"})
	if err != nil || args[3] != filepath.Join(root, "repo") || args[5] != ".eng.yaml" {
		t.Fatalf("project/config path behavior changed: %v %v", args, err)
	}
}

func TestServeCommandEnqueuesWithoutTakingScheduler(t *testing.T) {
	store := newStoreForTest(t)
	socketProject(t, store, t.TempDir(), []string{"/bin/true"})
	input := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(input, []byte("No runtime may start inside an HTTP handler\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Own the actual scheduler lock. The server handler must enqueue through the
	// normal CLI parser without trying to acquire it again or probing /health.
	lock, err := AcquireScheduler(store.DataDir())
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	body, _ := json.Marshal(commandRequest{Args: []string{"run", "fixture", "--task", input}})
	request := httptest.NewRequest(http.MethodPost, "/command", bytes.NewReader(body))
	response := httptest.NewRecorder()
	serveCommand(store, response, request)
	var reply commandResponse
	if err := json.Unmarshal(response.Body.Bytes(), &reply); err != nil || reply.ExitCode != 0 {
		t.Fatalf("queue-only handler failed: %+v %v", reply, err)
	}
	var task Task
	if err := json.Unmarshal([]byte(reply.Stdout), &task); err != nil || task.State != StateQueued || task.CurrentAttempt != 0 {
		t.Fatalf("handler started execution or lost the task: %+v %v", task, err)
	}
	attempts, err := store.ListAttempts(context.Background(), task.ID)
	if err != nil || len(attempts) != 0 {
		t.Fatalf("handler claimed an attempt: %+v %v", attempts, err)
	}
}
