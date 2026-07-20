package api

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

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

// AssetTrendPoint 是资产趋势曲线的单个数据点。
type AssetTrendPoint struct {
	Date         string  `json:"date"`          // 日期 YYYY-MM-DD
	TotalAsset   float64 `json:"total_asset"`   // 总资产
	Profit       float64 `json:"profit"`        // 持仓盈亏
	ProfitRate   float64 `json:"profit_rate"`   // 收益率（小数）
	Cash         float64 `json:"cash"`          // 现金
	MarketValue  float64 `json:"market_value"`  // 市值
	PositionRate float64 `json:"position_rate"` // 仓位（小数）
}

// GetAssetTrend 拉取账户资产趋势曲线数据。
// days 指定查询天数，默认 30 天。
func (c *Client) GetAssetTrend(acc Account, days int) ([]AssetTrendPoint, error) {
	if days <= 0 {
		days = 30
	}

	params := map[string]string{
		"days": fmt.Sprintf("%d", days),
	}

	// 按账户类型填充标识参数
	if acc.Category == "fund" {
		params["fundid"] = acc.fundID()
	} else {
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

	ex, err := c.call(epAssetTrend, params)
	if err != nil {
		return nil, err
	}

	// 实际格式: {"total_asset": [...], "month_profit": [...], "year_profit": [...], "month_init_zczs": 938.69, "year_init_zczs": 1000}
	var wrap struct {
		TotalAsset    []map[string]any `json:"total_asset"`
		MonthProfit   []map[string]any `json:"month_profit"`
		YearProfit    []map[string]any `json:"year_profit"`
		MonthInitZczs Num              `json:"month_init_zczs"`
		YearInitZczs  Num              `json:"year_init_zczs"`
	}
	if err := json.Unmarshal(ex, &wrap); err == nil {
		// DEBUG: 输出各数据集长度
		if os.Getenv("DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "DEBUG: total_asset=%d, month_profit=%d, year_profit=%d, month_init_zczs=%.2f, year_init_zczs=%.2f\n",
				len(wrap.TotalAsset), len(wrap.MonthProfit), len(wrap.YearProfit), wrap.MonthInitZczs.F(), wrap.YearInitZczs.F())
		}

		// 根据 days 参数决定用哪个数据集和对应的基准
		var data []map[string]any
		var initZczs float64

		if days <= 31 && len(wrap.MonthProfit) > 0 {
			// 近一个月用 month_profit
			data = wrap.MonthProfit
			initZczs = wrap.MonthInitZczs.F()
		} else if days > 60 && len(wrap.YearProfit) > 0 {
			// 长期查询（>60 天）用 year_profit（年度完整数据）
			data = wrap.YearProfit
			initZczs = wrap.YearInitZczs.F()
		} else if len(wrap.TotalAsset) > 0 {
			// 中期（32-60 天）用 total_asset（最近 30 天完整数据）
			data = wrap.TotalAsset
			// total_asset 跨月，用 month_init_zczs
			initZczs = wrap.MonthInitZczs.F()
		} else if len(wrap.YearProfit) > 0 {
			// 回退到 year_profit
			data = wrap.YearProfit
			initZczs = wrap.YearInitZczs.F()
		}

		if len(data) > 0 {
			if initZczs == 0 {
				initZczs = 1000 // 默认基准
			}
			return parseAssetTrendData(data, initZczs), nil
		}
	}

	// 兼容其他格式
	var wrap2 struct {
		Data []map[string]any `json:"data"`
		List []map[string]any `json:"list"`
	}
	if err := json.Unmarshal(ex, &wrap2); err == nil {
		if len(wrap2.Data) > 0 {
			return parseAssetTrendData(wrap2.Data, 1000), nil
		}
		if len(wrap2.List) > 0 {
			return parseAssetTrendData(wrap2.List, 1000), nil
		}
	}

	return nil, nil
}

// parseAssetTrendData 从原始 map 数组解析成 AssetTrendPoint。
// initZczs 是收益率计算基准（如 938.69 或 1000）。
func parseAssetTrendData(raw []map[string]any, initZczs float64) []AssetTrendPoint {
	var points []AssetTrendPoint

	for _, m := range raw {
		p := AssetTrendPoint{}

		// 日期字段：date / time / day
		if v, ok := m["date"]; ok {
			dateStr := fmt.Sprintf("%v", v)
			// 转换 20260622 → 2026-06-22
			if len(dateStr) == 8 {
				p.Date = dateStr[0:4] + "-" + dateStr[4:6] + "-" + dateStr[6:8]
			} else {
				p.Date = dateStr
			}
		} else if v, ok := m["time"]; ok {
			p.Date = fmt.Sprintf("%v", v)
		} else if v, ok := m["day"]; ok {
			p.Date = fmt.Sprintf("%v", v)
		}

		// 总资产：asset / total_asset / total_assets
		p.TotalAsset = parseFloat(m, "asset", "total_asset", "total_assets", "totalAsset")

		// 盈亏：profit / hold_profit / total_profit
		p.Profit = parseFloat(m, "profit", "hold_profit", "total_profit")

		// 收益率：从 zczs（增长指数）计算
		// zczs 是以基准（month_init_zczs 或 1000）为基准的指数
		// 收益率 = (zczs - initZczs) / initZczs
		if zczs := parseFloat(m, "zczs"); zczs > 0 && initZczs > 0 {
			p.ProfitRate = (zczs - initZczs) / initZczs
		} else {
			// 回退：profit_rate / rate / return_rate
			rate := parseFloat(m, "profit_rate", "rate", "return_rate", "profitRate")
			if rate > 1 || rate < -1 {
				rate = rate / 100
			}
			p.ProfitRate = rate
		}

		// 资金流入流出
		fundIn := parseFloat(m, "fundIn", "fund_in")
		fundOut := parseFloat(m, "fundOut", "fund_out")

		// 现金：从 asset - profit 估算（如果没有直接字段）
		cash := parseFloat(m, "cash", "money_remain", "available", "moneyRemain")
		if cash == 0 && p.TotalAsset > 0 {
			// 估算：现金 = 总资产 - 市值
			// 但我们还不知道市值，先留 0
		}
		p.Cash = cash

		// 市值：优先 market_value，否则从资产和现金推算
		marketValue := parseFloat(m, "market_value", "value", "total_value", "marketValue")
		if marketValue == 0 && p.TotalAsset > 0 {
			// 如果有资金流入流出，调整后的市值
			if fundIn > 0 || fundOut > 0 {
				// 简化：不考虑资金流，直接用总资产作为市值近似
				marketValue = p.TotalAsset
			} else if cash > 0 {
				marketValue = p.TotalAsset - cash
			} else {
				// 假设全仓
				marketValue = p.TotalAsset
			}
		}
		p.MarketValue = marketValue

		// 现金回填（如果之前为 0）
		if p.Cash == 0 && p.TotalAsset > 0 && p.MarketValue > 0 {
			p.Cash = p.TotalAsset - p.MarketValue
		}

		// 仓位：position_rate / position（小数或百分比）
		pos := parseFloat(m, "position_rate", "position", "positionRate")
		if pos > 1 {
			pos = pos / 100
		}
		// 如果没有仓位字段，从市值和总资产计算
		if pos == 0 && p.TotalAsset > 0 && p.MarketValue > 0 {
			pos = p.MarketValue / p.TotalAsset
		}
		p.PositionRate = pos

		points = append(points, p)
	}

	return points
}

// parseFloat 从 map 中按候选键顺序提取第一个非零数值。
func parseFloat(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}

		switch x := v.(type) {
		case float64:
			return x
		case float32:
			return float64(x)
		case int:
			return float64(x)
		case int64:
			return float64(x)
		case json.Number:
			f, _ := x.Float64()
			return f
		case string:
			s := strings.TrimSpace(x)
			s = strings.ReplaceAll(s, ",", "")
			if s == "" || s == "null" {
				continue
			}
			f, err := strconv.ParseFloat(s, 64)
			if err == nil {
				return f
			}
		}
	}
	return 0
}
