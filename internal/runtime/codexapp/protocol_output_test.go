package codexapp

import (
	"context"
	"io"
	"os/exec"
	"testing"
	"time"
)

func TestProtocolOutputRetainsFrameAfterBackgroundWait(t *testing.T) {
	const frame = "{\"method\":\"turn/completed\",\"params\":{\"threadId\":\"thread-1\",\"turn\":{\"id\":\"turn-1\",\"status\":\"completed\"}}}\n"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/cat")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, writer, err := protocolOutput(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	defer writer.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	joined := make(chan error, 1)
	go func() { joined <- cmd.Wait() }()
	if _, err := io.WriteString(input, frame); err != nil {
		t.Fatal(err)
	}
	_ = input.Close()
	if err := <-joined; err != nil {
		t.Fatal(err)
	}
	// Deliberately resume the protocol reader only after the background owner
	// has reaped the process. The final frame must still belong to the reader.
	data, err := io.ReadAll(output)
	if err != nil || string(data) != frame {
		t.Fatalf("final frame lost after Wait: %q %v", data, err)
	}
}

func TestProtocolOutputReachesEOFAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sleep", "60")
	output, writer, err := protocolOutput(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	defer writer.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	joined := make(chan error, 1)
	go func() { joined <- cmd.Wait() }()
	cancel()
	if err := <-joined; err == nil {
		t.Fatal("cancelled process exited successfully")
	}
	read := make(chan error, 1)
	go func() { _, err := io.ReadAll(output); read <- err }()
	select {
	case err := <-read:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parent retained a writer after process cancellation")
	}
}
