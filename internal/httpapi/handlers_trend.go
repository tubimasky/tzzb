package httpapi

import (
	"net/http"
	"strconv"

	"github.com/liyanran/tzzb-cli/internal/service"
	"github.com/liyanran/tzzb-cli/pkg/api"
)

// TrendPointDTO 是资产趋势曲线上的一天。
// position_rate / profit_rate 是小数（0.78 = 78%），与 account 的 position_rate 一致。
type TrendPointDTO struct {
	Date         string  `json:"date"` // YYYY-MM-DD
	TotalAsset   float64 `json:"total_asset"`
	Profit       float64 `json:"profit"`
	ProfitRate   float64 `json:"profit_rate"`
	Cash         float64 `json:"cash"`
	MarketValue  float64 `json:"market_value"`
	PositionRate float64 `json:"position_rate"`
	FundIn       float64 `json:"fund_in"`
	FundOut      float64 `json:"fund_out"`
	// AssetChange 是相对前一日的资产变化，首日为 0。
	AssetChange float64 `json:"asset_change"`
	// DailyReturn 是剔除银证转账后的当日收益率（小数）。首日为 0。
	DailyReturn float64 `json:"daily_return"`
	// CumReturn 是相对区间首日的复利累计收益率（小数）。
	CumReturn float64 `json:"cum_return"`
	// HasReturn 区分「收益率为 0」与「当日算不出收益率」。
	HasReturn bool `json:"has_return"`
	// Benchmark 是当日上证涨跌（小数）。未请求基准时为 0。
	Benchmark float64 `json:"benchmark"`
	// BenchmarkCum 是上证相对区间首日的累计涨跌（小数），与 CumReturn 同轴可比。
	BenchmarkCum float64 `json:"benchmark_cum"`
	// HasBenchmark 区分「基准为 0」与「无基准数据」。
	HasBenchmark bool `json:"has_benchmark"`
}

// TrendStatsDTO 是区间统计。
type TrendStatsDTO struct {
	FirstDate      string  `json:"first_date"`
	FirstAsset     float64 `json:"first_asset"`
	LastDate       string  `json:"last_date"`
	LastAsset      float64 `json:"last_asset"`
	AssetChange    float64 `json:"asset_change"`
	ProfitChange   float64 `json:"profit_change"`
	MaxDrawdown    float64 `json:"max_drawdown"` // 小数，基于总资产曲线（含出入金）
	HasMaxDrawdown bool    `json:"has_max_drawdown"`
	// CumReturn 是区间累计收益率（复利，已剔除出入金）。
	CumReturn    float64 `json:"cum_return"`
	HasCumReturn bool    `json:"has_cum_return"`
	// ReturnDrawdown 是收益曲线上的最大回撤（小数），不受出入金干扰。
	ReturnDrawdown    float64 `json:"return_drawdown"`
	HasReturnDrawdown bool    `json:"has_return_drawdown"`
	// BenchmarkCum 是同区间上证累计涨跌；Excess 是超额收益（CumReturn - BenchmarkCum）。
	BenchmarkCum float64 `json:"benchmark_cum"`
	Excess       float64 `json:"excess"`
	HasBenchmark bool    `json:"has_benchmark"`
}

// TrendResponse 资产趋势响应。
type TrendResponse struct {
	Category string                     `json:"category"` // all/stock/fund
	Days     int                        `json:"days"`
	Complete bool                       `json:"complete"`
	Warnings []service.TrendLoadWarning `json:"warnings"`
	Points   []TrendPointDTO            `json:"points"`
	Stats    TrendStatsDTO              `json:"stats"`
}

// handleTrend GET /api/trend?days=30&category=all|stock|fund&account=&benchmark=1
//
// 注意：上游按天数分档返回（days=7 与 30 得到同一批点，90 与 365 同理），
// 故前端只提供 30/60/90 这类实际有差异的档位。
// 该接口按账户串行拉取资产趋势 + 银证转账，耗时约数秒，前端应非阻塞加载。
func (s *Server) handleTrend(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	account := q.Get("account")
	category := q.Get("category")
	if category == "" {
		category = "all"
	}

	days := 30
	if v := q.Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "days 应为正整数")
			return
		}
		days = n
	}

	wantBenchmark := q.Get("benchmark") == "1" || q.Get("benchmark") == "true"

	client := s.client.WithContext(r.Context())
	svc := service.NewTrendService(client)

	accounts, err := client.GetAccountMetadata()
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	matched := svc.FilterAccounts(accounts, account)
	if len(matched) == 0 {
		writeError(w, http.StatusNotFound, "没有匹配的账户")
		return
	}

	// 按类型取子集：股票口径含 common/rzrq/manual（非 fund 即股票侧）。
	groups := svc.GroupAccountsByCategory(matched)
	target := groups.All
	switch category {
	case "stock":
		target = groups.Stock
	case "fund":
		target = groups.Fund
	}
	if len(target) == 0 {
		writeJSON(w, TrendResponse{
			Category: category,
			Days:     days,
			Complete: true,
			Warnings: []service.TrendLoadWarning{},
			Points:   []TrendPointDTO{},
		})
		return
	}

	points, warnings := svc.AggregateTrendDataWithWarnings(target, days)
	stats := svc.ComputeStatistics(points)

	// 剔除银证转账后的逐日/累计收益率，用于与指数同轴对比。
	returns := svc.ComputeHoldingReturns(points)
	returnDD, hasReturnDD := svc.ComputeReturnDrawdown(returns)

	// 基准（上证指数）：取不到就静默降级，不让主数据失败。
	// 累计口径以区间首日收盘价为基准，和 CumReturn 同起点。
	var benchmarkCum map[string]float64
	var benchmarkDaily map[string]float64
	if wantBenchmark && len(points) > 0 {
		// 多取些天数，保证覆盖到首日之前的最后一个交易日
		if bmPoints, err := api.GetShanghaiIndexHistoryContext(r.Context(), len(points)+30); err == nil {
			benchmarkCum = api.CalculateCumulativeReturns(bmPoints, points[0].Date)
			benchmarkDaily = api.CalculateBenchmarkReturns(bmPoints)
		}
	}

	resp := TrendResponse{
		Category: category,
		Days:     days,
		Complete: len(warnings) == 0,
		Warnings: append([]service.TrendLoadWarning{}, warnings...),
		Points:   make([]TrendPointDTO, 0, len(points)),
		Stats: TrendStatsDTO{
			FirstDate:         stats.FirstDate,
			FirstAsset:        stats.FirstAsset,
			LastDate:          stats.LastDate,
			LastAsset:         stats.LastAsset,
			AssetChange:       stats.AssetChange,
			ProfitChange:      stats.ProfitChange,
			MaxDrawdown:       stats.MaxDrawdown,
			HasMaxDrawdown:    stats.HasMaxDrawdown,
			ReturnDrawdown:    returnDD,
			HasReturnDrawdown: hasReturnDD,
		},
	}

	if len(returns) > 0 {
		resp.Stats.CumReturn = returns[len(returns)-1].Cumulative
		resp.Stats.HasCumReturn = len(points) >= 2
	}

	for i, p := range points {
		dto := TrendPointDTO{
			Date:         p.Date,
			TotalAsset:   p.TotalAsset,
			Profit:       p.Profit,
			ProfitRate:   p.ProfitRate,
			Cash:         p.Cash,
			MarketValue:  p.MarketValue,
			PositionRate: p.PositionRate,
			FundIn:       p.FundIn,
			FundOut:      p.FundOut,
		}
		if i > 0 {
			dto.AssetChange = p.TotalAsset - points[i-1].TotalAsset
		}
		if i < len(returns) {
			dto.DailyReturn = returns[i].Daily
			dto.CumReturn = returns[i].Cumulative
			dto.HasReturn = returns[i].Valid
		}
		if benchmarkCum != nil {
			if cum, ok := benchmarkCum[p.Date]; ok {
				dto.BenchmarkCum = cum
				dto.Benchmark = benchmarkDaily[p.Date]
				dto.HasBenchmark = true
			}
		}
		resp.Points = append(resp.Points, dto)
	}

	// 区间超额收益：用最后一个有基准数据的点，避免末日停牌/节假日拿不到值。
	for i := len(resp.Points) - 1; i >= 0; i-- {
		if resp.Points[i].HasBenchmark {
			resp.Stats.BenchmarkCum = resp.Points[i].BenchmarkCum
			resp.Stats.Excess = resp.Points[i].CumReturn - resp.Points[i].BenchmarkCum
			resp.Stats.HasBenchmark = true
			break
		}
	}

	writeJSON(w, resp)
}
