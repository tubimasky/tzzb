package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

var profitCmd = &cobra.Command{
	Use:   "profit",
	Short: "收益统计",
	Long:  `按账户汇总当日盈亏、持仓盈亏和总盈亏`,
}

var profitSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "收益汇总",
	Run: func(cmd *cobra.Command, args []string) {
		accounts, err := apiClient.GetAccountList()
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取收益数据失败: %v\n", err)
			os.Exit(1)
		}

		if len(accounts) == 0 {
			fmt.Println("暂无账户")
			return
		}

		// 按总盈亏从高到低排序，方便一眼看出贡献
		sort.Slice(accounts, func(i, j int) bool {
			return accounts[i].TotalProfit.F() > accounts[j].TotalProfit.F()
		})

		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"账户", "类型", "本金", "总资产", "持仓盈亏", "当日盈亏", "总盈亏", "总收益率"})
		table.SetBorder(true)
		table.SetRowLine(true)

		var totalPrincipal, totalAssets, totalHold, totalToday, totalProfit float64
		for _, acc := range accounts {
			principal := acc.Principal.F()
			totalPrincipal += principal
			totalAssets += acc.TotalAssets.F()
			totalHold += acc.HoldProfit.F()
			totalToday += acc.TodayProfit.F()
			totalProfit += acc.TotalProfit.F()

			rate := 0.0
			if principal > 0 {
				rate = acc.TotalProfit.F() / principal * 100
			}

			table.Append([]string{
				acc.Name(),
				api.CategoryLabel(acc.Category),
				fmt.Sprintf("%.2f", principal),
				fmt.Sprintf("%.2f", acc.TotalAssets.F()),
				colorNum(acc.HoldProfit.F()),
				colorNum(acc.TodayProfit.F()),
				colorNum(acc.TotalProfit.F()),
				colorPct(rate),
			})
		}
		table.Render()

		totalRate := 0.0
		if totalPrincipal > 0 {
			totalRate = totalProfit / totalPrincipal * 100
		}

		fmt.Println()
		color.New(color.Bold).Printf(
			"汇总  本金: %.2f | 总资产: %.2f | 持仓盈亏: %s | 当日盈亏: %s | 总盈亏: %s | 总收益率: %s\n",
			totalPrincipal, totalAssets,
			colorNum(totalHold), colorNum(totalToday), colorNum(totalProfit), colorPct(totalRate),
		)
	},
}

func init() {
	profitCmd.AddCommand(profitSummaryCmd)
}
