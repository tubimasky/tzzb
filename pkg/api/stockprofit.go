package api

import (
	"encoding/json"
	"fmt"
	"time"
)

// 股票时间维度盈亏（梯队 A）。与 combineCalendar 同源账户口径，但多返回
// profitloss_rate（账户收益率）与 sz（同期大盘涨跌），可做大盘对比。
//
// 两个坑（抓包验证）：
//  1. startdate/enddate 用 YYYYMMDD 无横杠；带横杠会返回 -2 服务器繁忙。
//  2. fund_key 用 stock 账户 fund_key（与 qsid/zjzh 对齐），不是基金 fundid。
//     服务端主要靠 zjzh+qsid 匹配，fund_key 冗余，但按插件口径带上更稳妥。

// StockPLPoint 是股票盈亏的单个时间点（某月或某日）。
type StockPLPoint struct {
	Date       string  `json:"date"`        // 202607（月）或 20260720（日）
	ProfitLoss float64 `json:"profit_loss"` // 盈亏金额
	Rate       float64 `json:"rate"`        // 账户收益率（小数，如 -0.0576）
	SZRate     float64 `json:"sz_rate"`     // 同期大盘涨跌（小数）
}

// StockPLResult 是一次股票盈亏查询的结果。
type StockPLResult struct {
	Points     []StockPLPoint   // 按时间的盈亏序列
	Codes      []CalendarCodePL // 区间内个股盈亏
	TotalPL    float64          // 区间合计盈亏
	TotalRate  float64          // 区间合计收益率（小数）
	TotalSZ    float64          // 区间大盘涨跌（小数）
}

// stockPLExData 是 monthlyProfitLoss / profitlossByCode 的 ex_data 结构。
type stockPLExData struct {
	TotalProfit     Num `json:"totalProfit"`
	TotalProfitRate Num `json:"totalProfitRate"`
	TotalSZ         Num `json:"totalSz"`
	ProfitlossList  []struct {
		Date          string `json:"date"`
		Profitloss    Num    `json:"profitloss"`
		ProfitlossRate Num   `json:"profitloss_rate"`
		SZ            Num    `json:"sz"`
	} `json:"profitlossList"`
	ProfitlossCode []struct {
		Code       string `json:"code"`
		Name       string `json:"name"`
		Profitloss Num    `json:"profitloss"`
		Type       string `json:"type"`
	} `json:"profitlossCode"`
}

// stockPLParams 构造梯队 A 的集约参数（全账户合算口径 + 计算开关常量）。
func stockPLParams(p CalendarParams) map[string]string {
	return map[string]string{
		"cus_stockType": "all",
		"types":         p.StockTypes,
		"merge_type":    p.StockTypes,
		"zjzh":          p.Zjzh,
		"qsid":          p.Qsid,
		"fund_key":      p.StockFundKey,
		"rzrq_fund_key": "",
		"manual_id":     p.ManualID,
		"from_pc":       "1",
		"terminal":      "123",
		"version":       "11",
		// 计算开关（插件常量 G）：市值/交易开，持仓关。
		"sz_cal": "1",
		"jy_cal": "1",
		"cw_cal": "0",
	}
}

// GetStockMonthlyPL 拉取某年各月股票盈亏。year 格式 YYYY。
func (c *Client) GetStockMonthlyPL(p CalendarParams, year string) (*StockPLResult, error) {
	if len(year) != 4 {
		return nil, fmt.Errorf("year 格式应为 YYYY，得到: %s", year)
	}
	if p.empty() {
		return &StockPLResult{}, nil
	}
	params := stockPLParams(p)
	params["year"] = year
	return c.callStockPL(epStockMonthlyPL, params)
}

// GetStockRangePL 拉取某区间每日股票盈亏 + 区间内个股盈亏。
// start/end 格式 YYYYMMDD（无横杠）。
func (c *Client) GetStockRangePL(p CalendarParams, start, end string) (*StockPLResult, error) {
	if len(start) != 8 || len(end) != 8 {
		return nil, fmt.Errorf("日期格式应为 YYYYMMDD，得到: %s~%s", start, end)
	}
	if p.empty() {
		return &StockPLResult{}, nil
	}
	params := stockPLParams(p)
	params["startdate"] = start
	params["enddate"] = end
	return c.callStockPL(epStockCodePL, params)
}

// callStockPL 发请求并解析成 StockPLResult。
func (c *Client) callStockPL(path string, params map[string]string) (*StockPLResult, error) {
	ex, err := c.call(path, params)
	if err != nil {
		return nil, err
	}
	var d stockPLExData
	if err := json.Unmarshal(ex, &d); err != nil {
		return nil, fmt.Errorf("解析股票盈亏失败: %v", err)
	}

	result := &StockPLResult{
		TotalPL:   d.TotalProfit.F(),
		TotalRate: d.TotalProfitRate.F(),
		TotalSZ:   d.TotalSZ.F(),
	}
	for _, p := range d.ProfitlossList {
		result.Points = append(result.Points, StockPLPoint{
			Date:       p.Date,
			ProfitLoss: p.Profitloss.F(),
			Rate:       p.ProfitlossRate.F(),
			SZRate:     p.SZ.F(),
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

// FormatCompactDate 把 time 转成接口要求的 YYYYMMDD。
func FormatCompactDate(t time.Time) string { return t.Format("20060102") }
