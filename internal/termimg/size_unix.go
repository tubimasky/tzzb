//go:build linux || darwin

package termimg

import (
	"os"
	"syscall"
	"unsafe"
)

type winsize struct {
	row, col, xpixel, ypixel uint16
}

// terminalSize 用 ioctl(TIOCGWINSZ) 读终端实际列数/行数；失败返回 (0, 0)。
func terminalSize() (int, int) {
	for _, fd := range []uintptr{os.Stdout.Fd(), os.Stderr.Fd()} {
		var ws winsize
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(ioctlTIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
		if errno == 0 && ws.col >= 20 {
			return int(ws.col), int(ws.row)
		}
	}
	return 0, 0
}
