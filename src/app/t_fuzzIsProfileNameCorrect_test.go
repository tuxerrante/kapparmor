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
				// --- EXPECTED FAILURES (Success) ---

				// 1. Expected fail for empty strings
				if len(directory) == 0 || len(filename) == 0 {
					// t.Log("Expected fail for empty string")
					return
				}
				// 2. Expected fail if the error is due to a correctly blocked unsafe path
				// This specifically handles the fuzzer finding path traversal/absolute path issues
				if strings.Contains(err.Error(), "unsafe file path detected") {
					return
				}
				// 3. Expected fail for files that don't exist in the test environment
				if strings.Contains(err.Error(), "no such file or directory") {
					// t.Logf("expected error for missing file %q", filename)
					return
				}
				// 4. Expected fail for syntactically invalid filename
				if ok, _ := isValidFilename(filename); !ok {
					// t.Logf("expected error for invalid filename %q", filename)
					return
				}
				// 5. Expected fail for syntactically invalid directory path
				if ok, _ := isValidPath(directory); !ok {
					return
				}

				t.Fatal("Error:", err)
			}
		})
}
