package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/liyanran/tzzb-cli/internal/browsercookie"
	"github.com/liyanran/tzzb-cli/internal/formatter"
	"github.com/liyanran/tzzb-cli/internal/hexinv"
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
	// 登录态失效时自动重新从浏览器读取。读不到就返回错误，由 pkg/api 决定不重试。
	// serve 与所有命令共用这一个客户端，因此常驻进程也能跟着续期。
	apiClient.SetAuthRefresher(func() (string, error) {
		result, err := browsercookie.Extract(tzzbDomain)
		if err != nil {
			return "", err
		}
		return result.Cookie, nil
	})

	// 反爬：同花顺要求每个请求带 hexin-v 头，且每个请求都要换新 token
	// （同一个 token 复用约 8~10 次后就会被判 403）。token 由页面上的
	// chameleon 脚本生成，这里用纯 Go 的 JS 引擎跑同一份脚本自己算，
	// 不依赖浏览器。TZZB_NO_HEXIN=1 可关闭（便于排查问题）。
	if !envBool("TZZB_NO_HEXIN") {
		var (
			hexinMu  sync.Mutex
			hexinGen *hexinv.Generator
		)
		apiClient.SetHexinProvider(func() (string, error) {
			hexinMu.Lock()
			defer hexinMu.Unlock()
			if hexinGen == nil {
				// 惰性初始化：不联网的命令不会为它付出任何代价。
				g, err := hexinv.Load(context.Background())
				if err != nil {
					return "", err
				}
				hexinGen = g
			}
			return hexinGen.Token()
		})
	}

	// 全局 --json 标志
	rootCmd.PersistentFlags().BoolVarP(&outputJSON, "json", "j", false, "以 JSON 格式输出")
	// 全局 --no-color 标志
	rootCmd.PersistentFlags().BoolVarP(&noColor, "no-color", "n", false, "禁用颜色输出")

	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(accountCmd)
	rootCmd.AddCommand(positionCmd)
	rootCmd.AddCommand(tradeCmd)
	rootCmd.AddCommand(profitCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(trendCmd)
	rootCmd.AddCommand(bankCmd)
	rootCmd.AddCommand(calendarCmd)
	rootCmd.AddCommand(analysisCmd)
	rootCmd.AddCommand(detailCmd)
}

// 在命令执行前设置全局 formatter 配置
func init() {
	cobra.OnInitialize(func() {
		// 将 cmd 层的 noColor 标志同步到 formatter 包
		formatter.NoColor = noColor
	})
}

// envBool 读取布尔型环境变量（1/true/yes/on 视为真）。
func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
