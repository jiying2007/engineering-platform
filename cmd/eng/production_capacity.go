package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/production"
)

func parseProductionCapacity(args []string) (production.CapacitySource, error) {
	var source production.CapacitySource
	fs := flag.NewFlagSet("production-capacity", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&source.ServerCertificateDigest, "server-cert-digest", "", "expected Core server leaf certificate SHA256")
	fs.StringVar(&source.AccessPolicyDigest, "access-policy-digest", "", "expected loaded access policy raw-byte SHA256")
	fs.StringVar(&source.SourceCommit, "source-commit", "", "expected clean Control executable source commit")
	fs.StringVar(&source.InstanceID, "instance", "", "expected instance nonce from selected Core startup record")
	seen := map[string]bool{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			key, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			if seen[key] {
				return source, fmt.Errorf("duplicate production-capacity flag")
			}
			seen[key] = true
		}
	}
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || source.Validate() != nil {
		return source, fmt.Errorf("production-capacity requires externally expected --server-cert-digest --access-policy-digest --source-commit --instance")
	}
	return source, nil
}

func writeProductionCapacity(ctx context.Context, client *controlclient.Client, expected production.CapacitySource, out io.Writer) error {
	if expected.Validate() != nil {
		return fmt.Errorf("externally expected Core capacity identity required")
	}
	read, err := client.ReadCapacity(ctx, expected.ServerCertificateDigest)
	if err != nil {
		return err
	}
	observation, err := production.ObserveCapacity(read.Body, read.PeerCertificateDigest, read.RequestedAt, read.ReceivedAt, expected)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(observation, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := ctx.Err(); err != nil {
		return err
	}
	n, err := out.Write(data)
	if err != nil {
		return fmt.Errorf("capacity observation output failed: %w", err)
	}
	if n != len(data) {
		return fmt.Errorf("capacity observation output failed: %w", io.ErrShortWrite)
	}
	return ctx.Err()
}

func productionCapacity(args []string) error {
	expected, err := parseProductionCapacity(args)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, closeOutput, err := prepareProductionMonitorOutput(ctx, os.Stdout)
	if err != nil {
		return err
	}
	defer closeOutput()
	client, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return fmt.Errorf("capacity observation requires configured authenticated Control access")
	}
	defer client.Close()
	return writeProductionCapacity(ctx, client, expected, out)
}
