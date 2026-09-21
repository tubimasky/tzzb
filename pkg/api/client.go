package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	BaseURL = "https://tzzb.10jqka.com.cn/caishen_httpserver/tzzb"
	Origin  = "https://tzzb.10jqka.com.cn"
	Referer = "https://tzzb.10jqka.com.cn/pc/"
	UA      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

	epAccountList     = "/caishen_fund/pc/account/v1/account_list"
	epMoneyHistory    = "/caishen_fund/pc/account/v1/get_money_history"
	epStockPosition   = "/caishen_fund/pc/asset/v1/stock_position"
	epFundPosition    = "/caishen_fund/pc/asset/v1/fund_position"
	epStockCard       = "/caishen_fund/pc/account/v1/stock_card"
	epFundCard        = "/caishen_fund/pc/account/v1/fund_card"
	epGetQuotes       = "/caishen_fund/invest/getQuotes"                // 插件 requestPriceInfo 同源
	epMergeDayTrading = "/caishen_fund/pc/account/v1/merge_day_trading" // 当日成交，算今日盈亏用
	epAssetTrend      = "/caishen_fund/pc/asset/v1/asset_trend"         // 资产趋势曲线
	epBankHistory     = "/caishen_fund/pc/asset/v1/query_bank_history"  // 银证转账历史

	epStockGetAccounts = "/caishen_fund/stock/getAccounts"                 // 股票账户列表（含 zjzh 资金账号）
	epCalDaily         = "/caishen_fund/combineCalendar/dailyProfitloss"   // 盈亏日历：某月各日
	epCalMonthly       = "/caishen_fund/combineCalendar/monthlyProfitloss" // 盈亏日历：某年各月

	// 股票时间维度盈亏（梯队 A，全账户合算走 Merge 变体）。
	// 关键坑：startdate/enddate 用 YYYYMMDD 无横杠（与 combineCalendar 带横杠不同，带横杠会返回 -2 服务器繁忙）。
	epStockMonthlyPL = "/caishen_fund/stock/monthlyProfitLossMerge" // 某年各月盈亏（个股盈亏为年度口径）
	epStockCodePL    = "/caishen_fund/stock/profitlossByCodeMerge"  // 某区间每日盈亏 + 个股盈亏（后者为整月口径，起始日须为月初）

	// 股票仓位分析（梯队 B）。fundkey 用 stock 账户 fund_key（非基金 fundid）。
	epAnalysisPosDist = "/caishen_fund/stockAnalysisSummary/position_distribution" // 持仓分布（按市值）
	epAnalysisPosRate = "/caishen_fund/stockAnalysisSummary/accountPositionRate"   // 账户仓位率

	// 单只股票钻取明细（handstock/itemDetail 系）。坐标：fundkey/rzrq_fundkey/manualid + stock_code。
	epMergePosQuery     = "/caishen_fund/stock_position/v1/merge_position_query" // 合并持仓明细
	epTotalPosQuery     = "/caishen_fund/stock_position/v1/total_position_query" // 建仓记录
	epStockHistoryQuery = "/caishen_fund/stock_position/v1/stock_history_query"  // 逐笔历史成交
	epClearPosQuery     = "/caishen_fund/stock_position/v1/clear_position_query" // 清仓汇总
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

// ErrAuthExpired 表示服务端拒绝了当前登录态。
//
// 403 也可能是 WAF 拦了请求（缺 origin/referer 等），所以拿到这个错误并不等于
// Cookie 一定过期，必须再看能不能真的取到一份不同的新 Cookie。
var ErrAuthExpired = errors.New("登录态失效")

// session 是共享的登录态。WithContext 是浅拷贝，凡是"刷新后要让所有副本都看见"
// 的东西都必须挂在指针后面，否则在克隆上写入的 Cookie 传不回父客户端——那样
// 常驻的 tzzb serve 就得重启才能生效。
type session struct {
	mu      sync.RWMutex
	cookies string
	uid     string
}

func (s *session) get() (cookies string, uid string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cookies, s.uid
}

func (s *session) set(cookies string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cookies = cookies
	s.uid = uidFromCookie(cookies)
}

// authState 是自动续期的共享状态。同样必须是指针：互斥锁按值内嵌会被浅拷贝复制。
type authState struct {
	mu          sync.Mutex
	refresher   func() (string, error)
	lastAttempt time.Time
}

// Client 是投资账本 API 的客户端，认证依赖浏览器登录后的 Cookie。
type Client struct {
	httpClient *http.Client
	ctx        context.Context
	sess       *session
	auth       *authState
}

// WithContext 返回绑定请求上下文的浅拷贝，可安全用于并发 HTTP handler。
// 登录态与续期状态仍与父客户端共享。
func (c *Client) WithContext(ctx context.Context) *Client {
	clone := *c
	clone.ctx = ctx
	return &clone
}

// uid 返回当前登录态里的用户 id。
func (c *Client) uid() string {
	_, uid := c.sess.get()
	return uid
}

// SetAuthRefresher 注入"重新获取 Cookie"的回调，由 cmd 层接到浏览器读取上。
// pkg/api 不关心 Cookie 从哪来，因此这里只收一个函数，也便于测试塞假实现。
func (c *Client) SetAuthRefresher(refresher func() (string, error)) {
	c.auth.mu.Lock()
	defer c.auth.mu.Unlock()
	c.auth.refresher = refresher
}

func NewClient() *Client {
	cookies := loadCookies()
	return NewClientWithHTTPClient(&http.Client{Timeout: 30 * time.Second}, cookies)
}

// NewClientWithHTTPClient 创建可注入 HTTP transport 的客户端，便于本地代理和测试。
func NewClientWithHTTPClient(httpClient *http.Client, cookies string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	client := &Client{
		httpClient: httpClient,
		sess:       &session{},
		auth:       &authState{},
	}
	client.sess.set(cookies)
	return client
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
	c.sess.set(cookies)
	return nil
}

// NewClientWithCookies 用指定的 Cookie 建一个临时客户端。
// 主要用途是在写盘之前先验证一份 Cookie 能不能用。
func NewClientWithCookies(cookies string) *Client {
	return NewClientWithHTTPClient(nil, cookies)
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

// refreshCooldown 限制自动续期的频率。serve 是常驻进程，一批并发请求同时拿到
// 403 时，不应该每个请求都去读一次浏览器数据库和钥匙串。
const refreshCooldown = 30 * time.Second

// tryRefresh 重新获取登录态，返回是否拿到了与当前不同的可用 Cookie。
// 拿到的还是同一份，就说明失效原因不是过期（多半是 WAF 拦的），此时不重试。
func (c *Client) tryRefresh() bool {
	auth := c.auth
	auth.mu.Lock()
	defer auth.mu.Unlock()

	if auth.refresher == nil || time.Since(auth.lastAttempt) < refreshCooldown {
		return false
	}
	auth.lastAttempt = time.Now()

	cookies, err := auth.refresher()
	if err != nil {
		return false
	}
	cookies = strings.TrimSpace(cookies)
	if cookies == "" {
		return false
	}
	if current, _ := c.sess.get(); cookies == current {
		return false
	}
	if err := c.SaveCookies(cookies); err != nil {
		return false
	}
	fmt.Fprintln(os.Stderr, "↻ 登录态已过期，已自动从浏览器续期")
	return true
}

// post 发起一个表单编码的 POST 请求。origin/referer 头是绕过 WAF 403 的关键。
func (c *Client) post(path string, params map[string]string) ([]byte, error) {
	cookies, uid := c.sess.get()
	if cookies == "" {
		return nil, fmt.Errorf("未登录，请先运行: tzzb login")
	}

	form := url.Values{}
	form.Set("terminal", "1")
	form.Set("version", "0.0.0")
	// 空字符串也要带上：get_money_history 等接口缺字段会直接 HTTP 400（Spring Whitelabel）。
	for k, v := range params {
		form.Set(k, v)
	}
	// 身份字段放在最后设：调用方可以覆盖 terminal/version（stockprofit、calendar
	// 确实会传自己的值），但 userid 必须始终跟着当前登录态。否则自动续期换了 Cookie
	// 之后，调用方在构造参数时读到的旧 uid 会把新 uid 覆盖掉，请求直接变成 403。
	form.Set("userid", uid)
	form.Set("user_id", uid)

	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, "POST", BaseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	req.Header.Set("origin", Origin)
	req.Header.Set("referer", Referer)
	req.Header.Set("user-agent", UA)
	req.Header.Set("cookie", cookies)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 401 是登录态真的失效；403 更常见的是 WAF 拦了请求（见 docs/API.md 1.2）。
	// 两者都按"可能过期"上报，由上层"能否取到一份不同的 Cookie"决定要不要重试。
	switch resp.StatusCode {
	case 401:
		return nil, fmt.Errorf("%w：登录态已失效(401)，请重新运行 tzzb login", ErrAuthExpired)
	case 403:
		return nil, fmt.Errorf("%w：被服务器拒绝(403)，Cookie 可能已过期，请重新运行 tzzb login", ErrAuthExpired)
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
//
// 遇到登录态失效时先尝试自动续期，续期成功才重试一次——重试条件里包含
// "新 Cookie 必须与当前不同"，所以 WAF 拦出来的 403 不会造成无意义的重试。
func (c *Client) call(path string, params map[string]string) (json.RawMessage, error) {
	raw, err := c.post(path, params)
	if err != nil && errors.Is(err, ErrAuthExpired) && c.tryRefresh() {
		raw, err = c.post(path, params)
	}
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
