package main

import (
	"math"
	"os"
	"strconv"
	"testing"
)

// https://goplay.tools/snippet/LwP6ggjUkwi
func Test_preFlightChecks(t *testing.T) {
	f := preFlightChecksInit(t)
	defer func() {
		if err := os.Remove(f.Name()); err != nil {
			t.Log(err)
		}
	}()

	tests := []struct {
		name, testingPollTime string
		want                  int
	}{
		{
			"Test case: 30",
			"30",
			30,
		},
		{
			"Test case: 0",
			"0",
			1,
		},
		{
			"Test case: negative time delay",
			"-1",
			1,
		},
		{
			"Test case: symbols",
			":)",
			0,
		},
		{
			"Test case: MaxInt64",
			strconv.Itoa(math.MaxInt64),
			0,
		},
		{
			"Test case: MaxInt32",
			strconv.Itoa(math.MaxInt32),
			0,
		},
		{
			"Test case: MaxInt16",
			strconv.Itoa(math.MaxInt16),
			0,
		},
	}
	for _, tt := range tests {
		PollTimeArg = tt.testingPollTime
		PollTimeArgInt, errAtoi := strconv.Atoi(PollTimeArg)

		t.Run(tt.name, func(t *testing.T) {
			if got, err := preFlightChecks(); got != tt.want {
				// Input can't be converted to an integer
				if errAtoi != nil {
					return
				}

				if err != nil {
					// Expected error for invalid input
					if got == 0 {
						return
						// input out of range
					} else if PollTimeArgInt > MaxAllowedPollingTime {
						return
					}

					t.Errorf("preFlightChecks() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
