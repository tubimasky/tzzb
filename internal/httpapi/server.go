// Package httpapi 提供本地 Web 服务，把 CLI 的 service 层能力以 JSON 接口暴露给前端。
// 认证复用 CLI 的 Cookie（~/.tzzb/cookies），故默认只监听回环地址，不对外网开放。
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

// Server 承载 HTTP 路由与底层 API 客户端。
type Server struct {
	client *api.Client
	mux    *http.ServeMux
	dev    bool // 开发模式：放开 CORS，方便 Vite dev server 跨端口调试
}

// NewServer 创建服务实例。client 为已加载 Cookie 的 API 客户端。
func NewServer(client *api.Client, dev bool) *Server {
	s := &Server{
		client: client,
		mux:    http.NewServeMux(),
		dev:    dev,
	}
	s.routes()
	return s
}

// Run 在 host:port 上启动服务，阻塞直到收到中断信号后优雅退出。
func (s *Server) Run(host string, port int) error {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	// 安全提示：监听非回环地址意味着同网段其他机器可凭你的登录态访问账户数据。
	if !isLoopback(host) {
		log.Printf("⚠️  正在监听 %s（非回环地址）。任何能访问该地址的人都可读取你的账户数据，请确保处于可信网络。", addr)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           s.withCommon(s.mux),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	// 优雅关闭
	idleClosed := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		log.Println("正在关闭服务...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		close(idleClosed)
	}()

	log.Printf("投资账本 Web 服务已启动：http://%s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	<-idleClosed
	return nil
}

// withCommon 包裹通用中间件：日志、dev CORS。
func (s *Server) withCommon(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.dev {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// SetStaticFS 注册嵌入的前端静态资源作为根路由的兜底处理器。
// dist 为前端打包产物（含 index.html）；SPA 路由回退到 index.html。
func (s *Server) SetStaticFS(dist fs.FS) {
	fileServer := http.FileServer(http.FS(dist))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// /api 已由更具体的路由接管，这里只处理静态资源。
		// 尝试直接命中文件，命中不了则回退 index.html（支持前端 SPA 路由）。
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(dist, p); err != nil {
			// 文件不存在 → SPA 回退
			r.URL.Path = "/"
			serveIndex(w, r, dist)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

// serveIndex 直接输出 index.html（SPA 回退）。
func serveIndex(w http.ResponseWriter, r *http.Request, dist fs.FS) {
	data, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		http.Error(w, "前端资源缺失，请先执行 make web 构建", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

// --- JSON 响应辅助 ---

// writeJSON 输出统一格式的成功响应。
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	if err := enc.Encode(v); err != nil {
		log.Printf("JSON 编码失败: %v", err)
	}
}

// writeError 输出统一格式的错误响应。
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// isLoopback 判断 host 是否为回环地址。
func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
