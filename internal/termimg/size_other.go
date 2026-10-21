//go:build !linux && !darwin

package termimg

// terminalSize 在 Windows 等平台暂不探测，退回 $COLUMNS 或 80。
func terminalSize() (int, int) { return 0, 0 }
