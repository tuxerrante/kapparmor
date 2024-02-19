package main

import (
	"strings"
	"testing"
)

func FuzzIsProfileNameCorrect(f *testing.F) {

	f.Fuzz(
		func(t *testing.T, directory, filename string) {
			err := IsProfileNameCorrect(directory, filename)
			if err != nil {
				if len(directory) == 0 || len(filename) == 0 {
					//t.Log("Expected fail for empty string")
					return
				}
				if strings.Contains(err.Error(), "no such file or directory") {
					//t.Logf("expected error for missing file %q", filename)
					return
				}
				if ok, _ := isValidFilename(filename); !ok {
					//t.Logf("expected error for invalid filename %q", filename)
					return
				}
				if ok, _ := isValidPath(directory); !ok {
					return
				} else {
					t.Fatal("Error:", err)
				}
			}
		})
}
