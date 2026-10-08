package githubpublish

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

type RemoteService struct {
	remote           Remote
	artifactRoot     string
	artifactIdentity os.FileInfo
	targets          map[string]TargetPolicy
	controlSubject   string
}

type publishRequest struct {
	Plan Plan `json:"plan"`
}

type observeRequest struct {
	Plan Plan `json:"plan"`
}

func LoadRemoteService(configPath, controlSubject string) (*RemoteService, error) {
	if !validPublisherSubject(controlSubject) {
		return nil, fmt.Errorf("exact publisher control subject required")
	}
	data, err := access.ReadConfiguration(configPath, false)
	if err != nil {
		return nil, err
	}
	var config Configuration
	if err := strictjson.Decode(data, &config); err != nil {
		return nil, err
	}
	artifactRoot, info, err := canonicalDirectory(config.ArtifactRoot)
	if err != nil {
		return nil, err
	}
	remote, err := newGitHubRemote(config.GitExecutable, config.TokenFile)
	if err != nil {
		return nil, err
	}
	targets, err := targetMap(config.Version, config.Targets)
	if err != nil {
		return nil, err
	}
	return &RemoteService{
		remote: remote, artifactRoot: artifactRoot, artifactIdentity: info,
		targets: targets, controlSubject: controlSubject,
	}, nil
}

func (s *RemoteService) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /v1/publish", s.handlePublish)
	mux.HandleFunc("POST /v1/observe", s.handleObserve)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s == nil || !s.authorized(r) {
			writePublisherError(w, http.StatusUnauthorized, "verified control-plane identity required")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *RemoteService) authorized(r *http.Request) bool {
	if r == nil || r.TLS == nil || !r.TLS.HandshakeComplete || r.TLS.Version < tls.VersionTLS13 ||
		len(r.TLS.PeerCertificates) == 0 || len(r.TLS.VerifiedChains) == 0 {
		return false
	}
	leaf := r.TLS.PeerCertificates[0]
	verified := false
	for _, chain := range r.TLS.VerifiedChains {
		if len(chain) > 0 && chain[0] != nil && leaf != nil && bytes.Equal(chain[0].Raw, leaf.Raw) {
			verified = true
			break
		}
	}
	return verified && leaf != nil && len(leaf.URIs) == 1 && leaf.URIs[0] != nil &&
		leaf.URIs[0].String() == s.controlSubject
}

func (s *RemoteService) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writePublisherJSON(w, http.StatusOK, map[string]string{
		"status": "ok", "service": "engineering-github-publisher",
	})
}

func (s *RemoteService) handlePublish(w http.ResponseWriter, r *http.Request) {
	var req publishRequest
	if !decodePublisherRequest(w, r, &req) {
		return
	}
	if err := s.authorizePlan(req.Plan); err != nil {
		writePublisherError(w, http.StatusForbidden, err.Error())
		return
	}
	bundle, cleanup, err := s.verifyBundle(req.Plan)
	if err != nil {
		writePublisherError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	// Publish consumes only the privately staged, digest-verified snapshot.
	// The original Worker artifact may change after verification; never pass
	// its mutable path to Git or GitHub transport.
	defer cleanup()
	receipt, err := s.remote.Publish(r.Context(), req.Plan, bundle)
	if err != nil {
		writePublisherError(w, http.StatusBadGateway, "GitHub publication failed")
		return
	}
	if err := receipt.Validate(req.Plan); err != nil {
		writePublisherError(w, http.StatusBadGateway, "GitHub publication receipt invalid")
		return
	}
	writePublisherJSON(w, http.StatusOK, receipt)
}

func (s *RemoteService) handleObserve(w http.ResponseWriter, r *http.Request) {
	var req observeRequest
	if !decodePublisherRequest(w, r, &req) {
		return
	}
	if err := s.authorizePlan(req.Plan); err != nil {
		writePublisherError(w, http.StatusForbidden, err.Error())
		return
	}
	result, err := s.remote.Observe(r.Context(), req.Plan)
	if err != nil {
		writePublisherError(w, http.StatusBadGateway, "GitHub observation failed")
		return
	}
	switch result.Outcome {
	case ObservedAbsent, ObservedConfirmed, ObservedPartial, ObservedConflict:
	default:
		writePublisherError(w, http.StatusBadGateway, "GitHub observation invalid")
		return
	}
	if result.Outcome == ObservedConfirmed && result.Receipt.Validate(req.Plan) != nil {
		writePublisherError(w, http.StatusBadGateway, "GitHub observation receipt invalid")
		return
	}
	writePublisherJSON(w, http.StatusOK, result)
}

func (s *RemoteService) authorizePlan(plan Plan) error {
	if plan.Validate() != nil {
		return fmt.Errorf("valid publication plan required")
	}
	target, ok := s.targets[plan.Repository]
	if !ok || target.BaseRef != plan.BaseRef || !strings.HasPrefix(plan.Branch, target.BranchPrefix) {
		return fmt.Errorf("publication plan is outside publisher target policy")
	}
	return nil
}

func (s *RemoteService) verifyBundle(plan Plan) (string, func(), error) {
	if s == nil || plan.Validate() != nil {
		return "", nil, fmt.Errorf("valid publication plan required")
	}
	current, err := os.Lstat(s.artifactRoot)
	if err != nil || !current.IsDir() || !os.SameFile(current, s.artifactIdentity) {
		return "", nil, fmt.Errorf("publisher artifact root changed")
	}
	root, err := os.OpenRoot(s.artifactRoot)
	if err != nil {
		return "", nil, err
	}
	defer root.Close()
	name := plan.ExecutionID + ".bundle"
	before, err := root.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0o022 != 0 || before.Size() != plan.BundleSize {
		return "", nil, fmt.Errorf("retained result bundle is not the expected regular file")
	}
	source, err := root.Open(name)
	if err != nil {
		return "", nil, err
	}
	opened, err := source.Stat()
	if err != nil || !os.SameFile(before, opened) {
		_ = source.Close()
		return "", nil, fmt.Errorf("retained result bundle changed before read")
	}

	// Stage a private copy under the publisher's UID. Verifying a hash and
	// then handing Git the Worker-owned original path would allow a rename/
	// rewrite between verification and consumption (a TOCTOU gap).
	stage, err := os.MkdirTemp("", "engineering-publisher-verified-")
	if err != nil {
		_ = source.Close()
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(stage) }
	path := filepath.Join(stage, "verified.bundle")
	copyFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = source.Close()
		cleanup()
		return "", nil, err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(copyFile, hash), io.LimitReader(source, plan.BundleSize+1))
	closeSourceErr := source.Close()
	syncErr := copyFile.Sync()
	closeCopyErr := copyFile.Close()
	after, afterErr := root.Lstat(name)
	if copyErr != nil || closeSourceErr != nil || syncErr != nil || closeCopyErr != nil ||
		afterErr != nil || !os.SameFile(before, after) || after.Size() != before.Size() ||
		n != plan.BundleSize {
		cleanup()
		return "", nil, fmt.Errorf("retained result bundle changed or private snapshot failed")
	}
	if "sha256:"+hex.EncodeToString(hash.Sum(nil)) != plan.BundleDigest {
		cleanup()
		return "", nil, fmt.Errorf("retained result bundle digest mismatch")
	}
	return path, cleanup, nil
}

func targetMap(version int, targets []TargetPolicy) (map[string]TargetPolicy, error) {
	if version != 1 || len(targets) == 0 || len(targets) > 64 {
		return nil, fmt.Errorf("publisher service requires version 1 and 1..64 targets")
	}
	result := make(map[string]TargetPolicy, len(targets))
	for _, target := range targets {
		if !validRepository(target.Repository) || !validRef(target.BaseRef) || !validBranchPrefix(target.BranchPrefix) {
			return nil, fmt.Errorf("invalid GitHub publication target policy")
		}
		if _, exists := result[target.Repository]; exists {
			return nil, fmt.Errorf("duplicate GitHub publication repository policy")
		}
		result[target.Repository] = target
	}
	return result, nil
}

func validPublisherSubject(value string) bool {
	return strings.HasPrefix(value, "urn:engineering-platform:") &&
		len(value) <= 256 && strings.TrimSpace(value) == value &&
		!strings.ContainsAny(value, "\r\n\x00")
}

func decodePublisherRequest(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.Header.Get("Content-Encoding") != "" || r.Header.Get("Content-Type") != "application/json" {
		writePublisherError(w, http.StatusUnsupportedMediaType, "unencoded application/json required")
		return false
	}
	data, err := strictjson.ReadObject(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil || strictjson.Decode(data, dst) != nil {
		writePublisherError(w, http.StatusBadRequest, "invalid publisher request")
		return false
	}
	return true
}

func writePublisherError(w http.ResponseWriter, status int, message string) {
	writePublisherJSON(w, status, map[string]string{"error": message})
}

func writePublisherJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
