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
	trendAccount string
	trendDays    int
	trendPeriod  string // month 或 year
	trendLimit   int    // 显示最近 N 条，0 表示全部
)

var trendCmd = &cobra.Command{
	Use:   "trend",
	Short: "资产趋势",
	Long:  `查看账户的资产趋势曲线数据`,
}

var trendShowCmd = &cobra.Command{
	Use:   "show",
	Short: "显示资产趋势",
	Long:  `显示指定账户的资产趋势数据（默认近 30 天）`,
	Run: func(cmd *cobra.Command, args []string) {
		matched := mustMatchTrendAccounts()

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
	if trendLimit > 0 && len(points) > trendLimit {
		start = len(points) - trendLimit
	}

	for i := start; i < len(points); i++ {
		p := points[i]

		// 计算资产变化（相对前一天）
		var assetChange string
		var dayProfit string
		var holdingProfitRate string

		if i > 0 {
			change := p.TotalAsset - points[i-1].TotalAsset
			assetChange = colorNum(change)
			dayProfit = colorNum(change)

			// 持仓盈亏率 = (当日市值 - 前日市值) / 前日市值
			// 剔除资金流入流出的影响，只看持仓本身的涨跌
			if points[i-1].MarketValue > 0 {
				marketValueChange := p.MarketValue - points[i-1].MarketValue
				rate := marketValueChange / points[i-1].MarketValue
				holdingProfitRate = colorPct(rate)
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
		fmt.Printf("（已省略前 %d 个数据点，仅显示最近 %d 天）\n", start, len(points)-start)
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
	maxAsset := first.TotalAsset
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
		fmt.Printf("最大回撤: %s\n", colorPct(-maxDrawdown))
	}
}

func init() {
	trendCmd.AddCommand(trendShowCmd)
	trendShowCmd.Flags().StringVarP(&trendAccount, "account", "a", "", "按账户名过滤（模糊匹配），默认全部")
	trendShowCmd.Flags().IntVarP(&trendDays, "days", "d", 30, "查询天数（默认 30 天）")
	trendShowCmd.Flags().IntVarP(&trendLimit, "limit", "l", 10, "显示最近 N 条记录，0 表示全部（默认 10）")
}
