//go:build darwin

package termimg

// ioctlTIOCGWINSZ 是 macOS 上读取终端窗口大小的 ioctl 号。
const ioctlTIOCGWINSZ = 0x40087468
