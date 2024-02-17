package main

import (
	"os"
	"testing"
)

func Test_main(t *testing.T) {

	os.Setenv("TESTING", "true")

	tests := []struct {
		name string
	}{
		{"Simple invocation"},
	}

	f := preFlightChecksInit(t)
	defer os.Remove(f.Name())

	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			assertDontPanic(t, main)
		})
	}
}
