package cievidence

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
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
	"github.com/jiying2007/engineering-platform/internal/verification"
)

const (
	GitHubIssuer            = verification.GitHubActionsIssuer
	GitHubProcedure         = verification.GitHubActionsProcedure
	GitHubArtifactMediaType = "application/vnd.github.actions.artifact+zip"
	maxGitHubJSON           = 4 << 20
	maxEvidenceZIP          = 2 << 20
	maxRetainedArtifact     = 1 << 30
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
}

type githubJobs struct {
	Jobs []struct {
		ID         int64  `json:"id"`
		RunID      int64  `json:"run_id"`
		HeadSHA    string `json:"head_sha"`
		Name       string `json:"name"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	} `json:"jobs"`
}

type githubCommit struct {
	SHA      string `json:"sha"`
	Message  string `json:"message"`
	Parents  []struct {
		SHA string `json:"sha"`
	} `json:"parents"`
	Verification struct {
		Verified bool   `json:"verified"`
		Reason   string `json:"reason"`
	} `json:"verification"`
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

type PullRequestFacts struct {
	repository    string
	runID         int64
	envelope      Envelope
	artifactRefs  []core.ArtifactRef
	trustedDigest string
}

func (f PullRequestFacts) SourceSHA() string     { return f.envelope.Receipt.SourceSHA }
func (f PullRequestFacts) BaseSHA() string       { return f.envelope.Receipt.BaseSHA }
func (f PullRequestFacts) TestedSHA() string     { return f.envelope.Receipt.TestedSHA }
func (f PullRequestFacts) ReceiptDigest() string { return f.envelope.ReceiptDigest }

func (f PullRequestFacts) Artifacts() []core.ArtifactRef {
	out := make([]core.ArtifactRef, len(f.artifactRefs))
	copy(out, f.artifactRefs)
	return out
}

func (f PullRequestFacts) ValidateTask(task core.TaskContract) error {
	if f.repository == "" || task.Repository != f.repository || task.BaseCommit != f.BaseSHA() {
		return fmt.Errorf("task repository/base do not match verified GitHub PR")
	}
	return nil
}

func (v *githubVerifier) ResolvePullRequest(ctx context.Context, repository string, runID int64) (PullRequestFacts, error) {
	var facts PullRequestFacts
	if v == nil || v.client == nil || v.base == nil || !repoName.MatchString(repository) || runID <= 0 {
		return facts, fmt.Errorf("complete repository/run identity required")
	}
	var run githubRun
	if err := v.getJSON(ctx, "/repos/"+repository+"/actions/runs/"+strconv.FormatInt(runID, 10), &run); err != nil {
		return facts, err
	}
	if run.ID != runID || run.Name != "CI" || run.Event != "pull_request" || run.Status != "completed" || run.Conclusion != "success" || run.RunAttempt <= 0 || !sha40.MatchString(run.HeadSHA) {
		return facts, fmt.Errorf("workflow run is not an accepted completed PR CI run")
	}
	var jobs githubJobs
	if err := v.getJSON(ctx, "/repos/"+repository+"/actions/runs/"+strconv.FormatInt(runID, 10)+"/jobs?per_page=100", &jobs); err != nil {
		return facts, err
	}
	var artifacts githubArtifacts
	if err := v.getJSON(ctx, "/repos/"+repository+"/actions/runs/"+strconv.FormatInt(runID, 10)+"/artifacts?per_page=100", &artifacts); err != nil {
		return facts, err
	}
	trusted, err := uniqueTrustedArtifact(artifacts.Artifacts, runID, run.HeadSHA)
	if err != nil {
		return facts, err
	}
	zipBytes, err := v.download(ctx, "/repos/"+repository+"/actions/artifacts/"+strconv.FormatInt(trusted.ID, 10)+"/zip")
	if err != nil {
		return facts, err
	}
	if canonical.BytesDigest(zipBytes) != trusted.Digest {
		return facts, fmt.Errorf("trusted evidence artifact digest mismatch")
	}
	envelopeBytes, err := readEnvelopeZIP(zipBytes)
	if err != nil {
		return facts, err
	}
	var envelope Envelope
	if err := strictjson.Decode(envelopeBytes, &envelope); err != nil {
		return facts, fmt.Errorf("strict CI evidence envelope: %w", err)
	}
	if err := envelope.Verify(); err != nil {
		return facts, err
	}
	receipt := envelope.Receipt
	if receipt.Repository != repository || receipt.Workflow != "CI" || receipt.Event != "pull_request" || receipt.RunID != runID || receipt.RunAttempt != run.RunAttempt || receipt.SourceSHA != run.HeadSHA {
		return facts, fmt.Errorf("CI envelope does not identify the live workflow source")
	}
	if trusted.Name != "trusted-ci-evidence-"+receipt.TestedSHA {
		return facts, fmt.Errorf("trusted CI artifact does not identify the tested commit")
	}
	var tested githubCommit
	if err := v.getJSON(ctx, "/repos/"+repository+"/git/commits/"+receipt.TestedSHA, &tested); err != nil {
		return facts, err
	}
	if !validTestedMerge(tested, receipt.BaseSHA, receipt.SourceSHA, receipt.TestedSHA) {
		return facts, fmt.Errorf("tested SHA is not the exact GitHub-verified PR merge commit")
	}
	if err := liveJobsMatch(jobs, receipt.Jobs, runID, receipt.SourceSHA); err != nil {
		return facts, err
	}
	liveArtifactByID := make(map[int64]githubArtifact, len(artifacts.Artifacts))
	for _, a := range artifacts.Artifacts {
		if _, exists := liveArtifactByID[a.ID]; exists {
			return facts, fmt.Errorf("duplicate live artifact ID")
		}
		liveArtifactByID[a.ID] = a
	}
	refs := make([]core.ArtifactRef, 0, len(receipt.Artifacts)+1)
	for _, retained := range receipt.Artifacts {
		live, ok := liveArtifactByID[retained.ID]
		if !ok || !sameArtifact(live, retained, runID, run.HeadSHA) {
			return facts, fmt.Errorf("live GitHub artifact does not match CI envelope")
		}
		refs = append(refs, githubArtifactRef(repository, runID, live))
	}
	refs = append(refs, githubArtifactRef(repository, runID, trusted))
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	return PullRequestFacts{
		repository:    repository,
		runID:         runID,
		envelope:      envelope,
		artifactRefs:  refs,
		trustedDigest: trusted.Digest,
	}, nil
}

func (f PullRequestFacts) BindDelivery(delivery core.DeliveryReceipt, requirementID string) (core.EvidenceRef, error) {
	var zero core.EvidenceRef
	if f.repository == "" || delivery.ID == "" || delivery.SubjectDigest == "" || delivery.ResultCommit == "" || delivery.BaseCommit == "" || strings.TrimSpace(requirementID) == "" {
		return zero, fmt.Errorf("complete delivery/requirement identity required")
	}
	if delivery.ResultCommit != f.SourceSHA() || delivery.BaseCommit != f.BaseSHA() {
		return zero, fmt.Errorf("delivery commits do not match CI PR source/base")
	}
	artifactIDs := make([]string, 0, len(f.artifactRefs))
	for _, required := range f.artifactRefs {
		if !deliveryHasArtifact(delivery, required.ID, required.Digest) {
			return zero, fmt.Errorf("delivery is missing an exact GitHub CI artifact")
		}
		artifactIDs = append(artifactIDs, required.ID)
	}
	sort.Strings(artifactIDs)
	evidenceDigest, err := canonical.Digest(struct {
		DeliverySubject string `json:"delivery_subject"`
		RequirementID   string `json:"requirement_id"`
		ReceiptDigest   string `json:"receipt_digest"`
		TrustedArtifact string `json:"trusted_artifact"`
	}{
		DeliverySubject: delivery.SubjectDigest,
		RequirementID:   requirementID,
		ReceiptDigest:   f.ReceiptDigest(),
		TrustedArtifact: f.trustedDigest,
	})
	if err != nil {
		return zero, err
	}
	return core.EvidenceRef{
		ID:                "github-ci-" + strings.TrimPrefix(evidenceDigest, "sha256:"),
		DeliveryReceiptID: delivery.ID,
		RequirementID:     requirementID,
		SubjectDigest:     delivery.SubjectDigest,
		Issuer:            GitHubIssuer,
		Procedure:         GitHubProcedure,
		Result:            "PASS",
		ArtifactRefs:      artifactIDs,
		Applicable:        true,
	}, nil
}

func (v *githubVerifier) VerifyPullRequest(ctx context.Context, repository string, runID int64, delivery core.DeliveryReceipt, requirementID string) (core.EvidenceRef, Envelope, error) {
	facts, err := v.ResolvePullRequest(ctx, repository, runID)
	if err != nil {
		return core.EvidenceRef{}, Envelope{}, err
	}
	evidence, err := facts.BindDelivery(delivery, requirementID)
	if err != nil {
		return core.EvidenceRef{}, facts.envelope, err
	}
	return evidence, facts.envelope, nil
}

func githubArtifactRef(repository string, runID int64, artifact githubArtifact) core.ArtifactRef {
	return core.ArtifactRef{
		ID:        GitHubArtifactID(runID, artifact.ID),
		Digest:    artifact.Digest,
		MediaType: GitHubArtifactMediaType,
		Locator:   "https://github.com/" + repository + "/actions/runs/" + strconv.FormatInt(runID, 10) + "/artifacts/" + strconv.FormatInt(artifact.ID, 10),
	}
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

func uniqueTrustedArtifact(artifacts []githubArtifact, runID int64, sourceSHA string) (githubArtifact, error) {
	var found githubArtifact
	count := 0
	for _, artifact := range artifacts {
		if strings.HasPrefix(artifact.Name, "trusted-ci-evidence-") {
			if !validTrustedArtifact(artifact, runID, sourceSHA) {
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

func validArtifactIdentity(a githubArtifact, runID int64, headSHA string, maxSize int64) bool {
	return a.ID > 0 && a.Size > 0 && a.Size <= maxSize && canonical.ValidDigest(a.Digest) && !a.Expired && a.Workflow.ID == runID && a.Workflow.HeadSHA == headSHA
}

func validTrustedArtifact(a githubArtifact, runID int64, headSHA string) bool {
	return validArtifactIdentity(a, runID, headSHA, maxEvidenceZIP)
}

func sameArtifact(live githubArtifact, fact Artifact, runID int64, headSHA string) bool {
	return validArtifactIdentity(live, runID, headSHA, maxRetainedArtifact) && live.Name == fact.Name && live.ID == fact.ID && live.Digest == fact.Digest && live.Size == fact.Size
}

func validTestedMerge(commit githubCommit, baseSHA, sourceSHA, testedSHA string) bool {
	return commit.SHA == testedSHA &&
		commit.Verification.Verified &&
		commit.Verification.Reason == "valid" &&
		len(commit.Parents) == 2 &&
		commit.Parents[0].SHA == baseSHA &&
		commit.Parents[1].SHA == sourceSHA
}

func liveJobsMatch(live githubJobs, retained []Job, runID int64, sourceSHA string) error {
	type liveJob struct {
		runID                    int64
		headSHA                  string
		name, status, conclusion string
	}
	byID := make(map[int64]liveJob, len(live.Jobs))
	for _, job := range live.Jobs {
		if _, exists := byID[job.ID]; exists {
			return fmt.Errorf("duplicate live job ID")
		}
		byID[job.ID] = liveJob{runID: job.RunID, headSHA: job.HeadSHA, name: job.Name, status: job.Status, conclusion: job.Conclusion}
	}
	for _, fact := range retained {
		job, ok := byID[fact.ID]
		if !ok || job.runID != runID || job.headSHA != sourceSHA || job.name != fact.Name || job.status != "completed" || job.conclusion != fact.Conclusion || fact.Conclusion != "success" {
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

