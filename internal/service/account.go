package service

import (
	"sort"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

// AccountService 账户服务层
type AccountService struct {
	client *api.Client
}

// NewAccountService 创建账户服务实例
func NewAccountService(client *api.Client) *AccountService {
	return &AccountService{client: client}
}

// AccountSummary 账户汇总统计
type AccountSummary struct {
	TotalPrincipal float64
	TotalAssets    float64
	HoldProfit     float64
	TodayProfit    float64
	TotalProfit    float64
	TotalRate      float64 // 总收益率 (totalProfit / totalPrincipal * 100)
}

// GetAccountList 获取账户列表
func (s *AccountService) GetAccountList() ([]api.Account, error) {
	return s.client.GetAccountList()
}

// CalculateSummary 计算账户汇总统计
func (s *AccountService) CalculateSummary(accounts []api.Account) AccountSummary {
	var summary AccountSummary

	for _, acc := range accounts {
		summary.TotalPrincipal += acc.Principal.F()
		summary.TotalAssets += acc.TotalAssets.F()
		summary.HoldProfit += acc.HoldProfit.F()
		summary.TodayProfit += acc.TodayProfit.F()
		summary.TotalProfit += acc.TotalProfit.F()
	}

	// 计算总收益率
	if summary.TotalPrincipal > 0 {
		summary.TotalRate = summary.TotalProfit / summary.TotalPrincipal * 100
	}

	return summary
}

// SortByTotalProfit 按总盈亏从高到低排序
func (s *AccountService) SortByTotalProfit(accounts []api.Account) {
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].TotalProfit.F() > accounts[j].TotalProfit.F()
	})
}

// CalculateAccountRate 计算单个账户的收益率
func (s *AccountService) CalculateAccountRate(acc api.Account) float64 {
	principal := acc.Principal.F()
	if principal > 0 {
		return acc.TotalProfit.F() / principal * 100
	}
	return 0.0
}
