package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/sourcecheckpoint"
	"io"
	"time"
)

func sourceCheckpoint(args []string) error {
	if len(args) == 0 || (args[0] != "verify" && args[0] != "restore") {
		return fmt.Errorf("usage: eng source-checkpoint verify|restore --archive FILE --digest SHA256 --run ID [--destination NEW_PRIVATE_DIR]")
	}
	fs := flag.NewFlagSet("source-checkpoint", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	archive := fs.String("archive", "", "private source archive")
	digest := fs.String("digest", "", "exact digest read from authenticated Core checkpoint")
	run := fs.String("run", "", "exact Run ID")
	destination := fs.String("destination", "", "new recovery directory (not live slot)")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || *archive == "" || *digest == "" || *run == "" || (args[0] == "verify" && *destination != "") || (args[0] == "restore" && *destination == "") {
		return fmt.Errorf("explicit archive/digest/run and restore destination required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var facts codexexec.SourceCheckpoint
	var err error
	status := "SOURCE_BYTES_VERIFIED"
	if args[0] == "verify" {
		facts, err = sourcecheckpoint.Verify(ctx, *archive, *digest, *run)
	} else {
		facts, err = sourcecheckpoint.Restore(ctx, *archive, *digest, *run, *destination)
		status = "SOURCE_BYTES_RESTORED"
	}
	if err != nil {
		return err
	}
	descriptorDigest, err := facts.Digest()
	if err != nil {
		return err
	}
	printJSON(struct {
		DescriptorDigest    string                     `json:"descriptor_digest"`
		Status              string                     `json:"status"`
		Facts               codexexec.SourceCheckpoint `json:"facts"`
		ExecutionAuthorized bool                       `json:"execution_authorized"`
	}{DescriptorDigest: descriptorDigest, Status: status, Facts: facts})
	return nil
}
