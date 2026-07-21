package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// StockAccountRef 是 stock/getAccounts 返回的股票账户标识，
// 关键是拿到 account_list 里没有的 zjzh（资金账号）——盈亏日历接口按 qsid+zjzh 匹配账户。
type StockAccountRef struct {
	Zjzh    string `json:"zjzh"`
	Qsid    string `json:"qsid"`
	FundKey string `json:"fund_key"`
	Type    string `json:"-"` // common / rzrq（由分组填充）
}

// stockAccountsResp 是 stock/getAccounts 的 ex_data 结构。
// list 按类型分组：common(普通同步)/rzrq(两融)。
type stockAccountsResp struct {
	List struct {
		Common []StockAccountRef `json:"common"`
		Rzrq   []StockAccountRef `json:"rzrq"`
	} `json:"list"`
}

// GetStockAccounts 拉取股票账户列表，返回带 zjzh 的账户引用。
// 盈亏日历（combineCalendar）按 qsid+zjzh 匹配 stock 账户，而 account_list 不返回 zjzh，
// 必须调这个接口补齐。参数与插件 requestUserList_stock 对齐（id=773777, accountType=all_plus）。
func (c *Client) GetStockAccounts() ([]StockAccountRef, error) {
	params := map[string]string{
		"id":          "773777",
		"accountType": "all_plus",
		"userid":      c.uid,
		"terminal":    "1",
		"version":     "1",
		"device":      "1",
	}

	ex, err := c.call(epStockGetAccounts, params)
	if err != nil {
		return nil, err
	}

	var resp stockAccountsResp
	if err := json.Unmarshal(ex, &resp); err != nil {
		return nil, fmt.Errorf("解析股票账户列表失败: %v", err)
	}

	var out []StockAccountRef
	for _, a := range resp.List.Common {
		a.Type = "common"
		out = append(out, a)
	}
	for _, a := range resp.List.Rzrq {
		a.Type = "rzrq"
		out = append(out, a)
	}
	return out, nil
}

// CalendarParams 是盈亏日历汇总口径的账户聚合参数。
// combineCalendar 是「全账户合并」视图：把纳入的账户标识按类型连接成逗号分隔串。
type CalendarParams struct {
	Fundid       string // 各基金 fundId 连接
	FundTypes    string // 各基金 type 连接（manFund 等）
	Qsid         string // 各股票账户 qsid 连接
	Zjzh         string // 各股票账户 zjzh 连接
	StockTypes   string // 各股票账户 cus_stockType 连接（common/rzrq），与 Qsid/Zjzh 同序
	StockFundKey string // 各股票账户 fund_key 连接（stock 侧盈亏/分析接口的 fundkey，非基金 fundid）
	ManualID     string // 手动账户 manual_id 连接
}

// empty 判断是否没有任何账户标识（无法查询）。
func (p CalendarParams) empty() bool {
	return p.Fundid == "" && p.Qsid == "" && p.ManualID == ""
}

// CalendarPoint 是盈亏日历的单个时间点（某日或某月）。
type CalendarPoint struct {
	Date    string  `json:"date"`     // 20260720（日）或 202607（月）
	StockPL float64 `json:"stock_pl"` // 股票盈亏
	FundPL  float64 `json:"fund_pl"`  // 基金盈亏
	TotalPL float64 `json:"total_pl"` // 合计盈亏
}

// CalendarCodePL 是区间内单只标的盈亏（profitlossCode）。
type CalendarCodePL struct {
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	ProfitLoss float64 `json:"profit_loss"`
	Type       string  `json:"type"` // stock / fund
}

// CalendarResult 是一次日历查询的完整结果。
type CalendarResult struct {
	Points  []CalendarPoint  // 日历主数据（按日或按月）
	Codes   []CalendarCodePL // 区间内个股盈亏
	TotalPL float64          // 区间合计
	StockPL float64
	FundPL  float64
	OtherPL float64
}

// calendarExData 是 combineCalendar 接口的 ex_data 结构。
type calendarExData struct {
	TotalProfit    Num `json:"totalProfit"`
	StockProfit    Num `json:"stockProfit"`
	FundProfit     Num `json:"fundProfit"`
	OtherProfit    Num `json:"otherProfit"`
	ProfitlossList []struct {
		Date            string `json:"date"`
		StockProfitloss Num    `json:"stockProfitloss"`
		FundProfitloss  Num    `json:"fundProfitloss"`
		TotalProfitloss Num    `json:"totalProfitloss"`
	} `json:"profitlossList"`
	ProfitlossCode []struct {
		Code       string `json:"code"`
		Name       string `json:"name"`
		Profitloss Num    `json:"profitloss"`
		Type       string `json:"type"`
	} `json:"profitlossCode"`
}

// BuildCalendarParams 从账户列表构造盈亏日历的聚合参数。
// stockRefs 来自 GetStockAccounts（提供 zjzh）；accounts 来自 GetAccountList（提供 fund/manual 信息）。
// stock 账户靠 fund_key 与 stockRefs 关联：只有 accounts 里出现的 fund_key 才纳入（支持单账户过滤）。
func BuildCalendarParams(accounts []Account, stockRefs []StockAccountRef) CalendarParams {
	var fundids, fundTypes, qsids, zjzhs, stockTypes, stockFundKeys, manualIDs []string

	// 收集本次纳入的 stock 账户 fund_key，用于筛选 stockRefs。
	stockKeys := map[string]bool{}

	for _, a := range accounts {
		switch a.Category {
		case "fund":
			fundids = append(fundids, a.fundID())
			ft := a.FundType
			if ft == "" {
				ft = "manFund"
			}
			fundTypes = append(fundTypes, ft)
		case "manual":
			manualIDs = append(manualIDs, a.ManualID)
		case "common", "rzrq":
			if a.FundKey != "" {
				stockKeys[a.FundKey] = true
			}
		}
	}

	// stock 账户（common/rzrq）：qsid + zjzh + type，来自 getAccounts（唯一提供 zjzh 的地方）。
	for _, r := range stockRefs {
		if !stockKeys[r.FundKey] {
			continue
		}
		qsids = append(qsids, r.Qsid)
		zjzhs = append(zjzhs, r.Zjzh)
		stockTypes = append(stockTypes, r.Type)
		stockFundKeys = append(stockFundKeys, r.FundKey)
	}

	return CalendarParams{
		Fundid:       strings.Join(fundids, ","),
		FundTypes:    strings.Join(fundTypes, ","),
		Qsid:         strings.Join(qsids, ","),
		Zjzh:         strings.Join(zjzhs, ","),
		StockTypes:   strings.Join(stockTypes, ","),
		StockFundKey: strings.Join(stockFundKeys, ","),
		ManualID:     strings.Join(manualIDs, ","),
	}
}

// GetCalendarDaily 拉取某月每日盈亏。month 格式 YYYYMM。
func (c *Client) GetCalendarDaily(p CalendarParams, month string) (*CalendarResult, error) {
	if len(month) != 6 {
		return nil, fmt.Errorf("month 格式应为 YYYYMM，得到: %s", month)
	}
	if p.empty() {
		return &CalendarResult{}, nil
	}
	start, end, err := monthRange(month)
	if err != nil {
		return nil, err
	}

	params := map[string]string{
		"id":          "11" + month,
		"startdate":   start,
		"enddate":     end,
		"fundid":      p.Fundid,
		"fund_types":  p.FundTypes,
		"stock_types": p.StockTypes,
		"manual_id":   p.ManualID,
		"zjzh":        p.Zjzh,
		"qsid":        p.Qsid,
	}
	return c.callCalendar(epCalDaily, params)
}

// GetCalendarMonthly 拉取某年每月盈亏。year 格式 YYYY。
func (c *Client) GetCalendarMonthly(p CalendarParams, year string) (*CalendarResult, error) {
	if len(year) != 4 {
		return nil, fmt.Errorf("year 格式应为 YYYY，得到: %s", year)
	}
	if p.empty() {
		return &CalendarResult{}, nil
	}

	params := map[string]string{
		"id":          "13" + year,
		"year":        year,
		"fundid":      p.Fundid,
		"fund_types":  p.FundTypes,
		"stock_types": p.StockTypes,
		"manual_id":   p.ManualID,
		"zjzh":        p.Zjzh,
		"qsid":        p.Qsid,
	}
	return c.callCalendar(epCalMonthly, params)
}

// callCalendar 发请求并把 ex_data 解析成 CalendarResult。
func (c *Client) callCalendar(path string, params map[string]string) (*CalendarResult, error) {
	ex, err := c.call(path, params)
	if err != nil {
		return nil, err
	}
	var d calendarExData
	if err := json.Unmarshal(ex, &d); err != nil {
		return nil, fmt.Errorf("解析盈亏日历失败: %v", err)
	}

	result := &CalendarResult{
		TotalPL: d.TotalProfit.F(),
		StockPL: d.StockProfit.F(),
		FundPL:  d.FundProfit.F(),
		OtherPL: d.OtherProfit.F(),
	}
	for _, p := range d.ProfitlossList {
		result.Points = append(result.Points, CalendarPoint{
			Date:    p.Date,
			StockPL: p.StockProfitloss.F(),
			FundPL:  p.FundProfitloss.F(),
			TotalPL: p.TotalProfitloss.F(),
		})
	}
	for _, cpl := range d.ProfitlossCode {
		result.Codes = append(result.Codes, CalendarCodePL{
			Code:       cpl.Code,
			Name:       cpl.Name,
			ProfitLoss: cpl.Profitloss.F(),
			Type:       cpl.Type,
		})
	}
	return result, nil
}

// monthRange 返回某月的起止日期 YYYY-MM-DD。month 格式 YYYYMM。
func monthRange(month string) (start, end string, err error) {
	t, err := time.Parse("200601", month)
	if err != nil {
		return "", "", fmt.Errorf("month 格式应为 YYYYMM，得到: %s", month)
	}
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1)
	return first.Format("2006-01-02"), last.Format("2006-01-02"), nil
}
