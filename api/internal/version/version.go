// Package version provides build-time version information.
// These variables are set at build time using ldflags:
//
//	go build -ldflags "-X github.com/jmylchreest/refyne-api/internal/version.Version=1.0.0 ..."
package version

import (
	"fmt"
	"runtime"
)

// Build-time variables set via ldflags
var (
	// Version is the semantic version (e.g., "1.0.0")
	Version = "0.0.0-dev"

	// Commit is the git commit SHA
	Commit = "unknown"

	// Date is the build date in RFC3339 format
	Date = "unknown"

	// Dirty indicates if the git tree was dirty at build time
	Dirty = "false"
)

// Info holds all version information
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	Dirty     bool   `json:"dirty"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// Get returns the version info
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		Date:      Date,
		Dirty:     Dirty == "true",
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns a human-readable version string
func (i Info) String() string {
	dirty := ""
	if i.Dirty {
		dirty = "-dirty"
	}
	return fmt.Sprintf("%s (%s%s) built %s", i.Version, i.Commit, dirty, i.Date)
}

// Short returns a short version string (version only)
func (i Info) Short() string {
	if i.Dirty {
		return i.Version + "-dirty"
	}
	return i.Version
}
