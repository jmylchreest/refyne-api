// Package solver provides CAPTCHA solving interfaces and implementations.
package solver

import (
	"context"
	"time"

	"github.com/jmylchreest/refyne-api/captcha/internal/challenge"
)

// Solver is the interface for CAPTCHA solving services.
type Solver interface {
	// Name returns the solver's name (e.g., "2captcha", "capsolver").
	Name() string

	// CanSolve returns true if this solver can handle the given challenge type.
	CanSolve(challengeType challenge.Type) bool

	// Solve attempts to solve a CAPTCHA challenge.
	Solve(ctx context.Context, params SolveParams) (*SolveResult, error)

	// Cost returns the estimated cost for solving a challenge of the given type.
	Cost(challengeType challenge.Type) float64

	// Balance returns the current account balance (optional, returns -1 if not supported).
	Balance(ctx context.Context) (float64, error)
}

// SolveParams contains parameters for solving a CAPTCHA.
type SolveParams struct {
	// Type is the challenge type to solve.
	Type challenge.Type

	// SiteKey is the CAPTCHA site key (required for Turnstile, hCaptcha, reCAPTCHA).
	SiteKey string

	// PageURL is the URL of the page with the CAPTCHA.
	PageURL string

	// Action is optional action parameter (for Turnstile).
	Action string

	// CData is optional cData parameter (for Turnstile).
	CData string

	// Proxy is optional proxy configuration to use for solving.
	Proxy *ProxyConfig
}

// ProxyConfig contains proxy configuration for CAPTCHA solving.
type ProxyConfig struct {
	Type     string // "http", "socks4", "socks5"
	Host     string
	Port     int
	Username string
	Password string
}

// SolveResult contains the result of a successful CAPTCHA solve.
type SolveResult struct {
	// Token is the solution token to inject into the page.
	Token string

	// Valid is how long the token is valid for.
	Valid time.Duration

	// Cost is the actual cost incurred for this solve.
	Cost float64

	// SolverName is the name of the solver that solved this.
	SolverName string
}

// Chain is a solver that tries multiple solvers in order.
type Chain struct {
	solvers []Solver
}

// NewChain creates a new solver chain.
func NewChain(solvers ...Solver) *Chain {
	return &Chain{solvers: solvers}
}

// Name returns "chain".
func (c *Chain) Name() string {
	return "chain"
}

// CanSolve returns true if any solver in the chain can solve the challenge.
func (c *Chain) CanSolve(challengeType challenge.Type) bool {
	for _, s := range c.solvers {
		if s.CanSolve(challengeType) {
			return true
		}
	}
	return false
}

// Solve tries each solver in order until one succeeds.
func (c *Chain) Solve(ctx context.Context, params SolveParams) (*SolveResult, error) {
	var lastErr error

	for _, s := range c.solvers {
		if !s.CanSolve(params.Type) {
			continue
		}

		result, err := s.Solve(ctx, params)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrNoSolverAvailable
}

// Cost returns the lowest cost among solvers that can solve the challenge.
func (c *Chain) Cost(challengeType challenge.Type) float64 {
	minCost := float64(-1)
	for _, s := range c.solvers {
		if s.CanSolve(challengeType) {
			cost := s.Cost(challengeType)
			if minCost < 0 || cost < minCost {
				minCost = cost
			}
		}
	}
	return minCost
}

// Balance returns the minimum balance across all solvers.
func (c *Chain) Balance(ctx context.Context) (float64, error) {
	minBalance := float64(-1)
	for _, s := range c.solvers {
		balance, err := s.Balance(ctx)
		if err == nil && balance >= 0 {
			if minBalance < 0 || balance < minBalance {
				minBalance = balance
			}
		}
	}
	return minBalance, nil
}

// Errors
var (
	ErrNoSolverAvailable = &SolverError{Message: "no solver available for this challenge type"}
	ErrSolverTimeout     = &SolverError{Message: "solver timeout"}
	ErrSolverFailed      = &SolverError{Message: "solver failed"}
	ErrInsufficientFunds = &SolverError{Message: "insufficient funds"}
)

// SolverError represents a solver error.
type SolverError struct {
	Message string
	Cause   error
}

func (e *SolverError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *SolverError) Unwrap() error {
	return e.Cause
}
