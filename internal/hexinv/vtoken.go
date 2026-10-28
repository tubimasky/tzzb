package hexinv

// v3 token 的数学刷新（不执行 JS）。
//
// 官方 PC 页面上的 chameleon 会在**每个请求前**生成一枚新 token，并同时写回
// Cookie 的 v（页面本身不发 hexin-v 头）。token 是一个 60 字符 URL-safe base64
// 字符串，解码后 45 字节：
//
//	raw[0]       = 3              版本
//	raw[1..2]    = 校验和（见下）
//	raw[2..45]   = 43 字节 payload，逐字节与 state 链异或
//
// payload 里只有少数几个字段会变，其余是会话/设备相关的模板：
//
//	payload[0:8]   会话/设备 ID
//	payload[8:12]  Unix 秒（大端）
//	payload[12:20] 指纹相关字段
//	payload[20:40] 基本固定
//	payload[40:42] 计数器（大端 uint16，每个 token +1）
//	payload[42]    = 3
//
// 所以正确且轻量的刷新方式是：解出 payload、把时间戳改成当前秒、计数器 +1、
// 重算校验位、重新编码 —— 保留模板，只动时间与计数。实测（2026-10-28）：
// 从 6 天前的旧 v 刷新后连续请求全部 200；随机改模板则约 20% 概率 403。

import (
	"encoding/base64"
	"encoding/binary"
	"regexp"
	"strings"
	"time"
)

const (
	// VTokenLen 是 v3 token 的固定长度（URL-safe base64，60 字符）。
	VTokenLen = 60

	// vPayloadLen 是解码后 payload 的长度。
	vPayloadLen = 43

	// vTokenVersion 是 token 的版本号（raw[0] 与 payload[42] 均为该值）。
	vTokenVersion = 3
)

// cookieVRe 匹配 Cookie 里的 v 字段。与官方页面一致：要求整条 Cookie 里
// 最多只有一个 v=，避免误改其它同名字段。
var cookieVRe = regexp.MustCompile(`(?:^|;\s*)v=([^;]*)`)

// vChecksum 复刻 token 的校验位算法：对 payload 逐字节做 v = v*31 + b（32 位），
// 取低 8 位。
func vChecksum(payload []byte) byte {
	var v int32
	for _, b := range payload {
		v = (v << 5) - v + int32(b)
	}
	return byte(v & 0xFF)
}

// DecodeV 把 token 解码成 43 字节 payload。
//
// 返回 false 表示不是可识别的 v3 token（长度/版本/校验和不符）。调用方应回退到
// 其它方式（例如执行 chameleon 脚本生成新 token），而不是继续使用该 token。
func DecodeV(token string) (payload [vPayloadLen]byte, ok bool) {
	if len(token) != VTokenLen {
		return payload, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != vPayloadLen+2 || raw[0] != vTokenVersion {
		return payload, false
	}
	state := int32(raw[1])
	for i := 0; i < vPayloadLen; i++ {
		payload[i] = raw[i+2] ^ byte(state&0xFF)
		state = ^(state * 131)
	}
	if payload[42] != vTokenVersion || vChecksum(payload[:]) != raw[1] {
		return payload, false
	}
	return payload, true
}

// EncodeV 把 43 字节 payload 编码成 token（DecodeV 的逆运算）。
//
// payload 不合法时返回空字符串。
func EncodeV(payload [vPayloadLen]byte) string {
	raw := make([]byte, vPayloadLen+2)
	raw[0] = vTokenVersion
	state := int32(vChecksum(payload[:]))
	raw[1] = byte(state)
	for i := 0; i < vPayloadLen; i++ {
		raw[i+2] = payload[i] ^ byte(state&0xFF)
		state = ^(state * 131)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// RefreshCookie 把 Cookie 里的 v 刷新为"当前时间 + 计数器 +1"的新 token。
//
// 这是对齐官方页面行为的主路径：不执行 JS、不联网，只改时间与计数，保留
// 会话/设备模板。返回刷新后的 Cookie 与 token；ok=false 表示 Cookie 里没有
// 唯一的、可识别的 v3 token（调用方应回退到执行 chameleon 脚本）。
func RefreshCookie(cookies string, now time.Time) (newCookies, token string, ok bool) {
	locs := cookieVRe.FindAllStringSubmatchIndex(cookies, -1)
	if len(locs) != 1 {
		return cookies, "", false
	}
	start, end := locs[0][2], locs[0][3]
	payload, ok := DecodeV(cookies[start:end])
	if !ok {
		return cookies, "", false
	}
	sec := now.Unix()
	if sec < 0 || sec > 0xFFFFFFFF {
		return cookies, "", false
	}
	counter := binary.BigEndian.Uint16(payload[40:42]) + 1
	binary.BigEndian.PutUint32(payload[8:12], uint32(sec))
	binary.BigEndian.PutUint16(payload[40:42], counter)

	token = EncodeV(payload)
	return cookies[:start] + token + cookies[end:], token, true
}

// SetCookieV 把 Cookie 里的 v 替换为给定 token。
//
// 用于"执行 chameleon 生成了全新 token"的兜底路径：此时 Cookie 里可能没有 v，
// 或者原有 v 已无法解码；把新 token 写回内存 Cookie，后续请求就能走
// RefreshCookie 的快路径。ok=false 表示 Cookie 里有多个 v 字段、无法安全替换。
func SetCookieV(cookies, token string) (newCookies string, ok bool) {
	locs := cookieVRe.FindAllStringSubmatchIndex(cookies, -1)
	switch len(locs) {
	case 0:
		cookies = strings.TrimRight(cookies, " ;")
		if cookies == "" {
			return "v=" + token, true
		}
		return cookies + "; v=" + token, true
	case 1:
		start, end := locs[0][2], locs[0][3]
		return cookies[:start] + token + cookies[end:], true
	default:
		return cookies, false
	}
}
