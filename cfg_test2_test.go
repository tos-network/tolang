package lua

import (
	"testing"
)

// Test that while(true) with { if cond { return } else { continue } } is accepted
// when the function has a named return variable (Solidity implicit return convention).
func TestCFGInfiniteLoopWithContinueBranchAcceptedNamedReturn(t *testing.T) {
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
	if err != nil {
		t.Errorf("expected success (named return allows implicit return), got: %v", err)
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
