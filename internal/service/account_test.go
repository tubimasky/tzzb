package service

import (
	"testing"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

func TestAccountService_CalculateSummary(t *testing.T) {
	svc := NewAccountService(nil)

	accounts := []api.Account{
		{
			Category:    "stock",
			ManualID:    "1",
			ManualName:  "账户A",
			Principal:   100000,
			TotalAssets: 110000,
			HoldProfit:  8000,
			TodayProfit: 1000,
			TotalProfit: 10000,
		},
		{
			Category:    "fund",
			FundID:      "2",
			FundName:    "账户B",
			Principal:   50000,
			TotalAssets: 52000,
			HoldProfit:  1500,
			TodayProfit: 500,
			TotalProfit: 2000,
		},
	}

	summary := svc.CalculateSummary(accounts)

	// 验证本金汇总
	if summary.TotalPrincipal != 150000 {
		t.Errorf("TotalPrincipal = %v, want 150000", summary.TotalPrincipal)
	}

	// 验证总资产汇总
	if summary.TotalAssets != 162000 {
		t.Errorf("TotalAssets = %v, want 162000", summary.TotalAssets)
	}

	// 验证持仓盈亏汇总
	if summary.HoldProfit != 9500 {
		t.Errorf("HoldProfit = %v, want 9500", summary.HoldProfit)
	}

	// 验证当日盈亏汇总
	if summary.TodayProfit != 1500 {
		t.Errorf("TodayProfit = %v, want 1500", summary.TodayProfit)
	}

	// 验证总盈亏汇总
	if summary.TotalProfit != 12000 {
		t.Errorf("TotalProfit = %v, want 12000", summary.TotalProfit)
	}

	// 验证总收益率 = 总盈亏 / 本金 * 100 = 12000 / 150000 * 100 = 8%
	expectedRate := 8.0
	if abs(summary.TotalRate-expectedRate) > 0.01 {
		t.Errorf("TotalRate = %v%%, want %v%%", summary.TotalRate, expectedRate)
	}
}

func TestAccountService_CalculateSummary_Empty(t *testing.T) {
	svc := NewAccountService(nil)

	summary := svc.CalculateSummary([]api.Account{})

	if summary.TotalPrincipal != 0 {
		t.Errorf("Empty summary TotalPrincipal should be 0, got %v", summary.TotalPrincipal)
	}
	if summary.TotalRate != 0 {
		t.Errorf("Empty summary TotalRate should be 0, got %v", summary.TotalRate)
	}
}

func TestAccountService_SortByTotalProfit(t *testing.T) {
	svc := NewAccountService(nil)

	accounts := []api.Account{
		{
			Category:    "stock",
			ManualID:    "1",
			ManualName:  "账户A",
			TotalProfit: 5000,
		},
		{
			Category:    "stock",
			ManualID:    "2",
			ManualName:  "账户B",
			TotalProfit: 10000,
		},
		{
			Category:    "stock",
			ManualID:    "3",
			ManualName:  "账户C",
			TotalProfit: -2000,
		},
	}

	svc.SortByTotalProfit(accounts)

	// 验证降序排列：B (10000) > A (5000) > C (-2000)
	if accounts[0].Name() != "账户B" {
		t.Errorf("First account should be 账户B, got %s", accounts[0].Name())
	}
	if accounts[1].Name() != "账户A" {
		t.Errorf("Second account should be 账户A, got %s", accounts[1].Name())
	}
	if accounts[2].Name() != "账户C" {
		t.Errorf("Third account should be 账户C, got %s", accounts[2].Name())
	}
}

func TestAccountService_CalculateAccountRate(t *testing.T) {
	svc := NewAccountService(nil)

	tests := []struct {
		name     string
		account  api.Account
		wantRate float64
	}{
		{
			name: "正收益",
			account: api.Account{
				Principal:   100000,
				TotalProfit: 15000,
			},
			wantRate: 15.0,
		},
		{
			name: "负收益",
			account: api.Account{
				Principal:   100000,
				TotalProfit: -5000,
			},
			wantRate: -5.0,
		},
		{
			name: "零本金",
			account: api.Account{
				Principal:   0,
				TotalProfit: 1000,
			},
			wantRate: 0.0,
		},
		{
			name: "零收益",
			account: api.Account{
				Principal:   50000,
				TotalProfit: 0,
			},
			wantRate: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRate := svc.CalculateAccountRate(tt.account)

			if abs(gotRate-tt.wantRate) > 0.01 {
				t.Errorf("CalculateAccountRate() = %v, want %v", gotRate, tt.wantRate)
			}
		})
	}
}
