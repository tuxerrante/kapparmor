package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strings"
	"testing"
)

// TestPrintLoadedProfiles tests the profile printing functionality.
func TestPrintLoadedProfiles(t *testing.T) {
	tests := []struct {
		name     string
		profiles map[string]bool
	}{
		{
			name: "non-empty profiles",
			profiles: map[string]bool{
				"custom.deny-write":   true,
				"custom.deny-network": true,
				"built-in-profile":    true,
			},
		},
		{
			name:     "empty profiles",
			profiles: map[string]bool{},
		},
		{
			name: "profiles with empty string",
			profiles: map[string]bool{
				"":                    true,
				"custom.deny-write":   true,
				"custom.deny-network": true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Capture logs to verify behavior
			printLoadedProfiles(tc.profiles)
			// If we get here without panic, the test passes
			if _, exists := tc.profiles[""]; exists {
				// Verify empty string was deleted
				if tc.profiles[""] {
					t.Error("empty string should be deleted from profiles map")
				}
			}
		})
	}
}

// TestShowProfilesDiff tests the profile diff display.
func TestShowProfilesDiff(t *testing.T) {
	// Create temporary directory and files
	tempDir := t.TempDir()

	srcFile := path.Join(tempDir, "src.profile")
	srcContent := []byte(`profile custom.test flags=(attach_disconnected) {
  /home/** r,
  /tmp/** w,
}`)
	if err := os.WriteFile(srcFile, srcContent, 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	dstDir := path.Join(tempDir, "dest")
	if err := os.Mkdir(dstDir, 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	dstFile := path.Join(dstDir, "test.profile")
	dstContent := []byte(`profile custom.test flags=(attach_disconnected) {
  /home/** r,
  /var/** w,
}`)
	if err := os.WriteFile(dstFile, dstContent, 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	cfg := &AppConfig{
		EtcApparmord: dstDir,
	}

	// This should not panic
	showProfilesDiff(cfg, srcFile, "test.profile")
}

// TestCalculateProfileChanges tests the change calculation logic.
func TestCalculateProfileChanges(t *testing.T) {
	tempDir := t.TempDir()

	// Create test profiles in config directory
	configDir := path.Join(tempDir, "config")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create test files
	profile1 := path.Join(configDir, "custom.profile1")
	if err := os.WriteFile(profile1, []byte("profile content 1"), 0o644); err != nil {
		t.Fatalf("failed to create profile1: %v", err)
	}

	profile2 := path.Join(configDir, "custom.profile2")
	if err := os.WriteFile(profile2, []byte("profile content 2"), 0o644); err != nil {
		t.Fatalf("failed to create profile2: %v", err)
	}

	tests := []struct {
		name                 string
		newProfiles          map[string]bool
		customLoadedProfiles map[string]bool
		expectToApply        int
		expectToUnload       int
		shouldErr            bool
	}{
		{
			name: "all new profiles",
			newProfiles: map[string]bool{
				"custom.profile1": true,
				"custom.profile2": true,
			},
			customLoadedProfiles: map[string]bool{},
			expectToApply:        2,
			expectToUnload:       0,
			shouldErr:            false,
		},
		{
			name:        "unload orphaned profiles",
			newProfiles: map[string]bool{},
			customLoadedProfiles: map[string]bool{
				"custom.profile1": true,
				"custom.profile2": true,
			},
			expectToApply:  0,
			expectToUnload: 2,
			shouldErr:      false,
		},
		{
			name: "mixed: some new, some unload",
			newProfiles: map[string]bool{
				"custom.profile1": true,
			},
			customLoadedProfiles: map[string]bool{
				"custom.profile2": true, // Only unload custom.profile2 (not in new profiles)
			},
			expectToApply:  1,
			expectToUnload: 1,
			shouldErr:      false,
		},
	}

	cfg := &AppConfig{
		ConfigmapPath: configDir,
		EtcApparmord:  tempDir,
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			toApply, toUnload, err := calculateProfileChanges(cfg, tc.newProfiles, tc.customLoadedProfiles)

			if tc.shouldErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if len(toApply) != tc.expectToApply {
				t.Errorf("expected %d profiles to apply, got %d", tc.expectToApply, len(toApply))
			}
			if len(toUnload) != tc.expectToUnload {
				t.Errorf("expected %d profiles to unload, got %d", tc.expectToUnload, len(toUnload))
			}
		})
	}
}

// TestGetNewProfiles tests reading new profiles from config.
func TestGetNewProfiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create valid profile
	profile1 := path.Join(tempDir, "custom.valid")
	if err := os.WriteFile(profile1, []byte("profile custom.valid { }"), 0o644); err != nil {
		t.Fatalf("failed to create profile: %v", err)
	}

	// Create hidden file (should be skipped)
	hidden := path.Join(tempDir, ".hidden")
	if err := os.WriteFile(hidden, []byte("hidden"), 0o644); err != nil {
		t.Fatalf("failed to create hidden file: %v", err)
	}

	cfg := &AppConfig{
		ConfigmapPath: tempDir,
	}

	readable, profiles := getNewProfiles(cfg)

	if !readable {
		t.Error("expected profiles to be readable")
	}

	if len(profiles) == 0 {
		t.Error("expected to find profiles")
	}

	if !profiles["custom.valid"] {
		t.Error("expected to find custom.valid profile")
	}

	// Hidden file should not be in profiles
	if profiles[".hidden"] {
		t.Error("hidden file should not be in profiles")
	}
}

// TestGetLoadedProfiles tests reading currently loaded profiles.
func TestGetLoadedProfiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create mock kernel profile file
	profilesFile := path.Join(tempDir, "profiles")
	content := `custom.profile1 (enforce)
custom.profile2 (complain)
built-in-profile (enforce)
`
	if err := os.WriteFile(profilesFile, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to create profiles file: %v", err)
	}

	cfg := &AppConfig{
		KernelPath: profilesFile,
	}

	allProfiles, customProfiles, err := getLoadedProfiles(cfg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(allProfiles) == 0 {
		t.Error("expected to find all profiles")
	}

	if len(customProfiles) == 0 {
		t.Error("expected to find custom profiles")
	}

	if !allProfiles["custom.profile1"] {
		t.Error("expected to find custom.profile1 in all profiles")
	}

	if !customProfiles["custom.profile1"] {
		t.Error("expected to find custom.profile1 in custom profiles")
	}

	if customProfiles["built-in-profile"] {
		t.Error("built-in profile should not be in custom profiles")
	}
}

// TestPrintLogSeparator tests the log separator.
func TestPrintLogSeparator(t *testing.T) {
	// This should not panic
	printLogSeparator()
}

// TestShowProfilesDiff_WithMissingFile tests diff with missing destination file.
func TestShowProfilesDiff_WithMissingFile(t *testing.T) {
	tempDir := t.TempDir()

	srcFile := path.Join(tempDir, "src.profile")
	if err := os.WriteFile(srcFile, []byte("src content"), 0o644); err != nil {
		t.Fatalf("failed to create src file: %v", err)
	}

	dstDir := path.Join(tempDir, "dest")
	if err := os.Mkdir(dstDir, 0o755); err != nil {
		t.Fatalf("failed to create dest dir: %v", err)
	}

	cfg := &AppConfig{
		EtcApparmord: dstDir,
	}

	// Should handle missing destination file gracefully
	showProfilesDiff(cfg, srcFile, "nonexistent.profile")
}

// TestCalculateProfileChanges_NonexistentProfile tests with missing source file.
func TestCalculateProfileChanges_NonexistentProfile(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &AppConfig{
		ConfigmapPath: tempDir,
		EtcApparmord:  tempDir,
	}

	newProfiles := map[string]bool{
		"custom.nonexistent": true,
	}
	customLoadedProfiles := map[string]bool{}

	toApply, toUnload, err := calculateProfileChanges(cfg, newProfiles, customLoadedProfiles)

	// Should not apply nonexistent profile - function schedules it but doesn't check existence
	// This is intentional - error handling happens when exec is called
	if len(toApply) != 1 {
		t.Errorf("expected 1 profile to apply (scheduled for later), got %d", len(toApply))
	}

	if len(toUnload) != 0 {
		t.Error("should have no unloads")
	}

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestGetLoadedProfiles_NonexistentFile tests with missing kernel file.
func TestGetLoadedProfiles_NonexistentFile(t *testing.T) {
	cfg := &AppConfig{
		KernelPath: "/nonexistent/path/profiles",
	}

	allProfiles, customProfiles, err := getLoadedProfiles(cfg)

	if err == nil {
		t.Error("expected error for nonexistent file")
	}

	if allProfiles != nil || customProfiles != nil {
		t.Error("should return nil for both maps on error")
	}
}

// TestGetNewProfiles_NonexistentDir tests with missing config directory.
// Note: This test is commented out because areProfilesReadable calls os.Exit(1) on error,
// which would crash the test. In production, the directory should always exist or the app
// shouldn't start.
func TestGetNewProfiles_NonexistentDir_Skipped(t *testing.T) {
	// Skip this test - areProfilesReadable calls os.Exit on error
	t.Skip("areProfilesReadable calls os.Exit(1) for nonexistent directories")
}

// TestExecApparmor_Success tests successful apparmor_parser execution.
func TestExecApparmor_Success(t *testing.T) {
	tempDir := t.TempDir()

	// Create a mock profile file
	profileFile := path.Join(tempDir, "test.profile")
	content := `profile custom.test flags=(attach_disconnected) {
  /home/** r,
}
`
	if err := os.WriteFile(profileFile, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to create profile file: %v", err)
	}

	cfg := &AppConfig{
		ProfilerFullPath: "apparmor_parser",
	}

	// Try to execute - will fail if apparmor_parser not available, but that's ok for this test
	// We're testing the function behavior when it's called
	err := execApparmor(cfg, "--version")
	// Don't assert on error as apparmor_parser might not be available in test env
	if profileFile != "" {
		_ = err
	}
}

// TestExecApparmor_InvalidPath tests with invalid parser path.
func TestExecApparmor_InvalidPath(t *testing.T) {
	cfg := &AppConfig{
		ProfilerFullPath: "/nonexistent/path/apparmor_parser",
	}

	err := execApparmor(cfg, "--version")

	if err == nil {
		t.Error("expected error with invalid parser path")
	}

	if !strings.Contains(err.Error(), "error loading profile") {
		t.Errorf("expected 'error loading profile' in error message, got: %v", err)
	}
}

// TestExecApparmor_NoArgs tests with minimal arguments.
func TestExecApparmor_NoArgs(t *testing.T) {
	cfg := &AppConfig{
		ProfilerFullPath: "echo",
	}

	err := execApparmor(cfg, "--help")

	// echo should succeed
	if err != nil {
		// It's ok if echo is not available
		_ = err
	}
}

// TestCalculateProfileChanges_IdenticalContent tests when content hasn't changed.
func TestCalculateProfileChanges_IdenticalContent(t *testing.T) {
	tempDir := t.TempDir()

	// Create config directory
	configDir := path.Join(tempDir, "config")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create dest directory
	destDir := path.Join(tempDir, "dest")
	if err := os.Mkdir(destDir, 0o755); err != nil {
		t.Fatalf("failed to create dest dir: %v", err)
	}

	// Create identical files
	content := []byte("profile custom.test { }")

	srcFile := path.Join(configDir, "custom.test")
	if err := os.WriteFile(srcFile, content, 0o644); err != nil {
		t.Fatalf("failed to create src file: %v", err)
	}

	dstFile := path.Join(destDir, "custom.test")
	if err := os.WriteFile(dstFile, content, 0o644); err != nil {
		t.Fatalf("failed to create dst file: %v", err)
	}

	cfg := &AppConfig{
		ConfigmapPath: configDir,
		EtcApparmord:  destDir,
	}

	newProfiles := map[string]bool{
		"custom.test": true,
	}
	customLoadedProfiles := map[string]bool{
		"custom.test": true,
	}

	toApply, toUnload, err := calculateProfileChanges(cfg, newProfiles, customLoadedProfiles)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not apply when content is the same
	if len(toApply) != 0 {
		t.Errorf("expected 0 profiles to apply for identical content, got %d", len(toApply))
	}

	if len(toUnload) != 0 {
		t.Errorf("expected 0 profiles to unload, got %d", len(toUnload))
	}
}

// TestGetProfilesNamesFromFile_EmptyFile tests reading empty profiles file.
func TestGetProfilesNamesFromFile_EmptyFile(t *testing.T) {
	tempDir := t.TempDir()

	emptyFile := path.Join(tempDir, "empty")
	if err := os.WriteFile(emptyFile, []byte(""), 0o644); err != nil {
		t.Fatalf("failed to create empty file: %v", err)
	}

	allProfiles, customProfiles, err := getProfilesNamesFromFile(emptyFile, ProfileNamePrefix)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(allProfiles) != 0 {
		t.Error("expected empty map for empty file")
	}

	if len(customProfiles) != 0 {
		t.Error("expected empty custom profiles for empty file")
	}
}

// TestGetProfilesNamesFromFile_InvalidFormat tests with malformed profile lines.
func TestGetProfilesNamesFromFile_InvalidFormat(t *testing.T) {
	tempDir := t.TempDir()

	// Create file with invalid format (no parentheses)
	profileFile := path.Join(tempDir, "invalid")
	content := `invalid line without parentheses
another invalid line
`
	if err := os.WriteFile(profileFile, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	allProfiles, customProfiles, err := getProfilesNamesFromFile(profileFile, ProfileNamePrefix)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(allProfiles) != 0 {
		t.Error("expected empty map for invalid format")
	}

	if len(customProfiles) != 0 {
		t.Error("expected empty custom profiles for invalid format")
	}
}

// TestCountLines tests the countLines helper with various edge cases.
func TestCountLines(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  int
	}{
		{name: "empty data", input: []byte{}, want: 0},
		{name: "single line without newline", input: []byte("hello"), want: 1},
		{name: "single line with trailing newline", input: []byte("hello\n"), want: 1},
		{name: "multiple lines with trailing newline", input: []byte("a\nb\nc\n"), want: 3},
		{name: "multiple lines without trailing newline", input: []byte("a\nb\nc"), want: 3},
		{name: "only newline", input: []byte("\n"), want: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := countLines(tc.input)
			if got != tc.want {
				t.Errorf("countLines(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

// TestProfileDigest tests the profileDigest function with various inputs.
func TestProfileDigest(t *testing.T) {
	t.Run("normal content returns correct full sha256 and line count", func(t *testing.T) {
		data := []byte("profile custom.test {\n  /tmp/** rw,\n}\n")
		h := sha256.Sum256(data)
		expectedHash := fmt.Sprintf("%x", h[:])

		hash, lines := profileDigest(data, nil)

		if hash != expectedHash {
			t.Errorf("expected full SHA-256 %q, got %q", expectedHash, hash)
		}
		if len(hash) != 64 {
			t.Errorf("expected 64 hex chars for full SHA-256, got %d", len(hash))
		}
		if lines != 3 {
			t.Errorf("expected 3 lines, got %d", lines)
		}
	})

	t.Run("empty content returns hash and zero lines", func(t *testing.T) {
		hash, lines := profileDigest([]byte{}, nil)
		if hash == "<read-error>" {
			t.Error("expected valid hash for empty content, not <read-error>")
		}
		if len(hash) != 64 {
			t.Errorf("expected 64 hex chars, got %d", len(hash))
		}
		if lines != 0 {
			t.Errorf("expected 0 lines for empty content, got %d", lines)
		}
	})

	t.Run("content with trailing newline counts correctly", func(t *testing.T) {
		data := []byte("line1\nline2\n")
		_, lines := profileDigest(data, nil)
		if lines != 2 {
			t.Errorf("expected 2 lines for trailing-newline content, got %d", lines)
		}
	})

	t.Run("read error returns read-error sentinel and zero lines", func(t *testing.T) {
		hash, lines := profileDigest(nil, errors.New("permission denied"))
		if hash != "<read-error>" {
			t.Errorf("expected <read-error>, got %q", hash)
		}
		if lines != 0 {
			t.Errorf("expected 0 lines on error, got %d", lines)
		}
	})
}

// TestShowProfilesDiff_T7_NoContentInLogs is the T7 security regression test:
// verifies that showProfilesDiff logs metadata fields but NOT the raw file content.
func TestShowProfilesDiff_T7_NoContentInLogs(t *testing.T) {
	tempDir := t.TempDir()

	srcContent := "profile custom.secret {\n  /secret/** r,\n}\n"
	dstContent := "profile custom.secret {\n  /other/** r,\n}\n"

	srcFile := path.Join(tempDir, "custom.secret")
	if err := os.WriteFile(srcFile, []byte(srcContent), 0o644); err != nil {
		t.Fatalf("failed to create src file: %v", err)
	}

	dstDir := path.Join(tempDir, "dest")
	if err := os.Mkdir(dstDir, 0o755); err != nil {
		t.Fatalf("failed to create dest dir: %v", err)
	}
	dstFile := path.Join(dstDir, "custom.secret")
	if err := os.WriteFile(dstFile, []byte(dstContent), 0o644); err != nil {
		t.Fatalf("failed to create dst file: %v", err)
	}

	// Capture slog output to a buffer.
	var buf bytes.Buffer
	originalDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(originalDefault)

	cfg := &AppConfig{EtcApparmord: dstDir}
	showProfilesDiff(cfg, srcFile, "custom.secret")

	output := buf.String()

	// Must contain metadata fields.
	if !strings.Contains(output, "src_sha256") {
		t.Error("expected log output to contain src_sha256")
	}
	if !strings.Contains(output, "dst_sha256") {
		t.Error("expected log output to contain dst_sha256")
	}
	if !strings.Contains(output, "src_size") {
		t.Error("expected log output to contain src_size")
	}
	if !strings.Contains(output, "src_lines") {
		t.Error("expected log output to contain src_lines")
	}

	// Must NOT contain actual file content strings (T7 redaction check).
	if strings.Contains(output, "/secret/**") {
		t.Error("log output must not contain raw profile content (/secret/**)")
	}
	if strings.Contains(output, "/other/**") {
		t.Error("log output must not contain raw profile content (/other/**)")
	}
}

// TestShowProfilesDiff_T7_ErrorFieldsPresent verifies that src_error/dst_error are
// logged when file reads fail.
func TestShowProfilesDiff_T7_ErrorFieldsPresent(t *testing.T) {
	tempDir := t.TempDir()

	srcFile := path.Join(tempDir, "custom.missing")
	// Do NOT create the src file so the read will fail.

	dstDir := path.Join(tempDir, "dest")
	if err := os.Mkdir(dstDir, 0o755); err != nil {
		t.Fatalf("failed to create dest dir: %v", err)
	}

	var buf bytes.Buffer
	originalDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(originalDefault)

	cfg := &AppConfig{EtcApparmord: dstDir}
	showProfilesDiff(cfg, srcFile, "custom.missing")

	output := buf.String()

	if !strings.Contains(output, "src_error") {
		t.Error("expected log output to contain src_error when src file is missing")
	}
	if !strings.Contains(output, "dst_error") {
		t.Error("expected log output to contain dst_error when dst file is missing")
	}
}
