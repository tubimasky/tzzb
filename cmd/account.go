package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/internal/cli"
	"github.com/liyanran/tzzb-cli/internal/formatter"
	"github.com/liyanran/tzzb-cli/internal/service"
	"github.com/liyanran/tzzb-cli/pkg/api"
)

var accountService *service.AccountService

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "账户管理",
	Long:  `查看投资账户列表及汇总`,
}

var accountListCmd = &cobra.Command{
	Use:   "list",
	Short: "查看账户列表",
	Run: func(cmd *cobra.Command, args []string) {
		// 获取账户列表
		accounts, err := accountService.GetAccountList()
		if err != nil {
			cli.HandleError(err, "获取账户列表失败")
		}

		if len(accounts) == 0 {
			if outputJSON {
				formatter.PrintJSON([]any{})
			} else {
				fmt.Println("暂无账户")
			}
			return
		}

		// 计算汇总
		summary := accountService.CalculateSummary(accounts)

		// JSON 输出
		if outputJSON {
			type AccountOutput struct {
				Name         string  `json:"name"`
				Category     string  `json:"category"`
				TotalAssets  float64 `json:"total_assets"`
				HoldProfit   float64 `json:"hold_profit"`
				TodayProfit  float64 `json:"today_profit"`
				TotalProfit  float64 `json:"total_profit"`
				PositionRate float64 `json:"position_rate"`
			}
			var output []AccountOutput
			for _, acc := range accounts {
				output = append(output, AccountOutput{
					Name:         acc.Name(),
					Category:     api.CategoryLabel(acc.Category),
					TotalAssets:  acc.TotalAssets.F(),
					HoldProfit:   acc.HoldProfit.F(),
					TodayProfit:  acc.TodayProfit.F(),
					TotalProfit:  acc.TotalProfit.F(),
					PositionRate: acc.PositionRate.F(),
				})
			}
			result := map[string]any{
				"accounts": output,
				"summary": map[string]float64{
					"total_assets":  summary.TotalAssets,
					"hold_profit":   summary.HoldProfit,
					"today_profit":  summary.TodayProfit,
					"total_profit":  summary.TotalProfit,
				},
			}
			formatter.PrintJSON(result)
			return
		}

		// 表格输出
		headers := []string{"账户", "类型", "总资产", "持仓盈亏", "当日盈亏", "总盈亏", "仓位"}
		var rows [][]string

		for _, acc := range accounts {
			rows = append(rows, []string{
				acc.Name(),
				api.CategoryLabel(acc.Category),
				fmt.Sprintf("%.2f", acc.TotalAssets.F()),
				formatter.ColorNum(acc.HoldProfit.F()),
				formatter.ColorNum(acc.TodayProfit.F()),
				formatter.ColorNum(acc.TotalProfit.F()),
				fmt.Sprintf("%.1f%%", acc.PositionRate.F()*100),
			})
		}

		formatter.RenderTable(headers, rows, formatter.WithBorder(true), formatter.WithRowLine(true))

		fmt.Println()
		bold := color.New(color.Bold)
		bold.Printf("汇总  总资产: %.2f | 持仓盈亏: %s | 当日盈亏: %s | 总盈亏: %s\n",
			summary.TotalAssets, formatter.ColorNum(summary.HoldProfit),
			formatter.ColorNum(summary.TodayProfit), formatter.ColorNum(summary.TotalProfit))
	},
}

func init() {
	accountService = service.NewAccountService(apiClient)
	accountCmd.AddCommand(accountListCmd)
}
