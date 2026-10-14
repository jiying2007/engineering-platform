package servicelifecycle

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestBindHTTPPreservesRootAndRejectsDetachedHooks(t *testing.T) {
	type key struct{}
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), key{}, "retained"), time.Minute)
	defer cancel()
	server := &http.Server{}
	if err := BindHTTP(ctx, server); err != nil {
		t.Fatal(err)
	}
	bound := server.BaseContext(nil)
	if bound != ctx || bound.Value(key{}) != "retained" {
		t.Fatal("binding replaced the caller context")
	}
	if deadline, ok := bound.Deadline(); !ok {
		t.Fatal("deadline missing")
	} else if original, _ := ctx.Deadline(); !deadline.Equal(original) {
		t.Fatal("invented deadline")
	}
	cancel()
	if !errors.Is(bound.Err(), context.Canceled) {
		t.Fatal("binding dropped cancellation")
	}
	for _, kind := range []string{"nil-context", "unowned-root", "nil-server", "cancelled", "base-hook", "connection-hook"} {
		t.Run(kind, func(t *testing.T) {
			root, stop := context.WithCancel(context.Background())
			defer stop()
			var input context.Context = root
			s := &http.Server{}
			switch kind {
			case "nil-context":
				input = nil
			case "unowned-root":
				input = context.Background()
			case "nil-server":
				s = nil
			case "cancelled":
				stop()
			case "base-hook":
				s.BaseContext = func(net.Listener) context.Context { return context.Background() }
			case "connection-hook":
				s.ConnContext = func(context.Context, net.Conn) context.Context { return context.Background() }
			}
			if err := BindHTTP(input, s); err == nil {
				t.Fatal("unsupported context ownership admitted")
			}
			if kind == "connection-hook" && s.BaseContext != nil {
				t.Fatal("rejected binding partially mutated server")
			}
		})
	}
}
