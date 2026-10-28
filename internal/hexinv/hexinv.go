// Package hexinv 生成/刷新同花顺的反爬 token（Cookie 的 v / hexin-v 头）。
//
// # 背景
//
// 上游接口要求每个请求携带一枚**新的**反爬 token，否则会被 WAF 拒绝（403，
// 页面是 <h1>Nginx forbidden.</h1>）。实测：同一个 token 复用约 8~10 次后开始
// 403；每请求换新 token 可以零延迟连发几十个请求全部 200 —— 关键不是 IP、
// 频率或浏览器指纹，而是 token 不能复用。
//
// 2026-10-28 抓包确认官方 PC 页面的真实行为：chameleon 脚本每个请求生成一枚
// 新 token 并**写回 Cookie 的 v**（页面不发 hexin-v 请求头；v 是页面里执行
// document.cookie=... 写入的）。服务端两种都接受，关键是"每请求一枚新 token"。
//
// # 两条路径
//
//  1. RefreshCookie / DecodeV / EncodeV：数学刷新（推荐，主路径）。token 是
//     60 字符 URL-safe base64、解码后 45 字节的 v3 结构，payload 里只有时间戳
//     （[8:12] 大端秒）和计数器（[40:42] 大端 uint16）会变，其余是会话/设备
//     模板。解码 → 改时间、计数 → 重算校验位 → 重编码即可，不用跑 JS、不用联网。
//  2. Generator：执行官方 chameleon 脚本现算（兜底）。脚本用 go:embed 内置，
//     用纯 Go 的 JS 引擎 goja + 最小 DOM 垫片跑起来，不依赖 Chromium / Node。
//
// # 用法
//
//	// 主路径：刷新 Cookie 里已有的 v
//	newCookies, tok, ok := hexinv.RefreshCookie(cookies, time.Now())
//	if !ok {
//		// 兜底：没有可识别的 v 时生成全新 token 并写回 Cookie
//		tok, err = g.Token()
//		newCookies, _ = hexinv.SetCookieV(cookies, tok)
//	}
//
// 一个 Generator 内部串行工作（goja 运行时非并发安全），*Generator 可并发调用。
package hexinv

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

//go:embed chameleon.min.js
var embeddedScript string

const (
	// ScriptURLTemplate 是官方 chameleon 脚本的地址模板。
	// %d 为版本号，约等于 Unix 秒 / 1000（每 ~17 分钟滚动一次），
	// 旧版本文件在 CDN 上仍可访问。
	ScriptURLTemplate = "https://s.thsi.cn/js/chameleon/chameleon.min.%d.js"

	// tokenLen 是 hexin-v token 的固定长度（URL-safe base64，60 字符）。
	tokenLen = 60

	// triggerURL 只用于触发脚本 hook，不会真正被访问。
	triggerURL = "https://tzzb.10jqka.com.cn/caishen_httpserver/tzzb/__token"

	// cacheTTL 是本地缓存脚本的保鲜期。脚本里的 TOKEN_SERVER_TIME 只影响
	// 指纹里的时间字段，服务端容忍度较长，所以没必要每次都联网刷新。
	cacheTTL = 30 * time.Minute

	// fetchTimeout 是单次脚本下载超时。刷新失败要能快速退回内置副本。
	fetchTimeout = 5 * time.Second
)

// Generator 用嵌入式 chameleon 脚本生成 hexin-v token。
type Generator struct {
	mu     sync.Mutex
	vm     *goja.Runtime
	script string // 实际使用的脚本源码（便于排查版本问题）
}

// New 用内置的 chameleon 脚本创建生成器（不联网）。
func New() (*Generator, error) {
	return NewWithScript(embeddedScript)
}

// NewWithScript 用指定的脚本源码创建生成器。
func NewWithScript(src string) (*Generator, error) {
	if strings.TrimSpace(src) == "" {
		return nil, fmt.Errorf("chameleon 脚本为空")
	}
	vm := goja.New()
	if _, err := vm.RunString(shimsJS); err != nil {
		return nil, fmt.Errorf("初始化浏览器环境失败: %w", err)
	}
	if _, err := vm.RunString(src); err != nil {
		return nil, fmt.Errorf("加载 chameleon 脚本失败: %w", err)
	}
	g := &Generator{vm: vm, script: src}
	// 立即验证一次，避免把坏脚本留到第一个真实请求才暴露。
	if _, err := g.Token(); err != nil {
		return nil, err
	}
	return g, nil
}

// Load 按以下顺序挑选脚本并创建生成器：
//
//  1. 环境变量 TZZB_CHAMELEON_JS 指向的本地文件；
//  2. 本地缓存 ~/.tzzb/chameleon.min.js（30 分钟内有效）；
//  3. 联网下载当前版本（失败则忽略）；
//  4. 内置副本。
//
// 也就是说：没有任何网络也能工作，联网时则自动跟上最新版本。
func Load(ctx context.Context) (*Generator, error) {
	if path := strings.TrimSpace(os.Getenv("TZZB_CHAMELEON_JS")); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("读取 TZZB_CHAMELEON_JS 失败: %w", err)
		}
		return NewWithScript(string(raw))
	}

	cachePath := cacheFilePath()
	if raw, err := os.ReadFile(cachePath); err == nil {
		if info, statErr := os.Stat(cachePath); statErr == nil && time.Since(info.ModTime()) < cacheTTL {
			if g, gErr := NewWithScript(string(raw)); gErr == nil {
				return g, nil
			}
		}
	}

	if src, err := fetchScript(ctx, nil); err == nil {
		if g, gErr := NewWithScript(src); gErr == nil {
			// 缓存失败不影响使用，忽略即可。
			_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
			_ = os.WriteFile(cachePath, []byte(src), 0o644)
			return g, nil
		}
	}

	return New()
}

// Token 执行 chameleon 脚本生成一枚全新的 token（兜底路径）。
//
// 主路径应优先用 RefreshCookie：从 Cookie 里已有的 v 做数学刷新即可，
// 不用跑 JS。只有当 Cookie 里没有可识别的 v3 token（首次使用、旧格式、
// 被改坏）时，才需要这里生成一枚全新的会话模板。
//
// 每调用一次内部会触发一次 XHR hook，让 chameleon 重新生成保存在
// document.cookie 里的 v，再把新值取出来。取到后立刻清空队列，
// 避免长期运行的进程（serve）把 token 数组攒大。
func (g *Generator) Token() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	trigger := fmt.Sprintf(`(function () {
		var x = new XMLHttpRequest();
		x.open('POST', %q);
		x.send('x=1');
		return __tokens.length;
	})()`, triggerURL)
	if _, err := g.vm.RunString(trigger); err != nil {
		return "", fmt.Errorf("生成 hexin-v 失败: %w", err)
	}

	v, err := g.vm.RunString(`__tokens.length > 0 ? __tokens[__tokens.length - 1] : ""`)
	if err != nil {
		return "", fmt.Errorf("读取 hexin-v 失败: %w", err)
	}
	tok := strings.TrimSpace(v.String())
	_, _ = g.vm.RunString(`__tokens = []`)

	if len(tok) != tokenLen {
		return "", fmt.Errorf("hexin-v 长度异常: 得到 %d 字符，期望 %d", len(tok), tokenLen)
	}
	if tok[0] != 'A' {
		return "", fmt.Errorf("hexin-v 格式异常: %q", truncate(tok, 16))
	}
	return tok, nil
}

// Script 返回当前使用的脚本源码，便于排查版本问题。
func (g *Generator) Script() string { return g.script }

// fetchScript 下载当前版本的 chameleon 脚本。
//
// 版本号约等于 Unix 秒 / 1000，为了容忍时钟偏差和版本刚滚动的情况，
// 会依次尝试当前、前一个、前两个版本。
func fetchScript(ctx context.Context, hc *http.Client) (string, error) {
	if hc == nil {
		hc = &http.Client{Timeout: fetchTimeout}
	}
	ver := time.Now().Unix() / 1000
	var lastErr error
	for i := int64(0); i < 3; i++ {
		u := fmt.Sprintf(ScriptURLTemplate, ver-i)
		reqCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u, nil)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}
		resp, err := hc.Do(req)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
			continue
		}
		src := string(body)
		if !strings.Contains(src, "TOKEN_SERVER_TIME") {
			lastErr = fmt.Errorf("%s: 内容不像 chameleon 脚本", u)
			continue
		}
		return src, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("没有可用的脚本版本")
	}
	return "", lastErr
}

// cacheFilePath 返回脚本缓存路径。
func cacheFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".tzzb", "chameleon.min.js")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
