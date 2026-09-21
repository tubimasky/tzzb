package browsercookie

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"testing"
)

// testKey 固定用一组密钥，避免测试依赖真实钥匙串。
func testKey(t *testing.T) []byte {
	t.Helper()
	key, err := deriveKey("test-password")
	if err != nil {
		t.Fatalf("派生密钥失败: %v", err)
	}
	return key
}

// encryptForTest 按 Chromium 的格式造一条密文，用于加解密对拍。
func encryptForTest(t *testing.T, key []byte, host, value string, version string, withHostHash bool) []byte {
	t.Helper()
	plain := []byte(value)
	if withHostHash {
		sum := sha256.Sum256([]byte(host))
		plain = append(sum[:], plain...)
	}
	n := aes.BlockSize - len(plain)%aes.BlockSize
	plain = append(plain, bytes.Repeat([]byte{byte(n)}, n)...)

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("构造 cipher 失败: %v", err)
	}
	out := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, cbcIV).CryptBlocks(out, plain)
	return append([]byte(version), out...)
}

func TestDecryptValue(t *testing.T) {
	key := testKey(t)
	host := ".10jqka.com.cn"

	tests := []struct {
		name    string
		enc     func(t *testing.T) []byte
		want    string
		wantErr bool
	}{
		{
			name: "v10 基本格式",
			enc:  func(t *testing.T) []byte { return encryptForTest(t, key, host, "sess-value", "v10", false) },
			want: "sess-value",
		},
		{
			name: "v11 前缀同样支持",
			enc:  func(t *testing.T) []byte { return encryptForTest(t, key, host, "sess-value", "v11", false) },
			want: "sess-value",
		},
		{
			name: "带 32 字节 host 哈希（app-bound 格式）",
			enc:  func(t *testing.T) []byte { return encryptForTest(t, key, host, "sess-value", "v10", true) },
			want: "sess-value",
		},
		{
			name: "无前缀按明文处理",
			enc:  func(t *testing.T) []byte { return []byte("plain-cookie") },
			want: "plain-cookie",
		},
		{
			name: "空值",
			enc:  func(t *testing.T) []byte { return nil },
			want: "",
		},
		{
			name: "密钥不匹配",
			enc: func(t *testing.T) []byte {
				other, err := deriveKey("another-password")
				if err != nil {
					t.Fatalf("派生密钥失败: %v", err)
				}
				return encryptForTest(t, other, host, "sess-value", "v10", false)
			},
			wantErr: true,
		},
		{
			name:    "长度不是块大小整数倍",
			enc:     func(t *testing.T) []byte { return []byte("v10abc") },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decryptValue(key, host, tt.enc(t))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望报错，实际得到 %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("解密失败: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUnpadPKCS7(t *testing.T) {
	tests := []struct {
		name    string
		in      []byte
		want    string
		wantErr bool
	}{
		{name: "正常填充", in: append([]byte("abc"), bytes.Repeat([]byte{13}, 13)...), want: "abc"},
		{name: "整块填充", in: bytes.Repeat([]byte{16}, 16), want: ""},
		{name: "空输入", in: nil, wantErr: true},
		{name: "填充值为 0", in: append([]byte("abc"), 0), wantErr: true},
		{name: "填充超过块大小", in: append([]byte("abc"), 17), wantErr: true},
		{name: "填充字节不一致", in: append([]byte("abc"), 2, 3), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := unpadPKCS7(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望报错，实际得到 %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("去填充失败: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHostRank(t *testing.T) {
	tests := []struct {
		host string
		want int
	}{
		{host: ".10jqka.com.cn", want: 3},
		{host: "10jqka.com.cn", want: 2},
		{host: "tzzb.10jqka.com.cn", want: 1},
		{host: "www.10jqka.com.cn", want: 1},
		{host: "example.com", want: 0},
		{host: "not10jqka.com.cn.evil.com", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := hostRank(tt.host, "10jqka.com.cn"); got != tt.want {
				t.Errorf("hostRank(%q) = %d, want %d", tt.host, got, tt.want)
			}
		})
	}
}
