package service

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/liyanran/tzzb-cli/internal/formatter"
	"github.com/liyanran/tzzb-cli/pkg/api"
	"github.com/olekukonko/tablewriter"
)

// TrendRenderService 趋势渲染服务
type TrendRenderService struct {
	trendService *TrendService
}

// NewTrendRenderService 创建趋势渲染服务
func NewTrendRenderService(trendService *TrendService) *TrendRenderService {
	return &TrendRenderService{trendService: trendService}
}

// RenderTrendTable 渲染趋势表格
func (s *TrendRenderService) RenderTrendTable(points []api.AssetTrendPoint, limit int) {
	displayPoints, start := s.trendService.LimitPoints(points, limit)

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"日期", "总资产", "资产变化", "持仓盈亏率", "现金", "市值", "仓位"})
	table.SetBorder(true)
	table.SetAutoWrapText(false)

	for i := 0; i < len(displayPoints); i++ {
		p := displayPoints[i]
		actualIdx := start + i

		// 计算资产变化（相对前一天）
		var assetChange string
		var holdingProfitRate string

		if actualIdx > 0 {
			change := p.TotalAsset - points[actualIdx-1].TotalAsset
			assetChange = formatter.ColorNum(change)

			// 持仓盈亏率 = (当日市值 - 前日市值) / 前日市值
			if points[actualIdx-1].MarketValue > 0 {
				marketValueChange := p.MarketValue - points[actualIdx-1].MarketValue
				rate := marketValueChange / points[actualIdx-1].MarketValue
				holdingProfitRate = formatter.ColorPctRaw(rate)
			} else {
				holdingProfitRate = "-"
			}
		} else {
			assetChange = "-"
			holdingProfitRate = "-"
		}

		table.Append([]string{
			p.Date,
			fmt.Sprintf("%.2f", p.TotalAsset),
			assetChange,
			holdingProfitRate,
			fmt.Sprintf("%.2f", p.Cash),
			fmt.Sprintf("%.2f", p.MarketValue),
			fmt.Sprintf("%.1f%%", p.PositionRate*100),
		})
	}
	table.Render()

	if start > 0 {
		fmt.Printf("（已省略前 %d 个数据点，仅显示最近 %d 天）\n", start, len(displayPoints))
	}
}

// PrintTrendSummary 打印趋势汇总（区间变化、最大回撤）
func (s *TrendRenderService) PrintTrendSummary(stats TrendStatistics) {
	if stats.FirstDate == "" {
		return
	}

	color.New(color.Bold).Printf("\n区间变化  ")
	fmt.Printf("起始: %s (%.2f) → 最新: %s (%.2f) | ",
		stats.FirstDate, stats.FirstAsset, stats.LastDate, stats.LastAsset)
	fmt.Printf("资产变化: %s | 盈亏变化: %s\n",
		formatter.ColorNum(stats.AssetChange), formatter.ColorNum(stats.ProfitChange))

	if stats.HasMaxDrawdown {
		fmt.Printf("最大回撤: %s\n", formatter.ColorPctRaw(-stats.MaxDrawdown))
	}
}

// RenderSingleAccountTrend 渲染单个账户的趋势
func (s *TrendRenderService) RenderSingleAccountTrend(data TrendData, limit int) {
	if len(data.Points) == 0 {
		fmt.Printf("\n【%s】暂无趋势数据\n", data.Account)
		return
	}

	fmt.Printf("\n【%s · %s】近 %d 天资产趋势（共 %d 个数据点）\n",
		data.Account, api.CategoryLabel(data.Category), data.Days, len(data.Points))
	s.RenderTrendTable(data.Points, limit)

	stats := s.trendService.ComputeStatistics(data.Points)
	s.PrintTrendSummary(stats)
}

// RenderAggregatedTrend 渲染聚合后的趋势（汇总视图）
func (s *TrendRenderService) RenderAggregatedTrend(title string, points []api.AssetTrendPoint, days int, limit int) {
	if len(points) == 0 {
		fmt.Printf("\n【%s】暂无趋势数据\n", title)
		return
	}

	fmt.Printf("\n【%s】近 %d 天资产趋势（共 %d 个数据点）\n", title, days, len(points))
	s.RenderTrendTable(points, limit)

	stats := s.trendService.ComputeStatistics(points)
	s.PrintTrendSummary(stats)
}
