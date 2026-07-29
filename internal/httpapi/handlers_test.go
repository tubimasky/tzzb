package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAccountsHandlerWithInjectedTransport(t *testing.T) {
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var exData any
		switch {
		case strings.HasSuffix(req.URL.Path, "/account_list"):
			exData = map[string]any{
				"common": []map[string]any{{"fundname": "测试账户", "fund_key": "account-1"}},
			}
		case strings.HasSuffix(req.URL.Path, "/stock_position"):
			exData = map[string]any{
				"total_asset": 100000, "total_value": 80000, "money_remain": 20000,
				"position_rate": 0.8, "position": []any{},
			}
		case strings.HasSuffix(req.URL.Path, "/stock_card"):
			exData = map[string]any{"asset": 100000, "profit": 5000, "now_profit": 100}
		default:
			t.Fatalf("未处理的上游路径: %s", req.URL.Path)
		}
		body, err := json.Marshal(map[string]any{"error_code": "0", "ex_data": exData})
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(body)),
			Request:    req,
		}, nil
	})
	client := api.NewClientWithHTTPClient(&http.Client{Transport: transport}, "userid=test")
	server := NewServer(client, false)
	req := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	recorder := httptest.NewRecorder()

	server.mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("期望 HTTP 200，实际 %d: %s", recorder.Code, recorder.Body.String())
	}
	var resp accountsResp
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Complete || len(resp.Warnings) != 0 {
		t.Fatalf("完整快照不应包含警告: %+v", resp.Warnings)
	}
	if len(resp.Accounts) != 1 || resp.Accounts[0].TotalAssets != 100000 {
		t.Fatalf("账户响应错误: %+v", resp.Accounts)
	}
	if resp.Accounts[0].Rate != nil || resp.Summary.TotalRate != nil {
		t.Fatal("handler 不应输出伪造收益率")
	}
}

func TestBuildAccountsResponseDoesNotInventRates(t *testing.T) {
	accounts := []api.Account{{
		Category:    "common",
		FundName:    "测试账户",
		TotalAssets: 100000,
		HoldProfit:  5000,
		TodayProfit: 100,
		TotalProfit: 5000,
		Principal:   50000,
	}}
	warnings := []api.AccountLoadWarning{{Account: "测试账户", Component: "账户卡", Message: "timeout"}}

	resp := buildAccountsResponse(accounts, warnings)
	if resp.Complete {
		t.Fatal("有加载警告时 complete 不应为 true")
	}
	if len(resp.Warnings) != 1 {
		t.Fatalf("期望 1 条警告，实际 %d", len(resp.Warnings))
	}
	if resp.Accounts[0].Rate != nil || resp.Summary.TotalRate != nil || resp.Summary.TotalPrincipal != nil {
		t.Fatal("缺少可信本金口径时收益率和本金必须为 null")
	}
	if resp.Summary.ProfitScope != "holding" || resp.Summary.HoldProfit != 5000 {
		t.Fatalf("持仓盈亏口径错误: %+v", resp.Summary)
	}
}
