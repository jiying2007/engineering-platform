package local

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const maxCommandRequest = 2 << 20
const maxCommandResponse = 64 << 20

type serveCommandKey struct{}
type socketPeerKey struct{}

type commandRequest struct {
	Args []string `json:"args"`
}

type commandResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// ServingCommand marks a command dispatched by the scheduler's private socket.
// runQueued must return its queued task in this context instead of trying to
// acquire the already-owned scheduler or synchronously calling itself.
func ServingCommand(ctx context.Context) bool {
	serving, _ := ctx.Value(serveCommandKey{}).(bool)
	return serving
}

func forwardedCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "doctor", "runtime", "project", "run", "submit", "status", "logs", "stop", "resume", "accept":
		return true
	}
	return false
}

// ForwardCLI sends an eligible command to the local account's running service.
// Only a missing/refused initial connection permits local fallback. Once a
// connection exists, an unreadable response is an unknown result, not permission
// to replay a potentially committed submission. The HTTP exchange is deliberately
// written once to one connection; no transport retry or redirect is involved.
func ForwardCLI(ctx context.Context, dataDir string, args []string, out, errOut io.Writer) (handled bool, exitCode int) {
	if !forwardedCommand(args) {
		return false, 0
	}
	socket := filepath.Join(dataDir, "eng.sock")
	info, err := os.Lstat(socket)
	if errors.Is(err, os.ErrNotExist) {
		return false, 0
	}
	if err != nil {
		return true, writeCLIResult(out, errOut, nil, fmt.Errorf("inspect local service socket: %w", err))
	}
	if info.Mode()&os.ModeSocket == 0 {
		return true, writeCLIResult(out, errOut, nil, errors.New("eng.sock must be a real Unix socket"))
	}
	args, err = absoluteCommandPaths(args)
	if err != nil {
		return true, writeCLIResult(out, errOut, nil, err)
	}
	body, err := json.Marshal(commandRequest{Args: args})
	if err != nil || len(body) > maxCommandRequest {
		return true, writeCLIResult(out, errOut, nil, errors.New("local service command exceeds 2 MiB"))
	}
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(requestCtx, "unix", socket)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) {
			return false, 0
		}
		return true, writeCLIResult(out, errOut, nil, fmt.Errorf("cannot connect to the local service: %w", err))
	}
	defer conn.Close()
	uid, err := socketPeerUID(conn)
	if err != nil || uid != os.Getuid() {
		return true, writeCLIResult(out, errOut, nil, errors.New("local service must belong to the current OS account"))
	}
	deadline, _ := requestCtx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return true, writeCLIResult(out, errOut, nil, err)
	}
	stopCancel := context.AfterFunc(requestCtx, func() { _ = conn.Close() })
	defer stopCancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, "http://localhost/command", bytes.NewReader(body))
	if err != nil {
		return true, writeCLIResult(out, errOut, nil, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Close = true
	unknown := func(cause error) (bool, int) {
		_, _ = fmt.Fprintf(errOut, "eng: local service result is unknown: %v; do not repeat the command blindly; inspect eng --data-dir %q status first\n", cause, dataDir)
		return true, 1
	}
	if err := request.Write(conn); err != nil {
		return unknown(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), request)
	if err != nil {
		return unknown(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxCommandResponse+1))
	if err != nil || len(raw) > maxCommandResponse {
		return unknown(errors.New("service response was incomplete or too large; query one task at a time"))
	}
	var reply struct {
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		ExitCode *int   `json:"exit_code"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil || reply.ExitCode == nil || *reply.ExitCode < 0 || *reply.ExitCode > 255 {
		return unknown(errors.New("service returned an invalid command response"))
	}
	if response.StatusCode != http.StatusOK && *reply.ExitCode == 0 {
		return unknown(fmt.Errorf("service returned HTTP %d", response.StatusCode))
	}
	if _, err := io.WriteString(out, reply.Stdout); err != nil {
		return unknown(err)
	}
	if _, err := io.WriteString(errOut, reply.Stderr); err != nil {
		return true, 1
	}
	return true, *reply.ExitCode
}

// Paths are interpreted by the invoking CLI, not by the server's working
// directory. A project --config remains relative to its now-absolute root,
// matching the foreground registration behavior. Runtime argv stays unchanged.
func absoluteCommandPaths(args []string) ([]string, error) {
	result := append([]string(nil), args...)
	if len(result) >= 4 && result[0] == "project" && result[1] == "add" {
		root, err := filepath.Abs(result[3])
		if err != nil {
			return nil, err
		}
		result[3] = root
	}
	if len(result) == 0 || result[0] != "run" && result[0] != "submit" && result[0] != "resume" {
		return result, nil
	}
	for i := 2; i < len(result); i++ {
		arg := result[i]
		if arg == "--" || !strings.HasPrefix(arg, "-") {
			break
		}
		if arg == "--task" || arg == "-task" {
			if i+1 < len(result) && result[i+1] != "" {
				path, err := filepath.Abs(result[i+1])
				if err != nil {
					return nil, err
				}
				i++
				result[i] = path
			}
		} else if flag, path, found := strings.Cut(arg, "="); found && (flag == "--task" || flag == "-task") && path != "" {
			path, err := filepath.Abs(path)
			if err != nil {
				return nil, err
			}
			result[i] = flag + "=" + path
		}
	}
	return result, nil
}

func socketPeerUID(conn net.Conn) (int, error) {
	unix, ok := conn.(*net.UnixConn)
	if !ok {
		return -1, errors.New("local service requires a Unix socket")
	}
	raw, err := unix.SyscallConn()
	if err != nil {
		return -1, err
	}
	var credentials *syscall.Ucred
	var credentialErr error
	if err := raw.Control(func(fd uintptr) {
		credentials, credentialErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return -1, err
	}
	if credentialErr != nil || credentials == nil {
		return -1, errors.New("cannot verify local service OS account")
	}
	return int(credentials.Uid), nil
}

func serveCommand(s *Store, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	reject := func(status int, message string) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(commandResponse{Stderr: "eng: " + message + "\n", ExitCode: 1})
	}
	var request commandRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCommandRequest))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		reject(http.StatusBadRequest, "command must be one JSON object no larger than 2 MiB")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		reject(http.StatusBadRequest, "command must be one JSON object no larger than 2 MiB")
		return
	}
	if !forwardedCommand(request.Args) {
		reject(http.StatusForbidden, "this command is not available through the local service")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, serveCommandKey{}, true)
	var stdout, stderr bytes.Buffer
	value, err := command(ctx, s, request.Args, &stdout, &stderr)
	code := writeCLIResult(&stdout, &stderr, value, err)
	_ = json.NewEncoder(w).Encode(commandResponse{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: code})
}

// Serve is the same execution loop as foreground run, owned by one OS account.
// The socket is local and private; there is no network API or shared login store.
func Serve(ctx context.Context, s *Store, log io.Writer) error {
	lock, err := AcquireScheduler(s.DataDir())
	if err != nil {
		return err
	}
	defer lock.Close()
	if _, err := Recover(ctx, s); err != nil {
		return err
	}
	socket := filepath.Join(s.DataDir(), "eng.sock")
	if len(socket) > 100 {
		return errors.New("data directory path is too long for a local Unix socket; choose a shorter path")
	}
	if info, err := os.Lstat(socket); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return errors.New("refusing to replace a non-socket eng.sock")
		}
		if err := os.Remove(socket); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(socket)
	if err := os.Chmod(socket, 0600); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ready":true,"scope":"local-account","concurrency":1}`)
	})
	mux.HandleFunc("POST /command", func(w http.ResponseWriter, r *http.Request) { serveCommand(s, w, r) })
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, ok := r.Context().Value(socketPeerKey{}).(int)
		if !ok || uid != os.Getuid() {
			http.Error(w, "local service requires the same OS account", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 2 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx },
		ConnContext: func(ctx context.Context, conn net.Conn) context.Context {
			uid, _ := socketPeerUID(conn)
			return context.WithValue(ctx, socketPeerKey{}, uid)
		},
	}
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
		}
	}()
	_, _ = fmt.Fprintln(log, "eng: serving local queue; one task at a time; Ctrl-C stops the active process group")
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return nil
		}
		if _, err := Recover(ctx, s); err != nil {
			return err
		}
		tasks, err := s.ListTasks(ctx)
		if err != nil {
			return err
		}
		var next *Task
		for i := range tasks {
			if tasks[i].State == StateQueued && (next == nil || tasks[i].CreatedAt.Before(next.CreatedAt)) {
				next = &tasks[i]
			}
		}
		if next != nil {
			result, runErr := Execute(ctx, s, next.ID)
			if runErr != nil {
				_, _ = fmt.Fprintf(log, "eng: task %s: %v\n", next.ID, runErr)
			} else {
				_, _ = fmt.Fprintf(log, "eng: task %s: %s\n", result.ID, result.State)
			}
			if runErr == nil {
				continue
			}
			// A foreign owner or unresolved child can keep a queued item pending.
			// Wait before trying again instead of spinning on the same database row.
		}
		select {
		case <-ctx.Done():
			return nil
		case err := <-serverErr:
			if !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		case <-ticker.C:
		}
	}
}

func pingServer(dataDir string) error {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(dataDir, "eng.sock"))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	response, err := client.Get("http://localhost/health")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("scheduler is not ready")
	}
	return nil
}
