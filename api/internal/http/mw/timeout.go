package mw

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// panicWithStack captures a panic value along with its stack trace.
type panicWithStack struct {
	value interface{}
	stack []byte
}

// TimeoutConfig defines timeout behavior for different path patterns.
type TimeoutConfig struct {
	// Default timeout for most endpoints
	Default time.Duration
	// Extended timeout for long-running operations (LLM calls, etc.)
	Extended time.Duration
	// Patterns that get extended timeout (e.g., "/analyze", "/extract")
	ExtendedPatterns []string
	// Patterns that skip timeout entirely (e.g., "/stream" for SSE)
	SkipPatterns []string
}

// Timeout returns a middleware that applies configurable timeouts to requests.
// - Paths matching SkipPatterns have no timeout (for SSE streaming)
// - Paths matching ExtendedPatterns get the Extended timeout (for LLM operations)
// - All other paths get the Default timeout
func Timeout(cfg TimeoutConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if this path should skip timeout entirely (SSE streaming)
			for _, pattern := range cfg.SkipPatterns {
				if strings.Contains(r.URL.Path, pattern) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Determine timeout: extended for LLM operations, default otherwise
			timeout := cfg.Default
			for _, pattern := range cfg.ExtendedPatterns {
				if strings.Contains(r.URL.Path, pattern) {
					timeout = cfg.Extended
					break
				}
			}

			// Apply timeout
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			// Create a channel to signal completion
			done := make(chan struct{})
			panicChan := make(chan *panicWithStack, 1)

			go func() {
				defer func() {
					if p := recover(); p != nil {
						// Capture the stack trace at the point of panic
						panicChan <- &panicWithStack{
							value: p,
							stack: debug.Stack(),
						}
					}
				}()
				next.ServeHTTP(w, r.WithContext(ctx))
				close(done)
			}()

			select {
			case <-done:
				return
			case p := <-panicChan:
				// Re-panic with the original value and stack trace for proper error reporting
				panic(fmt.Sprintf("%v\n\nOriginal stack trace:\n%s", p.value, p.stack))
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					w.WriteHeader(http.StatusGatewayTimeout)
					return
				}
			}
		})
	}
}
