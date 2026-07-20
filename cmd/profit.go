package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/internal/cli"
	"github.com/liyanran/tzzb-cli/internal/formatter"
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
		// 获取账户列表
		accounts, err := accountService.GetAccountList()
		if err != nil {
			cli.HandleError(err, "获取收益数据失败")
		}

		if len(accounts) == 0 {
			if outputJSON {
				formatter.PrintJSON([]any{})
			} else {
				fmt.Println("暂无账户")
			}
			return
		}

		// 按总盈亏从高到低排序
		accountService.SortByTotalProfit(accounts)

		// 计算汇总
		summary := accountService.CalculateSummary(accounts)

		// JSON 输出
		if outputJSON {
			type ProfitJSON struct {
				Account      string  `json:"account"`
				Category     string  `json:"category"`
				Principal    float64 `json:"principal"`
				TotalAssets  float64 `json:"total_assets"`
				HoldProfit   float64 `json:"hold_profit"`
				TodayProfit  float64 `json:"today_profit"`
				TotalProfit  float64 `json:"total_profit"`
				TotalRate    float64 `json:"total_rate"`
			}
			var result []ProfitJSON

			for _, acc := range accounts {
				result = append(result, ProfitJSON{
					Account:     acc.Name(),
					Category:    api.CategoryLabel(acc.Category),
					Principal:   acc.Principal.F(),
					TotalAssets: acc.TotalAssets.F(),
					HoldProfit:  acc.HoldProfit.F(),
					TodayProfit: acc.TodayProfit.F(),
					TotalProfit: acc.TotalProfit.F(),
					TotalRate:   accountService.CalculateAccountRate(acc),
				})
			}

			formatter.PrintJSON(map[string]any{
				"accounts": result,
				"summary": map[string]float64{
					"principal":    summary.TotalPrincipal,
					"total_assets": summary.TotalAssets,
					"hold_profit":  summary.HoldProfit,
					"today_profit": summary.TodayProfit,
					"total_profit": summary.TotalProfit,
					"total_rate":   summary.TotalRate,
				},
			})
			return
		}

		// 表格输出
		headers := []string{"账户", "类型", "本金", "总资产", "持仓盈亏", "当日盈亏", "总盈亏", "总收益率"}
		var rows [][]string

		for _, acc := range accounts {
			rows = append(rows, []string{
				acc.Name(),
				api.CategoryLabel(acc.Category),
				fmt.Sprintf("%.2f", acc.Principal.F()),
				fmt.Sprintf("%.2f", acc.TotalAssets.F()),
				formatter.ColorNum(acc.HoldProfit.F()),
				formatter.ColorNum(acc.TodayProfit.F()),
				formatter.ColorNum(acc.TotalProfit.F()),
				formatter.ColorPct(accountService.CalculateAccountRate(acc)),
			})
		}

		formatter.RenderTable(headers, rows, formatter.WithBorder(true), formatter.WithRowLine(true))

		fmt.Println()
		color.New(color.Bold).Printf(
			"汇总  本金: %.2f | 总资产: %.2f | 持仓盈亏: %s | 当日盈亏: %s | 总盈亏: %s | 总收益率: %s\n",
			summary.TotalPrincipal, summary.TotalAssets,
			formatter.ColorNum(summary.HoldProfit), formatter.ColorNum(summary.TodayProfit),
			formatter.ColorNum(summary.TotalProfit), formatter.ColorPct(summary.TotalRate),
		)
	},
}

func init() {
	profitCmd.AddCommand(profitSummaryCmd)
}
