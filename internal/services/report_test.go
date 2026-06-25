package services

import (
	"errors"
	"strings"
	"testing"
)

func TestRunReportHasFailures(t *testing.T) {
	tests := []struct {
		name   string
		report RunReport
		want   bool
	}{
		{
			name: "all steps ok",
			report: RunReport{Users: []UserReport{{
				Username: "alice",
				Steps: []StepReport{
					{Step: "EVM transaction fetch", Core: true, Stats: OpStats{Ok: 10}},
					{Step: "token detection", Stats: OpStats{Ok: 3}},
				},
			}}},
			want: false,
		},
		{
			name: "core step with zero successes fails",
			report: RunReport{Users: []UserReport{{
				Username: "alice",
				Steps: []StepReport{
					{Step: "EVM transaction fetch", Core: true, Stats: OpStats{Failed: 215}},
				},
			}}},
			want: true,
		},
		{
			name: "non-core step with zero successes does not fail the run",
			report: RunReport{Users: []UserReport{{
				Username: "alice",
				Steps: []StepReport{
					{Step: "online events fetch", Core: false, Stats: OpStats{Failed: 2}},
				},
			}}},
			want: false,
		},
		{
			name: "step error fails",
			report: RunReport{Users: []UserReport{{
				Username: "alice",
				Steps: []StepReport{
					{Step: "balance snapshot", Err: errors.New("boom")},
				},
			}}},
			want: true,
		},
		{
			name:   "fatal error fails",
			report: RunReport{FatalErr: errors.New("contract break")},
			want:   true,
		},
		{
			name: "core step with no work attempted is not a failure",
			report: RunReport{Users: []UserReport{{
				Username: "alice",
				Steps: []StepReport{
					{Step: "EVM transaction fetch", Core: true, Stats: OpStats{}},
				},
			}}},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.report.HasFailures(); got != tc.want {
				t.Errorf("HasFailures() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRunReportSummary(t *testing.T) {
	report := RunReport{Users: []UserReport{{
		Username: "alice",
		Steps: []StepReport{
			{Step: "EVM transaction fetch", Core: true, Stats: OpStats{Failed: 215}},
			{Step: "token detection", Stats: OpStats{Ok: 3}},
		},
	}}}

	summary := report.Summary()
	if !strings.Contains(summary, "alice") {
		t.Errorf("summary should name the user: %q", summary)
	}
	if !strings.Contains(summary, "0 ok / 215 failed") {
		t.Errorf("summary should report counts: %q", summary)
	}
	if !strings.Contains(summary, "FAILED") {
		t.Errorf("summary should mark the failed step: %q", summary)
	}
}
