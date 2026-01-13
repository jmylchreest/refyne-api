package mw

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// TimeoutWithSkip returns a middleware that applies timeout to requests,
// except for paths that match the skip patterns (e.g., SSE streaming endpoints).
func TimeoutWithSkip(timeout time.Duration, skipPatterns ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if this path should skip timeout
			for _, pattern := range skipPatterns {
				if strings.Contains(r.URL.Path, pattern) {
					// No timeout for this path
					next.ServeHTTP(w, r)
					return
				}
			}

			// Apply timeout for other paths
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			// Create a channel to signal completion
			done := make(chan struct{})
			panicChan := make(chan interface{}, 1)

			go func() {
				defer func() {
					if p := recover(); p != nil {
						panicChan <- p
					}
				}()
				next.ServeHTTP(w, r.WithContext(ctx))
				close(done)
			}()

			select {
			case <-done:
				return
			case p := <-panicChan:
				panic(p)
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					w.WriteHeader(http.StatusGatewayTimeout)
					return
				}
			}
		})
	}
}
