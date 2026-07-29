package httpapi

import (
	"net/http"

	"github.com/liyanran/tzzb-cli/internal/service"
)

// StockProfitPointDTO 股票盈亏时间点。
type StockProfitPointDTO struct {
	Date       string  `json:"date"` // 202607 或 20260720
	ProfitLoss float64 `json:"profit_loss"`
	Rate       float64 `json:"rate"`    // 账户收益率（小数）
	SZRate     float64 `json:"sz_rate"` // 同期大盘涨跌（小数）
}

// StockProfitResponse 股票盈亏响应。
type StockProfitResponse struct {
	Points    []StockProfitPointDTO `json:"points"`
	TopCodes  []CodePLDTO           `json:"top_codes"`
	TotalPL   float64               `json:"total_pl"`
	TotalRate float64               `json:"total_rate"`
	TotalSZ   float64               `json:"total_sz"`
}

// handleStockProfitMonthly GET /api/stockprofit/monthly?year=YYYY&account=
func (s *Server) handleStockProfitMonthly(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	account := q.Get("account")
	year := q.Get("year")

	svc := service.NewStockProfitService(s.client.WithContext(r.Context()))
	res, usedYear, err := svc.LoadMonthly(account, year)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	resp := StockProfitResponse{
		Points:    make([]StockProfitPointDTO, 0, len(res.Points)),
		TopCodes:  toCodePLDTOs(svc.TopCodes(res, 10)),
		TotalPL:   res.TotalPL,
		TotalRate: res.TotalRate,
		TotalSZ:   res.TotalSZ,
	}
	for _, p := range res.Points {
		resp.Points = append(resp.Points, StockProfitPointDTO{
			Date:       service.FormatMonth(p.Date), // YYYY-MM
			ProfitLoss: p.ProfitLoss,
			Rate:       p.Rate,
			SZRate:     p.SZRate,
		})
	}
	_ = usedYear // 前端可从 points[0].date 推断

	writeJSON(w, resp)
}

// handleStockProfitRange GET /api/stockprofit/range?start=YYYYMMDD&end=YYYYMMDD&account=
func (s *Server) handleStockProfitRange(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	account := q.Get("account")
	start := q.Get("start")
	end := q.Get("end")

	svc := service.NewStockProfitService(s.client.WithContext(r.Context()))
	res, usedStart, usedEnd, err := svc.LoadRange(account, start, end)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	resp := StockProfitResponse{
		Points:    make([]StockProfitPointDTO, 0, len(res.Points)),
		TopCodes:  toCodePLDTOs(svc.TopCodes(res, 10)),
		TotalPL:   res.TotalPL,
		TotalRate: res.TotalRate,
		TotalSZ:   res.TotalSZ,
	}
	for _, p := range res.Points {
		resp.Points = append(resp.Points, StockProfitPointDTO{
			Date:       service.FormatDay(p.Date), // YYYY-MM-DD
			ProfitLoss: p.ProfitLoss,
			Rate:       p.Rate,
			SZRate:     p.SZRate,
		})
	}
	_, _ = usedStart, usedEnd // 前端可从 points 推断

	writeJSON(w, resp)
}
