package main

import (
	"context"
	"os"
	"testing"
	"time"
)

func Test_main_RunApp_StartsAndStops(t *testing.T) {
	// Skip if AppArmor not present
	if _, err := os.Stat("/sys/module/apparmor/parameters/enabled"); os.IsNotExist(err) {
		t.Skip(`\n\n====================\n
		WARNING: AppArmor is not available in this environment.\n
		Some tests were skipped.\n
		Run tests in a real Linux VM with AppArmor enabled for full coverage.\n
		====================\n`)
	}

	t.Setenv("TESTING", "true")
	cfg, f := preFlightChecksInit(t)
	defer func() {
		_ = os.Remove(f.Name())
	}()

	done := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		assertDontPanic(t, func() { _ = RunApp(ctx, cfg) })
		close(done)
	}()

	// Allow bootstrap then send a stop
	select {
	case <-time.After(750 * time.Millisecond):
		// fall-through
	case <-done:
		// Process ended. OK.
	}

	// If process is still running, send interrupt signal
	select {
	case <-done:
		// ok
	default:
		// Send SIGINT to stop the app
		cancel()

		// Wait for process to end or timeout
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("RunApp did not stop on SIGINT")
		}
	}
}
