package api

import (
	"encoding/json"
	"fmt"
)

// 单只股票钻取明细（对齐插件 handstock/itemDetail 的四个接口，抓包验证）。
// 四个接口都靠「股票代码 + 账户坐标」定位：账户坐标只需 fundkey/rzrq_fundkey/manualid
// 之一（与 account_list 的 fund_key/rzrq_fund_key/manual_id 对应，但这里字段名去掉下划线），
// stock_account 实测可留空。日期参数（history/clear）用 YYYYMMDD 无横杠。
//
// 四个接口：
//   merge_position_query  单只合并明细（持有天数/买入均价/现价/市值/成本/浮盈/费用/清仓标记）
//   total_position_query  建仓记录（建仓起始日 + 分段收益）
//   stock_history_query   逐笔历史成交（需 start/end）
//   clear_position_query  清仓汇总（未清仓返回空，需 start/end）

// DetailCoord 定位单只股票钻取所需的账户坐标 + 股票代码。
type DetailCoord struct {
	Fundkey     string // 普通账户 fund_key（插件字段名 fundkey，无下划线）
	RzrqFundkey string // 两融账户 rzrq_fund_key
	Manualid    string // 手动账户 id
	StockCode   string // 股票代码
	Market      string // 1=深 2=沪，展示用
	AccountName string // 账户名，展示用
}

// params 构造四个接口共用的账户 + 代码定位参数。
func (d DetailCoord) params() map[string]string {
	return map[string]string{
		"manualid":      d.Manualid,
		"fundkey":       d.Fundkey,
		"rzrq_fundkey":  d.RzrqFundkey,
		"stock_code":    d.StockCode,
		"stock_account": "",
		"from_pc":       "1",
	}
}

// StockMergeDetail 是单只股票的合并持仓明细（merge_position_query）。
type StockMergeDetail struct {
	StockCode   string  `json:"stock_code"`
	StockName   string  `json:"stock_name"`
	Count       float64 `json:"count"`        // 持仓数量
	HoldDays    int     `json:"hold_days"`    // 持有天数
	PriceBuyAv  float64 `json:"price_buy_av"` // 买入均价
	PriceCurr   float64 `json:"price_curr"`   // 现价
	MarketValue float64 `json:"market_value"` // 市值
	Zcb         float64 `json:"zcb"`          // 总成本
	Yke         float64 `json:"yke"`          // 浮动盈亏
	ProfitRate  float64 `json:"profit_rate"`  // 收益率（小数）
	Fee         float64 `json:"fee"`          // 费用
	ComeProfit  float64 `json:"come_profit"`  // 累计已实现收益
	ClearFlag   string  `json:"clear_flag"`   // 是否清仓（0/1）
}

// mergeDetailExData 是 merge_position_query 的 ex_data 原始结构。
type mergeDetailExData struct {
	StockCode   string  `json:"stock_code"`
	StockName   string  `json:"stock_name"`
	Count       Num     `json:"count"`
	HoldDays    Num     `json:"hold_days"`
	PriceBuyAv  Num     `json:"price_buy_av"`
	PriceCurr   Num     `json:"price_curr"`
	MarketValue Num     `json:"market_value"`
	Zcb         Num     `json:"zcb"`
	Yke         Num     `json:"yke"`
	ProfitRate  Num     `json:"profit_rate"`
	Fee         Num     `json:"fee"`
	ComeProfit  Num     `json:"come_profit"`
	ClearFlag   flexStr `json:"clear_flag"`
}

// GetStockMergeDetail 拉取单只股票的合并持仓明细。
func (c *Client) GetStockMergeDetail(coord DetailCoord) (*StockMergeDetail, error) {
	ex, err := c.call(epMergePosQuery, coord.params())
	if err != nil {
		return nil, err
	}
	var d mergeDetailExData
	if err := json.Unmarshal(ex, &d); err != nil {
		return nil, fmt.Errorf("解析合并持仓明细失败: %v", err)
	}
	return &StockMergeDetail{
		StockCode:   d.StockCode,
		StockName:   d.StockName,
		Count:       d.Count.F(),
		HoldDays:    int(d.HoldDays.F()),
		PriceBuyAv:  d.PriceBuyAv.F(),
		PriceCurr:   d.PriceCurr.F(),
		MarketValue: d.MarketValue.F(),
		Zcb:         d.Zcb.F(),
		Yke:         d.Yke.F(),
		ProfitRate:  d.ProfitRate.F(),
		Fee:         d.Fee.F(),
		ComeProfit:  d.ComeProfit.F(),
		ClearFlag:   d.ClearFlag.String(),
	}, nil
}

// HoldRecord 是一条建仓记录（total_position_query 的 list 元素）。
type HoldRecord struct {
	StartDate  string  `json:"start_date"`  // 建仓起始日 YYYYMMDD
	EndDate    string  `json:"end_date"`    // 结束日（持有中为空）
	Profit     float64 `json:"profit"`      // 分段收益
	ProfitRate float64 `json:"profit_rate"` // 分段收益率（小数）
}

// holdRecordsExData 是 total_position_query 的 ex_data。
type holdRecordsExData struct {
	List []struct {
		StartDate  string `json:"start_date"`
		EndDate    string `json:"end_date"`
		Profit     Num    `json:"profit"`
		ProfitRate Num    `json:"profit_rate"`
	} `json:"list"`
}

// GetStockHoldRecords 拉取单只股票的建仓记录。
func (c *Client) GetStockHoldRecords(coord DetailCoord) ([]HoldRecord, error) {
	ex, err := c.call(epTotalPosQuery, coord.params())
	if err != nil {
		return nil, err
	}
	var d holdRecordsExData
	if err := json.Unmarshal(ex, &d); err != nil {
		return nil, fmt.Errorf("解析建仓记录失败: %v", err)
	}
	var out []HoldRecord
	for _, r := range d.List {
		out = append(out, HoldRecord{
			StartDate:  r.StartDate,
			EndDate:    r.EndDate,
			Profit:     r.Profit.F(),
			ProfitRate: r.ProfitRate.F(),
		})
	}
	return out, nil
}

// StockTrade 是一笔历史成交（stock_history_query 的 list 元素）。
type StockTrade struct {
	TransDate   string  `json:"trans_date"`  // 成交日 YYYYMMDD
	TransTime   string  `json:"trans_time"`  // 成交时间 YYYYMMDDHHMMSS
	Op          string  `json:"op"`          // 方向：1 买入 / 2 卖出
	Price       float64 `json:"price"`       // 成交价
	Count       float64 `json:"count"`       // 成交数量
	Amount      float64 `json:"amount"`      // 成交金额
	Fee         float64 `json:"fee"`         // 费用
	Profit      float64 `json:"profit"`      // 该笔盈亏
	AccountName string  `json:"account_name"` // 所属账户名
}

// tradesExData 是 stock_history_query 的 ex_data。
type tradesExData struct {
	List []struct {
		TransDate    string `json:"trans_date"`
		TransDateTime string `json:"transDateTime"`
		Op           flexStr `json:"op"`
		TransPrice   Num    `json:"trans_price"`
		TransCount   Num    `json:"trans_count"`
		TransAmount  Num    `json:"trans_amount"`
		TransFee     Num    `json:"trans_fee"`
		Profit       Num    `json:"profit"`
		AccountName  string `json:"account_name"`
	} `json:"list"`
}

// GetStockTradeHistory 拉取单只股票的逐笔历史成交。start/end 格式 YYYYMMDD。
func (c *Client) GetStockTradeHistory(coord DetailCoord, start, end string) ([]StockTrade, error) {
	params := coord.params()
	params["start_date"] = start
	params["end_date"] = end
	ex, err := c.call(epStockHistoryQuery, params)
	if err != nil {
		return nil, err
	}
	var d tradesExData
	if err := json.Unmarshal(ex, &d); err != nil {
		return nil, fmt.Errorf("解析历史成交失败: %v", err)
	}
	var out []StockTrade
	for _, t := range d.List {
		out = append(out, StockTrade{
			TransDate:   t.TransDate,
			TransTime:   t.TransDateTime,
			Op:          t.Op.String(),
			Price:       t.TransPrice.F(),
			Count:       t.TransCount.F(),
			Amount:      t.TransAmount.F(),
			Fee:         t.TransFee.F(),
			Profit:      t.Profit.F(),
			AccountName: t.AccountName,
		})
	}
	return out, nil
}

// ClearSummary 是单只股票的清仓汇总（clear_position_query）。
type ClearSummary struct {
	StartDate       string  `json:"start_date"`
	EndDate         string  `json:"end_date"`
	HoldDays        int     `json:"hold_days"`
	Fee             float64 `json:"fee"`
	TotalBuy        float64 `json:"total_buy"`
	TotalSell       float64 `json:"total_sell"`
	TotalProfit     float64 `json:"total_profit"`
	TotalProfitRate float64 `json:"total_profit_rate"`
}

// HasData 判断是否有清仓记录（无清仓时买卖额均为 0）。
func (c ClearSummary) HasData() bool {
	return c.TotalBuy != 0 || c.TotalSell != 0 || c.TotalProfit != 0
}

// clearSummaryExData 是 clear_position_query 的 ex_data。
type clearSummaryExData struct {
	StartDate       string `json:"start_date"`
	EndDate         string `json:"end_date"`
	HoldDays        Num    `json:"hold_days"`
	Fee             Num    `json:"fee"`
	TotalBuy        Num    `json:"total_buy"`
	TotalSell       Num    `json:"total_sell"`
	TotalProfit     Num    `json:"total_profit"`
	TotalProfitRate Num    `json:"total_profit_rate"`
}

// GetStockClearSummary 拉取单只股票的清仓汇总。start/end 格式 YYYYMMDD。
// 未清仓时各金额字段为 0（用 HasData 判断）。
func (c *Client) GetStockClearSummary(coord DetailCoord, start, end string) (*ClearSummary, error) {
	params := coord.params()
	params["start_date"] = start
	params["end_date"] = end
	ex, err := c.call(epClearPosQuery, params)
	if err != nil {
		return nil, err
	}
	var d clearSummaryExData
	if err := json.Unmarshal(ex, &d); err != nil {
		return nil, fmt.Errorf("解析清仓汇总失败: %v", err)
	}
	return &ClearSummary{
		StartDate:       d.StartDate,
		EndDate:         d.EndDate,
		HoldDays:        int(d.HoldDays.F()),
		Fee:             d.Fee.F(),
		TotalBuy:        d.TotalBuy.F(),
		TotalSell:       d.TotalSell.F(),
		TotalProfit:     d.TotalProfit.F(),
		TotalProfitRate: d.TotalProfitRate.F(),
	}, nil
}
