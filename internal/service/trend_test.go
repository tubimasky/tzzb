package service

import (
	"testing"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

func TestHoldingProfitRateCalculation(t *testing.T) {
	tests := []struct {
		name          string
		previousPoint api.AssetTrendPoint
		currentPoint  api.AssetTrendPoint
		expectedRate  float64
		description   string
	}{
		{
			name: "无资金流入，纯市值变化",
			previousPoint: api.AssetTrendPoint{
				Date:        "2026-01-01",
				TotalAsset:  10000,
				MarketValue: 10000,
				FundIn:      0,
				FundOut:     0,
			},
			currentPoint: api.AssetTrendPoint{
				Date:        "2026-01-02",
				TotalAsset:  10500,
				MarketValue: 10500,
				FundIn:      0,
				FundOut:     0,
			},
			expectedRate: 0.05, // 5% 收益
			description:  "市值从 10000 涨到 10500，无资金流入，收益率应为 5%",
		},
		{
			name: "有资金流入，不应算作收益",
			previousPoint: api.AssetTrendPoint{
				Date:        "2026-01-01",
				TotalAsset:  100,
				MarketValue: 100,
				FundIn:      0,
				FundOut:     0,
			},
			currentPoint: api.AssetTrendPoint{
				Date:        "2026-01-02",
				TotalAsset:  200,
				MarketValue: 200,
				FundIn:      100,
				FundOut:     0,
			},
			expectedRate: 0.0, // 0% 收益（充值不算收益）
			description:  "充值 100 元后市值变为 200，实际收益率应为 0%",
		},
		{
			name: "充值后有涨幅",
			previousPoint: api.AssetTrendPoint{
				Date:        "2026-01-01",
				TotalAsset:  100,
				MarketValue: 100,
				FundIn:      0,
				FundOut:     0,
			},
			currentPoint: api.AssetTrendPoint{
				Date:        "2026-01-02",
				TotalAsset:  210,
				MarketValue: 210,
				FundIn:      100,
				FundOut:     0,
			},
			expectedRate: 0.10, // 10% 收益
			description:  "充值 100 后市值涨到 210，实际市值增长 10（210-100-100=10），收益率 10%",
		},
		{
			name: "有资金流出",
			previousPoint: api.AssetTrendPoint{
				Date:        "2026-01-01",
				TotalAsset:  10000,
				MarketValue: 10000,
				FundIn:      0,
				FundOut:     0,
			},
			currentPoint: api.AssetTrendPoint{
				Date:        "2026-01-02",
				TotalAsset:  9500,
				MarketValue: 9500,
				FundIn:      0,
				FundOut:     500,
			},
			expectedRate: 0.0, // 0% 收益（纯取出）
			description:  "取出 500 元后市值变为 9500，实际收益率应为 0%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 计算持仓盈亏率
			marketValueChange := tt.currentPoint.MarketValue - tt.previousPoint.MarketValue
			netFundFlow := tt.currentPoint.FundIn - tt.currentPoint.FundOut
			actualMarketReturn := marketValueChange - netFundFlow
			rate := actualMarketReturn / tt.previousPoint.MarketValue

			if rate != tt.expectedRate {
				t.Errorf("%s\n期望收益率: %.2f%%, 实际收益率: %.2f%%",
					tt.description, tt.expectedRate*100, rate*100)
			}
		})
	}
}

func TestComputeHoldingReturns(t *testing.T) {
	svc := &TrendService{}

	t.Run("转入不算收益，累计按复利", func(t *testing.T) {
		points := []api.AssetTrendPoint{
			{Date: "2026-01-01", TotalAsset: 10000},
			// 涨 10%
			{Date: "2026-01-02", TotalAsset: 11000},
			// 转入 5000，市值不动 → 当日收益 0
			{Date: "2026-01-05", TotalAsset: 16000, FundIn: 5000},
			// 跌 10%
			{Date: "2026-01-06", TotalAsset: 14400},
		}

		got := svc.ComputeHoldingReturns(points)
		if len(got) != 4 {
			t.Fatalf("长度应为 4，实际 %d", len(got))
		}

		want := []float64{0, 0.10, 0, -0.10}
		for i, w := range want {
			if diff := got[i].Daily - w; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("第 %d 天单日收益：期望 %.4f，实际 %.4f", i, w, got[i].Daily)
			}
		}

		// 复利：1.10 × 1.00 × 0.90 - 1 = -0.01
		if diff := got[3].Cumulative - (-0.01); diff > 1e-9 || diff < -1e-9 {
			t.Errorf("累计收益：期望 -1%%，实际 %.4f%%", got[3].Cumulative*100)
		}
	})

	t.Run("转出不算亏损", func(t *testing.T) {
		points := []api.AssetTrendPoint{
			{Date: "2026-01-01", TotalAsset: 10000},
			{Date: "2026-01-02", TotalAsset: 9500, FundOut: 500},
		}

		got := svc.ComputeHoldingReturns(points)
		if got[1].Daily != 0 {
			t.Errorf("纯转出应为 0 收益，实际 %.4f", got[1].Daily)
		}
	})

	t.Run("前一日资产为 0 时标记 invalid 且不重置累计", func(t *testing.T) {
		points := []api.AssetTrendPoint{
			{Date: "2026-01-01", TotalAsset: 10000},
			{Date: "2026-01-02", TotalAsset: 11000},
			{Date: "2026-01-05", TotalAsset: 0},
			{Date: "2026-01-06", TotalAsset: 5000},
		}

		got := svc.ComputeHoldingReturns(points)
		if got[3].Valid {
			t.Error("前一日资产为 0，第 4 天应为 invalid")
		}
		// 累计沿用 1.10 - 1
		if diff := got[3].Cumulative - 0.10; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("累计应沿用 10%%，实际 %.4f%%", got[3].Cumulative*100)
		}
	})

	t.Run("空输入", func(t *testing.T) {
		if got := svc.ComputeHoldingReturns(nil); len(got) != 0 {
			t.Errorf("空输入应返回空切片，实际长度 %d", len(got))
		}
	})
}

func TestComputeReturnDrawdown(t *testing.T) {
	svc := &TrendService{}

	// 收益曲线 0 → +20% → -10%：从净值 1.20 跌到 0.90，回撤 25%
	returns := []HoldingReturn{
		{Cumulative: 0, Valid: true},
		{Cumulative: 0.20, Valid: true},
		{Cumulative: -0.10, Valid: true},
	}

	dd, ok := svc.ComputeReturnDrawdown(returns)
	if !ok {
		t.Fatal("应识别出回撤")
	}
	if diff := dd - 0.25; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("最大回撤：期望 25%%，实际 %.4f%%", dd*100)
	}

	// 单调上涨无回撤
	if _, ok := svc.ComputeReturnDrawdown([]HoldingReturn{
		{Cumulative: 0}, {Cumulative: 0.05}, {Cumulative: 0.11},
	}); ok {
		t.Error("单调上涨不应有回撤")
	}

	if _, ok := svc.ComputeReturnDrawdown(nil); ok {
		t.Error("空输入不应有回撤")
	}
}

func TestAggregateTrendSeriesAlignsAccounts(t *testing.T) {
	series := [][]api.AssetTrendPoint{
		{
			{Date: "2026-01-01", TotalAsset: 100, MarketValue: 80, FundIn: 10},
			{Date: "2026-01-02", TotalAsset: 110, MarketValue: 90},
			{Date: "2026-01-03", TotalAsset: 120, MarketValue: 100},
			{Date: "2026-01-04", TotalAsset: 130, MarketValue: 110},
		},
		{
			{Date: "2026-01-02", TotalAsset: 200, MarketValue: 150, FundIn: 20},
			{Date: "2026-01-04", TotalAsset: 220, MarketValue: 170, FundOut: 5},
		},
	}

	got := aggregateTrendSeries(series)
	if len(got) != 3 {
		t.Fatalf("共同区间应包含 3 个日期，实际 %d", len(got))
	}
	if got[0].Date != "2026-01-02" || got[2].Date != "2026-01-04" {
		t.Fatalf("日期范围错误: %s ~ %s", got[0].Date, got[2].Date)
	}
	if got[1].TotalAsset != 320 {
		t.Errorf("缺点账户应向前填充，2026-01-03 总资产期望 320，实际 %.2f", got[1].TotalAsset)
	}
	if got[1].FundIn != 0 || got[1].FundOut != 0 {
		t.Errorf("向前填充不应重复资金流，实际 in=%.2f out=%.2f", got[1].FundIn, got[1].FundOut)
	}
	if got[0].FundIn != 20 || got[2].FundOut != 5 {
		t.Errorf("真实数据点资金流丢失: first=%+v last=%+v", got[0], got[2])
	}
}
