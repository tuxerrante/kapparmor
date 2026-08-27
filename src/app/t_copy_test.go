package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func Test_CopyFile(t *testing.T) {
	t.Parallel()

	// Use /tmp directly so paths pass isSafePath validation.
	dir, err := os.MkdirTemp("/tmp", "test-copyfile-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(dir)

	src := filepath.Join(dir, "custom.demo")
	dstDir := filepath.Join(dir, "dst")

	err = os.WriteFile(src, []byte("demo"), 0o600)
	if err != nil {
		t.Fatalf("write src: %v", err)
	}

	err = os.MkdirAll(dstDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir dst: %v", err)
	}

	err = CopyFile(src, dstDir)
	if err != nil {
		t.Fatalf("CopyFile error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dstDir, "custom.demo")); err != nil {
		t.Fatalf("copied file missing: %v", err)
	}
}

func Test_CopyFile_UnsafePath(t *testing.T) {
	t.Parallel()

	err := CopyFile("/var/secret/file", "/tmp/dst")
	if err == nil {
		t.Fatal("expected error for unsafe source path")
	}
	if !strings.Contains(err.Error(), "unsafe path") {
		t.Errorf("expected unsafe path error, got: %v", err)
	}
}

func Test_CopyFile_NonexistentSource(t *testing.T) {
	t.Parallel()

	err := CopyFile("/tmp/nonexistent-file-abc123", "/tmp/dst")
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}
	if !strings.Contains(err.Error(), "stat source") {
		t.Errorf("expected stat error, got: %v", err)
	}
}
