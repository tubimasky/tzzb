# 投资账本 Web

本地 Web 界面，对接 `tzzb serve` 暴露的 `/api/*` 接口。

## 开发

```bash
# 终端 1：后端（需已登录 cookie）
make serve

# 终端 2：前端 HMR（代理 /api → :8080）
cd web && npm run dev
```

浏览器打开 http://127.0.0.1:5173

## 构建 / 嵌入

```bash
make web        # 打包到 web/dist
make web-embed  # 打包并复制到 internal/httpapi/dist（go:embed）
make build      # 构建含嵌入前端的二进制
```

## 约定

- API 路径：axios `baseURL='/api'`，请求写相对路径如 `/accounts`（勿再写 `/api/...`）
- 比率语义：
  - `rate` / `total_rate` / `hold_rate` / `day_rate`：百分数（15.2 → 15.20%）
  - `position_rate`：小数（0.788 → 78.80%）
