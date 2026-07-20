package service

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/liyanran/tzzb-cli/internal/formatter"
	"github.com/liyanran/tzzb-cli/pkg/api"
	"github.com/olekukonko/tablewriter"
)

// TrendService 趋势数据业务逻辑
type TrendService struct {
	client *api.Client
}

// NewTrendService 创建趋势服务
func NewTrendService(client *api.Client) *TrendService {
	return &TrendService{client: client}
}

// AccountGroups 账户分组结果
type AccountGroups struct {
	All   []api.Account
	Stock []api.Account
	Fund  []api.Account
}

// GroupAccountsByCategory 按类型分组账户
func (s *TrendService) GroupAccountsByCategory(accounts []api.Account) AccountGroups {
	var groups AccountGroups
	for _, acc := range accounts {
		groups.All = append(groups.All, acc)
		if acc.Category == "fund" {
			groups.Fund = append(groups.Fund, acc)
		} else {
			groups.Stock = append(groups.Stock, acc)
		}
	}
	return groups
}

// FilterAccounts 按账户名过滤
func (s *TrendService) FilterAccounts(accounts []api.Account, accountFilter string) []api.Account {
	if accountFilter == "" {
		return accounts
	}
	var matched []api.Account
	for _, acc := range accounts {
		if strings.Contains(acc.Name(), accountFilter) {
			matched = append(matched, acc)
		}
	}
	return matched
}

// TrendData 趋势数据（带元信息）
type TrendData struct {
	Account  string
	Category string
	Days     int
	Points   []api.AssetTrendPoint
}

// LoadTrendForAccounts 加载多个账户的趋势数据
func (s *TrendService) LoadTrendForAccounts(accounts []api.Account, days int) []TrendData {
	var results []TrendData
	for _, acc := range accounts {
		points, err := s.client.GetAssetTrend(acc, days)
		if err != nil {
			continue
		}
		results = append(results, TrendData{
			Account:  acc.Name(),
			Category: acc.Category,
			Days:     days,
			Points:   points,
		})
	}
	return results
}

// AggregateTrendData 聚合多个账户的趋势数据（按日期合并）
func (s *TrendService) AggregateTrendData(accounts []api.Account, days int) []api.AssetTrendPoint {
	dateMap := make(map[string]*api.AssetTrendPoint)

	for _, acc := range accounts {
		points, err := s.client.GetAssetTrend(acc, days)
		if err != nil {
			continue
		}

		for _, p := range points {
			if existing, ok := dateMap[p.Date]; ok {
				existing.TotalAsset += p.TotalAsset
				existing.Profit += p.Profit
				existing.Cash += p.Cash
				existing.MarketValue += p.MarketValue
			} else {
				dateMap[p.Date] = &api.AssetTrendPoint{
					Date:        p.Date,
					TotalAsset:  p.TotalAsset,
					Profit:      p.Profit,
					Cash:        p.Cash,
					MarketValue: p.MarketValue,
				}
			}
		}
	}

	if len(dateMap) == 0 {
		return nil
	}

	// 转成有序列表（按日期排序）
	var dates []string
	for date := range dateMap {
		dates = append(dates, date)
	}
	// 简单排序（日期字符串可以直接比较）
	for i := 0; i < len(dates); i++ {
		for j := i + 1; j < len(dates); j++ {
			if dates[i] > dates[j] {
				dates[i], dates[j] = dates[j], dates[i]
			}
		}
	}

	var points []api.AssetTrendPoint
	for _, date := range dates {
		p := dateMap[date]
		// 计算仓位
		if p.TotalAsset > 0 {
			p.PositionRate = p.MarketValue / p.TotalAsset
		}
		points = append(points, *p)
	}

	return points
}

// TrendStatistics 趋势统计数据
type TrendStatistics struct {
	FirstDate      string
	FirstAsset     float64
	LastDate       string
	LastAsset      float64
	AssetChange    float64
	ProfitChange   float64
	MaxDrawdown    float64
	HasMaxDrawdown bool
}

// ComputeStatistics 计算趋势统计（区间变化、最大回撤）
func (s *TrendService) ComputeStatistics(points []api.AssetTrendPoint) TrendStatistics {
	if len(points) < 2 {
		return TrendStatistics{}
	}

	first := points[0]
	last := points[len(points)-1]

	stats := TrendStatistics{
		FirstDate:    first.Date,
		FirstAsset:   first.TotalAsset,
		LastDate:     last.Date,
		LastAsset:    last.TotalAsset,
		AssetChange:  last.TotalAsset - first.TotalAsset,
		ProfitChange: last.Profit - first.Profit,
	}

	// 计算最大回撤
	maxAsset := 0.0
	maxDrawdown := 0.0
	for _, p := range points {
		if p.TotalAsset > maxAsset {
			maxAsset = p.TotalAsset
		}
		if maxAsset > 0 {
			dd := (maxAsset - p.TotalAsset) / maxAsset
			if dd > maxDrawdown {
				maxDrawdown = dd
			}
		}
	}

	if maxDrawdown > 0 {
		stats.MaxDrawdown = maxDrawdown
		stats.HasMaxDrawdown = true
	}

	return stats
}

// LimitPoints 限制显示的数据点数量（返回最近 N 条）
func (s *TrendService) LimitPoints(points []api.AssetTrendPoint, limit int) ([]api.AssetTrendPoint, int) {
	if limit <= 0 || len(points) <= limit {
		return points, 0
	}
	start := len(points) - limit
	return points[start:], start
}


// TrendCommandService 趋势命令服务（封装重复的加载+渲染流程）
type TrendCommandService struct {
	trendService  *TrendService
	renderService *TrendRenderService
	jsonService   *TrendJSONService
	client        *api.Client
}

// NewTrendCommandService 创建趋势命令服务
func NewTrendCommandService(client *api.Client) *TrendCommandService {
	trendSvc := NewTrendService(client)
	return &TrendCommandService{
		trendService:  trendSvc,
		renderService: NewTrendRenderService(trendSvc),
		jsonService:   NewTrendJSONService(trendSvc),
		client:        client,
	}
}

// ExecuteShowCommand 执行 show 命令逻辑
func (s *TrendCommandService) ExecuteShowCommand(accountFilter string, days int, limit int, outputJSON bool, showBenchmark bool) {
	accounts, err := s.client.GetAccountList()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取账户失败: %v\n", err)
		os.Exit(1)
	}

	matched := s.trendService.FilterAccounts(accounts, accountFilter)
	if len(matched) == 0 {
		fmt.Println("没有匹配的账户")
		os.Exit(0)
	}

	allData := s.trendService.LoadTrendForAccounts(matched, days)

	if outputJSON {
		var results []map[string]any
		for _, data := range allData {
			results = append(results, s.jsonService.BuildSingleAccountJSON(data, limit))
		}
		s.jsonService.PrintJSON(map[string]any{"accounts": results})
		return
	}

	for _, data := range allData {
		s.renderService.RenderSingleAccountTrend(data, limit, showBenchmark)
	}
}

// ExecuteSummaryCommand 执行 summary 命令逻辑
func (s *TrendCommandService) ExecuteSummaryCommand(category string, days int, limit int, outputJSON bool, showBenchmark bool) {
	accounts, err := s.client.GetAccountList()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取账户失败: %v\n", err)
		os.Exit(1)
	}

	groups := s.trendService.GroupAccountsByCategory(accounts)

	if outputJSON {
		result := map[string]any{}
		switch category {
		case "all", "":
			result["all"] = s.jsonService.BuildAggregatedJSON("全部账户", s.trendService.AggregateTrendData(groups.All, days), days, limit)
			if len(groups.Stock) > 0 {
				result["stock"] = s.jsonService.BuildAggregatedJSON("股票账户", s.trendService.AggregateTrendData(groups.Stock, days), days, limit)
			}
			if len(groups.Fund) > 0 {
				result["fund"] = s.jsonService.BuildAggregatedJSON("基金账户", s.trendService.AggregateTrendData(groups.Fund, days), days, limit)
			}
		case "stock":
			if len(groups.Stock) > 0 {
				result["stock"] = s.jsonService.BuildAggregatedJSON("股票账户", s.trendService.AggregateTrendData(groups.Stock, days), days, limit)
			} else {
				result["error"] = "没有股票账户"
			}
		case "fund":
			if len(groups.Fund) > 0 {
				result["fund"] = s.jsonService.BuildAggregatedJSON("基金账户", s.trendService.AggregateTrendData(groups.Fund, days), days, limit)
			} else {
				result["error"] = "没有基金账户"
			}
		default:
			fmt.Fprintf(os.Stderr, "未知的账户类型: %s（支持 all/stock/fund）\n", category)
			os.Exit(1)
		}
		s.jsonService.PrintJSON(result)
		return
	}

	// 文本渲染
	switch category {
	case "all", "":
		s.renderService.RenderAggregatedTrend("全部账户", s.trendService.AggregateTrendData(groups.All, days), days, limit, showBenchmark)
		if len(groups.Stock) > 0 {
			s.renderService.RenderAggregatedTrend("股票账户", s.trendService.AggregateTrendData(groups.Stock, days), days, limit, showBenchmark)
		}
		if len(groups.Fund) > 0 {
			s.renderService.RenderAggregatedTrend("基金账户", s.trendService.AggregateTrendData(groups.Fund, days), days, limit, showBenchmark)
		}
	case "stock":
		if len(groups.Stock) > 0 {
			s.renderService.RenderAggregatedTrend("股票账户", s.trendService.AggregateTrendData(groups.Stock, days), days, limit, showBenchmark)
		} else {
			fmt.Println("没有股票账户")
		}
	case "fund":
		if len(groups.Fund) > 0 {
			s.renderService.RenderAggregatedTrend("基金账户", s.trendService.AggregateTrendData(groups.Fund, days), days, limit, showBenchmark)
		} else {
			fmt.Println("没有基金账户")
		}
	default:
		fmt.Fprintf(os.Stderr, "未知的账户类型: %s（支持 all/stock/fund）\n", category)
		os.Exit(1)
	}
}


// TrendJSONService 趋势 JSON 输出服务
type TrendJSONService struct {
	trendService *TrendService
}

// NewTrendJSONService 创建趋势 JSON 服务
func NewTrendJSONService(trendService *TrendService) *TrendJSONService {
	return &TrendJSONService{trendService: trendService}
}

// ConvertTrendPointsToJSON 转换趋势数据点为 JSON（带变化量计算）
func (s *TrendJSONService) ConvertTrendPointsToJSON(points []api.AssetTrendPoint, limit int) []map[string]any {
	displayPoints, start := s.trendService.LimitPoints(points, limit)

	var result []map[string]any
	for i := 0; i < len(displayPoints); i++ {
		p := displayPoints[i]
		actualIdx := start + i

		item := map[string]any{
			"date":          p.Date,
			"total_asset":   p.TotalAsset,
			"cash":          p.Cash,
			"market_value":  p.MarketValue,
			"position_rate": p.PositionRate,
		}

		// 计算变化量
		if actualIdx > 0 {
			change := p.TotalAsset - points[actualIdx-1].TotalAsset
			item["asset_change"] = change
			item["day_profit"] = change

			// 持仓盈亏率
			if points[actualIdx-1].MarketValue > 0 {
				marketValueChange := p.MarketValue - points[actualIdx-1].MarketValue
				rate := marketValueChange / points[actualIdx-1].MarketValue * 100
				item["holding_profit_rate"] = rate
			}
		}

		result = append(result, item)
	}
	return result
}

// BuildSingleAccountJSON 构建单个账户趋势 JSON
func (s *TrendJSONService) BuildSingleAccountJSON(data TrendData, limit int) map[string]any {
	return map[string]any{
		"account":  data.Account,
		"category": api.CategoryLabel(data.Category),
		"days":     data.Days,
		"points":   s.ConvertTrendPointsToJSON(data.Points, limit),
	}
}

// BuildAggregatedJSON 构建聚合趋势 JSON
func (s *TrendJSONService) BuildAggregatedJSON(title string, points []api.AssetTrendPoint, days int, limit int) map[string]any {
	if len(points) == 0 {
		return map[string]any{"error": "no trend data"}
	}

	return map[string]any{
		"title":  title,
		"days":   days,
		"count":  len(points),
		"points": s.ConvertTrendPointsToJSON(points, limit),
	}
}

// PrintJSON 输出 JSON（统一入口）
func (s *TrendJSONService) PrintJSON(data map[string]any) {
	formatter.PrintJSON(data)
}


// TrendRenderService 趋势渲染服务
type TrendRenderService struct {
	trendService *TrendService
}

// NewTrendRenderService 创建趋势渲染服务
func NewTrendRenderService(trendService *TrendService) *TrendRenderService {
	return &TrendRenderService{trendService: trendService}
}

// RenderTrendTable 渲染趋势表格
func (s *TrendRenderService) RenderTrendTable(points []api.AssetTrendPoint, limit int, showBenchmark bool) {
	displayPoints, start := s.trendService.LimitPoints(points, limit)

	// 获取上证指数数据（如果需要）
	var benchmarkReturns map[string]float64
	if showBenchmark {
		// 获取足够的天数（包括被省略的数据点）
		totalDays := len(points) + 10
		benchmarkPoints, err := api.GetShanghaiIndexHistory(totalDays)
		if err == nil {
			benchmarkReturns = api.CalculateBenchmarkReturns(benchmarkPoints)
		}
	}

	headers := []string{"日期", "总资产", "资产变化", "持仓盈亏率", "现金", "市值", "仓位"}
	if showBenchmark {
		headers = []string{"日期", "总资产", "资产变化", "持仓盈亏率", "上证涨跌", "超额收益", "现金", "市值", "仓位"}
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(headers)
	table.SetBorder(true)
	table.SetAutoWrapText(false)

	for i := 0; i < len(displayPoints); i++ {
		p := displayPoints[i]
		actualIdx := start + i

		// 计算资产变化（相对前一天）
		var assetChange string
		var holdingProfitRate string
		var benchmarkReturn string
		var excessReturn string

		if actualIdx > 0 {
			change := p.TotalAsset - points[actualIdx-1].TotalAsset
			assetChange = formatter.ColorNum(change)

			// 持仓盈亏率 = (当日市值 - 前日市值) / 前日市值
			if points[actualIdx-1].MarketValue > 0 {
				marketValueChange := p.MarketValue - points[actualIdx-1].MarketValue
				rate := marketValueChange / points[actualIdx-1].MarketValue
				holdingProfitRate = formatter.ColorPctRaw(rate)

				// 如果有基准数据，计算超额收益
				if showBenchmark && benchmarkReturns != nil {
					if bmRet, ok := benchmarkReturns[p.Date]; ok {
						benchmarkReturn = formatter.ColorPctRaw(bmRet)
						excessReturn = formatter.ColorPctRaw(rate - bmRet)
					} else {
						benchmarkReturn = "-"
						excessReturn = "-"
					}
				}
			} else {
				holdingProfitRate = "-"
				if showBenchmark {
					benchmarkReturn = "-"
					excessReturn = "-"
				}
			}
		} else {
			assetChange = "-"
			holdingProfitRate = "-"
			if showBenchmark {
				benchmarkReturn = "-"
				excessReturn = "-"
			}
		}

		row := []string{
			p.Date,
			fmt.Sprintf("%.2f", p.TotalAsset),
			assetChange,
			holdingProfitRate,
		}

		if showBenchmark {
			row = append(row, benchmarkReturn, excessReturn)
		}

		row = append(row,
			fmt.Sprintf("%.2f", p.Cash),
			fmt.Sprintf("%.2f", p.MarketValue),
			fmt.Sprintf("%.1f%%", p.PositionRate*100),
		)

		table.Append(row)
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
func (s *TrendRenderService) RenderSingleAccountTrend(data TrendData, limit int, showBenchmark bool) {
	if len(data.Points) == 0 {
		fmt.Printf("\n【%s】暂无趋势数据\n", data.Account)
		return
	}

	fmt.Printf("\n【%s · %s】近 %d 天资产趋势（共 %d 个数据点）\n",
		data.Account, api.CategoryLabel(data.Category), data.Days, len(data.Points))
	s.RenderTrendTable(data.Points, limit, showBenchmark)

	stats := s.trendService.ComputeStatistics(data.Points)
	s.PrintTrendSummary(stats)
}

// RenderAggregatedTrend 渲染聚合后的趋势（汇总视图）
func (s *TrendRenderService) RenderAggregatedTrend(title string, points []api.AssetTrendPoint, days int, limit int, showBenchmark bool) {
	if len(points) == 0 {
		fmt.Printf("\n【%s】暂无趋势数据\n", title)
		return
	}

	fmt.Printf("\n【%s】近 %d 天资产趋势（共 %d 个数据点）\n", title, days, len(points))
	s.RenderTrendTable(points, limit, showBenchmark)

	stats := s.trendService.ComputeStatistics(points)
	s.PrintTrendSummary(stats)
}

