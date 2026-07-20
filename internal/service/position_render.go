package service

import (
	"fmt"
	"os"
	"sort"

	"github.com/fatih/color"
	"github.com/liyanran/tzzb-cli/internal/formatter"
	"github.com/liyanran/tzzb-cli/pkg/api"
	"github.com/olekukonko/tablewriter"
)

// RenderService 持仓渲染服务
type RenderService struct {
	positionService *PositionService
}

// NewRenderService 创建渲染服务
func NewRenderService(positionService *PositionService) *RenderService {
	return &RenderService{positionService: positionService}
}

// AccountGroup 按账户分组的持仓数据
type AccountGroup struct {
	Account  string
	Category string
	Rows     []PositionRow
}

// GroupByAccount 按账户分组持仓行
func (s *RenderService) GroupByAccount(rows []PositionRow) []AccountGroup {
	var order []string
	groups := map[string][]PositionRow{}
	catOf := map[string]string{}

	for _, r := range rows {
		if _, ok := groups[r.Account]; !ok {
			order = append(order, r.Account)
			catOf[r.Account] = r.Category
		}
		groups[r.Account] = append(groups[r.Account], r)
	}

	var result []AccountGroup
	for _, name := range order {
		result = append(result, AccountGroup{
			Account:  name,
			Category: catOf[name],
			Rows:     groups[name],
		})
	}
	return result
}

// AccountSummary 账户汇总数据
type AccountSummary struct {
	Value      float64
	HoldProfit float64
	DayProfit  float64
	HasDay     bool
	Count      int
}

// ComputeAccountSummary 计算账户汇总
func (s *RenderService) ComputeAccountSummary(rows []PositionRow, ctx *LiveContext, isFund bool) AccountSummary {
	var summary AccountSummary
	summary.Count = len(rows)

	for _, r := range rows {
		var lv api.LiveView
		if isFund {
			lv = s.positionService.RecomputePosition(r, nil)
		} else {
			lv = s.positionService.RecomputePosition(r, ctx)
		}
		summary.Value += lv.Value
		summary.HoldProfit += lv.HoldProfit
		if lv.DayOK {
			summary.DayProfit += lv.DayProfit
			summary.HasDay = true
		}
	}
	return summary
}

// RenderByAccount 按账户渲染持仓
func (s *RenderService) RenderByAccount(rows []PositionRow, ctx *LiveContext) {
	groups := s.GroupByAccount(rows)
	showDay := ctx != nil && len(ctx.Quotes) > 0

	var grandValue, grandProfit, grandDay float64
	var hasDay bool

	for _, group := range groups {
		fmt.Printf("\n【%s · %s】\n", group.Account, api.CategoryLabel(group.Category))
		table := tablewriter.NewWriter(os.Stdout)
		useLive := showDay && group.Category != "fund"

		if useLive {
			table.SetHeader([]string{"代码", "名称", "持仓量", "成本价", "现价", "市值", "持仓盈亏", "今日盈亏", "今日涨幅", "收益率"})
		} else {
			table.SetHeader([]string{"代码", "名称", "持仓量", "成本价", "现价", "市值", "持仓盈亏", "本周盈亏", "收益率"})
		}
		table.SetBorder(true)

		summary := s.ComputeAccountSummary(group.Rows, ctx, group.Category == "fund")

		for _, r := range group.Rows {
			if useLive {
				lv := s.positionService.RecomputePosition(r, ctx)
				table.Append([]string{
					r.Position.Code, r.Position.Name,
					fmt.Sprintf("%.0f", r.Position.Count.F()),
					formatter.PriceStr(r.Position.Cost.F()),
					formatter.PriceStr(lv.Price),
					fmt.Sprintf("%.2f", lv.Value),
					formatter.ColorNum(lv.HoldProfit),
					formatter.DayStr(lv.DayProfit, lv.DayOK),
					formatter.RateStr(lv.DayRate, lv.DayOK),
					formatter.ColorPct(lv.HoldRate),
				})
			} else {
				table.Append(s.renderFundCells(r))
			}
		}
		table.Render()

		grandValue += summary.Value
		grandProfit += summary.HoldProfit
		if summary.HasDay {
			grandDay += summary.DayProfit
			hasDay = true
		}

		if summary.HasDay {
			fmt.Printf("小计  市值: %.2f | 持仓盈亏: %s | 今日盈亏: %s | 标的: %d\n",
				summary.Value, formatter.ColorNum(summary.HoldProfit), formatter.ColorNum(summary.DayProfit), summary.Count)
		} else {
			fmt.Printf("小计  市值: %.2f | 持仓盈亏: %s | 标的: %d\n",
				summary.Value, formatter.ColorNum(summary.HoldProfit), summary.Count)
		}
	}

	fmt.Println()
	if hasDay {
		color.New(color.Bold).Printf("合计  市值: %.2f | 持仓盈亏: %s | 今日盈亏: %s | 行数: %d | 账户: %d\n",
			grandValue, formatter.ColorNum(grandProfit), formatter.ColorNum(grandDay), len(rows), len(groups))
	} else {
		color.New(color.Bold).Printf("合计  市值: %.2f | 持仓盈亏: %s | 行数: %d | 账户: %d\n",
			grandValue, formatter.ColorNum(grandProfit), len(rows), len(groups))
	}
}

// renderFundCells 基金/无行情行：不展示今日盈亏列，用接口值（本周盈亏列）
func (s *RenderService) renderFundCells(r PositionRow) []string {
	lv := s.positionService.RecomputePosition(r, nil)
	return []string{
		r.Position.Code,
		r.Position.Name,
		fmt.Sprintf("%.0f", r.Position.Count.F()),
		formatter.PriceStr(r.Position.Cost.F()),
		formatter.PriceStr(lv.Price),
		fmt.Sprintf("%.2f", lv.Value),
		formatter.ColorNum(lv.HoldProfit),
		formatter.ColorNum(r.Position.WProfit.F()),
		formatter.ColorPct(lv.HoldRate),
	}
}

// FlatView 扁平视图（单行 + LiveView）
type FlatView struct {
	Row      PositionRow
	LiveView api.LiveView
}

// RenderFlat 扁平表格渲染
func (s *RenderService) RenderFlat(rows []PositionRow, ctx *LiveContext) {
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
	sort.Slice(views, func(i, j int) bool {
		return views[i].LiveView.Value > views[j].LiveView.Value
	})

	showDay := ctx != nil && len(ctx.Quotes) > 0
	table := tablewriter.NewWriter(os.Stdout)
	if showDay {
		table.SetHeader([]string{"账户", "类型", "代码", "名称", "持仓量", "成本价", "现价", "市值", "持仓盈亏", "今日盈亏", "今日涨幅", "收益率"})
	} else {
		table.SetHeader([]string{"账户", "类型", "代码", "名称", "持仓量", "成本价", "现价", "市值", "持仓盈亏", "本周盈亏", "收益率"})
	}
	table.SetBorder(true)

	var totalValue, totalProfit, totalDay float64
	var hasDay bool
	for _, v := range views {
		r, lv := v.Row, v.LiveView
		totalValue += lv.Value
		totalProfit += lv.HoldProfit
		if showDay && r.Category != "fund" {
			if lv.DayOK {
				totalDay += lv.DayProfit
				hasDay = true
			}
			table.Append([]string{
				r.Account, api.CategoryLabel(r.Category),
				r.Position.Code, r.Position.Name,
				fmt.Sprintf("%.0f", r.Position.Count.F()),
				formatter.PriceStr(r.Position.Cost.F()),
				formatter.PriceStr(lv.Price),
				fmt.Sprintf("%.2f", lv.Value),
				formatter.ColorNum(lv.HoldProfit),
				formatter.DayStr(lv.DayProfit, lv.DayOK),
				formatter.RateStr(lv.DayRate, lv.DayOK),
				formatter.ColorPct(lv.HoldRate),
			})
		} else {
			cells := []string{r.Account, api.CategoryLabel(r.Category)}
			cells = append(cells, s.renderFundCells(r)...)
			table.Append(cells)
		}
	}
	table.Render()

	fmt.Println()
	if hasDay {
		color.New(color.Bold).Printf("合计  市值: %.2f | 持仓盈亏: %s | 今日盈亏: %s | 行数: %d\n",
			totalValue, formatter.ColorNum(totalProfit), formatter.ColorNum(totalDay), len(rows))
	} else {
		color.New(color.Bold).Printf("合计  市值: %.2f | 持仓盈亏: %s | 行数: %d\n",
			totalValue, formatter.ColorNum(totalProfit), len(rows))
	}
}
