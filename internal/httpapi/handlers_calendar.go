package httpapi

import (
	"net/http"

	"github.com/liyanran/tzzb-cli/internal/service"
	"github.com/liyanran/tzzb-cli/pkg/api"
)

// DayCellDTO 是月历中的一天。
type DayCellDTO struct {
	Day     int     `json:"day"` // 1..31；0 表示占位空格
	Date    string  `json:"date"`
	TotalPL float64 `json:"total_pl"`
	StockPL float64 `json:"stock_pl"`
	FundPL  float64 `json:"fund_pl"`
	HasData bool    `json:"has_data"`
}

// CodePLDTO 是区间内单只标的盈亏。
type CodePLDTO struct {
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	ProfitLoss float64 `json:"profit_loss"`
	Type       string  `json:"type"`
}

// CalendarMonthResponse 月历视图响应。
type CalendarMonthResponse struct {
	Year     int            `json:"year"`
	Month    int            `json:"month"`
	Weeks    [][]DayCellDTO `json:"weeks"`
	TopCodes []CodePLDTO    `json:"top_codes"`
	TotalPL  float64        `json:"total_pl"`
	StockPL  float64        `json:"stock_pl"`
	FundPL   float64        `json:"fund_pl"`
}

// handleCalendarMonth GET /api/calendar/month?month=YYYYMM&account=&cat=total|stock|fund
func (s *Server) handleCalendarMonth(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	account := q.Get("account")
	month := q.Get("month")
	cat := categoryParam(q.Get("cat"), q.Get("category"))

	svc := service.NewCalendarService(s.client.WithContext(r.Context()))
	res, usedMonth, err := svc.LoadMonth(account, month)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	grid, err := svc.BuildMonthGrid(res, usedMonth)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := CalendarMonthResponse{
		Year:     grid.Year,
		Month:    grid.Month,
		Weeks:    make([][]DayCellDTO, 0, len(grid.Weeks)),
		TopCodes: toCodePLDTOs(svc.TopCodes(res, 10, cat)),
		TotalPL:  res.TotalPL,
		StockPL:  res.StockPL,
		FundPL:   res.FundPL,
	}
	for _, week := range grid.Weeks {
		row := make([]DayCellDTO, 0, len(week))
		for _, c := range week {
			row = append(row, DayCellDTO{
				Day:     c.Day,
				Date:    c.Date,
				TotalPL: c.TotalPL,
				StockPL: c.StockPL,
				FundPL:  c.FundPL,
				HasData: c.HasData,
			})
		}
		resp.Weeks = append(resp.Weeks, row)
	}

	writeJSON(w, resp)
}

// MonthPLDTO 是年视图里的某个月盈亏。
type MonthPLDTO struct {
	Date    string  `json:"date"` // 202607
	TotalPL float64 `json:"total_pl"`
	StockPL float64 `json:"stock_pl"`
	FundPL  float64 `json:"fund_pl"`
}

// CalendarYearResponse 年视图响应。
type CalendarYearResponse struct {
	Year     string       `json:"year"`
	Months   []MonthPLDTO `json:"months"`
	TopCodes []CodePLDTO  `json:"top_codes"`
	TotalPL  float64      `json:"total_pl"`
	StockPL  float64      `json:"stock_pl"`
	FundPL   float64      `json:"fund_pl"`
}

// handleCalendarYear GET /api/calendar/year?year=YYYY&account=&cat=
func (s *Server) handleCalendarYear(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	account := q.Get("account")
	year := q.Get("year")
	cat := categoryParam(q.Get("cat"), q.Get("category"))

	svc := service.NewCalendarService(s.client.WithContext(r.Context()))
	res, usedYear, err := svc.LoadYear(account, year)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	resp := CalendarYearResponse{
		Year:     usedYear,
		Months:   make([]MonthPLDTO, 0, len(res.Points)),
		TopCodes: toCodePLDTOs(svc.TopCodes(res, 10, cat)),
		TotalPL:  res.TotalPL,
		StockPL:  res.StockPL,
		FundPL:   res.FundPL,
	}
	for _, p := range res.Points {
		resp.Months = append(resp.Months, MonthPLDTO{
			Date:    p.Date,
			TotalPL: p.TotalPL,
			StockPL: p.StockPL,
			FundPL:  p.FundPL,
		})
	}

	writeJSON(w, resp)
}

// categoryParam 归一化展示类别参数。历史上前端发过 category=，后端只读 cat=，
// 导致筛选静默失效；这里两者都接受，cat 优先，空值回落 total。
func categoryParam(cat, category string) string {
	if cat != "" {
		return cat
	}
	if category != "" {
		return category
	}
	return "total"
}

// toCodePLDTOs 把 service 的 CalendarCodePL 列表转成 DTO。
func toCodePLDTOs(codes []api.CalendarCodePL) []CodePLDTO {
	out := make([]CodePLDTO, 0, len(codes))
	for _, c := range codes {
		out = append(out, CodePLDTO{
			Code:       c.Code,
			Name:       c.Name,
			ProfitLoss: c.ProfitLoss,
			Type:       c.Type,
		})
	}
	return out
}
