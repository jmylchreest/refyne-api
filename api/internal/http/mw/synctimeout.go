package mw

import (
	"net/http"
	"time"

	"github.com/jmylchreest/refyne-api/internal/constants"
)

// ExtendWriteDeadlineForSyncRequests is middleware that extends the HTTP write deadline
// for long-running synchronous requests (like wait=true on crawl).
// This allows requests to block longer than the server's default WriteTimeout.
// The deadline is extended to constants.MaxSyncWaitTimeout plus a buffer for processing.
func ExtendWriteDeadlineForSyncRequests() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only extend if wait=true is present
			if r.URL.Query().Get("wait") == "true" {
				// Extend deadline to max sync timeout plus 30s buffer for response writing
				rc := http.NewResponseController(w)
				deadline := time.Now().Add(constants.MaxSyncWaitTimeout + 30*time.Second)
				if err := rc.SetWriteDeadline(deadline); err != nil {
					// If we can't extend, log but continue - request may timeout early
					// Some proxies/servers don't support this
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
