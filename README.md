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

工具用浏览器登录态的 Cookie 认证。获取步骤：

1. 浏览器打开 https://tzzb.10jqka.com.cn/pc 并登录
2. F12 打开开发者工具 → Network 标签
3. 刷新页面，点任意一个 `caishen_httpserver` 请求
4. 在 Request Headers 里复制完整的 `Cookie` 值

由于 Cookie 很长，终端交互式粘贴会因单行长度限制卡住，**推荐用管道或文件传入**：

```bash
# 从剪贴板管道传入（最省事）
pbpaste | tzzb login

# 从文件读取
tzzb login --file cookie.txt

# 直接参数传入
tzzb login --cookie "userid=xxx; u_name=xxx; ..."
```

Cookie 存到 `~/.tzzb/cookies`（权限 0600）。工具会自动从 Cookie 里解析 `userid`。

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

## 已知限制

- **本金/总收益率**：持仓接口不返回本金，`profit summary` 的本金列和总收益率会显示 0。总盈亏用持仓盈亏近似（不含已清仓的历史收益）。详见 `docs/API.md`。
- **重复持仓行**：某些账户后端会返回完全相同的重复持仓行。经核对，后端顶层市值口径**包含**这些重复行，故工具不去重，以与网页端数值一致。
- **Cookie 过期**：过期后接口返回 403，重新 `tzzb login` 即可。

## 依赖

- [cobra](https://github.com/spf13/cobra) — 命令行框架
- [color](https://github.com/fatih/color) — 彩色输出
- [tablewriter](https://github.com/olekukonko/tablewriter) — 表格输出

## License

MIT
