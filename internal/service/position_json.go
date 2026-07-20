package service

import (
	"github.com/liyanran/tzzb-cli/internal/formatter"
	"github.com/liyanran/tzzb-cli/pkg/api"
)

// PositionJSON JSON 输出的持仓结构
type PositionJSON struct {
	Code       string   `json:"code"`
	Name       string   `json:"name"`
	Count      float64  `json:"count"`
	Cost       float64  `json:"cost"`
	Price      float64  `json:"price"`
	Value      float64  `json:"value"`
	HoldProfit float64  `json:"hold_profit"`
	HoldRate   float64  `json:"hold_rate"`
	DayProfit  *float64 `json:"day_profit,omitempty"`
	DayRate    *float64 `json:"day_rate,omitempty"`
	WeekProfit float64  `json:"week_profit"`
}

// AccountGroupJSON 按账户分组的 JSON 结构
type AccountGroupJSON struct {
	Account   string         `json:"account"`
	Category  string         `json:"category"`
	Positions []PositionJSON `json:"positions"`
	Summary   struct {
		Value      float64  `json:"value"`
		HoldProfit float64  `json:"hold_profit"`
		DayProfit  *float64 `json:"day_profit,omitempty"`
		Count      int      `json:"count"`
	} `json:"summary"`
}

// BuildByAccountJSON 构建按账户分组的 JSON 数据
func (s *RenderService) BuildByAccountJSON(rows []PositionRow, ctx *LiveContext) map[string]any {
	groups := s.GroupByAccount(rows)
	showDay := ctx != nil && len(ctx.Quotes) > 0

	var result []AccountGroupJSON
	var grandValue, grandProfit, grandDay float64
	var hasDay bool

	for _, group := range groups {
		useLive := showDay && group.Category != "fund"

		jsonGroup := AccountGroupJSON{
			Account:  group.Account,
			Category: api.CategoryLabel(group.Category),
		}

		var subValue, subProfit, subDay float64
		var subHasDay bool

		for _, r := range group.Rows {
			var lv api.LiveView
			if useLive {
				lv = s.positionService.RecomputePosition(r, ctx)
			} else {
				lv = s.positionService.RecomputePosition(r, nil)
			}

			pos := PositionJSON{
				Code:       r.Position.Code,
				Name:       r.Position.Name,
				Count:      r.Position.Count.F(),
				Cost:       r.Position.Cost.F(),
				Price:      lv.Price,
				Value:      lv.Value,
				HoldProfit: lv.HoldProfit,
				HoldRate:   lv.HoldRate,
				WeekProfit: r.Position.WProfit.F(),
			}
			if useLive && lv.DayOK {
				pos.DayProfit = &lv.DayProfit
				pos.DayRate = &lv.DayRate
			}
			jsonGroup.Positions = append(jsonGroup.Positions, pos)

			subValue += lv.Value
			subProfit += lv.HoldProfit
			if lv.DayOK && useLive {
				subDay += lv.DayProfit
				subHasDay = true
			}
		}

		jsonGroup.Summary.Value = subValue
		jsonGroup.Summary.HoldProfit = subProfit
		jsonGroup.Summary.Count = len(group.Rows)
		if subHasDay {
			jsonGroup.Summary.DayProfit = &subDay
		}

		result = append(result, jsonGroup)
		grandValue += subValue
		grandProfit += subProfit
		if subHasDay {
			grandDay += subDay
			hasDay = true
		}
	}

	summary := map[string]any{
		"value":       grandValue,
		"hold_profit": grandProfit,
		"rows":        len(rows),
		"accounts":    len(groups),
	}
	if hasDay {
		summary["day_profit"] = grandDay
	}

	return map[string]any{
		"accounts": result,
		"summary":  summary,
	}
}

// FlatPositionJSON 扁平视图 JSON 结构
type FlatPositionJSON struct {
	Account    string   `json:"account"`
	Category   string   `json:"category"`
	Code       string   `json:"code"`
	Name       string   `json:"name"`
	Count      float64  `json:"count"`
	Cost       float64  `json:"cost"`
	Price      float64  `json:"price"`
	Value      float64  `json:"value"`
	HoldProfit float64  `json:"hold_profit"`
	HoldRate   float64  `json:"hold_rate"`
	DayProfit  *float64 `json:"day_profit,omitempty"`
	DayRate    *float64 `json:"day_rate,omitempty"`
	WeekProfit float64  `json:"week_profit"`
}

// BuildFlatJSON 构建扁平表格 JSON 数据
func (s *RenderService) BuildFlatJSON(rows []PositionRow, ctx *LiveContext) map[string]any {
	views := make([]FlatView, 0, len(rows))
	for _, r := range rows {
		c := ctx
		if r.Category == "fund" {
			c = nil
		}
		views = append(views, FlatView{
			Row:      r,
			LiveView: s.positionService.RecomputePosition(r, c),
		})
	}

	showDay := ctx != nil && len(ctx.Quotes) > 0
	var result []FlatPositionJSON
	var totalValue, totalProfit, totalDay float64
	var hasDay bool

	for _, v := range views {
		r, lv := v.Row, v.LiveView
		totalValue += lv.Value
		totalProfit += lv.HoldProfit

		pos := FlatPositionJSON{
			Account:    r.Account,
			Category:   api.CategoryLabel(r.Category),
			Code:       r.Position.Code,
			Name:       r.Position.Name,
			Count:      r.Position.Count.F(),
			Cost:       r.Position.Cost.F(),
			Price:      lv.Price,
			Value:      lv.Value,
			HoldProfit: lv.HoldProfit,
			HoldRate:   lv.HoldRate,
			WeekProfit: r.Position.WProfit.F(),
		}

		if showDay && r.Category != "fund" && lv.DayOK {
			totalDay += lv.DayProfit
			hasDay = true
			pos.DayProfit = &lv.DayProfit
			pos.DayRate = &lv.DayRate
		}
		result = append(result, pos)
	}

	summary := map[string]any{
		"value":       totalValue,
		"hold_profit": totalProfit,
		"rows":        len(rows),
	}
	if hasDay {
		summary["day_profit"] = totalDay
	}

	return map[string]any{
		"positions": result,
		"summary":   summary,
	}
}

// TotalPositionJSON 汇总持仓 JSON 结构
type TotalPositionJSON struct {
	Code       string   `json:"code"`
	Name       string   `json:"name"`
	Count      float64  `json:"count"`
	Cost       float64  `json:"cost"`
	Price      float64  `json:"price"`
	Value      float64  `json:"value"`
	HoldProfit float64  `json:"hold_profit"`
	HoldRate   float64  `json:"hold_rate"`
	DayProfit  *float64 `json:"day_profit,omitempty"`
	DayRate    *float64 `json:"day_rate,omitempty"`
	Accounts   []string `json:"accounts"`
}

// BuildTotalJSON 构建汇总 JSON 数据
func (s *RenderService) BuildTotalJSON(merged []MergedPosition, ctx *LiveContext, sortBy string, sortAsc bool) map[string]any {
	showDay := ctx != nil && len(ctx.Quotes) > 0
	var result []TotalPositionJSON
	var totalValue, totalProfit, totalDay float64
	var hasDay bool

	// 转换为 TotalRowView 并排序
	views := make([]TotalRowView, 0, len(merged))
	for _, m := range merged {
		lv := s.positionService.RecomputeMerged(m, ctx)
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

	for _, v := range views {
		pos := TotalPositionJSON{
			Code:       v.Code,
			Name:       v.Name,
			Count:      v.Count,
			Cost:       v.Cost,
			Price:      v.Price,
			Value:      v.Value,
			HoldProfit: v.HoldProfit,
			HoldRate:   v.HoldRate,
			Accounts:   v.Accounts,
		}

		if showDay && v.DayProfit != 0 {
			pos.DayProfit = &v.DayProfit
			pos.DayRate = &v.DayRate
			totalDay += v.DayProfit
			hasDay = true
		}

		result = append(result, pos)
		totalValue += v.Value
		totalProfit += v.HoldProfit
	}

	summary := map[string]any{
		"value":       totalValue,
		"hold_profit": totalProfit,
		"count":       len(merged),
	}
	if hasDay {
		summary["day_profit"] = totalDay
	}

	return map[string]any{
		"positions": result,
		"summary":   summary,
	}
}

// PrintJSON 输出 JSON（统一入口）
func (s *RenderService) PrintJSON(data map[string]any) {
	formatter.PrintJSON(data)
}
