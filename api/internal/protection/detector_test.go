package protection

import (
	"net/http"
	"testing"
)

func TestDetector_DetectFromResponse(t *testing.T) {
	d := NewDetector()

	tests := []struct {
		name           string
		statusCode     int
		headers        http.Header
		body           string
		wantDetected   bool
		wantSignal     SignalType
		wantRetryable  bool
	}{
		{
			name:         "normal 200 response",
			statusCode:   200,
			body:         "<html><body><article>This is real content with enough text to pass the minimum length check.</article></body></html>",
			wantDetected: false,
		},
		{
			name:           "403 forbidden",
			statusCode:     403,
			body:           "Forbidden",
			wantDetected:   true,
			wantSignal:     SignalAccessDenied,
			wantRetryable:  true,
		},
		{
			name:           "503 service unavailable",
			statusCode:     503,
			body:           "Service Unavailable",
			wantDetected:   true,
			wantSignal:     SignalCloudflare,
			wantRetryable:  true,
		},
		{
			name:           "429 rate limited",
			statusCode:     429,
			body:           "Too Many Requests",
			wantDetected:   true,
			wantSignal:     SignalRateLimited,
			wantRetryable:  false, // Rate limiting won't be helped by browser rendering
		},
		{
			name:       "cloudflare challenge page",
			statusCode: 200,
			body: `<!DOCTYPE html>
				<html>
				<head><title>Just a moment...</title></head>
				<body>
					<div id="cf-browser-verification">
						Checking your browser before accessing the site.
					</div>
				</body>
				</html>`,
			wantDetected:  true,
			wantSignal:    SignalCloudflare,
			wantRetryable: true,
		},
		{
			name:       "cloudflare attention required",
			statusCode: 200,
			body:       `<title>Attention Required! | Cloudflare</title>`,
			wantDetected:  true,
			wantSignal:    SignalCloudflare,
			wantRetryable: true,
		},
		{
			name:       "recaptcha challenge",
			statusCode: 200,
			body: `<html>
				<body>
					<div class="g-recaptcha" data-sitekey="xxx"></div>
				</body>
			</html>`,
			wantDetected:  true,
			wantSignal:    SignalCaptcha,
			wantRetryable: true,
		},
		{
			name:       "hcaptcha challenge",
			statusCode: 200,
			body: `<html>
				<body>
					<div class="h-captcha" data-sitekey="xxx"></div>
				</body>
			</html>`,
			wantDetected:  true,
			wantSignal:    SignalCaptcha,
			wantRetryable: true,
		},
		{
			name:       "turnstile challenge",
			statusCode: 200,
			body: `<html>
				<body>
					<div class="cf-turnstile" data-sitekey="xxx"></div>
				</body>
			</html>`,
			wantDetected:  true,
			wantSignal:    SignalCaptcha,
			wantRetryable: true,
		},
		{
			name:           "access denied message",
			statusCode:     200,
			body:           `<html><body><h1>Access Denied</h1><p>You don't have permission to access this resource.</p></body></html>`,
			wantDetected:   true,
			wantSignal:     SignalAccessDenied,
			wantRetryable:  true,
		},
		{
			name:           "bot detection message",
			statusCode:     200,
			body:           `<html><body><p>Bot detected. Please verify you are human.</p></body></html>`,
			wantDetected:   true,
			wantSignal:     SignalAccessDenied,
			wantRetryable:  true,
		},
		{
			name:           "javascript required",
			statusCode:     200,
			body:           `<html><body><noscript>Please enable JavaScript to view this page.</noscript></body></html>`,
			wantDetected:   true,
			wantSignal:     SignalJavaScriptRequired,
			wantRetryable:  true,
		},
		{
			name:           "empty response",
			statusCode:     200,
			body:           "",
			wantDetected:   true,
			wantSignal:     SignalEmptyContent,
			wantRetryable:  true,
		},
		{
			name:           "minimal response without content",
			statusCode:     200,
			body:           "<html><head></head><body></body></html>",
			wantDetected:   true,
			wantSignal:     SignalEmptyContent,
			wantRetryable:  true,
		},
		{
			name:       "cloudflare header with challenge",
			statusCode: 200,
			headers: http.Header{
				"Cf-Ray":       []string{"abc123"},
				"Cf-Mitigated": []string{"challenge"},
			},
			body:          "<html><body>Challenge</body></html>",
			wantDetected:  true,
			wantSignal:    SignalCloudflare,
			wantRetryable: true,
		},
		{
			name:       "empty React root element",
			statusCode: 200,
			body:       `<html><head></head><body><div id="root"></div><script src="/app.js"></script></body></html>`,
			wantDetected:  true,
			wantSignal:    SignalJavaScriptRequired,
			wantRetryable: true,
		},
		{
			name:       "empty Next.js root element",
			statusCode: 200,
			body:       `<html><head></head><body><div id="__next"></div></body></html>`,
			wantDetected:  true,
			wantSignal:    SignalJavaScriptRequired,
			wantRetryable: true,
		},
		{
			name:       "navigation-only content like Instructables static",
			statusCode: 200,
			body: `<html><head></head><body>
				<nav><a href="/one">Link</a><a href="/two">Link</a><a href="/three">Link</a>
				<a href="/four">Link</a><a href="/five">Link</a><a href="/six">Link</a></nav>
				<footer>Copyright 2026</footer>
			</body></html>`,
			wantDetected:  true,
			wantSignal:    SignalJavaScriptRequired,
			wantRetryable: true,
		},
		{
			name:       "real content page should not be detected",
			statusCode: 200,
			body: `<html><head></head><body>
				<article>
					<h1>How to Build Something Amazing</h1>
					<p>This is a detailed tutorial about building something. It has lots of content
					that describes the steps involved. First you need to gather materials. Then you
					start assembling the pieces together. This paragraph contains enough text to
					demonstrate that this is a real content page and not just navigation links.
					The minimum threshold is around 500 characters of visible text content.</p>
					<p>Here is more content to ensure we pass the threshold comfortably.</p>
				</article>
			</body></html>`,
			wantDetected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.DetectFromResponse(tt.statusCode, tt.headers, []byte(tt.body))

			if result.Detected != tt.wantDetected {
				t.Errorf("Detected = %v, want %v", result.Detected, tt.wantDetected)
			}
			if tt.wantDetected {
				if result.Signal != tt.wantSignal {
					t.Errorf("Signal = %v, want %v", result.Signal, tt.wantSignal)
				}
				if result.IsRetryable() != tt.wantRetryable {
					t.Errorf("IsRetryable() = %v, want %v", result.IsRetryable(), tt.wantRetryable)
				}
				if result.UserMessage() == "" {
					t.Error("UserMessage() should not be empty when Detected is true")
				}
			}
		})
	}
}

func TestDetectionResult_UserMessage(t *testing.T) {
	tests := []struct {
		signal      SignalType
		wantContains string
	}{
		{SignalCloudflare, "Cloudflare"},
		{SignalCaptcha, "captcha"},
		{SignalAccessDenied, "blocking"},
		{SignalRateLimited, "rate limited"},
		{SignalEmptyContent, "minimal content"},
		{SignalJavaScriptRequired, "JavaScript"},
	}

	for _, tt := range tests {
		t.Run(string(tt.signal), func(t *testing.T) {
			result := DetectionResult{
				Detected: true,
				Signal:   tt.signal,
			}
			msg := result.UserMessage()
			if msg == "" {
				t.Error("UserMessage() should not be empty")
			}
			// Just verify we get a non-empty message for each signal type
		})
	}
}
