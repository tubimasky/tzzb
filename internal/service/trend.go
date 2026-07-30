package service

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

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

// TrendLoadWarning 描述某个账户趋势的降级来源。
type TrendLoadWarning struct {
	Account string `json:"account"`
	Source  string `json:"source"`
	Message string `json:"message"`
}

// LoadTrendForAccounts 加载多个账户的趋势数据
func (s *TrendService) LoadTrendForAccounts(accounts []api.Account, days int) ([]TrendData, []TrendLoadWarning) {
	var results []TrendData
	var warnings []TrendLoadWarning
	for _, acc := range accounts {
		points, err := s.client.GetAssetTrend(acc, days)
		if err != nil {
			warnings = append(warnings, TrendLoadWarning{Account: acc.Name(), Source: "资产趋势", Message: err.Error()})
			continue
		}
		if len(points) == 0 {
			warnings = append(warnings, TrendLoadWarning{Account: acc.Name(), Source: "资产趋势", Message: "上游未返回数据"})
			continue
		}

		// 加载银证转账数据并填充到趋势点中
		points, err = s.enrichTrendWithBankTransfers(acc, points, days)
		if err != nil {
			warnings = append(warnings, TrendLoadWarning{Account: acc.Name(), Source: "银证转账", Message: err.Error()})
		}

		results = append(results, TrendData{
			Account:  acc.Name(),
			Category: acc.Category,
			Days:     days,
			Points:   points,
		})
	}
	return results, warnings
}

// enrichTrendWithBankTransfers 用银证转账数据丰富趋势点
func (s *TrendService) enrichTrendWithBankTransfers(acc api.Account, points []api.AssetTrendPoint, days int) ([]api.AssetTrendPoint, error) {
	if len(points) == 0 {
		return points, nil
	}
	// 银证历史接口只覆盖股票侧账户；基金趋势沿用 asset_trend 自带资金流字段。
	if acc.Category == "fund" {
		return points, nil
	}

	// 获取银证转账数据
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days-30) // 多查一些天数以确保覆盖

	transfers, err := s.client.GetBankHistory(acc, startDate, endDate)
	if err != nil {
		return points, err
	}

	if len(transfers) == 0 {
		return points, nil
	}

	// 按日期建立转账映射
	transferMap := make(map[string]struct{ In, Out float64 })
	for _, t := range transfers {
		key := t.Date
		entry := transferMap[key]
		// 根据操作码判断：86=转入(收入)，87=转出(支出)
		// 兼容旧的字符串判断
		if t.OpCode == 86 || t.Direction == "银行转证券" || t.Direction == "收入" {
			entry.In += t.Amount
		} else if t.OpCode == 87 || t.Direction == "证券转银行" || t.Direction == "支出" {
			entry.Out += t.Amount
		}
		transferMap[key] = entry
	}

	// 填充到趋势点
	for i := range points {
		if entry, ok := transferMap[points[i].Date]; ok {
			points[i].FundIn = entry.In
			points[i].FundOut = entry.Out
		}
	}

	return points, nil
}

// AggregateTrendDataWithWarnings 聚合多个账户趋势，并返回所有降级信息。
func (s *TrendService) AggregateTrendDataWithWarnings(accounts []api.Account, days int) ([]api.AssetTrendPoint, []TrendLoadWarning) {
	var series [][]api.AssetTrendPoint
	var warnings []TrendLoadWarning
	for _, acc := range accounts {
		points, err := s.client.GetAssetTrend(acc, days)
		if err != nil {
			warnings = append(warnings, TrendLoadWarning{Account: acc.Name(), Source: "资产趋势", Message: err.Error()})
			continue
		}
		if len(points) == 0 {
			warnings = append(warnings, TrendLoadWarning{Account: acc.Name(), Source: "资产趋势", Message: "上游未返回数据"})
			continue
		}

		// 加载银证转账数据并填充到趋势点中
		points, err = s.enrichTrendWithBankTransfers(acc, points, days)
		if err != nil {
			warnings = append(warnings, TrendLoadWarning{Account: acc.Name(), Source: "银证转账", Message: err.Error()})
		}
		series = append(series, points)
	}
	aggregated := aggregateTrendSeries(series)
	return aggregated, warnings
}

// AggregateTrendData 保留原调用形态；需要判断完整性时使用 AggregateTrendDataWithWarnings。
func (s *TrendService) AggregateTrendData(accounts []api.Account, days int) []api.AssetTrendPoint {
	points, _ := s.AggregateTrendDataWithWarnings(accounts, days)
	return points
}

// LoadDailyProfitLoss 按月拼接盈亏日历，返回区间内每天的真实盈亏。
// combineCalendar 直接给出股票 + 基金盈亏，不用资产差额倒推，因此转账、申购和赎回
// 不会被误算成投资收益。
func (s *TrendService) LoadDailyProfitLoss(accounts []api.Account, startDate, endDate string) (map[string]float64, []TrendLoadWarning) {
	result := make(map[string]float64)
	months, err := monthsInRange(startDate, endDate)
	if err != nil {
		return result, []TrendLoadWarning{{Account: "全部账户", Source: "盈亏日历", Message: err.Error()}}
	}

	var stockRefs []api.StockAccountRef
	if hasStockAccount(accounts) {
		stockRefs, err = s.client.GetStockAccounts()
		if err != nil {
			return result, []TrendLoadWarning{{
				Account: "全部账户", Source: "盈亏日历", Message: fmt.Sprintf("获取股票账户失败: %v", err),
			}}
		}
	}
	params := api.BuildCalendarParams(accounts, stockRefs)

	var warnings []TrendLoadWarning
	for _, month := range months {
		calendar, err := s.client.GetCalendarDaily(params, month)
		if err != nil {
			warnings = append(warnings, TrendLoadWarning{
				Account: "全部账户", Source: "盈亏日历", Message: fmt.Sprintf("%s: %v", month, err),
			})
			continue
		}
		for _, point := range calendar.Points {
			date := compactDateToISO(point.Date)
			if date < startDate || date > endDate {
				continue
			}
			result[date] = point.TotalPL
		}
	}
	return result, warnings
}

func monthsInRange(startDate, endDate string) ([]string, error) {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("起始日期格式错误: %s", startDate)
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("结束日期格式错误: %s", endDate)
	}
	if start.After(end) {
		return nil, fmt.Errorf("起始日期晚于结束日期: %s > %s", startDate, endDate)
	}

	current := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)
	var months []string
	for !current.After(last) {
		months = append(months, current.Format("200601"))
		current = current.AddDate(0, 1, 0)
	}
	return months, nil
}

func compactDateToISO(date string) string {
	if len(date) != 8 {
		return date
	}
	return date[:4] + "-" + date[4:6] + "-" + date[6:]
}

// aggregateTrendSeries 按所有账户日期并集聚合曲线。
// 某账户首日前按 0 处理；首日后缺点时向前填充资产快照，但不会重复计入上一日资金流。
func aggregateTrendSeries(series [][]api.AssetTrendPoint) []api.AssetTrendPoint {
	filtered := make([][]api.AssetTrendPoint, 0, len(series))
	for _, points := range series {
		if len(points) == 0 {
			continue
		}
		cp := append([]api.AssetTrendPoint(nil), points...)
		sort.Slice(cp, func(i, j int) bool { return cp[i].Date < cp[j].Date })
		filtered = append(filtered, cp)
	}
	if len(filtered) == 0 {
		return nil
	}

	dateSet := make(map[string]struct{})
	for _, points := range filtered {
		for _, point := range points {
			dateSet[point.Date] = struct{}{}
		}
	}
	dates := make([]string, 0, len(dateSet))
	for date := range dateSet {
		dates = append(dates, date)
	}
	sort.Strings(dates)

	indexes := make([]int, len(filtered))
	result := make([]api.AssetTrendPoint, 0, len(dates))
	for _, date := range dates {
		total := api.AssetTrendPoint{Date: date}
		for i, points := range filtered {
			if date < points[0].Date {
				continue
			}
			for indexes[i]+1 < len(points) && points[indexes[i]+1].Date <= date {
				indexes[i]++
			}
			point := points[indexes[i]]
			total.TotalAsset += point.TotalAsset
			total.Profit += point.Profit
			total.Cash += point.Cash
			total.MarketValue += point.MarketValue
			if point.Date == date {
				total.FundIn += point.FundIn
				total.FundOut += point.FundOut
			}
		}
		if total.TotalAsset > 0 {
			total.PositionRate = total.MarketValue / total.TotalAsset
		}
		result = append(result, total)
	}
	return result
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

// HoldingReturn 是剔除银证转账后的单日与累计收益率（均为小数）。
type HoldingReturn struct {
	Daily      float64
	Cumulative float64
	// Valid 为 false 表示当日无法算出 Daily（前一日资产为 0 或首日），
	// 此时 Cumulative 沿用前值，不把曲线拉回 0。
	Valid bool
}

// ComputeHoldingReturns 用盈亏日历的真实每日盈亏计算复利累计收益率。
//
// 单日口径：当日盈亏 / (当日总资产 - 当日盈亏)。分母是扣除当日盈亏后的资金基数，
// 转账、申购、新账户起始资产只改变本金，不进入收益分子。某日缺少盈亏日历数据时
// 标记为无效并沿用累计值，绝不回退到资产差额。
func (s *TrendService) ComputeHoldingReturns(points []api.AssetTrendPoint, dailyProfitLoss map[string]float64) []HoldingReturn {
	out := make([]HoldingReturn, len(points))
	if len(points) == 0 {
		return out
	}

	out[0] = HoldingReturn{Valid: true} // 首日是基准，累计 0
	growth := 1.0

	for i := 1; i < len(points); i++ {
		profitLoss, ok := dailyProfitLoss[points[i].Date]
		capitalBase := points[i].TotalAsset - profitLoss
		if !ok || capitalBase <= 0 {
			out[i] = HoldingReturn{Cumulative: growth - 1}
			continue
		}

		daily := profitLoss / capitalBase
		growth *= 1 + daily
		out[i] = HoldingReturn{Daily: daily, Cumulative: growth - 1, Valid: true}
	}

	return out
}

// ComputeReturnDrawdown 在累计收益曲线上算最大回撤（小数，正值表示回撤幅度）。
//
// 与 ComputeStatistics 里基于总资产的回撤不同：这里的曲线已剔除出入金，
// 转出资金不会被误算成回撤。
func (s *TrendService) ComputeReturnDrawdown(returns []HoldingReturn) (float64, bool) {
	if len(returns) < 2 {
		return 0, false
	}

	peak := 0.0 // 收益曲线从 0 起步
	maxDD := 0.0
	for _, r := range returns {
		if r.Cumulative > peak {
			peak = r.Cumulative
		}
		// 净值口径：(1+peak) 相对 (1+cur) 的跌幅
		if dd := (peak - r.Cumulative) / (1 + peak); dd > maxDD {
			maxDD = dd
		}
	}

	if maxDD <= 0 {
		return 0, false
	}
	return maxDD, true
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
	accounts, err := s.client.GetAccountMetadata()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取账户失败: %v\n", err)
		os.Exit(1)
	}

	matched := s.trendService.FilterAccounts(accounts, accountFilter)
	if len(matched) == 0 {
		fmt.Println("没有匹配的账户")
		os.Exit(0)
	}

	allData, warnings := s.trendService.LoadTrendForAccounts(matched, days)
	s.printTrendWarnings(warnings)

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
	accounts, err := s.client.GetAccountMetadata()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取账户失败: %v\n", err)
		os.Exit(1)
	}

	groups := s.trendService.GroupAccountsByCategory(accounts)

	if outputJSON {
		result := map[string]any{}
		switch category {
		case "all", "":
			result["all"] = s.jsonService.BuildAggregatedJSON("全部账户", s.aggregateTrendData(groups.All, days), days, limit)
			if len(groups.Stock) > 0 {
				result["stock"] = s.jsonService.BuildAggregatedJSON("股票账户", s.aggregateTrendData(groups.Stock, days), days, limit)
			}
			if len(groups.Fund) > 0 {
				result["fund"] = s.jsonService.BuildAggregatedJSON("基金账户", s.aggregateTrendData(groups.Fund, days), days, limit)
			}
		case "stock":
			if len(groups.Stock) > 0 {
				result["stock"] = s.jsonService.BuildAggregatedJSON("股票账户", s.aggregateTrendData(groups.Stock, days), days, limit)
			} else {
				result["error"] = "没有股票账户"
			}
		case "fund":
			if len(groups.Fund) > 0 {
				result["fund"] = s.jsonService.BuildAggregatedJSON("基金账户", s.aggregateTrendData(groups.Fund, days), days, limit)
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
		s.renderService.RenderAggregatedTrend("全部账户", s.aggregateTrendData(groups.All, days), days, limit, showBenchmark)
		if len(groups.Stock) > 0 {
			s.renderService.RenderAggregatedTrend("股票账户", s.aggregateTrendData(groups.Stock, days), days, limit, showBenchmark)
		}
		if len(groups.Fund) > 0 {
			s.renderService.RenderAggregatedTrend("基金账户", s.aggregateTrendData(groups.Fund, days), days, limit, showBenchmark)
		}
	case "stock":
		if len(groups.Stock) > 0 {
			s.renderService.RenderAggregatedTrend("股票账户", s.aggregateTrendData(groups.Stock, days), days, limit, showBenchmark)
		} else {
			fmt.Println("没有股票账户")
		}
	case "fund":
		if len(groups.Fund) > 0 {
			s.renderService.RenderAggregatedTrend("基金账户", s.aggregateTrendData(groups.Fund, days), days, limit, showBenchmark)
		} else {
			fmt.Println("没有基金账户")
		}
	default:
		fmt.Fprintf(os.Stderr, "未知的账户类型: %s（支持 all/stock/fund）\n", category)
		os.Exit(1)
	}
}

func (s *TrendCommandService) aggregateTrendData(accounts []api.Account, days int) []api.AssetTrendPoint {
	points, warnings := s.trendService.AggregateTrendDataWithWarnings(accounts, days)
	s.printTrendWarnings(warnings)
	return points
}

func (s *TrendCommandService) printTrendWarnings(warnings []TrendLoadWarning) {
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "警告: %s %s加载失败: %s\n", warning.Account, warning.Source, warning.Message)
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
			"fund_in":       p.FundIn,
			"fund_out":      p.FundOut,
		}

		// 计算变化量
		if actualIdx > 0 {
			change := p.TotalAsset - points[actualIdx-1].TotalAsset
			item["asset_change"] = change
			item["day_profit"] = change

			// 持仓盈亏率计算（扣除资金流入流出后的市值涨跌幅）
			if points[actualIdx-1].MarketValue > 0 {
				marketValueChange := p.MarketValue - points[actualIdx-1].MarketValue
				fundNetIn := p.FundIn - p.FundOut
				rate := (marketValueChange - fundNetIn) / points[actualIdx-1].MarketValue * 100
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

			// 持仓盈亏率 = (市值变化 - 资金净流入) / 前日市值
			if points[actualIdx-1].MarketValue > 0 {
				marketValueChange := p.MarketValue - points[actualIdx-1].MarketValue
				fundNetIn := p.FundIn - p.FundOut
				rate := (marketValueChange - fundNetIn) / points[actualIdx-1].MarketValue
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
