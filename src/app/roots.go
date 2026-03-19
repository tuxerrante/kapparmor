package main

import (
	"bytes"
	"fmt"
	"os"
	"path"
)

// openProfileRoots opens os.Root handles for the configmap volume and the host
// AppArmor custom profile directory. Operations through these roots cannot escape
// the tree via ".." or symlink tricks (see [os.Root]).
func openProfileRoots(cfg *AppConfig) error {
	closeProfileRoots(cfg)

	cr, err := os.OpenRoot(cfg.ConfigmapPath)
	if err != nil {
		return fmt.Errorf("open configmap root %q: %w", cfg.ConfigmapPath, err)
	}

	er, err := os.OpenRoot(cfg.EtcApparmord)
	if err != nil {
		_ = cr.Close()

		return fmt.Errorf("open etc profile root %q: %w", cfg.EtcApparmord, err)
	}

	cfg.ConfigmapRoot = cr
	cfg.EtcRoot = er

	return nil
}

// closeProfileRoots releases root handles (idempotent).
func closeProfileRoots(cfg *AppConfig) {
	if cfg.ConfigmapRoot != nil {
		_ = cfg.ConfigmapRoot.Close()
		cfg.ConfigmapRoot = nil
	}

	if cfg.EtcRoot != nil {
		_ = cfg.EtcRoot.Close()
		cfg.EtcRoot = nil
	}
}

// readProfileBytes reads a profile file by leaf name under root when non-nil,
// otherwise uses basePath/name (tests and fallback).
func readProfileBytes(root *os.Root, basePath, name string) ([]byte, error) {
	if root != nil {
		return root.ReadFile(name)
	}

	// #nosec G304 -- basePath is configured root; name is a validated profile leaf
	return os.ReadFile(path.Join(basePath, name))
}

// profileBytesEqual mirrors HasTheSameContent trimming semantics for comparing
// configmap vs on-disk profile bytes without calling os.Exit on read errors.
func profileBytesEqual(a, b []byte) bool {
	return bytes.Equal(bytes.TrimSpace(a), bytes.TrimSpace(b))
}
