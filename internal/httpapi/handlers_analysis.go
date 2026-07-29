package httpapi

import (
	"net/http"

	"github.com/liyanran/tzzb-cli/internal/service"
)

// DistItemDTO 持仓分布项。
type DistItemDTO struct {
	Code   string  `json:"code"`
	Name   string  `json:"name"`
	Value  float64 `json:"value"`
	Weight float64 `json:"weight"` // 占比 %
	Market string  `json:"market"`
}

// AnalysisDistResponse 持仓分布响应。
type AnalysisDistResponse struct {
	Items []DistItemDTO `json:"items"`
	Total float64       `json:"total"`
}

// handleAnalysisDistribution GET /api/analysis/distribution?account=
func (s *Server) handleAnalysisDistribution(w http.ResponseWriter, r *http.Request) {
	account := r.URL.Query().Get("account")

	svc := service.NewAnalysisService(s.client.WithContext(r.Context()))
	items, total, err := svc.LoadDistribution(account)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	resp := AnalysisDistResponse{
		Items: make([]DistItemDTO, 0, len(items)),
		Total: total,
	}
	for _, it := range items {
		resp.Items = append(resp.Items, DistItemDTO{
			Code:   it.Code,
			Name:   it.Name,
			Value:  it.Value,
			Weight: it.Weight,
			Market: it.Market,
		})
	}

	writeJSON(w, resp)
}

// AccountPosRateDTO 单个账户的仓位率。
type AccountPosRateDTO struct {
	Name        string  `json:"name"`
	Broker      string  `json:"broker"`
	StockValue  float64 `json:"stock_value"`
	MoneyRemain float64 `json:"money_remain"`
	Total       float64 `json:"total"`
	Rate        float64 `json:"rate"` // 仓位率 %
	Zjzh        string  `json:"zjzh"`
}

// AnalysisPosRateResponse 账户仓位率响应。
type AnalysisPosRateResponse struct {
	Accounts []AccountPosRateDTO `json:"accounts"`
}

// handleAnalysisPosRate GET /api/analysis/posrate?account=
func (s *Server) handleAnalysisPosRate(w http.ResponseWriter, r *http.Request) {
	account := r.URL.Query().Get("account")

	svc := service.NewAnalysisService(s.client.WithContext(r.Context()))
	rows, err := svc.LoadPositionRate(account)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	resp := AnalysisPosRateResponse{
		Accounts: make([]AccountPosRateDTO, 0, len(rows)),
	}
	for _, row := range rows {
		resp.Accounts = append(resp.Accounts, AccountPosRateDTO{
			Name:        row.Name,
			Broker:      row.Broker,
			StockValue:  row.StockValue,
			MoneyRemain: row.MoneyRemain,
			Total:       row.Total(),
			Rate:        row.Rate(),
			Zjzh:        row.Zjzh,
		})
	}

	writeJSON(w, resp)
}
