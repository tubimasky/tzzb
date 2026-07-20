package cmd

import (
	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/internal/service"
)

var (
	trendAccount   string
	trendDays      int
	trendPeriod    string // month 或 year
	trendLimit     int    // 显示最近 N 条，0 表示全部
	trendCategory  string // 账户类型过滤：all/stock/fund
	trendBenchmark bool   // 是否显示基准对比
)

func getTrendCommandService() *service.TrendCommandService {
	return service.NewTrendCommandService(apiClient)
}

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
		cmdSvc := getTrendCommandService()
		cmdSvc.ExecuteSummaryCommand(trendCategory, trendDays, trendLimit, outputJSON, trendBenchmark)
	},
}

var trendShowCmd = &cobra.Command{
	Use:   "show",
	Short: "显示资产趋势",
	Long:  `显示指定账户的资产趋势数据（默认近 30 天）`,
	Run: func(cmd *cobra.Command, args []string) {
		cmdSvc := getTrendCommandService()
		cmdSvc.ExecuteShowCommand(trendAccount, trendDays, trendLimit, outputJSON, trendBenchmark)
	},
}

func init() {
	trendCmd.AddCommand(trendShowCmd)
	trendCmd.AddCommand(trendSummaryCmd)
	trendShowCmd.Flags().StringVarP(&trendAccount, "account", "a", "", "按账户名过滤（模糊匹配），默认全部")
	trendShowCmd.Flags().IntVarP(&trendDays, "days", "d", 30, "查询天数（默认 30 天）")
	trendShowCmd.Flags().IntVarP(&trendLimit, "limit", "l", 10, "显示最近 N 条记录，0 表示全部（默认 10）")
	trendShowCmd.Flags().BoolVarP(&trendBenchmark, "benchmark", "b", false, "显示上证指数对比")
	trendSummaryCmd.Flags().IntVarP(&trendDays, "days", "d", 30, "查询天数（默认 30 天）")
	trendSummaryCmd.Flags().IntVarP(&trendLimit, "limit", "l", 10, "显示最近 N 条记录，0 表示全部（默认 10）")
	trendSummaryCmd.Flags().StringVarP(&trendCategory, "category", "c", "all", "账户类型过滤：all(全部)/stock(股票)/fund(基金)")
	trendSummaryCmd.Flags().BoolVarP(&trendBenchmark, "benchmark", "b", false, "显示上证指数对比")
}
