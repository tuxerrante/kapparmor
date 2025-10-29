package main

import (
	"errors"
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
