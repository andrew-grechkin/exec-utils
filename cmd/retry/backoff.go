package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Pick the recurrence used to compute the delay after each failure. Numeric multipliers passed via -b turn into
// BackoffExponential (or BackoffFixed when the multiplier is exactly 1).
type BackoffKind int

const (
	BackoffFixed BackoffKind = iota
	BackoffLinear
	BackoffExponential
	BackoffFibonacci
)

type BackoffSpec struct {
	Kind   BackoffKind
	Factor float64
}

// Turn the -b argument into a BackoffSpec. Accept either a strategy name (fixed / linear / fib | fibonacci) or a
// numeric multiplier (1 -> fixed; 2 -> classic exponential doubling; any factor > 1 works).
func ParseBackoff(s string) (BackoffSpec, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "fixed":
		return BackoffSpec{Kind: BackoffFixed}, nil
	case "linear", "lin":
		return BackoffSpec{Kind: BackoffLinear}, nil
	case "fib", "fibonacci":
		return BackoffSpec{Kind: BackoffFibonacci}, nil
	}

	factor, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return BackoffSpec{}, fmt.Errorf("invalid backoff %q: not a strategy name or number", s)
	}
	if factor < 1 {
		return BackoffSpec{}, fmt.Errorf("invalid backoff factor %v: must be >= 1", factor)
	}
	if factor == 1 {
		return BackoffSpec{Kind: BackoffFixed}, nil
	}

	return BackoffSpec{Kind: BackoffExponential, Factor: factor}, nil
}

// Return the pause to wait after attempt number `n` fails, before attempt n+1. Attempts are 1-indexed. The base
// duration seeds all four strategies.
func (s BackoffSpec) Delay(attempt int, base time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	switch s.Kind {
	case BackoffFixed:
		return base
	case BackoffLinear:
		// d, 2d, 3d, 4d, ...
		return base * time.Duration(attempt)
	case BackoffExponential:
		// d, factor*d, factor^2*d, ...
		return time.Duration(float64(base) * math.Pow(s.Factor, float64(attempt-1)))
	case BackoffFibonacci:
		// d, d, 2d, 3d, 5d, 8d, ... (fib(1)=1, fib(2)=1, fib(3)=2, ...)
		return base * time.Duration(fib(attempt))
	}

	return base
}

// Return the n-th Fibonacci number (fib(1)=1, fib(2)=1, fib(3)=2, ...). Growth is ~1.618x per step, so overflow into
// negative time.Duration is a concern beyond ~n=90 -- callers cap with --max-delay.
func fib(n int) int64 {
	if n <= 0 {
		return 0
	}

	var a, b int64 = 0, 1
	for range n {
		a, b = b, a+b
	}

	return a
}
