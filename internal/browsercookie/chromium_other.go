//go:build !darwin

package browsercookie

// 非 macOS 平台暂未实现：Linux/Windows 上 Chromium 的落盘路径和密钥来源都不同
// （Linux 是固定的 "peanuts" 口令，Windows 走 DPAPI），需要各写一套。
// 这里留桩保证交叉编译能过，并给出明确报错而不是静默返回空。

// Supported 报告当前平台是否支持自动读取浏览器 Cookie。
func Supported() bool { return false }

// SupportedBrowsers 返回可探测的浏览器内核名。
func SupportedBrowsers() []string { return nil }

func platformProfiles(string) ([]profileRows, error) {
	return nil, ErrUnsupported
}
