package main

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// ---- Test fixtures ----.
const testProfileData = `profile foo.profile {
#include <abstractions/base>
}`

const testProfileDataDifferent = `
profile bar.profile {
#include <abstractions/base>
}`

// Creates a temp dir and symlinks it under /app to satisfy isSafePath.
func makeSafeDirForTest(t *testing.T) string {
	t.Helper()
	localTmp := t.TempDir()
	safeBase := "/app"
	safeDir := filepath.Join(safeBase, filepath.Base(localTmp))
	defer os.RemoveAll(safeDir) // clean previous

	_ = os.MkdirAll(filepath.Dir(safeDir), 0o755)

	err := os.Symlink(localTmp, safeDir)
	if err != nil {
		// fallback: just use /tmp if /app not writable
		t.Logf("WARN: could not symlink under /app, fallback to local temp: %v", err)

		return localTmp
	}

	return safeDir
}

// ---- Tests ----

func TestHasTheSameContent_fsFS(t *testing.T) {
	fsmap := fstest.MapFS{
		"foo.profile":      {Data: []byte(testProfileData)},
		"foo.profile.copy": {Data: []byte(testProfileData)},
		"bar.profile":      {Data: []byte(testProfileDataDifferent)},
	}

	t.Run("identical profiles", func(t *testing.T) {
		got, err := HasTheSameContent(fsmap, "foo.profile", "foo.profile.copy")
		ok(t, err)
		assertBool(t, got, true)
	})
	t.Run("different profiles", func(t *testing.T) {
		got, err := HasTheSameContent(fsmap, "foo.profile", "bar.profile")
		ok(t, err)
		assertBool(t, got, false)
	})
}

func TestHasTheSameContent_localFiles(t *testing.T) {
	tmp := t.TempDir()

	tmp1 := filepath.Join(tmp, "a.profile")
	tmp2 := filepath.Join(tmp, "b.profile")
	tmp3 := filepath.Join(tmp, "c.profile")

	os.WriteFile(tmp1, []byte(testProfileData), 0o644)
	os.WriteFile(tmp2, []byte(testProfileData), 0o644)
	os.WriteFile(tmp3, []byte(testProfileDataDifferent), 0o644)

	t.Run("same local files", func(t *testing.T) {
		got, err := HasTheSameContent(nil, tmp1, tmp2)
		ok(t, err)
		assertBool(t, got, true)
	})

	t.Run("different local files", func(t *testing.T) {
		got, err := HasTheSameContent(nil, tmp1, tmp3)
		ok(t, err)
		assertBool(t, got, false)
	})
}

func TestAreProfilesReadable(t *testing.T) {
	// t.Parallel()  // cannot be parallel due to os operations going in race conditions

	tmp := makeSafeDirForTest(t)
	os.MkdirAll(tmp, 0o755)

	testingFileName := "foo.profile"
	validProfile := filepath.Join(tmp, testingFileName)
	content := []byte(testProfileData)
	os.WriteFile(validProfile, content, 0o644)

	t.Run("folder with valid profile", func(t *testing.T) {
		readable, profiles := areProfilesReadable(tmp)
		assertBool(t, readable, true)

		if !profiles[testingFileName] {
			t.Fatalf("expected profile 'valid.profile' to be found. Got: %v", profiles)
		}
	})

	t.Run("folder with hidden file", func(t *testing.T) {
		os.WriteFile(filepath.Join(tmp, ".hidden"), []byte("ignored"), 0o644)
		readable, profiles := areProfilesReadable(tmp)
		assertBool(t, readable, true)

		if profiles[".hidden"] {
			t.Fatalf("hidden file should be skipped")
		}
	})
}

func TestPreFlightChecks_createsDir(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	cfg := &AppConfig{
		PollTimeArg:       "10",
		ProfilerFullPath:  "/bin/true", // exists
		EtcApparmord:      filepath.Join(tmp, "custom"),
		ProfilerBinFolder: "/sbin",
		Logger:            slog.Default(),
	}

	poll, err := preFlightChecks(cfg)
	ok(t, err)

	if poll != 10 {
		t.Fatalf("expected poll time 10, got %d", poll)
	}

	if _, err := os.Stat(cfg.EtcApparmord); err != nil {
		t.Fatalf("expected directory to be created: %v", err)
	}
}

func TestCopyFileContents(t *testing.T) {
	t.Parallel()
	const testContent = "profile custom.hello {}"
	tmp := makeSafeDirForTest(t)
	os.MkdirAll(tmp, 0o755)
	src := filepath.Join(tmp, "src.txt")
	dst := filepath.Join(tmp, "dst.txt")

	os.WriteFile(src, []byte(testContent), 0o644)

	err := copyFileContents(src, dst)
	ok(t, err)

	b, _ := os.ReadFile(dst)
	if string(b) != testContent {
		t.Fatalf("expected copied contents, got %s", string(b))
	}
}

// ---- small edge check: invalid fs path ----.
type badFS struct{ fs.FS }

func (b badFS) Open(name string) (fs.File, error) { return nil, os.ErrNotExist }

func TestCompareFSFiles_error(t *testing.T) {
	_, err := HasTheSameContent(badFS{}, "nonexistent", "other")
	if err == nil {
		t.Fatal("expected error for nonexistent files")
	}
}
