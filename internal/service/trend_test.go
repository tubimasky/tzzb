package service

import (
	"testing"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

func TestComputeHoldingReturns(t *testing.T) {
	svc := &TrendService{}

	t.Run("使用真实盈亏且累计按复利", func(t *testing.T) {
		points := []api.AssetTrendPoint{
			{Date: "2026-01-01", TotalAsset: 10000},
			{Date: "2026-01-02", TotalAsset: 11000},
			{Date: "2026-01-05", TotalAsset: 16000},
			{Date: "2026-01-06", TotalAsset: 14400},
		}
		dailyPL := map[string]float64{
			"2026-01-02": 1000,  // 1000 / (11000 - 1000) = 10%
			"2026-01-05": 0,     // 转入 5000，但真实盈亏为 0
			"2026-01-06": -1600, // -1600 / (14400 + 1600) = -10%
		}

		got := svc.ComputeHoldingReturns(points, dailyPL)
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

	t.Run("资产暴增但盈亏为零时收益为零", func(t *testing.T) {
		points := []api.AssetTrendPoint{
			{Date: "2026-01-01", TotalAsset: 10000},
			{Date: "2026-01-02", TotalAsset: 110000},
		}

		got := svc.ComputeHoldingReturns(points, map[string]float64{"2026-01-02": 0})
		if !got[1].Valid || got[1].Daily != 0 || got[1].Cumulative != 0 {
			t.Errorf("资产转入不应产生收益，实际 %+v", got[1])
		}
	})

	t.Run("缺少盈亏日历时标记 invalid 且不重置累计", func(t *testing.T) {
		points := []api.AssetTrendPoint{
			{Date: "2026-01-01", TotalAsset: 10000},
			{Date: "2026-01-02", TotalAsset: 11000},
			{Date: "2026-01-05", TotalAsset: 12000},
		}

		got := svc.ComputeHoldingReturns(points, map[string]float64{"2026-01-02": 1000})
		if got[2].Valid {
			t.Error("缺少日历数据的日期应为 invalid")
		}
		if diff := got[2].Cumulative - 0.10; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("累计应沿用 10%%，实际 %.4f%%", got[2].Cumulative*100)
		}
	})

	t.Run("空输入", func(t *testing.T) {
		if got := svc.ComputeHoldingReturns(nil, nil); len(got) != 0 {
			t.Errorf("空输入应返回空切片，实际长度 %d", len(got))
		}
	})
}

func TestMonthsInRange(t *testing.T) {
	got, err := monthsInRange("2026-04-22", "2026-07-29")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"202604", "202605", "202606", "202607"}
	if len(got) != len(want) {
		t.Fatalf("月份数量：期望 %d，实际 %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 个月：期望 %s，实际 %s", i, want[i], got[i])
		}
	}
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

func TestAggregateTrendSeriesUsesUnionDates(t *testing.T) {
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
	if len(got) != 4 {
		t.Fatalf("日期并集应包含 4 个日期，实际 %d", len(got))
	}
	if got[0].Date != "2026-01-01" || got[3].Date != "2026-01-04" {
		t.Fatalf("日期范围错误: %s ~ %s", got[0].Date, got[2].Date)
	}
	if got[0].TotalAsset != 100 {
		t.Errorf("晚出现账户首日前不应回填，2026-01-01 总资产期望 100，实际 %.2f", got[0].TotalAsset)
	}
	if got[2].TotalAsset != 320 {
		t.Errorf("缺点账户应向前填充，2026-01-03 总资产期望 320，实际 %.2f", got[2].TotalAsset)
	}
	if got[2].FundIn != 0 || got[2].FundOut != 0 {
		t.Errorf("向前填充不应重复资金流，实际 in=%.2f out=%.2f", got[2].FundIn, got[2].FundOut)
	}
	if got[0].FundIn != 10 || got[1].FundIn != 20 || got[3].FundOut != 5 {
		t.Errorf("真实数据点资金流丢失: first=%+v second=%+v last=%+v", got[0], got[1], got[3])
	}
}
