package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Runner interface {
	Run(ctx context.Context) (Result, error)
}

type Result struct {
	RunID      string    `json:"run_id"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Steps      []Step    `json:"steps"`
	Error      string    `json:"error,omitempty"`
}

type Step struct {
	Name     string    `json:"name"`
	Command  string    `json:"command"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished,omitempty"`
	Output   string    `json:"output,omitempty"`
	Error    string    `json:"error,omitempty"`
}

type Manager struct {
	mu         sync.Mutex
	running    bool
	current    Result
	runner     Runner
	statusFile string
}

type Option func(*Manager)

func WithStatusFile(path string) Option {
	return func(m *Manager) {
		m.statusFile = path
	}
}

func NewManager(runner Runner, opts ...Option) *Manager {
	m := &Manager{runner: runner}
	for _, opt := range opts {
		opt(m)
	}
	_ = m.loadStatus()
	return m
}

func (m *Manager) Start(ctx context.Context) (Result, bool, error) {
	m.mu.Lock()
	if m.runner == nil {
		m.mu.Unlock()
		return Result{}, false, errors.New("upgrade runner is not configured")
	}
	if m.running {
		current := m.current
		m.mu.Unlock()
		return current, false, nil
	}
	runID := time.Now().UTC().Format("20060102T150405.000")
	m.running = true
	m.current = Result{RunID: runID, Status: "running", StartedAt: time.Now().UTC()}
	current := m.current
	_ = m.saveStatusLocked()
	m.mu.Unlock()

	go func() {
		runCtx, cancel := context.WithTimeout(ctx, upgradeTimeout())
		defer cancel()
		result, err := m.runner.Run(runCtx)
		m.mu.Lock()
		defer m.mu.Unlock()
		result.RunID = runID
		result.StartedAt = current.StartedAt
		result.FinishedAt = time.Now().UTC()
		if err != nil {
			result.Status = "failed"
			result.Error = err.Error()
		} else {
			result.Status = "succeeded"
		}
		m.current = result
		_ = m.saveStatusLocked()
		m.running = false
	}()

	return current, true, nil
}

func (m *Manager) Status() Result {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

func (m *Manager) loadStatus() error {
	if m.statusFile == "" {
		return nil
	}
	b, err := os.ReadFile(m.statusFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var result Result
	if err := json.Unmarshal(b, &result); err != nil {
		return err
	}
	m.current = result
	return nil
}

func (m *Manager) saveStatusLocked() error {
	if m.statusFile == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(m.statusFile), 0o755); err != nil && filepath.Dir(m.statusFile) != "." {
		return err
	}
	b, err := json.MarshalIndent(m.current, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.statusFile + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.statusFile)
}

type ShellRunner struct {
	WorkDir        string
	BinaryPath     string
	ServiceName    string
	RestartCommand string
	RunTests       bool
	BuildBinary    bool
}

const defaultUpgradeTimeout = 45 * time.Minute

func NewShellRunnerFromEnv() (ShellRunner, error) {
	wd := os.Getenv("TV_OKX_WORKDIR")
	if wd == "" {
		var err error
		wd, err = os.Getwd()
		if err != nil {
			return ShellRunner{}, err
		}
	}
	binary := os.Getenv("TV_OKX_BINARY")
	if binary == "" {
		name := "tv-okx-bot"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		binary = filepath.Join(wd, name)
	}
	service := os.Getenv("TV_OKX_SERVICE")
	if service == "" {
		service = "tv-okx-bot.service"
	}
	return ShellRunner{
		WorkDir:        wd,
		BinaryPath:     binary,
		ServiceName:    service,
		RestartCommand: os.Getenv("TV_OKX_RESTART_CMD"),
		RunTests:       envBool("TV_OKX_UPGRADE_RUN_TESTS", false),
		BuildBinary:    envBool("TV_OKX_UPGRADE_BUILD", false),
	}, nil
}

func (r ShellRunner) Run(ctx context.Context) (Result, error) {
	if r.WorkDir == "" {
		return Result{}, errors.New("upgrade workdir is required")
	}
	if r.BinaryPath == "" {
		return Result{}, errors.New("upgrade binary path is required")
	}
	result := Result{Status: "running", StartedAt: time.Now().UTC()}
	if err := r.runStep(ctx, &result, "git_pull", "git", "pull", "--ff-only"); err != nil {
		return result, err
	}
	if r.RunTests {
		if err := r.runStep(ctx, &result, "go_test", "go", "test", "./..."); err != nil {
			return result, err
		}
	} else {
		result.Steps = append(result.Steps, Step{
			Name:     "go_test",
			Command:  "skipped (TV_OKX_UPGRADE_RUN_TESTS=false)",
			Started:  time.Now().UTC(),
			Finished: time.Now().UTC(),
			Output:   "server-side test suite skipped; run go test ./... before publishing",
		})
	}
	tmpBinary := r.BinaryPath + ".new"
	if r.BuildBinary {
		if err := r.runStep(ctx, &result, "go_build", "go", "build", "-o", tmpBinary, "./cmd/tv-okx-bot"); err != nil {
			return result, err
		}
	} else {
		step := Step{
			Name:     "go_build",
			Command:  "skipped (TV_OKX_UPGRADE_BUILD=false)",
			Started:  time.Now().UTC(),
			Finished: time.Now().UTC(),
		}
		info, err := os.Stat(tmpBinary)
		if err != nil {
			step.Error = fmt.Sprintf("prebuilt binary %s is required: %v", tmpBinary, err)
			result.Steps = append(result.Steps, step)
			return result, errors.New(step.Error)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			step.Error = fmt.Sprintf("prebuilt binary %s is empty or not a regular file", tmpBinary)
			result.Steps = append(result.Steps, step)
			return result, errors.New(step.Error)
		}
		step.Output = fmt.Sprintf("using prebuilt binary %s (%d bytes)", tmpBinary, info.Size())
		result.Steps = append(result.Steps, step)
	}
	if err := os.Rename(tmpBinary, r.BinaryPath); err != nil {
		step := Step{Name: "replace_binary", Command: "rename " + tmpBinary + " " + r.BinaryPath, Started: time.Now().UTC(), Finished: time.Now().UTC(), Error: err.Error()}
		result.Steps = append(result.Steps, step)
		return result, err
	}
	result.Steps = append(result.Steps, Step{Name: "replace_binary", Command: "rename " + tmpBinary + " " + r.BinaryPath, Started: time.Now().UTC(), Finished: time.Now().UTC(), Output: "binary replaced"})

	if cmd := strings.TrimSpace(r.RestartCommand); cmd != "" {
		if err := r.scheduleRestart(ctx, &result, cmd); err != nil {
			return result, err
		}
	} else if runtime.GOOS == "linux" {
		cmd := "/usr/bin/sudo /bin/systemctl restart " + r.ServiceName
		if err := r.scheduleRestart(ctx, &result, cmd); err != nil {
			return result, err
		}
	} else {
		result.Steps = append(result.Steps, Step{Name: "restart_service", Command: "skipped", Started: time.Now().UTC(), Finished: time.Now().UTC(), Output: "restart is only automatic on linux or when TV_OKX_RESTART_CMD is set"})
	}
	return result, nil
}

func (r ShellRunner) runStep(ctx context.Context, result *Result, name string, command string, args ...string) error {
	step := Step{Name: name, Command: commandLine(command, args...), Started: time.Now().UTC()}
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = r.WorkDir
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	step.Finished = time.Now().UTC()
	step.Output = truncate(output.String(), 12000)
	if err != nil {
		step.Error = err.Error()
		result.Steps = append(result.Steps, step)
		return fmt.Errorf("%s failed: %w", name, err)
	}
	result.Steps = append(result.Steps, step)
	return nil
}

func (r ShellRunner) runShellStep(ctx context.Context, result *Result, name string, script string) error {
	if runtime.GOOS == "windows" {
		return r.runStep(ctx, result, name, "powershell", "-NoProfile", "-Command", script)
	}
	return r.runStep(ctx, result, name, "/bin/sh", "-c", script)
}

func (r ShellRunner) scheduleRestart(ctx context.Context, result *Result, command string) error {
	if runtime.GOOS == "windows" {
		return r.runShellStep(ctx, result, "restart_service", "Start-Job -ScriptBlock { Start-Sleep -Seconds 1; "+command+" } | Out-Null")
	}
	script := fmt.Sprintf("(sleep 1; %s) >/tmp/tv-okx-bot-restart.log 2>&1 &", command)
	return r.runStep(ctx, result, "restart_service", "/bin/sh", "-c", script)
}

func commandLine(command string, args ...string) string {
	parts := append([]string{command}, args...)
	return strings.Join(parts, " ")
}

func truncate(v string, limit int) string {
	v = strings.TrimSpace(v)
	if len(v) <= limit {
		return v
	}
	return v[:limit] + "\n... truncated ..."
}

func envBool(name string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func upgradeTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("TV_OKX_UPGRADE_TIMEOUT"))
	if raw == "" {
		return defaultUpgradeTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultUpgradeTimeout
	}
	return d
}
