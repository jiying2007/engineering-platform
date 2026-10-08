package githubpublish

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type serviceRemote struct {
	publishCalls int
	observeCalls int
	onPublish func(string) error
}

func (r *serviceRemote) Publish(_ context.Context, plan Plan, bundlePath string) (PublicationReceipt, error) {
	r.publishCalls++
	if r.onPublish != nil {
		if err := r.onPublish(bundlePath); err != nil {
			return PublicationReceipt{}, err
		}
	}
	return PublicationReceipt{
		Version: 1, Repository: plan.Repository, BaseRef: plan.BaseRef, BaseCommit: plan.BaseCommit,
		Branch: plan.Branch, ResultCommit: plan.ResultCommit, PullRequestNumber: 17,
		PullRequestURL:   "https://github.com/" + plan.Repository + "/pull/17",
		PullRequestState: "open", PublicationOutcome: "CREATED",
	}, nil
}

func (r *serviceRemote) Observe(_ context.Context, plan Plan) (ObserveResult, error) {
	r.observeCalls++
	return ObserveResult{Outcome: ObservedAbsent}, nil
}

func publisherServiceFixture(t *testing.T) (*RemoteService, Plan) {
	t.Helper()
	root := t.TempDir()
	execution := strings.Repeat("a", 64)
	bundle := []byte("bundle-bytes")
	sum := sha256.Sum256(bundle)
	bundleDigest := "sha256:" + hex.EncodeToString(sum[:])
	if err := os.WriteFile(filepath.Join(root, execution+".bundle"), bundle, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	remote := &serviceRemote{}
	service := &RemoteService{
		remote: remote, artifactRoot: root, artifactIdentity: info,
		targets: map[string]TargetPolicy{
			"jiying2007/engineering-platform": {
				Repository: "jiying2007/engineering-platform", BaseRef: "main",
				BranchPrefix: "engineering-platform/",
			},
		},
		controlSubject: "urn:engineering-platform:control:publisher-client",
	}
	plan := Plan{
		Version: 1, RunID: "run-1", Repository: "jiying2007/engineering-platform",
		BaseRef: "main", BaseCommit: strings.Repeat("1", 40),
		Branch:        "engineering-platform/" + execution[:24],
		ResultCommit:  strings.Repeat("2", 40),
		ResultDigest:  "sha256:" + strings.Repeat("3", 64),
		ReceiptDigest: "sha256:" + strings.Repeat("4", 64),
		BundleDigest:  bundleDigest, BundleSize: int64(len(bundle)), ExecutionID: execution,
	}
	return service, plan
}

func authorizedPublisherRequest(t *testing.T, method, path string, body []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	uri, err := url.Parse("urn:engineering-platform:control:publisher-client")
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{Raw: []byte{1}, URIs: []*url.URL{uri}}
	req.TLS = &tls.ConnectionState{
		HandshakeComplete: true, Version: tls.VersionTLS13,
		PeerCertificates: []*x509.Certificate{cert},
		VerifiedChains:   [][]*x509.Certificate{{cert}},
	}
	return req
}

func TestRemoteServicePublishesOnlyVerifiedBundleAndControlIdentity(t *testing.T) {
	service, plan := publisherServiceFixture(t)
	body, err := json.Marshal(publishRequest{Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	service.Handler().ServeHTTP(rec, authorizedPublisherRequest(t, http.MethodPost, "/v1/publish", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", rec.Code, rec.Body.String())
	}
	if service.remote.(*serviceRemote).publishCalls != 1 {
		t.Fatal("publisher remote not called exactly once")
	}

	unauthorized := httptest.NewRequest(http.MethodPost, "/v1/publish", bytes.NewReader(body))
	unauthorized.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	service.Handler().ServeHTTP(rec, unauthorized)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized publish status=%d", rec.Code)
	}
}

func TestRemoteServiceRejectsBundleDriftBeforeGitHub(t *testing.T) {
	service, plan := publisherServiceFixture(t)
	path := filepath.Join(service.artifactRoot, plan.ExecutionID+".bundle")
	if err := os.WriteFile(path, []byte("different"), 0o600); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(publishRequest{Plan: plan})
	rec := httptest.NewRecorder()
	service.Handler().ServeHTTP(rec, authorizedPublisherRequest(t, http.MethodPost, "/v1/publish", body))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("drifted bundle status=%d body=%s", rec.Code, rec.Body.String())
	}
	if service.remote.(*serviceRemote).publishCalls != 0 {
		t.Fatal("drifted bundle reached GitHub remote")
	}
}


func TestRemoteServicePublishesVerifiedPrivateSnapshotAfterSourceSwap(t *testing.T) {
	service, plan := publisherServiceFixture(t)
	original := filepath.Join(service.artifactRoot, plan.ExecutionID+".bundle")
	remote := service.remote.(*serviceRemote)
	var snapshotPath string
	remote.onPublish = func(path string) error {
		snapshotPath = path
		if path == original {
			return fmt.Errorf("mutable original path was handed to Git")
		}
		// Model/Worker-side original changes after Publisher verification.
		// The Git-side consumer must still receive exactly the hashed bytes.
		if err := os.Remove(original); err != nil {
			return err
		}
		if err := os.WriteFile(original, []byte("changed-after-verification"), 0o600); err != nil {
			return err
		}
		got, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, []byte("bundle-bytes")) {
			return fmt.Errorf("publisher passed unverified bytes to Git")
		}
		return nil
	}
	body, err := json.Marshal(publishRequest{Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	service.Handler().ServeHTTP(rec, authorizedPublisherRequest(t, http.MethodPost, "/v1/publish", body))
	if rec.Code != http.StatusOK || remote.publishCalls != 1 || snapshotPath == "" {
		t.Fatalf("snapshot publication status=%d calls=%d body=%s", rec.Code, remote.publishCalls, rec.Body.String())
	}
	if _, err := os.Stat(snapshotPath); !os.IsNotExist(err) {
		t.Fatalf("verified temporary snapshot was not removed: %v", err)
	}
}

func TestRemoteServiceObserveDoesNotNeedPublisherBundle(t *testing.T) {
	service, plan := publisherServiceFixture(t)
	body, _ := json.Marshal(observeRequest{Plan: plan})
	rec := httptest.NewRecorder()
	service.Handler().ServeHTTP(rec, authorizedPublisherRequest(t, http.MethodPost, "/v1/observe", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("observe status=%d body=%s", rec.Code, rec.Body.String())
	}
	if service.remote.(*serviceRemote).observeCalls != 1 {
		t.Fatal("observe remote not called")
	}
}

var _ Remote = (*remoteClient)(nil)
