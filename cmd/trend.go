package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

var (
	trendAccount  string
	trendDays     int
	trendPeriod   string // month 或 year
	trendLimit    int    // 显示最近 N 条，0 表示全部
	trendCategory string // 账户类型过滤：all/stock/fund
)

var trendCmd = &cobra.Command{
	Use:   "trend",
	Short: "资产趋势",
	Long:  `查看账户的资产趋势曲线数据`,
}

var trendSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "汇总资产趋势",
	Long:  `按账户类型汇总显示资产趋势：全部账户、股票账户、基金账户`,
	Run: func(cmd *cobra.Command, args []string) {
		accounts, err := apiClient.GetAccountList()
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取账户失败: %v\n", err)
			os.Exit(1)
		}

		// 分组：全部、股票（manual+common+rzrq）、基金
		var allAccounts, stockAccounts, fundAccounts []api.Account
		for _, acc := range accounts {
			allAccounts = append(allAccounts, acc)
			if acc.Category == "fund" {
				fundAccounts = append(fundAccounts, acc)
			} else {
				stockAccounts = append(stockAccounts, acc)
			}
		}

		if outputJSON {
			result := map[string]any{}
			switch trendCategory {
			case "all", "":
				result["all"] = getSummaryTrendJSON("全部账户", allAccounts)
				if len(stockAccounts) > 0 {
					result["stock"] = getSummaryTrendJSON("股票账户", stockAccounts)
				}
				if len(fundAccounts) > 0 {
					result["fund"] = getSummaryTrendJSON("基金账户", fundAccounts)
				}
			case "stock":
				if len(stockAccounts) > 0 {
					result["stock"] = getSummaryTrendJSON("股票账户", stockAccounts)
				} else {
					result["error"] = "没有股票账户"
				}
			case "fund":
				if len(fundAccounts) > 0 {
					result["fund"] = getSummaryTrendJSON("基金账户", fundAccounts)
				} else {
					result["error"] = "没有基金账户"
				}
			default:
				fmt.Fprintf(os.Stderr, "未知的账户类型: %s（支持 all/stock/fund）\n", trendCategory)
				os.Exit(1)
			}
			printJSON(result)
			return
		}

		// 根据 -c/--category 参数决定显示哪些
		switch trendCategory {
		case "all", "":
			renderSummaryTrend("全部账户", allAccounts)
			if len(stockAccounts) > 0 {
				renderSummaryTrend("股票账户", stockAccounts)
			}
			if len(fundAccounts) > 0 {
				renderSummaryTrend("基金账户", fundAccounts)
			}
		case "stock":
			if len(stockAccounts) > 0 {
				renderSummaryTrend("股票账户", stockAccounts)
			} else {
				fmt.Println("没有股票账户")
			}
		case "fund":
			if len(fundAccounts) > 0 {
				renderSummaryTrend("基金账户", fundAccounts)
			} else {
				fmt.Println("没有基金账户")
			}
		default:
			fmt.Fprintf(os.Stderr, "未知的账户类型: %s（支持 all/stock/fund）\n", trendCategory)
			os.Exit(1)
		}
	},
}

var trendShowCmd = &cobra.Command{
	Use:   "show",
	Short: "显示资产趋势",
	Long:  `显示指定账户的资产趋势数据（默认近 30 天）`,
	Run: func(cmd *cobra.Command, args []string) {
		matched := mustMatchTrendAccounts()

		if outputJSON {
			var allResults []map[string]any
			for _, acc := range matched {
				points, err := apiClient.GetAssetTrend(acc, trendDays)
				if err != nil {
					fmt.Fprintf(os.Stderr, "【%s】获取趋势失败: %v\n", acc.Name(), err)
					continue
				}
				allResults = append(allResults, map[string]any{
					"account":  acc.Name(),
					"category": api.CategoryLabel(acc.Category),
					"days":     trendDays,
					"points":   convertTrendPointsToJSON(points),
				})
			}
			printJSON(allResults)
			return
		}

		for _, acc := range matched {
			points, err := apiClient.GetAssetTrend(acc, trendDays)
			if err != nil {
				fmt.Fprintf(os.Stderr, "【%s】获取趋势失败: %v\n", acc.Name(), err)
				continue
			}
			if len(points) == 0 {
				fmt.Printf("\n【%s】暂无趋势数据\n", acc.Name())
				continue
			}

			fmt.Printf("\n【%s · %s】近 %d 天资产趋势（共 %d 个数据点）\n",
				acc.Name(), api.CategoryLabel(acc.Category), trendDays, len(points))
			renderTrendTable(points)
			printTrendSummary(points)
		}
	},
}

func mustMatchTrendAccounts() []api.Account {
	accounts, err := apiClient.GetAccountList()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取账户失败: %v\n", err)
		os.Exit(1)
	}
	var matched []api.Account
	for _, acc := range accounts {
		if trendAccount == "" || strings.Contains(acc.Name(), trendAccount) {
			matched = append(matched, acc)
		}
	}
	if len(matched) == 0 {
		fmt.Println("没有匹配的账户")
		os.Exit(0)
	}
	return matched
}

func renderTrendTable(points []api.AssetTrendPoint) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"日期", "总资产", "资产变化", "当日盈亏", "持仓盈亏率", "现金", "市值", "仓位"})
	table.SetBorder(true)
	table.SetAutoWrapText(false)

	// 根据 --limit 参数决定显示范围
	start := 0
	displayPoints := points
	if trendLimit > 0 && len(points) > trendLimit {
		start = len(points) - trendLimit
		displayPoints = points[start:]
	}

	for i := 0; i < len(displayPoints); i++ {
		p := displayPoints[i]
		actualIdx := start + i

		// 计算资产变化（相对前一天）
		var assetChange string
		var dayProfit string
		var holdingProfitRate string

		if actualIdx > 0 {
			change := p.TotalAsset - points[actualIdx-1].TotalAsset
			assetChange = colorNum(change)
			dayProfit = colorNum(change)

			// 持仓盈亏率 = (当日市值 - 前日市值) / 前日市值
			// 剔除资金流入流出的影响，只看持仓本身的涨跌
			if points[actualIdx-1].MarketValue > 0 {
				marketValueChange := p.MarketValue - points[actualIdx-1].MarketValue
				rate := marketValueChange / points[actualIdx-1].MarketValue
				holdingProfitRate = colorPctRaw(rate)
			} else {
				holdingProfitRate = "-"
			}
		} else {
			assetChange = "-"
			dayProfit = "-"
			holdingProfitRate = "-"
		}

		table.Append([]string{
			p.Date,
			fmt.Sprintf("%.2f", p.TotalAsset),
			assetChange,
			dayProfit,
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

func printTrendSummary(points []api.AssetTrendPoint) {
	if len(points) < 2 {
		return
	}

	first := points[0]
	last := points[len(points)-1]

	assetChange := last.TotalAsset - first.TotalAsset
	profitChange := last.Profit - first.Profit

	color.New(color.Bold).Printf("\n区间变化  ")
	fmt.Printf("起始: %s (%.2f) → 最新: %s (%.2f) | ",
		first.Date, first.TotalAsset, last.Date, last.TotalAsset)
	fmt.Printf("资产变化: %s | 盈亏变化: %s\n",
		colorNum(assetChange), colorNum(profitChange))

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
		fmt.Printf("最大回撤: %s\n", colorPctRaw(-maxDrawdown))
	}
}

// renderSummaryTrend 汇总多个账户的趋势数据并展示
func renderSummaryTrend(title string, accounts []api.Account) {
	if len(accounts) == 0 {
		return
	}

	// 1. 获取所有账户的趋势数据
	dateMap := make(map[string]*api.AssetTrendPoint) // date -> 汇总点

	for _, acc := range accounts {
		points, err := apiClient.GetAssetTrend(acc, trendDays)
		if err != nil {
			fmt.Fprintf(os.Stderr, "【%s】获取趋势失败: %v\n", acc.Name(), err)
			continue
		}

		// 按日期累加
		for _, p := range points {
			if existing, ok := dateMap[p.Date]; ok {
				existing.TotalAsset += p.TotalAsset
				existing.Profit += p.Profit
				existing.Cash += p.Cash
				existing.MarketValue += p.MarketValue
			} else {
				// 复制一份新点
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
		fmt.Printf("\n【%s】暂无趋势数据\n", title)
		return
	}

	// 2. 转成有序列表（按日期排序）
	var dates []string
	for date := range dateMap {
		dates = append(dates, date)
	}
	// 简单排序
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

	// 3. 渲染
	fmt.Printf("\n【%s】近 %d 天资产趋势（共 %d 个数据点）\n", title, trendDays, len(points))
	renderTrendTable(points)
	printTrendSummary(points)
}


func init() {
	trendCmd.AddCommand(trendShowCmd)
	trendCmd.AddCommand(trendSummaryCmd)
	trendShowCmd.Flags().StringVarP(&trendAccount, "account", "a", "", "按账户名过滤（模糊匹配），默认全部")
	trendShowCmd.Flags().IntVarP(&trendDays, "days", "d", 30, "查询天数（默认 30 天）")
	trendShowCmd.Flags().IntVarP(&trendLimit, "limit", "l", 10, "显示最近 N 条记录，0 表示全部（默认 10）")
	trendSummaryCmd.Flags().IntVarP(&trendDays, "days", "d", 30, "查询天数（默认 30 天）")
	trendSummaryCmd.Flags().IntVarP(&trendLimit, "limit", "l", 10, "显示最近 N 条记录，0 表示全部（默认 10）")
	trendSummaryCmd.Flags().StringVarP(&trendCategory, "category", "c", "all", "账户类型过滤：all(全部)/stock(股票)/fund(基金)")
}

// JSON 转换函数

func convertTrendPointsToJSON(points []api.AssetTrendPoint) []map[string]any {
	// 根据 --limit 参数决定显示范围
	start := 0
	if trendLimit > 0 && len(points) > trendLimit {
		start = len(points) - trendLimit
	}

	var result []map[string]any
	for i := start; i < len(points); i++ {
		p := points[i]
		item := map[string]any{
			"date":          p.Date,
			"total_asset":   p.TotalAsset,
			"cash":          p.Cash,
			"market_value":  p.MarketValue,
			"position_rate": p.PositionRate,
		}

		// 计算变化量
		if i > 0 {
			change := p.TotalAsset - points[i-1].TotalAsset
			item["asset_change"] = change
			item["day_profit"] = change

			// 持仓盈亏率
			if points[i-1].MarketValue > 0 {
				marketValueChange := p.MarketValue - points[i-1].MarketValue
				rate := marketValueChange / points[i-1].MarketValue * 100
				item["holding_profit_rate"] = rate
			}
		}

		result = append(result, item)
	}
	return result
}

func getSummaryTrendJSON(title string, accounts []api.Account) map[string]any {
	if len(accounts) == 0 {
		return map[string]any{"error": "no accounts"}
	}

	// 1. 获取所有账户的趋势数据
	dateMap := make(map[string]*api.AssetTrendPoint)

	for _, acc := range accounts {
		points, err := apiClient.GetAssetTrend(acc, trendDays)
		if err != nil {
			continue
		}

		// 按日期累加
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
		return map[string]any{"error": "no trend data"}
	}

	// 2. 转成有序列表
	var dates []string
	for date := range dateMap {
		dates = append(dates, date)
	}
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
		if p.TotalAsset > 0 {
			p.PositionRate = p.MarketValue / p.TotalAsset
		}
		points = append(points, *p)
	}

	return map[string]any{
		"title":  title,
		"days":   trendDays,
		"count":  len(points),
		"points": convertTrendPointsToJSON(points),
	}
}
