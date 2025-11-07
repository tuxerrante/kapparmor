package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func Test_isSafePath(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"/app/profiles/custom.some", true},
		{"/etc/apparmor.d/custom", true},
		{"/sys/kernel/security/apparmor/profiles", true},
		{"../../etc/passwd", false},
		{"/root/.ssh/id_rsa", false},
	}
	for _, c := range cases {
		got := isSafePath(c.in)
		if got != c.want {
			t.Fatalf("isSafePath(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func Test_isValidFilename(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"custom.profile", true},
		{"a-b_c", true},
		{"", false},
		{".", false},
		{"..", false},
		{"bad/seg", false},
		{"bad\\seg", false},
		{"__--..", false},
	}
	for _, c := range cases {
		ok, _ := isValidFilename(c.name)
		if ok != c.ok {
			t.Fatalf("isValidFilename(%q) = %v, want %v", c.name, ok, c.ok)
		}
	}
}

func Test_isValidPath(t *testing.T) {
	cases := []struct {
		p  string
		ok bool
	}{
		{".", true},
		{"profiles", true},
		{filepath.Join("profiles", "custom"), true},
		{"", false},
	}
	for _, c := range cases {
		ok, _ := isValidPath(c.p)
		if ok != c.ok {
			t.Fatalf("isValidPath(%q) = %v, want %v", c.p, ok, c.ok)
		}
	}
}

func Test_parseProfileName(t *testing.T) {
	if got := parseProfileName("custom.profile (enforce)"); got != "custom.profile" {
		t.Fatalf("parseProfileName failed, got %q", got)
	}

	if got := parseProfileName("invalid-line"); got != "" {
		t.Fatalf("parseProfileName should be empty, got %q", got)
	}
}

func Test_HasTheSameContent_WithFS(t *testing.T) {
	m := fstest.MapFS{
		"a": &fstest.MapFile{Data: []byte("hello")},
		"b": &fstest.MapFile{Data: []byte("hello")},
		"c": &fstest.MapFile{Data: []byte("world")},
	}

	eq, err := HasTheSameContent(fs.FS(m), "a", "b")
	if err != nil || !eq {
		t.Fatalf("expected equal, got eq=%v err=%v", eq, err)
	}

	eq, err = HasTheSameContent(fs.FS(m), "a", "c")
	if err != nil || eq {
		t.Fatalf("expected not equal, got eq=%v err=%v", eq, err)
	}
}

func Test_getProfilesNamesFromFile_parsing(t *testing.T) {
	tmp := t.TempDir()
	profilesFile := filepath.Join(tmp, "profiles")

	lines := []string{
		"custom.foo (enforce)",
		"ns://custom.bar (complain)",
		"random.baz (enforce)",
		"   ", // whitespace
	}

	if err := os.WriteFile(profilesFile, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatalf("write profiles list: %v", err)
	}

	all, custom, err := getProfilesNamesFromFile(profilesFile, "custom.")
	if err != nil {
		t.Fatalf("getProfilesNamesFromFile: %v", err)
	}

	if !all["custom.foo"] || !all["ns://custom.bar"] || !all["random.baz"] {
		t.Fatalf("missing names in 'all': %#v", all)
	}

	if !custom["custom.foo"] {
		t.Fatalf("missing names in 'custom': %#v", custom)
	}

	if custom["random.baz"] || custom["ns://custom.bar"] {
		t.Fatalf("unexpected non-custom in 'custom', \n\ttesting lines: %#v, \n\tcustom map: %#v", lines, custom)
	}
}
