# 投资账本 CLI 工具

基于同花顺投资账本（tzzb）的命令行工具，用浏览器登录后的 Cookie 调其 PC 端 API，支持查看账户汇总、持仓明细、交易流水、收益汇总和数据导出。

> 实现原理、接口细节、反爬绕过、字段坑位等开发向文档见 [`docs/API.md`](docs/API.md)。二次开发前务必先读。

## 编译

```bash
make          # 或 make build → 生成 ./tzzb
make install  # 装到 $(go env GOPATH)/bin
make run ARGS="account list"
```

也可用 `go build -o tzzb .`，该方式只保证 CLI 和 HTTP API 可用，不会嵌入 Web 前端。
要构建包含 Web 界面的单文件程序，请使用 `make build`。`make help` 看全部目标。

## 使用

### 1. 登录（配置 Cookie）

**方式一（推荐，完全不需要浏览器）：同花顺投资账本 APP 扫码**

```bash
tzzb login --qr
```

终端里会直接画出二维码（按终端宽高自适应；终端太矮时自动改用看图程序打开
`~/.tzzb/qrcode.png`），手机打开同花顺投资账本 APP → 扫一扫 → 确认登录即可。
二维码失效会自动刷新。全程纯 HTTP，不走图形验证码 / 滑块 / 短信。

> 二维码内容是给 APP 的「扫一扫」解析 qrid 用的；用手机系统相机或浏览器直接打开
> 那个 URL 会跳首页，属正常现象。扫码链路与官方登录页 `qrcode.js` 一致
> （`/scan/creatCode` → `/scan/creatImg` → `/scan/getInfoNew`），
> `TZZB_QR_SOURCE` 可覆盖二维码 URL 形态（默认 `tzzb_web`）。

**方式二：浏览器 Cookie（自动读取）**

工具用浏览器登录态的 Cookie 认证。**默认自动从本机浏览器读取，不需要手工复制**：

```bash
tzzb login
```

只要浏览器里登录过 https://tzzb.10jqka.com.cn/pc，这一条就能直接成功。它会扫描
Chrome / Edge / Brave / Chromium 的所有 profile，按登录新鲜度逐个联网校验，取第一个
可用的写入 `~/.tzzb/cookies`（权限 0600），并从中解析 `userid`。

首次运行 macOS 会弹钥匙串授权框（读取浏览器 Cookie 的解密密钥），请点**“始终允许”**。
只想读某个浏览器时加 `--browser chrome`。

Cookie 过期后，其它命令与 `tzzb serve` 会自动重新读取浏览器登录态并重试一次，
不再需要手动重新登录（`serve` 也不用重启）。

也可以手工导入 Cookie：

```bash
# 从文件读取
tzzb login --file cookie.txt

# 直接参数传入
tzzb login --cookie "userid=xxx; u_name=xxx; ..."

# 从剪贴板管道传入（Cookie 很长，交互式粘贴会被终端单行长度限制卡住）
pbpaste | tzzb login
```

### 2. 账户列表

```bash
tzzb account list
```

列出全部账户（手动/基金/两融/普通）及资产、持仓盈亏、当日盈亏、仓位，并给出汇总行。

### 3. 持仓明细

```bash
tzzb position list              # 全部账户的持仓
tzzb position list -a 华宝      # 只看名字含"华宝"的账户（模糊匹配）
tzzb detail 159227              # 单只钻取：合并明细/建仓记录/历史成交/清仓汇总
tzzb detail 159227 -i           # 交互式界面（Tab 切换四块、多账户切换、成交表可滚动）
```

### 4. 交易流水

```bash
tzzb trade list                 # 全部账户
tzzb trade list -a 华宝         # 按账户名过滤
```

> 流水接口后端字段不固定，工具动态列出字段并优先展示常见列（日期/类型/代码/名称/数量/价格等）。

### 5. 收益汇总

```bash
tzzb profit summary
```

按账户汇总持仓盈亏、当日盈亏、总盈亏，并按总盈亏降序排列。

### 6. 导出数据

```bash
# 账户汇总
tzzb export account -f csv -o accounts.csv
tzzb export account -f json

# 持仓明细（扁平化，每行带所属账户）
tzzb export position -f csv -o positions.csv
tzzb export position -f json
```

默认格式 csv，默认文件名 `accounts_YYYYMMDD.csv` / `positions_YYYYMMDD.csv`。CSV 带 UTF-8 BOM，Excel 打开中文不乱码。

## 命令一览

| 命令 | 说明 | 参数 |
| --- | --- | --- |
| `login` | 保存 Cookie | `-c/--cookie`、`-f/--file`，或管道 |
| `refresh` | 触发服务端重算（`init`/`status`），让持仓/现金/总资产变新；等价于打开一次 PC 网页，零浏览器 | `--timeout` 等待上限（默认 30s）/ `-j` JSON |
| `account list` | 账户列表+汇总 | 无 |
| `position list` | 按账户持仓明细 | `-a` 账户 / `-t all\|stock\|fund` / `--flat` |
| `position total` | 跨账户按代码汇总 | `-a` / `-t all\|stock\|fund` |
| `position stock` | 股票/ETF 汇总 | `-a` |
| `position fund` | 场外基金汇总 | `-a` |
| `trade list` | 交易流水 | `-a/--account` 账户名过滤 |
| `profit summary` | 收益汇总（账户维度快照） | 无 |
| `profit month` | 月度盈亏（某年各月，含收益率/大盘对比） | `-a` / `-y` 年份 / `-t` Top N |
| `profit range` | 区间盈亏（每日 + 区间个股盈亏） | `-a` / `-s` 起始 / `-e` 结束（YYYYMMDD）/ `-t` Top N |
| `analysis dist` | 持仓分布（按市值占比） | `-a` / `-t` Top N |
| `analysis rate` | 账户仓位率 | `-a` |
| `detail <代码>` | 单只股票钻取明细（合并/建仓/成交/清仓） | `-a` / `--from` / `--to`（YYYYMMDD）/ `-i` 交互式 TUI |
| `trend show` | 单账户资产趋势 | `-a` / `-d` 天数 / `-b` 大盘对比 |
| `trend summary` | 聚合资产趋势 | `-t all\|stock\|fund` / `-d` / `-b` |
| `bank list` | 银证转账明细 | `-a` / `-d` 天数 |
| `calendar month` | 盈亏月历（某月每日） | `-a` / `-m` 月份 / `-t` Top N |
| `calendar year` | 盈亏年视图（某年每月） | `-a` / `-y` 年份 / `-t` Top N |
| `export account` | 导出账户 | `-f/--format` csv/json、`-o/--output` |
| `export position` | 导出持仓 | `-f/--format` csv/json、`-o/--output` |

> 服务端汇总值（持仓/现金/总资产）不是实时计算的：手机投资账本 APP 上传新数据后，
> 需要触发一次 `init` 才会重算。所以**手机 APP 同步完成后，先执行 `tzzb refresh`，再查询**
> 才能拿到最新值。手机端「同步/上传」是产品设计约束，CLI 无法代替。

## 反爬（`hexin-v`）

接口要求每个请求都携带一枚**新的**反爬 token（Cookie 里的 `v` / `hexin-v` 头），它由页面上的
`chameleon.min.<ver>.js` 生成。2026-10-28 抓包确认：官方 PC 页面是**每个请求换一枚新 token
并写回 Cookie 的 `v`**，**不发 `hexin-v` 头**；服务端两种都认，关键是 token 不能复用：

| 发法 | 结果 |
| --- | --- |
| 旧 Cookie `v` + 不带 `hexin-v` 头 | 403 |
| 新 Cookie `v`（每请求轮换） | 200（官方页面行为） |
| 旧 Cookie `v` + 新 `hexin-v` 头 | 200 |
| 固定一个 token 反复用 | 前 8~10 个 200，之后全部 `<h1>Nginx forbidden.</h1>` |
| **每请求换新 token** | **零延迟连发几十个全部 200** |

token 是 60 字符 URL-safe base64，解码后 45 字节（版本 3），payload 里只有时间戳和
计数器会变、其余是会话模板，所以 `internal/hexinv` 用两条路径：

1. **数学刷新（主路径，不跑 JS）**：解码 Cookie 里的 `v` → 时间戳改当前秒、计数器 +1 →
   重算校验位 → 重编码，写回**内存** Cookie（不写盘），同时设 `hexin-v` 头；
2. **chameleon 兜底**：Cookie 里没有可识别的 v3 token 时，用纯 Go 的 JS 引擎
   [goja](https://github.com/dop251/goja) + 最小 DOM 垫片跑内置的官方 chameleon 脚本现算一枚
   （不需要 Chromium、不需要 Node；初始化约 30ms，之后每枚 token 微秒级）。

被 403 拒绝时会换一枚新 token 有界重试（最多 2 次，300ms/900ms），连续失败才按登录态
失效上报。脚本版本约 `Unix秒/1000`（每 ~17 分钟滚动），缓存 30 分钟，失败自动退回内置副本，
完全离线也能工作。

相关环境变量：`TZZB_NO_HEXIN=1`（关闭自动刷新/生成）、`TZZB_CHAMELEON_JS=<path>`
（指定本地脚本）、`TZZB_REQUEST_INTERVAL_MS=<n>`（请求间隔，默认 100ms）。

## 已知限制

- **本金/总收益率**：持仓接口不返回本金，`profit summary` 的本金列和总收益率会显示 0。总盈亏用持仓盈亏近似（不含已清仓的历史收益）。详见 `docs/API.md`。
- **重复持仓行**：某些账户后端会返回完全相同的重复持仓行。经核对，后端顶层市值口径**包含**这些重复行，故工具不去重，以与网页端数值一致。
- **Cookie 过期**：过期后接口返回 403（与反爬拦截页长得一样），重新 `tzzb login` 即可。

## 依赖

- [cobra](https://github.com/spf13/cobra) — 命令行框架
- [color](https://github.com/fatih/color) — 彩色输出
- [tablewriter](https://github.com/olekukonko/tablewriter) — 表格输出

## License

MIT
