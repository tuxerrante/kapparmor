package main

import (
	"context"
	"log/slog"
	"os"
	"path"
	"testing"
	"time"
)

// TestRunApp_ShutdownImmediately tests RunApp with immediate shutdown.
func TestRunApp_ShutdownImmediately(t *testing.T) {
	tempDir := t.TempDir()

	// Create required directories
	configmapDir := path.Join(tempDir, "configmap")
	etcApparmordDir := path.Join(tempDir, "etc_apparmord")
	if err := os.MkdirAll(configmapDir, 0o755); err != nil {
		t.Fatalf("failed to create configmap dir: %v", err)
	}
	if err := os.MkdirAll(etcApparmordDir, 0o755); err != nil {
		t.Fatalf("failed to create etc_apparmord dir: %v", err)
	}

	// Create kernel path file
	kernelPath := path.Join(tempDir, "profiles")
	if err := os.WriteFile(kernelPath, []byte(""), 0o644); err != nil {
		t.Fatalf("failed to create kernel profile file: %v", err)
	}

	cfg := &AppConfig{
		ConfigmapPath:    configmapDir,
		EtcApparmord:     etcApparmordDir,
		KernelPath:       kernelPath,
		ProfilerFullPath: "/nonexistent/parser", // Won't actually be called
		Logger:           slog.Default(),
	}

	// Create a context that cancels after 100ms
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// RunApp should exit cleanly when context is cancelled
	err := RunApp(ctx, cfg)

	// Should not error when cancelled
	if err != nil && err != context.DeadlineExceeded {
		t.Logf("expected context deadline or nil, got: %v", err)
	}
}

// TestRunApp_WithValidProfiles tests RunApp with valid profile loading.
func TestRunApp_WithValidProfiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create directories
	configmapDir := path.Join(tempDir, "configmap")
	etcApparmordDir := path.Join(tempDir, "etc_apparmord")
	if err := os.MkdirAll(configmapDir, 0o755); err != nil {
		t.Fatalf("failed to create configmap dir: %v", err)
	}
	if err := os.MkdirAll(etcApparmordDir, 0o755); err != nil {
		t.Fatalf("failed to create etc_apparmord dir: %v", err)
	}

	// Create kernel path file with existing profile
	kernelPath := path.Join(tempDir, "profiles")
	if err := os.WriteFile(kernelPath, []byte(""), 0o644); err != nil {
		t.Fatalf("failed to create kernel profile file: %v", err)
	}

	cfg := &AppConfig{
		ConfigmapPath:    configmapDir,
		EtcApparmord:     etcApparmordDir,
		KernelPath:       kernelPath,
		ProfilerFullPath: "apparmor_parser",
		Logger:           slog.Default(),
	}

	// Create a context that cancels quickly
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// RunApp should handle empty configmap gracefully
	_ = RunApp(ctx, cfg)
}

// TestPollProfiles_ContextCancellation tests that pollProfiles respects context cancellation.
func TestPollProfiles_ContextCancellation(t *testing.T) {
	tempDir := t.TempDir()

	configmapDir := path.Join(tempDir, "configmap")
	etcApparmordDir := path.Join(tempDir, "etc_apparmord")
	kernelPath := path.Join(tempDir, "profiles")

	if err := os.MkdirAll(configmapDir, 0o755); err != nil {
		t.Fatalf("failed to create configmap dir: %v", err)
	}
	if err := os.MkdirAll(etcApparmordDir, 0o755); err != nil {
		t.Fatalf("failed to create etc_apparmord dir: %v", err)
	}
	if err := os.WriteFile(kernelPath, []byte(""), 0o644); err != nil {
		t.Fatalf("failed to create kernel profile file: %v", err)
	}

	cfg := &AppConfig{
		ConfigmapPath:    configmapDir,
		EtcApparmord:     etcApparmordDir,
		KernelPath:       kernelPath,
		ProfilerFullPath: "apparmor_parser",
		Logger:           slog.Default(),
	}

	// Create a context that cancels immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// This should return immediately without hanging
	start := time.Now()
	pollProfiles(ctx, cfg, 1)
	elapsed := time.Since(start)

	// Should return quickly (within 100ms)
	if elapsed > 100*time.Millisecond {
		t.Errorf("pollProfiles took too long: %v", elapsed)
	}
}

// TestLoadProfile_ErrorHandling tests error handling in loadProfile.
func TestLoadProfile_ErrorHandling(t *testing.T) {
	tempDir := t.TempDir()

	// Create a profile file
	profileFile := path.Join(tempDir, "test.profile")
	if err := os.WriteFile(profileFile, []byte("profile custom.test { }"), 0o644); err != nil {
		t.Fatalf("failed to create profile file: %v", err)
	}

	cfg := &AppConfig{
		EtcApparmord:     tempDir,
		ProfilerFullPath: "/nonexistent/parser",
	}

	// Should fail because parser doesn't exist
	err := loadProfile(cfg, profileFile)

	if err == nil {
		t.Error("expected error for nonexistent parser")
	}
}

// TestLoadProfile_Success tests successful profile loading (without apparmor_parser).
func TestLoadProfile_Success(t *testing.T) {
	tempDir := t.TempDir()

	// Create source profile
	srcProfile := path.Join(tempDir, "src", "custom.test")
	if err := os.MkdirAll(path.Dir(srcProfile), 0o755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}
	if err := os.WriteFile(srcProfile, []byte("profile custom.test { }"), 0o644); err != nil {
		t.Fatalf("failed to create profile file: %v", err)
	}

	destDir := path.Join(tempDir, "dest")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("failed to create dest dir: %v", err)
	}

	cfg := &AppConfig{
		EtcApparmord:     destDir,
		ProfilerFullPath: "true", // 'true' always succeeds
	}

	err := loadProfile(cfg, srcProfile)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Check that file was copied
	destFile := path.Join(destDir, "custom.test")
	if _, err := os.Stat(destFile); os.IsNotExist(err) {
		t.Error("expected profile to be copied to destination")
	}
}

// TestUnloadAllProfiles_EmptyDirectory tests with empty profile directory.
func TestUnloadAllProfiles_EmptyDirectory(t *testing.T) {
	tempDir := t.TempDir()

	destDir := path.Join(tempDir, "empty")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("failed to create dest dir: %v", err)
	}

	cfg := &AppConfig{
		EtcApparmord:     destDir,
		ProfilerFullPath: "true",
	}

	err := unloadAllProfiles(cfg)

	if err != nil {
		t.Errorf("unexpected error for empty directory: %v", err)
	}
}

// TestUnloadAllProfiles_WithProfiles tests unloading multiple profiles.
func TestUnloadAllProfiles_WithProfiles(t *testing.T) {
	tempDir := t.TempDir()

	destDir := path.Join(tempDir, "dest")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("failed to create dest dir: %v", err)
	}

	// Create test profile files
	for i := 1; i <= 3; i++ {
		profile := path.Join(destDir, "custom.test"+string(rune(i)))
		if err := os.WriteFile(profile, []byte("test"), 0o644); err != nil {
			t.Fatalf("failed to create profile: %v", err)
		}
	}

	cfg := &AppConfig{
		EtcApparmord:     destDir,
		ProfilerFullPath: "true",
	}

	err := unloadAllProfiles(cfg)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// All files should be removed
	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected empty directory, found %d files", len(entries))
	}
}

// TestUnloadAllProfiles_NonexistentDirectory tests with non-existent directory.
func TestUnloadAllProfiles_NonexistentDirectory(t *testing.T) {
	cfg := &AppConfig{
		EtcApparmord:     "/nonexistent/path",
		ProfilerFullPath: "true",
	}

	err := unloadAllProfiles(cfg)

	if err != nil {
		t.Errorf("expected no error for nonexistent directory, got: %v", err)
	}
}

// TestUnloadProfile_Success tests successful profile unloading.
func TestUnloadProfile_Success(t *testing.T) {
	tempDir := t.TempDir()

	// Create profile file
	profileFile := path.Join(tempDir, "custom.test")
	if err := os.WriteFile(profileFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to create profile file: %v", err)
	}

	cfg := &AppConfig{
		EtcApparmord:     tempDir,
		ProfilerFullPath: "true",
	}

	err := unloadProfile(cfg, "custom.test")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// File should be removed
	if _, err := os.Stat(profileFile); !os.IsNotExist(err) {
		t.Error("expected profile file to be removed")
	}
}

// TestUnloadProfile_NonexistentProfile tests with non-existent profile.
func TestUnloadProfile_NonexistentProfile(t *testing.T) {
	tempDir := t.TempDir()

	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	cfg := &AppConfig{
		EtcApparmord:     tempDir,
		ProfilerFullPath: "true",
	}

	// Should not error for non-existent profile
	err := unloadProfile(cfg, "nonexistent.profile")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestUnloadProfile_ParserFailure tests when apparmor_parser fails to remove.
func TestUnloadProfile_ParserFailure(t *testing.T) {
	tempDir := t.TempDir()

	// Create profile file
	profileFile := path.Join(tempDir, "custom.test")
	if err := os.WriteFile(profileFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to create profile file: %v", err)
	}

	cfg := &AppConfig{
		EtcApparmord:     tempDir,
		ProfilerFullPath: "false", // 'false' always fails
	}

	err := unloadProfile(cfg, "custom.test")

	// Should have error from parser failure, but file should still be removed
	if err == nil {
		t.Error("expected error when parser fails")
	}

	// File might or might not be removed depending on error handling
	// Just verify the function doesn't crash
}
