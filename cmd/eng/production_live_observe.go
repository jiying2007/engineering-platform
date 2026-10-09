package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/githubpublish"
	"github.com/jiying2007/engineering-platform/internal/production"
)

// This is a live host diagnostic assembled from existing, independent
// authorized observations. It never changes OperationalStatus v3 or grants
// production READY, provider credentials, execution, capacity or publication.
type productionLiveReport struct {
	Version int `json:"version"`
	Scope string `json:"scope"`
	SourceCommit string `json:"source_commit"`
	ObservedAt time.Time `json:"observed_at"`
	RecoveryEpoch uint64 `json:"recovery_epoch"`
	DatabaseAuthorityClear bool `json:"database_authority_clear"`
	PublisherConfigDigest string `json:"publisher_config_digest"`
	PublisherEndpointObserved bool `json:"publisher_endpoint_observed"`
	LocalServiceProcessesObserved bool `json:"local_service_processes_observed"`
	Services []production.LiveServiceObservation `json:"services"`
	ProviderLiveQualified bool `json:"provider_live_qualified"`
	CapacityObserved bool `json:"capacity_observed"`
	Ready bool `json:"ready"`
	ProductionQualified bool `json:"production_qualified"`
	State string `json:"state"`
}

func parseProductionLiveObserve(args []string) (string, error) {
	fs := flag.NewFlagSet("production-live-observe", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	config := fs.String("config", "", "owner-private deployed production preflight configuration")
	seen := map[string]bool{}
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		k, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if seen[k] {
			return "", fmt.Errorf("duplicate live-observation flag")
		}
		seen[k] = true
	}
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *config == "" {
		return "", fmt.Errorf("usage: eng production-live-observe --config PRODUCTION.json")
	}
	return *config, nil
}

func sameLiveServiceIdentities(first, second []production.LiveServiceObservation) bool {
	if len(first) != 4 || len(second) != 4 {
		return false
	}
	for i := range first {
		if first[i].Unit != second[i].Unit || first[i].ServiceUser != second[i].ServiceUser ||
			first[i].PID != second[i].PID || first[i].Binary != second[i].Binary ||
			first[i].ObservedAt.IsZero() || second[i].ObservedAt.IsZero() ||
			second[i].ObservedAt.Before(first[i].ObservedAt) {
			return false
		}
	}
	return true
}

func productionLiveObserve(args []string) error {
	file, err := parseProductionLiveObserve(args)
	if err != nil {
		return err
	}
	cfg, err := loadProductionConfig(file)
	if err != nil {
		return err
	}
	// The real production-host preflight is a prerequisite, not merely an
	// externally supplied HOST_VALIDATED assertion.
	host, err := production.Check(cfg)
	if err != nil || !host.HostValidated || host.SourceCommit != cfg.ExpectedSourceCommit {
		return fmt.Errorf("source-bound production host preflight not verified: %v", err)
	}
	endpoint, remoteConfig, err := production.LiveControlLinks(cfg)
	if err != nil {
		return err
	}
	// A valid mTLS URI pointing at a different Control installation is NOT
	// proof about this deployment. Refuse operator-side environment drift.
	if os.Getenv("CONTROL_ENDPOINT") != endpoint {
		return fmt.Errorf("operator mTLS Control endpoint differs from the deployed listener")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	before, err := production.ObserveLiveServices(ctx, cfg)
	if err != nil {
		return err
	}
	client, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return err
	}
	defer client.Close()
	data, err := client.Raw(ctx, http.MethodGet, "/api/v1/operations/status", nil)
	if err != nil {
		return err
	}
	status, err := decodeProductionStatus(data, time.Now().UTC())
	if err != nil {
		return err
	}
	if !status.AuthorityClear {
		return fmt.Errorf("deployed Control database authority is not clear: %s", status.AuthorityState)
	}
	publisher, err := githubpublish.LoadRemoteHealth(ctx, remoteConfig)
	if err != nil {
		return err
	}
	if publisher.Version != 1 || publisher.Status != "PUBLISHER_ENDPOINT_OBSERVED" ||
		!publisher.EndpointObserved || publisher.UpstreamObserved || publisher.CapacityObserved ||
		publisher.PublicationAuthorized || publisher.ExecutionAuthorized || publisher.ProductionQualified ||
		publisher.ConfigurationDigest == "" {
		return fmt.Errorf("publisher health must be authenticated endpoint observation only")
	}
	after, err := production.ObserveLiveServices(ctx, cfg)
	if err != nil || !sameLiveServiceIdentities(before, after) {
		return fmt.Errorf("deployed service process identity changed during observation: %v", err)
	}
	if err := status.ValidateAt(time.Now().UTC()); err != nil {
		return err
	}
	// Endpoint/process availability is not capacity or a qualified model lane.
	// No caller-controlled flag can promote this report to production READY.
	printJSON(productionLiveReport{
		Version: 1, Scope: "bounded-host-live-service-observations",
		SourceCommit: host.SourceCommit, ObservedAt: time.Now().UTC(),
		RecoveryEpoch: status.Snapshot.RecoveryEpoch, DatabaseAuthorityClear: true,
		PublisherConfigDigest: publisher.ConfigurationDigest,
		PublisherEndpointObserved: true, LocalServiceProcessesObserved: true,
		Services: after, State: "LOCAL_COMPONENTS_OBSERVED_PROVIDER_CAPACITY_PENDING",
	})
	return nil
}
