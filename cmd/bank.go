package cmd

import (
	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/internal/service"
)

var bankAccount string
var bankDays int

func getBankService() *service.BankService {
	return service.NewBankService(apiClient)
}

var bankCmd = &cobra.Command{
	Use:   "bank",
	Short: "银证转账",
	Long:  `查看银证转账记录`,
}

var bankListCmd = &cobra.Command{
	Use:   "list",
	Short: "查看银证转账明细",
	Long:  `列出银证转账明细记录。可用 -a 过滤账户，-d 指定天数。`,
	Run: func(cmd *cobra.Command, args []string) {
		svc := getBankService()
		svc.ExecuteListCommand(bankAccount, bankDays, outputJSON)
	},
}

func init() {
	bankCmd.AddCommand(bankListCmd)
	bankListCmd.Flags().StringVarP(&bankAccount, "account", "a", "", "按账户名过滤（模糊匹配），默认全部")
	bankListCmd.Flags().IntVarP(&bankDays, "days", "d", 90, "查询天数，默认 90 天")
}
