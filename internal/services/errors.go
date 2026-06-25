package services

import "fmt"

// ContractBreakError signals that a core endpoint the CLI depends on no longer
// exists on the backend (typically a 404 after a rotki-core upgrade removed or
// renamed a route). It is fatal: the run cannot do its job, so it aborts loudly
// instead of looping the same 404 once per account.
type ContractBreakError struct {
	Step     string
	Endpoint string
	Err      error
}

func (e *ContractBreakError) Error() string {
	return fmt.Sprintf("endpoint contract break during %s (%s): %v", e.Step, e.Endpoint, e.Err)
}

func (e *ContractBreakError) Unwrap() error { return e.Err }
