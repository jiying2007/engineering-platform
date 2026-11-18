package local

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var Version = "dev"

const helpText = `eng — local engineering tasks

Usage: eng [--data-dir DIR] COMMAND

  doctor                              Check local prerequisites without a model call
  runtime add NAME --model|--no-model [--stdin task|none] [--timeout 30m] -- COMMAND [ARG...]
  runtime list
  project add NAME PATH [--config FILE] [--runtime NAME] [--base REF] [--replace]
  project list
  run PROJECT --task FILE|--text TEXT   Create and run a task, or queue it for serve
  submit PROJECT --task FILE|--text TEXT  Create a queued task
  status [TASK]                        JSON task, attempts and event history
  logs TASK [--attempt N] [--stderr] [--tail 200]
  stop TASK                           Request process-group cancellation
  resume TASK [--task FILE|--text TEXT]  Explicitly create a new attempt
  accept TASK --note TEXT              Accept a successful result without configured checks
  recover                             Inspect interrupted work; never invoke a model
  serve                               Own the local queue until stopped
  publish TASK --repo OWNER/REPO       Optional GitHub publication; unknown results are read back
  backup DEST                         Quiescent, consistent backup to a new directory
  restore SOURCE                      Restore into an absent data directory; queue stays paused
  version

All task results are local deliveries. Repository acceptance, merge and device
verification remain explicit. Runtime credentials stay in the existing CLI's
private configuration. A worktree is not an OS security sandbox.
`

func CLI(ctx context.Context, args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("eng", flag.ContinueOnError)
	fs.SetOutput(errOut)
	defaultDir := os.Getenv("ENG_DATA_DIR")
	if defaultDir == "" {
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			defaultDir = filepath.Join(xdg, "eng")
		} else if home, err := os.UserHomeDir(); err == nil {
			defaultDir = filepath.Join(home, ".local", "share", "eng")
		}
	}
	dataDir := fs.String("data-dir", defaultDir, "local state directory")
	_ = fs.Bool("json", true, "machine-readable output (the default)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = io.WriteString(out, helpText)
			return 0
		}
		return 2
	}
	args = fs.Args()
	if len(args) == 0 || args[0] == "help" {
		_, _ = io.WriteString(out, helpText)
		return 0
	}
	if args[0] == "version" {
		return writeCLIResult(out, errOut, map[string]string{"version": Version}, nil)
	}
	if *dataDir == "" {
		return writeCLIResult(out, errOut, nil, errors.New("set --data-dir to a local directory"))
	}
	absDir, err := filepath.Abs(*dataDir)
	if err != nil {
		return writeCLIResult(out, errOut, nil, err)
	}
	if args[0] == "restore" {
		if len(args) != 2 {
			return writeCLIResult(out, errOut, nil, errors.New("usage: eng --data-dir ORIGINAL_PATH restore BACKUP"))
		}
		err = Restore(ctx, args[1], absDir)
		return writeCLIResult(out, errOut, map[string]any{"restored": err == nil, "data_dir": absDir, "queue": "paused_for_reconciliation"}, err)
	}
	if _, err := os.Lstat(filepath.Join(absDir, ".restore-incomplete")); err == nil {
		return writeCLIResult(out, errOut, nil, errors.New("this directory contains an incomplete restore; preserve it and restore a verified backup into the absent original path"))
	} else if !errors.Is(err, os.ErrNotExist) {
		return writeCLIResult(out, errOut, nil, err)
	}
	if handled, exitCode := ForwardCLI(ctx, absDir, args, out, errOut); handled {
		return exitCode
	}
	s, err := OpenStore(absDir)
	if err != nil {
		return writeCLIResult(out, errOut, nil, err)
	}
	defer s.Close()
	value, err := command(ctx, s, args, out, errOut)
	return writeCLIResult(out, errOut, value, err)
}

func writeCLIResult(out, errOut io.Writer, value any, err error) int {
	if value != nil {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if writeErr := enc.Encode(value); writeErr != nil {
			_, _ = fmt.Fprintln(errOut, "eng: write output:", writeErr)
			return 1
		}
	}
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "eng:", err)
		return 1
	}
	return 0
}

func command(ctx context.Context, s *Store, args []string, out, errOut io.Writer) (any, error) {
	switch args[0] {
	case "doctor":
		return doctor(s.DataDir())
	case "runtime":
		return runtimeCommand(s.DataDir(), args[1:], errOut)
	case "project":
		return projectCommand(ctx, s.DataDir(), args[1:], errOut)
	case "run", "submit":
		if len(args) < 2 {
			return nil, errors.New("usage: eng run|submit PROJECT (--task FILE | --text TEXT)")
		}
		fs := newFlags(args[0], errOut)
		file := fs.String("task", "", "instruction file")
		text := fs.String("text", "", "inline task text (visible in shell history; use --task FILE for sensitive text)")
		if err := fs.Parse(args[2:]); err != nil {
			return nil, err
		}
		fileSet, textSet := instructionFlags(fs)
		if fs.NArg() != 0 || fileSet == textSet {
			return nil, errors.New("provide exactly one of --task FILE or --text TEXT")
		}
		var instruction string
		var err error
		if fileSet {
			instruction, err = readInstruction(*file)
		} else {
			instruction, err = validateInstruction(*text)
		}
		if err != nil {
			return nil, err
		}
		task, err := newTask(ctx, s, args[1], instruction)
		if err != nil || args[0] == "submit" {
			return task, err
		}
		return runQueued(ctx, s, task.ID)
	case "status":
		if len(args) == 1 {
			return s.ListTasks(ctx)
		}
		if len(args) != 2 {
			return nil, errors.New("usage: eng status [TASK]")
		}
		t, err := s.GetTask(ctx, args[1])
		if err != nil {
			return nil, err
		}
		a, err := s.ListAttempts(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		e, err := s.ListEvents(ctx, t.ID)
		return map[string]any{"task": t, "attempts": a, "events": e}, err
	case "logs":
		return nil, logCommand(ctx, s, args[1:], out, errOut)
	case "stop":
		if len(args) != 2 {
			return nil, errors.New("usage: eng stop TASK")
		}
		if handled, err := StopPublication(ctx, s, args[1]); handled || err != nil {
			if err != nil {
				return nil, err
			}
			return s.GetTask(ctx, args[1])
		}
		if err := StopTask(ctx, s, args[1]); err != nil {
			return nil, err
		}
		return s.GetTask(ctx, args[1])
	case "resume":
		if len(args) < 2 {
			return nil, errors.New("usage: eng resume TASK [--task FILE | --text TEXT]")
		}
		fs := newFlags("resume", errOut)
		file := fs.String("task", "", "replacement instruction file")
		text := fs.String("text", "", "replacement inline task text")
		if err := fs.Parse(args[2:]); err != nil || fs.NArg() != 0 {
			return nil, errors.New("usage: eng resume TASK [--task FILE | --text TEXT]")
		}
		fileSet, textSet := instructionFlags(fs)
		if fileSet && textSet {
			return nil, errors.New("provide at most one replacement: --task FILE or --text TEXT")
		}
		var instruction string
		var err error
		if fileSet {
			instruction, err = readInstruction(*file)
		} else if textSet {
			instruction, err = validateInstruction(*text)
		}
		if err != nil {
			return nil, err
		}
		task, err := s.EnqueueResume(ctx, args[1], instruction)
		if err != nil {
			return nil, err
		}
		return runQueued(ctx, s, task.ID)
	case "accept":
		if len(args) < 2 {
			return nil, errors.New("usage: eng accept TASK --note TEXT")
		}
		fs := newFlags("accept", errOut)
		note := fs.String("note", "", "actual review and acceptance note")
		if err := fs.Parse(args[2:]); err != nil || fs.NArg() != 0 {
			return nil, errors.New("usage: eng accept TASK --note TEXT")
		}
		return s.AcceptTask(ctx, args[1], *note, currentActor())
	case "recover":
		lock, err := AcquireScheduler(s.DataDir())
		if err != nil {
			return nil, fmt.Errorf("stop the current scheduler before recovery: %w", err)
		}
		defer lock.Close()
		return Recover(ctx, s)
	case "serve":
		return nil, Serve(ctx, s, errOut)
	case "backup":
		if len(args) != 2 {
			return nil, errors.New("usage: eng backup NEW_DIRECTORY")
		}
		return Backup(ctx, s, args[1])
	case "publish":
		if len(args) < 2 {
			return nil, errors.New("usage: eng publish TASK --repo OWNER/REPO")
		}
		fs := newFlags("publish", errOut)
		repo := fs.String("repo", "", "GitHub repository matching the configured origin")
		if err := fs.Parse(args[2:]); err != nil || fs.NArg() != 0 {
			return nil, errors.New("usage: eng publish TASK --repo OWNER/REPO")
		}
		publication, err := Publish(ctx, s, args[1], *repo)
		if err == nil && publication.State != "confirmed" {
			err = errors.New(publication.Reason)
		}
		return publication, err
	default:
		return nil, fmt.Errorf("unknown command %q; use eng help", args[0])
	}
}

func newFlags(name string, errOut io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(errOut)
	return fs
}

func runtimeCommand(dataDir string, args []string, errOut io.Writer) (any, error) {
	if len(args) == 1 && args[0] == "list" {
		return registrations(dataDir, "runtimes")
	}
	if len(args) < 2 || args[0] != "add" {
		return nil, errors.New("usage: eng runtime add NAME --model|--no-model [options] -- COMMAND [ARG...]")
	}
	fs := newFlags("runtime add", errOut)
	model := fs.Bool("model", false, "runtime may invoke a paid or remote model")
	noModel := fs.Bool("no-model", false, "runtime is explicitly a deterministic local command")
	stdin := fs.String("stdin", "task", "task or none")
	timeout := fs.String("timeout", "30m", "execution ceiling")
	replace := fs.Bool("replace", false, "replace this local registration")
	if err := fs.Parse(args[2:]); err != nil {
		return nil, err
	}
	if *model == *noModel {
		return nil, errors.New("explicitly choose exactly one of --model and --no-model")
	}
	r := Runtime{Name: args[1], Argv: fs.Args(), Stdin: *stdin, Timeout: *timeout, Model: *model}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	// A project-relative program is resolved in its Git worktree at execution.
	// Registration happens before any project is linked, so checking it against
	// the caller's current directory would reject a valid existing checkout.
	if filepath.IsAbs(r.Argv[0]) || !strings.ContainsRune(r.Argv[0], '/') {
		if _, err := exec.LookPath(r.Argv[0]); err != nil {
			return nil, fmt.Errorf("runtime executable: %w", err)
		}
	}
	if err := saveRegistration(dataDir, "runtimes", r.Name, r, *replace); err != nil {
		return nil, err
	}
	return r, nil
}

func projectCommand(ctx context.Context, dataDir string, args []string, errOut io.Writer) (any, error) {
	if len(args) == 1 && args[0] == "list" {
		return registrations(dataDir, "projects")
	}
	if len(args) < 3 || args[0] != "add" {
		return nil, errors.New("usage: eng project add NAME PATH [--config FILE] [--runtime NAME] [--base REF]")
	}
	name, root := args[1], args[2]
	fs := newFlags("project add", errOut)
	config := fs.String("config", "", "project YAML file")
	runtimeName := fs.String("runtime", "", "registered runtime")
	base := fs.String("base", "", "base branch or exact commit (default: checked-out branch)")
	replace := fs.Bool("replace", false, "replace this local registration")
	if err := fs.Parse(args[3:]); err != nil || fs.NArg() != 0 {
		return nil, errors.New("invalid project add arguments")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	p := Project{Name: name, Root: absRoot, Runtime: *runtimeName}
	if *config == "" {
		candidate := filepath.Join(absRoot, ".eng.yaml")
		if _, statErr := os.Stat(candidate); statErr == nil {
			*config = candidate
		}
	}
	if *config != "" {
		if !filepath.IsAbs(*config) {
			*config = filepath.Join(absRoot, *config)
		}
		// Read once to preserve explicit YAML base/runtime values. A missing
		// base is resolved below, not silently replaced with "main".
		var declared Project
		if err = decodeConfig(*config, &declared); err != nil {
			return nil, err
		}
		p.Base, p.Checks, p.Artifacts = declared.Base, declared.Checks, declared.Artifacts
		if p.Runtime == "" {
			p.Runtime = declared.Runtime
		}
	}
	if *base != "" {
		p.Base = *base
	}
	if *runtimeName != "" {
		p.Runtime = *runtimeName
	}
	if p.Base == "" {
		p.Base, err = checkedOutProjectBase(ctx, absRoot)
		if err != nil {
			return nil, err
		}
	}
	p, err = normalizeProject(p)
	if err != nil {
		return nil, err
	}
	if err = p.Validate(); err != nil {
		return nil, err
	}
	if _, err = InspectRepository(ctx, p.Root, p.Base); err != nil {
		return nil, fmt.Errorf("project base %q is unavailable; select an existing branch or commit with --base (or edit base in .eng.yaml): %w", p.Base, err)
	}
	if _, err = LoadRuntime(filepath.Join(dataDir, "runtimes", p.Runtime+".yaml"), p.Runtime); err != nil {
		return nil, fmt.Errorf("register the runtime first: %w", err)
	}
	if err = saveRegistration(dataDir, "projects", name, p, *replace); err != nil {
		return nil, err
	}
	return p, nil
}

// Resolve the user's existing checkout once at registration, never at task
// execution. Detached HEAD is pinned to its exact commit instead of a
// floating pseudo-ref, so switching worktrees later cannot change the base.
func checkedOutProjectBase(ctx context.Context, root string) (string, error) {
	branch, err := gitOutput(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err == nil && branch != "" {
		return branch, nil
	}
	sha, shaErr := gitOutput(ctx, root, "rev-parse", "--verify", "HEAD")
	if shaErr != nil {
		return "", fmt.Errorf("cannot determine a committed project base; make an initial commit or supply --base: %w", shaErr)
	}
	return sha, nil
}

func saveRegistration(dataDir, kind, name string, value any, replace bool) error {
	if !configName.MatchString(name) {
		return errors.New("invalid registration name")
	}
	dir := filepath.Join(dataDir, kind)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, name+".yaml")
	if !replace {
		if _, err := os.Lstat(path); err == nil {
			return errors.New("registration exists; use --replace to explicitly update future tasks")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if !replace {
		// A hard link is an atomic no-clobber install, unlike a check+rename.
		return os.Link(file.Name(), path)
	}
	return os.Rename(file.Name(), path)
}

func registrations(dataDir, kind string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(dataDir, kind))
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".yaml") {
			names = append(names, strings.TrimSuffix(entry.Name(), ".yaml"))
		}
	}
	return names, nil
}

// Only one input source can be used per attempt. Flag visitation, not string
// emptiness, also rejects an explicitly empty --text on resume.
func instructionFlags(fs *flag.FlagSet) (file, text bool) {
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "task":
			file = true
		case "text":
			text = true
		}
	})
	return file, text
}

func validateInstruction(instruction string) (string, error) {
	if len(instruction) > 1<<20 || strings.TrimSpace(instruction) == "" {
		return "", errors.New("task input must be nonempty and at most 1 MiB")
	}
	return instruction, nil
}

func readInstruction(path string) (string, error) {
	data, err := readRegularFile(path, 1<<20)
	if err != nil {
		return "", err
	}
	return validateInstruction(string(data))
}

func newTask(ctx context.Context, s *Store, project, instruction string) (Task, error) {
	if !configName.MatchString(project) {
		return Task{}, errors.New("invalid project name")
	}
	p, err := LoadProject(filepath.Join(s.DataDir(), "projects", project+".yaml"), project)
	if err != nil {
		return Task{}, err
	}
	r, err := LoadRuntime(filepath.Join(s.DataDir(), "runtimes", p.Runtime+".yaml"), p.Runtime)
	if err != nil {
		return Task{}, err
	}
	snapshot, digest, err := Freeze(p, r)
	if err != nil {
		return Task{}, err
	}
	return s.CreateTask(ctx, Task{Project: p.Name, Owner: currentActor(), Instruction: instruction, Config: snapshot, ConfigDigest: digest})
}

func runQueued(ctx context.Context, s *Store, id string) (Task, error) {
	if ServingCommand(ctx) {
		return s.GetTask(ctx, id)
	}
	lock, err := AcquireScheduler(s.DataDir())
	if err != nil {
		if pingServer(s.DataDir()) == nil {
			return s.GetTask(ctx, id)
		}
		task, readErr := s.GetTask(ctx, id)
		if readErr != nil {
			return task, readErr
		}
		return task, fmt.Errorf("task remains queued; another foreground execution owns the scheduler: %w", err)
	}
	defer lock.Close()
	if _, err = Recover(ctx, s); err != nil {
		return Task{}, err
	}
	task, err := Execute(ctx, s, id)
	if err == nil && task.State != StateDone {
		err = fmt.Errorf("task %s is %s: %s", task.ID, task.State, task.Reason)
	}
	return task, err
}

func currentActor() string {
	if u, err := user.Current(); err == nil {
		return u.Username + " (uid " + u.Uid + ")"
	}
	return fmt.Sprintf("uid %d", os.Getuid())
}

func logCommand(ctx context.Context, s *Store, args []string, out, errOut io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: eng logs TASK [--attempt N] [--stderr] [--tail 200]")
	}
	fs := newFlags("logs", errOut)
	number := fs.Int("attempt", 0, "attempt number, default latest")
	stderr := fs.Bool("stderr", false, "read stderr instead of stdout")
	tail := fs.Int("tail", 200, "last lines (1..10000)")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 || *tail < 1 || *tail > 10000 {
		return errors.New("invalid log arguments")
	}
	attempts, err := s.ListAttempts(ctx, args[0])
	if err != nil {
		return err
	}
	var chosen *Attempt
	for i := range attempts {
		if (*number == 0 && (chosen == nil || attempts[i].Number > chosen.Number)) || attempts[i].Number == *number {
			chosen = &attempts[i]
		}
	}
	if chosen == nil {
		return errors.New("no matching execution attempt")
	}
	name := "runtime.stdout.log"
	if *stderr {
		name = "runtime.stderr.log"
	}
	path := filepath.Join(s.DataDir(), "tasks", chosen.TaskID, fmt.Sprintf("attempt-%d", chosen.Number), name)
	f, err := openRegularFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("log is not a regular file")
	}
	const maxTailBytes = 4 << 20
	start := max(int64(0), info.Size()-maxTailBytes)
	if _, err = f.Seek(start, io.SeekStart); err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(f, maxTailBytes))
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) > *tail {
		lines = lines[len(lines)-*tail:]
	}
	_, err = fmt.Fprintln(out, strings.Join(lines, "\n"))
	return err
}
