package cmd

import (
	"fmt"
	"os"

	"github.com/liyanran/tzzb-cli/pkg/api"
	"github.com/spf13/cobra"
)

var (
	apiClient  *api.Client
	outputJSON bool // 全局 JSON 输出标志
	noColor    bool // 全局禁用颜色标志
	rootCmd    = &cobra.Command{
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

	// 全局 --json 标志
	rootCmd.PersistentFlags().BoolVarP(&outputJSON, "json", "j", false, "以 JSON 格式输出")
	// 全局 --no-color 标志
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "禁用颜色输出")

	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(accountCmd)
	rootCmd.AddCommand(positionCmd)
	rootCmd.AddCommand(tradeCmd)
	rootCmd.AddCommand(profitCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(trendCmd)
}
