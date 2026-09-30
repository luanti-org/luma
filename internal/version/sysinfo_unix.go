//go:build unix

package version

import (
	"runtime"

	"golang.org/x/sys/unix"
)

// sysinfo mirrors Luanti's porting::get_sysinfo(): "<sysname>/<release> <machine>".
func sysinfo() string {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return runtime.GOOS + " " + runtime.GOARCH
	}
	return unix.ByteSliceToString(u.Sysname[:]) + "/" +
		unix.ByteSliceToString(u.Release[:]) + " " +
		unix.ByteSliceToString(u.Machine[:])
}
