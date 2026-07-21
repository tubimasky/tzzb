package service

import (
	"testing"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

func TestHoldingProfitRateCalculation(t *testing.T) {
	tests := []struct {
		name           string
		previousPoint  api.AssetTrendPoint
		currentPoint   api.AssetTrendPoint
		expectedRate   float64
		description    string
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
