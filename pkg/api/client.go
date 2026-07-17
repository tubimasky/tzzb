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

	epAccountList   = "/caishen_fund/pc/account/v1/account_list"
	epMoneyHistory  = "/caishen_fund/pc/account/v1/get_money_history"
	epStockPosition = "/caishen_fund/pc/asset/v1/stock_position"
	epFundPosition  = "/caishen_fund/pc/asset/v1/fund_position"
	epStockCard     = "/caishen_fund/pc/account/v1/stock_card"
	epFundCard      = "/caishen_fund/pc/account/v1/fund_card"
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
	for k, v := range params {
		if v != "" {
			form.Set(k, v)
		}
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

// Position 是一条持仓明细。
type Position struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	Count      Num    `json:"count"`
	Cost       Num    `json:"cost"`
	Price      Num    `json:"price"`
	Value      Num    `json:"value"`
	HoldRate   Num    `json:"hold_rate"`   // 持仓收益率(小数)
	HoldProfit Num    `json:"hold_profit"` // 持仓盈亏
	WProfit    Num    `json:"w_profit"`    // 本周盈亏（不是当日！当日看账户级 card.now_profit）
	Market     string `json:"market"`      // 1=深 2=沪（股票/ETF 有；基金常空）
}

// Quote 是一条实时行情（新浪 hq.sinajs.cn）。
type Quote struct {
	Code      string  // 原始代码，如 512890
	Name      string
	Price     float64 // 现价
	PrevClose float64 // 昨收
}

// DayProfit 按 (现价-昨收)*数量 估算当日浮动盈亏（不含当日买卖已实现部分）。
func (q Quote) DayProfit(count float64) float64 {
	if q.PrevClose == 0 {
		return 0
	}
	price := q.Price
	if price == 0 {
		return 0
	}
	return (price - q.PrevClose) * count
}

// DayRate 当日涨跌幅（小数，-0.02 = -2%）。
func (q Quote) DayRate() float64 {
	if q.PrevClose == 0 {
		return 0
	}
	return (q.Price - q.PrevClose) / q.PrevClose
}

// MarketPrefix 把 market 字段/代码规则映射成新浪前缀 sh/sz。
func MarketPrefix(code, market string) string {
	code = normalizeStockCode(code)
	switch strings.TrimSpace(market) {
	case "1", "SZ", "sz", "0":
		return "sz"
	case "2", "SH", "sh", "1A":
		return "sh"
	}
	// 无 market 时按代码启发式（A 股常见规则）
	if len(code) >= 1 {
		switch code[0] {
		case '5', '6', '9':
			return "sh"
		case '0', '1', '2', '3':
			return "sz"
		}
	}
	return "sz"
}

func normalizeStockCode(code string) string {
	code = strings.TrimSpace(code)
	if i := strings.IndexAny(code, "."); i > 0 {
		return code[:i]
	}
	return code
}

// SinaSymbol 生成新浪行情代码，如 sh512890。
func SinaSymbol(code, market string) string {
	return MarketPrefix(code, market) + normalizeStockCode(code)
}

// FetchQuotes 批量拉新浪实时行情。keys 为 sina 代码（sh512890）或 {code, market} 对。
// 返回 map[归一化代码]Quote，如 "512890" → Quote。
func FetchQuotes(symbols []string) (map[string]Quote, error) {
	// 去重
	seen := map[string]struct{}{}
	var list []string
	for _, s := range symbols {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		list = append(list, s)
	}
	if len(list) == 0 {
		return map[string]Quote{}, nil
	}

	out := map[string]Quote{}
	// 新浪单次 list 不宜过长，按 50 一批
	const batch = 50
	client := &http.Client{Timeout: 10 * time.Second}
	for i := 0; i < len(list); i += batch {
		end := i + batch
		if end > len(list) {
			end = len(list)
		}
		chunk := list[i:end]
		url := "https://hq.sinajs.cn/list=" + strings.Join(chunk, ",")
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return out, err
		}
		req.Header.Set("Referer", "https://finance.sina.com.cn")
		req.Header.Set("User-Agent", UA)

		resp, err := client.Do(req)
		if err != nil {
			return out, err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return out, err
		}
		// 新浪返回 GBK；数字字段 ASCII，直接按字节拆也够用，名称可能乱码但不影响计算
		text := string(body)
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			// var hq_str_sh512890="name,open,prev,price,...";
			const prefix = "var hq_str_"
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			rest := strings.TrimPrefix(line, prefix)
			eq := strings.Index(rest, "=")
			if eq < 0 {
				continue
			}
			sym := rest[:eq]
			payload := strings.Trim(rest[eq+1:], `";`)
			if payload == "" {
				continue
			}
			parts := strings.Split(payload, ",")
			if len(parts) < 4 {
				continue
			}
			// 去掉 sh/sz 前缀得到纯代码
			code := sym
			if len(sym) > 2 && (sym[:2] == "sh" || sym[:2] == "sz") {
				code = sym[2:]
			}
			prev, _ := strconv.ParseFloat(parts[2], 64)
			price, _ := strconv.ParseFloat(parts[3], 64)
			if prev == 0 && price == 0 {
				continue
			}
			out[code] = Quote{
				Code:      code,
				Name:      parts[0],
				Price:     price,
				PrevClose: prev,
			}
		}
	}
	return out, nil
}

// Account 覆盖手动/基金/两融/普通四类账户的字段。
type Account struct {
	Category     string     `json:"-"` // 由客户端填充: manual/fund/rzrq/common
	ManualID     string     `json:"manual_id"`
	ManualName   string     `json:"manualname"`
	FundID       string     `json:"fundid"`
	FundIDCap    string     `json:"fundId"` // account_list 里 fund 账户用大写 I
	FundName     string     `json:"fundname"`
	FundCode     string     `json:"fundcode"`
	FundKey      string     `json:"fund_key"`
	RzrqFundKey  string     `json:"rzrq_fund_key"`
	TotalAssets  Num        `json:"total_assets"`
	TodayProfit  Num        `json:"today_profit"`
	HoldProfit   Num        `json:"hold_profit"`
	TotalProfit  Num        `json:"total_profit"`
	PositionRate Num        `json:"position_rate"`
	Cost         Num        `json:"cost"`
	MarketValue  Num        `json:"market_value"`
	Principal    Num        `json:"principal"`
	DayAdd       Num        `json:"day_add"`
	Cash         Num        `json:"cash"`
	StockNum     Num        `json:"stock_num"`
	UpdateTime   string     `json:"update_time"`
	Positions    []Position `json:"position"`
}

// Name 返回账户显示名。
func (a Account) Name() string {
	switch {
	case a.ManualName != "":
		return a.ManualName
	case a.FundName != "":
		return a.FundName
	default:
		return "未命名账户"
	}
}

// ID 返回账户标识。
func (a Account) ID() string {
	if a.ManualID != "" {
		return a.ManualID
	}
	return a.fundID()
}

// fundID 兼容 account_list(fundId) 与持仓接口(fundid) 两种大小写。
func (a Account) fundID() string {
	if a.FundID != "" {
		return a.FundID
	}
	return a.FundIDCap
}

// CategoryLabel 把内部分类码转成中文标签。
func CategoryLabel(cat string) string {
	switch cat {
	case "manual":
		return "手动"
	case "fund":
		return "基金"
	case "rzrq":
		return "两融"
	case "common":
		return "普通"
	default:
		return cat
	}
}

type accountExData struct {
	Rzrq   []Account `json:"rzrq"`
	Common []Account `json:"common"`
	Fund   []Account `json:"fund"`
	Manual []Account `json:"manual"`
}

// GetAccountList 拉取全部账户，并为每个账户富化真实资产与持仓。
// account_list 只返回账户元数据（名字/ID）；持仓明细走 position 接口，
// 账户级资产/盈亏走 card 接口（与网页账户卡同源，口径正确）。
func (c *Client) GetAccountList() ([]Account, error) {
	ex, err := c.call(epAccountList, map[string]string{"userid": c.uid})
	if err != nil {
		return nil, err
	}
	var d accountExData
	if err := json.Unmarshal(ex, &d); err != nil {
		return nil, fmt.Errorf("解析账户数据失败: %v", err)
	}

	var out []Account
	tag := func(list []Account, cat string) {
		for i := range list {
			list[i].Category = cat
			out = append(out, list[i])
		}
	}
	tag(d.Manual, "manual")
	tag(d.Fund, "fund")
	tag(d.Rzrq, "rzrq")
	tag(d.Common, "common")

	// 逐个富化：持仓明细 + 账户卡（card 覆盖资产/当日盈亏/持仓盈亏）。
	for i := range out {
		detail, err := c.fetchAccountDetail(out[i])
		if err == nil {
			out[i].mergeDetail(detail)
		}
		// card 失败时保留 position 汇总的持仓盈亏/市值；当日盈亏 position 侧不可靠，不强行用 w_profit。
		if card, err := c.fetchAccountCard(out[i]); err == nil {
			out[i].mergeCard(card)
		}
	}
	return out, nil
}

// positionResp 是持仓接口的响应。不同账户类型顶层字段不一：
// stock_position 有 total_asset(单数)/total_value/money_remain/position_rate；
// fund_position 顶层只有 position。
// 注意：持仓行 w_profit 是「本周盈亏」，不是当日；且同 code 多行时 w_profit 常重复。
// 账户级当日/持仓盈亏请用 card 接口（mergeCard）。
type positionResp struct {
	TotalAsset   Num        `json:"total_asset"`   // stock_position: 总资产
	TotalAssets  Num        `json:"total_assets"`  // 个别接口用复数
	TotalValue   Num        `json:"total_value"`   // stock_position: 总市值
	MoneyRemain  Num        `json:"money_remain"`  // stock_position: 可用现金
	PositionRate Num        `json:"position_rate"` // 仓位
	UploadTime   string     `json:"upload_time"`
	Positions    []Position `json:"position"`
}

// accountCardResp 是账户卡接口响应，与网页账户卡同源。
// stock_card / fund_card 顶层都有 asset/profit/now_profit；position/data 明细这里不用。
type accountCardResp struct {
	Asset     Num `json:"asset"`      // 总资产
	Profit    Num `json:"profit"`     // 持仓盈亏
	NowProfit Num `json:"now_profit"` // 当日盈亏
	Rate      Num `json:"rate"`       // 持仓收益率(小数)
	NowRate   Num `json:"now_rate"`   // 当日收益率(百分比口径，如 -1.41 表示 -1.41%)
}

// fetchAccountDetail 按账户类型请求对应持仓接口，返回带资产与持仓的账户详情。
func (c *Client) fetchAccountDetail(acc Account) (*positionResp, error) {
	var path string
	params := map[string]string{}
	if acc.Category == "fund" {
		path = epFundPosition
		params["fundid"] = acc.fundID()
	} else {
		path = epStockPosition
		if acc.ManualID != "" {
			params["manual_id"] = acc.ManualID
		}
		if acc.FundKey != "" {
			params["fund_key"] = acc.FundKey
		}
		if acc.RzrqFundKey != "" {
			params["rzrq_fund_key"] = acc.RzrqFundKey
		}
	}

	ex, err := c.call(path, params)
	if err != nil {
		return nil, err
	}
	var detail positionResp
	if err := json.Unmarshal(ex, &detail); err != nil {
		return nil, fmt.Errorf("解析持仓详情失败: %v", err)
	}
	return &detail, nil
}

// fetchAccountCard 拉取账户卡（与网页同源），用于账户级资产/当日盈亏/持仓盈亏。
func (c *Client) fetchAccountCard(acc Account) (*accountCardResp, error) {
	var path string
	params := map[string]string{}
	if acc.Category == "fund" {
		path = epFundCard
		params["fundid"] = acc.fundID()
	} else {
		path = epStockCard
		if acc.ManualID != "" {
			params["manual_id"] = acc.ManualID
		}
		if acc.FundKey != "" {
			params["fund_key"] = acc.FundKey
		}
		if acc.RzrqFundKey != "" {
			params["rzrq_fund_key"] = acc.RzrqFundKey
		}
	}

	ex, err := c.call(path, params)
	if err != nil {
		return nil, err
	}
	var card accountCardResp
	if err := json.Unmarshal(ex, &card); err != nil {
		return nil, fmt.Errorf("解析账户卡失败: %v", err)
	}
	return &card, nil
}

// mergeDetail 把持仓接口返回的持仓明细、市值、仓位并入账户。
// 不从 w_profit 推当日盈亏（字段语义是本周，且同 code 多行会重复）。
func (a *Account) mergeDetail(d *positionResp) {
	a.Positions = d.Positions
	a.PositionRate = d.PositionRate
	if d.UploadTime != "" {
		a.UpdateTime = d.UploadTime
	}

	// 从持仓行累加持仓盈亏与市值（value/hold_profit 是分行真实值，可加）。
	var holdProfit, value float64
	for _, p := range d.Positions {
		holdProfit += p.HoldProfit.F()
		value += p.Value.F()
	}
	a.HoldProfit = Num(holdProfit)
	// 总盈亏后端未直接给，用持仓盈亏近似（无法覆盖已清仓收益，仅作展示）。
	// 当日盈亏留给 mergeCard 用 now_profit 覆盖；这里不写 w_profit 合计。
	a.TotalProfit = a.HoldProfit

	// 市值优先用顶层 total_value，否则用累加值。
	if d.TotalValue.F() != 0 {
		a.MarketValue = d.TotalValue
	} else {
		a.MarketValue = Num(value)
	}

	// 总资产优先 total_asset(单数)，再 total_assets(复数)，都没有则用市值+现金。
	// 随后 mergeCard 会用 card.asset 覆盖（与网页一致）。
	switch {
	case d.TotalAsset.F() != 0:
		a.TotalAssets = d.TotalAsset
	case d.TotalAssets.F() != 0:
		a.TotalAssets = d.TotalAssets
	default:
		a.TotalAssets = Num(a.MarketValue.F() + d.MoneyRemain.F())
	}
	a.Cash = d.MoneyRemain

	// 基金 position 接口通常不给仓位：有市值时用 市值/总资产 估算。
	if a.PositionRate.F() == 0 && a.TotalAssets.F() > 0 && a.MarketValue.F() > 0 {
		a.PositionRate = Num(a.MarketValue.F() / a.TotalAssets.F())
	}
}

// mergeCard 用账户卡覆盖账户级资产与盈亏（与网页账户卡一致）。
func (a *Account) mergeCard(c *accountCardResp) {
	a.TotalAssets = c.Asset
	a.HoldProfit = c.Profit
	a.TodayProfit = c.NowProfit
	a.TotalProfit = c.Profit

	// card 给了更准的总资产后，再补一次仓位（主要服务基金账户）。
	if a.PositionRate.F() == 0 && a.TotalAssets.F() > 0 && a.MarketValue.F() > 0 {
		a.PositionRate = Num(a.MarketValue.F() / a.TotalAssets.F())
	}
}

// GetMoneyHistory 拉取某账户的资金/交易流水。后端结构不完全固定，做兜底解析。
func (c *Client) GetMoneyHistory(acc Account) ([]map[string]any, error) {
	params := map[string]string{}
	if acc.ManualID != "" {
		params["manual_id"] = acc.ManualID
	}
	if acc.FundID != "" {
		params["fundid"] = acc.FundID
	}
	if acc.FundKey != "" {
		params["fund_key"] = acc.FundKey
	}
	ex, err := c.call(epMoneyHistory, params)
	if err != nil {
		return nil, err
	}
	return extractRecords(ex), nil
}

// extractRecords 从 ex_data 里尽力提取一个记录数组，兼容"数组"或"含数组的对象"两种形态。
func extractRecords(ex json.RawMessage) []map[string]any {
	var arr []map[string]any
	if err := json.Unmarshal(ex, &arr); err == nil {
		return arr
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(ex, &obj); err == nil {
		for _, v := range obj {
			var inner []map[string]any
			if err := json.Unmarshal(v, &inner); err == nil && len(inner) > 0 {
				return inner
			}
		}
	}
	return nil
}
