package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/liyanran/tzzb-cli/internal/cli"
	"github.com/liyanran/tzzb-cli/internal/termimg"
	"github.com/liyanran/tzzb-cli/internal/upass"
	"github.com/spf13/cobra"
)

// 扫码登录：用同花顺投资账本 APP 扫二维码并确认，全程纯 HTTP。
// 这是最稳的免浏览器登录通道：不碰图形验证码，也不碰短信/滑块风控。
var loginQR bool

func init() {
	loginCmd.Flags().BoolVar(&loginQR, "qr", false, "用同花顺投资账本 APP 扫码登录（纯 HTTP，不需要浏览器/短信/验证码）")
	// 保留原有 login 行为，只在前置分支走扫码。
	originalRun := loginCmd.Run
	loginCmd.Run = func(cmd *cobra.Command, args []string) {
		if loginQR {
			qrLogin()
			return
		}
		originalRun(cmd, args)
	}
}

func qrLogin() {
	opts := []upass.Option{upass.WithHexinProvider(hexinToken)}
	if envBool("TZZB_DEBUG") {
		opts = append(opts, upass.WithDebugWriter(os.Stderr))
	}
	client := upass.New(opts...)
	ctx := context.Background()

	fmt.Println("正在申请登录二维码…")
	qr, err := client.CreateQRCode(ctx)
	if err != nil {
		qrFatal(err)
	}
	showQRCode(qr.Image)

	const pollInterval = 1500 * time.Millisecond
	const maxRefresh = 3
	deadline := time.Now().Add(3 * time.Minute)
	lastStatus, refreshed, confirmed := -1, 0, false

	for time.Now().Before(deadline) {
		st, err := client.PollQRCode(ctx, qr.QRID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️ 查询扫码状态失败：%v（继续重试）\n", err)
			time.Sleep(pollInterval)
			continue
		}
		switch st {
		case upass.QRStatusConfirmed:
			confirmed = true
		case upass.QRStatusExpired:
			refreshed++
			if refreshed > maxRefresh {
				fmt.Fprintln(os.Stderr, "✗ 二维码多次失效，已退出，请重试：tzzb login --qr")
				os.Exit(1)
			}
			fmt.Println("⏳ 二维码已失效，正在刷新…")
			if qr, err = client.CreateQRCode(ctx); err != nil {
				qrFatal(err)
			}
			showQRCode(qr.Image)
			lastStatus = -1
		case upass.QRStatusScanned:
			if lastStatus != st {
				fmt.Println("📱 已扫码，请在手机上点「确认登录」。")
			}
			lastStatus = st
		default:
			if lastStatus != st {
				fmt.Println("⏳ 等待扫码…（用同花顺投资账本 APP 扫一扫）")
			}
			lastStatus = st
		}
		if confirmed {
			break
		}
		time.Sleep(pollInterval)
	}
	if !confirmed {
		fmt.Fprintln(os.Stderr, "✗ 等待扫码超时（3 分钟），请重试：tzzb login --qr")
		os.Exit(1)
	}

	cookie := client.Cookies()
	if cookie == "" {
		fmt.Fprintln(os.Stderr, "✗ 扫码成功但没拿到任何 Cookie（接口可能已变更）")
		os.Exit(1)
	}
	if err := apiClient.SaveCookies(cookie); err != nil {
		cli.HandleError(err, "保存 Cookie 失败")
	}
	fmt.Println("✓ 扫码登录成功，Cookie 已保存到 ~/.tzzb/cookies")
	if _, err := apiClient.GetStockAccounts(); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ Cookie 已保存，但接口校验失败：%v\n", err)
		return
	}
	fmt.Println("✓ 接口校验通过，登录态可用")
}

// showQRCode 在终端里画出二维码；终端放不下时改用图片窗口。
func showQRCode(img []byte) {
	path, err := saveQRImage(img)
	if err != nil {
		cli.HandleError(err, "保存二维码失败")
	}
	fmt.Printf("🖼  二维码图片：%s\n", path)

	if envBool("TZZB_CAPTCHA_NO_INLINE") || termimg.NoColor() {
		openImage(path)
		return
	}
	cols := termimg.Cols() - 2
	rows := termimg.Rows() - 5
	if rows < 10 {
		rows = 10
	}
	colsOut, rowsOut, err := termimg.RenderQR(os.Stdout, img, cols, rows)
	if err != nil {
		if errors.Is(err, termimg.ErrQRTooLarge) {
			fmt.Fprintf(os.Stderr, "⚠️ 当前终端放不下清晰的二维码（需要 %d×%d 字符，终端只有 %d×%d）。\n", colsOut, rowsOut, cols, rows)
			fmt.Fprintln(os.Stderr, "   已用看图程序打开二维码图片；也可以最大化终端窗口后重试。")
		} else {
			fmt.Fprintf(os.Stderr, "⚠️ 终端内显示二维码失败（%v），改为打开图片。\n", err)
		}
		openImage(path)
	}
}

// saveQRImage 把登录二维码写到 ~/.tzzb/qrcode.png。
func saveQRImage(img []byte) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".tzzb")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "qrcode.png")
	if err := os.WriteFile(path, img, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// qrFatal 打印登录错误（尽量给出可操作提示）并退出。
func qrFatal(err error) {
	var apiErr *upass.APIError
	if errors.As(err, &apiErr) {
		fmt.Fprintf(os.Stderr, "✗ %s\n", apiErr.Hint())
	} else {
		fmt.Fprintf(os.Stderr, "✗ %v\n", err)
	}
	os.Exit(1)
}

// openImage 用系统看图程序打开图片（无图形环境时静默跳过）。
func openImage(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return
		}
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err == nil {
		_ = cmd.Process.Release()
		fmt.Println("  （已尝试用系统看图程序打开；若没弹出，请手动打开上面的路径）")
	}
}
