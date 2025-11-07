package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func ok(t testing.TB, err error) {
	if err != nil {
		t.Fatalf("Function call returned an error:\n %s", err)
	}
}

func assertBool(t *testing.T, got, want bool) {
	t.Helper()

	if got != want {
		t.Fatalf("Bool check failed! Got %t, expected %t", got, want)
	}
}

func assertError(t *testing.T, got, want error) {
	t.Helper()

	if want != nil {
		if got == nil || want.Error() != got.Error() {
			t.Fatalf("Error check failed! Got %v, expected %v", got, want)
		}
	}
}

func assertDontPanic(t *testing.T, f func()) {
	defer func() {
		if recover() != nil {
			t.Errorf("The code did panic")
		}
	}()

	f()
}

// newTestConfig returns (*AppConfig, *os.File) and builds a temp fake apparmor_parser.
func preFlightChecksInit(t *testing.T) (*AppConfig, *os.File) {
	t.Helper()

	tmp := t.TempDir()

	// Create fake apparmor binary
	f, err := os.CreateTemp(tmp, "apparmor_parser")
	if err != nil {
		t.Fatalf("failed to create temporary file: %v", err)
	}

	if err := os.WriteFile(f.Name(), []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
		t.Fatalf("failed to make fake parser executable: %v", err)
	}

	// Temp dirs/files used by the app
	profilesDir := filepath.Join(tmp, "profiles")
	etcCustom := filepath.Join(tmp, "etc_apparmor.d_custom")
	kernelFile := filepath.Join(tmp, "kernel_profiles")
	_ = os.MkdirAll(profilesDir, 0o750)
	_ = os.MkdirAll(etcCustom, 0o750)
	_ = os.WriteFile(kernelFile, []byte{}, 0o600)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := &AppConfig{
		ConfigmapPath:     profilesDir,
		EtcApparmord:      etcCustom,
		PollTimeArg:       "1",
		ProfilerBinFolder: filepath.Dir(f.Name()),
		ProfilerFullPath:  f.Name(),
		KernelPath:        kernelFile,
		Logger:            logger,
		// Global chan to avoid race conditions.
		// Signals: globalSignals,
	}
	slog.SetDefault(cfg.Logger)

	return cfg, f
}

// writeParserScript creates a script that returns the given exit code and writes to stderr if specified.
func writeParserScript(t *testing.T, dir string, exitCode int, writeStderr bool) string {
	t.Helper()

	s := filepath.Join(dir, "apparmor_parser")

	payload := "#!/bin/sh\n"
	if writeStderr {
		payload += "echo 'ERR: simulated failure' 1>&2\n"
	} else {
		payload += "echo 'OK: simulated load'\n"
	}

	payload += "exit " + func() string {
		if exitCode == 0 {
			return "0"
		}

		return "1"
	}() + "\n"

	err := os.WriteFile(s, []byte(payload), 0o600)
	if err != nil {
		t.Fatalf("write parser: %v", err)
	}

	err = os.Chmod(s, 0o700)
	if err != nil { // #nosec G302
		t.Fatalf("chmod parser: %v", err)
	}

	return s
}
