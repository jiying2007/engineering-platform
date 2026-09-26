package githubpublish

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
)

type githubRemote struct {
	git       string
	tokenFile string
	client    *http.Client
	apiBase   string
}

type refRecord struct {
	Object struct {
		SHA string `json:"sha"`
	} `json:"object"`
}

type pullRecord struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	State   string `json:"state"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Head    struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"base"`
}

func newGitHubRemote(git, tokenFile string) (*githubRemote, error) {
	canonicalGit, err := canonicalExecutable(git)
	if err != nil {
		return nil, err
	}
	canonicalToken, err := canonicalSecret(tokenFile)
	if err != nil {
		return nil, err
	}
	return &githubRemote{
		git: canonicalGit, tokenFile: canonicalToken,
		client: &http.Client{Timeout: 20 * time.Second},
		apiBase: "https://api.github.com",
	}, nil
}

func (g *githubRemote) Publish(ctx context.Context, plan Plan, bundlePath string) (PublicationReceipt, error) {
	var empty PublicationReceipt
	if g == nil || plan.Validate() != nil || bundlePath == "" {
		return empty, fmt.Errorf("valid GitHub publication plan and bundle are required")
	}
	token, err := g.loadToken()
	if err != nil {
		return empty, err
	}
	base, exists, err := g.getRef(ctx, token, plan.Repository, "heads/"+plan.BaseRef)
	if err != nil {
		return empty, err
	}
	if !exists || base != plan.BaseCommit {
		return empty, fmt.Errorf("target base ref does not match frozen base commit")
	}
	stage, cleanup, err := g.stageBundle(ctx, token, plan, bundlePath)
	if err != nil {
		return empty, err
	}
	defer cleanup()

	branchSHA, branchExists, err := g.getRef(ctx, token, plan.Repository, "heads/"+plan.Branch)
	if err != nil {
		return empty, err
	}
	if branchExists && branchSHA != plan.ResultCommit {
		return empty, fmt.Errorf("publication branch already points at a different commit")
	}
	if !branchExists {
		if err := g.push(ctx, token, stage, plan); err != nil {
			return empty, err
		}
	}
	branchSHA, branchExists, err = g.getRef(ctx, token, plan.Repository, "heads/"+plan.Branch)
	if err != nil || !branchExists || branchSHA != plan.ResultCommit {
		if err != nil {
			return empty, err
		}
		return empty, fmt.Errorf("published branch cannot be confirmed")
	}

	title, body := publicationText(plan)
	pr, found, err := g.findOpenPull(ctx, token, plan)
	if err != nil {
		return empty, err
	}
	outcome := "EXISTING"
	if found {
		if !pullMatches(pr, plan) {
			return empty, fmt.Errorf("existing pull request does not bind publication plan")
		}
		if pr.Title != title || pr.Body != body {
			pr, err = g.updatePull(ctx, token, plan, pr.Number, title, body)
			if err != nil {
				return empty, err
			}
			outcome = "UPDATED"
		}
	} else {
		pr, err = g.createPull(ctx, token, plan, title, body)
		if err != nil {
			return empty, err
		}
		outcome = "CREATED"
	}
	if !pullMatches(pr, plan) {
		return empty, fmt.Errorf("GitHub pull request response does not bind publication plan")
	}
	receipt := receiptFromPull(plan, pr, outcome)
	if err := receipt.Validate(plan); err != nil {
		return empty, err
	}
	return receipt, nil
}

func (g *githubRemote) Observe(ctx context.Context, plan Plan) (ObserveResult, error) {
	if g == nil || plan.Validate() != nil {
		return ObserveResult{}, fmt.Errorf("valid GitHub publication plan required")
	}
	token, err := g.loadToken()
	if err != nil {
		return ObserveResult{}, err
	}
	base, exists, err := g.getRef(ctx, token, plan.Repository, "heads/"+plan.BaseRef)
	if err != nil {
		return ObserveResult{}, err
	}
	if !exists || base != plan.BaseCommit {
		return ObserveResult{Outcome: ObservedConflict}, nil
	}
	branch, exists, err := g.getRef(ctx, token, plan.Repository, "heads/"+plan.Branch)
	if err != nil {
		return ObserveResult{}, err
	}
	if !exists {
		return ObserveResult{Outcome: ObservedAbsent}, nil
	}
	if branch != plan.ResultCommit {
		return ObserveResult{Outcome: ObservedConflict}, nil
	}
	pr, found, err := g.findOpenPull(ctx, token, plan)
	if err != nil {
		return ObserveResult{}, err
	}
	if !found {
		return ObserveResult{Outcome: ObservedPartial}, nil
	}
	if !pullMatches(pr, plan) {
		return ObserveResult{Outcome: ObservedConflict}, nil
	}
	receipt := receiptFromPull(plan, pr, "OBSERVED")
	if err := receipt.Validate(plan); err != nil {
		return ObserveResult{Outcome: ObservedConflict}, nil
	}
	return ObserveResult{Outcome: ObservedConfirmed, Receipt: receipt}, nil
}

func (g *githubRemote) loadToken() (string, error) {
	path, err := canonicalSecret(g.tokenFile)
	if err != nil {
		return "", err
	}
	data, err := access.ReadConfiguration(path, true)
	if err != nil {
		return "", err
	}
	token := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if token == "" || len(token) > 4096 || strings.TrimSpace(token) != token {
		return "", fmt.Errorf("publisher credential is invalid")
	}
	return token, nil
}

func (g *githubRemote) getRef(ctx context.Context, token, repository, ref string) (string, bool, error) {
	var record refRecord
	status, err := g.request(ctx, token, http.MethodGet,
		"/repos/"+repository+"/git/ref/"+escapeRef(ref), nil, nil, &record)
	if status == http.StatusNotFound {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !fullSHA.MatchString(strings.ToLower(record.Object.SHA)) {
		return "", false, fmt.Errorf("GitHub returned invalid ref SHA")
	}
	return strings.ToLower(record.Object.SHA), true, nil
}

func (g *githubRemote) findOpenPull(ctx context.Context, token string, plan Plan) (pullRecord, bool, error) {
	owner := strings.SplitN(plan.Repository, "/", 2)[0]
	query := url.Values{}
	query.Set("state", "open")
	query.Set("head", owner+":"+plan.Branch)
	query.Set("base", plan.BaseRef)
	query.Set("per_page", "10")
	var pulls []pullRecord
	_, err := g.request(ctx, token, http.MethodGet, "/repos/"+plan.Repository+"/pulls", query, nil, &pulls)
	if err != nil {
		return pullRecord{}, false, err
	}
	if len(pulls) == 0 {
		return pullRecord{}, false, nil
	}
	if len(pulls) != 1 {
		return pullRecord{}, false, fmt.Errorf("GitHub returned multiple open pull requests for one publication branch")
	}
	return pulls[0], true, nil
}

func (g *githubRemote) createPull(ctx context.Context, token string, plan Plan, title, body string) (pullRecord, error) {
	input := map[string]any{
		"title": title, "body": body, "head": plan.Branch, "base": plan.BaseRef,
		"maintainer_can_modify": false,
	}
	var result pullRecord
	_, err := g.request(ctx, token, http.MethodPost, "/repos/"+plan.Repository+"/pulls", nil, input, &result)
	return result, err
}

func (g *githubRemote) updatePull(ctx context.Context, token string, plan Plan, number int, title, body string) (pullRecord, error) {
	input := map[string]any{"title": title, "body": body}
	var result pullRecord
	_, err := g.request(ctx, token, http.MethodPatch,
		"/repos/"+plan.Repository+"/pulls/"+strconv.Itoa(number), nil, input, &result)
	return result, err
}

func pullMatches(pr pullRecord, plan Plan) bool {
	return pr.Number > 0 && pr.State == "open" && pr.HTMLURL != "" &&
		pr.Head.Ref == plan.Branch && strings.ToLower(pr.Head.SHA) == plan.ResultCommit &&
		pr.Base.Ref == plan.BaseRef && strings.ToLower(pr.Base.SHA) == plan.BaseCommit
}

func receiptFromPull(plan Plan, pr pullRecord, outcome string) PublicationReceipt {
	return PublicationReceipt{
		Version: 1, Repository: plan.Repository, BaseRef: plan.BaseRef, BaseCommit: plan.BaseCommit,
		Branch: plan.Branch, ResultCommit: plan.ResultCommit, PullRequestNumber: pr.Number,
		PullRequestURL: pr.HTMLURL, PullRequestState: pr.State, PublicationOutcome: outcome,
	}
}

func publicationText(plan Plan) (string, string) {
	title := "engineering-platform: retained result " + plan.ExecutionID[:12]
	body := "Published by the Engineering Platform Action Gateway.\n\n" +
		"- execution: `" + plan.ExecutionID + "`\n" +
		"- frozen base: `" + plan.BaseCommit + "`\n" +
		"- retained result: `" + plan.ResultCommit + "`\n" +
		"- Codex result digest: `" + plan.ResultDigest + "`\n" +
		"- Codex receipt digest: `" + plan.ReceiptDigest + "`\n" +
		"- Git bundle digest: `" + plan.BundleDigest + "`\n\n" +
		"This PR is publication transport only; CI, Evidence, Verification, Review and Closure remain independent gates.\n"
	return title, body
}

func (g *githubRemote) request(ctx context.Context, token, method, endpoint string, query url.Values, input, output any) (int, error) {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(data)
	}
	target := g.apiBase + endpoint
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "engineering-platform-github-publisher")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := g.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if readErr != nil || len(data) > 2<<20 {
		return response.StatusCode, fmt.Errorf("GitHub API response outside size limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, fmt.Errorf("GitHub API returned status %d", response.StatusCode)
	}
	if output != nil {
		if len(data) == 0 || json.Unmarshal(data, output) != nil {
			return response.StatusCode, fmt.Errorf("GitHub API returned invalid JSON")
		}
	}
	return response.StatusCode, nil
}

func (g *githubRemote) stageBundle(ctx context.Context, token string, plan Plan, bundlePath string) (string, func(), error) {
	root, err := os.MkdirTemp("", "engineering-platform-publisher-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	if err := os.Chmod(root, 0o700); err != nil {
		cleanup()
		return "", func() {}, err
	}
	repository := filepath.Join(root, "repository")
	home := filepath.Join(root, "home")
	if err := os.Mkdir(repository, 0o700); err != nil {
		cleanup()
		return "", func() {}, err
	}
	if err := os.Mkdir(home, 0o700); err != nil {
		cleanup()
		return "", func() {}, err
	}
	if _, err := g.gitOutput(ctx, token, repository, home, "init", "--bare", "--object-format=sha1"); err != nil {
		cleanup()
		return "", func() {}, err
	}
	remoteURL := "https://github.com/" + plan.Repository + ".git"
	if _, err := g.gitOutput(ctx, token, repository, home,
		"fetch", "--no-tags", "--no-recurse-submodules", "--depth=1", "--no-write-fetch-head",
		remoteURL, "refs/heads/"+plan.BaseRef+":refs/ep/base"); err != nil {
		cleanup()
		return "", func() {}, err
	}
	base, err := g.gitOutput(ctx, token, repository, home, "rev-parse", "--verify", "refs/ep/base^{commit}")
	if err != nil || strings.TrimSpace(base) != plan.BaseCommit {
		cleanup()
		return "", func() {}, fmt.Errorf("fetched target base does not match frozen commit")
	}
	if _, err := g.gitOutput(ctx, token, repository, home, "bundle", "verify", bundlePath); err != nil {
		cleanup()
		return "", func() {}, err
	}
	if _, err := g.gitOutput(ctx, token, repository, home,
		"fetch", "--no-tags", "--no-recurse-submodules", "--no-write-fetch-head",
		bundlePath, "HEAD:refs/ep/result"); err != nil {
		cleanup()
		return "", func() {}, err
	}
	result, err := g.gitOutput(ctx, token, repository, home, "rev-parse", "--verify", "refs/ep/result^{commit}")
	if err != nil || strings.TrimSpace(result) != plan.ResultCommit {
		cleanup()
		return "", func() {}, fmt.Errorf("Git bundle does not contain the retained result commit")
	}
	if _, err := g.gitOutput(ctx, token, repository, home,
		"merge-base", "--is-ancestor", "refs/ep/base", "refs/ep/result"); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("retained result is not based on frozen target commit")
	}
	if _, err := g.gitOutput(ctx, token, repository, home, "fsck", "--strict", "--no-reflogs"); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return repository, cleanup, nil
}

func (g *githubRemote) push(ctx context.Context, token, staged string, plan Plan) error {
	home := filepath.Join(filepath.Dir(staged), "home")
	remoteURL := "https://github.com/" + plan.Repository + ".git"
	_, err := g.gitOutput(ctx, token, staged, home, "push", "--porcelain", "--no-verify",
		remoteURL, "refs/ep/result:refs/heads/"+plan.Branch)
	return err
}

type boundedOutput struct{ data []byte }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > 4<<20 {
		return 0, fmt.Errorf("Git output limit exceeded")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func (g *githubRemote) gitOutput(ctx context.Context, token, directory, home string, args ...string) (string, error) {
	fixed := []string{
		"-c", "core.hooksPath=/dev/null",
		"-c", "core.fsmonitor=false",
		"-c", "credential.helper=",
		"-c", "protocol.allow=never",
		"-c", "protocol.https.allow=always",
		"-c", "protocol.file.allow=always",
		"-c", "fetch.fsckObjects=true",
		"-c", "transfer.fsckObjects=true",
		"-c", "gc.auto=0",
		"-C", directory,
	}
	command := exec.CommandContext(ctx, g.git, append(fixed, args...)...)
	basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	command.Env = []string{
		"PATH=" + filepath.Dir(g.git) + ":/usr/bin:/bin",
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + home,
		"LANG=C",
		"LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_ATTR_NOSYSTEM=1",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.https://github.com/.extraheader",
		"GIT_CONFIG_VALUE_0=AUTHORIZATION: basic " + basic,
	}
	command.WaitDelay = 2 * time.Second
	var output, diagnostic boundedOutput
	command.Stdout, command.Stderr = &output, &diagnostic
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("publisher Git command failed: %w", err)
	}
	return string(output.data), nil
}

func escapeRef(ref string) string {
	parts := strings.Split(ref, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}
