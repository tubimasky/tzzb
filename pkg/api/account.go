package api

import (
	"encoding/json"
	"fmt"
)

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
