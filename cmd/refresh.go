package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/liyanran/tzzb-cli/internal/service"
	"github.com/spf13/cobra"
)

var refreshTimeout time.Duration

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "触发服务端重新计算（init/status，零浏览器）",
	Long: `触发同花顺服务端重新计算账户汇总值，等价于"打开一次 PC 网页"。

背景：服务端汇总值（持仓/现金/总资产）不是实时算的。手机投资账本 APP 上传新数据后，
服务端要执行一次 init 才会重算；CLI 之前只读取、从不触发，所以当日成交后汇总值可能长时间
滞后（例如清仓后仍显示旧持仓）。PC 网页加载时会自动调 init + 轮询 status，本命令用纯
HTTP 复刻这一动作，不需要浏览器。

注意：同花顺的设计要求「手机端触发一次同步/上传」，这一步 CLI 无法代替；
本命令只负责服务端重算那一步。所以完整流程是：

  手机投资账本 APP 同步一次  →  tzzb refresh  →  tzzb position list / account list

示例：
  tzzb refresh
  tzzb refresh --timeout 60s
  tzzb refresh -j
`,
	Run: func(cmd *cobra.Command, args []string) {
		if apiClient == nil {
			fmt.Fprintln(os.Stderr, "错误：未初始化 API 客户端")
			os.Exit(1)
		}
		res, err := service.TriggerServerCalc(apiClient, refreshTimeout)
		if err != nil {
			if outputJSON && res != nil {
				if data, mErr := json.MarshalIndent(res, "", "  "); mErr == nil {
					fmt.Println(string(data))
				}
			}
			fmt.Fprintln(os.Stderr, "触发服务端重算失败:", err)
			os.Exit(1)
		}
		if outputJSON {
			data, mErr := json.MarshalIndent(res, "", "  ")
			if mErr != nil {
				fmt.Fprintln(os.Stderr, "序列化结果失败:", mErr)
				os.Exit(1)
			}
			fmt.Println(string(data))
			return
		}
		fmt.Printf("✓ 已触发服务端重算：%d 个账户，轮询 %d 次，耗时 %s\n",
			res.Accounts, res.Calc.Polls, res.Calc.Elapsed.Round(time.Millisecond))
	},
}

func init() {
	rootCmd.AddCommand(refreshCmd)
	refreshCmd.Flags().DurationVar(&refreshTimeout, "timeout", 30*time.Second, "等待服务端计算完成的超时时间")
}
