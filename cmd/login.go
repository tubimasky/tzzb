package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/liyanran/tzzb-cli/internal/browsercookie"
	"github.com/liyanran/tzzb-cli/internal/cli"
	"github.com/liyanran/tzzb-cli/pkg/api"
	"github.com/spf13/cobra"
)

var (
	loginCookie  string
	loginFile    string
	loginBrowser string
)

// tzzbDomain 是登录态所在的域名，浏览器 Cookie 与 uid 都从这里取。
const tzzbDomain = "10jqka.com.cn"

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "登录投资账本",
	Long: `登录投资账本（设置登录凭证 Cookie）

默认自动从本机浏览器的 Cookie 数据库里读取登录态，不需要手工复制：

  tzzb login

只要浏览器里登录过 https://tzzb.10jqka.com.cn/pc 就能直接用。首次运行 macOS 会
弹出钥匙串授权框，请点“始终允许”，否则无法解密浏览器的 Cookie。

也可以只读取某个浏览器，或无视浏览器走手工方式：

  tzzb login --browser chrome
  tzzb login --cookie "u_ukey=xxx; userid=xxx; ..."
  tzzb login --file cookie.txt
  pbpaste | tzzb login

由于 Cookie 通常很长，终端交互式输入可能因单行长度限制而卡住，因此手工方式
推荐用 --file 或管道传入。`,
	Run: func(cmd *cobra.Command, args []string) {
		// 显式来源优先：用户明确给了 Cookie 就不要再自作聪明。
		cookie, ok, err := explicitCookie()
		if err != nil {
			cli.HandleError(err, "读取 Cookie 失败")
		}
		if ok {
			saveExplicitCookie(cookie)
			return
		}

		err = loginFromBrowser()
		if err == nil {
			return
		}
		fmt.Fprintf(os.Stderr, "✗ 自动读取浏览器登录态失败：%v\n", err)
		if !errors.Is(err, browsercookie.ErrUnsupported) {
			fmt.Fprintln(os.Stderr, "  请先用浏览器打开 https://tzzb.10jqka.com.cn/pc 登录一次再重试。")
		}
		fmt.Fprintln(os.Stderr, "  也可以改用下面的手工方式导入 Cookie。")

		// 最后的兜底：交互式粘贴。
		saveExplicitCookie(interactiveCookie())
	},
}

// explicitCookie 按 --cookie → --file → 管道 stdin 的顺序取 Cookie。
// ok 为 false 表示三个来源都没给，需要走自动读取或交互粘贴。
func explicitCookie() (cookie string, ok bool, err error) {
	if loginCookie != "" {
		return loginCookie, true, nil
	}
	if loginFile != "" {
		data, err := os.ReadFile(loginFile)
		if err != nil {
			return "", false, err
		}
		return string(data), true, nil
	}

	// stdin 不是字符设备说明是管道/重定向，按整段读取，绕开终端单行长度限制。
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", false, err
	}
	if stat.Mode()&os.ModeCharDevice == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", false, err
		}
		if strings.TrimSpace(string(data)) == "" {
			return "", false, nil // 空管道等同于没给
		}
		return string(data), true, nil
	}
	return "", false, nil
}

// interactiveCookie 提示用户粘贴，是自动读取失败后的兜底。
//
// macOS 终端在 canonical 模式下单行输入有硬限制（实测 fpathconf PC_MAX_INPUT =
// 1024 字节）。粘贴超长内容时，超出部分（包括末尾回车）会被内核直接丢弃，不会
// 产生 EOF 或错误——ReadString('\n') 会因此永久阻塞，表现为"粘贴回车不管用"。
// 所以这里用超时代替阻塞等待：短时间内读不到换行，就判定为疑似截断并主动退出。
func interactiveCookie() string {
	fmt.Println("提示：Cookie 较长时终端可能无法一次性粘贴，建议改用：")
	fmt.Println("  pbpaste | tzzb login    或    tzzb login --file cookie.txt")
	fmt.Print("请粘贴 Cookie（回车结束）: ")

	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		ch <- result{line, err}
	}()

	select {
	case r := <-ch:
		if r.err != nil && r.err != io.EOF {
			cli.HandleError(r.err, "读取输入失败")
		}
		return r.line
	case <-time.After(10 * time.Second):
		// 10 秒内既没有换行也没有 EOF：可能是被截断卡住，也可能只是还没粘贴完。
		// 无法精确区分，但真实粘贴通常瞬间完成，10 秒是相对宽松的阈值。
		fmt.Fprintln(os.Stderr, "✗ 等待输入超时：程序卡住没反应，很可能是粘贴内容被终端截断（Cookie 通常超过终端单行 1024 字节的输入上限）。")
		fmt.Fprintln(os.Stderr, "  这是终端本身的限制，无法通过重新粘贴解决，请改用：")
		fmt.Fprintln(os.Stderr, "    pbpaste | tzzb login")
		fmt.Fprintln(os.Stderr, "    tzzb login --file cookie.txt")
		os.Exit(1)
	}
	return ""
}

// loginFromBrowser 从浏览器读取候选登录态，逐个联网校验后写盘。
func loginFromBrowser() error {
	if !browsercookie.Supported() {
		return browsercookie.ErrUnsupported
	}
	if loginBrowser != "" && !validBrowser(loginBrowser) {
		return fmt.Errorf("不支持的浏览器 %q，可选：%s",
			loginBrowser, strings.Join(browsercookie.SupportedBrowsers(), " / "))
	}

	candidates, err := browsercookie.CandidatesFor(tzzbDomain, loginBrowser)
	if err != nil {
		return err
	}

	// 多个 profile 都登录过时，新鲜的排前面。逐个校验，避免把过期的那份写进去。
	var lastErr error
	for _, c := range candidates {
		_, err := api.NewClientWithCookies(c.Cookie).GetAccountMetadata()
		if err == nil {
			return saveBrowserCookie(c, "")
		}
		if !errors.Is(err, api.ErrAuthExpired) {
			// 网络不通或服务端故障：判断不了这份登录态好不好使，先存下来，
			// 但要明确告诉用户没有验证过。
			return saveBrowserCookie(c, err.Error())
		}
		lastErr = err
	}
	return fmt.Errorf("找到 %d 份登录态但都已失效：%v", len(candidates), lastErr)
}

// saveBrowserCookie 保存浏览器里读到的登录态。verifyErr 非空表示这次没能联网验证。
func saveBrowserCookie(c *browsercookie.Result, verifyErr string) error {
	if err := apiClient.SaveCookies(c.Cookie); err != nil {
		return err
	}
	fmt.Printf("✓ 已从 %s / %s 自动登录，Cookie 已保存到 ~/.tzzb/cookies\n", c.Browser, c.Profile)
	if verifyErr != "" {
		fmt.Fprintf(os.Stderr, "⚠️ 未能联网校验（%s），若命令报 403 请重新登录\n", verifyErr)
	}
	return nil
}

// saveExplicitCookie 保存用户手工提供的 Cookie。
func saveExplicitCookie(cookie string) {
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		fmt.Fprintln(os.Stderr, "Cookie 不能为空。用 --cookie / --file 传入，或通过管道输入。运行 tzzb login --help 查看用法。")
		os.Exit(1)
	}

	if missing := browsercookie.MissingFields(cookie); len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "✗ Cookie 缺少关键字段：%s，很可能是粘贴时被截断或复制不完整。\n", strings.Join(missing, ", "))
		fmt.Fprintln(os.Stderr, "  请改用以下方式重新获取完整 Cookie：")
		fmt.Fprintln(os.Stderr, "    pbpaste | tzzb login")
		fmt.Fprintln(os.Stderr, "    tzzb login --file cookie.txt")
		os.Exit(1)
	}

	if err := apiClient.SaveCookies(cookie); err != nil {
		cli.HandleError(err, "保存 Cookie 失败")
	}
	fmt.Println("✓ 登录成功！Cookie 已保存到 ~/.tzzb/cookies")
}

func validBrowser(name string) bool {
	for _, b := range browsercookie.SupportedBrowsers() {
		if strings.EqualFold(b, name) {
			return true
		}
	}
	return false
}

func init() {
	loginCmd.Flags().StringVarP(&loginCookie, "cookie", "c", "", "直接传入 Cookie 字符串")
	loginCmd.Flags().StringVarP(&loginFile, "file", "f", "", "从文件读取 Cookie")
	loginCmd.Flags().StringVar(&loginBrowser, "browser", "", "只读取指定浏览器（chrome/edge/brave/chromium）")
}
