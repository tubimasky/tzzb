package cmd

import (
	"fmt"
	"os"

	"github.com/liyanran/tzzb-cli/pkg/api"
	"github.com/spf13/cobra"
)

var (
	apiClient *api.Client
	rootCmd   = &cobra.Command{
		Use:   "tzzb",
		Short: "投资账本命令行工具",
		Long:  `投资账本命令行工具 - 查看持仓、记录交易、查看收益、导出数据`,
	}
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	apiClient = api.NewClient()

	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(accountCmd)
	rootCmd.AddCommand(positionCmd)
	rootCmd.AddCommand(tradeCmd)
	rootCmd.AddCommand(profitCmd)
	rootCmd.AddCommand(exportCmd)
}
