package browsercookie

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"
)

const testDomain = "10jqka.com.cn"

// chromTimestamp 把时间转成 Chromium 时间戳（1601 年起的微秒数）。
func chromTimestamp(t time.Time) int64 {
	return (t.Unix() + chromToUnixSeconds) * 1_000_000
}

// sessionRows 造一组字段齐全的登录态行。
func sessionRows(host string, expires int64) []cookieRow {
	return []cookieRow{
		{Host: host, Name: "userid", Value: "12345", Expires: expires},
		{Host: host, Name: "ticket", Value: "tk", Expires: expires},
		{Host: host, Name: "sess_tk", Value: "st", Expires: expires},
	}
}

func TestBuildCookie(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	future := chromTimestamp(now.Add(24 * time.Hour))
	past := chromTimestamp(now.Add(-time.Hour))

	tests := []struct {
		name       string
		rows       []cookieRow
		wantCookie string
		wantOK     bool
	}{
		{
			name:       "字段齐全且未过期",
			rows:       sessionRows(".10jqka.com.cn", future),
			wantCookie: "sess_tk=st; ticket=tk; userid=12345",
			wantOK:     true,
		},
		{
			name: "会话 cookie（过期为 0）保留",
			rows: sessionRows(".10jqka.com.cn", 0),
			wantCookie: "sess_tk=st; ticket=tk; userid=12345",
			wantOK:     true,
		},
		{
			name: "缺 sess_tk 不算登录态",
			rows: []cookieRow{
				{Host: ".10jqka.com.cn", Name: "userid", Value: "12345", Expires: future},
				{Host: ".10jqka.com.cn", Name: "ticket", Value: "tk", Expires: future},
			},
			wantOK: false,
		},
		{
			name:   "已过期不算登录态",
			rows:   sessionRows(".10jqka.com.cn", past),
			wantOK: false,
		},
		{
			name: "空值不算登录态",
			rows: []cookieRow{
				{Host: ".10jqka.com.cn", Name: "userid", Value: "", Expires: future},
				{Host: ".10jqka.com.cn", Name: "ticket", Value: "tk", Expires: future},
				{Host: ".10jqka.com.cn", Name: "sess_tk", Value: "st", Expires: future},
			},
			wantOK: false,
		},
		{
			name: "同名时优先域 cookie",
			rows: append(sessionRows(".10jqka.com.cn", future),
				cookieRow{Host: "www.10jqka.com.cn", Name: "userid", Value: "wrong", Expires: future}),
			wantCookie: "sess_tk=st; ticket=tk; userid=12345",
			wantOK:     true,
		},
		{
			name: "域名不相关的行也一并带上（由调用方按 LIKE 过滤）",
			rows: append(sessionRows(".10jqka.com.cn", future),
				cookieRow{Host: "stat.10jqka.com.cn", Name: "uid", Value: "9", Expires: future}),
			wantCookie: "sess_tk=st; ticket=tk; uid=9; userid=12345",
			wantOK:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cookie, ok := buildCookie(tt.rows, testDomain, now)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (cookie=%q)", ok, tt.wantOK, cookie)
			}
			if cookie != tt.wantCookie {
				t.Errorf("cookie = %q, want %q", cookie, tt.wantCookie)
			}
		})
	}
}

// stubProfiles 替换 readProfiles 并返回还原函数。
func stubProfiles(t *testing.T, rows []profileRows, err error) {
	t.Helper()
	original := readProfiles
	readProfiles = func(string) ([]profileRows, error) { return rows, err }
	t.Cleanup(func() { readProfiles = original })
}

func TestCandidates(t *testing.T) {
	if !Supported() {
		t.Skip("当前平台不支持读取浏览器 Cookie")
	}
	now := time.Now()
	older := chromTimestamp(now.Add(2 * time.Hour))
	newer := chromTimestamp(now.Add(48 * time.Hour))

	t.Run("按新鲜度降序，缺登录态的 profile 被排除", func(t *testing.T) {
		stubProfiles(t, []profileRows{
			{Browser: "Chrome", Profile: "Profile 1", Rows: sessionRows(".10jqka.com.cn", older)},
			{Browser: "Chrome", Profile: "Default", Rows: []cookieRow{
				{Host: ".10jqka.com.cn", Name: "Hm_lvt_x", Value: "1", Expires: newer},
			}},
			{Browser: "Edge", Profile: "Default", Rows: sessionRows(".10jqka.com.cn", newer)},
		}, nil)

		got, err := Candidates(testDomain)
		if err != nil {
			t.Fatalf("Candidates 失败: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("候选数 = %d, want 2", len(got))
		}
		if got[0].Browser != "Edge" {
			t.Errorf("第一个候选来自 %s，期望新鲜度更高的 Edge", got[0].Browser)
		}
		if got[1].Profile != "Profile 1" {
			t.Errorf("第二个候选 profile = %s, want Profile 1", got[1].Profile)
		}
	})

	t.Run("只读指定浏览器", func(t *testing.T) {
		stubProfiles(t, []profileRows{
			{Browser: "Chrome", Profile: "Default", Rows: sessionRows(".10jqka.com.cn", older)},
			{Browser: "Edge", Profile: "Default", Rows: sessionRows(".10jqka.com.cn", newer)},
		}, nil)

		got, err := CandidatesFor(testDomain, "chrome")
		if err != nil {
			t.Fatalf("CandidatesFor 失败: %v", err)
		}
		if len(got) != 1 || got[0].Browser != "Chrome" {
			t.Fatalf("got %+v, want 只有 Chrome", got)
		}

		if _, err := CandidatesFor(testDomain, "firefox"); err == nil {
			t.Error("指定不存在的浏览器时应当报错")
		}
	})

	t.Run("没有可用登录态时报错并带上域名", func(t *testing.T) {
		stubProfiles(t, []profileRows{
			{Browser: "Chrome", Profile: "Default", Rows: []cookieRow{
				{Host: ".10jqka.com.cn", Name: "Hm_lvt_x", Value: "1", Expires: newer},
			}},
		}, nil)

		_, err := Candidates(testDomain)
		if err == nil {
			t.Fatal("期望报错，实际成功")
		}
		if !strings.Contains(err.Error(), testDomain) {
			t.Errorf("错误信息应包含域名，实际: %v", err)
		}
	})
}

func TestParseRows(t *testing.T) {
	key := testKey(t)
	host := ".10jqka.com.cn"
	expires := chromTimestamp(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))

	good := encryptForTest(t, key, host, "st-value", "v10", false)
	broken := encryptForTest(t, key, host, "other", "v10", false)
	broken[5] ^= 0xff // 破坏密文，让这一条解密失败

	out := strings.Join([]string{
		strings.Join([]string{host, "sess_tk", hex.EncodeToString(good), fmt.Sprint(expires)}, fieldSep),
		strings.Join([]string{host, "ticket", hex.EncodeToString(broken), fmt.Sprint(expires)}, fieldSep),
		"列数不对的一行",
		"",
	}, "\n")

	rows := parseRows([]byte(out), key)
	if len(rows) != 1 {
		t.Fatalf("解析出 %d 行, want 1（解密失败与格式错误都应跳过）", len(rows))
	}
	if rows[0].Name != "sess_tk" || rows[0].Value != "st-value" || rows[0].Host != host {
		t.Errorf("解析结果不对: %+v", rows[0])
	}
	if rows[0].Expires != expires {
		t.Errorf("过期时间 = %d, want %d", rows[0].Expires, expires)
	}
}

func TestMissingFields(t *testing.T) {
	tests := []struct {
		name   string
		cookie string
		want   []string
	}{
		{name: "齐全", cookie: "userid=1; ticket=2; sess_tk=3", want: nil},
		{name: "缺 sess_tk", cookie: "userid=1; ticket=2", want: []string{"sess_tk"}},
		{name: "空值等同于缺失", cookie: "userid=1; ticket=; sess_tk=3", want: []string{"ticket"}},
		{name: "空串三个都缺", cookie: "", want: []string{"userid", "ticket", "sess_tk"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MissingFields(tt.cookie)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got %v, want %v", got, tt.want)
					break
				}
			}
		})
	}
}
