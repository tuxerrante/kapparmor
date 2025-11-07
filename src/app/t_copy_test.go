package main

import (
	"os"
	"path/filepath"
	"testing"
)

func Test_CopyFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "custom.demo")
	dstDir := filepath.Join(dir, "dst")

	err := os.WriteFile(src, []byte("demo"), 0o600)
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
