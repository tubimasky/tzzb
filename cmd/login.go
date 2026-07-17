package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	loginCookie string
	loginFile   string
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "登录投资账本",
	Long: `设置登录凭证 (Cookie)

获取 Cookie 步骤：
  1. 在浏览器中打开 https://tzzb.10jqka.com.cn/pc
  2. 按 F12 打开开发者工具，切换到 Network 标签
  3. 刷新页面，找到任意一个请求
  4. 在请求头中复制完整的 Cookie 值

由于 Cookie 通常很长，终端交互式输入可能因单行长度限制而卡住，
推荐用以下任一方式传入：

  # 用参数传入
  tzzb login --cookie "u_ukey=xxx; userid=xxx; ..."

  # 从文件读取（把 Cookie 存到 cookie.txt）
  tzzb login --file cookie.txt

  # 从管道读取
  pbpaste | tzzb login
  echo "u_ukey=xxx; ..." | tzzb login`,
	Run: func(cmd *cobra.Command, args []string) {
		cookie, err := readCookie()
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取 Cookie 失败: %v\n", err)
			os.Exit(1)
		}

		cookie = strings.TrimSpace(cookie)
		if cookie == "" {
			fmt.Fprintln(os.Stderr, "Cookie 不能为空。用 --cookie / --file 传入，或通过管道输入。运行 tzzb login --help 查看用法。")
			os.Exit(1)
		}

		if err := apiClient.SaveCookies(cookie); err != nil {
			fmt.Fprintf(os.Stderr, "保存 Cookie 失败: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("✓ 登录成功！Cookie 已保存到 ~/.tzzb/cookies")
	},
}

// readCookie 按优先级从 --cookie、--file、管道 stdin 读取 Cookie。
func readCookie() (string, error) {
	if loginCookie != "" {
		return loginCookie, nil
	}

	if loginFile != "" {
		data, err := os.ReadFile(loginFile)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	// 检查 stdin 是否为管道（非终端），是则读取全部内容，避免终端单行长度限制。
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	// 交互式终端：提示改用推荐方式，仍尝试读取一行作为兜底。
	fmt.Println("提示：Cookie 较长时终端可能无法一次性粘贴，建议改用：")
	fmt.Println("  pbpaste | tzzb login    或    tzzb login --file cookie.txt")
	fmt.Print("请粘贴 Cookie（回车结束）: ")
	reader := bufio.NewReader(os.Stdin)
	return reader.ReadString('\n')
}

func init() {
	loginCmd.Flags().StringVarP(&loginCookie, "cookie", "c", "", "直接传入 Cookie 字符串")
	loginCmd.Flags().StringVarP(&loginFile, "file", "f", "", "从文件读取 Cookie")
}
