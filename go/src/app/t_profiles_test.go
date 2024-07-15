package main

import (
	"errors"
	"testing"
)

/*
		TestIsProfileNameCorrect checks if profile names and filenames match.
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
			name:     "OK_custom.myValidProfile",
			filename: "custom.myValidProfile",
			want:     nil,
		},
		{
			name:     "OK_custom.deny-write-outside-app",
			filename: "custom.deny-write-outside-app",
			want:     nil,
		},
		{
			name:     "OK_..data/custom.deny-write-outside-app",
			filename: "..data/custom.deny-write-outside-app",
			want:     nil,
		},
		{
			name:     "OK_custom.bin.foo",
			filename: "custom.bin.foo",
			want:     nil,
		},
		{
			name:     "OK_custom.linked.profile",
			filename: "custom.linked.profile",
			want:     nil,
		},
		{
			name:     "KO_filename different from profile name",
			filename: "custom.myNotValidProfile",
			want:     errors.New("filename 'custom.myNotValidProfile' and profile name 'myNotValidProfile' seems to be different"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsProfileNameCorrect(testsDirectory, tt.filename)
			assertError(t, got, tt.want)
		})
	}
}
