package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/internal/httpapi"
)

var (
	servePort int
	serveHost string
	serveDev  bool
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "启动 Web 服务",
	Long: `启动本地 HTTP 服务，前端通过浏览器访问账户数据和图表。

默认监听 127.0.0.1:8080（只允许本机访问）。
若需局域网内其他设备访问，可指定 --host 0.0.0.0，但请注意数据安全。

开发模式（--dev）会放开 CORS，方便前端 dev server 跨端口调试。

示例：
  tzzb serve                    # 默认 127.0.0.1:8080
  tzzb serve --port 3000        # 自定义端口
  tzzb serve --host 0.0.0.0     # 监听全部网卡（慎用）
  tzzb serve --dev              # 开发模式（CORS 全开）
`,
	Run: func(cmd *cobra.Command, args []string) {
		if apiClient == nil {
			fmt.Fprintln(os.Stderr, "错误：未初始化 API 客户端")
			os.Exit(1)
		}

		srv := httpapi.NewServer(apiClient, serveDev)

		// 尝试加载嵌入的前端资源（生产模式）
		if dist, err := httpapi.GetEmbeddedDistFS(); err == nil {
			srv.SetStaticFS(dist)
		} else {
			// 前端资源不存在：提示用户先构建，但 API 接口仍可用（方便开发）
			fmt.Fprintf(os.Stderr, "提示：前端资源未找到（%v）\n", err)
			fmt.Fprintln(os.Stderr, "  → 生产环境请先执行 make web 构建前端")
			fmt.Fprintln(os.Stderr, "  → 开发环境可直接用 Vite dev server (cd web && npm run dev)")
			fmt.Fprintln(os.Stderr, "  → API 接口仍可用：http://127.0.0.1:8080/api/accounts")
			fmt.Fprintln(os.Stderr)
		}

		if err := srv.Run(serveHost, servePort); err != nil {
			fmt.Fprintf(os.Stderr, "服务启动失败: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 8080, "监听端口")
	serveCmd.Flags().StringVar(&serveHost, "host", "127.0.0.1", "监听地址（默认回环，0.0.0.0 为全网卡）")
	serveCmd.Flags().BoolVar(&serveDev, "dev", false, "开发模式（放开 CORS）")
	rootCmd.AddCommand(serveCmd)
}
