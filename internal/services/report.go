package services

import (
	"fmt"
	"strings"
)

// OpStats counts the per-item outcome of a sync step that loops over many
// accounts or chains (e.g. "EVM fetch: 0 ok / 215 failed").
type OpStats struct {
	Ok     int
	Failed int
}

// Total returns the number of items the step attempted.
func (s OpStats) Total() int { return s.Ok + s.Failed }

// StepReport captures the outcome of a single sync step for one user.
type StepReport struct {
	// Step is a stable, human-readable name (e.g. "EVM fetch").
	Step string
	// Core marks steps whose total failure means the run did not do its job.
	// A core step with zero successes drives a non-zero exit code.
	Core bool
	// Stats holds per-item ok/failed counts for looping steps. Steps that are
	// a single operation leave it zero-valued and rely on Err instead.
	Stats OpStats
	// Err is set when the step could not run at all (setup failure) or aborted.
	Err error
}

// failed reports whether this step should be considered failed for summary and
// exit-code purposes: it errored, or it is a core step that attempted work but
// had zero successes.
func (s StepReport) failed() bool {
	if s.Err != nil {
		return true
	}
	if s.Core && s.Stats.Total() > 0 && s.Stats.Ok == 0 {
		return true
	}
	return false
}

// UserReport aggregates the step outcomes for a single user.
type UserReport struct {
	Username string
	Steps    []StepReport
}

func (u *UserReport) add(step StepReport) {
	u.Steps = append(u.Steps, step)
}

// RunReport aggregates the outcome of an entire sync run across all users.
type RunReport struct {
	Users []UserReport
	// FatalErr is set when a contract break (e.g. a removed endpoint) aborted
	// the run. It is distinct from ordinary per-item failures.
	FatalErr error
}

func (r *RunReport) add(user UserReport) {
	r.Users = append(r.Users, user)
}

// HasFailures reports whether any step in the run failed or the run aborted.
func (r *RunReport) HasFailures() bool {
	if r.FatalErr != nil {
		return true
	}
	for _, user := range r.Users {
		for _, step := range user.Steps {
			if step.failed() {
				return true
			}
		}
	}
	return false
}

// Summary renders a multi-line, human-readable summary of the run suitable for
// logs and alerts.
func (r *RunReport) Summary() string {
	var b strings.Builder
	b.WriteString("Sync run summary:")

	if r.FatalErr != nil {
		fmt.Fprintf(&b, "\n  FATAL: %v", r.FatalErr)
	}

	for _, user := range r.Users {
		fmt.Fprintf(&b, "\n  user %s:", user.Username)
		for _, step := range user.Steps {
			marker := "ok"
			if step.failed() {
				marker = "FAILED"
			}
			switch {
			case step.Err != nil:
				fmt.Fprintf(&b, "\n    [%s] %s: %v", marker, step.Step, step.Err)
			case step.Stats.Total() > 0:
				fmt.Fprintf(&b, "\n    [%s] %s: %d ok / %d failed",
					marker, step.Step, step.Stats.Ok, step.Stats.Failed)
			default:
				fmt.Fprintf(&b, "\n    [%s] %s", marker, step.Step)
			}
		}
	}

	return b.String()
}
