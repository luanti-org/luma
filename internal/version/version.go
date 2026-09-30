// Package version reports luma's own version and the User-Agent it sends
package version

import (
	"runtime/debug"
)

// Version is set at release build time via -ldflags "-X .../internal/version.Version=0.1.0".
var Version = "dev"

// String returns Version, else the module version recorded by `go install ...@vX.Y.Z`.
func String() string {
	if Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return trimV(v)
		}
	}
	return Version
}

// UserAgent returns e.g. "Luma/0.1.0 (Linux/6.8.0 x86_64) Luanti/5.17.0"; engineVersion may be empty.
func UserAgent(engineVersion string) string {
	ua := "Luma/" + String() + " (" + sysinfo() + ")"
	if engineVersion != "" {
		ua += " Luanti/" + engineVersion
	}
	return ua
}

func trimV(v string) string {
	if len(v) > 1 && v[0] == 'v' {
		return v[1:]
	}
	return v
}
