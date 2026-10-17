package upgrade

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

type instantRunner struct{}

func (instantRunner) Run(ctx context.Context) (Result, error) {
	return Result{Steps: []Step{{Name: "go_test", Command: "go test ./...", Output: "ok"}}}, nil
}

func TestManagerPersistsStatus(t *testing.T) {
	statusFile := filepath.Join(t.TempDir(), "upgrade-status.json")
	manager := NewManager(instantRunner{}, WithStatusFile(statusFile))
	if _, started, err := manager.Start(context.Background()); err != nil || !started {
		t.Fatalf("start started=%v err=%v", started, err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if manager.Status().Status == "succeeded" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if manager.Status().Status != "succeeded" {
		t.Fatalf("status = %q, want succeeded", manager.Status().Status)
	}
	reloaded := NewManager(nil, WithStatusFile(statusFile))
	if got := reloaded.Status(); got.Status != "succeeded" || len(got.Steps) != 1 {
		t.Fatalf("reloaded status = %#v", got)
	}
}

func TestNewShellRunnerDefaultsToSkippingServerTests(t *testing.T) {
	t.Setenv("TV_OKX_WORKDIR", t.TempDir())
	t.Setenv("TV_OKX_UPGRADE_RUN_TESTS", "")

	runner, err := NewShellRunnerFromEnv()
	if err != nil {
		t.Fatalf("NewShellRunnerFromEnv() error = %v", err)
	}
	if runner.RunTests {
		t.Fatal("RunTests = true, want false by default")
	}
	if runner.BuildBinary {
		t.Fatal("BuildBinary = true, want false by default")
	}

	for _, value := range []string{"1", "true", "yes", "on"} {
		t.Setenv("TV_OKX_UPGRADE_RUN_TESTS", value)
		runner, err := NewShellRunnerFromEnv()
		if err != nil {
			t.Fatalf("NewShellRunnerFromEnv(%q) error = %v", value, err)
		}
		if !runner.RunTests {
			t.Fatalf("RunTests = false for %q, want true", value)
		}
	}

	t.Setenv("TV_OKX_UPGRADE_BUILD", "true")
	runner, err = NewShellRunnerFromEnv()
	if err != nil {
		t.Fatalf("NewShellRunnerFromEnv(build) error = %v", err)
	}
	if !runner.BuildBinary {
		t.Fatal("BuildBinary = false for true, want true")
	}
}

func TestUpgradeTimeout(t *testing.T) {
	t.Setenv("TV_OKX_UPGRADE_TIMEOUT", "")
	if got := upgradeTimeout(); got != defaultUpgradeTimeout {
		t.Fatalf("default upgrade timeout = %s, want %s", got, defaultUpgradeTimeout)
	}

	t.Setenv("TV_OKX_UPGRADE_TIMEOUT", "17m")
	if got := upgradeTimeout(); got != 17*time.Minute {
		t.Fatalf("configured upgrade timeout = %s, want 17m", got)
	}

	for _, value := range []string{"invalid", "0", "-1s"} {
		t.Setenv("TV_OKX_UPGRADE_TIMEOUT", value)
		if got := upgradeTimeout(); got != defaultUpgradeTimeout {
			t.Fatalf("invalid timeout %q = %s, want %s", value, got, defaultUpgradeTimeout)
		}
	}
}

func TestEnvBoolFallback(t *testing.T) {
	const name = "TV_OKX_TEST_ENV_BOOL"
	for _, tc := range []struct {
		value    string
		fallback bool
		want     bool
	}{
		{"", true, true},
		{"", false, false},
		{"true", false, true},
		{"0", true, false},
		{"unknown", true, true},
	} {
		t.Setenv(name, tc.value)
		if got := envBool(name, tc.fallback); got != tc.want {
			t.Fatalf("envBool(%q, %v) = %v, want %v", tc.value, tc.fallback, got, tc.want)
		}
	}
}
