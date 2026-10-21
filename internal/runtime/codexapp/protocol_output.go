package codexapp

import (
	"os"
	"os/exec"
)

// protocolOutput gives the protocol reader ownership of its file descriptor.
// Cmd.Wait owns process reaping and may run before the reader is scheduled;
// StdoutPipe would close that reader before its final buffered frame is read.
// The caller closes writer immediately after Start and reader after the client.
func protocolOutput(cmd *exec.Cmd) (reader, writer *os.File, err error) {
	reader, writer, err = os.Pipe()
	if err == nil {
		cmd.Stdout = writer
	}
	return reader, writer, err
}
