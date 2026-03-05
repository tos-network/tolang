package lua

import (
	"testing"
	"strings"
)

// Test that while(true) with { if cond { return } else { continue } } is rejected
// (because the "else continue" branch doesn't guarantee return)
func TestCFGInfiniteLoopWithContinueBranchRejected(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract Demo {
  function f(u256 x) public pure returns (u256 r) {
    while (true) {
      if (x > 0) {
        return x;
      } else {
        continue;
      }
    }
  }
}
`)
	_, err := BuildIR(src, "<tol>")
	if err == nil {
		t.Errorf("expected TOL2017 (missing return path), got success - this is the known false-negative")
	} else if !strings.Contains(err.Error(), "TOL2017") {
		t.Errorf("expected TOL2017, got: %v", err)
	}
}

// Test that while(true) + { if cond { return } } + revert after is accepted
func TestCFGInfiniteLoopWithExplicitRevertAfter(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract Demo {
  function f(u256 x) public pure returns (u256 r) {
    while (true) {
      if (x > 0) {
        return x;
      }
    }
    revert("unreachable");
  }
}
`)
	_, err := BuildIR(src, "<tol>")
	if err != nil {
		t.Errorf("expected success, got: %v", err)
	}
}
