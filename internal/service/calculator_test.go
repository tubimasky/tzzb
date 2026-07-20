package service

import (
	"testing"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

func TestCalculator_RecomputeLive_NoQuote(t *testing.T) {
	calc := NewCalculator()

	// 测试：无行情时，使用接口价格
	result := calc.RecomputeLive(
		1000,  // count
		1.5,   // cost
		1.6,   // apiPrice
		1600,  // apiValue
		100,   // apiHold
		0.067, // apiHoldRate (6.7%)
		nil,   // quote
		nil,   // trades
	)

	if result.Price != 1.6 {
		t.Errorf("Price = %v, want 1.6", result.Price)
	}
	if result.Value != 1600 {
		t.Errorf("Value = %v, want 1600", result.Value)
	}
	if result.HoldProfit != 100 {
		t.Errorf("HoldProfit = %v, want 100", result.HoldProfit)
	}
	if result.Live {
		t.Error("Live should be false when no quote")
	}
}

func TestCalculator_RecomputeLive_WithQuote(t *testing.T) {
	calc := NewCalculator()

	// 测试：有行情时，使用实时价格并重新计算
	quote := &api.Quote{
		Code:      "512890",
		Market:    "2",
		Price:     1.7,
		PrevClose: 1.6,
	}

	result := calc.RecomputeLive(
		1000,  // count
		1.5,   // cost
		1.6,   // apiPrice
		1600,  // apiValue
		100,   // apiHold
		0.067, // apiHoldRate
		quote,
		nil, // trades
	)

	if result.Price != 1.7 {
		t.Errorf("Price = %v, want 1.7", result.Price)
	}
	if result.Value != 1700 {
		t.Errorf("Value = %v, want 1700 (1000 * 1.7)", result.Value)
	}
	// 持仓盈亏 = 接口盈亏 + (现价 - 接口价) * 数量 = 100 + (1.7 - 1.6) * 1000 = 200
	if abs(result.HoldProfit-200) > 0.01 {
		t.Errorf("HoldProfit = %v, want 200", result.HoldProfit)
	}
	// 今日涨幅 = (1.7 - 1.6) / 1.6 * 100 = 6.25%
	expectedDayRate := 6.25
	if abs(result.DayRate-expectedDayRate) > 0.01 {
		t.Errorf("DayRate = %v, want %v", result.DayRate, expectedDayRate)
	}
	if !result.Live {
		t.Error("Live should be true when price changed")
	}
	if !result.DayOK {
		t.Error("DayOK should be true when prevClose available")
	}
}

func TestCalculator_RecomputeLive_WithTrades(t *testing.T) {
	calc := NewCalculator()

	quote := &api.Quote{
		Code:      "512890",
		Market:    "2",
		Price:     1.7,
		PrevClose: 1.6,
	}

	trades := []api.DayTrade{
		{
			Code:    "512890",
			OpType:  api.OpBuy,
			Price:   1.65,
			Count:   100,
			Fee:     1.5,
		},
		{
			Code:    "512890",
			OpType:  api.OpSell,
			Price:   1.75,
			Count:   50,
			Fee:     0.8,
		},
	}

	result := calc.RecomputeLive(
		1000,  // count
		1.5,   // cost
		1.6,   // apiPrice
		1600,  // apiValue
		100,   // apiHold
		0.067, // apiHoldRate
		quote,
		trades,
	)

	// 今日盈亏计算：
	// 买入收益 = 100 * (1.7 - 1.65) = 5
	// 卖出收益 = 50 * (1.75 - 1.6) = 7.5
	// 持仓差价 = (现价 - 昨收) * (持仓 - 买入) = (1.7 - 1.6) * (1000 - 100) = 90
	// 今日盈亏 = 5 + 7.5 + 90 - (1.5 + 0.8) = 100.2
	expectedDayProfit := 100.2
	if abs(result.DayProfit-expectedDayProfit) > 0.01 {
		t.Errorf("DayProfit = %v, want %v", result.DayProfit, expectedDayProfit)
	}

	// 当日盈亏比验证
	// 卖出金额 = 50 * 1.75 = 87.5
	// 分母 = 市值 - 今日盈亏 + 卖出额 = 1700 - 100.2 + 87.5 = 1687.3
	// 当日盈亏比 = 100.2 / 1687.3 * 100 ≈ 5.94%
	if result.DayYKRate < 5.9 || result.DayYKRate > 6.0 {
		t.Errorf("DayYKRate = %v, want ~5.94%%", result.DayYKRate)
	}
}

func TestCalculator_RecomputeMerged(t *testing.T) {
	calc := NewCalculator()

	quote := &api.Quote{
		Code:      "512890",
		Market:    "2",
		Price:     1.7,
		PrevClose: 1.6,
	}

	result := calc.RecomputeMerged(
		2000,  // count (跨账户合并)
		1.45,  // cost (加权平均成本)
		1.6,   // apiPrice
		3200,  // apiValue
		300,   // holdProfit
		quote,
		nil, // trades
	)

	if result.Price != 1.7 {
		t.Errorf("Price = %v, want 1.7", result.Price)
	}
	// 市值 = 2000 * 1.7 = 3400
	if result.Value != 3400 {
		t.Errorf("Value = %v, want 3400", result.Value)
	}
	// 持仓盈亏 = 300 + (1.7 - 1.6) * 2000 = 500
	if abs(result.HoldProfit-500) > 0.01 {
		t.Errorf("HoldProfit = %v, want 500", result.HoldProfit)
	}
	// 收益率 = 500 / (1.45 * 2000) * 100 = 17.24%
	expectedRate := 17.24
	if abs(result.HoldRate-expectedRate) > 0.01 {
		t.Errorf("HoldRate = %v, want %v%%", result.HoldRate, expectedRate)
	}
}

func TestCalculator_RecomputePosition_Fund(t *testing.T) {
	calc := NewCalculator()

	quote := &api.Quote{
		Code:      "001234",
		Price:     1.5,
		PrevClose: 1.4,
	}

	trades := []api.DayTrade{
		{Code: "001234", OpType: api.OpBuy, Price: 1.45, Count: 100, Fee: 1.0},
	}

	// 基金账户应该忽略行情和成交
	result := calc.RecomputePosition(
		1000,  // count
		1.3,   // cost
		1.4,   // apiPrice
		1400,  // apiValue
		100,   // holdProfit
		0.077, // apiHoldRate
		quote,
		trades,
		true, // isFund = true
	)

	// 基金账户不应该有今日数据
	if result.DayOK {
		t.Error("Fund account should not have DayOK")
	}
	if result.DayProfit != 0 {
		t.Errorf("Fund account DayProfit should be 0, got %v", result.DayProfit)
	}
	// 价格应该使用 API 价格（因为 quote 被忽略）
	if result.Price != 1.4 {
		t.Errorf("Fund price = %v, want 1.4 (API price)", result.Price)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
