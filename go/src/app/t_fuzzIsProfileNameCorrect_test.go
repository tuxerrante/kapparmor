package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func FuzzIsProfileNameCorrect(f *testing.F) {

	f.Fuzz(
		func(t *testing.T, directory, filename string) {
			err := IsProfileNameCorrect(directory, filename)
			if err != nil {
				if len(directory) == 0 || len(filename) == 0 {
					return
				}
				if strings.Contains(err.Error(), "no such file or directory") {
					return
				}
				if ok, _ := isValidPath(filepath.Clean(directory)); !ok {
					return
				}
				if ok, _ := isValidFilename(filename); !ok {
					return
				} else {
					t.Fatal("failed test:", err)
				}
			}
		})
}
