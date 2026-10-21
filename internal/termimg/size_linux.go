//go:build linux

package termimg

// ioctlTIOCGWINSZ 是 Linux 上读取终端窗口大小的 ioctl 号。
const ioctlTIOCGWINSZ = 0x5413
