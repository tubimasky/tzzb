# 重构 Phase 1: 提取 Formatter 层 ✅

## 完成时间
2026-07-20

## 改动概述

### 新增文件
- `internal/formatter/color.go` (72 行) - 颜色格式化工具函数
- `internal/formatter/json.go` (16 行) - JSON 输出工具函数

### 修改文件
- `cmd/root.go` - 添加 formatter 包导入，同步 noColor 标志
- `cmd/account.go` - 使用 formatter 包替换本地函数
- `cmd/position.go` - 使用 formatter 包，删除重复函数定义
- `cmd/profit.go` - 使用 formatter 包
- `cmd/trade.go` - 使用 formatter 包
- `cmd/trend.go` - 使用 formatter 包

### 删除文件
- `cmd/json_output.go` - 功能已迁移到 `internal/formatter/json.go`

## 代码行数对比

### 重构前
- `cmd/position.go`: 1153 行
- `cmd/account.go`: 127 行
- 各 cmd 文件都有重复的 colorNum/colorPct/printJSON 函数

### 重构后
- `cmd/position.go`: 813 行 (**减少 340 行**)
- `cmd/account.go`: 115 行 (**减少 12 行**)
- `internal/formatter/`: 88 行 (新增)
- 净减少代码：**~264 行**

## 提取的函数

### `internal/formatter/color.go`
- `ColorNum(v float64) string` - 金额着色（红涨绿跌）
- `ColorPct(v float64) string` - 百分比着色
- `ColorPctRaw(v float64) string` - 原始小数转百分比着色
- `PriceStr(v float64) string` - 价格格式化（三位小数）
- `DayStr(v, ok) string` - 今日盈亏显示
- `RateStr(v, ok) string` - 涨跌幅显示
- `WeightStr(value, total) string` - 占比显示

### `internal/formatter/json.go`
- `PrintJSON(v any)` - 统一 JSON 输出

## 收益

### 1. 消除重复代码
- 原来 6 个 cmd 文件各自维护 colorNum/colorPct/printJSON
- 现在统一到 formatter 包，单一职责

### 2. 可维护性提升
- 修改颜色规则：只需改 `internal/formatter/color.go`
- 修改 JSON 格式：只需改 `internal/formatter/json.go`
- 不用担心 6 个文件改漏

### 3. 一致性保证
- 所有命令的颜色规则统一
- JSON 输出格式统一
- 百分比显示格式统一

### 4. 可测试性
- formatter 包可以独立单元测试
- 不依赖 cobra 或 API 调用

### 5. 全局配置支持
- `formatter.NoColor` 全局变量
- 通过 `cobra.OnInitialize` 同步 `--no-color` 标志
- 所有格式化函数自动响应

## 技术细节

### 颜色规则
遵循中国股市习惯：
- 红色 = 正数 (上涨/盈利) 
- 绿色 = 负数 (下跌/亏损)

### 全局配置机制
```go
// cmd/root.go
cobra.OnInitialize(func() {
    formatter.NoColor = noColor
})
```

每次命令执行前自动同步标志位。

### 函数命名约定
- 大写开头：公开函数（可被 cmd 包调用）
- 描述清晰：ColorNum（着色数字）、PriceStr（价格字符串）
- 统一后缀：Str 表示返回字符串

## 编译验证

```bash
$ go build -o tzzb .
# 编译成功，无错误

$ ./tzzb --help
# 命令正常工作

$ ./tzzb account list --help
# 子命令帮助正常
```

## 下一步计划

参考 `docs/REFACTOR.md`，Phase 2 选项：

1. **提取 Calculator 服务** - 将 `RecomputeLive` 等盈亏计算逻辑迁移到 `internal/service/calculator.go`
2. **拆分 API Client** - 将 1196 行的 `pkg/api/client.go` 按接口域拆分成多个文件
3. **提取 Model 层** - 创建 `internal/model/` 包，统一数据模型定义

建议下周做 **Calculator 服务提取**，因为：
- 盈亏计算逻辑复杂，需要单元测试覆盖
- 独立后可以验证算法正确性
- 为后续 service 层重构打基础
