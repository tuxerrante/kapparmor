package main

import (
	"context"
	"testing"
	"time"
)

func TestPollProfiles(t *testing.T) {

	t.Run("pollProfiles()", func(t *testing.T) {

		pollTimeSeconds := 2
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(pollTimeSeconds*2))
		keepItRunning := make(chan struct{})

		// Send closing signal after 'polltime' seconds
		go func() {
			time.Sleep(time.Duration(pollTimeSeconds))
			cancel()
		}()

		pollProfiles(pollTimeSeconds, ctx, keepItRunning)
	})
}
