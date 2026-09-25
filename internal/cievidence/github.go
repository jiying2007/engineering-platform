package cievidence

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

const maxGitHubJSON = 4 << 20

type GitHubClient struct {
	base  *url.URL
	http  *http.Client
	token string
}

func NewGitHubClient(token string) (*GitHubClient, error) {
	token = strings.TrimSpace(token)
	if strings.ContainsAny(token, "\r\n") || len(token) > 4096 {
		return nil, fmt.Errorf("invalid GitHub token")
	}
	base, _ := url.Parse("https://api.github.com")
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS13, ServerName: "api.github.com"},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConns:          2,
		MaxConnsPerHost:       2,
		MaxResponseHeaderBytes: 32 << 10,
		DisableCompression:    true,
	}
	return &GitHubClient{
		base: base,
		http: &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		token: token,
	}, nil
}

func (c *GitHubClient) Close() {
	if c != nil && c.http != nil {
		c.http.CloseIdleConnections()
	}
}

func (c *GitHubClient) FetchLiveFacts(ctx context.Context, repository string, runID int64) (LiveFacts, error) {
	var facts LiveFacts
	if c == nil || c.http == nil || repository != TrustedRepository || runID <= 0 {
		return facts, fmt.Errorf("exact trusted repository and workflow run required")
	}
	var run struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		Event      string `json:"event"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		RunAttempt int64  `json:"run_attempt"`
		HeadSHA    string `json:"head_sha"`
		HeadBranch string `json:"head_branch"`
		Path       string `json:"path"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := c.getJSON(ctx, "/repos/"+TrustedRepository+"/actions/runs/"+strconv.FormatInt(runID, 10), &run); err != nil {
		return facts, err
	}
	facts.Run = RunFact{
		ID: run.ID, Attempt: run.RunAttempt, Repository: run.Repository.FullName,
		Workflow: run.Name, WorkflowPath: run.Path, Event: run.Event, HeadBranch: run.HeadBranch, HeadSHA: run.HeadSHA,
		Status: run.Status, Conclusion: run.Conclusion,
	}
	var jobs struct {
		TotalCount int `json:"total_count"`
		Jobs []struct {
			ID         int64  `json:"id"`
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			HeadSHA    string `json:"head_sha"`
		} `json:"jobs"`
	}
	if err := c.getJSON(ctx, "/repos/"+TrustedRepository+"/actions/runs/"+strconv.FormatInt(runID, 10)+"/jobs?per_page=100&filter=latest", &jobs); err != nil {
		return facts, err
	}
	if jobs.TotalCount < 3 || jobs.TotalCount > 100 || len(jobs.Jobs) != jobs.TotalCount {
		return facts, fmt.Errorf("GitHub job pagination/count is not authoritative")
	}
	for _, job := range jobs.Jobs {
		if job.ID <= 0 || strings.TrimSpace(job.Name) == "" || job.Status != "completed" || job.HeadSHA != run.HeadSHA {
			return facts, fmt.Errorf("GitHub job is incomplete or malformed")
		}
		facts.Jobs = append(facts.Jobs, Job{Name: job.Name, ID: job.ID, Conclusion: job.Conclusion})
	}
	var artifacts struct {
		TotalCount int `json:"total_count"`
		Artifacts []struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			Size        int64  `json:"size_in_bytes"`
			Digest      string `json:"digest"`
			Expired     bool   `json:"expired"`
			WorkflowRun struct {
				ID      int64  `json:"id"`
				HeadSHA string `json:"head_sha"`
			} `json:"workflow_run"`
		} `json:"artifacts"`
	}
	if err := c.getJSON(ctx, "/repos/"+TrustedRepository+"/actions/runs/"+strconv.FormatInt(runID, 10)+"/artifacts?per_page=100", &artifacts); err != nil {
		return facts, err
	}
	if artifacts.TotalCount < 3 || artifacts.TotalCount > 100 || len(artifacts.Artifacts) != artifacts.TotalCount {
		return facts, fmt.Errorf("GitHub artifact pagination/count is not authoritative")
	}
	for _, artifact := range artifacts.Artifacts {
		if artifact.ID <= 0 || artifact.Size <= 0 || strings.TrimSpace(artifact.Name) == "" || !canonical.ValidDigest(artifact.Digest) {
			return facts, fmt.Errorf("GitHub artifact is malformed or lacks sha256 digest")
		}
		facts.Artifacts = append(facts.Artifacts, ArtifactFact{
			ID: artifact.ID, Name: artifact.Name, Digest: artifact.Digest, Size: artifact.Size,
			Expired: artifact.Expired, RunID: artifact.WorkflowRun.ID, HeadSHA: artifact.WorkflowRun.HeadSHA,
		})
	}
	return facts, nil
}

func (c *GitHubClient) getJSON(ctx context.Context, requestPath string, dst any) error {
	if c == nil || c.http == nil || !strings.HasPrefix(requestPath, "/repos/"+TrustedRepository+"/actions/") || strings.ContainsAny(requestPath, "\r\n#") {
		return fmt.Errorf("invalid GitHub API path")
	}
	u := *c.base
	u.Path = strings.SplitN(requestPath, "?", 2)[0]
	if parts := strings.SplitN(requestPath, "?", 2); len(parts) == 2 {
		values, err := url.ParseQuery(parts[1])
		if err != nil {
			return err
		}
		u.RawQuery = values.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "engineering-platform-ci-evidence-importer/1")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("GitHub API transport: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned HTTP %d", response.StatusCode)
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || (contentType != "application/json" && contentType != "application/vnd.github+json") || response.Header.Get("Content-Encoding") != "" {
		return fmt.Errorf("GitHub API must return unencoded JSON")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxGitHubJSON+1))
	if err != nil || len(body) > maxGitHubJSON {
		return fmt.Errorf("GitHub API response exceeds bound")
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode GitHub API response: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("trailing GitHub API JSON")
	}
	return nil
}
