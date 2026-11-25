package main

import "testing"

func TestRunnerEnableGateRequiresExplicitAC008Acceptance(t *testing.T) {
	for _, tc := range []struct {
		enabled  string
		accepted string
		want     bool
		wantErr  bool
	}{
		{want: false},
		{enabled: "1", wantErr: true},
		{accepted: "1", want: false},
		{enabled: "1", accepted: "1", want: true},
	} {
		got, err := runnerEnabled(tc.enabled, tc.accepted)
		if got != tc.want || (err != nil) != tc.wantErr {
			t.Errorf("enabled=%q accepted=%q got=%v err=%v", tc.enabled, tc.accepted, got, err)
		}
	}
}
