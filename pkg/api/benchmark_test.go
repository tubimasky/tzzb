package api

import "testing"

func TestCalculateCumulativeReturns(t *testing.T) {
	points := []BenchmarkPoint{
		{Date: "2026-06-28", Close: 3000}, // 基准日之前的最后一个交易日
		{Date: "2026-07-01", Close: 3300}, // +10%
		{Date: "2026-07-02", Close: 2700}, // -10%
	}

	t.Run("基准日非交易日时取之前最后一个交易日", func(t *testing.T) {
		// 2026-06-29 是周日，无 K 线；应回退到 06-28 的 3000
		got := CalculateCumulativeReturns(points, "2026-06-29")
		if diff := got["2026-07-01"] - 0.10; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("07-01 累计：期望 +10%%，实际 %.4f%%", got["2026-07-01"]*100)
		}
		if diff := got["2026-07-02"] - (-0.10); diff > 1e-9 || diff < -1e-9 {
			t.Errorf("07-02 累计：期望 -10%%，实际 %.4f%%", got["2026-07-02"]*100)
		}
	})

	t.Run("基准日本身为交易日时累计为 0", func(t *testing.T) {
		got := CalculateCumulativeReturns(points, "2026-07-01")
		if got["2026-07-01"] != 0 {
			t.Errorf("基准日累计应为 0，实际 %.4f", got["2026-07-01"])
		}
		// 3000/3300 - 1
		want := 3000.0/3300.0 - 1
		if diff := got["2026-06-28"] - want; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("基准日之前的点：期望 %.4f，实际 %.4f", want, got["2026-06-28"])
		}
	})

	t.Run("基准日早于全部数据时退化为序列首日", func(t *testing.T) {
		got := CalculateCumulativeReturns(points, "2026-01-01")
		if got["2026-06-28"] != 0 {
			t.Errorf("应以序列首日为基准，实际 %.4f", got["2026-06-28"])
		}
	})

	t.Run("空输入", func(t *testing.T) {
		if got := CalculateCumulativeReturns(nil, "2026-07-01"); got != nil {
			t.Errorf("空输入应返回 nil，实际 %v", got)
		}
	})
}
