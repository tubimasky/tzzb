package api

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// roundTripFunc 让单测用假传输层接管请求，无需真的起服务器。
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fakeResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

const okEnvelope = `{"error_code":"0","error_msg":"","ex_data":{"ok":true}}`

func TestCallAutoRefresh(t *testing.T) {
	const (
		oldCookie = "userid=1; ticket=old"
		newCookie = "userid=1; ticket=new"
	)

	tests := []struct {
		name        string
		refresher   func(current string) (string, error)
		wantCalls   int32
		wantRefresh int32
		wantErr     bool
	}{
		{
			name:        "403 后续期成功则用新 Cookie 重试一次",
			refresher:   func(string) (string, error) { return newCookie, nil },
			wantCalls:   2,
			wantRefresh: 1,
		},
		{
			name:        "续期拿到的还是同一份则不重试",
			refresher:   func(current string) (string, error) { return current, nil },
			wantCalls:   1,
			wantRefresh: 1,
			wantErr:     true,
		},
		{
			name:        "续期失败则不重试",
			refresher:   func(string) (string, error) { return "", errors.New("没找到登录态") },
			wantCalls:   1,
			wantRefresh: 1,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 重定向 HOME，避免测试写坏真实的 ~/.tzzb/cookies。
			t.Setenv("HOME", t.TempDir())

			var calls, refreshes int32
			transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				atomic.AddInt32(&calls, 1)
				if r.Header.Get("cookie") == newCookie {
					return fakeResponse(200, okEnvelope), nil
				}
				return fakeResponse(403, "forbidden"), nil
			})

			client := NewClientWithHTTPClient(&http.Client{Transport: transport}, oldCookie)
			client.SetAuthRefresher(func() (string, error) {
				atomic.AddInt32(&refreshes, 1)
				current, _ := client.sess.get()
				return tt.refresher(current)
			})

			_, err := client.call("/test", nil)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if got := atomic.LoadInt32(&calls); got != tt.wantCalls {
				t.Errorf("请求次数 = %d, want %d", got, tt.wantCalls)
			}
			if got := atomic.LoadInt32(&refreshes); got != tt.wantRefresh {
				t.Errorf("续期次数 = %d, want %d", got, tt.wantRefresh)
			}
			if tt.wantErr && !errors.Is(err, ErrAuthExpired) {
				t.Errorf("错误应当可以用 errors.Is 匹配 ErrAuthExpired，实际 %v", err)
			}
		})
	}
}

func TestCallAutoRefreshPersistsCookie(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("cookie") == "userid=1; ticket=new" {
			return fakeResponse(200, okEnvelope), nil
		}
		return fakeResponse(403, "forbidden"), nil
	})

	client := NewClientWithHTTPClient(&http.Client{Transport: transport}, "userid=1; ticket=old")
	client.SetAuthRefresher(func() (string, error) { return "userid=1; ticket=new", nil })

	if _, err := client.call("/test", nil); err != nil {
		t.Fatalf("续期后应当请求成功，实际 %v", err)
	}

	saved, err := os.ReadFile(filepath.Join(home, ".tzzb", "cookies"))
	if err != nil {
		t.Fatalf("读取落盘 Cookie 失败: %v", err)
	}
	if string(saved) != "userid=1; ticket=new" {
		t.Errorf("落盘 Cookie = %q, want %q", saved, "userid=1; ticket=new")
	}
	if client.uid() != "1" {
		t.Errorf("续期后 uid = %q, want 1", client.uid())
	}
}

func TestCallAutoRefreshCooldown(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var calls, refreshes int32
	// 传输层永远 403，且每次续期都返回不同的 Cookie，用来观察冷却是否生效。
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return fakeResponse(403, "forbidden"), nil
	})

	client := NewClientWithHTTPClient(&http.Client{Transport: transport}, "userid=1; ticket=old")
	client.SetAuthRefresher(func() (string, error) {
		n := atomic.AddInt32(&refreshes, 1)
		return "userid=1; ticket=" + string(rune('a'+n)), nil
	})

	if _, err := client.call("/test", nil); err == nil {
		t.Fatal("第一次调用应当失败")
	}
	if _, err := client.call("/test", nil); err == nil {
		t.Fatal("第二次调用应当失败")
	}

	if got := atomic.LoadInt32(&refreshes); got != 1 {
		t.Errorf("续期次数 = %d, want 1（冷却期内不应重复读浏览器）", got)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("请求次数 = %d, want 3（首次 + 一次重试 + 冷却期内的直连）", got)
	}
}

func TestCallDoesNotRefreshOnOtherErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var refreshes int32
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return fakeResponse(500, "boom"), nil
	})

	client := NewClientWithHTTPClient(&http.Client{Transport: transport}, "userid=1; ticket=old")
	client.SetAuthRefresher(func() (string, error) {
		atomic.AddInt32(&refreshes, 1)
		return "userid=1; ticket=new", nil
	})

	if _, err := client.call("/test", nil); err == nil {
		t.Fatal("500 应当报错")
	}
	if got := atomic.LoadInt32(&refreshes); got != 0 {
		t.Errorf("续期次数 = %d, want 0（只有 403 才触发续期）", got)
	}
}

// 续期换了账号之后，调用方在构造参数时读到的旧 uid 不能覆盖当前登录态，
// 否则请求体的 userid 和 Cookie 对不上，服务端会返回 403。
func TestPostUsesSessionUID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var body string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		return fakeResponse(200, okEnvelope), nil
	})

	client := NewClientWithHTTPClient(&http.Client{Transport: transport}, "userid=851606222; ticket=t")
	if _, err := client.call("/test", map[string]string{"userid": "999999999"}); err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	if !strings.Contains(body, "userid=851606222") || strings.Contains(body, "999999999") {
		t.Errorf("请求体应当使用当前登录态的 uid，实际 %q", body)
	}
}

func TestWithContextSharesSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	client := NewClientWithHTTPClient(&http.Client{}, "userid=1; ticket=old")
	clone := client.WithContext(t.Context())

	if err := clone.SaveCookies("userid=2; ticket=new"); err != nil {
		t.Fatalf("SaveCookies 失败: %v", err)
	}
	if client.uid() != "2" {
		t.Errorf("父客户端 uid = %q, want 2（登录态应当共享）", client.uid())
	}
}
