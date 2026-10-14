package main

import "testing"

// TestDeliberateFailure exists only to prove that a pull request whose CI fails
// cannot be merged. It is removed again when the probe is finished.
func TestDeliberateFailure(t *testing.T) {
	t.Fatal("deliberate failure: R01 branch-protection negative probe")
}
