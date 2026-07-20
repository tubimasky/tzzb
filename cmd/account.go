package cmd

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "账户管理",
	Long:  `查看投资账户列表及汇总`,
}

var accountListCmd = &cobra.Command{
	Use:   "list",
	Short: "查看账户列表",
	Run: func(cmd *cobra.Command, args []string) {
		accounts, err := apiClient.GetAccountList()
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取账户列表失败: %v\n", err)
			os.Exit(1)
		}

		if len(accounts) == 0 {
			if outputJSON {
				printJSON([]any{})
			} else {
				fmt.Println("暂无账户")
			}
			return
		}

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
			var totalAssets, totalHold, totalToday, totalProfit float64
			for _, acc := range accounts {
				totalAssets += acc.TotalAssets.F()
				totalHold += acc.HoldProfit.F()
				totalToday += acc.TodayProfit.F()
				totalProfit += acc.TotalProfit.F()
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
					"total_assets":  totalAssets,
					"hold_profit":   totalHold,
					"today_profit":  totalToday,
					"total_profit":  totalProfit,
				},
			}
			printJSON(result)
			return
		}

		// 表格输出
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"账户", "类型", "总资产", "持仓盈亏", "当日盈亏", "总盈亏", "仓位"})
		table.SetBorder(true)
		table.SetRowLine(true)

		var totalAssets, totalHold, totalToday, totalProfit float64
		for _, acc := range accounts {
			totalAssets += acc.TotalAssets.F()
			totalHold += acc.HoldProfit.F()
			totalToday += acc.TodayProfit.F()
			totalProfit += acc.TotalProfit.F()

			table.Append([]string{
				acc.Name(),
				api.CategoryLabel(acc.Category),
				fmt.Sprintf("%.2f", acc.TotalAssets.F()),
				colorNum(acc.HoldProfit.F()),
				colorNum(acc.TodayProfit.F()),
				colorNum(acc.TotalProfit.F()),
				fmt.Sprintf("%.1f%%", acc.PositionRate.F()*100),
			})
		}
		table.Render()

		fmt.Println()
		bold := color.New(color.Bold)
		bold.Printf("汇总  总资产: %.2f | 持仓盈亏: %s | 当日盈亏: %s | 总盈亏: %s\n",
			totalAssets, colorNum(totalHold), colorNum(totalToday), colorNum(totalProfit))
	},
}

// colorNum 按正负给数字上色（红涨绿跌是 A 股习惯，这里保持红负绿正的通用财务口径）。
// colorNum 金额着色：中国股市，红涨绿跌
func colorNum(v float64) string {
	if noColor {
		return fmt.Sprintf("%.2f", v)
	}
	if v < 0 {
		return color.New(color.FgGreen).Sprintf("%.2f", v)
	}
	return color.New(color.FgRed).Sprintf("%.2f", v)
}

func init() {
	accountCmd.AddCommand(accountListCmd)
}
