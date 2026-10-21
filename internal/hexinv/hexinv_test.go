package hexinv

import (
	"context"
	"strings"
	"testing"
)

// TestNewAndToken 验证内置脚本能在 goja 里跑起来并生成合法 token。
func TestNewAndToken(t *testing.T) {
	g, err := New()
	if err != nil {
		t.Fatalf("New() 失败: %v", err)
	}
	tok, err := g.Token()
	if err != nil {
		t.Fatalf("Token() 失败: %v", err)
	}
	if len(tok) != tokenLen {
		t.Fatalf("token 长度 = %d, 期望 %d: %q", len(tok), tokenLen, tok)
	}
	if !strings.HasPrefix(tok, "A") {
		t.Fatalf("token 应以 A 开头, 得到 %q", tok)
	}
	for _, r := range tok {
		if !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_", r) {
			t.Fatalf("token 含非法字符 %q: %q", r, tok)
		}
	}
}

// TestTokenUnique 验证每次生成的 token 都不同。
//
// 这是整个方案成立的前提：同一个 token 复用约 8~10 次就会被判 403，
// 所以必须保证"每请求一个新 token"。
func TestTokenUnique(t *testing.T) {
	g, err := New()
	if err != nil {
		t.Fatalf("New() 失败: %v", err)
	}
	seen := make(map[string]bool)
	for i := 0; i < 8; i++ {
		tok, err := g.Token()
		if err != nil {
			t.Fatalf("第 %d 次 Token() 失败: %v", i+1, err)
		}
		if seen[tok] {
			t.Fatalf("第 %d 次生成了重复 token: %s", i+1, tok)
		}
		seen[tok] = true
	}
}

// TestTokenConcurrent 验证并发调用是安全的（serve 模式会并发取 token）。
func TestTokenConcurrent(t *testing.T) {
	g, err := New()
	if err != nil {
		t.Fatalf("New() 失败: %v", err)
	}
	const n = 8
	results := make(chan string, n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			tok, err := g.Token()
			if err != nil {
				errs <- err
				return
			}
			results <- tok
		}()
	}
	seen := make(map[string]bool)
	for i := 0; i < n; i++ {
		select {
		case err := <-errs:
			t.Fatalf("并发 Token() 失败: %v", err)
		case tok := <-results:
			if seen[tok] {
				t.Fatalf("并发下出现重复 token: %s", tok)
			}
			seen[tok] = true
		}
	}
}

// TestNewWithEmptyScript 验证空脚本会被拒绝。
func TestNewWithEmptyScript(t *testing.T) {
	if _, err := NewWithScript("   "); err == nil {
		t.Fatal("空脚本应当报错")
	}
}

// TestNewWithBadScript 验证坏脚本会在创建阶段就报错。
func TestNewWithBadScript(t *testing.T) {
	if _, err := NewWithScript("this is not javascript {{{"); err == nil {
		t.Fatal("坏脚本应当报错")
	}
}

// TestLoadFallback 验证离线时能退回内置副本（不依赖网络）。
func TestLoadFallback(t *testing.T) {
	t.Setenv("TZZB_CHAMELEON_JS", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立刻取消：确保联网分支一定失败，走内置副本
	g, err := Load(ctx)
	if err != nil {
		t.Fatalf("Load() 失败: %v", err)
	}
	if _, err := g.Token(); err != nil {
		t.Fatalf("回退后用内置副本生成的 token 不可用: %v", err)
	}
}

// TestLoadEnvOverride 验证 TZZB_CHAMELEON_JS 指定的文件优先。
func TestLoadEnvOverride(t *testing.T) {
	path := t.TempDir() + "/chameleon.js"
	if err := writeFile(path, embeddedScript); err != nil {
		t.Fatalf("写临时脚本失败: %v", err)
	}
	t.Setenv("TZZB_CHAMELEON_JS", path)
	g, err := Load(context.Background())
	if err != nil {
		t.Fatalf("Load() 失败: %v", err)
	}
	if g.Script() != embeddedScript {
		t.Fatal("没有使用 TZZB_CHAMELEON_JS 指定的脚本")
	}
}

func writeFile(path, content string) error {
	return osWriteFile(path, []byte(content), 0o644)
}
