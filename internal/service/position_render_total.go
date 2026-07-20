package service

import (
	"fmt"
	"os"
	"strings"

	"github.com/liyanran/tzzb-cli/internal/formatter"
	"github.com/olekukonko/tablewriter"
)

// RenderTotal 汇总表格渲染
func (s *RenderService) RenderTotal(merged []MergedPosition, ctx *LiveContext, sortBy string, sortAsc bool) {
	views := make([]TotalRowView, 0, len(merged))
	var totalValue, totalProfit, totalDay, totalCount, totalCostBasis, totalSell float64
	var hasDay bool

	for _, m := range merged {
		lv := s.positionService.RecomputeMerged(m, ctx)
		totalValue += lv.Value
		totalProfit += lv.HoldProfit
		totalCount += m.Count
		totalCostBasis += m.Cost * m.Count
		totalSell += lv.SellAmount
		if lv.DayOK {
			totalDay += lv.DayProfit
			hasDay = true
		}
		views = append(views, TotalRowView{
			Code:       m.Code,
			Name:       m.Name,
			Count:      m.Count,
			Cost:       m.Cost,
			Price:      lv.Price,
			Value:      lv.Value,
			HoldProfit: lv.HoldProfit,
			HoldRate:   lv.HoldRate,
			DayProfit:  lv.DayProfit,
			DayRate:    lv.DayRate,
			Accounts:   m.Accounts,
		})
	}

	s.positionService.SortTotalViews(views, sortBy, sortAsc)

	// 合计「收益率」= 总持仓盈亏 / 总成本
	totalRate := 0.0
	if totalCostBasis != 0 {
		totalRate = totalProfit / totalCostBasis * 100
	}
	// 合计行「今日涨幅」= 组合持仓当日盈亏比（相对总市值，非账户总资产）：
	//   总今日盈亏 / (总市值 − 总今日盈亏 + 总卖出额)
	// 与单票 drykb 同一套分母；现金不摊薄。账本 now_rate 用总资产，口径不同。
	totalDayRate := 0.0
	totalDayRateOK := false
	if hasDay {
		den := totalValue - totalDay + totalSell
		if den != 0 {
			totalDayRate = totalDay / den * 100
			totalDayRateOK = true
		}
	}

	showDay := ctx != nil && len(ctx.Quotes) > 0
	table := tablewriter.NewWriter(os.Stdout)
	if showDay {
		table.SetHeader([]string{"代码", "名称", "总数量", "均成本", "现价", "总市值", "占比", "持仓盈亏", "今日盈亏", "今日涨幅", "收益率", "账户"})
	} else {
		table.SetHeader([]string{"代码", "名称", "总数量", "均成本", "现价", "总市值", "占比", "持仓盈亏", "收益率", "账户"})
	}
	table.SetBorder(true)
	table.SetRowLine(false)

	for _, v := range views {
		weight := formatter.WeightStr(v.Value, totalValue)
		dayOK := v.DayProfit != 0 || hasDay
		rateOK := false
		if q := s.positionService.QuoteOf(ctx, v.Code); q != nil && q.PrevClose > 0 && v.Price > 0 {
			rateOK = true
		}
		if showDay {
			table.Append([]string{
				v.Code,
				v.Name,
				fmt.Sprintf("%.0f", v.Count),
				formatter.PriceStr(v.Cost),
				formatter.PriceStr(v.Price),
				fmt.Sprintf("%.2f", v.Value),
				weight,
				formatter.ColorNum(v.HoldProfit),
				formatter.DayStr(v.DayProfit, dayOK),
				formatter.RateStr(v.DayRate, rateOK),
				formatter.ColorPct(v.HoldRate),
				strings.Join(v.Accounts, ","),
			})
		} else {
			table.Append([]string{
				v.Code,
				v.Name,
				fmt.Sprintf("%.0f", v.Count),
				formatter.PriceStr(v.Cost),
				formatter.PriceStr(v.Price),
				fmt.Sprintf("%.2f", v.Value),
				weight,
				formatter.ColorNum(v.HoldProfit),
				formatter.ColorPct(v.HoldRate),
				strings.Join(v.Accounts, ","),
			})
		}
	}

	if showDay {
		table.Append([]string{
			"合计",
			fmt.Sprintf("%d只", len(merged)),
			fmt.Sprintf("%.0f", totalCount),
			"-",
			"-",
			fmt.Sprintf("%.2f", totalValue),
			formatter.WeightStr(totalValue, totalValue),
			formatter.ColorNum(totalProfit),
			formatter.DayStr(totalDay, hasDay),
			formatter.RateStr(totalDayRate, totalDayRateOK),
			formatter.ColorPct(totalRate),
			"",
		})
	} else {
		table.Append([]string{
			"合计",
			fmt.Sprintf("%d只", len(merged)),
			fmt.Sprintf("%.0f", totalCount),
			"-",
			"-",
			fmt.Sprintf("%.2f", totalValue),
			formatter.WeightStr(totalValue, totalValue),
			formatter.ColorNum(totalProfit),
			formatter.ColorPct(totalRate),
			"",
		})
	}
	table.Render()
}
