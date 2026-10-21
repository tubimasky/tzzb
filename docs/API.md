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

### 1.2 绕过 403 的必要条件

直接用 Cookie 发 GET 会被 CDN 层（openresty/squid）拦掉，返回 `HTTP 403` + 文本 `请求失败，请稍后重试`。请求根本到不了后端。绕过它必须**同时满足**：

1. **方法是 POST**，不是 GET。
2. **body 是表单编码** `application/x-www-form-urlencoded`，不是 JSON。
3. **带 `origin` 和 `referer` 头**（值见上表）。这是 WAF 校验的核心，缺一个就 403。
4. **带完整 Cookie 头**。
5. **带 `hexin-v` 头，且每个请求都要换一个新值**。这是 2026-10-22 才定位到的
   **最关键的第五条**，详见 §1.5。

> 排查经验：如果收到 403 + `请求失败，请稍后重试`，先确认上面 4 点，而不是怀疑 Cookie 过期。
> 真正的 Cookie 过期通常表现为 HTTP 200 但 `error_code != 0`，或 401。
>
> CLI 现在把 401/403 都当作"可能过期"处理：会自动从浏览器重新读取 Cookie 并重试一次，
> 只有拿到与当前不同的新 Cookie 才重试（所以 WAF 拦出来的 403 不会被误判）。

### 1.5 hexin-v 反爬 token（403 的真正主因）

页面上的 `chameleon.min.<ver>.js` 会给每个 XHR 注入 `hexin-v` 头，值等于 Cookie 里的
`v`。实测结论（2026-10-22，同一台机器/同一出口 IP/同一 Cookie）：

| 发法 | 结果 |
| --- | --- |
| 不带 `hexin-v` | 200（能过，但拿到的是空模板数据） |
| 固定一个 `hexin-v` 复用 | 前 8~10 个 200，之后全部 `<h1>Nginx forbidden.</h1>` |
| **每请求换一个新 `hexin-v`** | **零延迟连发几十个全部 200** |

所以 403 的成因既不是 IP 封禁，也不是频率限流，而是 **token 复用超限**。浏览器里看起来
"永不封"，只是因为 chameleon 每发一个请求就重新生成一次 token。

token 形态：60 字符 URL-safe base64，以 `A` 开头，编码了时间等信息。生成它需要跑
chameleon 脚本（混淆过的），本仓库的做法见 `internal/hexinv`：

- 用 `go:embed` 内置一份 chameleon 脚本；
- 用纯 Go 的 JS 引擎 [goja](https://github.com/dop251/goja) + 最小 DOM 垫片把它跑起来；
- 每次要 token 时触发一次 `XMLHttpRequest.open`，chameleon 就会把新 `v` 写回
  `document.cookie`，取出来即可。

不需要 Chromium，也不需要 Node。脚本可以自动跟进版本：版本号约为 `Unix秒/1000`
（每 ~17 分钟滚动），旧版本 CDN 仍可访问，见 `hexinv.ScriptURLTemplate`。

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

另外每个请求还会自动带一个**全新生成**的 `hexin-v` 头（见 §1.5），由
`internal/hexinv` 负责，业务代码无需关心。

---

### 1.6 扫码登录（同花顺 APP 扫码，纯 HTTP）

比短信更稳的通道：不走图形验证码、不走滑块、不发短信，用户用同花顺投资账本 APP
（或同花顺 APP）扫码确认即可。实测 2026-10-22 可用。

| 步骤 | 请求 | 说明 |
| --- | --- | --- |
| 1 | `GET /scan/creatCode` | 返回 `{"qrid":"usk_..."}` |
| 2 | `GET /scan/creatImg?qrid=<qrid>&source=tzzb_web` | 返回 252×252 PNG 二维码（41×41 模块，每模块 6px） |
| 3 | `POST /scan/getInfoNew` | 表单 `qrid`、`state=1`、`source=pc_web`、`page_source=web_screen`、`request_type=login` |

轮询返回 `status`：

| status | 含义 |
| --- | --- |
| `0` | 二维码失效，需重新 `creatCode` / `creatImg` |
| `1` | 等待扫码 |
| `2` | 已扫码，等待手机上确认 |
| `3` | 确认成功，会话已登录（响应同时下发登录态 Cookie） |

要点：

- `creatImg` 的 `source` 用页面场景代号（tzzb 是 `tzzb_web`），与二维码里编码的
  `qrlogin?source=...` 一致；
- 轮询响应里的 `expired=1` 在**正常等待时**也会出现，判断失效只能看 `status=0`；
- 二维码是 41×41 模块：终端里用"1 模块 = 1 字符宽 × 2 模块高"的半方块渲染，
  实测可被手机识别；终端放不下时不要缩放（会丢模块），应改用图片窗口；
- 二维码内容由 `source` 决定：`tzzb_web`（默认）→ `https://tzzb.10jqka.com.cn/qrlogin?source=tzzb_web&qrid=...`；
  `pc_web`/`web_screen`/空 → `http://mobile.10jqka.com.cn/?source=PC&qrid=...`。两种 qrid 状态机相同，
  都需用同花顺投资账本 APP 的「扫一扫」解析（用系统相机/浏览器打开会跳首页）；
- `TZZB_QR_SOURCE` 可覆盖 `source`（排查 APP 扫码兼容性用）。

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

## 3. 代码结构

本 CLI 的代码按职责分层：

```
pkg/api/           # API 客户端（与同花顺后端交互）
├── client.go      # HTTP 客户端基础设施、通用类型、常量
├── quote.go       # 行情接口（GetQuotes）
├── account.go     # 账户接口（GetAccountList、账户详情、账户卡）
├── position.go    # 持仓类型、插件口径重算（RecomputeLive）
└── trade.go       # 交易流水（GetDayTrades、GetMoneyHistory、GetAssetTrend）

internal/service/  # 业务逻辑服务层
└── calculator/    # 计算服务（持仓汇总、收益计算）
    ├── calculator.go        # Calculator 接口定义
    ├── position.go          # 持仓汇总实现
    └── profit.go            # 收益计算实现

internal/formatter/ # 格式化输出层
├── account.go     # 账户列表格式化
├── position.go    # 持仓明细格式化
├── trade.go       # 交易流水格式化
├── profit.go      # 收益汇总格式化
└── trend.go       # 资产趋势格式化

cmd/               # 命令行入口
├── root.go        # 根命令
├── account.go     # account 子命令
├── position.go    # position 子命令
├── trade.go       # trade 子命令
├── profit.go      # profit 子命令
└── ...
```

**设计原则：**

- **pkg/api** 只负责 HTTP 通信和 JSON 解析，返回原始数据结构，不做业务逻辑
- **internal/service** 实现业务逻辑（汇总、计算、过滤），调用 api 层获取数据
- **internal/formatter** 负责输出格式化（表格、CSV、JSON），依赖 service 层
- **cmd** 只做参数解析和流程编排，调用 service 和 formatter

## 4. 已验证的接口

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

- **path**: `/caishen_fund/pc/account/v1/get_money_history`（PC 前端现多用 v2；CLI 固定 **v1**，v2 易繁忙）
- **踩坑**: 账户相关键即使为空串也必须出现在 form 里，否则 Spring **HTTP 400**。
- **参数**（与 PC `queryAccountHistory` 对齐）:

| 字段 | 说明 |
|---|---|
| `manual_id` / `fund_key` / `rzrq_fund_key` / `fundid` / `custid` | 账户标识，没有则传 `""` |
| `start_date` / `end_date` | `YYYY-MM-DD`，CLI 默认近 3 个月 |
| `page` / `count` | 分页，CLI `count=100`，`page` 从 1 递增 |
| `query_list` / `sort_type` / `sort_order` / `h5id` | 可空；`sort_order=0`；`h5id` 首请求 `"0"`，之后用上一页返回值 |

---

### 3.6.1 trade 记录解析方法（核心）

trade 没有独立 struct。一条流水始终是 `map[string]any`，从 HTTP body 到表格/汇总分 **5 步**。

实现位置：

| 步骤 | 函数 | 文件 |
|---|---|---|
| 1 请求+信封 | `post` → `call` | `pkg/api/client.go` |
| 2 单页解析 | `parseMoneyHistoryPage` / `extractRecords` | 同上 |
| 3 自动翻页拼全量 | `GetMoneyHistory` | 同上 |
| 4 明细列映射 | `renderRecords` / `pickField` | `cmd/trade.go` |
| 5 买卖归类汇总 | `tradeAgg.add` / `mapFloat` | 同上 |

#### 步骤 1：HTTP → `ex_data`

```
POST form body
  → post() 读原始 JSON
  → call() 解 envelope{error_code, error_msg, ex_data}
  → error_code 必须为 "0"，否则报错
  → 返回 ex_data (json.RawMessage)
```

公共参数 `terminal/version/userid/user_id` 与接口参数一并 `form.Set`；**空串不丢弃**。

#### 步骤 2：`ex_data` → 单页 `moneyHistoryPage`

结构体：

```go
type moneyHistoryPage struct {
    List    []map[string]any `json:"list"`     // 本页流水
    Page    Num              `json:"page"`
    MaxPage Num              `json:"max_page"` // 总页数
    Count   Num              `json:"count"`
    Total   Num              `json:"total"`    // 总条数
    H5ID    string           `json:"h5id"`     // 翻页游标
}
```

`parseMoneyHistoryPage(ex)` 顺序：

1. `ex` 为空 / `"null"` → 空页。
2. `json.Unmarshal` 进 `moneyHistoryPage`；若 `list != nil` **或** `max_page != 0` **或** `total != 0` → 采用（`list` 为 nil 时置 `[]`）。
3. 否则走兼容路径 `extractRecords(ex)`：
   1. 有 `{"list":[...]}` → 用 `list`
   2. 顶层就是数组 `[...]` → 直接用
   3. 对象里再试键：`list` / `data` / `stock` / `records` / `history`

单条记录**不**再 unmarshal 成强类型；保持 `map[string]any`，避免接口加字段就挂。

#### 步骤 3：多页拼接 → 全量 `[]map[string]any`

`GetMoneyHistory(acc)`：

```
start = today-3m, end = today
h5id = "0"
all = []
for page = 1 .. 20:
    请求参数 = 账户键 + 日期 + count=100 + page + h5id
    ex = call(v1/get_money_history, 参数)
    pg = parseMoneyHistoryPage(ex)
    if pg.H5ID != "" { h5id = pg.H5ID }   // 下一页游标
    all = append(all, pg.List...)
    结束条件:
      - max_page > 0 且 page >= max_page
      - list 为空
      - 无 max_page 且本页 len(list) < 100
return all
```

到这里，CLI 手里是「该账户近 3 个月全部流水 map」。

#### 步骤 4：单条 map → 明细表列（`trade list`）

`renderRecords` **不读全字段**，只按固定中文列从候选键里取第一个非空值：

| 展示列 | 解析键顺序（`pickField`） |
|---|---|
| 日期 | `entry_date` → `date` |
| 时间 | `entry_time` → `time` |
| 类型 | `op_name` → `type` → `op` |
| 代码 | `code` |
| 名称 | `name` |
| 数量 | `entry_count` → `count` |
| 价格 | `entry_price` → `price` |
| 金额 | `entry_money` → `amount` → `money` |
| 费用 | `fee` → `commission` |

规则：

- 某列在**本批记录**里所有候选键都无值 → 该列不显示。
- 全部列都空 → 回退「所有 key 并集 + 字母序」动态表；key 以 `_` 开头的内部字段跳过。
- 单元格：`fmt.Sprintf("%v", v)`，空 / `"<nil>"` 当无值。

实测国* / 华宝走固定列即可，常见原始字段即上表左侧 API 名。

#### 步骤 5：单条 map → 买卖汇总语义（`trade summary`）

`tradeAgg.add(r)` 从 map 抽数并归类：

```
op    = trim(string(r["op_name"]))
money = mapFloat(r, "entry_money")
cnt   = mapFloat(r, "entry_count")
fee   = mapFloat(r, "fee"); 若 0 再 mapFloat(r, "commission")
fee 始终累加

if op 含 "银行":
    otherN++; return          // 银证转账：不进买卖

if op 含 "买" 或 op ∈ {转入, 担保品转入}:
    buyAmt += money; buyCnt += cnt; buyN++
elif op 含 "卖" 或 op ∈ {转出, 担保品转出}:
    sellAmt += money; sellCnt += cnt; sellN++
else:
    otherN++

name 取第一条非空 r["name"]
净买入 = buyAmt - sellAmt   // 正=净加仓/净投入，负=净减仓
```

`mapFloat` 解析顺序：`float64` / `float32` / `int` / `int64` / `json.Number` / 去逗号后的数字字符串；失败当 0。

汇总维度（在已解析的全量 records 上再聚合，**不再请求接口**）：

1. **按账户**：每个账户自己的 records 跑一遍 `tradeAgg`。
2. **按代码**：`code` 为空 → `"(无代码)"`；只输出有买卖笔的代码；按 `buyAmt+sellAmt` 降序，Top 40。
3. **总汇总**：全部 records 一个 `tradeAgg`。

summary 跨账户时会在 map 上临时写 `_account`（仅内存；步骤 4 动态表会跳过 `_` 前缀）。

#### 解析口径注意点

| 点 | 说明 |
|---|---|
| 无强类型 Trade 结构体 | 全程 `map[string]any`，字段名以接口为准 |
| 分类主键是中文 `op_name` | 用 `Contains("买"/"卖"/"银行")`，不是枚举 code |
| 逆回购算买卖 | `204001`/`131810` 等 op 仍是买入/卖出，会撑大区间买卖总额 |
| 净买入 | 买入金额 − 卖出金额；正 = 净加仓/净投入，负 = 净减仓（标的视角，非账户现金回流） |
| 费用 | 每条都加，与买卖/银证归类独立 |
| 分页上限 | 最多 20 页 × 100 = 2000 条/账户 |

### 3.7 asset_trend — 资产趋势曲线

- **path**: `/caishen_fund/pc/asset/v1/asset_trend`
- **参数**: `days`（天数，默认 30）+ 账户标识（`manual_id` / `fund_key` / `rzrq_fund_key` / `fundid`）
- **用途**: 获取账户的资产趋势历史数据，用于绘制收益走势图。

`ex_data` 结构：

```json
{
  "total_asset": [     // 近 30 天完整数据
    {
      "date": "20260622",       // 日期（YYYYMMDD 格式）
      "asset": 379733.38,       // 总资产
      "profit": 559.50,         // 持仓盈亏（相对基准日）
      "zczs": 956.28,           // 增长指数（以基准为 1000 或基准值）
      "fundIn": 0,              // 资金转入
      "fundOut": 0              // 资金转出
    },
    ...
  ],
  "month_profit": [   // 本月数据（相对月初）
    {
      "date": "20260701",
      "asset": 395940.27,
      "profit": 3189.79,        // 7月1日相对 6月30日的盈亏增量
      "zczs": 948.58,
      "fundIn": 0,
      "fundOut": 0
    },
    ...
  ],
  "year_profit": [    // 年度数据（相对年初或建仓日）
    {
      "date": "20260717",
      "asset": 494237.52,
      "profit": -22300.01,      // 相对年初的累计盈亏
      "zczs": 945.89,
      "fundIn": 0,
      "fundOut": 0
    },
    ...
  ],
  "month_init_zczs": 938.69,   // 月初基准指数
  "year_init_zczs": 1000        // 年初基准指数
}
```

**重要字段说明**：

| 字段 | 含义 | 计算方式 |
|---|---|---|
| `date` | 日期 | YYYYMMDD 格式，需转换为 YYYY-MM-DD |
| `asset` | 总资产 | 当日收盘总资产 |
| `profit` | 盈亏 | **相对基准日**的盈亏增量（不是总盈亏） |
| `zczs` | 增长指数 | 以基准为 1000 的归一化指数 |
| `fundIn` / `fundOut` | 资金流 | 当日转入/转出金额 |

**数据集选择逻辑**：

| 查询天数 | 优先使用 | 基准指数 | 覆盖范围 |
|---|---|---|---|
| `days <= 31` | `month_profit` | `month_init_zczs` | 本月数据（约 20 个交易日） |
| `32 <= days <= 60` | `total_asset` | `month_init_zczs` | 近 30 天完整数据（约 20 个交易日） |
| `days > 60` | `year_profit` | `year_init_zczs` | 年度或建仓以来数据（实测约 4 个月，89 个交易日） |

**CLI 显示字段说明**：

| CLI 列名 | 含义 | 计算方式 |
|---|---|---|
| 总资产 | 当日总资产 | API `asset` 字段 |
| 资产变化 | 相对前一日的资产变化 | `当日总资产 - 前一日总资产` |
| 当日盈亏 | 当日资产变化金额 | 等于资产变化（包含资金流入流出） |
| **持仓盈亏率** | **纯持仓涨跌幅（剔除转账影响）** | **`(当日市值 - 前日市值) / 前日市值 × 100%`** |
| 现金 | 账户可用现金 | API `asset - (asset × position_rate)` 推算 |
| 市值 | 持仓市值 | API `asset × position_rate` 推算 |
| 仓位 | 持仓占比 | 市值 / 总资产 × 100% |

**持仓盈亏率计算逻辑**：

```go
// 基于市值变化计算，剔除资金流入流出的影响
持仓盈亏率 = (当日市值 - 前一日市值) / 前一日市值 × 100%
```

**示例**：
```
7月16日市值：505521.25
7月17日市值：494237.52
持仓盈亏率 = (494237.52 - 505521.25) / 505521.25 × 100% = -2.23%
```

**注意**：
- `持仓盈亏率` 只反映**持仓本身的涨跌**，不包含转账、分红、买卖操作的影响
- `资产变化` 包含所有因素（持仓涨跌 + 资金流入流出）
- 当资产变化 ≠ 市值变化时，说明有资金流入流出操作

**CLI 实现**：

```go
// pkg/api/client.go
type AssetTrendPoint struct {
    Date         string  // 日期 YYYY-MM-DD
    TotalAsset   float64 // 总资产
    Profit       float64 // 盈亏（相对基准）
    ProfitRate   float64 // 收益率（小数）
    Cash         float64 // 现金
    MarketValue  float64 // 市值
    PositionRate float64 // 仓位（小数）
}

func (c *Client) GetAssetTrend(acc Account, days int) ([]AssetTrendPoint, error)
```

**注意事项**：

1. **`profit` 不是总盈亏**：
   - `month_profit` 里的 `profit` 是**月内增量**（相对 6月30日）
   - 要看总盈亏，需要用 `year_profit` 或账户级 `card.profit`
   
2. **日期格式转换**：
   - API 返回 `20260622`，需转换为 `2026-06-22`
   
3. **现金/市值推算**：
   - API 不直接返回现金和市值
   - 市值 ≈ 总资产（假设全仓）或 总资产 - 现金

### 3.8 插件全量接口清单（从 tzzb-plugin JS 扒取）

从 `~/Downloads/tzzb-plugin` 的 JS bundle 里扒到的全部 `/caishen_fund/...` 端点。所有路径需拼公共前缀 `/caishen_httpserver/tzzb`（见 §1.1）。

#### 已接入（21 个）

| path | 用途 | 对应命令 |
|---|---|---|
| `invest/getQuotes` | 行情报价 | 持仓实时重算 |
| `pc/account/v1/account_list` | 账户列表 | `account list` |
| `pc/account/v1/fund_card` | 基金账户卡 | `account` 富化 |
| `pc/account/v1/stock_card` | 股票账户卡 | `account` 富化 |
| `pc/account/v1/get_money_history` | 资金/交易流水 | `trade` |
| `pc/account/v1/merge_day_trading` | 当日成交 | 持仓实时重算 |
| `pc/asset/v1/asset_trend` | 资产趋势曲线 | `trend` |
| `pc/asset/v1/fund_position` | 基金持仓 | `position` |
| `pc/asset/v1/stock_position` | 股票持仓 | `position` |
| `pc/asset/v1/query_bank_history` | 银证转账 | `bank` |
| `stock/getAccounts` | 股票账户列表（含 zjzh） | 盈亏/日历集约参数 |
| `combineCalendar/dailyProfitloss` | 组合每日盈亏 | `calendar month` |
| `combineCalendar/monthlyProfitloss` | 组合月度盈亏 | `calendar year` |
| `stock/monthlyProfitLossMerge` | 月度股票盈亏（含收益率/大盘） | `profit month` |
| `stock/profitlossByCodeMerge` | 区间每日盈亏 + 个股盈亏（个股为**整月口径**） | `profit range` |
| `stockAnalysisSummary/position_distribution` | 持仓分布（按市值） | `analysis dist` |
| `stockAnalysisSummary/accountPositionRate` | 账户仓位率 | `analysis rate` |
| `stock_position/v1/merge_position_query` | 单只合并明细 | `detail` |
| `stock_position/v1/total_position_query` | 单只建仓记录 | `detail` |
| `stock_position/v1/stock_history_query` | 单只历史成交 | `detail` |
| `stock_position/v1/clear_position_query` | 单只清仓汇总 | `detail` |

#### 未接入 — 只读查询类（有做成命令的价值）

**股票盈亏分析**（`profit month`/`range` 已接入 Merge 变体，见 §3.11）
```
stock/monthlyProfitLossMerge         月度盈亏（合并）        ✅ profit month
stock/profitlossByCodeMerge          按代码盈亏（合并）      ✅ profit range
stock/accountProfitloss              账户维度盈亏
stock/accountProfitlossMerge         账户盈亏（合并）
stock/dailyStockProfitLoss           每日个股盈亏
stock/monthlyProfitLoss              月度盈亏（单账户，非合并）
stock/monthlyStockProfitLoss         月度个股盈亏
stock/profitlossByCode               按股票代码查盈亏（单账户）
stock/v1/yearly_profit_loss          年度盈亏（响应下划线命名，见 §3.11 备注）
stock/v1/yearly_stock_profit_loss    年度个股盈亏
stock/v1/distribution_merge          分布（合并）
stock/bs                             买卖点记录
stock/asset                          股票资产
stock/index                          指数
stock/getAccounts                    账户列表（股票侧）      ✅ 集约参数用
```

**股票分析汇总**（仓位/分布/统计）
```
stockAnalysisSummary/position_distribution   持仓分布        ✅ analysis dist
stockAnalysisSummary/accountPositionRate     账户仓位率      ✅ analysis rate
stockAnalysisSummary/analysis                综合分析
stockAnalysisSummary/summaryInfo             汇总信息（begin_time 建仓起始日）
stockAnalysisSummary/statistics              统计指标（需 command 命令码，未展开）
stockAnalysisSummary/dayAnalysis             当日分析
stockAnalysisSummary/v1/duration_rate        持有期收益率
stockAnalysisSummary/v1/select_analysis      选股分析
```

**单只股票钻取**（都要 stock_code + 账户坐标，见 §3.12）
```
stock_position/v1/merge_position_query       单只合并明细      ✅ detail
stock_position/v1/total_position_query       单只建仓记录      ✅ detail
stock_position/v1/stock_history_query        单只历史成交      ✅ detail
stock_position/v1/clear_position_query       单只清仓汇总      ✅ detail
```

**盈亏日历/月历视图**
```
combineCalendar/dailyProfitloss     组合每日盈亏
combineCalendar/monthlyProfitloss   组合月度盈亏
combineCalendar/accountProfitloss   组合账户盈亏
fundCalendar/dayProfit              基金每日收益
fundCalendar/monthView              基金月视图
fundCalendar/yearView               基金年视图
fundCalendar/fundidListH5           基金 ID 列表
```

**基金分析 & 穿透**（fund 侧几乎没做）
```
fundAnalysisSummary/analysis         基金分析
fundAnalysisSummary/summaryInfo      基金汇总
fundAnalysisSummary/position         基金持仓分析
fundAnalysisSummary/moneyChange      资金变动
fundAnalysisSummary/fundDistribution 基金分布
fundPenetrate/heavilyHeldShares      基金重仓穿透
fundPenetrate/reportThrough          穿透报告
fundPenetrate/box                    穿透箱体
fundPenetrate/dateList               穿透日期列表
fund_quota/v1/history_nav            历史净值
fund_quota/v1/nav_graph              净值曲线
fund_quota/v1/heavy_held_stock       重仓股
fund_quota/v1/position_detail        持仓明细
fund_quota/v1/stage_increase_rate    阶段涨幅
fund_quota/v1/bs_point               买卖点
fund_quota/v1/distribution           分布
fund_quota/v1/gz_graph               估值曲线
fund_quota/v1/trans_history          交易历史
fund_summary/position/v1/history_date_span   持仓日期跨度
fund_summary/position/v2/query_history_month 历史月度持仓
valuation/fundMarket                 基金市场估值
```

**行情/市场辅助**
```
stock/info/v1/detail                 个股详情
stock/market/v1/rate                 市场涨跌
stock/market/v1/ths_market           同花顺市场分类
stock_common/v1/hk_rate              港股汇率
stock_common/v1/hk_rate_list         港股汇率列表
stock_common/v1/last_trading_day     最近交易日
stock_common/v1/market_query         市场查询
stockSummary/judgeHkStock            判断港股
cloud/v1/stockdict/search            股票字典搜索
```

**自选（可做 watchlist 命令）**
```
optional/v3/get_stock_list           自选股列表
optional/v3/get_fund_list            自选基金列表
optional_stock/v2/group_position     自选分组持仓
optional_stock/v2/history_curve      自选历史曲线
optional_stock/v2/time_of_day        自选分时
optionalStock/getAllInfo             自选股全部信息
optionalFund/getAllInfo              自选基金全部信息
```

**交割单 / 组合 / 达人跟投**
```
delivery/v1/position_list            交割单持仓
delivery/w/v1/daily_history          交割每日历史
delivery/w/v1/history_filter         交割历史筛选
fundGroup/selectMyAndSubscribeFundGroup 我的+订阅组合
fundGroup/fundGroupInit              组合初始化
leaderPosition/bs                    达人买卖点
stock_radar/user/v1/history          雷达历史
stock_radar/user/v1/range_detail     雷达区间明细
community/personal_homepage/v1/view_homepage 个人主页
```

#### 未接入 — 增删改类（对查询型 CLI 无意义，仅记录）

```
账户:   pc/account/v1/{add,del,edit}_account, init, init_fund, status, status_fund, qs_list
资金:   pc/account/v1/{add,edit,delete}_money_history, edit_stock_position_history
资产:   pc/asset/v1/{add_bank_history, add_money_history, add_stock_position,
              clear_stock, delete_stock, fund_quota, merge_compare, time_share}
手动股: manualstock/{addAccount, addBankHistory, addMoneyHistory, addPositionHistory,
              deleteBankHistory, deleteMoneyHistory, deleteStockPosition, modifyRemain,
              updateBankHistory, updateMoneyHistory, ...}
手动基: manualFund/{addPositionList, bonusEdit, delPosition, editPosition, ...}
定投:   fixedPlan/{addPlan, deletePlan, editPlan, editPlanStatus, planList, fixedHistory}
自选:   optional{Stock,Fund}/{createGroup, deleteStock/Fund, editStock/Fund, importFund}
组合:   fundGroup/{creatGroup, deleteFundGroup, updateGroupPosition, importAccounts}
费用:   manual_stock/fee_config/v1/{edit, query}
快照:   manual_stock/position_snapshot/v1/{add, delete, edit, edit_before}
命令:   command/{decrypt, encrypt_new}
水印:   common/v1/{query_watermark, upload_watermark}, invest/watermark
其它:   common/v1/post_scan, common/fund_cal_{init,polling}, merge_config/v1/query_status
```

> **推荐优先做：** 股票盈亏分析（`stock/*ProfitLoss` 系列）、仓位分布（`stockAnalysisSummary/*`）、盈亏日历（`combineCalendar/*`）。三块都是纯查询、与现有 account/position/profit 同源，补齐后分析能力会完整很多。接入前需先抓真实请求参数与响应结构。

### 3.11 股票盈亏 / 仓位分析接口（已接入，抓包验证）

第一梯队接入的四个接口，与 combineCalendar 同一套「全账户合算」账户口径（`BuildCalendarParams`），但**参数字段名与日期格式两套 builder 有差异，且踩过两个坑**，此处记全。

对应命令：`profit month`、`profit range`、`analysis dist`、`analysis rate`。

#### 共用集约参数来源

三系接口（combineCalendar / stock*ProfitLoss / stockAnalysisSummary）都靠 `CalendarParams` 提供账户标识。其中 **stock 账户的 `zjzh`（资金账号）不在 account_list**，须另调 `stock/getAccounts` 补齐（见 [[calendar-profit-api]]）。`CalendarParams` 现含 `StockFundKey` 字段：与 `qsid`/`zjzh` 同序的 stock 账户 fund_key 连接串，梯队 A/B 都要用。

#### 梯队 A — `stock/monthlyProfitLossMerge` / `stock/profitlossByCodeMerge`

集约参数（`stockPLParams`）：
```
cus_stockType=all, types=<stock_types>, merge_type=<stock_types>,
zjzh, qsid, fund_key=<stock 账户 fund_key>, rzrq_fund_key=, manual_id,
from_pc=1, terminal=123, version=11,
sz_cal=1, jy_cal=1, cw_cal=0        # 插件常量 G：市值/交易计算开，持仓关
```
业务参数：monthly 加 `year=YYYY`；range 加 `startdate`/`enddate`。

**坑 1（日期格式）**：`startdate`/`enddate` 用 **`YYYYMMDD` 无横杠**。带横杠（`2026-07-01`）会返回 `error_code=-2 服务器繁忙` —— 与 combineCalendar 用带横杠恰好相反，别混用。

**坑 2（fund_key 口径）**：`fund_key` 是 **stock 账户 fund_key**（与 qsid/zjzh 对齐），不是基金 fundid。实测服务端主要靠 `zjzh+qsid` 匹配，fund_key 填基金 fundid 也能返回相同结果，但按插件口径带 stock fund_key 更稳妥。

**坑 3（个股盈亏是整月口径）**：`profitlossCode`（个股盈亏）**不随 `startdate`/`enddate` 收窄**，只有 **`startdate` 为月初 1 号**时才返回，且内容是该月**整月**数据（忽略 `enddate`）；
- `20260801~20260810`（合计 +10434.97）与 `20260801~20260831`（合计 +3333.19）打印出的个股表**完全相同**；
- `20260815~20260831`、`20260907~20260911` 等**非月初起点返回空 codes**。
- 官方插件同样按整月调用（`startdate = new Date(y, m, 1)`，`enddate = 月末/今天`），并把 `profitlossCode` 缓存挂到**月** key 上。
所以 CLI 的表标题/flag 文案是「个股盈亏（整月口径）」，`profit range` 在起始日非月初时不渲染该表并给出提示。

响应 `ex_data`：`totalProfit` / `totalProfitRate`（收益率，小数）/ `totalSz`（同期大盘涨跌，小数）+ `profitlossList[{date, profitloss, profitloss_rate, sz}]` + `profitlossCode[{code,name,profitloss,type}]`。

**已验证：`totalProfit`/`totalProfitRate`/`totalSz` 均随 startdate/enddate 收窄而变化**（三个区间实测各不同），且 `totalProfit` 与 profitlossList 逐日累加精确相等。区间合计直接用接口的这三个字段即可，无需手工累加。（注意 `profitlossCode` 例外，见坑 3。）

#### 梯队 B — `stockAnalysisSummary/{position_distribution,accountPositionRate}`

集约参数（`analysisParams`，字段名与梯队 A 不同）：
```
zjzh, qsid, fundkey=<stock 账户 fund_key>, rzrq_fund_key=, manual_id, from_pc=1
```
注意是 **`fundkey`（无下划线）**，且值必须是 **stock 账户 fund_key**；填成基金 fundid 会返回**空数组**（这是最初 position_list=[] 的原因）。业务参数加 `start_date`/`end_date`（YYYYMMDD）；accountPositionRate 另加 `type=0`（全部口径，对齐插件 analysis 的 `total` 枚举）。

响应：
- position_distribution → `total_value` + `position_list[{stock_code, stock_name, stock_value, market}]`。`total_value` 偶为 0，占比分母用明细累加兜底。
- accountPositionRate → `account_list[{name, qsmc, stock_total, money_remain, zjzh, ...}]`，仓位率 = stock_total / (stock_total + money_remain)。

### 3.12 单只股票钻取明细（已接入，抓包验证）

对应命令 `detail <代码>`。源自插件 `tzzbWeb/handstock/itemDetail.*.bundle.js` 的四个接口，全都是「钻取**单只**股票」，不是列全部持仓。命令一次聚合四块：合并明细 / 建仓记录 / 历史成交 / 清仓汇总。

#### 账户坐标（关键，与前面三系不同）

这四个接口**不走** `BuildCalendarParams` 的全账户合算口径，而是**单账户 + 单代码**定位。共用参数：
```
manualid, fundkey, rzrq_fundkey, stock_code, stock_account, from_pc=1
```
- 字段名**去掉下划线**：`fundkey`/`rzrq_fundkey`/`manualid`（对应 account_list 的 `fund_key`/`rzrq_fund_key`/`manual_id`）。
- 账户坐标只需三者之一（普通账户填 `fundkey` 即可），**`stock_account` 实测可留空**——插件多数场景也传空串，服务端靠 `fundkey + stock_code` 匹配。
- CLI 的做法：先 `GetAccountList` 扫描各股票账户持仓行，找出持有该 `stock_code` 的账户，取其 `fund_key`/`rzrq_fund_key`/`manual_id` 作坐标；多账户命中就逐个钻取。fund 账户不参与（按 stock_code 匹配无意义）。

#### 四个接口

| path | 语义 | 额外参数 | 响应要点 |
|---|---|---|---|
| `stock_position/v1/merge_position_query` | 单只合并明细 | — | 顶层对象：`hold_days`/`price_buy_av`/`price_curr`/`market_value`/`zcb`(总成本)/`yke`(浮盈)/`profit_rate`(小数)/`fee`/`come_profit`/`clear_flag`(0/1) |
| `stock_position/v1/total_position_query` | 建仓记录 | — | `list[{start_date(建仓日 YYYYMMDD), end_date(持有中为空), profit, profit_rate}]` |
| `stock_position/v1/stock_history_query` | 逐笔历史成交 | `start_date`/`end_date` | `list[{trans_date, transDateTime, op(1买/2卖), trans_price, trans_count, trans_amount, trans_fee, profit, account_name}]` |
| `stock_position/v1/clear_position_query` | 清仓汇总 | `start_date`/`end_date` | 顶层：`total_buy`/`total_sell`/`total_profit`/`total_profit_rate`/`hold_days`/`fee`。**未清仓时各金额为 0**（CLI 用 `HasData()` 判断，为空则跳过不渲染） |

**日期格式**：history/clear 的 `start_date`/`end_date` 用 **`YYYYMMDD` 无横杠**（与梯队 A 一致，与 combineCalendar 相反）。插件里由 `String(jcrq).replace(/-/g,'')` 得到。CLI 默认区间 = 建仓记录最早 `start_date` → 今天，可用 `--from/--to` 覆盖，都拿不到则回落近一年。

**验证**：实测 `merge` 的 `market_value` = `count × price_curr`（5000×0.941=4705），`trans_amount` 与 `trans_count×trans_price` 一致；未清仓股 `clear_position_query` 返回全 0，`total_position_query` 给出建仓日。

### 3.9 全量原始清单（附录，共 227 条，一条不落）

§3.8 是分类精选，此处为从插件 JS 扒到的全部 `/caishen_fund/...` 端点原样罗列（去掉 `caishen_fund/` 前缀，按一级模块字母序分组），保证完整无遗漏。调用前均需拼公共前缀 `/caishen_httpserver/tzzb`（见 §1.1）。

**cloud**
```
cloud/get_user_customConfig
cloud/v1/stockdict/search
```

**combineCalendar**
```
combineCalendar/accountProfitloss
combineCalendar/dailyProfitloss
combineCalendar/monthlyProfitloss
```

**command**
```
command/decrypt
command/encrypt_new
```

**common**
```
common/fund_cal_init
common/fund_cal_polling
common/v1/post_scan
common/v1/query_watermark
common/v1/upload_watermark
```

**community_match**
```
community_match/v1/hold_stock_uid
community_match/v1/hot_ranking_bs
community_match/v1/query_upload_time
community_match/v2/daily_history
community_match/v2/my_match_simple
```

**community_share**
```
community_share/v1/change_bs_by_share
community_share/v1/daily_history_by_share
community_share/v1/uploadtime_by_share
```

**community**
```
community/content_provide/v1/get_optional_top_twenty
community/personal_homepage/v1/view_homepage
```

**deletefund**
```
deletefund
```

**delivery_match**
```
delivery_match/v1/query_upload_time
delivery_match/v1/query_upload_time_by_share
```

**delivery**
```
delivery/v1/position_list
delivery/v1/update_position
delivery/w/v1/daily_history
delivery/w/v1/daily_historyByShare
delivery/w/v1/history_filter
delivery/w/v1/history_filterByShare
```

**fixedPlan**
```
fixedPlan/addPlan
fixedPlan/deletePlan
fixedPlan/editPlan
fixedPlan/editPlanStatus
fixedPlan/fixedHistory
fixedPlan/planList
fixedPlan/suspendFundAllPlan
```

**forward**
```
forward/v1/wencai_spec
```

**fund_quota**
```
fund_quota/v1/bs_point
fund_quota/v1/distribution
fund_quota/v1/gz_graph
fund_quota/v1/heavy_held_stock
fund_quota/v1/history_nav
fund_quota/v1/nav_graph
fund_quota/v1/position_detail
fund_quota/v1/stage_increase_rate
fund_quota/v1/trans_history
```

**fund_summary**
```
fund_summary/position/v1/history_date_span
fund_summary/position/v2/query_history_month
```

**fundAnalysisSummary**
```
fundAnalysisSummary/analysis
fundAnalysisSummary/fundDistribution
fundAnalysisSummary/moneyChange
fundAnalysisSummary/position
fundAnalysisSummary/summaryInfo
```

**fundCalendar**
```
fundCalendar/dayProfit
fundCalendar/fundidListH5
fundCalendar/monthView
fundCalendar/yearView
```

**fundGroup**
```
fundGroup/beforeChangePosition
fundGroup/creatGroup
fundGroup/deleteFundGroup
fundGroup/fundGroupInit
fundGroup/fundGroupPolling
fundGroup/importAccounts
fundGroup/selectMyAndSubscribeFundGroup
fundGroup/updateGroupPosition
```

**fundPenetrate**
```
fundPenetrate/box
fundPenetrate/dateList
fundPenetrate/heavilyHeldShares
fundPenetrate/reportThrough
```

**grid**
```
grid/v1/get_ths_market_by_code
```

**imitateProfit**
```
imitateProfit/stockCodePrice
```

**index**
```
index/v1/select_upgrade
index/v1/stage
```

**interaction_match**
```
interaction_match/v1/change_bs
```

**invest**
```
invest/getQuotes
invest/watermark
```

**leaderPosition**
```
leaderPosition/bs
```

**manage**
```
manage/stock
```

**manual（根）**
```
manual
```

**manual_fund**
```
manual_fund/history/v1/query_net_by_confirm
manual_fund/history/v1/query_net_by_op
```

**manual_stock**
```
manual_stock/fee_config/v1/edit
manual_stock/fee_config/v1/query
manual_stock/position_snapshot/v1/add
manual_stock/position_snapshot/v1/delete
manual_stock/position_snapshot/v1/edit
manual_stock/position_snapshot/v1/edit_before
manual_stock/suspension/v1/query
manual_stock/suspension/v1/query_code
```

**manual/**
```
manual/add_history
manual/add_historylist
manual/deleteHistory
manual/deleteTransferHistory
manual/edit_history
manual/fast_position
manual/fundTransfer
manual/get_fundfee
manual/getnewfundnavh5
manual/select_history_detail
manual/selectTransferHistory
```

**manualFund**
```
manualFund/addPositionList
manualFund/addpositionRetype
manualFund/bonusEdit
manualFund/bonusPosTypeRetype
manualFund/delPosition
manualFund/editPosition
manualFund/latestNav
manualFund/positionDetail
```

**manualstock**
```
manualstock
manualstock/addAccount
manualstock/addBankHistory
manualstock/addMoneyHistory
manualstock/addPositionHistory
manualstock/calcByCalcModule
manualstock/deleteBankHistory
manualstock/deleteMoneyHistory
manualstock/deleteStockPosition
manualstock/getDateAndNum
manualstock/getTodayTradeHistory
manualstock/modifyRemain
manualstock/positions
manualstock/queryBankHistory
manualstock/queryMoneyHistory
manualstock/updateBankHistory
manualstock/updateMoneyHistory
manualstock/v1/historyDateSpan
manualstock/v2/queryMoneyHistory
```

**merge_config**
```
merge_config/v1/query_status
```

**optional_stock**
```
optional_stock/v2/group_position
optional_stock/v2/history_curve
optional_stock/v2/time_of_day
```

**optional**
```
optional/v3/get_fund_list
optional/v3/get_stock_list
```

**optionalFund**
```
optionalFund/createGroup
optionalFund/deleteFund
optionalFund/editFund
optionalFund/fundInGroup
optionalFund/getAllInfo
optionalFund/importFund
```

**optionalStock**
```
optionalStock/createGroup
optionalStock/deleteStock
optionalStock/editStock
optionalStock/getAllInfo
optionalStock/ocrOptionalStock
optionalStock/stockInGroup
```

**pc**
```
pc/account/v1/account_list
pc/account/v1/add_account
pc/account/v1/del_account
pc/account/v1/delete_money_history
pc/account/v1/edit_account
pc/account/v1/edit_money_history
pc/account/v1/edit_stock_position_history
pc/account/v1/fund_card
pc/account/v1/get_money_history
pc/account/v1/init
pc/account/v1/init_fund
pc/account/v1/merge_day_trading
pc/account/v1/qs_list
pc/account/v1/status
pc/account/v1/status_fund
pc/account/v1/stock_card
pc/asset/v1/add_bank_history
pc/asset/v1/add_money_history
pc/asset/v1/add_stock_position
pc/asset/v1/asset_trend
pc/asset/v1/clear_stock
pc/asset/v1/delete_stock
pc/asset/v1/fund_position
pc/asset/v1/fund_quota
pc/asset/v1/merge_compare
pc/asset/v1/query_bank_history
pc/asset/v1/stock_position
pc/asset/v1/time_share
pc/stock/v1/query_suspension_code
```

**report**
```
report/stock/distribution
```

**stock_common**
```
stock_common/v1/hk_rate
stock_common/v1/hk_rate_list
stock_common/v1/last_trading_day
stock_common/v1/market_query
stock_common/v1/market_query_by_share
```

**stock_group**
```
stock_group/v1/check_delete_stock_group
stock_group/v1/subscribe
stock_group/v1/subscribe_info
```

**stock_position**
```
stock_position/v1/clear_position_query
stock_position/v1/merge_position_query
stock_position/v1/stock_history_query
stock_position/v1/total_position_query
```

**stock_radar**
```
stock_radar/user/v1/bs_share
stock_radar/user/v1/collect_leader
stock_radar/user/v1/collect_leader_share
stock_radar/user/v1/daily_history
stock_radar/user/v1/daily_history_share
stock_radar/user/v1/exist_history
stock_radar/user/v1/history
stock_radar/user/v1/history_share
stock_radar/user/v1/range_detail
stock_radar/user/v1/share_conf
```

**stock**
```
stock/accountProfitloss
stock/accountProfitlossMerge
stock/asset
stock/bs
stock/dailyStockProfitLoss
stock/getAccounts
stock/index
stock/info/v1/detail
stock/market/v1/rate
stock/market/v1/ths_market
stock/monthlyProfitLoss
stock/monthlyProfitLossMerge
stock/monthlyStockProfitLoss
stock/profitlossByCode
stock/profitlossByCodeMerge
stock/v1/distribution_merge
stock/v1/yearly_profit_loss
stock/v1/yearly_stock_profit_loss
```

**stockAnalysisSummary**
```
stockAnalysisSummary/accountPositionRate
stockAnalysisSummary/analysis
stockAnalysisSummary/dayAnalysis
stockAnalysisSummary/position_distribution
stockAnalysisSummary/statistics
stockAnalysisSummary/summaryInfo
stockAnalysisSummary/v1/duration_rate
stockAnalysisSummary/v1/select_analysis
```

**stockSummary**
```
stockSummary/judgeHkStock
```

**valuation**
```
valuation/fundMarket
```

---

### 3.10 怎么从插件扒接口参数（操作手册）

下次要接入某个未验证接口时，按这套流程从插件 JS 里扒它的请求参数。**只能扒出字段名和大致构造逻辑，业务含义/取值靠字段名+上下文推断，最终仍需真实抓包验证。**

#### 前置：插件在哪

- 解包后的插件目录：`~/Downloads/tzzb-plugin`
- 业务代码在 **`tzzbWeb/*/`** 下的 `*.bundle.js`（不是 `js/pcIndex/`，那是 PC 壳）。已知几个大 bundle：
  - `tzzbWeb/stockzcfx/index.*.bundle.js` — 股票资产分析（盈亏、分析汇总、持仓）
  - `tzzbWeb/hzrl/calendarPage_sum.*.bundle.js` — 汇总日历（combineCalendar / fundCalendar / stock 盈亏）
  - `tzzbWeb/common/*.bundle.js` — 公共
- 关键：**这些 bundle 是已格式化的（多行、缩进），不是单行压缩**，所以能顺着读。

#### 第 1 步：定位路径常量 → 短变量名

bundle 里 API 路径被集中赋给单字母变量，形如：

```js
d = '/caishen_fund/stock/accountProfitloss',
p = '/caishen_fund/stock/monthlyProfitLoss',
```

先 grep 出接口名，拿到它绑定的变量名（这里 `accountProfitloss` → `d`）：

```bash
cd ~/Downloads/tzzb-plugin
grep -rn "accountProfitloss" tzzbWeb --include="*.js" | grep "caishen_fund"
```

> 注意：同一接口可能在多个 bundle 里各自绑不同变量名，按文件分别看。

#### 第 2 步：找请求封装 → 定位 `param`

插件统一用一个包装器发请求，签名是：

```js
(0, o.e4)({ hostCode: '2000', url: <路径变量>, param: <参数对象> })
```

（`e4` 常被 import 成 `o.e4` / `r.e4`。）grep 这个变量作为 `url:` 出现的调用点：

```bash
# 假设第 1 步拿到的变量是 d，且知道它在 calendarPage_sum 里
grep -n "url: d\b\|url:d\b" tzzbWeb/hzrl/calendarPage_sum.*.bundle.js
# 或直接搜包装器，再在附近对照 url 变量
grep -n "e4)(" tzzbWeb/hzrl/calendarPage_sum.*.bundle.js
```

`param:` 后面那个变量（常见 `l` / `g`）就是要扒的参数对象。

#### 第 3 步：往上读 `param` 的构造

在调用点上方读参数对象怎么拼出来的。实测能看到明文字段名，例如：

```js
l = { from_pc: '1' };
// switch 分支按类型补字段：
l.startdate = n.yk_startDate; l.enddate = n.yk_endDate;   // 区间
l.year = n.yk_year;                                        // 年度
// 手动账户分支：
l.manual_id = t.manual_id; l.flag = '3'; l.cus_stockType = 'manual';
```

日历类还常见 `{ startdate, enddate, date, retype, sz_cal, cw_cal, jy_cal }` 等。

#### 第 4 步：展开辅助函数

参数常由 `c(c({...}, y), a)` 这种合并出来（`c` 是对象 merge），其中 `y = h(n)`、账户前缀 `m(n)` 之类的辅助函数补充公共字段（如 `manual_id` / `cus_stockType` / 账户 id）。把 `h`、`m` 的定义也 grep 出来读一遍，才能拿到完整字段集：

```bash
grep -n "function h(\|h = function\|function m(" tzzbWeb/hzrl/calendarPage_sum.*.bundle.js
```

#### 第 5 步：合并公共参数

上面扒到的是**业务字段**。真实请求还要叠加 §1.4 的公共参数（`uid` / `source` / 空字段占位等）和 §1.2 的 4 个必要请求头，才能过 403。接入时以 `pkg/api/client.go` 的 `post()` 为准。

#### 第 6 步：验证（不可跳过）

JS 只能给字段名和拼装逻辑，**取值语义（枚举值含义、日期格式、必填性）必须真实抓一次**：浏览器登录 → F12 Network → 触发该功能 → 看 Payload 和 Response，和扒出来的字段对齐后再写进本文档的"已验证接口"。

#### 一句话流程

```
grep 接口名 → 拿路径变量 → grep url:变量 找 e4 调用点 →
上翻读 param 构造 → 展开 h()/m() 辅助函数 → 叠加 §1.4 公共参数/§1.2 头 → 真实抓包验证
```

---

## 4. CLI 实现原理

### 4.1 目录结构

```
main.go                 入口，调 cmd.Execute()
go.mod
pkg/api/client.go       API 客户端（认证、请求、解析、数据模型）——核心
cmd/root.go             根命令，注册子命令，初始化 apiClient
cmd/login.go            login：默认自动从浏览器读 Cookie，也支持 --cookie/--file/管道
cmd/account.go          account list：账户列表 + 汇总，含 colorNum 工具
cmd/position.go         position list：持仓明细，含 colorPct 工具
cmd/trade.go            trade list / trade summary；解析见 §3.6.1
cmd/profit.go           profit summary：收益汇总
cmd/export.go           export account/position：CSV/JSON 导出
docs/API.md             本文档
```

用 cobra 搭命令，fatih/color 上色，olekukonko/tablewriter 出表格。

**trade 记录怎么解析**：见 **§3.6.1**（`call` → `parseMoneyHistoryPage` → 翻页拼接 → `renderRecords` / `tradeAgg.add`）。

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

### 4.5 持仓展示：实时重算（对齐插件，同源）

插件链路（**不要用新浪**）：

1. `stock_position` 持仓数量/成本/接口价/接口 hold_profit  
2. **`/caishen_fund/invest/getQuotes`**（插件 `requestPriceInfo`，参数 `code=market:code,...`）拿 `xianjia`/`zuoshou`  
3. **`/caishen_fund/pc/account/v1/merge_day_trading`** 当日成交（`czdm` 1买/2卖/0速记/235转入/236转出）  
4. 本地 `RecomputeLive`（`pkg/api/client.go`）按插件 `stockCalc` 重算  

| 字段 | 公式 |
|---|---|
| 现价 | getQuotes.xianjia；无行情回落接口 `price` |
| 市值 | `数量 × 现价` |
| 今日盈亏 | 当日买卖收益 + `(现价−昨收)×(持仓−今日买入−正 remainchg) − 费用 + 速记`（插件 dryk） |
| 今日涨幅 | `(现价 − 昨收) / 昨收`（插件 **涨幅 zf**；不是当日盈亏比） |
| 单票当日盈亏比 | `今日盈亏 / (市值 − 今日盈亏 + 卖出金额)`（插件 drykb） |
| **合计今日涨幅** | `总今日盈亏 / (总市值 − 总今日盈亏 + 总卖出额)`（持仓口径，与单票 drykb 一致；**不是**账本/card 的总资产 `now_rate`） |
| 持仓盈亏 | `接口 hold_profit + (现价 − 接口价) × 数量`（auto 账户） |
| 收益率 | `持仓盈亏 / (成本价 × 数量)`（插件 浮动盈亏比） |

`account list` 账户级数字仍走 card（`now_profit`/`profit`/`asset`）。

---

## 5. 命令一览

```bash
# 登录（默认自动从本机浏览器读取登录态，无需手工复制）
tzzb login                           # 自动读取，推荐
tzzb login --browser chrome          # 只读指定浏览器
tzzb login --file cookie.txt         # 手工：从文件读
tzzb login --cookie "userid=...; ..."# 手工：直接参数传
pbpaste | tzzb login                 # 手工：从剪贴板管道输入

# 查看
tzzb account list                    # 账户列表 + 汇总
tzzb position list                   # 全部持仓
tzzb position list -a 华宝           # 按账户名模糊过滤
tzzb profit summary                  # 收益汇总（账户级快照）
tzzb profit month                    # 月度盈亏（某年各月，含账户收益率/同期大盘）
tzzb profit month -y 2026 -a 国      # 指定年份 + 账户过滤
tzzb profit range                    # 区间每日盈亏 + 区间个股盈亏（默认本月初到今天）
tzzb profit range -s 20260701 -e 20260710  # 指定区间（日期 YYYYMMDD 无横杠）
tzzb analysis dist                   # 持仓分布（按市值占比）
tzzb analysis rate                   # 各股票账户仓位率
tzzb detail 159227                   # 单只股票钻取明细（合并/建仓/成交/清仓）
tzzb detail 159227 -i                # 交互式 TUI（Tab 切换四块、多账户切换、成交表可滚动）
tzzb detail 600519 --from 20260101   # 指定历史成交/清仓查询区间
tzzb bank list                       # 银证转账明细
tzzb calendar month                  # 盈亏月历（某月每日）
tzzb calendar year                   # 盈亏年视图（某年每月）
tzzb trend show                      # 资产趋势（默认 30 天，最近 10 条）
tzzb trend show -a 国 -d 7           # 过滤账户，查看 7 天趋势
tzzb trend show -d 365 -l 0          # 查看全部历史数据（约 4 个月，89 个交易日）
tzzb trend show -d 365 -l 30         # 显示最近 30 条数据点

# 交易流水
tzzb trade list                      # 交易明细（近 3 个月）
tzzb trade summary                   # 交易汇总（按账户、按代码）

# 导出（CSV 带 UTF-8 BOM，Excel 打开中文不乱码）
tzzb export account  -f csv  -o 账户.csv
tzzb export position -f json -o 持仓.json

# Makefile 快捷命令
make account                         # = tzzb account list
make position                        # = tzzb position list
make profit                          # = tzzb profit summary
make trend                           # = tzzb trend show
make install                         # 安装到 ~/go/bin/tzzb
```

> 自动读取的实现见 `internal/browsercookie/`：钥匙串取 "Chrome Safe Storage" 口令
> → PBKDF2-SHA1(saltysalt, 1003 轮) → AES-128-CBC 解密 cookie 值。只支持 Chromium
> 系浏览器；非 macOS 平台是桩实现，会明确报错。
>
> 手工 `login` 卡住的根因：macOS 终端规范模式下单行输入约 1024 字节上限，Cookie 远超此值。
> 用管道/文件绕开，不要走交互式输入。

---

## 6. 给新 agent 的开发提示

- **加新接口**：在 `client.go` 加 `epXxx` 常量 → 写 `GetXxx()` 调 `c.call(epXxx, params)` → 定义对应 struct（数值字段一律用 `Num`）→ 在 `cmd/` 加子命令并 `AddCommand`。
- **不确定响应结构时先抓真实数据**，别凭字段名猜（本项目踩过 `total_asset` vs `total_assets`、`fundId` vs `fundid` 两个大小写/单复数坑）。抓包方式：带上 §1.2 的 4 个条件用 curl/脚本直接打接口，dump `ex_data` 的 keys。
- **验证要用真实 Cookie 跑**，编译通过 ≠ 功能对。第一版数值全 0 就是编译通过但字段映射全错。
- **改汇总口径**去 `mergeDetail` 看 §4.3，注意重复行问题（§4.4）。
- 参考 Python 实现 `~/github/stock/src/stock_assistant/integrations/tzzb.py`，那里有更完整的账户记录挖掘逻辑（`find_tzzb_account_records`、`walk_dicts`）。
