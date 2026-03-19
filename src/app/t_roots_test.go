package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileBytesEqual(t *testing.T) {
	t.Parallel()

	if !profileBytesEqual([]byte("  a\n"), []byte("a\n  ")) {
		t.Error("expected trimmed equality")
	}
	if profileBytesEqual([]byte("a"), []byte("b")) {
		t.Error("expected inequality")
	}
}

func TestReadProfileBytes_viaRootRejectsDotDot(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "safe.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := os.OpenRoot(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if _, err := readProfileBytes(r, tmp, ".."); err == nil {
		t.Fatal("expected error for path outside root")
	}
}

func TestOpenProfileRoots(t *testing.T) {
	tmp := t.TempDir()
	cm := filepath.Join(tmp, "cm")
	etc := filepath.Join(tmp, "etc")
	_ = os.MkdirAll(cm, 0o755)
	_ = os.MkdirAll(etc, 0o755)

	cfg := &AppConfig{ConfigmapPath: cm, EtcApparmord: etc}
	if err := openProfileRoots(cfg); err != nil {
		t.Fatal(err)
	}
	defer closeProfileRoots(cfg)

	if _, err := readProfileBytes(cfg.ConfigmapRoot, cfg.ConfigmapPath, "x"); err == nil {
		t.Fatal("expected error for missing file")
	}
}
