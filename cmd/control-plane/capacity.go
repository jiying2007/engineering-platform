package main

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"log"
	"runtime/debug"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/api"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/distribution"
	"github.com/jiying2007/engineering-platform/internal/production"
)

// Only bytes already loaded and validated for this startup are used here. Do
// not reopen policy/certificate paths: they could now describe another source.
// An unidentified development/test build loses this new diagnostic only.
func capacityFromLoaded(policy *access.Policy, policyBytes []byte, transport *tls.Config, info *debug.BuildInfo) (*api.OperationalCapacityOptions, error) {
	if policy == nil || len(policyBytes) == 0 || transport == nil || len(transport.Certificates) != 1 ||
		len(transport.Certificates[0].Certificate) == 0 || transport.GetCertificate != nil || transport.GetConfigForClient != nil ||
		info == nil || info.Path != distribution.Module+"/cmd/control-plane" {
		return nil, fmt.Errorf("identified loaded Core configuration required")
	}
	settings := map[string]string{}
	for _, setting := range info.Settings {
		if _, duplicate := settings[setting.Key]; duplicate {
			return nil, fmt.Errorf("ambiguous Core build identity")
		}
		settings[setting.Key] = setting.Value
	}
	if settings["vcs"] != "git" || settings["vcs.modified"] != "false" || settings["GOOS"] != "linux" || settings["GOARCH"] != "amd64" {
		return nil, fmt.Errorf("clean source-identified Linux/amd64 Core build required")
	}
	certificate := transport.Certificates[0].Certificate[0]
	parsed, err := x509.ParseCertificate(certificate)
	if err != nil || time.Now().Before(parsed.NotBefore) || !time.Now().Before(parsed.NotAfter) {
		return nil, fmt.Errorf("current loaded Core server certificate required")
	}
	workers := []production.WorkerCapacityGrant{}
	for _, grant := range policy.WorkerClaimGrants() {
		lane := "ADMISSION"
		if grant.Prepares {
			lane = "PREPARATION"
		}
		workers = append(workers, production.WorkerCapacityGrant{Subject: grant.Subject, Profiles: grant.Profiles, Lane: lane})
	}
	if _, err := production.CapacityProfiles(workers); err != nil {
		return nil, err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	source := production.CapacitySource{
		ServerCertificateDigest: canonical.BytesDigest(certificate), AccessPolicyDigest: canonical.BytesDigest(policyBytes),
		SourceCommit: settings["vcs.revision"], InstanceID: hex.EncodeToString(nonce[:]),
	}
	if err := source.Validate(); err != nil {
		return nil, err
	}
	return &api.OperationalCapacityOptions{Source: source, Workers: workers}, nil
}

func logCapacitySource(options *api.OperationalCapacityOptions) {
	if options == nil {
		log.Print("Core capacity observation unavailable: no bounded clean source identity")
		return
	}
	source := options.Source
	// This non-secret record is for an independently selected host's operator.
	// Reading it does not grant core:read or prove whole-deployment readiness.
	log.Printf("Core capacity source instance=%s source=%s certificate=%s policy=%s", source.InstanceID, source.SourceCommit, source.ServerCertificateDigest, source.AccessPolicyDigest)
}
