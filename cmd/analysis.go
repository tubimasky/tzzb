package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/internal/cli"
	"github.com/liyanran/tzzb-cli/internal/formatter"
	"github.com/liyanran/tzzb-cli/internal/service"
)

var (
	analysisAccount string
	analysisTop     int
)

func getAnalysisService() *service.AnalysisService {
	return service.NewAnalysisService(apiClient)
}

var analysisCmd = &cobra.Command{
	Use:   "analysis",
	Short: "股票仓位分析",
	Long:  `查看持仓分布（按市值占比）与各账户仓位率`,
}

var analysisDistCmd = &cobra.Command{
	Use:   "dist",
	Short: "持仓分布（按市值占比）",
	Long:  `列出各标的的持仓市值与占比，从高到低排序。可用 -a 过滤账户。`,
	Run: func(cmd *cobra.Command, args []string) {
		svc := getAnalysisService()
		items, total, err := svc.LoadDistribution(analysisAccount)
		if err != nil {
			cli.HandleError(err, "获取持仓分布失败")
		}

		if outputJSON {
			var arr []map[string]any
			for _, it := range items {
				arr = append(arr, map[string]any{
					"code":   it.Code,
					"name":   it.Name,
					"value":  it.Value,
					"weight": it.Weight,
				})
			}
			formatter.PrintJSON(map[string]any{
				"total_value": total,
				"positions":   arr,
			})
			return
		}

		if len(items) == 0 {
			fmt.Println("暂无持仓")
			return
		}

		if analysisTop > 0 && len(items) > analysisTop {
			items = items[:analysisTop]
		}

		headers := []string{"代码", "名称", "市值", "占比"}
		var rows [][]string
		for _, it := range items {
			rows = append(rows, []string{
				it.Code,
				it.Name,
				fmt.Sprintf("%.2f", it.Value),
				fmt.Sprintf("%.1f%%", it.Weight),
			})
		}
		formatter.RenderTable(headers, rows, formatter.WithBorder(true))
		fmt.Println()
		color.New(color.Bold).Printf("持仓总市值: %.2f\n", total)
	},
}

var analysisRateCmd = &cobra.Command{
	Use:   "rate",
	Short: "账户仓位率",
	Long:  `列出各股票账户的持仓市值、可用现金与仓位率。可用 -a 过滤账户。`,
	Run: func(cmd *cobra.Command, args []string) {
		svc := getAnalysisService()
		rows, err := svc.LoadPositionRate(analysisAccount)
		if err != nil {
			cli.HandleError(err, "获取账户仓位率失败")
		}

		if outputJSON {
			var arr []map[string]any
			for _, r := range rows {
				arr = append(arr, map[string]any{
					"name":         r.Name,
					"broker":       r.Broker,
					"stock_value":  r.StockValue,
					"money_remain": r.MoneyRemain,
					"total":        r.Total(),
					"rate":         r.Rate(),
				})
			}
			formatter.PrintJSON(map[string]any{"accounts": arr})
			return
		}

		if len(rows) == 0 {
			fmt.Println("暂无股票账户")
			return
		}

		var totalStock, totalRemain float64
		headers := []string{"账户", "持仓市值", "可用现金", "总资产", "仓位率"}
		var table [][]string
		for _, r := range rows {
			totalStock += r.StockValue
			totalRemain += r.MoneyRemain
			table = append(table, []string{
				r.Name,
				fmt.Sprintf("%.2f", r.StockValue),
				fmt.Sprintf("%.2f", r.MoneyRemain),
				fmt.Sprintf("%.2f", r.Total()),
				formatter.ColorPct(r.Rate()),
			})
		}
		formatter.RenderTable(headers, table, formatter.WithBorder(true))

		total := totalStock + totalRemain
		rate := 0.0
		if total > 0 {
			rate = totalStock / total * 100
		}
		fmt.Println()
		color.New(color.Bold).Printf(
			"合计  持仓市值: %.2f | 可用现金: %.2f | 总资产: %.2f | 仓位率: %s\n",
			totalStock, totalRemain, total, formatter.ColorPct(rate),
		)
	},
}

func init() {
	analysisCmd.AddCommand(analysisDistCmd)
	analysisCmd.AddCommand(analysisRateCmd)

	analysisDistCmd.Flags().StringVarP(&analysisAccount, "account", "a", "", "按账户名过滤（模糊匹配），默认全部")
	analysisDistCmd.Flags().IntVarP(&analysisTop, "top", "t", 0, "只显示市值 Top N，0 表示全部")

	analysisRateCmd.Flags().StringVarP(&analysisAccount, "account", "a", "", "按账户名过滤（模糊匹配），默认全部")
}
