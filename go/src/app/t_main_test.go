package main

import (
	"os"
	"testing"
	"time"
)

func Test_main(t *testing.T) {
	// Check for AppArmor presence (skip if not available)
	if _, err := os.Stat("/sys/kernel/security/apparmor"); os.IsNotExist(err) {
		t.Skip("\n\n====================\nWARNING: AppArmor is not available in this environment.\nSome tests were skipped.\nRun tests in a real Linux VM with AppArmor enabled for full coverage.\n====================\n")
	}

	if err := os.Setenv("TESTING", "true"); err != nil {
		t.Fatalf("Failed to set TESTING environment variable: %v", err)
	}

	tests := []struct {
		name string
	}{
		{"Simple invocation"},
	}

	f := preFlightChecksInit(t)
	defer func() {
		if err := os.Remove(f.Name()); err != nil {
			t.Logf("failed to remove temp file: %v", err)
		}
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test context cancellation: run main in a goroutine and cancel after a short delay
			done := make(chan struct{})
			go func() {
				assertDontPanic(t, main)
				close(done)
			}()
			select {
			case <-done:
				// main exited normally
			case <-time.After(2 * time.Second):
				// Simulate signal/cancel after 2s
				signals <- os.Interrupt
				<-done
			}
		})
	}
}
