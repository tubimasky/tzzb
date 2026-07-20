package cmd

import (
	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/internal/service"
)

var tradeAccount string

func getTradeService() *service.TradeService {
	return service.NewTradeService(apiClient)
}

func getTradeCommandService() *service.TradeCommandService {
	return service.NewTradeCommandService(getTradeService())
}

var tradeCmd = &cobra.Command{
	Use:   "trade",
	Short: "交易记录",
	Long:  `查看账户的资金/交易流水（近 3 个月）`,
}

var tradeListCmd = &cobra.Command{
	Use:   "list",
	Short: "查看交易明细",
	Long:  `按账户列出近 3 个月流水明细。可用 -a 过滤账户。`,
	Run: func(cmd *cobra.Command, args []string) {
		cmdSvc := getTradeCommandService()
		cmdSvc.ExecuteListCommand(tradeAccount, outputJSON)
	},
}

var tradeSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "交易汇总",
	Long: `汇总近 3 个月流水：
  1) 按账户：买入/卖出金额、净买入、笔数、费用
  2) 按代码：买入/卖出金额与数量、净买入、笔数

金额口径：净买入 = 买入金额 − 卖出金额（正数=净加仓/净投入，负数=净减仓）。
逆回购/货基等也会计入对应买卖（与流水 op_name 一致）。`,
	Run: func(cmd *cobra.Command, args []string) {
		cmdSvc := getTradeCommandService()
		cmdSvc.ExecuteSummaryCommand(tradeAccount, outputJSON)
	},
}

func init() {
	tradeCmd.AddCommand(tradeListCmd)
	tradeCmd.AddCommand(tradeSummaryCmd)
	tradeListCmd.Flags().StringVarP(&tradeAccount, "account", "a", "", "按账户名过滤（模糊匹配），默认全部")
	tradeSummaryCmd.Flags().StringVarP(&tradeAccount, "account", "a", "", "按账户名过滤（模糊匹配），默认全部")
}
