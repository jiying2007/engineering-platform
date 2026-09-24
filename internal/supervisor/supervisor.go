package supervisor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	runtimeprovider "github.com/jiying2007/engineering-platform/internal/runtime"
)

var (
	ErrSessionExists = errors.New("supervisor session already exists")
	ErrSessionAbsent = errors.New("supervisor session not found")
	ErrStaleEpoch    = errors.New("stale supervisor execution epoch")
)

type Stream string

const (
	Stdout Stream = "STDOUT"
	Stderr Stream = "STDERR"
	System Stream = "SYSTEM"
)

type Event struct {
	Sequence       uint64    `json:"sequence"`
	RunID          string    `json:"run_id"`
	AttemptID      string    `json:"attempt_id"`
	ExecutionEpoch uint64    `json:"execution_epoch"`
	Stream         Stream    `json:"stream"`
	Data           string    `json:"data"`
	CreatedAt      time.Time `json:"created_at"`
}

type Result struct {
	RunID          string    `json:"run_id"`
	AttemptID      string    `json:"attempt_id"`
	ExecutionEpoch uint64    `json:"execution_epoch"`
	ExitCode       int       `json:"exit_code"`
	Err            string    `json:"error,omitempty"`
	CompletedAt    time.Time `json:"completed_at"`
}

type managed struct {
	runID     string
	attemptID string
	epoch     uint64
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	events    chan Event
	done      chan struct{}

	mu       sync.RWMutex
	sequence uint64
	result   Result
	paused   bool
}

type Supervisor struct {
	provider runtimeprovider.Provider
	now      func() time.Time

	mu       sync.RWMutex
	sessions map[string]*managed
}

func New(provider runtimeprovider.Provider) *Supervisor {
	return &Supervisor{
		provider: provider,
		now:      func() time.Time { return time.Now().UTC() },
		sessions: make(map[string]*managed),
	}
}

func (s *Supervisor) Start(ctx context.Context, runID, attemptID string, epoch uint64, spec runtimeprovider.LaunchSpec) (<-chan Event, error) {
	if s.provider == nil {
		return nil, fmt.Errorf("runtime provider is required")
	}
	if runID == "" || attemptID == "" || epoch == 0 {
		return nil, fmt.Errorf("run_id, attempt_id and execution_epoch are required")
	}

	s.mu.Lock()
	if _, exists := s.sessions[runID]; exists {
		s.mu.Unlock()
		return nil, ErrSessionExists
	}

	cmd, err := s.provider.Command(ctx, spec)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}

	m := &managed{
		runID:     runID,
		attemptID: attemptID,
		epoch:     epoch,
		cmd:       cmd,
		stdin:     stdin,
		events:    make(chan Event, 64),
		done:      make(chan struct{}),
	}
	s.sessions[runID] = m
	s.mu.Unlock()

	if err := cmd.Start(); err != nil {
		s.mu.Lock()
		delete(s.sessions, runID)
		s.mu.Unlock()
		return nil, err
	}

	var readers sync.WaitGroup
	readers.Add(2)
	go s.capture(m, Stdout, stdout, &readers)
	go s.capture(m, Stderr, stderr, &readers)

	go func() {
		// StdoutPipe/StderrPipe require readers to drain before Wait closes the
		// underlying pipes. The child can exit independently; readers observe
		// EOF, then Wait safely reaps the process without losing tail output.
		readers.Wait()
		waitErr := cmd.Wait()

		exitCode := 0
		if waitErr != nil {
			exitCode = -1
			var exitErr *exec.ExitError
			if errors.As(waitErr, &exitErr) {
				exitCode = exitErr.ExitCode()
			}
		}

		m.mu.Lock()
		m.result = Result{
			RunID:          runID,
			AttemptID:      attemptID,
			ExecutionEpoch: epoch,
			ExitCode:       exitCode,
			CompletedAt:    s.now(),
		}
		if waitErr != nil {
			m.result.Err = waitErr.Error()
		}
		m.mu.Unlock()

		s.emit(m, System, "process-exited")
		close(m.events)
		close(m.done)
	}()

	return m.events, nil
}

func (s *Supervisor) capture(m *managed, stream Stream, reader io.Reader, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		s.emit(m, stream, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		s.emit(m, System, streamName(stream)+"-read-error: "+err.Error())
	}
}

func streamName(stream Stream) string {
	if stream == Stdout {
		return "stdout"
	}
	if stream == Stderr {
		return "stderr"
	}
	return "stream"
}

func (s *Supervisor) emit(m *managed, stream Stream, data string) {
	m.mu.Lock()
	m.sequence++
	event := Event{
		Sequence:       m.sequence,
		RunID:          m.runID,
		AttemptID:      m.attemptID,
		ExecutionEpoch: m.epoch,
		Stream:         stream,
		Data:           data,
		CreatedAt:      s.now(),
	}
	m.mu.Unlock()
	m.events <- event
}

func (s *Supervisor) Input(runID string, epoch uint64, data string) error {
	m, err := s.get(runID)
	if err != nil {
		return err
	}
	if err := checkEpoch(m, epoch); err != nil {
		return err
	}
	_, err = io.WriteString(m.stdin, data)
	return err
}

func (s *Supervisor) Pause(runID string, epoch uint64) error {
	m, err := s.get(runID)
	if err != nil {
		return err
	}
	if err := checkEpoch(m, epoch); err != nil {
		return err
	}
	if err := m.cmd.Process.Signal(syscall.SIGSTOP); err != nil {
		return err
	}
	m.mu.Lock()
	m.paused = true
	m.mu.Unlock()
	s.emit(m, System, "process-paused")
	return nil
}

func (s *Supervisor) Resume(runID string, epoch uint64) error {
	m, err := s.get(runID)
	if err != nil {
		return err
	}
	if err := checkEpoch(m, epoch); err != nil {
		return err
	}
	if err := m.cmd.Process.Signal(syscall.SIGCONT); err != nil {
		return err
	}
	m.mu.Lock()
	m.paused = false
	m.mu.Unlock()
	s.emit(m, System, "process-resumed")
	return nil
}

func (s *Supervisor) Abort(runID string, epoch uint64) error {
	m, err := s.get(runID)
	if err != nil {
		return err
	}
	if err := checkEpoch(m, epoch); err != nil {
		return err
	}
	if m.cmd.Process == nil {
		return fmt.Errorf("runtime process is not started")
	}
	return m.cmd.Process.Kill()
}

func (s *Supervisor) Wait(runID string) (Result, error) {
	m, err := s.get(runID)
	if err != nil {
		return Result{}, err
	}
	<-m.done
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.result, nil
}

func (s *Supervisor) PID(runID string) (int, error) {
	m, err := s.get(runID)
	if err != nil {
		return 0, err
	}
	if m.cmd.Process == nil {
		return 0, fmt.Errorf("runtime process is not started")
	}
	return m.cmd.Process.Pid, nil
}

func (s *Supervisor) get(runID string) (*managed, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.sessions[runID]
	if !ok {
		return nil, ErrSessionAbsent
	}
	return m, nil
}

func checkEpoch(m *managed, epoch uint64) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if epoch != m.epoch {
		return ErrStaleEpoch
	}
	return nil
}

// Compile-time guard that this file remains Linux/Unix-process oriented for M1.
// A different worker OS can provide another Supervisor backend later.
var _ os.Signal = syscall.SIGSTOP
