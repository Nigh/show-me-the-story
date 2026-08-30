package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrPiRuntimeUnavailable = errors.New("pi runtime unavailable")

type PiRuntimeOptions struct {
	NodePath   string
	EntryPath  string
	DataDir    string
	SocketPath string
	Logger     *LogBroadcaster
	Command    func(context.Context, string, ...string) *exec.Cmd
	NewClient  func(string) PiRuntime

	ReadinessTimeout  time.Duration
	ReadinessPoll     time.Duration
	ValidationTimeout time.Duration
	ShutdownTimeout   time.Duration
	Now               func() time.Time
	Sleep             func(context.Context, time.Duration) error
}

type PiRuntimeStatus struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Node      string `json:"node_version,omitempty"`
	Pi        string `json:"pi_version,omitempty"`
}

type PiRuntimeSupervisor struct {
	options PiRuntimeOptions
	ctx     context.Context
	cancel  context.CancelFunc

	mu              sync.RWMutex
	status          PiRuntimeStatus
	client          PiRuntime
	child           *piProcessRun
	generation      uint64
	selectedProject string
	closed          bool

	monitorDone chan struct{}
	closeOnce   sync.Once
	closeErr    error
}

type piProcessRun struct {
	command *exec.Cmd
	done    chan struct{}
	err     error
	output  *piBoundedOutput
}

type piNodeVersion struct {
	raw                 string
	major, minor, patch int
}

func (v piNodeVersion) String() string { return v.raw }

func NewPiRuntimeSupervisor(progDir string, overrides PiRuntimeOptions) *PiRuntimeSupervisor {
	options := withPiRuntimeDefaults(progDir, overrides)
	ctx, cancel := context.WithCancel(context.Background())
	supervisor := &PiRuntimeSupervisor{
		options:     options,
		ctx:         ctx,
		cancel:      cancel,
		status:      PiRuntimeStatus{Reason: "starting"},
		monitorDone: make(chan struct{}),
	}

	version, reason := supervisor.validate()
	if reason != "" {
		supervisor.status.Reason = reason
		close(supervisor.monitorDone)
		supervisor.logLifecycle("warn", reason)
		return supervisor
	}
	supervisor.status.Node = version.String()
	supervisor.logLifecycle("info", "starting")
	go supervisor.monitor()
	return supervisor
}

func withPiRuntimeDefaults(progDir string, options PiRuntimeOptions) PiRuntimeOptions {
	if options.NodePath == "" {
		options.NodePath = "node"
	}
	if options.EntryPath == "" {
		options.EntryPath = filepath.Join(progDir, "pi-runtime", "dist", "index.js")
	}
	if options.DataDir == "" {
		options.DataDir = filepath.Join(progDir, "pi-data")
	}
	if options.SocketPath == "" {
		options.SocketPath = filepath.Join(options.DataDir, "run", "pi.sock")
	}
	if options.Command == nil {
		options.Command = exec.CommandContext
	}
	if options.NewClient == nil {
		options.NewClient = NewPiRuntimeClient
	}
	if options.ReadinessTimeout <= 0 {
		options.ReadinessTimeout = 10 * time.Second
	}
	if options.ReadinessPoll <= 0 {
		options.ReadinessPoll = 100 * time.Millisecond
	}
	if options.ValidationTimeout <= 0 {
		options.ValidationTimeout = 3 * time.Second
	}
	if options.ShutdownTimeout <= 0 {
		options.ShutdownTimeout = 5 * time.Second
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Sleep == nil {
		options.Sleep = sleepWithContext
	}
	return options
}

func (s *PiRuntimeSupervisor) validate() (piNodeVersion, string) {
	if runtime.GOOS != "darwin" {
		return piNodeVersion{}, "unsupported_os"
	}
	entry, err := os.Stat(s.options.EntryPath)
	if err != nil || entry.IsDir() {
		return piNodeVersion{}, "entry_missing"
	}

	output := newPiBoundedOutput(32 * 1024)
	ctx, cancel := context.WithTimeout(s.ctx, s.options.ValidationTimeout)
	defer cancel()
	command := s.options.Command(ctx, s.options.NodePath, "--version")
	command.Stdout = output
	command.Stderr = output
	if err := command.Run(); err != nil {
		output.Reset()
		return piNodeVersion{}, "node_unavailable"
	}
	versionText := output.String()
	output.Reset()
	version, err := parseNodeVersion(versionText)
	if err != nil {
		return piNodeVersion{}, "node_version_unsupported"
	}
	return version, ""
}

func parseNodeVersion(value string) (piNodeVersion, error) {
	raw := strings.TrimSpace(value)
	if !strings.HasPrefix(raw, "v") {
		return piNodeVersion{}, errors.New("node version must start with v")
	}
	parts := strings.Split(strings.TrimPrefix(raw, "v"), ".")
	if len(parts) != 3 {
		return piNodeVersion{}, errors.New("node version must contain major, minor, and patch")
	}
	numbers := make([]int, len(parts))
	for index, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return piNodeVersion{}, errors.New("node version is malformed")
		}
		numbers[index] = number
	}
	version := piNodeVersion{raw: raw, major: numbers[0], minor: numbers[1], patch: numbers[2]}
	if version.major < 22 || (version.major == 22 && version.minor < 19) {
		return piNodeVersion{}, errors.New("node version must be at least v22.19.0")
	}
	return version, nil
}

func (s *PiRuntimeSupervisor) monitor() {
	defer close(s.monitorDone)
	restartAttempt := 0
	for {
		if s.ctx.Err() != nil || s.isClosed() {
			return
		}
		s.setUnavailable("starting")
		run, err := s.startChild()
		if err != nil {
			s.setUnavailable("child_start_failed")
			if !s.waitToRestart(&restartAttempt, 0) {
				return
			}
			continue
		}
		client := s.options.NewClient(s.options.SocketPath)
		s.setChild(run)
		health, reason := s.waitUntilReady(run, client)
		if reason != "" {
			s.setUnavailable(reason)
			s.terminateRun(run)
			<-run.done
			run.output.Reset()
			_ = client.Close()
			s.clearChild(run)
			if s.isClosed() || !s.waitToRestart(&restartAttempt, 0) {
				return
			}
			continue
		}

		healthyAt := s.options.Now()
		if !s.activate(run, client, health) {
			s.terminateRun(run)
			<-run.done
			run.output.Reset()
			_ = client.Close()
			return
		}
		<-run.done
		run.output.Reset()
		_ = client.Close()
		s.deactivate(run)
		if s.isClosed() {
			return
		}
		s.logProcessExit(run.err)
		if !s.waitToRestart(&restartAttempt, s.options.Now().Sub(healthyAt)) {
			return
		}
	}
}

func (s *PiRuntimeSupervisor) startChild() (*piProcessRun, error) {
	command := s.options.Command(
		s.ctx,
		s.options.NodePath,
		s.options.EntryPath,
		"--data-dir",
		s.options.DataDir,
		"--socket",
		s.options.SocketPath,
	)
	output := newPiBoundedOutput(32 * 1024)
	command.Stdout = output
	command.Stderr = output
	if err := command.Start(); err != nil {
		output.Reset()
		return nil, err
	}
	run := &piProcessRun{command: command, done: make(chan struct{}), output: output}
	go func() {
		run.err = command.Wait()
		close(run.done)
	}()
	return run, nil
}

func (s *PiRuntimeSupervisor) waitUntilReady(run *piProcessRun, client PiRuntime) (PiHealth, string) {
	ctx, cancel := context.WithTimeout(s.ctx, s.options.ReadinessTimeout)
	defer cancel()
	for {
		select {
		case <-run.done:
			return PiHealth{}, "child_exit"
		default:
		}

		health, err := client.Health(ctx)
		if err == nil && health.Status == "ok" && health.ProtocolVersion == 1 {
			selected := s.selectedProjectName()
			if selected != "" {
				summary, restoreErr := client.SelectProject(ctx, selected)
				if restoreErr != nil || !summary.Restored {
					return PiHealth{}, "project_restore_failed"
				}
			}
			return health, ""
		}
		if err := s.options.Sleep(ctx, s.options.ReadinessPoll); err != nil {
			if s.isClosed() || s.ctx.Err() != nil {
				return PiHealth{}, "stopped"
			}
			return PiHealth{}, "readiness_timeout"
		}
	}
}

func (s *PiRuntimeSupervisor) activate(run *piProcessRun, client PiRuntime, health PiHealth) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.child != run {
		return false
	}
	s.client = client
	s.generation++
	s.status.Available = true
	s.status.Reason = ""
	s.status.Pi = health.PiVersion
	s.logLifecycleLocked("info", "ready")
	return true
}

func (s *PiRuntimeSupervisor) deactivate(run *piProcessRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.child == run {
		s.child = nil
		s.client = nil
		s.generation++
		s.status.Available = false
		s.status.Reason = "child_exit"
		s.status.Pi = ""
	}
}

func (s *PiRuntimeSupervisor) waitToRestart(attempt *int, healthyFor time.Duration) bool {
	delay, next := nextPiRestartDelay(*attempt, healthyFor)
	*attempt = next
	return s.options.Sleep(s.ctx, delay) == nil && !s.isClosed()
}

func nextPiRestartDelay(attempt int, healthyFor time.Duration) (time.Duration, int) {
	delays := [...]time.Duration{250 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second, 5 * time.Second}
	if healthyFor >= 30*time.Second {
		attempt = 0
	}
	index := attempt
	if index >= len(delays) {
		index = len(delays) - 1
	}
	return delays[index], attempt + 1
}

func (s *PiRuntimeSupervisor) terminateRun(run *piProcessRun) {
	select {
	case <-run.done:
		return
	default:
	}
	if run.command.Process != nil {
		_ = run.command.Process.Signal(syscall.SIGTERM)
	}
	timer := time.NewTimer(s.options.ShutdownTimeout)
	defer timer.Stop()
	select {
	case <-run.done:
	case <-timer.C:
		if run.command.Process != nil {
			_ = run.command.Process.Kill()
		}
		<-run.done
	}
}

func (s *PiRuntimeSupervisor) Status() PiRuntimeStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *PiRuntimeSupervisor) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.status.Available = false
		s.status.Reason = "stopped"
		run := s.child
		s.mu.Unlock()

		if run != nil {
			s.terminateRun(run)
		}
		s.cancel()
		<-s.monitorDone
		s.mu.Lock()
		client := s.client
		s.client = nil
		s.child = nil
		s.mu.Unlock()
		if client != nil {
			s.closeErr = client.Close()
		}
		s.logLifecycle("info", "stopped")
	})
	return s.closeErr
}

func (s *PiRuntimeSupervisor) availableClient() (PiRuntime, uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.status.Available || s.client == nil || s.closed {
		return nil, 0, ErrPiRuntimeUnavailable
	}
	return s.client, s.generation, nil
}

func (s *PiRuntimeSupervisor) Health(ctx context.Context) (PiHealth, error) {
	client, _, err := s.availableClient()
	if err != nil {
		return PiHealth{}, err
	}
	return client.Health(ctx)
}

func (s *PiRuntimeSupervisor) Providers(ctx context.Context) ([]PiProvider, error) {
	client, _, err := s.availableClient()
	if err != nil {
		return nil, err
	}
	return client.Providers(ctx)
}

func (s *PiRuntimeSupervisor) Credentials(ctx context.Context) ([]PiCredential, error) {
	client, _, err := s.availableClient()
	if err != nil {
		return nil, err
	}
	return client.Credentials(ctx)
}

func (s *PiRuntimeSupervisor) StartLogin(ctx context.Context, request PiLoginStartRequest) (PiLoginSnapshot, error) {
	client, _, err := s.availableClient()
	if err != nil {
		return PiLoginSnapshot{}, err
	}
	return client.StartLogin(ctx, request)
}

func (s *PiRuntimeSupervisor) LoginStatus(ctx context.Context, id string, after int) (PiLoginSnapshot, error) {
	client, _, err := s.availableClient()
	if err != nil {
		return PiLoginSnapshot{}, err
	}
	return client.LoginStatus(ctx, id, after)
}

func (s *PiRuntimeSupervisor) RespondLogin(ctx context.Context, id string, response PiLoginResponse) error {
	client, _, err := s.availableClient()
	if err != nil {
		return err
	}
	return client.RespondLogin(ctx, id, response)
}

func (s *PiRuntimeSupervisor) CancelLogin(ctx context.Context, id string) error {
	client, _, err := s.availableClient()
	if err != nil {
		return err
	}
	return client.CancelLogin(ctx, id)
}

func (s *PiRuntimeSupervisor) Logout(ctx context.Context, providerID string) error {
	client, _, err := s.availableClient()
	if err != nil {
		return err
	}
	return client.Logout(ctx, providerID)
}

func (s *PiRuntimeSupervisor) ImportLegacy(ctx context.Context, request PiLegacyImportRequest) (PiLegacyImportResult, error) {
	client, _, err := s.availableClient()
	if err != nil {
		return PiLegacyImportResult{}, err
	}
	return client.ImportLegacy(ctx, request)
}

func (s *PiRuntimeSupervisor) SelectProject(ctx context.Context, projectName string) (PiSessionSummary, error) {
	client, generation, err := s.availableClient()
	if err != nil {
		return PiSessionSummary{}, err
	}
	summary, err := client.SelectProject(ctx, projectName)
	if err != nil {
		return PiSessionSummary{}, err
	}
	s.mu.Lock()
	if s.status.Available && !s.closed && s.generation == generation {
		s.selectedProject = projectName
	}
	s.mu.Unlock()
	return summary, nil
}

func (s *PiRuntimeSupervisor) CurrentSession(ctx context.Context) (*PiSessionSummary, error) {
	client, _, err := s.availableClient()
	if err != nil {
		return nil, err
	}
	return client.CurrentSession(ctx)
}

func (s *PiRuntimeSupervisor) selectedProjectName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.selectedProject
}

func (s *PiRuntimeSupervisor) isClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

func (s *PiRuntimeSupervisor) setChild(run *piProcessRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.child = run
	}
}

func (s *PiRuntimeSupervisor) clearChild(run *piProcessRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.child == run {
		s.child = nil
	}
}

func (s *PiRuntimeSupervisor) setUnavailable(reason string) {
	s.mu.Lock()
	s.status.Available = false
	s.status.Reason = reason
	s.status.Pi = ""
	s.logLifecycleLocked("warn", reason)
	s.mu.Unlock()
}

func (s *PiRuntimeSupervisor) logLifecycle(level, reason string) {
	if s.options.Logger != nil {
		s.options.Logger.Log(level, "pi_runtime:"+reason)
	}
}

func (s *PiRuntimeSupervisor) logLifecycleLocked(level, reason string) {
	if s.options.Logger != nil {
		s.options.Logger.Log(level, "pi_runtime:"+reason)
	}
}

func (s *PiRuntimeSupervisor) logProcessExit(err error) {
	if s.options.Logger == nil {
		return
	}
	detail := "unknown"
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		detail = fmt.Sprintf("code=%d", exitErr.ExitCode())
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			detail = "signal=" + status.Signal().String()
		}
	} else if err == nil {
		detail = "code=0"
	}
	s.options.Logger.Log("warn", "pi_runtime:child_exit "+detail)
}

func sleepWithContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type piBoundedOutput struct {
	mu       sync.Mutex
	contents []byte
	limit    int
}

func newPiBoundedOutput(limit int) *piBoundedOutput {
	return &piBoundedOutput{limit: limit}
}

func (output *piBoundedOutput) Write(value []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	written := len(value)
	if output.limit <= 0 {
		return written, nil
	}
	if len(value) >= output.limit {
		output.contents = append(output.contents[:0], value[len(value)-output.limit:]...)
		return written, nil
	}
	overflow := len(output.contents) + len(value) - output.limit
	if overflow > 0 {
		copy(output.contents, output.contents[overflow:])
		output.contents = output.contents[:len(output.contents)-overflow]
	}
	output.contents = append(output.contents, value...)
	return written, nil
}

func (output *piBoundedOutput) String() string {
	output.mu.Lock()
	defer output.mu.Unlock()
	return string(output.contents)
}

func (output *piBoundedOutput) Len() int {
	output.mu.Lock()
	defer output.mu.Unlock()
	return len(output.contents)
}

func (output *piBoundedOutput) Reset() {
	output.mu.Lock()
	defer output.mu.Unlock()
	for index := range output.contents {
		output.contents[index] = 0
	}
	output.contents = nil
}
