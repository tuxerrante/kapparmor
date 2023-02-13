package main

import (
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
