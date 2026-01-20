package solver

import (
	"context"
	"time"

	"github.com/jmylchreest/refyne-api/captcha/internal/challenge"
)

// WaitSolver implements the Solver interface for auto-resolving challenges.
// This solver waits for challenges that resolve themselves (e.g., Cloudflare's
// JavaScript challenge) without requiring external CAPTCHA solving services.
type WaitSolver struct {
	detector *challenge.Detector
	timeout  time.Duration
}

// NewWaitSolver creates a new wait solver.
func NewWaitSolver(detector *challenge.Detector, timeout time.Duration) *WaitSolver {
	return &WaitSolver{
		detector: detector,
		timeout:  timeout,
	}
}

// Name returns "wait".
func (w *WaitSolver) Name() string {
	return "wait"
}

// CanSolve returns true for challenge types that can auto-resolve.
func (w *WaitSolver) CanSolve(challengeType challenge.Type) bool {
	switch challengeType {
	case challenge.TypeCloudflareJS,
		challenge.TypeCloudflareInterstitial,
		challenge.TypeDDoSGuard:
		return true
	default:
		return false
	}
}

// Solve waits for the challenge to auto-resolve.
// Note: This solver requires the Page field to be set in SolveParams.
func (w *WaitSolver) Solve(ctx context.Context, params SolveParams) (*SolveResult, error) {
	if params.Page == nil {
		return nil, &SolverError{Message: "wait solver requires page"}
	}

	timeout := w.timeout
	if params.Timeout > 0 {
		timeout = params.Timeout
	}

	if err := w.detector.WaitForChallenge(ctx, params.Page, timeout); err != nil {
		return nil, &SolverError{Message: "challenge timeout", Cause: err}
	}

	return &SolveResult{
		Token:      "", // No token for wait-based solves
		Valid:      0,
		Cost:       0, // No cost for waiting
		SolverName: w.Name(),
	}, nil
}

// Cost returns 0 as waiting is free.
func (w *WaitSolver) Cost(challengeType challenge.Type) float64 {
	return 0
}

// Balance returns -1 as not applicable.
func (w *WaitSolver) Balance(ctx context.Context) (float64, error) {
	return -1, nil
}
