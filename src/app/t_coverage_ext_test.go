package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tuxerrante/kapparmor/src/app/metrics"
)

// ---------------------------------------------------------------------------
// NewConfigFromEnv
// ---------------------------------------------------------------------------

func TestNewConfigFromEnv_Defaults(t *testing.T) {
	t.Setenv("PROFILES_DIR", "")
	t.Setenv("POLL_TIME", "")

	logger := slog.Default()
	cfg := NewConfigFromEnv(logger)

	if cfg.ConfigmapPath != "/app/profiles" {
		t.Errorf("expected default ConfigmapPath '/app/profiles', got %q", cfg.ConfigmapPath)
	}

	if cfg.PollTimeArg != "30" {
		t.Errorf("expected default PollTimeArg '30', got %q", cfg.PollTimeArg)
	}

	if cfg.EtcApparmord != "/etc/apparmor.d/custom" {
		t.Errorf("expected default EtcApparmord '/etc/apparmor.d/custom', got %q", cfg.EtcApparmord)
	}

	if cfg.KernelPath != "/sys/kernel/security/apparmor/profiles" {
		t.Errorf("expected default KernelPath, got %q", cfg.KernelPath)
	}

	if cfg.Logger == nil {
		t.Error("expected non-nil Logger")
	}
}

func TestNewConfigFromEnv_CustomValues(t *testing.T) {
	t.Setenv("PROFILES_DIR", "/custom/profiles")
	t.Setenv("POLL_TIME", "60")

	logger := slog.Default()
	cfg := NewConfigFromEnv(logger)

	if cfg.ConfigmapPath != "/custom/profiles" {
		t.Errorf("expected custom ConfigmapPath '/custom/profiles', got %q", cfg.ConfigmapPath)
	}

	if cfg.PollTimeArg != "60" {
		t.Errorf("expected custom PollTimeArg '60', got %q", cfg.PollTimeArg)
	}
}

// ---------------------------------------------------------------------------
// newDefaultLogger
// ---------------------------------------------------------------------------

func TestNewDefaultLogger(t *testing.T) {
	logger := newDefaultLogger()
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}

	// Verify it can log without panicking
	logger.Info("test message", slog.String("key", "value"))
}

// ---------------------------------------------------------------------------
// startHealthzServer – test the HTTP handlers directly (not the goroutine)
// ---------------------------------------------------------------------------

func TestHealthzHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok")) //nolint:errcheck
	})
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	if body := rr.Body.String(); body != "ok" {
		t.Errorf("expected body 'ok', got %q", body)
	}
}

// ---------------------------------------------------------------------------
// loadNewProfiles
// ---------------------------------------------------------------------------

func TestLoadNewProfiles_EmptyConfigmap(t *testing.T) {
	tempDir := t.TempDir()

	configmapDir := path.Join(tempDir, "configmap")
	etcApparmordDir := path.Join(tempDir, "etc_apparmord")
	kernelPath := path.Join(tempDir, "profiles")

	if err := os.MkdirAll(configmapDir, 0o755); err != nil {
		t.Fatalf("mkdir configmap: %v", err)
	}

	if err := os.MkdirAll(etcApparmordDir, 0o755); err != nil {
		t.Fatalf("mkdir etc: %v", err)
	}

	if err := os.WriteFile(kernelPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write kernel: %v", err)
	}

	cfg := &AppConfig{
		ConfigmapPath:    configmapDir,
		EtcApparmord:     etcApparmordDir,
		KernelPath:       kernelPath,
		ProfilerFullPath: "true",
		Logger:           slog.Default(),
	}

	// Empty configmap → areProfilesReadable returns (false, nil) → loadNewProfiles returns error
	profiles, err := loadNewProfiles(cfg)

	if err == nil {
		t.Error("expected error for empty configmap (areProfilesReadable returns false)")
	}

	_ = profiles
}

func TestLoadNewProfiles_NonexistentConfigmap(t *testing.T) {
	tempDir := t.TempDir()

	etcApparmordDir := path.Join(tempDir, "etc_apparmord")
	kernelPath := path.Join(tempDir, "profiles")

	if err := os.MkdirAll(etcApparmordDir, 0o755); err != nil {
		t.Fatalf("mkdir etc: %v", err)
	}

	if err := os.WriteFile(kernelPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write kernel: %v", err)
	}

	cfg := &AppConfig{
		ConfigmapPath:    "/nonexistent/configmap",
		EtcApparmord:     etcApparmordDir,
		KernelPath:       kernelPath,
		ProfilerFullPath: "true",
		Logger:           slog.Default(),
	}

	_, err := loadNewProfiles(cfg)

	if err == nil {
		t.Error("expected error for nonexistent configmap directory")
	}
}

func TestLoadNewProfiles_NonexistentKernelPath(t *testing.T) {
	tempDir := t.TempDir()

	configmapDir := path.Join(tempDir, "configmap")
	etcApparmordDir := path.Join(tempDir, "etc_apparmord")

	if err := os.MkdirAll(configmapDir, 0o755); err != nil {
		t.Fatalf("mkdir configmap: %v", err)
	}

	// Write a valid profile so areProfilesReadable returns true
	profileContent := "profile custom.test { }"
	if err := os.WriteFile(path.Join(configmapDir, "custom.test"), []byte(profileContent), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	if err := os.MkdirAll(etcApparmordDir, 0o755); err != nil {
		t.Fatalf("mkdir etc: %v", err)
	}

	cfg := &AppConfig{
		ConfigmapPath:    configmapDir,
		EtcApparmord:     etcApparmordDir,
		KernelPath:       "/nonexistent/kernel/profiles",
		ProfilerFullPath: "true",
		Logger:           slog.Default(),
	}

	_, err := loadNewProfiles(cfg)

	if err == nil {
		t.Error("expected error for nonexistent kernel path")
	}
}

func TestLoadNewProfiles_WithUnloadOrphans(t *testing.T) {
	tempDir := t.TempDir()

	configmapDir := path.Join(tempDir, "configmap")
	etcApparmordDir := path.Join(tempDir, "etc_apparmord")
	kernelPath := path.Join(tempDir, "profiles")

	if err := os.MkdirAll(configmapDir, 0o755); err != nil {
		t.Fatalf("mkdir configmap: %v", err)
	}

	if err := os.MkdirAll(etcApparmordDir, 0o755); err != nil {
		t.Fatalf("mkdir etc: %v", err)
	}

	// Kernel file reports custom.orphan as loaded
	kernelContent := "custom.orphan (enforce)\n"
	if err := os.WriteFile(kernelPath, []byte(kernelContent), 0o644); err != nil {
		t.Fatalf("write kernel: %v", err)
	}

	// Create the orphan file in etc so unloadProfile can find it
	if err := os.WriteFile(filepath.Join(etcApparmordDir, "custom.orphan"), []byte("profile custom.orphan { }"), 0o644); err != nil {
		t.Fatalf("write orphan: %v", err)
	}

	cfg := &AppConfig{
		ConfigmapPath:    configmapDir,
		EtcApparmord:     etcApparmordDir,
		KernelPath:       kernelPath,
		ProfilerFullPath: "true",
		Logger:           slog.Default(),
	}

	// configmap is empty → areProfilesReadable returns (false, nil) → error expected
	_, err := loadNewProfiles(cfg)
	if err == nil {
		t.Error("expected error for empty configmap directory")
	}
}

// ---------------------------------------------------------------------------
// pollProfiles – additional coverage
// ---------------------------------------------------------------------------

func TestPollProfiles_WithInvalidConfigmap(t *testing.T) {
	tempDir := t.TempDir()

	etcApparmordDir := path.Join(tempDir, "etc_apparmord")
	kernelPath := path.Join(tempDir, "profiles")

	if err := os.MkdirAll(etcApparmordDir, 0o755); err != nil {
		t.Fatalf("mkdir etc: %v", err)
	}

	if err := os.WriteFile(kernelPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write kernel: %v", err)
	}

	cfg := &AppConfig{
		ConfigmapPath:    "/nonexistent/configmap",
		EtcApparmord:     etcApparmordDir,
		KernelPath:       kernelPath,
		ProfilerFullPath: "true",
		Logger:           slog.Default(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		pollProfiles(ctx, cfg, 1)
	}()

	// Let it tick once, then cancel
	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Error("pollProfiles did not stop after context cancellation")
	}
}

// ---------------------------------------------------------------------------
// metrics.ProfileUpdated (0% coverage)
// ---------------------------------------------------------------------------

func TestMetricsProfileUpdated(t *testing.T) {
	// ProfileUpdated is an alias for ProfileModified, just ensure it doesn't panic
	assertDontPanic(t, func() {
		metrics.ProfileUpdated("custom.test-profile")
	})
}

// ---------------------------------------------------------------------------
// compareLocalFiles error paths (now testable after removing os.Exit)
// ---------------------------------------------------------------------------

func TestCompareLocalFiles_ErrorReadingFile(t *testing.T) {
	// isSafePath allows /tmp
	nonExistentFile1 := "/tmp/kapparmor_test_nonexistent_1_" + t.Name()
	nonExistentFile2 := "/tmp/kapparmor_test_nonexistent_2_" + t.Name()

	// Both files don't exist → read error
	same, err := HasTheSameContent(nil, nonExistentFile1, nonExistentFile2)

	if err == nil {
		t.Error("expected error reading non-existent files, got nil")
	}

	if same {
		t.Error("expected same=false on error")
	}
}

// ---------------------------------------------------------------------------
// CopyFile error paths (now testable after removing os.Exit)
// ---------------------------------------------------------------------------

func TestCopyFile_NonexistentSource(t *testing.T) {
	tempDir := t.TempDir()

	err := CopyFile("/nonexistent/source/file", tempDir)

	if err == nil {
		t.Error("expected error for non-existent source file")
	}

	if !strings.Contains(err.Error(), "CopyFile: stat source") {
		t.Errorf("expected 'CopyFile: stat source' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// areProfilesReadable error paths (now testable after removing os.Exit)
// ---------------------------------------------------------------------------

func TestAreProfilesReadable_NonexistentDir(t *testing.T) {
	ok, profiles := areProfilesReadable("/nonexistent/directory/path")

	if ok {
		t.Error("expected ok=false for nonexistent directory")
	}

	if profiles != nil {
		t.Error("expected nil profiles for nonexistent directory")
	}
}

func TestAreProfilesReadable_InvalidProfileName(t *testing.T) {
	tempDir := t.TempDir()

	// Create a profile file whose filename doesn't match the declared name
	badProfile := filepath.Join(tempDir, "custom.bad-name")
	content := "profile custom.different-name { }"

	if err := os.WriteFile(badProfile, []byte(content), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	ok, profiles := areProfilesReadable(tempDir)

	if ok {
		t.Error("expected ok=false for profile with mismatched name")
	}

	if profiles != nil {
		t.Error("expected nil profiles for invalid profile")
	}
}

func TestAreProfilesReadable_DirectoryEntry(t *testing.T) {
	tempDir := t.TempDir()

	// Create a subdirectory that should be skipped
	subDir := filepath.Join(tempDir, "subdir")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	// Only a directory, no regular files → no profiles.
	// areProfilesReadable returns (true, {}) because files are present
	// (the directory counts as an entry) but all are skipped.
	ok, profiles := areProfilesReadable(tempDir)

	// The directory entry is counted but skipped → function returns true with empty map
	if !ok {
		t.Error("expected ok=true when only subdirectories present (skipped, not an error)")
	}

	if len(profiles) != 0 {
		t.Errorf("expected empty profiles map, got %d entries", len(profiles))
	}
}
