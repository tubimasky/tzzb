# 投资账本 API 开发文档

本文档记录同花顺「投资账本」(tzzb) 后端 API 的逆向结果与本 CLI 的实现原理。
新 agent 读完本文即可继续开发新命令，无需再抓包。

数据来源：Chrome 插件 `~/Downloads/tzzb-plugin` 的前端代码 + 真实请求验证。
参考实现：`~/github/stock/src/stock_assistant/integrations/tzzb.py`（Python，最早跑通此 API）。

---

## 1. 认证与反爬（最关键，先看这里）

### 1.1 域名与路径前缀

| 项 | 值 |
|---|---|
| BaseURL | `https://tzzb.10jqka.com.cn/caishen_httpserver/tzzb` |
| Origin  | `https://tzzb.10jqka.com.cn` |
| Referer | `https://tzzb.10jqka.com.cn/pc/` |

- 生产域名 `tzzb.10jqka.com.cn`，公共路径前缀 `/caishen_httpserver/tzzb`，接口路径再拼 `/caishen_fund/...`。
- 插件里还出现过测试环境 `pretestcapital.hexin.cn`、`//172.21.52.127`，**不要用**，那是内网/测试地址。
- 前端拼接逻辑（插件 JS 反混淆后）：`buildForwardUrl(t) => this.server + "/caishen_httpserver/tzzb" + t`，`this.server = "//tzzb.10jqka.com.cn"`。

### 1.2 绕过 403 的四个必要条件

直接用 Cookie 发 GET 会被 CDN 层（openresty/squid）拦掉，返回 `HTTP 403` + 文本 `请求失败，请稍后重试`。请求根本到不了后端。绕过它必须**同时满足**：

1. **方法是 POST**，不是 GET。
2. **body 是表单编码** `application/x-www-form-urlencoded`，不是 JSON。
3. **带 `origin` 和 `referer` 头**（值见上表）。这是 WAF 校验的核心，缺一个就 403。
4. **带完整 Cookie 头**。

> 排查经验：如果收到 403 + `请求失败，请稍后重试`，先确认上面 4 点，而不是怀疑 Cookie 过期。
> 真正的 Cookie 过期通常表现为 HTTP 200 但 `error_code != 0`，或 401。

### 1.3 Cookie 与 uid

- 认证完全依赖浏览器登录后的 Cookie（插件用 `chrome.cookies.getAll` 抓 `tzzb.10jqka.com.cn` 的 cookie）。
- 从 Cookie 里解析用户 id，按优先级找这些 key：`userid` → `user_id` → `uid` → `hexin_uid` → `u`。
- uid 会作为公共参数 `userid` / `user_id` 放进每个请求的 body。
- CLI 把 Cookie 存到 `~/.tzzb/cookies`（权限 0600），uid 在保存时解析。

### 1.4 公共请求参数

每个 POST 的 body 都会带上这四个公共参数（再叠加各接口自己的参数）：

```
terminal=1
version=0.0.0
userid=<uid>
user_id=<uid>
```

---

## 2. 统一响应结构

所有接口返回同一个外层信封（envelope）：

```json
{
  "error_code": "0",
  "error_msg": "success",
  "ex_data": { ... 或 [ ... ] }
}
```

- `error_code == "0"` 表示成功；**注意它可能是字符串 `"0"` 也可能是数字 `0`**，解析时要兼容（本 CLI 用 `flexStr` 类型统一按字符串比较）。
- 业务数据全在 `ex_data` 里，形态随接口而变（对象或数组）。
- 失败时读 `error_msg`。

### 数值字段的坑

后端对数值字段**时而返回字符串时而返回数字**（例如 `"count": "10600"` vs `"value": 3907.4`）。
本 CLI 定义了 `Num` 类型，`UnmarshalJSON` 里剥掉引号再 `ParseFloat`，空串/null 归零。**新加任何数值字段都用 `Num`，不要用 `float64`**。

---

## 3. 已验证的接口

所有路径都相对于 BaseURL（即完整 URL = BaseURL + path）。

### 3.1 account_list — 账户列表

- **path**: `/caishen_fund/pc/account/v1/account_list`
- **额外参数**: `userid=<uid>`
- **用途**: 列出用户所有账户。

**重要：这个接口只返回账户元数据（名字、ID、fund_key 等），不含资产金额和持仓！**
资产和持仓必须对每个账户再单独调持仓接口（见 3.2 / 3.3）。这是第一版数值全是 0 的根因。

`ex_data` 是一个对象，按账户类型分四组，每组是数组：

```json
{
  "manual": [ ... ],   // 手动记账账户
  "fund":   [ ... ],   // 场内/场外基金账户
  "rzrq":   [ ... ],   // 两融账户
  "common": [ ... ]    // 券商同步的普通账户
}
```

各类账户元数据的关键字段：

| 类型 | 账户名字段 | 账户 ID 字段 | 持仓接口需要的参数 |
|---|---|---|---|
| `fund` | `fundname` | **`fundId`（大写 I！）** | `fundid` = fundId |
| `common` / `manual` | `manualname` | `manual_id` / `manualid` | `manual_id`、`fund_key`、`rzrq_fund_key` |
| `rzrq` | `manualname` | `manual_id` | 同上 + `rzrq_fund_key` |

> **大小写坑**：`account_list` 里基金账户的 id 字段是 `fundId`（大写 I），
> 但持仓接口返回体里又叫 `fundid`（小写）。struct 里两个都要接（`FundID` + `FundIDCap`），
> 取值时优先小写再大写（见 `Account.fundID()`）。

`common` 账户实际返回的 keys（脱敏样例）：
```
access_upload, manualid, fund_key, upload_time, brokername, merge_flag,
manual_id, manualname, qsid, broker_type, time, qsmc
```

`fund` 账户实际返回的 keys：
```
custid, date, fundId, fundname, merge_flag, type
```

### 3.2 stock_position — 股票/ETF/普通账户持仓

- **path**: `/caishen_fund/pc/asset/v1/stock_position`
- **参数**: `manual_id`、`fund_key`、`rzrq_fund_key`（从对应账户元数据取，有哪个传哪个）
- **用途**: 手动 / 普通 / 两融账户的持仓和资产。

`ex_data` 顶层字段：

```json
{
  "total_asset": "499209.7300",     // 总资产（注意是单数 total_asset！）
  "total_value": "393354.900",       // 总市值
  "money_remain": "105854.8300",     // 可用现金
  "total_liability": "0",            // 负债（两融）
  "position_rate": "0.7880",         // 仓位（小数，0.788 = 78.8%）
  "upload_time": 1784258826000,      // 毫秒时间戳
  "position": [ ... ]                // 持仓明细数组
}
```

> **字段名坑**：这里是 `total_asset`（单数），而 3.3 基金接口/文档别处是 `total_assets`（复数）。
> 顶层**没有** hold_profit / today_profit，这两个要从持仓行累加。

### 3.3 fund_position — 基金账户持仓

- **path**: `/caishen_fund/pc/asset/v1/fund_position`
- **参数**: `fundid=<fundId>`
- **用途**: 基金账户的持仓。

`ex_data` 顶层字段随场景变化，从 account_list 列出的账户查询时，顶层通常**只有** `id` / `position` / `cur_position`，
资产和盈亏也需从持仓行累加。（单独用某些 fundid 直查时能看到 `total_assets` 等顶层字段，但不可依赖。）

```json
{
  "id": "...",
  "position": [ ... ],       // 当前持仓
  "cur_position": [ ... ]    // 可能为空
}
```

### 3.4 持仓行（position 数组元素）通用字段

stock 和 fund 的持仓行字段高度一致，本 CLI 的 `Position` 只取通用子集：

| JSON 字段 | 含义 | CLI 字段 |
|---|---|---|
| `code` | 证券代码（如 `515790.SZ` / `159227`） | Code |
| `name` | 名称 | Name |
| `count` | 持仓数量 | Count |
| `cost` | 成本价 | Cost |
| `price` | 现价 | Price |
| `value` | 市值 | Value |
| `hold_rate` | 持仓收益率（**小数**，-0.1489 = -14.89%） | HoldRate |
| `hold_profit` | 持仓盈亏 | HoldProfit |
| `w_profit` | **本周盈亏**（不是当日；前端表头 BZYK） | WProfit |

> **当日盈亏**在持仓行里常为空（`now_profit`）。账户级当日盈亏取 `stock_card`/`fund_card` 的 `now_profit`。

持仓行里还有一堆没用到的字段（`back`/`m1_rate`/`m3_rate`/`m6_rate`/`m12_rate`/`hold_days`/`stock_account`/`pre_profit`/`y_profit`/`close_profit` 等），
需要时按上表方式补进 `Position` struct 即可。

### 3.5 stock_card / fund_card — 账户卡（账户级汇总）

网页「我的账户」卡片同源接口，**账户列表展示应优先用这里**：

| | stock_card | fund_card |
|---|---|---|
| path | `/caishen_fund/pc/account/v1/stock_card` | `/caishen_fund/pc/account/v1/fund_card` |
| 参数 | `manual_id` / `fund_key` / `rzrq_fund_key` | `fundid` |

`ex_data` 关键字段：

```json
{
  "asset": 499209.73,        // 总资产
  "profit": -18402.8,        // 持仓盈亏
  "rate": -0.0447,           // 持仓收益率(小数)
  "now_profit": "-7220.20",  // 当日盈亏（字符串或数字）
  "now_rate": "-1.41",       // 当日收益率（百分比数，-1.41 = -1.41%）
  "position": [ ... ],       // 简化持仓（无盈亏字段），CLI 不用
  "data": [ ... ]            // 分时曲线，CLI 不用
}
```

> 踩坑：持仓行 `w_profit` ≠ 当日。用 `sum(w_profit)` 当当日盈亏会把「本周」错当「当日」，
> 且同 code 拆多行时 `w_profit` 会重复，国*证券实测误差上万。

### 3.6 get_money_history — 资金/交易流水

- **path**: `/caishen_fund/pc/account/v1/get_money_history`
- **参数**: `manual_id` / `fundid` / `fund_key`（有哪个传哪个）
- **状态**: **结构尚未完全确认**。CLI 用 `extractRecords` 做兜底解析（兼容"顶层是数组"或"顶层对象里嵌一个数组"两种形态），返回 `[]map[string]any`。
- 后续要做严格类型，需要先抓真实响应确认字段。

### 3.7 其它已知但未接入的接口

从插件 JS 里扒到的全部端点（`/caishen_fund/...`），未接入的可按需开发：

```
cloud/v1/stockdict                          common/v1/post_scan
delivery/v1/position_list                   delivery/v1/update_position
manual_stock/fee_config/v1/edit             manual_stock/fee_config/v1/query
pc/account/v1/add_account                   pc/account/v1/del_account
pc/account/v1/edit_account                  pc/account/v1/init
pc/account/v1/init_fund                     pc/account/v1/qs_list
pc/account/v1/status                        pc/account/v1/status_fund
pc/account/v1/merge_day_trading             pc/account/v1/delete_money_history
pc/account/v1/edit_money_history            pc/account/v1/add_money_history
pc/account/v1/edit_stock_position_history
pc/asset/v1/asset_trend                     pc/asset/v1/fund_quota
pc/asset/v1/merge_compare
pc/asset/v1/query_bank_history              pc/asset/v1/add_bank_history
pc/asset/v1/add_stock_position              pc/asset/v1/clear_stock
pc/asset/v1/delete_stock                    pc/asset/v1/time_share
pc/stock/v1/query_suspension_code           stock_common/v1/hk_rate
stock_common/v1/last_trading_day
```

> 已接入：`account_list` / `stock_position` / `fund_position` / `stock_card` / `fund_card` / `get_money_history`。
> `asset_trend` 看名字应该是资产趋势曲线，做收益趋势图时优先验证它。

---

## 4. CLI 实现原理

### 4.1 目录结构

```
main.go                 入口，调 cmd.Execute()
go.mod
pkg/api/client.go       API 客户端（认证、请求、解析、数据模型）——核心
cmd/root.go             根命令，注册子命令，初始化 apiClient
cmd/login.go            login：三种方式传 Cookie
cmd/account.go          account list：账户列表 + 汇总，含 colorNum 工具
cmd/position.go         position list：持仓明细，含 colorPct 工具
cmd/trade.go            trade list：资金流水（依赖 get_money_history）
cmd/profit.go           profit summary：收益汇总
cmd/export.go           export account/position：CSV/JSON 导出
docs/API.md             本文档
```

用 cobra 搭命令，fatih/color 上色，olekukonko/tablewriter 出表格。

### 4.2 请求流水线（client.go）

```
post(path, params)           拼公共参数 → 表单编码 → 加 4 个必要头 → 发 POST → 查 403/非200
  └─ call(path, params)      再剥 envelope 外壳 → 校验 error_code → 返回 ex_data(RawMessage)
       └─ GetXxx()           把 ex_data 反序列化成业务 struct
```

- `post`：Cookie 为空直接报"未登录"。403 专门提示 Cookie 过期。
- `call`：统一处理 `error_code`，非 0 返回 `接口错误[code]: msg`。
- 各 `GetXxx` 只管把 `ex_data` 解析成自己的结构。

### 4.3 账户富化（GetAccountList 的核心逻辑）

因为 account_list 不含金额/持仓，`GetAccountList` 的流程是：

1. 调 account_list，把 manual/fund/rzrq/common 四组打平成 `[]Account`，给每个打上 `Category`。
2. **逐个账户**调 `fetchAccountDetail`：基金走 `fund_position`，其余走 `stock_position` → `mergeDetail`。
3. **再调** `fetchAccountCard`：基金走 `fund_card`，其余走 `stock_card` → `mergeCard`。

`mergeDetail`（持仓明细侧）：

- **持仓明细 / 仓位 / 更新时间 / 现金**：直接用持仓接口返回值。
- **持仓盈亏 `HoldProfit`**：从持仓行 `hold_profit` 累加（card 成功后会被覆盖）。
- **市值 `MarketValue`**：优先顶层 `total_value`，没有才用持仓行累加。
- **总资产**：优先 `total_asset` → `total_assets` → 市值+现金（card 成功后会被覆盖）。
- **不要**用持仓行 `w_profit` 当当日盈亏：它是**本周盈亏**，且同 code 多行时常重复。

`mergeCard`（与网页账户卡同源，覆盖账户级数字）：

| CLI 字段 | card 字段 | 含义 |
|---|---|---|
| `TotalAssets` | `asset` | 总资产 |
| `HoldProfit` / `TotalProfit` | `profit` | 持仓盈亏 |
| `TodayProfit` | `now_profit` | **当日盈亏** |

基金仓位：position 接口常不给 `position_rate`，用 `市值/总资产` 估算。

> 富化是串行的（每账户 position + card），账户多时会慢。
> 单个接口失败不中断整体：position 失败则无明细；card 失败则保留 position 汇总（当日盈亏保持 0，不用 w_profit 凑）。

### 4.4 已知数据问题：持仓重复行

后端会返回**重复/拆分行持仓**（华宝 `164701 黄金LOF` 6 行不同 `stock_account`；国*证券 `512890` 两行不同账户）。

- **市值 / 持仓盈亏**：按行累加，与顶层 `total_value` 一致，**不要去重**。
- **`w_profit`（本周）**：同 code 多行常是同一值，**不能**按行直接加总当账户周收益；账户级当日/持仓盈亏一律信 card。

---

## 5. 命令一览

```bash
# 登录（Cookie 很长，终端交互式粘贴会因单行长度限制卡住，用下面三种之一）
pbpaste | tzzb login                 # 从剪贴板管道输入（推荐）
tzzb login --file cookie.txt         # 从文件读
tzzb login --cookie "userid=...; ..."# 直接参数传

# 查看
tzzb account list                    # 账户列表 + 汇总
tzzb position list                   # 全部持仓
tzzb position list -a 华宝           # 按账户名模糊过滤
tzzb profit summary                  # 收益汇总

# 导出（CSV 带 UTF-8 BOM，Excel 打开中文不乱码）
tzzb export account  -f csv  -o 账户.csv
tzzb export position -f json -o 持仓.json
```

> `login` 卡住的根因：macOS 终端规范模式下单行输入约 1024 字节上限，Cookie 远超此值。
> 用管道/文件绕开，不要走交互式输入。

---

## 6. 给新 agent 的开发提示

- **加新接口**：在 `client.go` 加 `epXxx` 常量 → 写 `GetXxx()` 调 `c.call(epXxx, params)` → 定义对应 struct（数值字段一律用 `Num`）→ 在 `cmd/` 加子命令并 `AddCommand`。
- **不确定响应结构时先抓真实数据**，别凭字段名猜（本项目踩过 `total_asset` vs `total_assets`、`fundId` vs `fundid` 两个大小写/单复数坑）。抓包方式：带上 §1.2 的 4 个条件用 curl/脚本直接打接口，dump `ex_data` 的 keys。
- **验证要用真实 Cookie 跑**，编译通过 ≠ 功能对。第一版数值全 0 就是编译通过但字段映射全错。
- **改汇总口径**去 `mergeDetail` 看 §4.3，注意重复行问题（§4.4）。
- 参考 Python 实现 `~/github/stock/src/stock_assistant/integrations/tzzb.py`，那里有更完整的账户记录挖掘逻辑（`find_tzzb_account_records`、`walk_dicts`）。
