package main

import (
	"os"
	"path"
	"testing"
)

func ok(t testing.TB, err error) {
	if err != nil {
		t.Fatalf("Function call returned an error:\n %s", err)
	}
}

func assertBool(t *testing.T, got, want bool) {
	t.Helper()
	if got != want {
		t.Fatalf("Bool check failed! Got %t, expected %t", got, want)
	}
}

func assertError(t *testing.T, got, want error) {
	t.Helper()
	if want != nil {
		if want.Error() != got.Error() {
			t.Fatalf("Error check failed! Got %t, expected %t", got, want)
		}
	}
}

// func assertPanic(t *testing.T, f func()) {
// 	defer func() {
// 		if recover() == nil {
// 			t.Errorf("The code did not panic")
// 		}
// 	}()
// 	f()
// }

func assertNoPanic(t *testing.T, f func()) {
	defer func() {
		if recover() != nil {
			t.Errorf("The code did panic")
		}
	}()
	f()
}

func preFlightChecksInit(t *testing.T) *os.File {
	// Create fake apparmor binary
	f, err := os.CreateTemp("", "apparmor_parser")
	if err != nil {
		t.Fatalf("failed to create temporary file: %v", err)
	}

	PROFILER_FULL_PATH = path.Join(f.Name())

	// Create fake apparmor config dir
	ETC_APPARMORD = path.Join("profile_test_samples/positive_tests")
	if err := os.MkdirAll(ETC_APPARMORD, 0777); err != nil {
		t.Fatalf("failed to create temporary dir: %v", err)
	}
	CONFIGMAP_PATH = ETC_APPARMORD
	KERNEL_PATH = ETC_APPARMORD
	POLL_TIME_ARG = "3"

	return f
}
