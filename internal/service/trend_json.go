package service

import (
	"github.com/liyanran/tzzb-cli/internal/formatter"
	"github.com/liyanran/tzzb-cli/pkg/api"
)

// TrendJSONService 趋势 JSON 输出服务
type TrendJSONService struct {
	trendService *TrendService
}

// NewTrendJSONService 创建趋势 JSON 服务
func NewTrendJSONService(trendService *TrendService) *TrendJSONService {
	return &TrendJSONService{trendService: trendService}
}

// ConvertTrendPointsToJSON 转换趋势数据点为 JSON（带变化量计算）
func (s *TrendJSONService) ConvertTrendPointsToJSON(points []api.AssetTrendPoint, limit int) []map[string]any {
	displayPoints, start := s.trendService.LimitPoints(points, limit)

	var result []map[string]any
	for i := 0; i < len(displayPoints); i++ {
		p := displayPoints[i]
		actualIdx := start + i

		item := map[string]any{
			"date":          p.Date,
			"total_asset":   p.TotalAsset,
			"cash":          p.Cash,
			"market_value":  p.MarketValue,
			"position_rate": p.PositionRate,
		}

		// 计算变化量
		if actualIdx > 0 {
			change := p.TotalAsset - points[actualIdx-1].TotalAsset
			item["asset_change"] = change
			item["day_profit"] = change

			// 持仓盈亏率
			if points[actualIdx-1].MarketValue > 0 {
				marketValueChange := p.MarketValue - points[actualIdx-1].MarketValue
				rate := marketValueChange / points[actualIdx-1].MarketValue * 100
				item["holding_profit_rate"] = rate
			}
		}

		result = append(result, item)
	}
	return result
}

// BuildSingleAccountJSON 构建单个账户趋势 JSON
func (s *TrendJSONService) BuildSingleAccountJSON(data TrendData, limit int) map[string]any {
	return map[string]any{
		"account":  data.Account,
		"category": api.CategoryLabel(data.Category),
		"days":     data.Days,
		"points":   s.ConvertTrendPointsToJSON(data.Points, limit),
	}
}

// BuildAggregatedJSON 构建聚合趋势 JSON
func (s *TrendJSONService) BuildAggregatedJSON(title string, points []api.AssetTrendPoint, days int, limit int) map[string]any {
	if len(points) == 0 {
		return map[string]any{"error": "no trend data"}
	}

	return map[string]any{
		"title":  title,
		"days":   days,
		"count":  len(points),
		"points": s.ConvertTrendPointsToJSON(points, limit),
	}
}

// PrintJSON 输出 JSON（统一入口）
func (s *TrendJSONService) PrintJSON(data map[string]any) {
	formatter.PrintJSON(data)
}
