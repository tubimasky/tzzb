package service

import (
	"strings"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

// TrendService 趋势数据业务逻辑
type TrendService struct {
	client *api.Client
}

// NewTrendService 创建趋势服务
func NewTrendService(client *api.Client) *TrendService {
	return &TrendService{client: client}
}

// AccountGroups 账户分组结果
type AccountGroups struct {
	All   []api.Account
	Stock []api.Account
	Fund  []api.Account
}

// GroupAccountsByCategory 按类型分组账户
func (s *TrendService) GroupAccountsByCategory(accounts []api.Account) AccountGroups {
	var groups AccountGroups
	for _, acc := range accounts {
		groups.All = append(groups.All, acc)
		if acc.Category == "fund" {
			groups.Fund = append(groups.Fund, acc)
		} else {
			groups.Stock = append(groups.Stock, acc)
		}
	}
	return groups
}

// FilterAccounts 按账户名过滤
func (s *TrendService) FilterAccounts(accounts []api.Account, accountFilter string) []api.Account {
	if accountFilter == "" {
		return accounts
	}
	var matched []api.Account
	for _, acc := range accounts {
		if strings.Contains(acc.Name(), accountFilter) {
			matched = append(matched, acc)
		}
	}
	return matched
}

// TrendData 趋势数据（带元信息）
type TrendData struct {
	Account  string
	Category string
	Days     int
	Points   []api.AssetTrendPoint
}

// LoadTrendForAccounts 加载多个账户的趋势数据
func (s *TrendService) LoadTrendForAccounts(accounts []api.Account, days int) []TrendData {
	var results []TrendData
	for _, acc := range accounts {
		points, err := s.client.GetAssetTrend(acc, days)
		if err != nil {
			continue
		}
		results = append(results, TrendData{
			Account:  acc.Name(),
			Category: acc.Category,
			Days:     days,
			Points:   points,
		})
	}
	return results
}

// AggregateTrendData 聚合多个账户的趋势数据（按日期合并）
func (s *TrendService) AggregateTrendData(accounts []api.Account, days int) []api.AssetTrendPoint {
	dateMap := make(map[string]*api.AssetTrendPoint)

	for _, acc := range accounts {
		points, err := s.client.GetAssetTrend(acc, days)
		if err != nil {
			continue
		}

		for _, p := range points {
			if existing, ok := dateMap[p.Date]; ok {
				existing.TotalAsset += p.TotalAsset
				existing.Profit += p.Profit
				existing.Cash += p.Cash
				existing.MarketValue += p.MarketValue
			} else {
				dateMap[p.Date] = &api.AssetTrendPoint{
					Date:        p.Date,
					TotalAsset:  p.TotalAsset,
					Profit:      p.Profit,
					Cash:        p.Cash,
					MarketValue: p.MarketValue,
				}
			}
		}
	}

	if len(dateMap) == 0 {
		return nil
	}

	// 转成有序列表（按日期排序）
	var dates []string
	for date := range dateMap {
		dates = append(dates, date)
	}
	// 简单排序（日期字符串可以直接比较）
	for i := 0; i < len(dates); i++ {
		for j := i + 1; j < len(dates); j++ {
			if dates[i] > dates[j] {
				dates[i], dates[j] = dates[j], dates[i]
			}
		}
	}

	var points []api.AssetTrendPoint
	for _, date := range dates {
		p := dateMap[date]
		// 计算仓位
		if p.TotalAsset > 0 {
			p.PositionRate = p.MarketValue / p.TotalAsset
		}
		points = append(points, *p)
	}

	return points
}

// TrendStatistics 趋势统计数据
type TrendStatistics struct {
	FirstDate      string
	FirstAsset     float64
	LastDate       string
	LastAsset      float64
	AssetChange    float64
	ProfitChange   float64
	MaxDrawdown    float64
	HasMaxDrawdown bool
}

// ComputeStatistics 计算趋势统计（区间变化、最大回撤）
func (s *TrendService) ComputeStatistics(points []api.AssetTrendPoint) TrendStatistics {
	if len(points) < 2 {
		return TrendStatistics{}
	}

	first := points[0]
	last := points[len(points)-1]

	stats := TrendStatistics{
		FirstDate:    first.Date,
		FirstAsset:   first.TotalAsset,
		LastDate:     last.Date,
		LastAsset:    last.TotalAsset,
		AssetChange:  last.TotalAsset - first.TotalAsset,
		ProfitChange: last.Profit - first.Profit,
	}

	// 计算最大回撤
	maxAsset := 0.0
	maxDrawdown := 0.0
	for _, p := range points {
		if p.TotalAsset > maxAsset {
			maxAsset = p.TotalAsset
		}
		if maxAsset > 0 {
			dd := (maxAsset - p.TotalAsset) / maxAsset
			if dd > maxDrawdown {
				maxDrawdown = dd
			}
		}
	}

	if maxDrawdown > 0 {
		stats.MaxDrawdown = maxDrawdown
		stats.HasMaxDrawdown = true
	}

	return stats
}

// LimitPoints 限制显示的数据点数量（返回最近 N 条）
func (s *TrendService) LimitPoints(points []api.AssetTrendPoint, limit int) ([]api.AssetTrendPoint, int) {
	if limit <= 0 || len(points) <= limit {
		return points, 0
	}
	start := len(points) - limit
	return points[start:], start
}
