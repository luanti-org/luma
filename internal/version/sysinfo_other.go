//go:build !unix && !windows

package version

import "runtime"

func sysinfo() string {
	return runtime.GOOS + " " + runtime.GOARCH
}
