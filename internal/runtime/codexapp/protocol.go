package codexapp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	ErrClientClosed = errors.New("codex app-server client is closed")
	ErrBackpressure = errors.New("codex event consumer exceeded bounded queue")
	ErrProtocol     = errors.New("invalid codex JSONL envelope")
	ErrRequestLimit = errors.New("codex pending request limit exceeded")
)

const MaxFrameBytes = 1 << 20
const maxPending = 128

type Message struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}
type EventKind string

const (
	Response      EventKind = "RESPONSE"
	Notification  EventKind = "NOTIFICATION"
	ServerRequest EventKind = "SERVER_REQUEST"
)

type Event struct {
	Kind    EventKind `json:"kind"`
	Message Message   `json:"message"`
}
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("app-server RPC error %d", e.Code) }

type response struct {
	message Message
	err     error
}

// Client owns its two pipe endpoints. Close MUST interrupt blocked Read/Write;
// os.File pipes and net.Conn satisfy this contract. No retry occurs on timeout.
// Parameters are untrusted provider data, not a platform authorization grant.
type Client struct {
	reader    io.ReadCloser
	writer    io.WriteCloser
	events    chan Event
	done      chan struct{}
	exited    chan struct{}
	writeSlot chan struct{}
	nextID    atomic.Uint64
	once      sync.Once
	mu        sync.Mutex
	readErr   error
	pending   map[uint64]chan response
}

func NewClient(reader io.ReadCloser, writer io.WriteCloser) *Client {
	c := &Client{reader: reader, writer: writer, events: make(chan Event, 64), done: make(chan struct{}), exited: make(chan struct{}), writeSlot: make(chan struct{}, 1), pending: make(map[uint64]chan response)}
	c.writeSlot <- struct{}{}
	if reader == nil || writer == nil {
		c.stop(ErrProtocol)
	}
	go c.readLoop()
	return c
}
func (c *Client) Events() <-chan Event  { return c.events }
func (c *Client) Done() <-chan struct{} { return c.done }
func (c *Client) ReadError() error      { c.mu.Lock(); defer c.mu.Unlock(); return c.readErr }
func (c *Client) Close() error          { c.stop(nil); return nil }
func (c *Client) Wait(ctx context.Context) error {
	select {
	case <-c.exited:
		return c.ReadError()
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (c *Client) stop(err error) {
	c.once.Do(func() {
		c.mu.Lock()
		c.readErr = err
		pendingErr := err
		if pendingErr == nil {
			pendingErr = ErrClientClosed
		}
		for id, ch := range c.pending {
			if ch != nil {
				ch <- response{err: pendingErr}
			}
			delete(c.pending, id)
		}
		close(c.done)
		c.mu.Unlock()
		// Closing pipes, not an abandoned writer goroutine, interrupts blocked I/O.
		if c.reader != nil {
			_ = c.reader.Close()
		}
		if c.writer != nil {
			_ = c.writer.Close()
		}
	})
}
func (c *Client) reserve(ch chan response) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.done:
		return 0, ErrClientClosed
	default:
	}
	if len(c.pending) >= maxPending {
		return 0, ErrRequestLimit
	}
	id := c.nextID.Add(1)
	c.pending[id] = ch
	return id, nil
}
func (c *Client) forget(id uint64) { c.mu.Lock(); delete(c.pending, id); c.mu.Unlock() }
func (c *Client) Request(ctx context.Context, method string, params any) (uint64, error) {
	if strings.TrimSpace(method) == "" {
		return 0, fmt.Errorf("method required")
	}
	id, err := c.reserve(nil)
	if err != nil {
		return 0, err
	}
	err = c.write(ctx, map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		c.forget(id)
	}
	return id, err
}

// Call correlates out-of-order responses without competing for Events. A timeout
// after sending closes the connection: the remote outcome is not assumed absent.
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	if strings.TrimSpace(method) == "" {
		return fmt.Errorf("method required")
	}
	ch := make(chan response, 1)
	id, err := c.reserve(ch)
	if err != nil {
		return err
	}
	defer c.forget(id)
	if err = c.write(ctx, map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	var reply response
	select {
	case reply = <-ch:
	case <-ctx.Done():
		c.stop(ctx.Err())
		return ctx.Err()
	case <-c.done:
		// EOF after the final reply must not win over an already correlated result.
		select {
		case reply = <-ch:
		default:
			return ErrClientClosed
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if reply.err != nil {
		return reply.err
	}
	if len(reply.message.Error) > 0 {
		var rpc RPCError
		if json.Unmarshal(reply.message.Error, &rpc) != nil {
			return ErrProtocol
		}
		return &rpc
	}
	if result != nil && json.Unmarshal(reply.message.Result, result) != nil {
		c.stop(ErrProtocol)
		return ErrProtocol
	}
	return nil
}
func (c *Client) Notify(ctx context.Context, method string, params any) error {
	if strings.TrimSpace(method) == "" {
		return fmt.Errorf("method required")
	}
	return c.write(ctx, map[string]any{"method": method, "params": params})
}
func (c *Client) Respond(ctx context.Context, id json.RawMessage, result, rpcErr any) error {
	if !validID(id) {
		return ErrProtocol
	}
	m := map[string]any{"id": id}
	if rpcErr != nil {
		m["error"] = rpcErr
	} else {
		m["result"] = result
	}
	return c.write(ctx, m)
}
func (c *Client) write(ctx context.Context, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data)+1 > MaxFrameBytes {
		return fmt.Errorf("%w: outgoing frame too large", ErrProtocol)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return ErrClientClosed
	case <-c.writeSlot:
	}
	defer func() { c.writeSlot <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-c.done:
		return ErrClientClosed
	default:
	}
	interrupt := context.AfterFunc(ctx, func() { c.stop(ctx.Err()) })
	defer interrupt()
	data = append(data, '\n')
	for len(data) > 0 {
		n, err := c.writer.Write(data)
		if n < 0 || n > len(data) {
			c.stop(io.ErrShortWrite)
			return io.ErrShortWrite
		}
		if err != nil {
			c.stop(err)
			return err
		}
		if n == 0 {
			c.stop(io.ErrShortWrite)
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	if err := ctx.Err(); err != nil {
		c.stop(err)
		return err
	}
	return nil
}
func (c *Client) readLoop() {
	defer close(c.exited)
	defer close(c.events)
	defer c.stop(nil)
	if c.reader == nil || c.writer == nil {
		c.stop(ErrProtocol)
		return
	}
	scanner := bufio.NewScanner(c.reader)
	scanner.Buffer(make([]byte, 4096), MaxFrameBytes)
	for scanner.Scan() {
		m, err := decodeMessage(scanner.Bytes())
		if err != nil {
			c.stop(err)
			return
		}
		if m.Method == "" {
			id, err := strconv.ParseUint(string(m.ID), 10, 64)
			if err != nil {
				c.stop(ErrProtocol)
				return
			}
			c.mu.Lock()
			ch, known := c.pending[id]
			delete(c.pending, id)
			c.mu.Unlock()
			// Late or unsolicited responses cannot satisfy another Call.
			if !known {
				continue
			}
			if ch != nil {
				ch <- response{message: m}
				continue
			}
		}
		select {
		case <-c.done:
			return
		case c.events <- Event{Kind: classify(m), Message: m}:
		default:
			c.stop(ErrBackpressure)
			return
		}
	}
	if err := scanner.Err(); err != nil {
		select {
		case <-c.done:
		default:
			c.stop(err)
		}
	}
}
func validID(id json.RawMessage) bool {
	if len(id) == 0 {
		return false
	}
	if id[0] == '"' {
		var s string
		return json.Unmarshal(id, &s) == nil && s != "" && len(s) <= 256
	}
	_, err := strconv.ParseUint(string(id), 10, 64)
	return err == nil
}
func decodeMessage(data []byte) (Message, error) {
	// Duplicate top-level routing fields must not select a different correlation.
	d := json.NewDecoder(bytes.NewReader(data))
	tok, err := d.Token()
	if err != nil || tok != json.Delim('{') {
		return Message{}, ErrProtocol
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		tok, err = d.Token()
		if err != nil {
			return Message{}, ErrProtocol
		}
		key, ok := tok.(string)
		if !ok {
			return Message{}, ErrProtocol
		}
		if _, ok = fields[key]; ok {
			return Message{}, ErrProtocol
		}
		switch key {
		case "id", "method", "params", "result", "error", "jsonrpc":
		default:
			return Message{}, fmt.Errorf("%w: unknown top-level field %q", ErrProtocol, key)
		}
		var raw json.RawMessage
		if d.Decode(&raw) != nil {
			return Message{}, ErrProtocol
		}
		fields[key] = raw
	}
	if _, err = d.Token(); err != nil {
		return Message{}, ErrProtocol
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return Message{}, ErrProtocol
	}
	if v, ok := fields["jsonrpc"]; ok && string(v) != `"2.0"` {
		return Message{}, fmt.Errorf("%w: unexpected jsonrpc version", ErrProtocol)
	}
	var m Message
	if json.Unmarshal(data, &m) != nil {
		return m, ErrProtocol
	}
	if len(m.ID) > 0 && !validID(m.ID) {
		return m, fmt.Errorf("%w: invalid message id", ErrProtocol)
	}
	if m.Method != "" {
		if strings.TrimSpace(m.Method) == "" || len(m.Result) > 0 || len(m.Error) > 0 {
			return m, fmt.Errorf("%w: request/notification routing fields conflict", ErrProtocol)
		}
	} else {
		if len(m.ID) == 0 || (len(m.Result) > 0) == (len(m.Error) > 0) || len(m.Params) > 0 {
			return m, ErrProtocol
		}
		if len(m.Error) > 0 {
			var e RPCError
			if string(m.Error) == "null" || json.Unmarshal(m.Error, &e) != nil || e.Message == "" {
				return m, ErrProtocol
			}
		}
	}
	return m, nil
}
func classify(m Message) EventKind {
	if m.Method != "" {
		if len(m.ID) > 0 {
			return ServerRequest
		}
		return Notification
	}
	return Response
}
