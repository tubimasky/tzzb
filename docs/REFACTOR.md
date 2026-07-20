# 代码架构重构建议

## 当前问题总结

### 1. 单文件过大
- `cmd/position.go`: 1153 行
- `pkg/api/client.go`: 1196 行

### 2. 职责不清
- cmd 层混杂了业务逻辑、数据转换、格式化输出
- api 层既有 HTTP 客户端，又有业务计算（RecomputeLive）

### 3. 代码重复
- JSON 输出函数各自为政
- 颜色工具函数到处拷贝
- 表格渲染逻辑未统一

### 4. 缺少分层
现在只有两层：cmd → api
应该有四层：cmd → service → repository → model

## 建议的目录结构

```
tzzb/
├── cmd/                    # CLI 命令层（薄层，只负责参数解析和调用 service）
│   ├── root.go
│   ├── account.go         # 精简到 50 行以内
│   ├── position.go        # 精简到 100 行以内
│   ├── trade.go
│   ├── profit.go
│   └── ...
│
├── internal/              # 内部包，不对外暴露
│   ├── model/            # 数据模型（纯数据结构）
│   │   ├── account.go    # Account, Position 等
│   │   ├── quote.go      # Quote, QuoteRef
│   │   └── trade.go      # DayTrade, TradeRecord
│   │
│   ├── service/          # 业务逻辑层
│   │   ├── account.go    # 账户相关业务：获取列表、汇总
│   │   ├── position.go   # 持仓相关：合并、排序、计算
│   │   ├── calculator.go # 盈亏计算：RecomputeLive 等
│   │   ├── trade.go      # 交易流水相关
│   │   └── profit.go     # 收益统计相关
│   │
│   ├── repository/       # 数据访问层（原 api 包）
│   │   └── tzzb/
│   │       ├── client.go # HTTP 客户端基础
│   │       ├── account.go # 账户接口
│   │       ├── position.go # 持仓接口
│   │       ├── quote.go   # 行情接口
│   │       └── trade.go   # 流水接口
│   │
│   ├── formatter/        # 格式化输出层
│   │   ├── table.go      # 表格输出通用逻辑
│   │   ├── json.go       # JSON 输出通用逻辑
│   │   ├── color.go      # 颜色工具函数
│   │   └── template.go   # 输出模板
│   │
│   └── util/             # 工具函数
│       ├── number.go     # 数值处理
│       ├── code.go       # 代码归一化
│       └── cookie.go     # Cookie 处理
│
├── pkg/                  # 公开的 SDK（如果需要）
│   └── tzzb/
│       └── client.go     # 简化的公开 API
│
├── main.go
├── go.mod
└── README.md
```

## 重构步骤（渐进式）

### Phase 1: 提取 Model 层（最小破坏）
1. 创建 `internal/model/` 目录
2. 将 `api.Account`, `api.Position` 等迁移到 `model` 包
3. 保留 `pkg/api` 作为兼容层，内部引用 `model`

### Phase 2: 提取 Formatter 层
1. 创建 `internal/formatter/` 目录
2. 统一 `colorNum`, `colorPct`, `printJSON` 等函数
3. 抽象表格渲染逻辑：`RenderTable(headers, rows, options)`

### Phase 3: 提取 Service 层
1. 创建 `internal/service/calculator.go`，迁移 `RecomputeLive`
2. 创建 `internal/service/position.go`，迁移 `mergeByCode`, `sortTotalViews`
3. cmd 层改为调用 service 方法

### Phase 4: 重构 Repository 层
1. 将 `pkg/api/client.go` 按接口拆分成多个文件
2. HTTP 客户端基础逻辑抽离到 `client.go`
3. 每个业务域独立文件：`account.go`, `position.go`, `quote.go`

### Phase 5: 精简 CMD 层
1. cmd 文件只保留：参数定义、参数验证、调用 service、调用 formatter
2. 目标：每个 cmd 文件 < 150 行

## 重构收益

### 1. 可测试性
- service 层可以独立单元测试，不依赖 HTTP
- calculator 的盈亏逻辑可以用测试用例覆盖

### 2. 可维护性
- 修改表格样式只需改 formatter/table.go
- 修改盈亏算法只需改 service/calculator.go
- 文件小了，改起来心智负担低

### 3. 可扩展性
- 新增命令时只需关注 cmd 层
- 新增数据源时只需实现 repository 接口
- 新增输出格式（如 CSV）只需扩展 formatter

### 4. 代码复用
- colorNum 等工具函数统一管理
- JSON 输出逻辑可以模板化
- 表格渲染可以配置化

## 示例：重构后的 position.go (cmd)

```go
// cmd/position.go - 重构后只有 ~80 行
package cmd

import (
    "github.com/spf13/cobra"
    "github.com/liyanran/tzzb-cli/internal/service"
    "github.com/liyanran/tzzb-cli/internal/formatter"
)

var positionCmd = &cobra.Command{
    Use:   "position",
    Short: "持仓管理",
}

var positionListCmd = &cobra.Command{
    Use:   "list",
    Short: "按账户查看持仓明细",
    Run: func(cmd *cobra.Command, args []string) {
        // 1. 获取数据（通过 service）
        result, err := positionService.GetPositionList(filterAccount, filterType, flatList)
        if err != nil {
            handleError(err)
            return
        }

        // 2. 格式化输出（通过 formatter）
        if outputJSON {
            formatter.PrintJSON(result)
        } else if flatList {
            formatter.PrintPositionFlat(result)
        } else {
            formatter.PrintPositionByAccount(result)
        }
    },
}

func init() {
    positionCmd.AddCommand(positionListCmd)
    positionListCmd.Flags().StringVarP(&filterAccount, "account", "a", "", "按账户名过滤")
    positionListCmd.Flags().StringVarP(&filterType, "type", "t", "all", "类型: all/stock/fund")
    positionListCmd.Flags().BoolVar(&flatList, "flat", false, "扁平大表")
}
```

## 示例：新的 service 层

```go
// internal/service/position.go
package service

import (
    "github.com/liyanran/tzzb-cli/internal/model"
    "github.com/liyanran/tzzb-cli/internal/repository/tzzb"
)

type PositionService struct {
    repo *tzzb.Client
    calc *Calculator
}

func (s *PositionService) GetPositionList(accountFilter, typeFilter string, flat bool) (*PositionListResult, error) {
    // 1. 从 repository 获取原始数据
    accounts, err := s.repo.GetAccountList()
    if err != nil {
        return nil, err
    }

    // 2. 过滤
    rows := s.filterPositions(accounts, accountFilter, typeFilter)

    // 3. 富化实时数据
    ctx := s.loadLiveContext(accounts, rows)

    // 4. 重新计算
    for i := range rows {
        rows[i].Live = s.calc.RecomputeLive(rows[i], ctx)
    }

    // 5. 返回结构化结果
    return &PositionListResult{
        Rows:    rows,
        Context: ctx,
        Flat:    flat,
    }, nil
}
```

## 不建议现在就大改的原因

1. **功能还在快速迭代**：大重构会拖慢新功能开发
2. **没有测试覆盖**：重构前应该先补充集成测试
3. **用户体验优先**：先保证功能正确，再优化内部结构

## 建议：渐进式重构

每次改一个小模块：
1. 这周：提取 `formatter` 包，统一输出逻辑
2. 下周：提取 `calculator` 包，测试盈亏计算
3. 再下周：拆分 `client.go`，每个接口一个文件

**不要一次性大改，容易翻车。**
