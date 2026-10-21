package upass

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"testing"
)

// realPubKeyJS 是 2026-10-22 从 https://upass.10jqka.com.cn/pubkey/default.js
// 抓到的真实公钥（公钥非密钥，可以固化在测试里）。
const realPubKeyJS = `var thspubkey={"v":"default_5","p":"10001","m":"D90F4DD5BF444916913F7B434F192587C354387FA531F2964725B5188FB9D5B40FDDD2B34F61B5560468D1F5C568796EB15F7799F03E4A3301EAF8B79B655F1B2B7DC6FFE1084E4C14A05DD9C6D0C72C5ED75890DC5D11AB5990A3C7DEBA0D68EFFD8C619B2A21AEFA3E7FF902CDAE0502025901EE2D42B76A0D7AD389F0BF69"};`

func TestParsePubKeyJS(t *testing.T) {
	k, err := parsePubKeyJS(realPubKeyJS)
	if err != nil {
		t.Fatalf("解析真实公钥失败: %v", err)
	}
	if k.Version != "default_5" {
		t.Errorf("Version = %q, 期望 default_5", k.Version)
	}
	if k.Exponent != "10001" {
		t.Errorf("Exponent = %q, 期望 10001", k.Exponent)
	}
	if len(k.Modulus) != 256 {
		t.Errorf("模数长度 = %d hex 字符, 期望 256（1024 位）", len(k.Modulus))
	}
}

func TestParsePubKeyJS_Invalid(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"空内容", ""},
		{"不是公钥", "console.log(1)"},
		{"缺模数", `var thspubkey={"v":"x","p":"10001"};`},
		{"缺指数", `var thspubkey={"v":"x","m":"ABCD"};`},
		{"JSON 损坏", `var thspubkey={"v":`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parsePubKeyJS(tc.src); err == nil {
				t.Fatalf("期望报错，实际解析成功")
			}
		})
	}
}

// TestEncryptHexRoundTrip 用自造密钥验证：我们加密出来的密文能被标准 RSA 私钥
// 解出原文 —— 说明 padding 方式和 jsbn 的 pkcs1pad2 等价（PKCS#1 v1.5）。
func TestEncryptHexRoundTrip(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("生成测试密钥失败: %v", err)
	}
	k := &pubKey{
		Version:  "test",
		Exponent: "10001",
		Modulus:  priv.N.Text(16),
	}

	const plain = "13800138000"
	enc, err := k.encryptHex(plain)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	if len(enc) != 256 { // 1024 位密钥 = 128 字节 = 256 hex 字符
		t.Errorf("密文长度 = %d, 期望 256", len(enc))
	}
	if strings.ToLower(enc) != enc {
		t.Errorf("密文应为小写 hex，实际 %q", enc)
	}

	ct, err := hex.DecodeString(enc)
	if err != nil {
		t.Fatalf("密文不是合法 hex: %v", err)
	}
	got, err := rsa.DecryptPKCS1v15(rand.Reader, priv, ct)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if string(got) != plain {
		t.Errorf("解密结果 = %q, 期望 %q", string(got), plain)
	}
}

func TestEncryptHex_InvalidKey(t *testing.T) {
	cases := []struct {
		name string
		key  *pubKey
	}{
		{"模数非法", &pubKey{Exponent: "10001", Modulus: "ZZZZ"}},
		{"模数为零", &pubKey{Exponent: "10001", Modulus: "0"}},
		{"指数非法", &pubKey{Exponent: "xyz", Modulus: "ABCD"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.key.encryptHex("x"); err == nil {
				t.Fatalf("期望报错，实际成功")
			}
		})
	}
}

func TestBaseForm(t *testing.T) {
	c := &Client{pub: &pubKey{Version: "default_5"}}
	form := c.baseForm(c.pub)

	want := map[string]string{
		"rsa_version":  "default_5",
		"request_type": "login",
		"country_code": "86",
		"source":       "pc_web",
		"ttype":        "WEB",
		"sdtis":        "C22",
	}
	for k, v := range want {
		if got := form.Get(k); got != v {
			t.Errorf("%s = %q, 期望 %q", k, got, v)
		}
	}
	if form.Get("timestamp") == "" {
		t.Errorf("缺少 timestamp")
	}
	for _, k := range []string{"captcha_type", "captcha_phrase", "captcha_ticket", "captcha_signature"} {
		if !form.Has(k) {
			t.Errorf("未触发风控时也应带上空字段 %s（与浏览器 jQuery 序列化一致）", k)
		}
	}
}

func TestNormalizePhone(t *testing.T) {
	cases := map[string]string{
		"13800138000":     "13800138000",
		" 138 0013 8000 ": "13800138000",
		"138-0013-8000":   "13800138000",
		"+8613800138000":  "13800138000",
		"008613800138000": "13800138000",
	}
	for in, want := range cases {
		if got := NormalizePhone(in); got != want {
			t.Errorf("NormalizePhone(%q) = %q, 期望 %q", in, got, want)
		}
	}
}

func TestValidPhone(t *testing.T) {
	valid := []string{"13800138000", "19912345678", "15012345678"}
	for _, p := range valid {
		if !ValidPhone(p) {
			t.Errorf("ValidPhone(%q) = false, 期望 true", p)
		}
	}
	invalid := []string{"", "1380013800", "138001380001", "12800138000", "1380013800a", "23800138000"}
	for _, p := range invalid {
		if ValidPhone(p) {
			t.Errorf("ValidPhone(%q) = true, 期望 false", p)
		}
	}
}

func TestValidSMSCode(t *testing.T) {
	if !validSMSCode("123456") {
		t.Errorf("validSMSCode(123456) = false, 期望 true")
	}
	for _, c := range []string{"", "12345", "1234567", "12345a", "abcdef"} {
		if validSMSCode(c) {
			t.Errorf("validSMSCode(%q) = true, 期望 false", c)
		}
	}
}

func TestAPIErrorClassification(t *testing.T) {
	cases := []struct {
		code        int
		risk        bool
		tooMany     bool
		refused     bool
		hintContain string
	}{
		{CodeRiskControl, true, false, false, "风控"},
		{CodeRiskControlAlt, true, false, false, "风控"},
		{CodeTooManyAttempts, false, true, false, "24 小时"},
		{CodeRefused, false, false, true, "拒绝"},
		{-1, false, false, false, "浏览器"},
	}
	for _, tc := range cases {
		err := &APIError{Code: tc.code, Msg: "测试"}
		if err.IsRiskControl() != tc.risk {
			t.Errorf("code %d IsRiskControl = %v, 期望 %v", tc.code, err.IsRiskControl(), tc.risk)
		}
		if err.IsTooMany() != tc.tooMany {
			t.Errorf("code %d IsTooMany = %v, 期望 %v", tc.code, err.IsTooMany(), tc.tooMany)
		}
		if err.IsRefused() != tc.refused {
			t.Errorf("code %d IsRefused = %v, 期望 %v", tc.code, err.IsRefused(), tc.refused)
		}
		if !strings.Contains(err.Hint(), tc.hintContain) {
			t.Errorf("code %d Hint() = %q, 期望包含 %q", tc.code, err.Hint(), tc.hintContain)
		}
	}
}

func TestAPIErrorIs(t *testing.T) {
	err := error(&APIError{Code: CodeRiskControl, Msg: "风控"})
	var target *APIError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As 应能提取 *APIError")
	}
	if target.Code != CodeRiskControl {
		t.Errorf("Code = %d, 期望 %d", target.Code, CodeRiskControl)
	}
}

// 确保 test 文件里的 big.Int 引用被使用（模数长度校验走的是字符串，这里补一个
// 真正的数值校验，防止真实公钥被换成非法内容）。
func TestRealPubKeyIsUsable(t *testing.T) {
	k, err := parsePubKeyJS(realPubKeyJS)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	n, ok := new(big.Int).SetString(k.Modulus, 16)
	if !ok || n.Sign() <= 0 {
		t.Fatalf("模数无法解析为正整数")
	}
	if n.BitLen() != 1024 {
		t.Errorf("模数位数 = %d, 期望 1024", n.BitLen())
	}
	if _, err := k.encryptHex("13800138000"); err != nil {
		t.Errorf("用真实公钥加密失败: %v", err)
	}
}
