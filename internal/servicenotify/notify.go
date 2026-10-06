// Package servicenotify implements only systemd's local READY=1 datagram.
// It does not assert upstream availability, capacity or production qualification.
package servicenotify

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FromEnvironment consumes the manager's notification address before any child
// process can inherit it. Empty means direct/unmanaged startup, not a fallback
// after a failed notification. With an address, failure is always fatal.
func FromEnvironment() (Notifier, error) {
	address := os.Getenv("NOTIFY_SOCKET")
	if err := os.Unsetenv("NOTIFY_SOCKET"); err != nil {
		return Notifier{}, fmt.Errorf("clear manager notification environment")
	}
	return New(address)
}

type Notifier struct{ address string }

// New accepts only bounded Unix-datagram addresses. No host/network address,
// user-supplied status body, PID override or forwarding operation is supported.
func New(address string) (Notifier, error) {
	if address == "" {
		return Notifier{}, nil
	}
	if len(address) > 107 || strings.IndexFunc(address, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
		return Notifier{}, fmt.Errorf("invalid manager notification address")
	}
	switch address[0] {
	case '/':
		if address == "/" || filepath.Clean(address) != address {
			return Notifier{}, fmt.Errorf("invalid manager notification path")
		}
	case '@':
		if len(address) == 1 {
			return Notifier{}, fmt.Errorf("empty abstract notification name")
		}
	default:
		return Notifier{}, fmt.Errorf("manager notification requires a Unix datagram address")
	}
	return Notifier{address: address}, nil
}

func (n Notifier) ready(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if n.address == "" {
		return nil
	}
	bounded, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	connection, err := (&net.Dialer{}).DialContext(bounded, "unixgram", n.address)
	if err != nil {
		return fmt.Errorf("connect manager notification socket")
	}
	defer connection.Close()
	deadline, _ := bounded.Deadline()
	if err := connection.SetWriteDeadline(deadline); err != nil {
		return fmt.Errorf("bound manager notification write")
	}
	stop := context.AfterFunc(bounded, func() { _ = connection.Close() })
	defer stop()
	if bounded.Err() != nil {
		return bounded.Err()
	}
	size, err := connection.Write([]byte("READY=1"))
	if err != nil || size != len("READY=1") {
		return fmt.Errorf("send manager startup notification")
	}
	return nil
}

type listener struct {
	net.Listener
	ctx      context.Context
	notifier Notifier
	once     sync.Once
	err      error
}

// Listener reports initialization only on entry to the real serving accept
// loop, after caller configuration/schema checks, bind, and ServeTLS setup.
// A failed notification prevents Accept, and can never turn into an unmanaged
// success or a retry. The manager authenticates the sender with NotifyAccess=main.
func (n Notifier) Listener(ctx context.Context, bound net.Listener) net.Listener {
	return &listener{Listener: bound, ctx: ctx, notifier: n}
}

func (l *listener) Accept() (net.Conn, error) {
	l.once.Do(func() { l.err = l.notifier.ready(l.ctx) })
	if l.err != nil {
		return nil, l.err
	}
	return l.Listener.Accept()
}
