package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

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
	// hexinToken 生成 hexin-v 反爬 token，供直接请求与登录页共用；
	// 为 nil 表示已禁用（TZZB_NO_HEXIN=1）。
	hexinToken func() (string, error)
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

	// 反爬（hexin-v）两套策略：
	//  1) API 主路径：数学刷新 Cookie 里的 v。官方页面每个请求都会换一枚
	//     新 token 并写回 Cookie 的 v（不发 hexin-v 头），token 里只有时间戳
	//     和计数器会变，所以解码后改时间/计数、重算校验位即可，不用跑 JS。
	//  2) 兜底：Cookie 里没有可识别的 v3 token 时，用纯 Go 的 JS 引擎跑官方
	//     chameleon 脚本现算一枚（登录页也复用这条路径），不依赖浏览器。
	// TZZB_NO_HEXIN=1 可整体关闭（便于排查问题）。
	if !envBool("TZZB_NO_HEXIN") {
		var (
			hexinMu  sync.Mutex
			hexinGen *hexinv.Generator
		)
		newToken := func() (string, error) {
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
		}
		hexinToken = newToken
		apiClient.SetVTokenProvider(func(cookies string) (string, string, error) {
			if newCookies, tok, ok := hexinv.RefreshCookie(cookies, time.Now()); ok {
				return newCookies, tok, nil
			}
			tok, err := newToken()
			if err != nil {
				return "", "", err
			}
			newCookies, ok := hexinv.SetCookieV(cookies, tok)
			if !ok {
				return cookies, tok, nil
			}
			return newCookies, tok, nil
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
