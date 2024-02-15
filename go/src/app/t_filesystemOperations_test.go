package main

import (
	"os"
	"path"
	"testing"
)

func Test_preFlightChecks(t *testing.T) {

	// Create fake apparmor binary
	f, err := os.CreateTemp("", "apparmor_parser")
	if err != nil {
		t.Fatalf("failed to create temporary file: %v", err)
	}
	defer os.Remove(f.Name())

	PROFILER_FULL_PATH = path.Join(f.Name())

	// Create fake apparmor config dir
	ETC_APPARMORD = path.Join(os.TempDir(), "test_apparmor.d")
	if err := os.Mkdir(ETC_APPARMORD, 0777); err != nil {
		t.Fatalf("failed to create temporary dir: %v", err)
	}
	defer os.Remove(ETC_APPARMORD)

	tests := []struct {
		name, testingPollTime string
		want                  int
	}{
		{
			"Testing with 30",
			"30",
			30,
		},
		{
			"Testing with 0",
			"0",
			1,
		},
		{
			"Testing with negative time delay",
			"-1",
			1,
		},
	}
	for _, tt := range tests {

		POLL_TIME_ARG = tt.testingPollTime

		t.Run(tt.name, func(t *testing.T) {
			if got := preFlightChecks(); got != tt.want {
				t.Errorf("preFlightChecks() = %v, want %v", got, tt.want)
			}
		})
	}
}
