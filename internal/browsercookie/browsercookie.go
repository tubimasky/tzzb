// Package browsercookie 从本机浏览器的 cookie 数据库里取出投资账本的登录态，
// 让 tzzb login 不必再依赖手工复制 Cookie。
//
// 只支持 Chromium 系（Chrome / Edge / Brave / Chromium）：它们共用同一套
// cookie 加密方式，差别仅在钥匙串条目名。Firefox 的库格式完全不同，未实现。
package browsercookie

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrUnsupported 表示当前平台没有实现浏览器 cookie 读取。
var ErrUnsupported = errors.New("当前平台暂不支持自动读取浏览器 Cookie")

// requiredFields 是登录态必须具备的字段，缺任意一个就说明该 profile 要么没
// 登录过、要么 cookie 已被清掉，不可能是可用登录态。
var requiredFields = []string{"userid", "ticket", "sess_tk"}

// fieldSep 是 sqlite3 输出各列的分隔符。取 ASCII 的 US（0x1f），
// 正常 cookie 的 host/name 不会包含它，能避免名字里带空格时切错。
const fieldSep = "\x1f"

// chromToUnixSeconds 是 Chromium 时间戳（1601-01-01 起）与 Unix 时间戳的秒差。
const chromToUnixSeconds = 11644473600

// Result 是一个浏览器 profile 的登录态。
type Result struct {
	Cookie  string // 可直接作为 Cookie 头使用的完整串
	Browser string // 浏览器名，如 "Chrome"
	Profile string // profile 目录名，如 "Profile 1"
}

// cookieRow 是从浏览器 cookie 库读出的原始行。
type cookieRow struct {
	Host    string
	Name    string
	Value   string
	Expires int64 // Chromium 时间戳（1601-01-01 起的微秒数），0 表示会话 cookie
}

// profileRows 是某个浏览器 profile 读出的全部原始行。
type profileRows struct {
	Browser string
	Profile string
	Rows    []cookieRow
}

// readProfiles 由平台实现提供，抽成变量便于单测注入假数据。
var readProfiles = platformProfiles

// expired 判断 cookie 是否已过期。会话 cookie（Expires 为 0）不算过期。
func (r cookieRow) expired(now time.Time) bool {
	if r.Expires <= 0 {
		return false
	}
	return r.expiresAt().Before(now)
}

// expiresAt 把 Chromium 时间戳换成 Unix 时间。
//
// 注意不能写成 time.Duration(Expires)*time.Microsecond：1601 年至今的微秒数
// 约 1.3e16，乘完会超出 int64 纳秒的上限直接溢出，所以先降到秒级再换算。
func (r cookieRow) expiresAt() time.Time {
	return time.Unix(r.Expires/1_000_000-chromToUnixSeconds, 0)
}

// hostRank 给 cookie 的 host 打分，越大越优先。登录态落在域 cookie
// （.10jqka.com.cn）上，其次才是具体子域。
func hostRank(host, domain string) int {
	switch {
	case host == "."+domain:
		return 3
	case host == domain:
		return 2
	case strings.HasSuffix(host, "."+domain):
		return 1
	default:
		return 0
	}
}

// better 判断候选 a 是否比已有的 b 更值得保留。
func better(a, b cookieRow, domain string) bool {
	if ra, rb := hostRank(a.Host, domain), hostRank(b.Host, domain); ra != rb {
		return ra > rb
	}
	return a.Expires > b.Expires
}

// buildCookie 把一个 profile 的原始行组装成 Cookie 头串。
// 第二个返回值为 false 表示这个 profile 没有可用登录态。
func buildCookie(rows []cookieRow, domain string, now time.Time) (string, bool) {
	best := make(map[string]cookieRow, len(rows))
	for _, r := range rows {
		if r.Value == "" || r.expired(now) {
			continue
		}
		if old, ok := best[r.Name]; ok && !better(r, old, domain) {
			continue
		}
		best[r.Name] = r
	}

	names := make([]string, 0, len(best))
	for name := range best {
		names = append(names, name)
	}
	// 顺序固定，这样"新旧 cookie 是否同一个"可以直接用字符串比较。
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+best[name].Value)
	}
	cookie := strings.Join(parts, "; ")
	if len(MissingFields(cookie)) > 0 {
		return "", false
	}
	return cookie, true
}

// freshness 用登录票据的过期时间衡量一个 profile 的登录新鲜度，
// 越晚说明越接近最近一次登录。
func freshness(rows []cookieRow) int64 {
	var f int64
	for _, r := range rows {
		if r.Name != "sess_tk" && r.Name != "ticket" {
			continue
		}
		if r.Expires > f {
			f = r.Expires
		}
	}
	return f
}

// Candidates 返回本机所有可用的登录态，最可能有效的排在最前。
// 调用方应当按顺序逐个联网校验，取第一个能通的。
func Candidates(domain string) ([]*Result, error) {
	return candidates(domain, "")
}

// CandidatesFor 同 Candidates，但只读取指定的浏览器内核（大小写不敏感）。
func CandidatesFor(domain, browser string) ([]*Result, error) {
	return candidates(domain, browser)
}

func candidates(domain, browser string) ([]*Result, error) {
	if !Supported() {
		return nil, ErrUnsupported
	}
	profiles, err := readProfiles(domain)
	if err != nil {
		return nil, err
	}

	type scored struct {
		res       *Result
		freshness int64
	}
	var picked []scored
	now := time.Now()
	for _, p := range profiles {
		if browser != "" && !strings.EqualFold(p.Browser, browser) {
			continue
		}
		cookie, ok := buildCookie(p.Rows, domain, now)
		if !ok {
			continue
		}
		picked = append(picked, scored{
			res:       &Result{Cookie: cookie, Browser: p.Browser, Profile: p.Profile},
			freshness: freshness(p.Rows),
		})
	}

	sort.SliceStable(picked, func(i, j int) bool {
		if picked[i].freshness != picked[j].freshness {
			return picked[i].freshness > picked[j].freshness
		}
		if picked[i].res.Browser != picked[j].res.Browser {
			return picked[i].res.Browser < picked[j].res.Browser
		}
		return picked[i].res.Profile < picked[j].res.Profile
	})

	if len(picked) == 0 {
		if browser != "" {
			return nil, fmt.Errorf("在 %s 里没有找到 %s 的有效登录态", browser, domain)
		}
		return nil, fmt.Errorf("在 %d 个浏览器 profile 里都没找到 %s 的有效登录态，请先用浏览器打开 https://tzzb.10jqka.com.cn/pc 登录一次", len(profiles), domain)
	}

	out := make([]*Result, 0, len(picked))
	for _, p := range picked {
		out = append(out, p.res)
	}
	return out, nil
}

// Extract 返回最可能有效的那个登录态。
func Extract(domain string) (*Result, error) {
	all, err := Candidates(domain)
	if err != nil {
		return nil, err
	}
	return all[0], nil
}

// MissingFields 返回 Cookie 串里缺失的关键字段名。空切片表示字段齐全。
func MissingFields(cookie string) []string {
	present := map[string]bool{}
	for _, item := range strings.Split(cookie, ";") {
		kv := strings.SplitN(strings.TrimSpace(item), "=", 2)
		if len(kv) == 2 && kv[1] != "" {
			present[kv[0]] = true
		}
	}
	var missing []string
	for _, name := range requiredFields {
		if !present[name] {
			missing = append(missing, name)
		}
	}
	return missing
}

// parseRows 解析 sqlite3 -list 的输出（host、name、hex(密文)、过期时间四列，
// 以 fieldSep 分隔）。encrypted_value 已用 hex() 转成 ASCII，所以这里不必处理
// 二进制。单条解密失败只跳过这一条，不影响同一 profile 的其它 cookie。
func parseRows(out []byte, key []byte) []cookieRow {
	text := strings.TrimRight(string(out), "\n")
	if text == "" {
		return nil
	}
	var rows []cookieRow
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Split(line, fieldSep)
		if len(fields) != 4 {
			continue
		}
		enc, err := hex.DecodeString(fields[2])
		if err != nil {
			continue
		}
		value, err := decryptValue(key, fields[0], enc)
		if err != nil {
			continue
		}
		expires, _ := strconv.ParseInt(fields[3], 10, 64)
		rows = append(rows, cookieRow{
			Host:    fields[0],
			Name:    fields[1],
			Value:   value,
			Expires: expires,
		})
	}
	return rows
}
