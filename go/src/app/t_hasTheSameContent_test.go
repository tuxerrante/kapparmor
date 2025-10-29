package main

import (
	"testing"
	"testing/fstest"
)

const (
	testProfileData = `
#include <tunables/global>

# a comment naming the application to confine
/usr/bin/foo {
#include <abstractions/base>

link /etc/sysconfig/foo -> /etc/foo.conf,
}`

	testProfileDataExtraNewLine = `
#include <tunables/global>

# a comment naming the application to confine
/usr/bin/foo {
#include <abstractions/base>

link /etc/sysconfig/foo -> /etc/foo.conf,
}
`
	testProfileDataDifferent = `
#include <tunables/global>

# a comment naming the application to confine
/usr/bin/bar {
#include <abstractions/base>

link /etc/sysconfig/foo -> /etc/foo.conf,
}
`
)

func TestHasTheSameContent(t *testing.T) {
	fs := fstest.MapFS{
		"foo.profile":         {Data: []byte(testProfileData)},
		"foo.profile.copy":    {Data: []byte(testProfileData)},
		"foo.newline.profile": {Data: []byte(testProfileDataExtraNewLine)},
		"bar.profile":         {Data: []byte(testProfileDataDifferent)},
	}

	t.Parallel()

	t.Run("Two profiles with same content", func(t *testing.T) {
		got, err := HasTheSameContent(fs, "foo.profile", "foo.profile.copy")
		want := true
		ok(t, err)
		assertBool(t, got, want)
	})

	t.Run("A profile with an extra newline", func(t *testing.T) {
		// We don't forgive newlines
		got, err := HasTheSameContent(fs, "foo.profile", "foo.newline.profile")
		want := false
		ok(t, err)
		assertBool(t, got, want)
	})

	t.Run("Two different profiles", func(t *testing.T) {
		// Very different profiles
		got, err := HasTheSameContent(fs, "foo.profile", "bar.profile")
		want := false
		ok(t, err)
		assertBool(t, got, want)
	})
}
