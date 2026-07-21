package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	BaseURL = "https://tzzb.10jqka.com.cn/caishen_httpserver/tzzb"
	Origin  = "https://tzzb.10jqka.com.cn"
	Referer = "https://tzzb.10jqka.com.cn/pc/"
	UA      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

	epAccountList      = "/caishen_fund/pc/account/v1/account_list"
	epMoneyHistory     = "/caishen_fund/pc/account/v1/get_money_history"
	epStockPosition    = "/caishen_fund/pc/asset/v1/stock_position"
	epFundPosition     = "/caishen_fund/pc/asset/v1/fund_position"
	epStockCard        = "/caishen_fund/pc/account/v1/stock_card"
	epFundCard         = "/caishen_fund/pc/account/v1/fund_card"
	epGetQuotes        = "/caishen_fund/invest/getQuotes"              // 插件 requestPriceInfo 同源
	epMergeDayTrading  = "/caishen_fund/pc/account/v1/merge_day_trading" // 当日成交，算今日盈亏用
	epAssetTrend       = "/caishen_fund/pc/asset/v1/asset_trend"       // 资产趋势曲线
	epBankHistory      = "/caishen_fund/pc/asset/v1/query_bank_history" // 银证转账历史

	epStockGetAccounts = "/caishen_fund/stock/getAccounts"              // 股票账户列表（含 zjzh 资金账号）
	epCalDaily         = "/caishen_fund/combineCalendar/dailyProfitloss"   // 盈亏日历：某月各日
	epCalMonthly       = "/caishen_fund/combineCalendar/monthlyProfitloss" // 盈亏日历：某年各月

	// 股票时间维度盈亏（梯队 A，全账户合算走 Merge 变体）。
	// 关键坑：startdate/enddate 用 YYYYMMDD 无横杠（与 combineCalendar 带横杠不同，带横杠会返回 -2 服务器繁忙）。
	epStockMonthlyPL = "/caishen_fund/stock/monthlyProfitLossMerge" // 某年各月盈亏
	epStockCodePL    = "/caishen_fund/stock/profitlossByCodeMerge"  // 某区间每日盈亏 + 区间内个股盈亏

	// 股票仓位分析（梯队 B）。fundkey 用 stock 账户 fund_key（非基金 fundid）。
	epAnalysisPosDist  = "/caishen_fund/stockAnalysisSummary/position_distribution" // 持仓分布（按市值）
	epAnalysisPosRate  = "/caishen_fund/stockAnalysisSummary/accountPositionRate"   // 账户仓位率
)

// 当日成交 czdm（与插件 De 常量一致）
const (
	OpBuy           = "1"   // 证券买入 lt
	OpSell          = "2"   // 证券卖出 Mx
	OpFlash         = "0"   // 速记 U_
	OpCollateralIn  = "235" // 担保品转入 HS
	OpCollateralOut = "236" // 担保品转出 PQ
)

// Num 兼容后端把数值序列化成字符串或数字两种情况。
type Num float64

func (n *Num) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*n = 0
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		*n = 0
		return nil
	}
	*n = Num(f)
	return nil
}

// F 返回 float64 值，方便格式化。
func (n Num) F() float64 { return float64(n) }

// flexStr 兼容后端把同一字段序列化成字符串或数字（如 error_code）。
type flexStr string

func (f *flexStr) UnmarshalJSON(b []byte) error {
	*f = flexStr(strings.Trim(string(b), `"`))
	return nil
}

func (f flexStr) String() string { return string(f) }

// Client 是投资账本 API 的客户端，认证依赖浏览器登录后的 Cookie。
type Client struct {
	httpClient *http.Client
	cookies    string
	uid        string
}

func NewClient() *Client {
	cookies := loadCookies()
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		cookies:    cookies,
		uid:        uidFromCookie(cookies),
	}
}

func cookiePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".tzzb", "cookies"), nil
}

func loadCookies() string {
	path, err := cookiePath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// SaveCookies 把 Cookie 持久化到 ~/.tzzb/cookies，并解析出 uid。
func (c *Client) SaveCookies(cookies string) error {
	cookies = strings.TrimSpace(cookies)
	path, err := cookiePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(cookies), 0600); err != nil {
		return err
	}
	c.cookies = cookies
	c.uid = uidFromCookie(cookies)
	return nil
}

// uidFromCookie 从 Cookie 头里提取用户 id。
func uidFromCookie(cookie string) string {
	names := []string{"userid", "user_id", "uid", "hexin_uid", "u"}
	m := map[string]string{}
	for _, item := range strings.Split(cookie, ";") {
		kv := strings.SplitN(strings.TrimSpace(item), "=", 2)
		if len(kv) == 2 {
			m[kv[0]] = kv[1]
		}
	}
	for _, n := range names {
		if v := m[n]; v != "" {
			return v
		}
	}
	return ""
}

// post 发起一个表单编码的 POST 请求。origin/referer 头是绕过 WAF 403 的关键。
func (c *Client) post(path string, params map[string]string) ([]byte, error) {
	if c.cookies == "" {
		return nil, fmt.Errorf("未登录，请先运行: tzzb login")
	}

	form := url.Values{}
	form.Set("terminal", "1")
	form.Set("version", "0.0.0")
	form.Set("userid", c.uid)
	form.Set("user_id", c.uid)
	// 空字符串也要带上：get_money_history 等接口缺字段会直接 HTTP 400（Spring Whitelabel）。
	for k, v := range params {
		form.Set(k, v)
	}

	req, err := http.NewRequest("POST", BaseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	req.Header.Set("origin", Origin)
	req.Header.Set("referer", Referer)
	req.Header.Set("user-agent", UA)
	req.Header.Set("cookie", c.cookies)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("被服务器拒绝(403)，Cookie 可能已过期，请重新运行 tzzb login")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	return data, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// envelope 是所有接口统一的外层结构。
type envelope struct {
	ErrorCode flexStr         `json:"error_code"`
	ErrorMsg  string          `json:"error_msg"`
	ExData    json.RawMessage `json:"ex_data"`
}

// call 发请求并剥掉外层信封，返回 ex_data 原始 JSON。
func (c *Client) call(path string, params map[string]string) (json.RawMessage, error) {
	raw, err := c.post(path, params)
	if err != nil {
		return nil, err
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}
	if env.ErrorCode.String() != "0" {
		return nil, fmt.Errorf("接口错误[%s]: %s", env.ErrorCode, env.ErrorMsg)
	}
	return env.ExData, nil
}
