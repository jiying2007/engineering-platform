package codexapp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func pair(t *testing.T) (*Client, net.Conn) {
	t.Helper()
	a, b := net.Pipe()
	c := NewClient(a, a)
	t.Cleanup(func() {
		_ = b.Close()
		_ = c.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = c.Wait(ctx)
	})
	return c, b
}
func deadline(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func TestCallCorrelatesOutOfOrderAndPreservesServerRequests(t *testing.T) {
	c, p := pair(t)
	ctx := deadline(t)
	server := make(chan error, 1)
	go func() {
		d := json.NewDecoder(p)
		var a, b map[string]json.RawMessage
		if err := d.Decode(&a); err != nil {
			server <- err
			return
		}
		if err := d.Decode(&b); err != nil {
			server <- err
			return
		}
		_, err := io.WriteString(p, `{"method":"progress","params":{}}`+"\n"+`{"id":`+string(b["id"])+`,"result":`+string(b["method"])+"}\n"+`{"id":`+string(a["id"])+`,"result":`+string(a["method"])+"}\n"+`{"id":"srv","method":"approval/request","params":{}}`+"\n")
		server <- err
	}()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, method := range []string{"one", "two"} {
		wg.Add(1)
		go func(method string) {
			defer wg.Done()
			var got string
			err := c.Call(ctx, method, nil, &got)
			if err == nil && got != method {
				err = errors.New("wrong correlation")
			}
			errs <- err
		}(method)
	}
	wg.Wait()
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []EventKind{Notification, ServerRequest} {
		select {
		case e := <-c.Events():
			if e.Kind != kind {
				t.Fatal(e)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if err := <-server; err != nil {
		t.Fatal(err)
	}
}
func TestClientCloseInterruptsReaderAndBlockedWriter(t *testing.T) {
	c, _ := pair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := c.Request(ctx, "blocked", nil); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("blocked write succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("write leak")
	}
	wait, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	_ = c.Wait(wait)
	if wait.Err() != nil {
		t.Fatal("reader leak")
	}
}
func TestClientBackpressureTerminatesWithoutConsumer(t *testing.T) {
	c, p := pair(t)
	go func() {
		for i := 0; i < 200; i++ {
			if _, err := io.WriteString(p, "{\"method\":\"event\",\"params\":{}}\n"); err != nil {
				return
			}
		}
	}()
	select {
	case <-c.Done():
		if !errors.Is(c.ReadError(), ErrBackpressure) {
			t.Fatal(c.ReadError())
		}
	case <-time.After(time.Second):
		t.Fatal("backpressure blocked")
	}
}
func TestMalformedFramesFailClosed(t *testing.T) {
	for _, text := range []string{`{}`, `null`, `{"id":1,"id":2,"result":{}}`, `{"ID":1,"result":{}}`, `{"id":null,"result":{}}`, `{"id":1,"result":{},"error":{}}`, `{"id":1,"error":null}`, `{"method":"x","result":{}}`, `{"id":-1,"result":{}}`, `{"id":1.0,"result":{}}`, `{"id":1,"result":{}} {}`} {
		t.Run(text, func(t *testing.T) {
			c, p := pair(t)
			go func() { _, _ = io.WriteString(p, text+"\n") }()
			select {
			case <-c.Done():
				if !errors.Is(c.ReadError(), ErrProtocol) {
					t.Fatal(c.ReadError())
				}
			case <-time.After(time.Second):
				t.Fatal("malformed frame accepted")
			}
		})
	}
}

func TestNotificationEmissionMetadataIsAcceptedButNeverCorrelates(t *testing.T) {
	c, p := pair(t)
	go func() {
		_, _ = io.WriteString(p, `{"method":"thread/started","params":{},"emittedAtMs":1234}`+"\n")
	}()
	select {
	case event := <-c.Events():
		if event.Kind != Notification || event.Message.EmittedAtMs == nil || *event.Message.EmittedAtMs != 1234 {
			t.Fatalf("unexpected notification metadata: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("notification metadata blocked")
	}

	for _, frame := range []string{
		`{"id":1,"result":{},"emittedAtMs":1234}`,
		`{"id":1,"method":"approval/request","params":{},"emittedAtMs":1234}`,
		`{"method":"thread/started","params":{},"emittedAtMs":-1}`,
	} {
		if _, err := decodeMessage([]byte(frame)); !errors.Is(err, ErrProtocol) {
			t.Fatalf("invalid emittedAtMs envelope accepted: %s err=%v", frame, err)
		}
	}
}

func TestOversizedFrameFailsClosed(t *testing.T) {
	c, p := pair(t)
	go func() { _, _ = io.WriteString(p, strings.Repeat("x", MaxFrameBytes+1)+"\n") }()
	select {
	case <-c.Done():
		if c.ReadError() == nil {
			t.Fatal("oversize accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("oversize blocked")
	}
}
func TestRPCErrorAndTimeoutAreNotRetried(t *testing.T) {
	c, p := pair(t)
	go func() {
		s := bufio.NewScanner(p)
		if s.Scan() {
			var m Message
			_ = json.Unmarshal(s.Bytes(), &m)
			_, _ = io.WriteString(p, `{"id":`+string(m.ID)+`,"error":{"code":-1,"message":"denied"}}`+"\n")
		}
	}()
	err := c.Call(deadline(t), "deny", nil, nil)
	var rpc *RPCError
	if !errors.As(err, &rpc) || rpc.Code != -1 {
		t.Fatal(err)
	}
	c2, p2 := pair(t)
	go func() { s := bufio.NewScanner(p2); s.Scan() }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := c2.Call(ctx, "ambiguous", nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	select {
	case <-c2.Done():
	default:
		t.Fatal("ambiguous connection remained usable")
	}
}
func TestClientRequestResponseAndBlankMethod(t *testing.T) {
	c, p := pair(t)
	go func() {
		s := bufio.NewScanner(p)
		if s.Scan() {
			var m Message
			_ = json.Unmarshal(s.Bytes(), &m)
			_, _ = io.WriteString(p, `{"id":`+string(m.ID)+`,"result":{}}`+"\n")
		}
	}()
	id, err := c.Request(deadline(t), "method", nil)
	if err != nil || id != 1 {
		t.Fatal(id, err)
	}
	e := <-c.Events()
	if e.Kind != Response {
		t.Fatal(e)
	}
	if _, err := c.Request(context.Background(), " ", nil); err == nil {
		t.Fatal("blank method accepted")
	}
	_ = c.Close()
	if err := c.Respond(context.Background(), json.RawMessage(`"x"`), nil, nil); !errors.Is(err, ErrClientClosed) {
		t.Fatal(err)
	}
}

type shortWriter struct {
	sync.Mutex
	data []byte
	zero bool
}

func (w *shortWriter) Write(p []byte) (int, error) {
	w.Lock()
	defer w.Unlock()
	if w.zero {
		return 0, nil
	}
	if len(p) > 3 {
		p = p[:3]
	}
	w.data = append(w.data, p...)
	return len(p), nil
}
func (w *shortWriter) Close() error { return nil }
func TestShortWritesAndZeroProgress(t *testing.T) {
	for _, zero := range []bool{false, true} {
		r, w := io.Pipe()
		out := &shortWriter{zero: zero}
		c := NewClient(r, out)
		err := c.Notify(context.Background(), "hello", nil)
		if zero && !errors.Is(err, io.ErrShortWrite) {
			t.Fatal(err)
		}
		if !zero && (err != nil || !strings.HasSuffix(string(out.data), "\n")) {
			t.Fatal(err)
		}
		_ = c.Close()
		_ = w.Close()
	}
}

func TestFinalReplyBeforeEOFIsRetained(t *testing.T) {
	for i := 0; i < 30; i++ {
		c, p := pair(t)
		go func() {
			var m Message
			_ = json.NewDecoder(p).Decode(&m)
			_, _ = io.WriteString(p, `{"id":`+string(m.ID)+`,"result":true}`+"\n")
			_ = p.Close()
		}()
		var result bool
		if err := c.Call(deadline(t), "last", nil, &result); err != nil || !result {
			t.Fatal("EOF lost reply", err)
		}
	}
}
func TestPendingLimitAndNilEndpoints(t *testing.T) {
	r, w := io.Pipe()
	out := &shortWriter{}
	c := NewClient(r, out)
	defer c.Close()
	defer w.Close()
	for i := 0; i < maxPending; i++ {
		if _, err := c.Request(context.Background(), "unanswered", nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.Request(context.Background(), "overflow", nil); !errors.Is(err, ErrRequestLimit) {
		t.Fatal(err)
	}
	bad := NewClient(nil, nil)
	if _, err := bad.Request(context.Background(), "invalid", nil); !errors.Is(err, ErrClientClosed) {
		t.Fatal(err)
	}
}
