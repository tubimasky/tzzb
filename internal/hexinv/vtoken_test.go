package hexinv

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

// testPayload 构造一个带可识别模板的 payload。
func testPayload(ts time.Time, counter uint16) [vPayloadLen]byte {
	var p [vPayloadLen]byte
	for i := range p {
		p[i] = byte(i * 7)
	}
	p[42] = vTokenVersion
	binary.BigEndian.PutUint32(p[8:12], uint32(ts.Unix()))
	binary.BigEndian.PutUint16(p[40:42], counter)
	return p
}

// TestEncodeDecodeRoundTrip 编码后再解码必须字节级一致。
func TestEncodeDecodeRoundTrip(t *testing.T) {
	p := testPayload(time.Unix(1793123784, 0), 8)
	tok := EncodeV(p)
	if len(tok) != VTokenLen {
		t.Fatalf("token 长度 = %d, 期望 %d", len(tok), VTokenLen)
	}
	got, ok := DecodeV(tok)
	if !ok {
		t.Fatal("DecodeV 失败")
	}
	if got != p {
		t.Fatalf("payload 不一致:\n got=%v\nwant=%v", got, p)
	}
}

// TestDecodeVRejectsInvalid 覆盖长度/版本/校验和/模板版本位。
func TestDecodeVRejectsInvalid(t *testing.T) {
	valid := EncodeV(testPayload(time.Unix(1793123784, 0), 8))

	cases := []struct {
		name  string
		token string
	}{
		{"长度不对", valid[:len(valid)-1]},
		{"空串", ""},
		{"非 base64", strings.Repeat("!", VTokenLen)},
		{"校验和坏", corruptToken(t, valid)},
	}
	for _, c := range cases {
		if _, ok := DecodeV(c.token); ok {
			t.Errorf("%s: 期望解码失败", c.name)
		}
	}
}

// corruptToken 翻转 token 中的一个字节（保持可解码），破坏校验和。
func corruptToken(t *testing.T, token string) string {
	t.Helper()
	// 通过内部表示重新编码：解出 raw，改一个 payload 位，再直接 base64。
	p, ok := DecodeV(token)
	if !ok {
		t.Fatal("测试前置：token 无法解码")
	}
	p[20] ^= 0xFF // 改 payload 但不改校验位 → 校验和必然不符
	raw := make([]byte, vPayloadLen+2)
	raw[0] = vTokenVersion
	state := int32(vChecksum(p[:]))
	raw[1] = byte(state)
	for i := 0; i < vPayloadLen; i++ {
		raw[i+2] = p[i] ^ byte(state&0xFF)
		state = ^(state * 131)
	}
	return base64RawURL(raw)
}

func base64RawURL(b []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	var sb strings.Builder
	for i := 0; i < len(b); i += 3 {
		var v uint32
		n := 0
		for j := 0; j < 3 && i+j < len(b); j++ {
			v |= uint32(b[i+j]) << (16 - 8*uint(j))
			n++
		}
		sb.WriteByte(alphabet[(v>>18)&63])
		if n > 1 {
			sb.WriteByte(alphabet[(v>>12)&63])
		}
		if n > 2 {
			sb.WriteByte(alphabet[(v>>6)&63])
		}
	}
	return sb.String()
}

// TestRefreshCookie 时间戳/counter 被更新，模板与 Cookie 其它字段保持不变。
func TestRefreshCookie(t *testing.T) {
	oldTs := time.Unix(1793123784, 0)
	p := testPayload(oldTs, 8)
	token := EncodeV(p)
	cookies := "userid=42; u_name=test; v=" + token + "; ticket=abc"

	now := time.Unix(1793200000, 0)
	newCookies, newToken, ok := RefreshCookie(cookies, now)
	if !ok {
		t.Fatal("RefreshCookie 失败")
	}
	if newToken == token {
		t.Error("token 未变化")
	}
	if strings.Contains(newCookies, oldTs.Format("x")) {
		t.Error("Cookie 里不应再出现旧 token")
	}
	got, ok := DecodeV(newToken)
	if !ok {
		t.Fatal("刷新后的 token 无法解码")
	}
	if ts := int64(binary.BigEndian.Uint32(got[8:12])); ts != now.Unix() {
		t.Errorf("时间戳 = %d, 期望 %d", ts, now.Unix())
	}
	if c := binary.BigEndian.Uint16(got[40:42]); c != 9 {
		t.Errorf("counter = %d, 期望 9", c)
	}
	// 模板（除 ts/counter）必须原样保留
	for i := 0; i < vPayloadLen; i++ {
		if i >= 8 && i < 12 || i >= 40 && i < 42 {
			continue
		}
		if got[i] != p[i] {
			t.Fatalf("payload[%d] 被意外修改: %02x -> %02x", i, p[i], got[i])
		}
	}
	// Cookie 其它字段保持
	if !strings.HasPrefix(newCookies, "userid=42; u_name=test; v=") || !strings.HasSuffix(newCookies, "; ticket=abc") {
		t.Errorf("Cookie 其它字段被破坏: %q", newCookies)
	}
}

// TestRefreshCookieUnchangedWhenUnavailable 无法识别时原样返回、ok=false。
func TestRefreshCookieUnchangedWhenUnavailable(t *testing.T) {
	now := time.Now()
	cases := []string{
		"userid=42; ticket=abc",                                                                // 没有 v
		"userid=42; v=short; ticket=abc",                                                       // v 长度不对
		"userid=42; v=" + strings.Repeat("A", VTokenLen) + "; x=1",                             // 校验和不对
		"userid=42; v=" + EncodeV(testPayload(now, 1)) + "; v=" + EncodeV(testPayload(now, 2)), // 多个 v
	}
	for _, c := range cases {
		got, _, ok := RefreshCookie(c, now)
		if ok {
			t.Errorf("期望失败: %q", c)
		}
		if got != c {
			t.Errorf("失败时应原样返回: %q -> %q", c, got)
		}
	}
}

// TestRefreshCookieRejectsFarFuture 超出 uint32 秒范围时拒绝（避免静默截断）。
func TestRefreshCookieRejectsFarFuture(t *testing.T) {
	token := EncodeV(testPayload(time.Unix(1793123784, 0), 1))
	if _, _, ok := RefreshCookie("v="+token, time.Unix(1<<33, 0)); ok {
		t.Error("超出 uint32 的时间应被拒绝")
	}
}

// TestSetCookieV 追加/替换 v 字段。
func TestSetCookieV(t *testing.T) {
	token := EncodeV(testPayload(time.Unix(1793123784, 0), 1))

	got, ok := SetCookieV("userid=42; ticket=abc", token)
	if !ok || got != "userid=42; ticket=abc; v="+token {
		t.Fatalf("追加 v 失败: %q (ok=%v)", got, ok)
	}
	got, ok = SetCookieV("userid=42; v=old; ticket=abc", token)
	if !ok || got != "userid=42; v="+token+"; ticket=abc" {
		t.Fatalf("替换 v 失败: %q (ok=%v)", got, ok)
	}
	if _, ok := SetCookieV("v="+token+"; v="+token, token); ok {
		t.Error("多个 v 时不应替换")
	}
}

// TestRefreshRealGeneratedToken 用官方 chameleon（goja）生成真实 token，
// 验证解码 → 刷新 → 再解码全链路。
func TestRefreshRealGeneratedToken(t *testing.T) {
	g, err := New()
	if err != nil {
		t.Fatalf("New() 失败: %v", err)
	}
	tok, err := g.Token()
	if err != nil {
		t.Fatalf("Token() 失败: %v", err)
	}
	old, ok := DecodeV(tok)
	if !ok {
		t.Fatal("真实 token 无法解码")
	}
	now := time.Now()
	newCookies, newTok, ok := RefreshCookie("userid=1; v="+tok, now)
	if !ok {
		t.Fatal("真实 token 刷新失败")
	}
	if newTok == tok {
		t.Fatal("token 未变化")
	}
	got, ok := DecodeV(newTok)
	if !ok {
		t.Fatal("刷新后的 token 无法解码")
	}
	if ts := int64(binary.BigEndian.Uint32(got[8:12])); ts != now.Unix() {
		t.Errorf("时间戳 = %d, 期望 %d", ts, now.Unix())
	}
	oldCounter := binary.BigEndian.Uint16(old[40:42])
	if c := binary.BigEndian.Uint16(got[40:42]); c != oldCounter+1 {
		t.Errorf("counter = %d, 期望 %d", c, oldCounter+1)
	}
	for i := 0; i < vPayloadLen; i++ {
		if i >= 8 && i < 12 || i >= 40 && i < 42 {
			continue
		}
		if got[i] != old[i] {
			t.Fatalf("payload[%d] 被意外修改", i)
		}
	}
	// 再刷新一次，counter 继续 +1（证明连续刷新可用）
	_, tok2, ok := RefreshCookie(newCookies, now.Add(time.Second))
	if !ok {
		t.Fatal("二次刷新失败")
	}
	got2, _ := DecodeV(tok2)
	if c := binary.BigEndian.Uint16(got2[40:42]); c != oldCounter+2 {
		t.Errorf("二次刷新 counter = %d, 期望 %d", c, oldCounter+2)
	}
}
