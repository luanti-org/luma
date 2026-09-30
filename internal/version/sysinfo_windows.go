//go:build windows

package version

import (
	"fmt"
	"runtime"

	"golang.org/x/sys/windows"
)

// sysinfo mirrors Luanti's porting::get_sysinfo(): "Windows/<major>.<minor>.<build> <arch>".
func sysinfo() string {
	v := windows.RtlGetVersion()
	return fmt.Sprintf("Windows/%d.%d.%d %s", v.MajorVersion, v.MinorVersion, v.BuildNumber, arch())
}

func arch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "386":
		return "x86"
	default:
		return runtime.GOARCH
	}
}
