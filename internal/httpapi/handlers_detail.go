package httpapi

import (
	"net/http"

	"github.com/liyanran/tzzb-cli/internal/service"
)

// StockDetailDTO 单个账户下某只股票的全维度明细。
type StockDetailDTO struct {
	AccountName string           `json:"account_name"`
	Merge       *MergeDetailDTO  `json:"merge"`
	Records     []HoldRecordDTO  `json:"records"`
	Trades      []StockTradeDTO  `json:"trades"`
	Clear       *ClearSummaryDTO `json:"clear"`
}

// MergeDetailDTO 合并持仓明细。
type MergeDetailDTO struct {
	StockCode   string  `json:"stock_code"`
	StockName   string  `json:"stock_name"`
	Count       float64 `json:"count"`
	HoldDays    int     `json:"hold_days"`
	PriceBuyAv  float64 `json:"price_buy_av"`
	PriceCurr   float64 `json:"price_curr"`
	MarketValue float64 `json:"market_value"`
	Zcb         float64 `json:"zcb"`
	Yke         float64 `json:"yke"`
	ProfitRate  float64 `json:"profit_rate"`
	Fee         float64 `json:"fee"`
	ComeProfit  float64 `json:"come_profit"`
	ClearFlag   string  `json:"clear_flag"`
}

// HoldRecordDTO 建仓记录。
type HoldRecordDTO struct {
	StartDate  string  `json:"start_date"`
	EndDate    string  `json:"end_date"`
	Profit     float64 `json:"profit"`
	ProfitRate float64 `json:"profit_rate"`
}

// StockTradeDTO 历史成交。
type StockTradeDTO struct {
	TransDate   string  `json:"trans_date"`
	TransTime   string  `json:"trans_time"`
	Op          string  `json:"op"` // 1=买入 2=卖出
	Price       float64 `json:"price"`
	Count       float64 `json:"count"`
	Amount      float64 `json:"amount"`
	Fee         float64 `json:"fee"`
	Profit      float64 `json:"profit"`
	AccountName string  `json:"account_name"`
}

// ClearSummaryDTO 清仓汇总。
type ClearSummaryDTO struct {
	StartDate       string  `json:"start_date"`
	EndDate         string  `json:"end_date"`
	HoldDays        int     `json:"hold_days"`
	Fee             float64 `json:"fee"`
	TotalBuy        float64 `json:"total_buy"`
	TotalSell       float64 `json:"total_sell"`
	TotalProfit     float64 `json:"total_profit"`
	TotalProfitRate float64 `json:"total_profit_rate"`
	HasData         bool    `json:"has_data"`
}

// DetailResponse 单股钻取响应（多账户列表）。
type DetailResponse struct {
	Code    string           `json:"code"`
	Details []StockDetailDTO `json:"details"`
}

// handleDetail GET /api/detail/{code}?account=&from=YYYYMMDD&to=YYYYMMDD
func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "缺少股票代码")
		return
	}

	q := r.URL.Query()
	account := q.Get("account")
	from := q.Get("from")
	to := q.Get("to")

	svc := service.NewDetailService(s.client.WithContext(r.Context()))
	details, err := svc.LoadDetail(code, account, from, to)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	resp := DetailResponse{
		Code:    code,
		Details: make([]StockDetailDTO, 0, len(details)),
	}
	for _, d := range details {
		dto := StockDetailDTO{
			AccountName: d.Coord.AccountName,
			Records:     make([]HoldRecordDTO, 0, len(d.Records)),
			Trades:      make([]StockTradeDTO, 0, len(d.Trades)),
		}

		if d.Merge != nil {
			dto.Merge = &MergeDetailDTO{
				StockCode:   d.Merge.StockCode,
				StockName:   d.Merge.StockName,
				Count:       d.Merge.Count,
				HoldDays:    d.Merge.HoldDays,
				PriceBuyAv:  d.Merge.PriceBuyAv,
				PriceCurr:   d.Merge.PriceCurr,
				MarketValue: d.Merge.MarketValue,
				Zcb:         d.Merge.Zcb,
				Yke:         d.Merge.Yke,
				ProfitRate:  d.Merge.ProfitRate,
				Fee:         d.Merge.Fee,
				ComeProfit:  d.Merge.ComeProfit,
				ClearFlag:   d.Merge.ClearFlag,
			}
		}

		for _, rec := range d.Records {
			dto.Records = append(dto.Records, HoldRecordDTO{
				StartDate:  rec.StartDate,
				EndDate:    rec.EndDate,
				Profit:     rec.Profit,
				ProfitRate: rec.ProfitRate,
			})
		}

		for _, t := range d.Trades {
			dto.Trades = append(dto.Trades, StockTradeDTO{
				TransDate:   t.TransDate,
				TransTime:   t.TransTime,
				Op:          t.Op,
				Price:       t.Price,
				Count:       t.Count,
				Amount:      t.Amount,
				Fee:         t.Fee,
				Profit:      t.Profit,
				AccountName: t.AccountName,
			})
		}

		if d.Clear != nil {
			dto.Clear = &ClearSummaryDTO{
				StartDate:       d.Clear.StartDate,
				EndDate:         d.Clear.EndDate,
				HoldDays:        d.Clear.HoldDays,
				Fee:             d.Clear.Fee,
				TotalBuy:        d.Clear.TotalBuy,
				TotalSell:       d.Clear.TotalSell,
				TotalProfit:     d.Clear.TotalProfit,
				TotalProfitRate: d.Clear.TotalProfitRate,
				HasData:         d.Clear.HasData(),
			}
		}

		resp.Details = append(resp.Details, dto)
	}

	writeJSON(w, resp)
}
