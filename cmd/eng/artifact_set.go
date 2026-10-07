package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/artifactset"
)

type artifactSetOptions struct{ command, plan, planDigest, archive, archiveDigest, run, out, into string }

func parseArtifactSet(args []string) (artifactSetOptions, error) {
	var o artifactSetOptions
	if len(args) == 0 {
		return o, fmt.Errorf("artifact-set requires pack, verify, restore or mirror")
	}
	o.command = args[0]
	fs := flag.NewFlagSet("artifact-set "+o.command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	switch o.command {
	case "pack":
		fs.StringVar(&o.plan, "plan", "", "explicit private input declaration")
		fs.StringVar(&o.planDigest, "plan-digest", "", "externally anchored raw plan SHA256")
		fs.StringVar(&o.out, "out", "", "new private archive path")
	case "verify", "restore", "mirror":
		fs.StringVar(&o.archive, "archive", "", "explicit private archive path")
		fs.StringVar(&o.archiveDigest, "archive-digest", "", "externally anchored archive SHA256")
		fs.StringVar(&o.run, "run", "", "expected original Run")
		if o.command == "restore" {
			fs.StringVar(&o.into, "into", "", "new private restore directory")
		}
		if o.command == "mirror" {
			fs.StringVar(&o.out, "out", "", "new archive path on a distinct private filesystem")
		}
	default:
		return o, fmt.Errorf("unsupported artifact-set command")
	}
	seen := map[string]bool{}
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") {
			key, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			if seen[key] {
				return o, fmt.Errorf("duplicate artifact-set flag")
			}
			seen[key] = true
		}
	}
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
		return o, fmt.Errorf("invalid artifact-set arguments")
	}
	if o.command == "pack" && (o.plan == "" || o.planDigest == "" || o.out == "") || o.command != "pack" && (o.archive == "" || o.archiveDigest == "" || o.run == "") || o.command == "restore" && o.into == "" || o.command == "mirror" && o.out == "" {
		return o, fmt.Errorf("explicit paths, identities and digest anchors required")
	}
	return o, nil
}
func executeArtifactSet(ctx context.Context, args []string) (artifactset.Report, error) {
	o, err := parseArtifactSet(args)
	if err != nil {
		return artifactset.Report{}, err
	}
	switch o.command {
	case "pack":
		return artifactset.Pack(ctx, o.plan, o.planDigest, o.out)
	case "verify":
		return artifactset.Verify(ctx, o.archive, o.archiveDigest, o.run)
	case "restore":
		return artifactset.Restore(ctx, o.archive, o.archiveDigest, o.run, o.into)
	case "mirror":
		return artifactset.Mirror(ctx, o.archive, o.archiveDigest, o.run, o.out)
	default:
		return artifactset.Report{}, fmt.Errorf("invalid artifact-set command")
	}
}
func artifactSet(args []string) error {
	if len(args) > 0 && args[0] == "capture-offline" {
		return captureOffline(args[1:])
	}
	if len(args) > 0 && args[0] == "capture-execution" {
		return captureExecution(args[1:])
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	r, err := executeArtifactSet(ctx, args)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(r)
}
