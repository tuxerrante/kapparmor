package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

/*
		TestIsProfileNameCorrect checks if profile names and filenames match
		The name must not begin with a : or . character.
	 	If it contains a whitespace, it must be quoted.
		If the name begins with a /, the profile is considered to be a standard profile,
		https://documentation.suse.com/sles/15-SP1/html/SLES-all/cha-apparmor-profiles.html#sec-apparmor-profiles-types-unattached
*/
func TestIsProfileNameCorrect(t *testing.T) {
	const testsDirectory string = "profile_test_samples"

	t.Parallel()

	// table-driven tests
	var tests = []struct {
		name, filename string
		want           error
	}{
		{
			name:     "OK: filename and profile name are the same",
			filename: "custom.myValidProfile",
			want:     nil,
		},
		{
			name:     "Deny a filename different from profile name",
			filename: "custom.myNotValidProfile",
			want:     errors.New("filename 'custom.myNotValidProfile' and profile name 'myNotValidProfile' seems to be different"),
		},
		{
			name:     "OK: filename and profile name are the same",
			filename: "custom.bin.foo",
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsProfileNameCorrect(testsDirectory, tt.filename)
			assertError(t, got, tt.want)
		})
	}
}

// newCfgForLoadUnload creates a temporary AppConfig and directories for load/unload tests.
func newCfgForLoadUnload(t *testing.T, parserExitOK bool) (*AppConfig, string, string) {
	t.Helper()
	tmp := t.TempDir()

	// Copy binaries in /tmp/etc/
	etc := filepath.Join(tmp, "etc_apparmor.d_custom")
	if err := os.MkdirAll(etc, 0o750); err != nil {
		t.Fatalf("mkdir etc: %v", err)
	}

	// Use "true"/"false" Linux binaries as fake apparmor_parser
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Fatalf("lookpath true: %v", err)
	}

	falsePath, err := exec.LookPath("false")
	if err != nil {
		t.Fatalf("lookpath false: %v", err)
	}

	parser := map[bool]string{true: truePath, false: falsePath}[parserExitOK]

	cfg := &AppConfig{
		ProfilerFullPath: parser,
		EtcApparmord:     etc,
	}

	return cfg, tmp, etc
}

func Test_loadProfile_copies_file_and_calls_parser(t *testing.T) {
	cfg, tmp, etc := newCfgForLoadUnload(t, true)

	// Create fake profile
	src := filepath.Join(tmp, "custom.demo")

	content := []byte("profile custom.demo { }")

	err := os.WriteFile(src, content, 0o600)
	if err != nil {
		t.Fatalf("write src: %v", err)
	}

	err = loadProfile(cfg, src)
	if err != nil {
		t.Fatalf("loadProfile error: %v", err)
	}

	// Verify copy
	if got, err := os.ReadFile(filepath.Join(etc, "custom.demo")); err != nil || string(got) != string(content) {
		t.Fatalf("copied content mismatch: %v", err)
	}
}

func Test_unloadProfile_removes_file(t *testing.T) {
	cfg, _, etc := newCfgForLoadUnload(t, true)
	// prepara un file in ETC
	name := "custom.demo"

	path := filepath.Join(etc, name)

	err := os.WriteFile(path, []byte("profile custom.demo { }"), 0o600)
	if err != nil {
		t.Fatalf("write etc file: %v", err)
	}

	err = unloadProfile(cfg, name)
	if err != nil {
		t.Fatalf("unloadProfile: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, err=%v", err)
	}
}

func Test_unloadAllProfiles_removes_all(t *testing.T) {
	cfg, _, etc := newCfgForLoadUnload(t, true)
	// two files in ETC
	for _, n := range []string{"custom.a", "custom.b"} {
		err := os.WriteFile(filepath.Join(etc, n), []byte("profile "+n+" { }"), 0o600)
		if err != nil {
			t.Fatalf("seed etc file %s: %v", n, err)
		}
	}

	err := unloadAllProfiles(cfg)
	if err != nil {
		t.Fatalf("unloadAllProfiles: %v", err)
	}
	// directory should be empty
	entries, _ := os.ReadDir(etc)
	if len(entries) != 0 {
		t.Fatalf("expected empty etc dir, got %d entries", len(entries))
	}
}
