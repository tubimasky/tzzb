package upass

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
)

// pubKey 是同花顺登录页 /pubkey/default.js 下发的 RSA 公钥。
//
// 文件内容形如：
//
//	var thspubkey={"v":"default_5","p":"10001","m":"D90F...F69"};
//
// p 是十六进制公钥指数（0x10001 = 65537），m 是十六进制模数。
// 登录接口要求 mobile / signcode 都按 PKCS#1 v1.5 加密后转成十六进制字符串，
// 与浏览器里 thsencrypt.encode()（jsbn 的 pkcs1pad2 + doPublic）行为一致。
type pubKey struct {
	Version  string `json:"v"`
	Exponent string `json:"p"`
	Modulus  string `json:"m"`
}

// parsePubKeyJS 解析 /pubkey/default.js 的内容。
func parsePubKeyJS(src string) (*pubKey, error) {
	s := strings.TrimSpace(src)
	if !strings.Contains(s, "thspubkey") {
		return nil, fmt.Errorf("公钥文件内容不符合预期（未找到 thspubkey）：%s", truncate(s, 120))
	}
	if i := strings.Index(s, "{"); i >= 0 {
		s = s[i:]
	}
	if i := strings.LastIndex(s, "}"); i >= 0 {
		s = s[:i+1]
	}
	var k pubKey
	if err := json.Unmarshal([]byte(s), &k); err != nil {
		return nil, fmt.Errorf("解析 RSA 公钥失败: %w", err)
	}
	if k.Modulus == "" {
		return nil, fmt.Errorf("RSA 公钥缺少模数（m 为空）")
	}
	if k.Exponent == "" {
		return nil, fmt.Errorf("RSA 公钥缺少指数（p 为空）")
	}
	return &k, nil
}

// encryptHex 按浏览器一致的方式加密：RSA PKCS#1 v1.5，输出小写十六进制。
//
// 浏览器侧是 jsbn 的 pkcs1pad2（随机填充 + 0x02 + 数据）再 modPow，等价于
// 标准 PKCS#1 v1.5；Go 标准库可直接复现，不需要额外依赖。
func (k *pubKey) encryptHex(plain string) (string, error) {
	n, ok := new(big.Int).SetString(k.Modulus, 16)
	if !ok || n.Sign() <= 0 {
		return "", fmt.Errorf("RSA 公钥模数非法")
	}
	e, err := strconv.ParseInt(k.Exponent, 16, 64)
	if err != nil || e <= 0 {
		return "", fmt.Errorf("RSA 公钥指数非法: %q", k.Exponent)
	}
	pub := &rsa.PublicKey{N: n, E: int(e)}
	ct, err := rsa.EncryptPKCS1v15(rand.Reader, pub, []byte(plain))
	if err != nil {
		return "", fmt.Errorf("RSA 加密失败: %w", err)
	}
	// jsbn 的 toString(16) 输出小写，服务端按大小写不敏感处理也无所谓，
	// 这里保持与浏览器完全一致。
	return hex.EncodeToString(ct), nil
}

// fetchPubKey 从登录站点拉取公钥脚本并解析。
func (c *Client) fetchPubKey(ctx context.Context) (*pubKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+pubkeyPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("获取 RSA 公钥失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, fmt.Errorf("读取 RSA 公钥失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取 RSA 公钥失败: HTTP %d（%s）", resp.StatusCode, truncate(string(body), 120))
	}
	return parsePubKeyJS(string(body))
}
