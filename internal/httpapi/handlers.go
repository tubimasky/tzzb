package httpapi

import (
	"net/http"

	"github.com/liyanran/tzzb-cli/internal/service"
	"github.com/liyanran/tzzb-cli/pkg/api"
)

// --- 账户总览 ---

type accountDTO struct {
	Name         string   `json:"name"`
	Category     string   `json:"category"`
	TotalAssets  float64  `json:"total_assets"`
	HoldProfit   float64  `json:"hold_profit"`
	TodayProfit  float64  `json:"today_profit"`
	TotalProfit  float64  `json:"total_profit"`
	PositionRate float64  `json:"position_rate"`
	Rate         *float64 `json:"rate"` // 本金缺失时为 null
}

type accountsResp struct {
	Accounts []accountDTO             `json:"accounts"`
	Complete bool                     `json:"complete"`
	Warnings []api.AccountLoadWarning `json:"warnings"`
	Summary  struct {
		TotalPrincipal *float64 `json:"total_principal"`
		TotalAssets    float64  `json:"total_assets"`
		HoldProfit     float64  `json:"hold_profit"`
		TodayProfit    float64  `json:"today_profit"`
		TotalProfit    float64  `json:"total_profit"`
		TotalRate      *float64 `json:"total_rate"`
		ProfitScope    string   `json:"profit_scope"`
	} `json:"summary"`
}

// handleAccounts GET /api/accounts — 账户列表 + 汇总。
func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	svc := service.NewAccountService(s.client.WithContext(r.Context()))
	accounts, warnings, err := svc.GetAccountListWithWarnings()
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	svc.SortByTotalProfit(accounts)
	writeJSON(w, buildAccountsResponse(accounts, warnings))
}

func buildAccountsResponse(accounts []api.Account, warnings []api.AccountLoadWarning) accountsResp {
	svc := service.NewAccountService(nil)
	summary := svc.CalculateSummary(accounts)
	var resp accountsResp
	resp.Complete = len(warnings) == 0
	resp.Warnings = append([]api.AccountLoadWarning{}, warnings...)
	resp.Accounts = make([]accountDTO, 0, len(accounts))
	for _, acc := range accounts {
		resp.Accounts = append(resp.Accounts, accountDTO{
			Name:         acc.Name(),
			Category:     api.CategoryLabel(acc.Category),
			TotalAssets:  acc.TotalAssets.F(),
			HoldProfit:   acc.HoldProfit.F(),
			TodayProfit:  acc.TodayProfit.F(),
			TotalProfit:  acc.TotalProfit.F(),
			PositionRate: acc.PositionRate.F(),
			Rate:         nil,
		})
	}
	resp.Summary.TotalAssets = summary.TotalAssets
	resp.Summary.HoldProfit = summary.HoldProfit
	resp.Summary.TodayProfit = summary.TodayProfit
	resp.Summary.TotalProfit = summary.TotalProfit
	resp.Summary.ProfitScope = "holding"
	return resp
}

// --- 持仓明细 ---

type positionItemDTO struct {
	Account    string  `json:"account"`
	Category   string  `json:"category"`
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	Count      float64 `json:"count"`
	Cost       float64 `json:"cost"`
	Price      float64 `json:"price"`
	Value      float64 `json:"value"`
	HoldProfit float64 `json:"hold_profit"`
	HoldRate   float64 `json:"hold_rate"`
	DayProfit  float64 `json:"day_profit"`
	DayRate    float64 `json:"day_rate"`
}

type positionsResp struct {
	Positions []positionItemDTO `json:"positions"`
	Summary   struct {
		Value      float64 `json:"value"`
		HoldProfit float64 `json:"hold_profit"`
		DayProfit  float64 `json:"day_profit"`
		Count      int     `json:"count"`
	} `json:"summary"`
}

// handlePositions GET /api/positions?account=&type= — 持仓明细（支持账户和类型筛选）。
func (s *Server) handlePositions(w http.ResponseWriter, r *http.Request) {
	accountFilter := r.URL.Query().Get("account")
	typeFilter := r.URL.Query().Get("type") // all/stock/fund
	if typeFilter == "" {
		typeFilter = "all"
	}

	calc := service.NewCalculator()
	positionSvc := service.NewPositionService(s.client.WithContext(r.Context()), calc)

	accounts, err := positionSvc.LoadAccounts()
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	rows := positionSvc.CollectRows(accounts, accountFilter, typeFilter)
	if len(rows) == 0 {
		writeJSON(w, positionsResp{
			Positions: []positionItemDTO{},
			Summary: struct {
				Value      float64 `json:"value"`
				HoldProfit float64 `json:"hold_profit"`
				DayProfit  float64 `json:"day_profit"`
				Count      int     `json:"count"`
			}{},
		})
		return
	}

	ctx := positionSvc.LoadLiveContext(accounts, rows)

	var items []positionItemDTO
	var totalValue, totalHoldProfit, totalDayProfit float64

	for _, row := range rows {
		var lv api.LiveView
		if row.Category == "fund" {
			lv = positionSvc.RecomputePosition(row, nil)
		} else {
			lv = positionSvc.RecomputePosition(row, ctx)
		}

		item := positionItemDTO{
			Account:    row.Account,
			Category:   api.CategoryLabel(row.Category),
			Code:       row.Position.Code,
			Name:       row.Position.Name,
			Count:      row.Position.Count.F(),
			Cost:       row.Position.Cost.F(),
			Price:      lv.Price,
			Value:      lv.Value,
			HoldProfit: lv.HoldProfit,
			HoldRate:   lv.HoldRate,
			DayProfit:  lv.DayProfit,
			DayRate:    lv.DayRate,
		}

		items = append(items, item)
		totalValue += lv.Value
		totalHoldProfit += lv.HoldProfit
		if lv.DayOK {
			totalDayProfit += lv.DayProfit
		}
	}

	var resp positionsResp
	resp.Positions = items
	resp.Summary.Value = totalValue
	resp.Summary.HoldProfit = totalHoldProfit
	resp.Summary.DayProfit = totalDayProfit
	resp.Summary.Count = len(items)

	writeJSON(w, resp)
}

// --- 合并持仓（跨账户按代码汇总） ---

type mergedPositionDTO struct {
	Code       string   `json:"code"`
	Name       string   `json:"name"`
	Count      float64  `json:"count"`
	Cost       float64  `json:"cost"`
	Price      float64  `json:"price"`
	Value      float64  `json:"value"`
	HoldProfit float64  `json:"hold_profit"`
	HoldRate   float64  `json:"hold_rate"`
	DayProfit  float64  `json:"day_profit"`
	DayRate    float64  `json:"day_rate"`
	Accounts   []string `json:"accounts"`
}

type mergedPositionsResp struct {
	Positions []mergedPositionDTO `json:"positions"`
	Summary   struct {
		Value      float64 `json:"value"`
		HoldProfit float64 `json:"hold_profit"`
		DayProfit  float64 `json:"day_profit"`
		Count      int     `json:"count"`
	} `json:"summary"`
}

// handleMergedPositions GET /api/positions/merged?type= — 跨账户合并持仓
func (s *Server) handleMergedPositions(w http.ResponseWriter, r *http.Request) {
	typeFilter := r.URL.Query().Get("type")
	if typeFilter == "" {
		typeFilter = "all"
	}

	calc := service.NewCalculator()
	positionSvc := service.NewPositionService(s.client.WithContext(r.Context()), calc)

	accounts, err := positionSvc.LoadAccounts()
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	rows := positionSvc.CollectRows(accounts, "", typeFilter)
	if len(rows) == 0 {
		writeJSON(w, mergedPositionsResp{
			Positions: []mergedPositionDTO{},
			Summary: struct {
				Value      float64 `json:"value"`
				HoldProfit float64 `json:"hold_profit"`
				DayProfit  float64 `json:"day_profit"`
				Count      int     `json:"count"`
			}{},
		})
		return
	}

	merged := positionSvc.MergeByCode(rows)
	ctx := positionSvc.LoadLiveContext(accounts, rows)

	var items []mergedPositionDTO
	var totalValue, totalHoldProfit, totalDayProfit float64

	for _, m := range merged {
		// 查找该代码的实时行情
		quote, hasQuote := ctx.Quotes[m.Code]
		var price, dayProfit, dayRate float64
		var dayOK bool

		if hasQuote {
			price = quote.Price
			dayProfit = (quote.Price - quote.PrevClose) * m.Count
			if quote.PrevClose > 0 {
				dayRate = (quote.Price - quote.PrevClose) / quote.PrevClose * 100
			}
			dayOK = true
		} else {
			price = m.Price
		}

		value := price * m.Count
		holdRate := 0.0
		if m.Count > 0 && m.Cost > 0 {
			holdRate = (price - m.Cost) / m.Cost * 100
		}

		item := mergedPositionDTO{
			Code:       m.Code,
			Name:       m.Name,
			Count:      m.Count,
			Cost:       m.Cost,
			Price:      price,
			Value:      value,
			HoldProfit: m.HoldProfit,
			HoldRate:   holdRate,
			DayProfit:  dayProfit,
			DayRate:    dayRate,
			Accounts:   m.Accounts,
		}

		items = append(items, item)
		totalValue += value
		totalHoldProfit += m.HoldProfit
		if dayOK {
			totalDayProfit += dayProfit
		}
	}

	var resp mergedPositionsResp
	resp.Positions = items
	resp.Summary.Value = totalValue
	resp.Summary.HoldProfit = totalHoldProfit
	resp.Summary.DayProfit = totalDayProfit
	resp.Summary.Count = len(items)

	writeJSON(w, resp)
}
