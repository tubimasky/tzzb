package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

// 股票仓位分析（梯队 B，stockAnalysisSummary/*）。纯股票口径：
// fundkey 用 stock 账户 fund_key（不是基金 fundid，填错会返回空数组）。

// analysisParams 构造梯队 B 的集约参数（stockzcfx m() 口径）。
// 注意字段名与梯队 A 不同：这里是 fundkey（无下划线）。
func analysisParams(p CalendarParams) map[string]string {
	return map[string]string{
		"zjzh":          p.Zjzh,
		"qsid":          p.Qsid,
		"fundkey":       p.StockFundKey,
		"rzrq_fund_key": "",
		"manual_id":     p.ManualID,
		"from_pc":       "1",
	}
}

// PositionDistItem 是持仓分布的一只标的。
type PositionDistItem struct {
	Code   string  `json:"code"`
	Name   string  `json:"name"`
	Value  float64 `json:"value"`  // 市值
	Market string  `json:"market"` // 1=深 2=沪
}

// posDistExData 是 position_distribution 的 ex_data。
type posDistExData struct {
	TotalValue   Num `json:"total_value"`
	PositionList []struct {
		StockCode  string `json:"stock_code"`
		StockName  string `json:"stock_name"`
		StockValue Num    `json:"stock_value"`
		Market     string `json:"market"`
	} `json:"position_list"`
}

// GetPositionDistribution 拉取持仓分布（按市值）。start/end 格式 YYYYMMDD。
func (c *Client) GetPositionDistribution(p CalendarParams, start, end string) ([]PositionDistItem, float64, error) {
	if p.empty() {
		return nil, 0, nil
	}
	params := analysisParams(p)
	params["start_date"] = start
	params["end_date"] = end

	ex, err := c.call(epAnalysisPosDist, params)
	if err != nil {
		return nil, 0, err
	}
	var d posDistExData
	if err := json.Unmarshal(ex, &d); err != nil {
		return nil, 0, fmt.Errorf("解析持仓分布失败: %v", err)
	}

	var out []PositionDistItem
	for _, it := range d.PositionList {
		out = append(out, PositionDistItem{
			Code:   it.StockCode,
			Name:   it.StockName,
			Value:  it.StockValue.F(),
			Market: it.Market,
		})
	}
	return out, d.TotalValue.F(), nil
}

// AccountPosRate 是单个账户的仓位情况。
type AccountPosRate struct {
	Name        string  // 账户名
	Broker      string  // 券商名（qsmc）
	StockValue  float64 // 持仓市值
	MoneyRemain float64 // 可用现金
	Zjzh        string  // 资金账号
}

// Total 返回账户总资产（市值 + 现金）。
func (a AccountPosRate) Total() float64 { return a.StockValue + a.MoneyRemain }

// Rate 返回仓位率（%）。
func (a AccountPosRate) Rate() float64 {
	total := a.Total()
	if total <= 0 {
		return 0
	}
	return a.StockValue / total * 100
}

// posRateExData 是 accountPositionRate 的 ex_data。
type posRateExData struct {
	AccountList []struct {
		Name        string `json:"name"`
		Qsmc        string `json:"qsmc"`
		StockTotal  Num    `json:"stock_total"`
		MoneyRemain Num    `json:"money_remain"`
		Zjzh        string `json:"zjzh"`
	} `json:"account_list"`
}

// GetAccountPositionRate 拉取各股票账户的仓位率。start/end 格式 YYYYMMDD。
// type=0 表示全部口径（对齐插件 analysis 的 total 枚举）。
func (c *Client) GetAccountPositionRate(p CalendarParams, start, end string) ([]AccountPosRate, error) {
	if p.empty() {
		return nil, nil
	}
	params := analysisParams(p)
	params["start_date"] = start
	params["end_date"] = end
	params["type"] = "0"

	ex, err := c.call(epAnalysisPosRate, params)
	if err != nil {
		return nil, err
	}
	var d posRateExData
	if err := json.Unmarshal(ex, &d); err != nil {
		return nil, fmt.Errorf("解析账户仓位率失败: %v", err)
	}

	var out []AccountPosRate
	for _, a := range d.AccountList {
		name := a.Name
		if name == "" {
			name = a.Qsmc
		}
		out = append(out, AccountPosRate{
			Name:        strings.TrimSpace(name),
			Broker:      a.Qsmc,
			StockValue:  a.StockTotal.F(),
			MoneyRemain: a.MoneyRemain.F(),
			Zjzh:        a.Zjzh,
		})
	}
	return out, nil
}
