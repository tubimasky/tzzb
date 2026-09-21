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

var profitCmd = &cobra.Command{
	Use:   "profit",
	Short: "收益统计",
	Long:  `按账户汇总当日盈亏、持仓盈亏和总盈亏`,
}

var profitSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "收益汇总",
	Run: func(cmd *cobra.Command, args []string) {
		svc := getAccountService()

		// 获取账户列表
		accounts, err := svc.GetAccountList()
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
		svc.SortByTotalProfit(accounts)

		// 计算汇总
		summary := svc.CalculateSummary(accounts)

		// JSON 输出
		if outputJSON {
			type ProfitJSON struct {
				Account     string  `json:"account"`
				Category    string  `json:"category"`
				Principal   float64 `json:"principal"`
				TotalAssets float64 `json:"total_assets"`
				HoldProfit  float64 `json:"hold_profit"`
				TodayProfit float64 `json:"today_profit"`
				TotalProfit float64 `json:"total_profit"`
				TotalRate   float64 `json:"total_rate"`
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
					TotalRate:   svc.CalculateAccountRate(acc),
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
				formatter.ColorPct(svc.CalculateAccountRate(acc)),
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

var (
	profitAccount string
	profitYear    string
	profitStart   string
	profitEnd     string
	profitTop     int
)

func getStockProfitService() *service.StockProfitService {
	return service.NewStockProfitService(apiClient)
}

var profitMonthCmd = &cobra.Command{
	Use:   "month",
	Short: "月度盈亏（某年各月股票盈亏，含大盘对比）",
	Long:  `按月展示某年股票盈亏，附账户收益率与同期大盘涨跌。可用 -a 过滤账户。`,
	Run: func(cmd *cobra.Command, args []string) {
		svc := getStockProfitService()
		res, year, err := svc.LoadMonthly(profitAccount, profitYear)
		if err != nil {
			cli.HandleError(err, "获取月度盈亏失败")
		}

		if outputJSON {
			printStockPLJSON(res)
			return
		}

		fmt.Printf("%s 年 月度盈亏\n\n", year)
		headers := []string{"月份", "盈亏", "收益率", "同期大盘"}
		var rows [][]string
		for _, p := range res.Points {
			rows = append(rows, []string{
				service.FormatMonth(p.Date),
				formatter.ColorNum(p.ProfitLoss),
				formatter.ColorPctRaw(p.Rate),
				formatter.ColorPctRaw(p.SZRate),
			})
		}
		formatter.RenderTable(headers, rows, formatter.WithBorder(true))
		printStockPLSummary(res)
		if profitTop != 0 {
			renderStockTopCodes(svc, res, fmt.Sprintf("年度个股盈亏 Top %d", profitTop))
		}
	},
}

var profitRangeCmd = &cobra.Command{
	Use:   "range",
	Short: "区间盈亏（每日股票盈亏；个股盈亏为整月口径）",
	Long:  `按日展示某区间股票盈亏（区间口径），并附起始月整月口径的个股盈亏 Top N。日期格式 YYYYMMDD，默认本月初到今天。`,
	Run: func(cmd *cobra.Command, args []string) {
		svc := getStockProfitService()
		res, start, end, err := svc.LoadRange(profitAccount, profitStart, profitEnd)
		if err != nil {
			cli.HandleError(err, "获取区间盈亏失败")
		}

		if outputJSON {
			printStockPLJSON(res)
			return
		}

		fmt.Printf("%s ~ %s 每日盈亏\n\n", service.FormatDay(start), service.FormatDay(end))
		headers := []string{"日期", "盈亏", "收益率", "同期大盘"}
		var rows [][]string
		for _, p := range res.Points {
			rows = append(rows, []string{
				service.FormatDay(p.Date),
				formatter.ColorNum(p.ProfitLoss),
				formatter.ColorPctRaw(p.Rate),
				formatter.ColorPctRaw(p.SZRate),
			})
		}
		formatter.RenderTable(headers, rows, formatter.WithBorder(true))
		printStockPLSummary(res)
		if profitTop != 0 {
			if len(res.Codes) == 0 {
				fmt.Printf("\n提示：个股盈亏为整月口径，仅当起始日为月初时返回；把起始日设为 %s 可查看该月个股盈亏。\n",
					service.FormatDay(start[:6]+"01"))
			} else {
				renderStockTopCodes(svc, res, fmt.Sprintf("个股盈亏 Top %d（起始月整月口径）", profitTop))
			}
		}
	},
}

func printStockPLSummary(res *api.StockPLResult) {
	fmt.Println()
	color.New(color.Bold).Printf(
		"区间合计  盈亏: %s | 收益率: %s | 同期大盘: %s\n",
		formatter.ColorNum(res.TotalPL),
		formatter.ColorPctRaw(res.TotalRate),
		formatter.ColorPctRaw(res.TotalSZ),
	)
}

func renderStockTopCodes(svc *service.StockProfitService, res *api.StockPLResult, title string) {
	codes := svc.TopCodes(res, profitTop)
	var rows []formatter.CodePLRow
	for _, c := range codes {
		rows = append(rows, formatter.CodePLRow{
			Code:       c.Code,
			Name:       c.Name,
			ProfitLoss: c.ProfitLoss,
			Type:       c.Type,
		})
	}
	formatter.RenderCodePL(title, rows)
}

func printStockPLJSON(res *api.StockPLResult) {
	formatter.PrintJSON(map[string]any{
		"total_pl":   res.TotalPL,
		"total_rate": res.TotalRate,
		"total_sz":   res.TotalSZ,
		"points":     res.Points,
		"codes":      res.Codes,
	})
}

func init() {
	profitCmd.AddCommand(profitSummaryCmd)
	profitCmd.AddCommand(profitMonthCmd)
	profitCmd.AddCommand(profitRangeCmd)

	profitMonthCmd.Flags().StringVarP(&profitAccount, "account", "a", "", "按账户名过滤（模糊匹配），默认全部账户汇总")
	profitMonthCmd.Flags().StringVarP(&profitYear, "year", "y", "", "年份 YYYY，默认当前年")
	profitMonthCmd.Flags().IntVarP(&profitTop, "top", "t", 10, "显示年度个股盈亏 Top N，0 表示不显示")

	profitRangeCmd.Flags().StringVarP(&profitAccount, "account", "a", "", "按账户名过滤（模糊匹配），默认全部账户汇总")
	profitRangeCmd.Flags().StringVarP(&profitStart, "start", "s", "", "起始日期 YYYYMMDD，默认本月初")
	profitRangeCmd.Flags().StringVarP(&profitEnd, "end", "e", "", "结束日期 YYYYMMDD，默认今天")
	profitRangeCmd.Flags().IntVarP(&profitTop, "top", "t", 10, "显示起始月个股盈亏 Top N（整月口径），0 表示不显示")
}
