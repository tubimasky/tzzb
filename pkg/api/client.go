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
)

// 当日成交 czdm（与插件 De 常量一致）
const (
	OpBuy       = "1"   // 证券买入 lt
	OpSell      = "2"   // 证券卖出 Mx
	OpFlash     = "0"   // 速记 U_
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

// Quote 是一条实时行情（同源插件 getQuotes：xianjia/zuoshou）。
type Quote struct {
	Code      string  // 归一化代码，如 512890
	Market    string  // 1=深 2=沪
	Price     float64 // 现价 xianjia
	PrevClose float64 // 昨收 zuoshou
}

// QuoteRef 拉行情时的代码+市场。
type QuoteRef struct {
	Code   string
	Market string
}

// DayTrade 当日成交（merge_day_trading），用于插件口径今日盈亏。
type DayTrade struct {
	Code         string
	Name         string
	Market       string
	Price        float64 // 成交价 cjjg
	Count        float64 // 成交数量 cjsl
	OpType       string  // czdm：1买 2卖 0速记 235转入 236转出
	Fee          float64
	CostChange   float64 // moneychg（速记用）
	RemainChange float64 // remainchg（速记用）
	Account      string  // 所属账户显示名（CLI 填充）
}

// NormalizeCode 去掉市场后缀（515790.SZ → 515790）。
func NormalizeCode(code string) string {
	code = strings.TrimSpace(code)
	if i := strings.IndexAny(code, "."); i > 0 {
		return code[:i]
	}
	return code
}

// GuessMarket 无 market 时按 A 股代码启发式：5/6/9→沪(2)，其余深(1)。
func GuessMarket(code, market string) string {
	market = strings.TrimSpace(market)
	if market != "" {
		return market
	}
	code = NormalizeCode(code)
	if code == "" {
		return "1"
	}
	switch code[0] {
	case '5', '6', '9':
		return "2"
	default:
		return "1"
	}
}

// GetQuotes 批量拉同花顺实时行情（插件 requestPriceInfo → /invest/getQuotes）。
// 参数 code 形如 "2:512890,1:159992"；返回 map[归一化代码]Quote。
func (c *Client) GetQuotes(refs []QuoteRef) (map[string]Quote, error) {
	seen := map[string]struct{}{}
	var parts []string
	for _, r := range refs {
		code := NormalizeCode(r.Code)
		if code == "" {
			continue
		}
		mkt := GuessMarket(code, r.Market)
		key := mkt + ":" + code
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		parts = append(parts, key)
	}
	out := map[string]Quote{}
	if len(parts) == 0 {
		return out, nil
	}

	// 单次不宜过长，按 80 一批
	const batch = 80
	for i := 0; i < len(parts); i += batch {
		end := i + batch
		if end > len(parts) {
			end = len(parts)
		}
		ex, err := c.call(epGetQuotes, map[string]string{
			"code":   strings.Join(parts[i:end], ","),
			"userid": c.uid,
			"date":   "",
		})
		if err != nil {
			return out, err
		}
		// ex_data 是数组
		var arr []struct {
			Xianjia string `json:"xianjia"`
			Zuoshou string `json:"zuoshou"`
			Zqdm    string `json:"zqdm"`
			Scdm    string `json:"scdm"`
		}
		if err := json.Unmarshal(ex, &arr); err != nil {
			return out, fmt.Errorf("解析行情失败: %v", err)
		}
		for _, it := range arr {
			code := NormalizeCode(it.Zqdm)
			if code == "" {
				continue
			}
			price, _ := strconv.ParseFloat(strings.TrimPrefix(strings.TrimSpace(it.Xianjia), "HK$"), 64)
			prev, _ := strconv.ParseFloat(strings.TrimPrefix(strings.TrimSpace(it.Zuoshou), "HK$"), 64)
			// 去掉港币前缀后的纯数字；空串归零
			if price == 0 && prev == 0 {
				continue
			}
			out[code] = Quote{
				Code:      code,
				Market:    strings.TrimSpace(it.Scdm),
				Price:     price,
				PrevClose: prev,
			}
		}
	}
	return out, nil
}

// GetDayTrades 拉某账户当日成交（插件 requestMergeTradesToday）。
func (c *Client) GetDayTrades(acc Account) ([]DayTrade, error) {
	params := map[string]string{"userid": c.uid}
	if acc.ManualID != "" {
		params["manual_id"] = acc.ManualID
	}
	if acc.FundKey != "" {
		params["fund_key"] = acc.FundKey
	}
	if acc.RzrqFundKey != "" {
		params["rzrq_fund_key"] = acc.RzrqFundKey
	}
	// 基金账户无此接口语义，直接空
	if acc.Category == "fund" {
		return nil, nil
	}
	ex, err := c.call(epMergeDayTrading, params)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Data []struct {
			Zqdm      string `json:"zqdm"`
			Zqmc      string `json:"zqmc"`
			Market    string `json:"market"`
			Cjjg      Num    `json:"cjjg"`
			Cjsl      Num    `json:"cjsl"`
			Czdm      string `json:"czdm"`
			Czlx      string `json:"czlx"`
			Fee       Num    `json:"fee"`
			Moneychg  Num    `json:"moneychg"`
			Remainchg Num    `json:"remainchg"`
		} `json:"data"`
	}
	if err := json.Unmarshal(ex, &wrap); err != nil {
		// 兼容顶层直接是数组
		var arr []struct {
			Zqdm      string `json:"zqdm"`
			Zqmc      string `json:"zqmc"`
			Market    string `json:"market"`
			Cjjg      Num    `json:"cjjg"`
			Cjsl      Num    `json:"cjsl"`
			Czdm      string `json:"czdm"`
			Fee       Num    `json:"fee"`
			Moneychg  Num    `json:"moneychg"`
			Remainchg Num    `json:"remainchg"`
		}
		if err2 := json.Unmarshal(ex, &arr); err2 != nil {
			return nil, fmt.Errorf("解析当日成交失败: %v", err)
		}
		out := make([]DayTrade, 0, len(arr))
		for _, t := range arr {
			out = append(out, DayTrade{
				Code: NormalizeCode(t.Zqdm), Name: t.Zqmc, Market: t.Market,
				Price: t.Cjjg.F(), Count: t.Cjsl.F(), OpType: strings.TrimSpace(t.Czdm),
				Fee: t.Fee.F(), CostChange: t.Moneychg.F(), RemainChange: t.Remainchg.F(),
				Account: acc.Name(),
			})
		}
		return out, nil
	}
	out := make([]DayTrade, 0, len(wrap.Data))
	for _, t := range wrap.Data {
		out = append(out, DayTrade{
			Code: NormalizeCode(t.Zqdm), Name: t.Zqmc, Market: t.Market,
			Price: t.Cjjg.F(), Count: t.Cjsl.F(), OpType: strings.TrimSpace(t.Czdm),
			Fee: t.Fee.F(), CostChange: t.Moneychg.F(), RemainChange: t.Remainchg.F(),
			Account: acc.Name(),
		})
	}
	return out, nil
}

// LiveView 按插件 stockCalc 重算后的展示字段。
type LiveView struct {
	Price      float64
	Value      float64
	HoldProfit float64
	HoldRate   float64 // 持仓收益率 %，对应插件 浮动盈亏比 ccykb
	DayProfit  float64
	DayRate    float64 // 今日涨幅 %，对应插件 涨幅 zf = (现价−昨收)/昨收
	DayYKRate  float64 // 当日盈亏比 %，对应插件 drykb（内部/合计用）
	SellAmount float64 // 当日卖出金额，合计盈亏比分母用
	DayOK      bool
	Live       bool
}

// RecomputeLive 对齐插件 stockCalc（auto 账户口径）：
//
//	现价       = 行情 xianjia（无则回落接口价）
//	市值       = 数量 × 现价
//	今日盈亏   = 当日买卖收益 + (现价−昨收)×(持仓−今日买入−正 remainchg) − 费用 + 速记
//	今日涨幅   = (现价 − 昨收) / 昨收          ← 插件 hq.zf，不是盈亏比
//	当日盈亏比 = 今日盈亏 / (市值 − 今日盈亏 + 卖出额)  ← 插件 drykb；合计行同口径
//	持仓盈亏   = 接口 hold_profit + (现价 − 接口价) × 数量
//	收益率     = 持仓盈亏 / (成本价 × 数量)   ← 插件 浮动盈亏比
func RecomputeLive(count, cost, apiPrice, apiValue, apiHold, apiHoldRate float64, q *Quote, trades []DayTrade) LiveView {
	v := LiveView{
		Price:      apiPrice,
		Value:      apiValue,
		HoldProfit: apiHold,
		HoldRate:   apiHoldRate * 100,
	}
	if count == 0 {
		return v
	}
	if v.Value == 0 && apiPrice > 0 {
		v.Value = apiPrice * count
	}

	price := apiPrice
	prev := 0.0
	if q != nil {
		if q.Price > 0 {
			price = q.Price
		}
		if q.PrevClose > 0 {
			prev = q.PrevClose
		}
	}
	v.Price = price
	if price > 0 {
		v.Value = count * price
		v.Live = apiPrice == 0 || price != apiPrice
	}

	// 持仓盈亏：接口 + 价差修正（插件 auto）
	if apiPrice > 0 && price > 0 {
		v.HoldProfit = apiHold + (price-apiPrice)*count
	} else if price > 0 && cost > 0 {
		v.HoldProfit = (price - cost) * count
		v.Live = true
	}
	// 浮动盈亏比 = 持仓盈亏 / 成本(成本价×数量)；与插件 Z=I/N 一致
	costBasis := cost * count
	if costBasis != 0 {
		v.HoldRate = v.HoldProfit / costBasis * 100
	} else if apiHoldRate != 0 {
		v.HoldRate = apiHoldRate * 100
	}

	// 今日涨幅（价格涨跌幅）：有昨收即可，与是否成交无关
	if price > 0 && prev > 0 {
		v.DayRate = (price - prev) / prev * 100
		v.DayOK = true
	}

	// 今日盈亏：无昨收且无成交则无法算
	if price <= 0 {
		return v
	}
	if prev <= 0 && len(trades) == 0 {
		return v
	}
	prevForPnL := prev
	if prevForPnL <= 0 {
		// 有成交但无昨收：昨收差价段为 0
		prevForPnL = price
	}

	var h, m, feeSum, buyCount, flashG, remainC float64
	var hasFlash bool
	var costChg float64
	for _, t := range trades {
		feeSum += t.Fee
		n := t.Price
		o := t.Count
		switch t.OpType {
		case OpBuy:
			buyCount += o
			h += o * (price - n) // 买入收益 = 数量×(现价−买入价)
		case OpSell:
			h += o * (n - prevForPnL) // 卖出收益 = 数量×(卖出价−昨收)
			m += o * n
		case OpCollateralIn:
			buyCount += o
		case OpCollateralOut:
			h += o * (price - prevForPnL)
			m += o * price
		case OpFlash:
			hasFlash = true
			costChg = t.CostChange
			remainC = t.RemainChange
		}
	}
	if hasFlash {
		if costChg < 0 {
			flashG = absFloat(remainC)*price + costChg
		} else {
			flashG = costChg - absFloat(remainC)*prevForPnL
		}
	}
	// 今日盈亏 = 买卖收益 + (现价−昨收)×(持仓−买入−正remain) − 费用 + 速记
	adjRemain := 0.0
	if remainC > 0 {
		adjRemain = remainC
	}
	h += (price-prevForPnL)*(count-buyCount-adjRemain) - feeSum + flashG
	v.DayProfit = h
	v.SellAmount = m
	v.DayOK = true

	// 当日盈亏比（插件 drykb，合计行用）
	den := v.Value - h + m
	if den != 0 {
		v.DayYKRate = h / den * 100
	}
	return v
}

func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
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

// moneyHistoryPage 是 get_money_history 单页响应。
type moneyHistoryPage struct {
	List    []map[string]any `json:"list"`
	Page    Num              `json:"page"`
	MaxPage Num              `json:"max_page"`
	Count   Num              `json:"count"` // 本页条数或页大小
	Total   Num              `json:"total"` // 总条数
	H5ID    string           `json:"h5id"`
}

// GetMoneyHistory 拉取某账户近 3 个月资金/交易流水（自动翻页，最多 20 页）。
// 必须带齐 manual_id/fund_key/rzrq_fund_key/fundid/custid 等键（可为空串），否则服务端 HTTP 400。
func (c *Client) GetMoneyHistory(acc Account) ([]map[string]any, error) {
	end := time.Now()
	start := end.AddDate(0, -3, 0)
	base := map[string]string{
		"manual_id":     acc.ManualID,
		"fund_key":      acc.FundKey,
		"rzrq_fund_key": acc.RzrqFundKey,
		"fundid":        acc.fundID(),
		"custid":        "",
		"start_date":    start.Format("2006-01-02"),
		"end_date":      end.Format("2006-01-02"),
		"query_list":    "",
		"count":         "100",
		"sort_type":     "",
		"sort_order":    "0",
		"h5id":          "0",
	}

	var all []map[string]any
	h5id := "0"
	maxPages := 20
	for page := 1; page <= maxPages; page++ {
		params := map[string]string{}
		for k, v := range base {
			params[k] = v
		}
		params["page"] = strconv.Itoa(page)
		params["h5id"] = h5id

		ex, err := c.call(epMoneyHistory, params)
		if err != nil {
			return nil, err
		}
		pg, err := parseMoneyHistoryPage(ex)
		if err != nil {
			return nil, err
		}
		if pg.H5ID != "" {
			h5id = pg.H5ID
		}
		all = append(all, pg.List...)

		maxPage := int(pg.MaxPage.F())
		if maxPage <= 0 {
			// 无 max_page 时：本页不足一页则结束
			if len(pg.List) < 100 {
				break
			}
			continue
		}
		if page >= maxPage {
			break
		}
		if len(pg.List) == 0 {
			break
		}
	}
	return all, nil
}

func parseMoneyHistoryPage(ex json.RawMessage) (moneyHistoryPage, error) {
	var pg moneyHistoryPage
	if len(ex) == 0 || string(ex) == "null" {
		return pg, nil
	}
	if err := json.Unmarshal(ex, &pg); err == nil && (pg.List != nil || pg.MaxPage != 0 || pg.Total != 0) {
		if pg.List == nil {
			pg.List = []map[string]any{}
		}
		return pg, nil
	}
	// 兼容旧形态
	recs := extractRecords(ex)
	pg.List = recs
	return pg, nil
}

// extractRecords 从 ex_data 里尽力提取记录数组。
// get_money_history 常见形态：{"list":[...],"count":N,"page":1,...}；也兼容顶层数组。
func extractRecords(ex json.RawMessage) []map[string]any {
	if len(ex) == 0 || string(ex) == "null" {
		return nil
	}
	// 优先 list 字段（流水接口标准结构）
	var wrap struct {
		List []map[string]any `json:"list"`
	}
	if err := json.Unmarshal(ex, &wrap); err == nil && wrap.List != nil {
		return wrap.List
	}
	var arr []map[string]any
	if err := json.Unmarshal(ex, &arr); err == nil {
		return arr
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(ex, &obj); err == nil {
		// 优先常见键
		for _, key := range []string{"list", "data", "stock", "records", "history"} {
			if v, ok := obj[key]; ok {
				var inner []map[string]any
				if err := json.Unmarshal(v, &inner); err == nil {
					return inner
				}
			}
		}
		for _, v := range obj {
			var inner []map[string]any
			if err := json.Unmarshal(v, &inner); err == nil && len(inner) > 0 {
				return inner
			}
		}
	}
	return nil
}
