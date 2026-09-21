package browsercookie

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha1"
	"crypto/sha256"
	"errors"
	"fmt"
)

// Chromium 系浏览器在 macOS 上解密 cookie 的参数。源自 Chromium 的
// components/os_crypt/os_crypt_mac.mm：口令存在钥匙串里，用固定 salt 和
// 1003 轮 PBKDF2-SHA1 派生出 16 字节 AES 密钥。
const (
	keychainSalt   = "saltysalt"
	keychainRounds = 1003
	keyLength      = 16
)

// cbcIV 是 Chromium 固定使用的初始向量：16 个空格。
var cbcIV = bytes.Repeat([]byte{' '}, aes.BlockSize)

// ciphertextPrefixes 是加密值的版本前缀。v10 是通用格式，v11 出现在部分平台的
// app-bound 加密里，两者在这里的解密方式相同。
var ciphertextPrefixes = [][]byte{[]byte("v10"), []byte("v11")}

// errBadPadding 表示解密结果不是合法密文（多半是密钥不匹配）。
var errBadPadding = errors.New("PKCS7 填充非法")

// deriveKey 由钥匙串口令派生出 AES-128 密钥。
func deriveKey(password string) ([]byte, error) {
	return pbkdf2.Key(sha1.New, password, []byte(keychainSalt), keychainRounds, keyLength)
}

// stripCiphertextPrefix 判断并剥掉版本前缀。第二个返回值为 false 表示这条值
// 没有前缀，即老版本浏览器留下的明文。
func stripCiphertextPrefix(enc []byte) ([]byte, bool) {
	for _, p := range ciphertextPrefixes {
		if bytes.HasPrefix(enc, p) {
			return enc[len(p):], true
		}
	}
	return enc, false
}

// decryptValue 解密一条 Chromium cookie 值。
//
// host 用来剥离新版 Chromium 在明文头部附加的 32 字节 SHA256(host_key)——
// 那是 app-bound 加密用来防止 cookie 被搬到别的站点复用的。
func decryptValue(key []byte, host string, enc []byte) (string, error) {
	if len(enc) == 0 {
		return "", nil
	}
	body, encrypted := stripCiphertextPrefix(enc)
	if !encrypted {
		return string(enc), nil
	}
	if len(body) == 0 || len(body)%aes.BlockSize != 0 {
		return "", fmt.Errorf("密文长度 %d 不是块大小 %d 的整数倍", len(body), aes.BlockSize)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	plain := make([]byte, len(body))
	cipher.NewCBCDecrypter(block, cbcIV).CryptBlocks(plain, body)

	if want := sha256.Sum256([]byte(host)); len(plain) >= sha256.Size && bytes.Equal(plain[:sha256.Size], want[:]) {
		plain = plain[sha256.Size:]
	}

	plain, err = unpadPKCS7(plain)
	if err != nil {
		return "", fmt.Errorf("%w（cookie 可能来自另一台机器，或加密方式已变更）", err)
	}
	return string(plain), nil
}

// unpadPKCS7 去掉 PKCS#7 填充。填充非法直接报错而不是返回半截明文——
// 半截的 cookie 会被当成有效值发出去，报错反而更容易定位问题。
func unpadPKCS7(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, errBadPadding
	}
	n := int(b[len(b)-1])
	if n == 0 || n > aes.BlockSize || n > len(b) {
		return nil, errBadPadding
	}
	for _, c := range b[len(b)-n:] {
		if int(c) != n {
			return nil, errBadPadding
		}
	}
	return b[:len(b)-n], nil
}
