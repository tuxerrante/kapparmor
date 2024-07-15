package main

import (
	"reflect"
	"testing"
)

func Test_getNewProfiles(t *testing.T) {

	CONFIGMAP_PATH = "profile_test_samples/positive_tests"

	tests := []struct {
		name  string
		want  bool
		want1 map[string]bool
	}{
		{
			"1_test",
			true,
			map[string]bool{
				"custom.deny-network":           true,
				"custom.deny-write-outside-app": true,
				"custom.linked":                 true,
				"custom.myValidProfile":         true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := getNewProfiles()
			if got != tt.want {
				t.Errorf("getNewProfiles() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("getNewProfiles() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
