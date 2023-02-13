package main

import (
	"errors"
	"log"
	"os"
	"path"
	"testing"
)

const directory string = "."

/*
		TestIsProfileNameCorrect checks if profile names and filenames match
		The name must not begin with a : or . character.
	 	If it contains a whitespace, it must be quoted.
		If the name begins with a /, the profile is considered to be a standard profile,
		https://documentation.suse.com/sles/15-SP1/html/SLES-all/cha-apparmor-profiles.html#sec-apparmor-profiles-types-unattached
*/
func TestIsProfileNameCorrect(t *testing.T) {

	t.Parallel()

	// table-driven tests
	var tests = []struct {
		name, filename, profileFirstLine string
		want                             error
	}{
		{
			name:             "Confirm a filename equal as profile name is OK",
			filename:         "custom.myValidProfile",
			profileFirstLine: "profile custom.myValidProfile flags=(attach_disconnected) {",
			want:             nil,
		},
		{
			name:             "Deny a filename different from profile name",
			filename:         "custom.myNotValidProfile",
			profileFirstLine: "profile myNotValidProfile flags=(attach_disconnected) {",
			want:             errors.New("filename 'custom.myNotValidProfile' and profile name 'myNotValidProfile' seems to be different"),
		},
		{
			name:             "Deny a filename different from profile name",
			filename:         "custom.myNotValidProfile.bkp",
			profileFirstLine: "profile custom.myNotValidProfile flags=(attach_disconnected) {",
			want:             errors.New("filename 'custom.myNotValidProfile.bkp' and profile name 'custom.myNotValidProfile' seems to be different"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.WriteFile(path.Join(directory, tt.filename), []byte(tt.profileFirstLine), 0666)
			if err != nil {
				log.Fatal(err)
			}
			defer os.Remove(path.Join(directory, tt.filename))

			got := IsProfileNameCorrect(directory, tt.filename)
			assertError(t, got, tt.want)
		})
	}
}
