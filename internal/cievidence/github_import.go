package cievidence

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

const (
	GitHubIssuer            = "github-actions"
	GitHubProcedure         = "github.actions.ci.v1"
	GitHubArtifactMediaType = "application/vnd.github.actions.artifact+zip"
	maxGitHubJSON           = 4 << 20
	maxEvidenceZIP          = 2 << 20
)

type githubVerifier struct {
	base   *url.URL
	client *http.Client
	token  string
}

type githubRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Event      string `json:"event"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HeadSHA    string `json:"head_sha"`
	RunAttempt int64  `json:"run_attempt"`
	Pulls      []struct {
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			SHA string `json:"sha"`
		} `json:"base"`
	} `json:"pull_requests"`
}

type githubJobs struct {
	Jobs []struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	} `json:"jobs"`
}

type githubArtifact struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Size      int64  `json:"size_in_bytes"`
	Digest    string `json:"digest"`
	Expired   bool   `json:"expired"`
	Workflow  struct {
		ID      int64  `json:"id"`
		HeadSHA string `json:"head_sha"`
	} `json:"workflow_run"`
}

type githubArtifacts struct {
	Artifacts []githubArtifact `json:"artifacts"`
}

func NewGitHubVerifier(token string) (*githubVerifier, error) {
	if strings.TrimSpace(token) == "" || len(token) > 4096 || strings.ContainsAny(token, "\r\n") {
		return nil, fmt.Errorf("bounded GitHub read token required")
	}
	base, _ := url.Parse("https://api.github.com")
	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  10 * time.Second,
		IdleConnTimeout:        30 * time.Second,
		MaxIdleConns:           4,
		MaxConnsPerHost:        4,
		MaxResponseHeaderBytes: 32 << 10,
		DisableCompression:     true,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 3 || req.URL.Scheme != "https" || req.URL.Hostname() == "" {
				return fmt.Errorf("unsafe GitHub redirect")
			}
			req.Header.Del("Authorization")
			req.Header.Set("Accept-Encoding", "identity")
			return nil
		},
	}
	return &githubVerifier{base: base, client: client, token: token}, nil
}

func (v *githubVerifier) Close() {
	if v != nil && v.client != nil {
		v.client.CloseIdleConnections()
	}
}

func GitHubArtifactID(runID, artifactID int64) string {
	return "github-actions/run/" + strconv.FormatInt(runID, 10) + "/artifact/" + strconv.FormatInt(artifactID, 10)
}

func (v *githubVerifier) VerifyPullRequest(ctx context.Context, repository string, runID int64, delivery core.DeliveryReceipt, requirementID string) (core.EvidenceRef, Envelope, error) {
	var zero core.EvidenceRef
	var envelope Envelope
	if v == nil || v.client == nil || v.base == nil || !repoName.MatchString(repository) || runID <= 0 || delivery.ID == "" || delivery.SubjectDigest == "" || delivery.ResultCommit == "" || delivery.BaseCommit == "" || strings.TrimSpace(requirementID) == "" {
		return zero, envelope, fmt.Errorf("complete repository/run/delivery/requirement identity required")
	}
	var run githubRun
	if err := v.getJSON(ctx, "/repos/"+repository+"/actions/runs/"+strconv.FormatInt(runID, 10), &run); err != nil {
		return zero, envelope, err
	}
	if run.ID != runID || run.Name != "CI" || run.Event != "pull_request" || run.Status != "completed" || run.Conclusion != "success" || run.RunAttempt <= 0 || !sha40.MatchString(run.HeadSHA) {
		return zero, envelope, fmt.Errorf("workflow run is not an accepted completed PR CI run")
	}
	var jobs githubJobs
	if err := v.getJSON(ctx, "/repos/"+repository+"/actions/runs/"+strconv.FormatInt(runID, 10)+"/jobs?per_page=100", &jobs); err != nil {
		return zero, envelope, err
	}
	var artifacts githubArtifacts
	if err := v.getJSON(ctx, "/repos/"+repository+"/actions/runs/"+strconv.FormatInt(runID, 10)+"/artifacts?per_page=100", &artifacts); err != nil {
		return zero, envelope, err
	}
	trusted, err := uniqueTrustedArtifact(artifacts.Artifacts, runID, run.HeadSHA)
	if err != nil {
		return zero, envelope, err
	}
	zipBytes, err := v.download(ctx, "/repos/"+repository+"/actions/artifacts/"+strconv.FormatInt(trusted.ID, 10)+"/zip")
	if err != nil {
		return zero, envelope, err
	}
	if canonical.BytesDigest(zipBytes) != trusted.Digest {
		return zero, envelope, fmt.Errorf("trusted evidence artifact digest mismatch")
	}
	envelopeBytes, err := readEnvelopeZIP(zipBytes)
	if err != nil {
		return zero, envelope, err
	}
	if err := strictjson.Decode(envelopeBytes, &envelope); err != nil {
		return zero, Envelope{}, fmt.Errorf("strict CI evidence envelope: %w", err)
	}
	if err := envelope.Verify(); err != nil {
		return zero, Envelope{}, err
	}
	receipt := envelope.Receipt
	if receipt.Repository != repository || receipt.Workflow != "CI" || receipt.Event != "pull_request" || receipt.RunID != runID || receipt.RunAttempt != run.RunAttempt || receipt.TestedSHA != run.HeadSHA {
		return zero, envelope, fmt.Errorf("CI envelope does not identify the live workflow run")
	}
	if delivery.ResultCommit != receipt.SourceSHA || delivery.BaseCommit != receipt.BaseSHA {
		return zero, envelope, fmt.Errorf("delivery commits do not match CI PR source/base")
	}
	if !livePullMatches(run, receipt.SourceSHA, receipt.BaseSHA) {
		return zero, envelope, fmt.Errorf("live workflow PR identity does not match envelope")
	}
	if err := liveJobsMatch(jobs, receipt.Jobs); err != nil {
		return zero, envelope, err
	}
	liveArtifactByID := make(map[int64]githubArtifact, len(artifacts.Artifacts))
	for _, a := range artifacts.Artifacts {
		if _, exists := liveArtifactByID[a.ID]; exists {
			return zero, envelope, fmt.Errorf("duplicate live artifact ID")
		}
		liveArtifactByID[a.ID] = a
	}
	requiredArtifactIDs := make([]string, 0, len(receipt.Artifacts)+1)
	for _, fact := range receipt.Artifacts {
		live, ok := liveArtifactByID[fact.ID]
		if !ok || !sameArtifact(live, fact, runID, run.HeadSHA) {
			return zero, envelope, fmt.Errorf("live GitHub artifact does not match CI envelope")
		}
		id := GitHubArtifactID(runID, fact.ID)
		if !deliveryHasArtifact(delivery, id, fact.Digest) {
			return zero, envelope, fmt.Errorf("delivery is missing an exact upstream CI artifact")
		}
		requiredArtifactIDs = append(requiredArtifactIDs, id)
	}
	trustedID := GitHubArtifactID(runID, trusted.ID)
	if !deliveryHasArtifact(delivery, trustedID, trusted.Digest) {
		return zero, envelope, fmt.Errorf("delivery is missing the trusted CI evidence artifact")
	}
	requiredArtifactIDs = append(requiredArtifactIDs, trustedID)
	sort.Strings(requiredArtifactIDs)

	evidenceDigest, err := canonical.Digest(struct {
		DeliverySubject string `json:"delivery_subject"`
		RequirementID   string `json:"requirement_id"`
		ReceiptDigest   string `json:"receipt_digest"`
		TrustedArtifact string `json:"trusted_artifact"`
	}{
		DeliverySubject: delivery.SubjectDigest,
		RequirementID:   requirementID,
		ReceiptDigest:   envelope.ReceiptDigest,
		TrustedArtifact: trusted.Digest,
	})
	if err != nil {
		return zero, envelope, err
	}
	evidence := core.EvidenceRef{
		ID:                "github-ci-" + strings.TrimPrefix(evidenceDigest, "sha256:"),
		DeliveryReceiptID: delivery.ID,
		RequirementID:     requirementID,
		SubjectDigest:     delivery.SubjectDigest,
		Issuer:            GitHubIssuer,
		Procedure:         GitHubProcedure,
		Result:            "PASS",
		ArtifactRefs:      requiredArtifactIDs,
		Applicable:        true,
	}
	return evidence, envelope, nil
}

func (v *githubVerifier) getJSON(ctx context.Context, p string, out any) error {
	body, err := v.request(ctx, p, maxGitHubJSON, "application/vnd.github+json")
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode GitHub API response")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing GitHub API JSON")
	}
	return nil
}

func (v *githubVerifier) download(ctx context.Context, p string) ([]byte, error) {
	return v.request(ctx, p, maxEvidenceZIP, "application/vnd.github+json")
}

func (v *githubVerifier) request(ctx context.Context, p string, limit int64, accept string) ([]byte, error) {
	if !strings.HasPrefix(p, "/repos/") || path.Clean(p) != strings.SplitN(p, "?", 2)[0] {
		// Queries are allowed only on the two fixed list call sites above.
		if !strings.HasSuffix(p, "?per_page=100") || path.Clean(strings.TrimSuffix(p, "?per_page=100")) != strings.TrimSuffix(p, "?per_page=100") {
			return nil, fmt.Errorf("invalid GitHub API path")
		}
	}
	u := *v.base
	u.Path = strings.SplitN(p, "?", 2)[0]
	if parts := strings.SplitN(p, "?", 2); len(parts) == 2 {
		u.RawQuery = parts[1]
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+v.token)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub transport: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Encoding") != "" {
		return nil, fmt.Errorf("GitHub API returned unusable response")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, fmt.Errorf("GitHub response exceeds bound")
	}
	return body, nil
}

func uniqueTrustedArtifact(artifacts []githubArtifact, runID int64, testedSHA string) (githubArtifact, error) {
	want := "trusted-ci-evidence-" + testedSHA
	var found githubArtifact
	count := 0
	for _, artifact := range artifacts {
		if artifact.Name == want {
			if !validLiveArtifact(artifact, runID, testedSHA) {
				return githubArtifact{}, fmt.Errorf("invalid trusted CI evidence artifact")
			}
			found = artifact
			count++
		}
	}
	if count != 1 {
		return githubArtifact{}, fmt.Errorf("exactly one trusted CI evidence artifact required")
	}
	return found, nil
}

func validLiveArtifact(a githubArtifact, runID int64, headSHA string) bool {
	return a.ID > 0 && a.Size > 0 && a.Size <= maxEvidenceZIP && canonical.ValidDigest(a.Digest) && !a.Expired && a.Workflow.ID == runID && a.Workflow.HeadSHA == headSHA
}

func sameArtifact(live githubArtifact, fact Artifact, runID int64, headSHA string) bool {
	return validLiveArtifact(live, runID, headSHA) && live.Name == fact.Name && live.ID == fact.ID && live.Digest == fact.Digest && live.Size == fact.Size
}

func livePullMatches(run githubRun, sourceSHA, baseSHA string) bool {
	count := 0
	for _, pr := range run.Pulls {
		if pr.Head.SHA == sourceSHA && pr.Base.SHA == baseSHA {
			count++
		}
	}
	return count == 1
}

func liveJobsMatch(live githubJobs, retained []Job) error {
	byID := make(map[int64]struct {
		name, status, conclusion string
	}, len(live.Jobs))
	for _, job := range live.Jobs {
		if _, exists := byID[job.ID]; exists {
			return fmt.Errorf("duplicate live job ID")
		}
		byID[job.ID] = struct {
			name, status, conclusion string
		}{job.Name, job.Status, job.Conclusion}
	}
	for _, fact := range retained {
		job, ok := byID[fact.ID]
		if !ok || job.name != fact.Name || job.status != "completed" || job.conclusion != fact.Conclusion || fact.Conclusion != "success" {
			return fmt.Errorf("live GitHub job does not match CI envelope")
		}
	}
	return nil
}

func deliveryHasArtifact(delivery core.DeliveryReceipt, id, digest string) bool {
	count := 0
	for _, artifact := range delivery.Artifacts {
		if artifact.ID == id && artifact.Digest == digest && artifact.MediaType == GitHubArtifactMediaType {
			count++
		}
	}
	return count == 1
}

func readEnvelopeZIP(data []byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(reader.File) != 1 {
		return nil, fmt.Errorf("trusted CI evidence artifact must contain exactly one file")
	}
	file := reader.File[0]
	if file.Name != "ci-evidence-envelope.json" || file.FileInfo().IsDir() || file.UncompressedSize64 == 0 || file.UncompressedSize64 > strictjson.MaxBytes {
		return nil, fmt.Errorf("unexpected trusted CI evidence artifact content")
	}
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	body, err := io.ReadAll(io.LimitReader(rc, strictjson.MaxBytes+1))
	if err != nil || len(body) == 0 || len(body) > strictjson.MaxBytes {
		return nil, fmt.Errorf("trusted CI evidence envelope exceeds bound")
	}
	return body, nil
}

func zipDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
