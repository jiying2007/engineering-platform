package codexapp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestClientWritesJSONLRequestsAndClassifiesMessages(t *testing.T) {
	input := strings.NewReader(
		"{\"id\":1,\"result\":{\"ok\":true}}\n" +
			"{\"method\":\"thread/event\",\"params\":{\"kind\":\"progress\"}}\n" +
			"{\"id\":7,\"method\":\"approval/request\",\"params\":{\"action\":\"shell\"}}\n",
	)
	var output bytes.Buffer
	client := NewClient(input, &output)

	id, err := client.Request(context.Background(), "thread/start", map[string]any{"cwd": "/workspace"})
	if err != nil {
		t.Fatal(err)
	}
	if id != 1 {
		t.Fatalf("expected first request id 1, got %d", id)
	}
	if err := client.Notify(context.Background(), "client/ready", map[string]any{"ready": true}); err != nil {
		t.Fatal(err)
	}

	var events []Event
	for event := range client.Events() {
		events = append(events, event)
	}
	if err := client.ReadError(); err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 ||
		events[0].Kind != Response ||
		events[1].Kind != Notification ||
		events[2].Kind != ServerRequest {
		t.Fatalf("unexpected events %#v", events)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two JSONL messages, got %q", output.String())
	}
	var request map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &request); err != nil {
		t.Fatal(err)
	}
	if request["method"] != "thread/start" || request["id"].(float64) != 1 {
		t.Fatalf("unexpected request %#v", request)
	}
}

func TestClientRespondsToServerRequest(t *testing.T) {
	input := strings.NewReader("")
	var output bytes.Buffer
	client := NewClient(input, &output)
	<-client.Done()

	// Closed clients fail closed instead of silently dropping approval responses.
	if err := client.Respond(context.Background(), json.RawMessage("7"), map[string]any{"approved": false}, nil); err != ErrClientClosed {
		t.Fatalf("expected ErrClientClosed, got %v", err)
	}
}
