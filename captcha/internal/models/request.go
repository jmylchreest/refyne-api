// Package models defines API request and response types.
package models

// Command constants for FlareSolverr compatibility.
const (
	CmdSessionsCreate  = "sessions.create"
	CmdSessionsList    = "sessions.list"
	CmdSessionsDestroy = "sessions.destroy"
	CmdRequestGet      = "request.get"
	CmdRequestPost     = "request.post"
)

// Cookie represents an HTTP cookie.
type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
	HTTPOnly bool   `json:"httpOnly,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	SameSite string `json:"sameSite,omitempty"`
}

// ProxyConfig represents proxy configuration.
type ProxyConfig struct {
	URL      string `json:"url"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// WaitCondition specifies what to wait for after navigation.
type WaitCondition struct {
	Selector    string `json:"selector,omitempty"`    // CSS selector to wait for
	Text        string `json:"text,omitempty"`        // Text content to wait for
	Timeout     int    `json:"timeout,omitempty"`     // Custom timeout in ms
	Delay       int    `json:"delay,omitempty"`       // Delay in ms before returning
	NetworkIdle bool   `json:"networkIdle,omitempty"` // Wait for network idle
	Load        bool   `json:"load,omitempty"`        // Wait for page load event
}

// SessionOptions specifies options for creating a browser session.
type SessionOptions struct {
	Headless     *bool        `json:"headless,omitempty"`
	WindowWidth  int          `json:"windowWidth,omitempty"`
	WindowHeight int          `json:"windowHeight,omitempty"`
	UserAgent    string       `json:"userAgent,omitempty"`
	Proxy        *ProxyConfig `json:"proxy,omitempty"`
	UserID       string       `json:"-"` // Set from auth context, not request body
}

// SolveRequest is a FlareSolverr-compatible request.
type SolveRequest struct {
	Cmd            string          `json:"cmd"`                     // "request.get" | "request.post" | "sessions.create" | "sessions.list" | "sessions.destroy"
	URL            string          `json:"url,omitempty"`           // Target URL
	Session        string          `json:"session,omitempty"`       // Session ID for persistent sessions
	SessionTTL     int             `json:"sessionTtl,omitempty"`    // Session TTL in seconds (default 900)
	SessionOptions *SessionOptions `json:"sessionOptions,omitempty"` // Options for session creation
	MaxTimeout     int             `json:"maxTimeout,omitempty"`    // Max timeout in ms (default 60000)
	Cookies        []Cookie        `json:"cookies,omitempty"`       // Cookies to set
	PostData       string          `json:"postData,omitempty"`      // POST body data
	Proxy          *ProxyConfig    `json:"proxy,omitempty"`         // Proxy configuration

	// Refyne extensions
	UserAgent  string            `json:"userAgent,omitempty"`  // Custom user agent
	Headers    map[string]string `json:"headers,omitempty"`    // Custom headers
	WaitFor    *WaitCondition    `json:"waitFor,omitempty"`    // Wait condition
	Screenshot bool              `json:"screenshot,omitempty"` // Capture screenshot
}

// HumaSolveRequest wraps SolveRequest for Huma API.
type HumaSolveRequest struct {
	Body SolveRequest
}
