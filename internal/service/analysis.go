package service

import (
	"sort"
	"time"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

// AnalysisService 股票仓位分析业务逻辑（梯队 B）。
type AnalysisService struct {
	client *api.Client
}

// NewAnalysisService 创建仓位分析服务。
func NewAnalysisService(client *api.Client) *AnalysisService {
	return &AnalysisService{client: client}
}

// defaultRange 返回本月初到今天的 YYYYMMDD 区间。
func defaultRange() (start, end string) {
	now := time.Now()
	return api.FormatCompactDate(now.AddDate(0, 0, -now.Day()+1)), api.FormatCompactDate(now)
}

// DistItem 是持仓分布的一只标的（带占比）。
type DistItem struct {
	api.PositionDistItem
	Weight float64 // 市值占比 %
}

// LoadDistribution 加载持仓分布，按市值降序并计算占比。
func (s *AnalysisService) LoadDistribution(accountFilter string) ([]DistItem, float64, error) {
	params, err := buildCalendarParams(s.client, accountFilter)
	if err != nil {
		return nil, 0, err
	}
	start, end := defaultRange()
	items, total, err := s.client.GetPositionDistribution(params, start, end)
	if err != nil {
		return nil, 0, err
	}

	// 接口的 total_value 有时为 0，用明细累加兜底。
	if total == 0 {
		for _, it := range items {
			total += it.Value
		}
	}

	out := make([]DistItem, 0, len(items))
	for _, it := range items {
		weight := 0.0
		if total > 0 {
			weight = it.Value / total * 100
		}
		out = append(out, DistItem{PositionDistItem: it, Weight: weight})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Value > out[j].Value
	})
	return out, total, nil
}

// LoadPositionRate 加载各股票账户的仓位率。
func (s *AnalysisService) LoadPositionRate(accountFilter string) ([]api.AccountPosRate, error) {
	params, err := buildCalendarParams(s.client, accountFilter)
	if err != nil {
		return nil, err
	}
	start, end := defaultRange()
	rows, err := s.client.GetAccountPositionRate(params, start, end)
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].StockValue > rows[j].StockValue
	})
	return rows, nil
}
