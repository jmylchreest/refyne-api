// Package mw provides HTTP middleware for the Refyne API.
package mw

import (
	"net/http"

	"github.com/jmylchreest/refyne-api/internal/version"
)

// APIVersion returns middleware that adds the X-API-Version header to all responses.
// This allows SDK clients to check compatibility with the API version.
func APIVersion() func(http.Handler) http.Handler {
	// Get version once at middleware creation time
	apiVersion := version.Get().Short()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-API-Version", apiVersion)
			next.ServeHTTP(w, r)
		})
	}
}
