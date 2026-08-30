package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseNodeVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value string
		ok    bool
	}{
		{"v22.19.0", true},
		{"v22.20.1\n", true},
		{"v23.0.0", true},
		{"v22.18.9", false},
		{"22.19", false},
		{"not-node", false},
	}
	for _, tc := range tests {
		t.Run(strings.TrimSpace(tc.value), func(t *testing.T) {
			version, err := parseNodeVersion(tc.value)
			if tc.ok && (err != nil || version.String() != strings.TrimSpace(tc.value)) {
				t.Fatalf("parseNodeVersion(%q) = %v, %v", tc.value, version, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("parseNodeVersion(%q) unexpectedly succeeded", tc.value)
			}
		})
	}
}

func TestPiRuntimeSupervisorStartsForwardsAndStopsOwnedChild(t *testing.T) {
	t.Parallel()
	root, entry, dataDir, socketPath := piSupervisorPaths(t)
	marker := filepath.Join(root, "terminated")
	readyMarker := filepath.Join(root, "ready")
	var commandMu sync.Mutex
	var runtimeName string
	var runtimeArgs []string
	fake := &fakePiRuntime{health: func(context.Context) (PiHealth, error) {
		if _, err := os.Stat(readyMarker); err != nil {
			return PiHealth{}, errors.New("not ready")
		}
		return PiHealth{Status: "ok", ProtocolVersion: 1, PiVersion: "0.83.0"}, nil
	}}
	logger := NewLogBroadcaster()
	logs := logger.Subscribe()
	defer logger.Close()

	supervisor := NewPiRuntimeSupervisor(root, PiRuntimeOptions{
		NodePath:   "fixture-node",
		EntryPath:  entry,
		DataDir:    dataDir,
		SocketPath: socketPath,
		Logger:     logger,
		Command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			if len(args) == 1 && args[0] == "--version" {
				return exec.CommandContext(ctx, "/bin/echo", "v22.19.0")
			}
			commandMu.Lock()
			runtimeName = name
			runtimeArgs = append([]string(nil), args...)
			commandMu.Unlock()
			script := `trap 'printf term > "$0"; exit 0' TERM INT; printf ready > "$1"; printf child-private-token; while :; do sleep 0.02; done`
			return exec.CommandContext(ctx, "/bin/sh", "-c", script, marker, readyMarker)
		},
		NewClient:        func(string) PiRuntime { return fake },
		ReadinessPoll:    time.Millisecond,
		ReadinessTimeout: time.Second,
		ShutdownTimeout:  time.Second,
	})

	eventually(t, time.Second, func() bool { return supervisor.Status().Available })
	status := supervisor.Status()
	if status.Node != "v22.19.0" || status.Pi != "0.83.0" {
		t.Fatalf("Status() = %#v", status)
	}
	providers, err := supervisor.Providers(context.Background())
	if err != nil || len(providers) != 1 || providers[0].ID != "fixture" {
		t.Fatalf("Providers() = %#v, %v", providers, err)
	}
	commandMu.Lock()
	if runtimeName != "fixture-node" {
		t.Errorf("runtime command = %q, want fixture-node", runtimeName)
	}
	wantArgs := []string{entry, "--data-dir", dataDir, "--socket", socketPath}
	if !reflect.DeepEqual(runtimeArgs, wantArgs) {
		t.Errorf("runtime args = %#v, want %#v", runtimeArgs, wantArgs)
	}
	commandMu.Unlock()

	if err := supervisor.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}
	eventually(t, time.Second, func() bool {
		content, err := os.ReadFile(marker)
		return err == nil && string(content) == "term"
	})
	logger.Unsubscribe(logs)
	for message := range logs {
		if strings.Contains(fmt.Sprint(message.Data), "child-private-token") {
			t.Fatalf("child output leaked to logger: %#v", message)
		}
	}
}

func TestPiRuntimeSupervisorRestartsWithBackoffAndRestoresProject(t *testing.T) {
	t.Parallel()
	root, entry, dataDir, socketPath := piSupervisorPaths(t)
	var commandMu sync.Mutex
	runtimeStarts := 0
	var clientMu sync.Mutex
	clients := 0
	replayed := make(chan int, 16)
	delays := make(chan time.Duration, 16)

	supervisor := NewPiRuntimeSupervisor(root, PiRuntimeOptions{
		NodePath:   "fixture-node",
		EntryPath:  entry,
		DataDir:    dataDir,
		SocketPath: socketPath,
		Command: func(ctx context.Context, _ string, args ...string) *exec.Cmd {
			if len(args) == 1 && args[0] == "--version" {
				return exec.CommandContext(ctx, "/bin/echo", "v23.0.0")
			}
			commandMu.Lock()
			runtimeStarts++
			start := runtimeStarts
			commandMu.Unlock()
			delay := "0.015"
			if start == 1 {
				delay = "0.15"
			}
			return exec.CommandContext(ctx, "/bin/sh", "-c", "sleep "+delay+"; exit 7")
		},
		NewClient: func(string) PiRuntime {
			clientMu.Lock()
			index := clients
			clients++
			clientMu.Unlock()
			return &fakePiRuntime{
				health: func(context.Context) (PiHealth, error) {
					return PiHealth{Status: "ok", ProtocolVersion: 1, PiVersion: "0.83.0"}, nil
				},
				selectProject: func(_ context.Context, name string) (PiSessionSummary, error) {
					if index > 0 {
						replayed <- index
					}
					return PiSessionSummary{ProjectID: "project-1", SessionID: "session-1", Restored: index > 0}, nil
				},
			}
		},
		Sleep: func(ctx context.Context, delay time.Duration) error {
			delays <- delay
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return nil
			}
		},
		ReadinessPoll:    time.Millisecond,
		ReadinessTimeout: time.Second,
		ShutdownTimeout:  100 * time.Millisecond,
	})
	defer supervisor.Close()

	eventually(t, time.Second, func() bool { return supervisor.Status().Available })
	selected, err := supervisor.SelectProject(context.Background(), "Story One")
	if err != nil || selected.Restored {
		t.Fatalf("SelectProject() = %#v, %v", selected, err)
	}

	select {
	case <-replayed:
	case <-time.After(time.Second):
		t.Fatal("project was not replayed after restart")
	}
	wantDelays := []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second, 5 * time.Second}
	gotDelays := make([]time.Duration, 0, len(wantDelays))
	for len(gotDelays) < len(wantDelays) {
		select {
		case delay := <-delays:
			gotDelays = append(gotDelays, delay)
		case <-time.After(2 * time.Second):
			t.Fatalf("restart delays = %v, want %v", gotDelays, wantDelays)
		}
	}
	if !reflect.DeepEqual(gotDelays, wantDelays) {
		t.Fatalf("restart delays = %v, want %v", gotDelays, wantDelays)
	}
}

func TestPiRuntimeSupervisorRestartPolicyResetsAfterHealthyWindow(t *testing.T) {
	t.Parallel()
	delay, next := nextPiRestartDelay(4, 29*time.Second)
	if delay != 5*time.Second || next != 5 {
		t.Fatalf("nextPiRestartDelay before reset = %v, %d", delay, next)
	}
	delay, next = nextPiRestartDelay(4, 30*time.Second)
	if delay != 250*time.Millisecond || next != 1 {
		t.Fatalf("nextPiRestartDelay after reset = %v, %d", delay, next)
	}
}

func TestPiRuntimeSupervisorDegradesWithoutNodeEntryOrReadiness(t *testing.T) {
	t.Parallel()
	t.Run("missing entry", func(t *testing.T) {
		root := t.TempDir()
		supervisor := NewPiRuntimeSupervisor(root, PiRuntimeOptions{EntryPath: filepath.Join(root, "missing.js")})
		defer supervisor.Close()
		if status := supervisor.Status(); status.Available || status.Reason != "entry_missing" {
			t.Fatalf("Status() = %#v", status)
		}
		if _, err := supervisor.Providers(context.Background()); !errors.Is(err, ErrPiRuntimeUnavailable) {
			t.Fatalf("Providers() error = %v, want ErrPiRuntimeUnavailable", err)
		}
	})

	t.Run("missing node", func(t *testing.T) {
		root, entry, _, _ := piSupervisorPaths(t)
		supervisor := NewPiRuntimeSupervisor(root, PiRuntimeOptions{
			EntryPath: entry,
			Command: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				return exec.CommandContext(ctx, filepath.Join(root, "missing-node"))
			},
		})
		defer supervisor.Close()
		if status := supervisor.Status(); status.Available || status.Reason != "node_unavailable" {
			t.Fatalf("Status() = %#v", status)
		}
	})

	t.Run("hung node version", func(t *testing.T) {
		root, entry, _, _ := piSupervisorPaths(t)
		started := time.Now()
		supervisor := NewPiRuntimeSupervisor(root, PiRuntimeOptions{
			EntryPath:         entry,
			ValidationTimeout: 20 * time.Millisecond,
			Command: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				return exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 10")
			},
		})
		defer supervisor.Close()
		if elapsed := time.Since(started); elapsed > time.Second {
			t.Fatalf("constructor blocked for %v", elapsed)
		}
		if status := supervisor.Status(); status.Available || status.Reason != "node_unavailable" {
			t.Fatalf("Status() = %#v", status)
		}
	})

	t.Run("readiness timeout", func(t *testing.T) {
		root, entry, dataDir, socketPath := piSupervisorPaths(t)
		supervisor := NewPiRuntimeSupervisor(root, PiRuntimeOptions{
			EntryPath:  entry,
			DataDir:    dataDir,
			SocketPath: socketPath,
			Command: func(ctx context.Context, _ string, args ...string) *exec.Cmd {
				if len(args) == 1 && args[0] == "--version" {
					return exec.CommandContext(ctx, "/bin/echo", "v22.20.1")
				}
				return exec.CommandContext(ctx, "/bin/sh", "-c", "while :; do sleep 0.1; done")
			},
			NewClient: func(string) PiRuntime {
				return &fakePiRuntime{health: func(context.Context) (PiHealth, error) {
					return PiHealth{}, errors.New("not ready")
				}}
			},
			Sleep: func(ctx context.Context, _ time.Duration) error {
				<-ctx.Done()
				return ctx.Err()
			},
			ReadinessPoll:    time.Millisecond,
			ReadinessTimeout: 20 * time.Millisecond,
			ShutdownTimeout:  100 * time.Millisecond,
		})
		defer supervisor.Close()
		eventually(t, time.Second, func() bool { return supervisor.Status().Reason == "readiness_timeout" })
	})
}

func TestPiRuntimeSupervisorOutputRingIsBounded(t *testing.T) {
	t.Parallel()
	output := newPiBoundedOutput(32 * 1024)
	_, _ = output.Write([]byte(strings.Repeat("secret", 20_000)))
	if output.Len() != 32*1024 {
		t.Fatalf("bounded output length = %d", output.Len())
	}
	output.Reset()
	if output.Len() != 0 {
		t.Fatalf("bounded output was not discarded")
	}
}

func piSupervisorPaths(t *testing.T) (root, entry, dataDir, socketPath string) {
	t.Helper()
	root = t.TempDir()
	entry = filepath.Join(root, "pi-runtime", "dist", "index.js")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("// fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	dataDir = filepath.Join(root, "pi-data")
	socketPath = filepath.Join(dataDir, "run", "pi.sock")
	return
}

func eventually(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition was not met before timeout")
		}
		time.Sleep(time.Millisecond)
	}
}

type fakePiRuntime struct {
	health        func(context.Context) (PiHealth, error)
	selectProject func(context.Context, string) (PiSessionSummary, error)
}

func (f *fakePiRuntime) Health(ctx context.Context) (PiHealth, error) {
	if f.health != nil {
		return f.health(ctx)
	}
	return PiHealth{Status: "ok", ProtocolVersion: 1, PiVersion: "0.83.0"}, nil
}

func (f *fakePiRuntime) Providers(context.Context) ([]PiProvider, error) {
	return []PiProvider{{ID: "fixture"}}, nil
}
func (f *fakePiRuntime) Credentials(context.Context) ([]PiCredential, error) { return nil, nil }
func (f *fakePiRuntime) StartLogin(context.Context, PiLoginStartRequest) (PiLoginSnapshot, error) {
	return PiLoginSnapshot{}, nil
}
func (f *fakePiRuntime) LoginStatus(context.Context, string, int) (PiLoginSnapshot, error) {
	return PiLoginSnapshot{}, nil
}
func (f *fakePiRuntime) RespondLogin(context.Context, string, PiLoginResponse) error { return nil }
func (f *fakePiRuntime) CancelLogin(context.Context, string) error                   { return nil }
func (f *fakePiRuntime) Logout(context.Context, string) error                        { return nil }
func (f *fakePiRuntime) ImportLegacy(context.Context, PiLegacyImportRequest) (PiLegacyImportResult, error) {
	return PiLegacyImportResult{}, nil
}
func (f *fakePiRuntime) SelectProject(ctx context.Context, name string) (PiSessionSummary, error) {
	if f.selectProject != nil {
		return f.selectProject(ctx, name)
	}
	return PiSessionSummary{}, nil
}
func (f *fakePiRuntime) CurrentSession(context.Context) (*PiSessionSummary, error) { return nil, nil }
func (f *fakePiRuntime) Close() error                                              { return nil }
