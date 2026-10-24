package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// rewriteTransport 把发往线上 BaseURL 的请求改写到本地 httptest 服务器，
// 这样测试不需要真实网络（目标仓库的 BaseURL 是常量，不能直接改）。
type rewriteTransport struct {
	base   http.RoundTripper
	target *url.URL
}

func (t *rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	u := *r.URL
	u.Scheme = t.target.Scheme
	u.Host = t.target.Host
	r2 := r.Clone(r.Context())
	r2.URL = &u
	r2.Host = t.target.Host
	return t.base.RoundTrip(r2)
}

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("解析测试服务器地址失败: %v", err)
	}
	c := NewClientWithHTTPClient(&http.Client{Transport: &rewriteTransport{base: http.DefaultTransport, target: u}}, "userid=100; user_id=100")
	return c
}

// writeEnvelope 写出统一响应外壳。
func writeEnvelope(w http.ResponseWriter, exData string) {
	w.Header().Set("content-type", "application/json")
	fmt.Fprintf(w, `{"error_code":"0","error_msg":"","ex_data":%s}`, exData)
}

func TestGetServerCalcScope(t *testing.T) {
	// 真实响应形态：manual 账户的 fund_key 是数字（曾导致 Account 结构体解析失败），
	// common 的 fund_key 是字符串；fund/monitor 不应进入计算范围。
	handler := func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, `{
			"manual":[{"manual_id":"2179680","fund_key":2179680}],
			"common":[{"fund_key":"148997765"},{"fund_key":"93694827"},{"fund_key":"115570104"}],
			"rzrq":[{"rzrq_fund_key":"55555"}],
			"fund":[{"fund_key":"99999"}],
			"monitor":[{"fund_key":"77777"}]
		}`)
	}
	c := newTestClient(t, handler)
	scope, err := c.GetServerCalcScope()
	if err != nil {
		t.Fatalf("GetServerCalcScope 失败: %v", err)
	}
	if got := strings.Join(scope.ManualIDs, ","); got != "2179680" {
		t.Errorf("ManualIDs = %q, 期望 2179680", got)
	}
	if got := strings.Join(scope.FundKeys, ","); got != "148997765,93694827,115570104" {
		t.Errorf("FundKeys = %q, 期望 3 个普通账户（manual/monitor/fund 不应混入）", got)
	}
	if got := strings.Join(scope.RzrqFundKeys, ","); got != "55555" {
		t.Errorf("RzrqFundKeys = %q, 期望 55555", got)
	}
	if scope.Empty() {
		t.Error("scope 不应为空")
	}
	if !(ServerCalcScope{}).Empty() {
		t.Error("零值 scope 应为空")
	}
}

func TestTriggerServerCalcPollsUntilDone(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	statusCalls := 0
	handler := func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.URL.Path)
		mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/account/v1/init"):
			writeEnvelope(w, `{}`)
		case strings.HasSuffix(r.URL.Path, "/account/v1/status"):
			statusCalls++
			if statusCalls == 1 {
				// 第一次：还在计算
				writeEnvelope(w, `{"data":[{"manual_id":"2179680","status":1}]}`)
				return
			}
			writeEnvelope(w, `{"data":[{"manual_id":"2179680","status":0}]}`)
		default:
			http.NotFound(w, r)
		}
	}
	c := newTestClient(t, handler)
	scope := ServerCalcScope{ManualIDs: []string{"2179680"}}
	res, err := c.TriggerServerCalc(context.Background(), scope, 5*time.Second)
	if err != nil {
		t.Fatalf("TriggerServerCalc 失败: %v", err)
	}
	if !res.Triggered {
		t.Error("Triggered 应为 true")
	}
	if res.Polls != 2 {
		t.Errorf("Polls = %d, 期望 2", res.Polls)
	}
	if len(calls) != 3 {
		t.Errorf("请求数 = %d (%v), 期望 3（1 次 init + 2 次 status）", len(calls), calls)
	}
	if !strings.HasSuffix(calls[0], "/account/v1/init") {
		t.Errorf("第一个请求应是 init，实际 %s", calls[0])
	}
}

func TestTriggerServerCalcFailureReported(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/account/v1/init") {
			writeEnvelope(w, `{}`)
			return
		}
		writeEnvelope(w, `{"data":[{"fund_key":"148997765","status":-1}]}`)
	}
	c := newTestClient(t, handler)
	_, err := c.TriggerServerCalc(context.Background(), ServerCalcScope{FundKeys: []string{"148997765"}}, time.Second)
	if err == nil {
		t.Fatal("status=-1 时应返回错误")
	}
	if !strings.Contains(err.Error(), "fund:148997765") {
		t.Errorf("错误信息应包含失败账户 fund:148997765，实际: %v", err)
	}
}

func TestTriggerServerCalcEmptyScopeMakesNoRequest(t *testing.T) {
	var n int
	handler := func(w http.ResponseWriter, r *http.Request) {
		n++
		http.NotFound(w, r)
	}
	c := newTestClient(t, handler)
	res, err := c.TriggerServerCalc(context.Background(), ServerCalcScope{}, time.Second)
	if err != nil {
		t.Fatalf("空范围不应报错: %v", err)
	}
	if res.Triggered || n != 0 {
		t.Errorf("空范围不应发请求：Triggered=%v 请求数=%d", res.Triggered, n)
	}
}
