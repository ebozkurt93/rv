package main

import "testing"

// TestRunHelpFlagsPrintUsageWithoutShellingOut guards a real bug: "rv -h"
// fell through to runTUI, which passed "-h" straight to `git diff` as a
// diff-spec — showing git's own usage text instead of rv's, and (for
// "--help") git error output. -h/--help/help must be recognized before the
// "anything unknown is a diff spec" fallback.
func TestRunHelpFlagsPrintUsageWithoutShellingOut(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"help"}} {
		if err := run(args); err != nil {
			t.Fatalf("run(%v) = %v, want nil (and rv's own usage printed, not a git error)", args, err)
		}
	}
}
