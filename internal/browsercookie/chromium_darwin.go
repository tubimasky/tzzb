//go:build darwin

package browsercookie

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// 可测试性注入点：单测用 t.TempDir 造目录、用假命令替换 execCommand。
var (
	execCommand = exec.Command
	homeDir     = os.UserHomeDir
)

// browser 描述一个 Chromium 系浏览器在 macOS 上的落盘位置和钥匙串条目。
type browser struct {
	Name     string // 展示名，同时用于 --browser 过滤
	Dir      string // ~/Library/Application Support 下的相对路径
	Keychain string // 钥匙串服务名
	Account  string // 钥匙串账号
}

var knownBrowsers = []browser{
	{Name: "Chrome", Dir: "Google/Chrome", Keychain: "Chrome Safe Storage", Account: "Chrome"},
	{Name: "Edge", Dir: "Microsoft Edge", Keychain: "Microsoft Edge Safe Storage", Account: "Microsoft Edge"},
	{Name: "Brave", Dir: "BraveSoftware/Brave-Browser", Keychain: "Brave Safe Storage", Account: "Brave"},
	{Name: "Chromium", Dir: "Chromium", Keychain: "Chromium Safe Storage", Account: "Chromium"},
}

// Supported 报告当前平台是否支持自动读取浏览器 Cookie。
func Supported() bool { return true }

// SupportedBrowsers 返回可探测的浏览器内核名，供 --browser 的提示使用。
func SupportedBrowsers() []string {
	names := make([]string, 0, len(knownBrowsers))
	for _, b := range knownBrowsers {
		names = append(names, b.Name)
	}
	return names
}

// platformProfiles 扫描所有已知浏览器，返回各自读到的 cookie 行。
//
// 单个 profile 读失败只是跳过（比如库里没有 cookies 表），但两类问题会直接
// 报错，因为它们是可修复的且不报错会让人误以为"没登录"：
//   - 一个浏览器目录都没找到
//   - 所有已安装浏览器的钥匙串条目都读不到（通常是授权被拒）
func platformProfiles(domain string) ([]profileRows, error) {
	home, err := homeDir()
	if err != nil {
		return nil, err
	}

	var (
		out       []profileRows
		installed int
		keyOK     int
		keyErrs   []string
	)
	for _, b := range knownBrowsers {
		root := filepath.Join(home, "Library", "Application Support", filepath.FromSlash(b.Dir))
		profiles := profileDirs(root)
		if len(profiles) == 0 {
			continue
		}
		installed++

		key, err := storageKey(b)
		if err != nil {
			keyErrs = append(keyErrs, fmt.Sprintf("%s: %v", b.Name, err))
			continue
		}
		keyOK++

		for _, profile := range profiles {
			rows, err := readCookieRows(filepath.Join(root, profile, "Cookies"), domain, key)
			if err != nil {
				continue
			}
			out = append(out, profileRows{Browser: b.Name, Profile: profile, Rows: rows})
		}
	}

	switch {
	case installed == 0:
		return nil, errors.New("未找到 Chrome / Edge / Brave / Chromium 的 Cookie 数据库，可能本机从未使用过它们")
	case keyOK == 0:
		return nil, fmt.Errorf("读取钥匙串失败，无法解密 Cookie：%s", strings.Join(keyErrs, "；"))
	}
	return out, nil
}

// profileDirs 列出目录下真正带 Cookie 库的 profile。
// 编号会跳号（本机就是 Default、Profile 1、3、5），所以只能枚举而不能按数字循环。
func profileDirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name != "Default" && !strings.HasPrefix(name, "Profile ") {
			continue // 跳过 System Profile / Guest Profile 等非用户 profile
		}
		if _, err := os.Stat(filepath.Join(root, name, "Cookies")); err != nil {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out) // Default 排在 Profile N 之前
	return out
}

// storageKey 从钥匙串取口令并派生出解密密钥。
func storageKey(b browser) ([]byte, error) {
	out, err := execCommand("security", "find-generic-password", "-w", "-s", b.Keychain, "-a", b.Account).Output()
	if err != nil {
		var exitErr *exec.ExitError
		// security 的退出码 44 是 errSecItemNotFound：条目压根不存在。
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 44 {
			return nil, fmt.Errorf("钥匙串里没有 %q 条目", b.Keychain)
		}
		return nil, errors.New("钥匙串授权被拒绝（首次运行请在弹窗中点“始终允许”）")
	}
	password := strings.TrimSpace(string(out))
	if password == "" {
		return nil, errors.New("钥匙串返回了空口令")
	}
	return deriveKey(password)
}

// sqliteBin 返回 sqlite3 可执行文件路径。
func sqliteBin() (string, error) {
	const systemSQLite = "/usr/bin/sqlite3"
	if _, err := os.Stat(systemSQLite); err == nil {
		return systemSQLite, nil
	}
	path, err := exec.LookPath("sqlite3")
	if err != nil {
		return "", errors.New("系统中找不到 sqlite3 命令，无法读取浏览器 Cookie 数据库")
	}
	return path, nil
}

// readCookieRows 读出指定 Cookie 库里匹配 domain 的所有行。
func readCookieRows(dbPath, domain string, key []byte) ([]cookieRow, error) {
	sqlite, err := sqliteBin()
	if err != nil {
		return nil, err
	}

	// 浏览器运行时会锁住 Cookie 库，而且库可能处于 WAL 模式，所以复制一份
	// （连同 -wal）再读。-shm 不用复制，SQLite 会自己从 WAL 重建。
	tmpDir, err := os.MkdirTemp("", "tzzb-cookies-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	dbCopy := filepath.Join(tmpDir, "Cookies")
	if err := copyFile(dbPath, dbCopy); err != nil {
		return nil, err
	}
	if _, err := os.Stat(dbPath + "-wal"); err == nil {
		_ = copyFile(dbPath+"-wal", dbCopy+"-wal")
	}

	// domain 拼进 LIKE 模式，这里做一次引号转义，避免特殊字符拼坏语句。
	pattern := strings.ReplaceAll(domain, "'", "''")
	query := fmt.Sprintf("select host_key, name, hex(encrypted_value), expires_utc from cookies where host_key like '%%%s';", pattern)

	out, err := execCommand(sqlite, "-list", "-separator", fieldSep, dbCopy, query).Output()
	if err != nil {
		return nil, fmt.Errorf("查询 Cookie 数据库失败: %w", err)
	}
	return parseRows(out, key), nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}
