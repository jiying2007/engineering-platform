package codexapp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
)

var ErrClientClosed = errors.New("codex app-server client is closed")

// Message models the JSON-RPC-lite shape used by Codex App Server.
// The transport deliberately does not encode Codex-specific method names.
// Those belong in a versioned protocol adapter generated from the App Server schema.
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

type Client struct {
	writer io.Writer
	events chan Event
	done   chan struct{}

	writeMu sync.Mutex
	nextID  atomic.Uint64
	once    sync.Once
	errMu   sync.RWMutex
	readErr error
}

func NewClient(reader io.Reader, writer io.Writer) *Client {
	client := &Client{
		writer: writer,
		events: make(chan Event, 64),
		done:   make(chan struct{}),
	}
	go client.readLoop(reader)
	return client
}

func (c *Client) Events() <-chan Event {
	return c.events
}

func (c *Client) Done() <-chan struct{} {
	return c.done
}

func (c *Client) ReadError() error {
	c.errMu.RLock()
	defer c.errMu.RUnlock()
	return c.readErr
}

func (c *Client) Request(ctx context.Context, method string, params any) (uint64, error) {
	if strings.TrimSpace(method) == "" {
		return 0, fmt.Errorf("method is required")
	}
	id := c.nextID.Add(1)
	return id, c.write(ctx, map[string]any{
		"id":     id,
		"method": method,
		"params": params,
	})
}

func (c *Client) Notify(ctx context.Context, method string, params any) error {
	if strings.TrimSpace(method) == "" {
		return fmt.Errorf("method is required")
	}
	return c.write(ctx, map[string]any{
		"method": method,
		"params": params,
	})
}

func (c *Client) Respond(ctx context.Context, id json.RawMessage, result any, rpcErr any) error {
	if len(id) == 0 {
		return fmt.Errorf("response id is required")
	}
	message := map[string]any{"id": json.RawMessage(id)}
	if rpcErr != nil {
		message["error"] = rpcErr
	} else {
		message["result"] = result
	}
	return c.write(ctx, message)
}

func (c *Client) write(ctx context.Context, value any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return ErrClientClosed
	default:
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal app-server message: %w", err)
	}
	payload = append(payload, '\n')

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := c.writer.Write(payload); err != nil {
		return fmt.Errorf("write app-server message: %w", err)
	}
	return nil
}

func (c *Client) readLoop(reader io.Reader) {
	defer c.once.Do(func() {
		close(c.done)
		close(c.events)
	})

	scanner := bufio.NewScanner(reader)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 8*1024*1024)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		var message Message
		if err := json.Unmarshal(line, &message); err != nil {
			c.setReadError(fmt.Errorf("decode app-server message: %w", err))
			return
		}
		c.events <- Event{Kind: classify(message), Message: message}
	}
	if err := scanner.Err(); err != nil {
		c.setReadError(fmt.Errorf("read app-server stream: %w", err))
	}
}

func (c *Client) setReadError(err error) {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	c.readErr = err
}

func classify(message Message) EventKind {
	if message.Method != "" && len(message.ID) > 0 {
		return ServerRequest
	}
	if message.Method != "" {
		return Notification
	}
	return Response
}
