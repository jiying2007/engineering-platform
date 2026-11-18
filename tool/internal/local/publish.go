package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Publication struct {
	State      string `json:"state"`
	Repository string `json:"repository"`
	Branch     string `json:"branch"`
	Commit     string `json:"commit"`
	URL        string `json:"url,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type githubPull struct {
	Number int    `json:"number"`
	URL    string `json:"html_url"`
	State  string `json:"state"`
	Head   struct {
		Ref  string `json:"ref"`
		SHA  string `json:"sha"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"head"`
	Base struct {
		Ref  string `json:"ref"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"base"`
}

var githubRepository = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// Publish is optional and explicit. It writes an intent before each external
// effect and reads exact remote identities. An ambiguous old intent never
// authorizes a repeat, even if the first read-back reports no matching object.
func Publish(ctx context.Context, s *Store, taskID, repository string) (Publication, error) {
	result := Publication{State: "waiting", Repository: repository}
	if !githubRepository.MatchString(repository) {
		return result, errors.New("--repo must be a GitHub OWNER/REPO")
	}
	scheduler, err := AcquireScheduler(s.DataDir())
	if err != nil {
		return result, fmt.Errorf("stop serve or wait for foreground work before publication: %w", err)
	}
	defer scheduler.Close()
	if err := ensureExecutionSlot(ctx, s); err != nil {
		return result, err
	}
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return result, err
	}
	if task.State != StateDone {
		return result, errors.New("only a completed local delivery can be published")
	}
	attempts, err := s.ListAttempts(ctx, task.ID)
	if err != nil {
		return result, err
	}
	if len(attempts) == 0 {
		return result, errors.New("task has no execution result")
	}
	attempt := attempts[len(attempts)-1]
	result.Branch, result.Commit = task.Branch, attempt.Result.ResultCommit
	if !validCommit(result.Commit) || !attempt.Result.LocalProcessStopped || attempt.Result.RemoteOutcomeUnknown {
		return result, errors.New("publication requires an exact result and confirmed local completion")
	}
	repoLock, _, err := AcquireRepository(ctx, task.Config.Project.Root)
	if err != nil {
		return result, err
	}
	defer repoLock.Close()
	if err := verifyWorkspace(ctx, task); err != nil {
		return result, err
	}
	head, err := gitOutput(ctx, task.Worktree, "rev-parse", "HEAD")
	if err != nil || head != result.Commit {
		return result, errors.New("worktree HEAD changed after the recorded result; publish the reviewed commit manually or create a new task")
	}
	changed, err := trackedWorkspaceChanged(ctx, task, result.Commit)
	if err != nil || changed {
		return result, errors.New("worktree changed after verification; publication is blocked")
	}
	origin, err := gitOutput(ctx, task.Config.Project.Root, "remote", "get-url", "origin")
	if err != nil || !matchingGitHubOrigin(origin, repository) {
		return result, errors.New("origin must identify the exact requested github.com repository without embedded credentials")
	}
	base := strings.TrimPrefix(task.Config.Project.Base, "refs/heads/")
	if validCommit(base) || base == "HEAD" || strings.HasPrefix(base, "refs/") {
		return result, errors.New("automatic PR publication needs a branch base; use the existing manual Git flow for a detached base")
	}
	if _, err := gitOutput(ctx, task.Config.Project.Root, "check-ref-format", "--branch", base); err != nil {
		return result, err
	}
	events, err := s.ListEvents(ctx, task.ID)
	if err != nil {
		return result, err
	}
	priorPush, priorPR, restored := false, false, false
	for _, event := range events {
		if event.Kind == "task.restored_reconcile" {
			restored = true
		}
		if event.Kind != "publication.intent" {
			continue
		}
		if event.Data["repository"] != repository || event.Data["branch"] != task.Branch || event.Data["commit"] != result.Commit {
			return result, errors.New("task already has a different publication intent; reconcile it manually")
		}
		if event.Data["phase"] == "push" {
			priorPush = true
		}
		if event.Data["phase"] == "pr" {
			priorPR = true
		}
	}
	opDir, err := os.MkdirTemp(filepath.Join(s.DataDir(), "tasks", task.ID), "publish-")
	if err != nil {
		return result, err
	}
	session := filepath.Base(opDir)
	if err := s.AddEvent(ctx, Event{TaskID: task.ID, AttemptID: attempt.ID, Kind: "publication.started", Actor: currentActor(), Data: map[string]any{"session_id": session}}); err != nil {
		return result, err
	}
	defer func() {
		endCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.AddEvent(endCtx, Event{TaskID: task.ID, AttemptID: attempt.ID, Kind: "publication.ended", Actor: "eng.publish", Data: map[string]any{"session_id": session}})
	}()
	stopRequested := func() error {
		events, err := s.ListEvents(ctx, task.ID)
		if err != nil {
			return err
		}
		for _, event := range events {
			if event.Kind == "publication.stop_requested" && event.Data["session_id"] == session {
				return errors.New("publication stopped by user; only a new explicit read-back is permitted")
			}
		}
		return nil
	}
	sequence := 0
	unresolved := false
	run := func(argv []string) (string, error) {
		if unresolved {
			return "", ErrProcessUnresolved
		}
		if err := stopRequested(); err != nil {
			return "", err
		}
		sequence++
		operation := fmt.Sprintf("%s-%d", filepath.Base(opDir), sequence)
		commandCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		stdout := filepath.Join(opDir, fmt.Sprintf("%02d.stdout.log", sequence))
		stderr := filepath.Join(opDir, fmt.Sprintf("%02d.stderr.log", sequence))
		outcome := runSupervised(commandCtx, argv, task.Worktree, "", publishingEnvironment(), stdout, stderr, []*os.File{scheduler.file, repoLock.file}, func(id ProcessIdentity) error {
			if err := stopRequested(); err != nil {
				return err
			}
			if err := writeLockRecord(repoLock, id); err != nil {
				return err
			}
			if err := s.AddEvent(commandCtx, Event{TaskID: task.ID, AttemptID: attempt.ID, Kind: "publication.process", Actor: currentActor(), Data: map[string]any{"operation_id": operation, "session_id": session, "process": id, "argv": argv}}); err != nil {
				return err
			}
			return stopRequested()
		})
		if !outcome.Stopped {
			unresolved = true
		}
		if outcome.Stopped && outcome.Identity != nil {
			recordCtx, recordCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer recordCancel()
			if err := s.AddEvent(recordCtx, Event{TaskID: task.ID, AttemptID: attempt.ID, Kind: "publication.process_stopped", Actor: "eng.publish", Data: map[string]any{"operation_id": operation, "exit_code": outcome.ExitCode}}); err != nil {
				return "", err
			}
			if err := writeLockRecord(repoLock, nil); err != nil {
				return "", err
			}
		}
		if outcome.Err != nil || outcome.ExitCode != 0 || !outcome.Stopped {
			return "", fmt.Errorf("publication command was not confirmed successful (exit %d, stopped %t); inspect %s", outcome.ExitCode, outcome.Stopped, opDir)
		}
		raw, err := readRegularFile(stdout, 4<<20)
		return strings.TrimSpace(string(raw)), err
	}
	git := func(args ...string) (string, error) {
		argv := []string{"git", "-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-C", task.Config.Project.Root}
		return run(append(argv, args...))
	}
	// Resolve both URLs using the same user credential/config environment as
	// publication. A separate pushurl must never silently target another repo.
	for _, args := range [][]string{{"remote", "get-url", "--all", "origin"}, {"remote", "get-url", "--push", "--all", "origin"}} {
		url, err := git(args...)
		if err != nil || !matchingGitHubOrigin(url, repository) {
			return result, errors.New("origin fetch and push URLs must each identify exactly the requested GitHub repository")
		}
	}
	remoteHead := func() (string, error) {
		raw, err := git("ls-remote", "--refs", "origin", "refs/heads/"+task.Branch)
		if err != nil {
			return "", err
		}
		if raw == "" {
			return "", nil
		}
		fields := strings.Fields(raw)
		if len(fields) != 2 || !validCommit(fields[0]) || fields[1] != "refs/heads/"+task.Branch {
			return "", errors.New("remote returned an ambiguous branch identity")
		}
		return fields[0], nil
	}
	readPull := func() (*githubPull, error) {
		owner := strings.Split(repository, "/")[0]
		raw, err := run([]string{"gh", "api", "--method", "GET", "repos/" + repository + "/pulls", "-f", "head=" + owner + ":" + task.Branch, "-f", "state=all", "-f", "per_page=100"})
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(strings.TrimSpace(raw), "[") {
			return nil, errors.New("GitHub pull read-back must be an explicit JSON list")
		}
		var pulls []githubPull
		if err := json.Unmarshal([]byte(raw), &pulls); err != nil {
			return nil, errors.New("GitHub pull read-back was not a complete JSON response")
		}
		if len(pulls) >= 100 {
			return nil, errors.New("pull read-back is truncated; reconcile manually")
		}
		var found *githubPull
		for i := range pulls {
			p := &pulls[i]
			if p.Head.Ref != task.Branch || !strings.EqualFold(p.Head.Repo.FullName, repository) || !strings.EqualFold(p.Base.Repo.FullName, repository) {
				return nil, errors.New("GitHub read-back returned a mismatched repository or branch")
			}
			if p.Head.SHA != result.Commit || p.Base.Ref != base || p.Number <= 0 || (p.State != "open" && p.State != "closed") || p.URL != fmt.Sprintf("https://github.com/%s/pull/%d", p.Base.Repo.FullName, p.Number) {
				return nil, errors.New("existing PR does not match the exact reviewed result/base; no update was made")
			}
			if found != nil {
				return nil, errors.New("more than one PR uses the task branch; reconcile manually")
			}
			found = p
		}
		return found, nil
	}
	intent := func(phase string) error {
		return s.AddEvent(ctx, Event{TaskID: task.ID, AttemptID: attempt.ID, Kind: "publication.intent", Actor: currentActor(), Data: map[string]any{"phase": phase, "repository": repository, "branch": task.Branch, "commit": result.Commit, "base": base}})
	}
	unknown := func(reason string, cause error) (Publication, error) {
		result.State, result.Reason = "waiting", reason
		recordCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := s.AddEvent(recordCtx, Event{TaskID: task.ID, AttemptID: attempt.ID, Kind: "publication.unknown", Actor: "eng.publish", Data: map[string]any{"reason": reason, "repository": repository, "branch": task.Branch, "commit": result.Commit}})
		return result, errors.Join(cause, err)
	}
	sha, err := remoteHead()
	if err != nil {
		return unknown("remote branch could not be observed; no mutation was authorized", err)
	}
	pull, err := readPull()
	if err != nil {
		return unknown("PR identity could not be confirmed; no mutation was authorized", err)
	}
	confirm := func(p *githubPull) (Publication, error) {
		result.State, result.URL, result.Reason = "confirmed", p.URL, "exact repository, branch, commit and PR observed; no merge requested"
		return result, s.AddEvent(ctx, Event{TaskID: task.ID, AttemptID: attempt.ID, Kind: "publication.confirmed", Actor: "eng.publish", Data: map[string]any{"repository": repository, "branch": task.Branch, "commit": result.Commit, "url": p.URL, "pr_state": p.State}})
	}
	if sha != "" && sha != result.Commit {
		return unknown("remote task branch has a different commit; no overwrite was attempted", nil)
	}
	if pull != nil {
		// A merged/closed PR remains an observed publication, even after its task
		// branch was deleted. Never recreate or reopen that branch or PR.
		if sha == result.Commit || pull.State == "closed" {
			return confirm(pull)
		}
		return unknown("PR exists but its branch cannot be confirmed; reconcile manually", nil)
	}
	if restored || priorPR || (priorPush && sha == "") {
		return unknown("a restored snapshot or prior intent may have external effects; nothing was resent; inspect GitHub and retained logs", nil)
	}
	if sha == "" {
		if err := intent("push"); err != nil {
			return result, err
		}
		// An empty expected value means create-only. Although Git names this a
		// force-with-lease option, it refuses every existing remote branch,
		// including a concurrent fast-forward collision. No overwrite is allowed.
		_, pushErr := git("push", "--porcelain", "--no-follow-tags", "--recurse-submodules=no", "--force-with-lease=refs/heads/"+task.Branch+":", "origin", result.Commit+":refs/heads/"+task.Branch)
		sha, err = remoteHead()
		if err != nil || sha != result.Commit {
			return unknown("push outcome requires exact remote read-back; do not retry blindly", errors.Join(pushErr, err))
		}
	}
	if err := intent("pr"); err != nil {
		return result, err
	}
	body := filepath.Join(opDir, "pr-body.md")
	text := fmt.Sprintf("## Local engineering delivery\n\nTask: `%s`\n\nAttempt: `%d`\n\nExact result: `%s`\n\nChecks: `%s`\n\nReview the diff and project checks before acceptance. This command did not merge, deploy, or verify hardware.\n", task.ID, attempt.Number, result.Commit, attempt.Result.ChecksStatus)
	if err := writeExclusive(body, []byte(text)); err != nil {
		return unknown("PR intent retained, but body could not be written; reconcile manually", err)
	}
	_, createErr := run([]string{"gh", "pr", "create", "--repo", repository, "--head", task.Branch, "--base", base, "--title", "eng: " + task.Project + " " + task.ID, "--body-file", body})
	pull, err = readPull()
	if err != nil || pull == nil {
		return unknown("PR create outcome is unknown; next invocation only reads it back", errors.Join(createErr, err))
	}
	sha, err = remoteHead()
	if err != nil || (sha != result.Commit && pull.State != "closed") {
		return unknown("post-publication branch identity changed; no success claimed", err)
	}
	return confirm(pull)
}

func matchingGitHubOrigin(origin, repository string) bool {
	for _, prefix := range []string{"https://github.com/", "git@github.com:", "ssh://git@github.com/"} {
		if strings.HasPrefix(origin, prefix) {
			return strings.EqualFold(strings.TrimSuffix(strings.TrimPrefix(origin, prefix), ".git"), repository)
		}
	}
	return false
}

func publishingEnvironment() []string {
	env := []string{}
	for _, item := range os.Environ() {
		if strings.HasPrefix(item, "GIT_") || strings.HasPrefix(item, "GH_HOST=") || strings.HasPrefix(item, "GH_PROMPT_DISABLED=") {
			continue
		}
		env = append(env, item)
	}
	return append(env, "GIT_TERMINAL_PROMPT=0", "GH_HOST=github.com", "GH_PROMPT_DISABLED=1")
}

type publicationProcess struct {
	operation string
	identity  ProcessIdentity
	attempt   string
	session   string
}

func pendingPublicationProcesses(ctx context.Context, s *Store, taskID string) ([]publicationProcess, error) {
	events, err := s.ListEvents(ctx, taskID)
	if err != nil {
		return nil, err
	}
	pending := map[string]publicationProcess{}
	for _, event := range events {
		operation, _ := event.Data["operation_id"].(string)
		if event.Kind == "publication.process_stopped" {
			delete(pending, operation)
		}
		if event.Kind != "publication.process" {
			continue
		}
		raw, err := json.Marshal(event.Data["process"])
		if err != nil {
			return nil, err
		}
		var id ProcessIdentity
		if err := json.Unmarshal(raw, &id); err != nil || operation == "" || id.PID <= 1 {
			return nil, errors.New("publication process identity is invalid")
		}
		session, _ := event.Data["session_id"].(string)
		pending[operation] = publicationProcess{operation, id, event.AttemptID, session}
	}
	result := []publicationProcess{}
	for _, process := range pending {
		result = append(result, process)
	}
	return result, nil
}

func ensurePublicationStopped(ctx context.Context, s *Store) error {
	tasks, err := s.ListTasks(ctx)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		pending, err := pendingPublicationProcesses(ctx, s, task.ID)
		if err != nil {
			return err
		}
		for _, process := range pending {
			alive, err := ProcessGroupAlive(process.identity)
			if err != nil || alive {
				return fmt.Errorf("%w: publication for task %s; use eng stop %s then read back publication", ErrProcessUnresolved, task.ID, task.ID)
			}
			if err := s.AddEvent(ctx, Event{TaskID: task.ID, AttemptID: process.attempt, Kind: "publication.process_stopped", Actor: "eng.recover", Data: map[string]any{"operation_id": process.operation, "observed_stopped": true}}); err != nil {
				return err
			}
		}
	}
	return nil
}

func StopPublication(ctx context.Context, s *Store, taskID string) (bool, error) {
	pending, err := pendingPublicationProcesses(ctx, s, taskID)
	if err != nil {
		return false, err
	}
	events, err := s.ListEvents(ctx, taskID)
	if err != nil {
		return false, err
	}
	sessions := map[string]string{}
	for _, event := range events {
		session, _ := event.Data["session_id"].(string)
		if event.Kind == "publication.started" {
			sessions[session] = event.AttemptID
		}
		if event.Kind == "publication.ended" {
			delete(sessions, session)
		}
	}
	for _, process := range pending {
		sessions[process.session] = process.attempt
	}
	if len(sessions) == 0 {
		return false, nil
	}
	for session, attempt := range sessions {
		if err := s.AddEvent(ctx, Event{TaskID: taskID, AttemptID: attempt, Kind: "publication.stop_requested", Actor: currentActor(), Data: map[string]any{"session_id": session}}); err != nil {
			return true, err
		}
	}
	pending, err = pendingPublicationProcesses(ctx, s, taskID)
	if err != nil {
		return true, err
	}
	for _, process := range pending {
		stopped, err := stopProcessGroup(process.identity, 500*time.Millisecond)
		if err != nil || !stopped {
			return true, errors.Join(ErrProcessUnresolved, err)
		}
		if err := s.AddEvent(ctx, Event{TaskID: taskID, AttemptID: process.attempt, Kind: "publication.process_stopped", Actor: currentActor(), Data: map[string]any{"operation_id": process.operation, "stop_requested": true, "external_outcome": "unknown"}}); err != nil {
			return true, err
		}
	}
	return true, nil
}
