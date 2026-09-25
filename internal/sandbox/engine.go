package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const apiVersion = "/v1.45"

var containerID = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Engine struct {
	client      *http.Client
	transport   *http.Transport
	guard       string
	guardDigest string
}

func New(socket, guard string) (*Engine, error) {
	if runtime.GOOS != "linux" {
		return nil, ErrPolicy
	}
	if !filepath.IsAbs(socket) || !filepath.IsAbs(guard) {
		return nil, ErrPolicy
	}
	for _, p := range []string{socket, guard} {
		resolved, err := filepath.EvalSymlinks(p)
		if err != nil || resolved != p {
			return nil, ErrPolicy
		}
	}
	st, err := os.Stat(socket)
	if err != nil || st.Mode()&os.ModeSocket == 0 {
		return nil, ErrPolicy
	}
	st, err = os.Stat(guard)
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0o022 != 0 || st.Mode().Perm()&0o111 == 0 || st.Size() > 64<<20 {
		return nil, ErrPolicy
	}
	f, err := os.Open(guard)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, (64<<20)+1))
	after, statErr := f.Stat()
	closeErr := f.Close()
	if readErr != nil || statErr != nil || closeErr != nil || len(data) > 64<<20 || !os.SameFile(st, after) {
		return nil, ErrPolicy
	}
	t := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", socket)
	}, MaxResponseHeaderBytes: 32 << 10, DisableCompression: true, ResponseHeaderTimeout: 10 * time.Second}
	return &Engine{guard: guard, guardDigest: Hash(data), transport: t, client: &http.Client{Transport: t, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrPolicy }}}, nil
}
func (e *Engine) Close() { e.transport.CloseIdleConnections() }

func canonicalDir(p string) error {
	if !filepath.IsAbs(p) || filepath.Clean(p) != p || strings.ContainsAny(p, "\x00\n\r") {
		return ErrPolicy
	}
	r, err := filepath.EvalSymlinks(p)
	if err != nil || r != p {
		return ErrPolicy
	}
	i, err := os.Stat(p)
	if err != nil || !i.IsDir() || i.Mode().Perm()&0o022 != 0 {
		return ErrPolicy
	}
	return nil
}
func (e *Engine) Ready(ctx context.Context, p Profile) error {
	if p.Validate() != nil || p.GuardDigest != e.guardDigest || os.Getuid() == 0 {
		return ErrPolicy
	}
	data, _, err := e.call(ctx, http.MethodGet, "/info", nil, 1<<20)
	if err != nil {
		return err
	}
	var info struct {
		OSType                              string
		SecurityOptions                     []string
		MemoryLimit, CPUCfsQuota, PidsLimit bool
	}
	if json.Unmarshal(data, &info) != nil || info.OSType != "linux" || !info.MemoryLimit || !info.CPUCfsQuota || !info.PidsLimit {
		return ErrPolicy
	}
	seccomp := false
	for _, v := range info.SecurityOptions {
		if strings.HasPrefix(v, "name=seccomp") {
			seccomp = true
		}
	}
	if !seccomp {
		return ErrPolicy
	}
	data, _, err = e.call(ctx, http.MethodGet, "/images/"+p.Image+"/json", nil, 1<<20)
	if err != nil {
		return err
	}
	var image struct {
		ID     string `json:"Id"`
		Config struct {
			Volumes map[string]any
			Env     []string
		}
	}
	if json.Unmarshal(data, &image) != nil || image.ID != p.Image || len(image.Config.Volumes) > 0 {
		return ErrPolicy
	}
	for _, env := range image.Config.Env {
		k, _, ok := strings.Cut(env, "=")
		if !ok || (k != "PATH" && k != "HOME" && k != "LANG" && k != "TMPDIR") {
			return ErrPolicy
		}
	}
	return nil
}

// Run never mounts a writable host directory. Engine/kernel/image/guard and host
// parents are trusted. Unknown cleanup cannot produce a successful Result.
func (e *Engine) Run(ctx context.Context, p Profile, source, bundle string) (result Result, err error) {
	if err = e.Ready(ctx, p); err != nil {
		return result, err
	}
	if canonicalDir(source) != nil || canonicalDir(bundle) != nil {
		return result, ErrPolicy
	}
	p.Argv = append([]string(nil), p.Argv...)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.Seconds+20)*time.Second)
	defer cancel()
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		return result, err
	}
	owner := hex.EncodeToString(nonce)
	mount := func(src, dst string) any {
		return map[string]any{"Type": "bind", "Source": src, "Target": dst, "ReadOnly": true, "BindOptions": map[string]any{"Propagation": "rprivate", "NonRecursive": true}}
	}
	user := strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())
	config := map[string]any{"Image": p.Image, "Entrypoint": []string{"/ep-guard"}, "Cmd": append([]string{strconv.Itoa(p.Seconds)}, p.Argv...), "User": user, "WorkingDir": "/workspace", "Env": []string{"PATH=/usr/bin:/bin", "HOME=/tmp", "LANG=C", "TMPDIR=/tmp"}, "NetworkDisabled": true, "Tty": false, "OpenStdin": false, "Labels": map[string]string{"engineering-platform.offline": owner}, "Healthcheck": map[string]any{"Test": []string{"NONE"}}, "HostConfig": map[string]any{"NetworkMode": "none", "ReadonlyRootfs": true, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges:true"}, "Memory": 256 << 20, "MemorySwap": 256 << 20, "NanoCpus": 1_000_000_000, "PidsLimit": 64, "IpcMode": "private", "ShmSize": 8 << 20, "RestartPolicy": map[string]any{"Name": "no"}, "LogConfig": map[string]any{"Type": "local", "Config": map[string]string{"max-size": "1m", "max-file": "1", "compress": "false"}}, "Tmpfs": map[string]string{"/tmp": "rw,nosuid,nodev,noexec,size=67108864,mode=1777"}, "Mounts": []any{mount(source, "/workspace"), mount(bundle, "/context"), mount(e.guard, "/ep-guard")}}}
	name := "ep-offline-" + owner
	created := false
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		target := name
		if created {
			target = result.ContainerID
		}
		_, status, removeErr := e.call(cleanup, http.MethodDelete, "/containers/"+target+"?force=true&v=true", nil, 32<<10)
		if removeErr != nil && status != 404 {
			err = errors.Join(err, ErrUnknown)
		}
		if err != nil {
			result = Result{}
		}
	}()
	data, _, err := e.call(ctx, http.MethodPost, "/containers/create?name="+name, config, 64<<10)
	if err != nil {
		return result, err
	}
	var response struct {
		ID       string `json:"Id"`
		Warnings []string
	}
	if json.Unmarshal(data, &response) != nil || !containerID.MatchString(response.ID) {
		return result, ErrUnknown
	}
	result.ContainerID = response.ID
	created = true
	if len(response.Warnings) != 0 {
		return result, fmt.Errorf("%w: engine could not apply requested constraints", ErrPolicy)
	}
	data, _, err = e.call(ctx, http.MethodGet, "/containers/"+response.ID+"/json", nil, 1<<20)
	if err != nil {
		return result, err
	}
	if err = validateContainer(data, p, user, owner, source, bundle, e.guard); err != nil {
		return result, err
	}
	if _, _, err = e.call(ctx, http.MethodPost, "/containers/"+response.ID+"/start", nil, 32<<10); err != nil {
		return result, err
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-ticker.C:
		}
		data, _, err = e.call(ctx, http.MethodGet, "/containers/"+response.ID+"/json", nil, 1<<20)
		if err != nil {
			return result, err
		}
		var current struct {
			State struct {
				Running   bool
				Status    string
				ExitCode  int
				OOMKilled bool
				Error     string
			}
		}
		if json.Unmarshal(data, &current) != nil {
			return result, ErrUnknown
		}
		if current.State.Running {
			continue
		}
		if current.State.Status != "exited" || current.State.OOMKilled || current.State.Error != "" {
			return result, ErrUnknown
		}
		if current.State.ExitCode == 122 {
			return result, fmt.Errorf("%w: guard output limit", ErrPolicy)
		}
		result.ExitCode = current.State.ExitCode
		break
	}
	data, _, err = e.call(ctx, http.MethodGet, "/containers/"+response.ID+"/logs?stdout=true&stderr=true", nil, 2*OutputLimit+32768)
	if err != nil {
		return result, err
	}
	result.Stdout, result.Stderr, err = demultiplex(data)
	if err != nil {
		return result, err
	}
	result.Recipe = Recipe
	result.ProfileDigest, _ = p.Digest()
	result.UserID = os.Getuid()
	result.StdoutDigest = Hash(result.Stdout)
	result.StderrDigest = Hash(result.Stderr)
	return result, result.Validate(p)
}
func demultiplex(data []byte) ([]byte, []byte, error) {
	out, stderr := []byte{}, []byte{}
	for len(data) > 0 {
		if len(data) < 8 || data[1] != 0 || data[2] != 0 || data[3] != 0 {
			return nil, nil, ErrPolicy
		}
		stream := data[0]
		n := int(binary.BigEndian.Uint32(data[4:8]))
		data = data[8:]
		if n > len(data) {
			return nil, nil, ErrPolicy
		}
		switch stream {
		case 1:
			out = append(out, data[:n]...)
		case 2:
			stderr = append(stderr, data[:n]...)
		default:
			return nil, nil, ErrPolicy
		}
		if len(out) > OutputLimit || len(stderr) > OutputLimit {
			return nil, nil, ErrPolicy
		}
		data = data[n:]
	}
	return out, stderr, nil
}
