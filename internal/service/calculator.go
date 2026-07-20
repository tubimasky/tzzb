package service

import "github.com/liyanran/tzzb-cli/pkg/api"

// Calculator 负责持仓盈亏计算逻辑。
type Calculator struct{}

// NewCalculator 创建盈亏计算器实例。
func NewCalculator() *Calculator {
	return &Calculator{}
}

// RecomputeLive 对齐插件 stockCalc（auto 账户口径）：
//
//	现价       = 行情 xianjia（无则回落接口价）
//	市值       = 数量 × 现价
//	今日盈亏   = 当日买卖收益 + (现价−昨收)×(持仓−今日买入−正 remainchg) − 费用 + 速记
//	今日涨幅   = (现价 − 昨收) / 昨收          ← 插件 hq.zf，不是盈亏比
//	当日盈亏比 = 今日盈亏 / (市值 − 今日盈亏 + 卖出额)  ← 插件 drykb；合计行同口径
//	持仓盈亏   = 接口 hold_profit + (现价 − 接口价) × 数量
//	收益率     = 持仓盈亏 / (成本价 × 数量)   ← 插件 浮动盈亏比
func (c *Calculator) RecomputeLive(count, cost, apiPrice, apiValue, apiHold, apiHoldRate float64, q *api.Quote, trades []api.DayTrade) api.LiveView {
	return api.RecomputeLive(count, cost, apiPrice, apiValue, apiHold, apiHoldRate, q, trades)
}

// RecomputePosition 为单条持仓行计算实时盈亏。
// 参数:
//   - pos: 持仓数据（posRow 结构）
//   - quote: 实时行情（可为 nil）
//   - trades: 当日成交列表（用于计算今日盈亏）
//   - isFund: 是否基金账户（基金不计算今日盈亏）
func (c *Calculator) RecomputePosition(count, cost, price, value, holdProfit, holdRate float64, quote *api.Quote, trades []api.DayTrade, isFund bool) api.LiveView {
	if isFund {
		// 基金账户不拉行情，不算今日盈亏
		quote = nil
		trades = nil
	}
	return c.RecomputeLive(count, cost, price, value, holdProfit, holdRate, quote, trades)
}

// RecomputeMerged 为跨账户合并后的持仓计算实时盈亏。
// 参数:
//   - count: 合并后总数量
//   - cost: 数量加权平均成本
//   - price: API 返回的价格
//   - value: API 返回的市值
//   - holdProfit: 合并后持仓盈亏
//   - quote: 实时行情（可为 nil）
//   - trades: 跨账户合并的当日成交
func (c *Calculator) RecomputeMerged(count, cost, price, value, holdProfit float64, quote *api.Quote, trades []api.DayTrade) api.LiveView {
	// 计算接口收益率（用于 RecomputeLive）
	apiHoldRate := 0.0
	costBasis := cost * count
	if costBasis > 0 {
		apiHoldRate = holdProfit / costBasis
	}
	return c.RecomputeLive(count, cost, price, value, holdProfit, apiHoldRate, quote, trades)
}
