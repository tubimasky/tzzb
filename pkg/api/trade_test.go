package api

import "testing"

func TestLimitAssetTrendPointsByDays(t *testing.T) {
	points := []AssetTrendPoint{
		{Date: "2026-04-30", TotalAsset: 100},
		{Date: "2026-05-01", TotalAsset: 101},
		{Date: "2026-07-28", TotalAsset: 102},
		{Date: "2026-07-29", TotalAsset: 103},
	}

	got := limitAssetTrendPointsByDays(points, 90)
	if len(got) != 3 {
		t.Fatalf("近 90 天应保留 3 个点，实际 %d (%+v)", len(got), got)
	}
	if got[0].Date != "2026-05-01" || got[2].Date != "2026-07-29" {
		t.Fatalf("裁剪区间错误: first=%s last=%s", got[0].Date, got[2].Date)
	}
}

func TestLimitAssetTrendPointsByDaysKeepsInvalidDates(t *testing.T) {
	points := []AssetTrendPoint{
		{Date: "bad-date", TotalAsset: 100},
		{Date: "2026-07-29", TotalAsset: 103},
	}

	got := limitAssetTrendPointsByDays(points, 90)
	if len(got) != 2 {
		t.Fatalf("无法解析的日期应保守保留，实际 %+v", got)
	}
}
