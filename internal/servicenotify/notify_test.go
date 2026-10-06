//go:build linux

package servicenotify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func receiver(t *testing.T, abstract bool) (*net.UnixConn, string) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ep-notify-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	address := filepath.Join(dir, "socket")
	if abstract {
		address = "@" + strings.ReplaceAll(dir, "/", "-")
	}
	c, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: address, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	raw, err := c.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var inner error
	if err := raw.Control(func(fd uintptr) { inner = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_PASSCRED, 1) }); err != nil || inner != nil {
		t.Fatal(err, inner)
	}
	return c, address
}

func packet(t *testing.T, c *net.UnixConn, want bool) {
	t.Helper()
	wait := 30 * time.Millisecond
	if want {
		wait = time.Second
	}
	if err := c.SetReadDeadline(time.Now().Add(wait)); err != nil {
		t.Fatal(err)
	}
	data, oob := make([]byte, 128), make([]byte, 128)
	n, on, flags, _, err := c.ReadMsgUnix(data, oob)
	if !want {
		var e net.Error
		if !errors.As(err, &e) || !e.Timeout() {
			t.Fatalf("unexpected startup datagram: n=%d err=%v", n, err)
		}
		return
	}
	if err != nil || string(data[:n]) != "READY=1" || flags & ^syscall.MSG_CMSG_CLOEXEC != 0 {
		t.Fatalf("notification mismatch: %q %v flags=%d", data[:n], err, flags)
	}
	messages, err := syscall.ParseSocketControlMessage(oob[:on])
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatal("missing sender credentials", len(messages))
	}
	cred, err := syscall.ParseUnixCredentials(&messages[0])
	if err != nil || int(cred.Pid) != os.Getpid() || int(cred.Uid) != os.Getuid() || int(cred.Gid) != os.Getgid() {
		t.Fatal("wrong notification process", cred, err)
	}
}

func TestReadyDatagramHasMainProcessCredentials(t *testing.T) {
	for _, abstract := range []bool{false, true} {
		t.Run(fmt.Sprint(abstract), func(t *testing.T) {
			c, address := receiver(t, abstract)
			n, err := New(address)
			if err != nil {
				t.Fatal(err)
			}
			if err = n.ready(context.Background()); err != nil {
				t.Fatal(err)
			}
			packet(t, c, true)
		})
	}
}

func TestNoNotificationBeforeActualAcceptLoop(t *testing.T) {
	c, address := receiver(t, false)
	n, _ := New(address)
	bound, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer bound.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wrapped := n.Listener(ctx, bound)
	packet(t, c, false) // bind/constructor alone is not startup completion
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") })}
	defer server.Close()
	done := make(chan error, 1)
	go func() { done <- server.Serve(wrapped) }()
	packet(t, c, true)
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	for range 2 {
		r, err := client.Get("http://" + bound.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
	}
	packet(t, c, false) // not emitted on every Accept
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, http.ErrServerClosed) {
		t.Fatal(err)
	}
}

func TestTLSSetupFailureDoesNotNotify(t *testing.T) {
	c, address := receiver(t, false)
	n, _ := New(address)
	bound, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer bound.Close()
	// Missing TLS identity must fail before the wrapped real Accept is entered.
	if err := (&http.Server{}).ServeTLS(n.Listener(context.Background(), bound), "", ""); err == nil {
		t.Fatal("missing TLS accepted")
	}
	packet(t, c, false)
}

type rejectAccept struct{ calls int }

func (l *rejectAccept) Accept() (net.Conn, error) {
	l.calls++
	return nil, errors.New("unexpected underlying accept")
}
func (*rejectAccept) Close() error   { return nil }
func (*rejectAccept) Addr() net.Addr { return &net.TCPAddr{} }

func TestNotifyFailureCannotFallbackOrRetry(t *testing.T) {
	c, address := receiver(t, false)
	c.Close()
	n, _ := New(address)
	bound := &rejectAccept{}
	wrapped := n.Listener(context.Background(), bound)
	if _, err := wrapped.Accept(); err == nil {
		t.Fatal("unreachable manager accepted")
	}
	// Even if a receiver appears, an already failed listener never retries.
	live, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: address, Net: "unixgram"})
	if err != nil {
		os.Remove(address)
		live, err = net.ListenUnixgram("unixgram", &net.UnixAddr{Name: address, Net: "unixgram"})
	}
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if _, err = wrapped.Accept(); err == nil || bound.calls != 0 {
		t.Fatal("fallback accept/retry occurred")
	}
	packet(t, live, false)
}

func TestCancelledStartupDoesNotNotify(t *testing.T) {
	c, address := receiver(t, false)
	n, _ := New(address)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	bound := &rejectAccept{}
	if _, err := n.Listener(ctx, bound).Accept(); !errors.Is(err, context.Canceled) || bound.calls != 0 {
		t.Fatal("cancelled startup accepted", err)
	}
	packet(t, c, false)
}

func TestNotificationAddressAndEnvironment(t *testing.T) {
	if _, err := New("@a\x00b"); err == nil {
		t.Fatal("embedded NUL accepted")
	}
	for _, address := range []string{"localhost:1234", "relative", "/", "@", "/tmp/../x", "/tmp//x", "/tmp/x\nREADY=1", "/" + strings.Repeat("a", 108)} {
		t.Run(fmt.Sprintf("%q", address), func(t *testing.T) {
			t.Setenv("NOTIFY_SOCKET", address)
			if _, err := FromEnvironment(); err == nil {
				t.Fatal("bad address accepted")
			}
			if _, ok := os.LookupEnv("NOTIFY_SOCKET"); ok {
				t.Fatal("manager socket inherited")
			}
		})
	}
	t.Setenv("NOTIFY_SOCKET", "")
	n, err := FromEnvironment()
	if err != nil || n.ready(context.Background()) != nil {
		t.Fatal("direct startup broke", err)
	}
	c, address := receiver(t, false)
	t.Setenv("NOTIFY_SOCKET", address)
	n, err = FromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := os.LookupEnv("NOTIFY_SOCKET"); ok {
		t.Fatal("notification address not consumed")
	}
	if err = n.ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	packet(t, c, true)
}
