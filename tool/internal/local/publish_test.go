package local

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Git pushes use a real local bare repository. Only GitHub's response/transport
// is replaced by deterministic processes, including an accepted-but-lost reply.
type publicationFixture struct {
	store                *Store
	task                 Task
	bare, state, realGit string
}

func makePublicationFixture(t *testing.T, mode string) publicationFixture {
	t.Helper()
	ctx := context.Background()
	store, root := executionStore(t), executionRepository(t)
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bare := filepath.Join(t.TempDir(), "remote.git")
	cmd := exec.Command(realGit, "clone", "--bare", root, bare)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone bare: %s: %v", out, err)
	}
	owner := executionOwner(t, store)
	task := executionTask(t, store, root, "cat > source.txt", []Check{{Name: "actual", Argv: []string{"/bin/sh", "-c", "grep -qx after source.txt"}, Timeout: "5s"}}, nil, false, "5s")
	task, err = Execute(ctx, store, task.ID)
	owner.Close()
	if err != nil || task.State != StateDone {
		t.Fatalf("local delivery: %+v %v", task, err)
	}
	if _, err := gitOutput(ctx, root, "remote", "add", "origin", "https://github.com/example/project.git"); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	state := filepath.Join(bin, "state.json")
	attempt := lastExecutionAttempt(t, store, task)
	value := map[string]any{"mode": mode, "branch": task.Branch, "sha": attempt.Result.ResultCommit, "base": task.BaseSHA, "pushes": 0, "creates": 0, "pr": false}
	raw, _ := json.Marshal(value)
	if err := os.WriteFile(state, raw, 0600); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{"git": gitPublicationFixture, "gh": githubPublicationFixture} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(source), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("ENG_TEST_REAL_GIT", realGit)
	t.Setenv("ENG_TEST_REMOTE", bare)
	t.Setenv("ENG_TEST_PUBLISH_STATE", state)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return publicationFixture{store, task, bare, state, realGit}
}

const gitPublicationFixture = `#!/usr/bin/python3
import json, os, subprocess, sys, time
args = sys.argv[1:]
real = os.environ['ENG_TEST_REAL_GIT']
remote = os.environ['ENG_TEST_REMOTE']
state = os.environ['ENG_TEST_PUBLISH_STATE']
if 'ls-remote' in args:
    args[args.index('origin')] = remote
elif 'push' in args:
    data = json.load(open(state))
    data['pushes'] += 1
    json.dump(data, open(state, 'w'))
    if data['mode'] == 'collision':
        subprocess.check_call([real, '--git-dir', remote, 'update-ref', 'refs/heads/'+data['branch'], data['base']])
    args[args.index('origin')] = remote
    code = subprocess.call([real] + args)
    if data['mode'] == 'stop_after_push':
        open(state+'.started', 'w').write('remote push completed')
        while True: time.sleep(1)
    sys.exit(code)
os.execv(real, [real] + args)
`

const githubPublicationFixture = `#!/usr/bin/python3
import json, os, sys
path = os.environ['ENG_TEST_PUBLISH_STATE']
data = json.load(open(path))
args = sys.argv[1:]
if args[0] == 'api':
    if data['mode'] == 'null':
        print('null'); sys.exit(0)
    if data.get('hide_once'):
        data['hide_once'] = False
        json.dump(data, open(path, 'w'))
        print('[]'); sys.exit(0)
    if not data['pr']:
        print('[]'); sys.exit(0)
    repo = 'example/project'
    print(json.dumps([{'number':7, 'html_url':'https://github.com/'+repo+'/pull/7', 'state':'open',
        'head':{'ref':data['branch'], 'sha':data['sha'], 'repo':{'full_name':repo}},
        'base':{'ref':'main','repo':{'full_name':repo}}}]))
    sys.exit(0)
if args[:2] == ['pr','create']:
    data['creates'] += 1
    data['pr'] = True
    if data['mode'] == 'lost_reply': data['hide_once'] = True
    json.dump(data, open(path, 'w'))
    if data['mode'] == 'lost_reply': sys.exit(1)
    print('https://github.com/example/project/pull/7'); sys.exit(0)
sys.exit(2)
`

func (f publicationFixture) counts(t *testing.T) (int, int) {
	t.Helper()
	raw, err := os.ReadFile(f.state)
	if err != nil {
		t.Fatal(err)
	}
	var value struct{ Pushes, Creates int }
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value.Pushes, value.Creates
}

func TestPublishExactGitResultAndIdempotentPR(t *testing.T) {
	f := makePublicationFixture(t, "success")
	for range 2 {
		result, err := Publish(context.Background(), f.store, f.task.ID, "example/project")
		if err != nil || result.State != "confirmed" || result.URL != "https://github.com/example/project/pull/7" {
			t.Fatalf("publication: %+v %v", result, err)
		}
	}
	pushes, creates := f.counts(t)
	if pushes != 1 || creates != 1 {
		t.Fatalf("effects repeated: pushes=%d creates=%d", pushes, creates)
	}
	cmd := exec.Command(f.realGit, "--git-dir", f.bare, "rev-parse", "refs/heads/"+f.task.Branch)
	raw, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(raw)) != lastExecutionAttempt(t, f.store, f.task).Result.ResultCommit {
		t.Fatalf("wrong remote result: %s %v", raw, err)
	}
}

func TestPublishLostReplyOnlyReadsBack(t *testing.T) {
	f := makePublicationFixture(t, "lost_reply")
	first, err := Publish(context.Background(), f.store, f.task.ID, "example/project")
	if first.State != "waiting" || err == nil {
		t.Fatalf("ambiguous result promoted: %+v %v", first, err)
	}
	second, err := Publish(context.Background(), f.store, f.task.ID, "example/project")
	if second.State != "confirmed" || err != nil {
		t.Fatalf("read-back: %+v %v", second, err)
	}
	pushes, creates := f.counts(t)
	if pushes != 1 || creates != 1 {
		t.Fatalf("lost reply resent: %d %d", pushes, creates)
	}
}

func TestPublishUnknownOrRestoredNeverBlindlyRetries(t *testing.T) {
	for _, reason := range []string{"push", "pr", "restore"} {
		t.Run(reason, func(t *testing.T) {
			f := makePublicationFixture(t, "success")
			if reason == "restore" {
				if err := f.store.MarkRestored(context.Background()); err != nil {
					t.Fatal(err)
				}
			} else {
				err := f.store.AddEvent(context.Background(), Event{TaskID: f.task.ID, Kind: "publication.intent", Actor: "fixture", Data: map[string]any{"phase": reason, "repository": "example/project", "branch": f.task.Branch, "commit": lastExecutionAttempt(t, f.store, f.task).Result.ResultCommit}})
				if err != nil {
					t.Fatal(err)
				}
			}
			result, err := Publish(context.Background(), f.store, f.task.ID, "example/project")
			if err != nil || result.State != "waiting" {
				t.Fatalf("unexpected: %+v %v", result, err)
			}
			pushes, creates := f.counts(t)
			if pushes != 0 || creates != 0 {
				t.Fatal("ambiguous or restored intent caused external writes")
			}
		})
	}
}

func TestPublishCollisionCannotFastForwardExistingBranch(t *testing.T) {
	f := makePublicationFixture(t, "collision")
	result, _ := Publish(context.Background(), f.store, f.task.ID, "example/project")
	if result.State != "waiting" {
		t.Fatalf("collision accepted: %+v", result)
	}
	cmd := exec.Command(f.realGit, "--git-dir", f.bare, "rev-parse", "refs/heads/"+f.task.Branch)
	raw, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(raw)) != f.task.BaseSHA {
		t.Fatalf("existing branch overwritten: %s %v", raw, err)
	}
	_, creates := f.counts(t)
	if creates != 0 {
		t.Fatal("PR created after branch collision")
	}
}

func TestPublishRejectsDifferentPushDestinationAndNullReadback(t *testing.T) {
	for _, mode := range []string{"pushurl", "null"} {
		t.Run(mode, func(t *testing.T) {
			f := makePublicationFixture(t, mode)
			if mode == "pushurl" {
				if _, err := gitOutput(context.Background(), f.task.Config.Project.Root, "config", "remote.origin.pushurl", "https://github.com/other/project.git"); err != nil {
					t.Fatal(err)
				}
			}
			result, err := Publish(context.Background(), f.store, f.task.ID, "example/project")
			if err == nil || result.State == "confirmed" {
				t.Fatalf("bad identity/read accepted: %+v %v", result, err)
			}
			pushes, creates := f.counts(t)
			if pushes != 0 || creates != 0 {
				t.Fatal("external write before read-back identity verified")
			}
		})
	}
}

func TestPublishStopAfterPushDoesNotCreatePR(t *testing.T) {
	f := makePublicationFixture(t, "stop_after_push")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan Publication, 1)
	go func() { result, _ := Publish(ctx, f.store, f.task.ID, "example/project"); done <- result }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(f.state + ".started"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("push never reached controlled stop point")
		}
		time.Sleep(10 * time.Millisecond)
	}
	handled, err := StopPublication(ctx, f.store, f.task.ID)
	if !handled || err != nil {
		t.Fatalf("stop: %t %v", handled, err)
	}
	select {
	case result := <-done:
		if result.State != "waiting" {
			t.Fatalf("stopped publication promoted: %+v", result)
		}
	case <-ctx.Done():
		t.Fatal("publication did not stop")
	}
	pushes, creates := f.counts(t)
	if pushes != 1 || creates != 0 {
		t.Fatalf("stop caused next effect: %d pushes, %d PR creates", pushes, creates)
	}
}
