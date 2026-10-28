package api

import (
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/liyanran/tzzb-cli/internal/hexinv"
)

// refreshProvider 用 RefreshCookie 作为 VTokenProvider（失败时返回错误）。
func refreshProvider(cookies string) (string, string, error) {
	nc, tok, ok := hexinv.RefreshCookie(cookies, time.Now())
	if !ok {
		return "", "", errors.New("refresh failed")
	}
	return nc, tok, nil
}

// makeVPayload 构造一枚可解析的 v3 token payload（模板字节固定，便于断言）。
func makeVPayload(ts time.Time, counter uint16) [43]byte {
	var p [43]byte
	for i := range p {
		p[i] = byte(i * 7)
	}
	p[42] = 3
	binary.BigEndian.PutUint32(p[8:12], uint32(ts.Unix()))
	binary.BigEndian.PutUint16(p[40:42], counter)
	return p
}

// captureTransport 记录每个请求的 Cookie/hexin-v 头，并按预设状态码返回。
type captureTransport struct {
	mu       sync.Mutex
	statuses []int // 按请求顺序返回；不足时默认 200
	cookies  []string
	tokens   []string
}

func (t *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	idx := len(t.cookies)
	t.cookies = append(t.cookies, req.Header.Get("cookie"))
	t.tokens = append(t.tokens, req.Header.Get("hexin-v"))

	status := 200
	if idx < len(t.statuses) {
		status = t.statuses[idx]
	}
	body := `{"error_code":0,"ex_data":{}}`
	if status == 403 {
		body = `<h1>Nginx forbidden.</h1>`
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

// newAntibotTestClient 建一个带捕获 transport 的客户端（重试零等待）。
func newAntibotTestClient(cookies string, t *captureTransport) *Client {
	c := NewClientWithHTTPClient(&http.Client{Transport: t}, cookies)
	c.retryDelay = func(int) time.Duration { return 0 }
	return c
}

// cookieV 从 Cookie 头里取 v 的值。
func cookieV(cookie string) string {
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "v=") {
			return strings.TrimPrefix(part, "v=")
		}
	}
	return ""
}

// TestVTokenProviderRefreshesCookieAndHeader 每个请求：Cookie v 与 hexin-v 头都是
// 新 token、二者相同、内存登录态被更新、counter 连续递增。
func TestVTokenProviderRefreshesCookieAndHeader(t *testing.T) {
	base := time.Unix(1793123784, 0)
	token := hexinv.EncodeV(makeVPayload(base, 8))
	rt := &captureTransport{}
	c := newAntibotTestClient("userid=42; v="+token+"; ticket=abc", rt)
	c.SetVTokenProvider(refreshProvider)

	for i := 0; i < 2; i++ {
		if _, err := c.post("/test", nil); err != nil {
			t.Fatalf("第 %d 次请求失败: %v", i+1, err)
		}
	}
	if len(rt.tokens) != 2 {
		t.Fatalf("请求次数 = %d, 期望 2", len(rt.tokens))
	}
	if rt.tokens[0] == rt.tokens[1] {
		t.Error("两次请求应使用不同 token")
	}
	for i, tok := range rt.tokens {
		if got := cookieV(rt.cookies[i]); got != tok {
			t.Errorf("第 %d 次请求: Cookie v=%q 与 hexin-v=%q 不一致", i+1, got, tok)
		}
	}
	// 内存登录态被更新为最新 token，且其它字段保留
	sessCookies, _ := c.sess.get()
	if !strings.Contains(sessCookies, "v="+rt.tokens[1]) {
		t.Errorf("内存 Cookie 未更新: %q", sessCookies)
	}
	if !strings.Contains(sessCookies, "ticket=abc") {
		t.Errorf("内存 Cookie 其它字段丢失: %q", sessCookies)
	}
	// counter 从 8 连续递增到 10
	for i, tok := range rt.tokens {
		p, ok := hexinv.DecodeV(tok)
		if !ok {
			t.Fatalf("第 %d 次 token 无法解码", i+1)
		}
		if want := uint16(8 + i + 1); binary.BigEndian.Uint16(p[40:42]) != want {
			t.Errorf("第 %d 次 counter = %d, 期望 %d", i+1, binary.BigEndian.Uint16(p[40:42]), want)
		}
	}
}

// Test403RetriedWithFreshToken 403 时换新 token 重试，成功则返回。
func Test403RetriedWithFreshToken(t *testing.T) {
	token := hexinv.EncodeV(makeVPayload(time.Unix(1793123784, 0), 8))
	rt := &captureTransport{statuses: []int{403, 403, 200}}
	c := newAntibotTestClient("userid=42; v="+token, rt)
	c.SetVTokenProvider(refreshProvider)

	if _, err := c.post("/test", nil); err != nil {
		t.Fatalf("重试后应成功: %v", err)
	}
	if len(rt.tokens) != 3 {
		t.Fatalf("请求次数 = %d, 期望 3", len(rt.tokens))
	}
	seen := map[string]bool{}
	for _, tok := range rt.tokens {
		if seen[tok] {
			t.Errorf("重试复用了 token: %q", tok)
		}
		seen[tok] = true
	}
}

// Test403ExhaustedReturnsAuthExpired 连续 403 用尽重试后按登录态失效上报。
func Test403ExhaustedReturnsAuthExpired(t *testing.T) {
	token := hexinv.EncodeV(makeVPayload(time.Unix(1793123784, 0), 8))
	rt := &captureTransport{statuses: []int{403, 403, 403, 403, 403}}
	c := newAntibotTestClient("userid=42; v="+token, rt)
	c.SetVTokenProvider(refreshProvider)

	_, err := c.post("/test", nil)
	if !errors.Is(err, ErrAuthExpired) {
		t.Fatalf("应返回 ErrAuthExpired，得到: %v", err)
	}
	if len(rt.tokens) != max403Retries+1 {
		t.Errorf("请求次数 = %d, 期望 %d", len(rt.tokens), max403Retries+1)
	}
}

// TestVTokenFallbackWhenCookieHasNoV Cookie 里没有 v 时，兜底生成 token 并写回 Cookie。
func TestVTokenFallbackWhenCookieHasNoV(t *testing.T) {
	gen := hexinv.EncodeV(makeVPayload(time.Unix(1793123784, 0), 1))
	rt := &captureTransport{}
	c := newAntibotTestClient("userid=42; ticket=abc", rt)
	c.SetVTokenProvider(func(cookies string) (string, string, error) {
		if nc, tok, ok := hexinv.RefreshCookie(cookies, time.Now()); ok {
			return nc, tok, nil
		}
		nc, ok := hexinv.SetCookieV(cookies, gen)
		if !ok {
			return cookies, gen, nil
		}
		return nc, gen, nil
	})

	if _, err := c.post("/test", nil); err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	if got := cookieV(rt.cookies[0]); got != gen {
		t.Errorf("Cookie v = %q, 期望兜底 token %q", got, gen)
	}
	if rt.tokens[0] != gen {
		t.Errorf("hexin-v = %q, 期望 %q", rt.tokens[0], gen)
	}
}

// TestHexinProviderLegacy 兼容旧接口：只设 hexin-v 头，不改 Cookie。
func TestHexinProviderLegacy(t *testing.T) {
	rt := &captureTransport{}
	c := newAntibotTestClient("userid=42; v=old", rt)
	c.SetHexinProvider(func() (string, error) { return "legacytoken", nil })

	if _, err := c.post("/test", nil); err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	if rt.tokens[0] != "legacytoken" {
		t.Errorf("hexin-v = %q", rt.tokens[0])
	}
	if got := cookieV(rt.cookies[0]); got != "old" {
		t.Errorf("旧接口不应改 Cookie v，得到 %q", got)
	}
}

// TestConcurrentRequestsGetUniqueTokens 并发请求也必须各自拿到不同 counter 的 token。
func TestConcurrentRequestsGetUniqueTokens(t *testing.T) {
	token := hexinv.EncodeV(makeVPayload(time.Unix(1793123784, 0), 8))
	rt := &captureTransport{}
	c := newAntibotTestClient("userid=42; v="+token, rt)
	c.SetVTokenProvider(refreshProvider)

	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.post("/test", nil); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("并发请求失败: %v", err)
	}

	if len(rt.tokens) != n {
		t.Fatalf("请求次数 = %d, 期望 %d", len(rt.tokens), n)
	}
	seen := map[string]bool{}
	counters := map[uint16]bool{}
	for _, tok := range rt.tokens {
		if seen[tok] {
			t.Errorf("并发请求复用了同一枚 token: %q", tok)
		}
		seen[tok] = true
		p, ok := hexinv.DecodeV(tok)
		if !ok {
			t.Fatal("并发 token 无法解码")
		}
		counters[binary.BigEndian.Uint16(p[40:42])] = true
	}
	if len(counters) != n {
		t.Errorf("counter 不唯一: %v", counters)
	}
}
