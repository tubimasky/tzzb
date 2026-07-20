package cmd

import (
	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/internal/service"
)

var (
	calculator = service.NewCalculator()

	filterAccount string
	filterType    string // all / stock / fund
	flatList      bool
	sortBy        string // value/day/hold/rate/dayrate/code/name/count
	sortAsc       bool
)

func getPositionService() *service.PositionService {
	return service.NewPositionService(apiClient, calculator)
}

func getRenderService() *service.RenderService {
	return service.NewRenderService(getPositionService())
}

func getPositionCommandService() *service.PositionCommandService {
	return service.NewPositionCommandService(getPositionService(), getRenderService())
}

var positionCmd = &cobra.Command{
	Use:   "position",
	Short: "持仓管理",
	Long: `查看股票/基金持仓。

子命令:
  list   按账户分块明细
  total  跨账户按代码汇总（同标的合并）
  stock  仅股票/ETF（普通/手动/两融账户）
  fund   仅场外基金账户`,
}

var positionListCmd = &cobra.Command{
	Use:   "list",
	Short: "按账户查看持仓明细",
	Long:  `默认按账户分块展示；加 --flat 打成一张大表（带账户列）。股票行按插件口径重算实时今日盈亏。`,
	Run: func(cmd *cobra.Command, args []string) {
		cmdSvc := getPositionCommandService()
		cmdSvc.ExecuteListCommand(filterAccount, filterType, flatList, outputJSON)
	},
}

var positionTotalCmd = &cobra.Command{
	Use:   "total",
	Short: "跨账户按代码汇总",
	Long: `把所有账户里相同代码的持仓合并成一行。股票/ETF 按插件口径重算实时今日盈亏。

排序（-s/--sort）: value(默认) day hold rate dayrate code name count
升序加 --asc，默认降序。`,
	Run: func(cmd *cobra.Command, args []string) {
		cmdSvc := getPositionCommandService()
		cmdSvc.ExecuteTotalCommand(filterAccount, filterType, sortBy, sortAsc, outputJSON, true)
	},
}

var positionStockCmd = &cobra.Command{
	Use:   "stock",
	Short: "股票/ETF 持仓（跨账户汇总）",
	Long: `只看普通/手动/两融账户，按代码跨账户合并。
现价/市值/持仓盈亏/今日盈亏/收益率按同花顺 getQuotes + 当日成交重算（对齐插件 stockCalc）。

排序（-s/--sort）: value(默认) day hold rate dayrate code name count
升序加 --asc，默认降序。

示例:
  tzzb position stock -s day
  tzzb position stock -s hold --asc
  tzzb position stock -s dayrate`,
	Run: func(cmd *cobra.Command, args []string) {
		cmdSvc := getPositionCommandService()
		cmdSvc.ExecuteTotalCommand(filterAccount, "stock", sortBy, sortAsc, outputJSON, true)
	},
}

var positionFundCmd = &cobra.Command{
	Use:   "fund",
	Short: "场外基金持仓（跨账户汇总）",
	Long: `只看基金账户，按代码跨账户合并。场外基金无实时今日盈亏（盘后净值）。

排序（-s/--sort）: value(默认) hold rate code name count
升序加 --asc。`,
	Run: func(cmd *cobra.Command, args []string) {
		cmdSvc := getPositionCommandService()
		cmdSvc.ExecuteTotalCommand(filterAccount, "fund", sortBy, sortAsc, outputJSON, false)
	},
}

func init() {
	positionCmd.AddCommand(positionListCmd)
	positionCmd.AddCommand(positionTotalCmd)
	positionCmd.AddCommand(positionStockCmd)
	positionCmd.AddCommand(positionFundCmd)

	positionListCmd.Flags().StringVarP(&filterAccount, "account", "a", "", "按账户名过滤（模糊匹配）")
	positionListCmd.Flags().StringVarP(&filterType, "type", "t", "all", "类型: all / stock / fund")
	positionListCmd.Flags().BoolVar(&flatList, "flat", false, "扁平大表（带账户列，不按账户分块）")

	for _, c := range []*cobra.Command{positionTotalCmd, positionStockCmd, positionFundCmd} {
		c.Flags().StringVarP(&sortBy, "sort", "s", "value", "排序: value day hold rate dayrate code name count")
		c.Flags().BoolVar(&sortAsc, "asc", false, "升序（默认降序）")
		c.Flags().StringVarP(&filterAccount, "account", "a", "", "按账户名过滤（模糊匹配）")
	}
}
