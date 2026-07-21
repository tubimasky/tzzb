package service

import (
	"fmt"
	"sort"
	"time"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

// StockProfitService 股票时间维度盈亏业务逻辑（梯队 A）。
type StockProfitService struct {
	client *api.Client
}

// NewStockProfitService 创建股票盈亏服务。
func NewStockProfitService(client *api.Client) *StockProfitService {
	return &StockProfitService{client: client}
}

// LoadMonthly 加载某年各月股票盈亏。year 格式 YYYY，空则用当前年。
func (s *StockProfitService) LoadMonthly(accountFilter, year string) (*api.StockPLResult, string, error) {
	if year == "" {
		year = time.Now().Format("2006")
	}
	params, err := buildCalendarParams(s.client, accountFilter)
	if err != nil {
		return nil, year, err
	}
	res, err := s.client.GetStockMonthlyPL(params, year)
	if err != nil {
		return nil, year, err
	}
	s.sortByDate(res)
	return res, year, nil
}

// LoadRange 加载某区间每日股票盈亏 + 区间个股盈亏。
// start/end 空则默认本月初到今天。返回实际使用的区间（YYYYMMDD）。
func (s *StockProfitService) LoadRange(accountFilter, start, end string) (*api.StockPLResult, string, string, error) {
	now := time.Now()
	if end == "" {
		end = api.FormatCompactDate(now)
	}
	if start == "" {
		start = api.FormatCompactDate(now.AddDate(0, 0, -now.Day()+1))
	}
	params, err := buildCalendarParams(s.client, accountFilter)
	if err != nil {
		return nil, start, end, err
	}
	res, err := s.client.GetStockRangePL(params, start, end)
	if err != nil {
		return nil, start, end, err
	}
	s.sortByDate(res)
	return res, start, end, nil
}

// sortByDate 把盈亏点按日期升序排列（接口返回是乱序的）。
func (s *StockProfitService) sortByDate(res *api.StockPLResult) {
	if res == nil {
		return
	}
	sort.Slice(res.Points, func(i, j int) bool {
		return res.Points[i].Date < res.Points[j].Date
	})
}

// TopCodes 返回区间内盈亏绝对值最大的前 n 只标的（按盈亏降序）。
func (s *StockProfitService) TopCodes(res *api.StockPLResult, n int) []api.CalendarCodePL {
	codes := make([]api.CalendarCodePL, len(res.Codes))
	copy(codes, res.Codes)
	sort.Slice(codes, func(i, j int) bool {
		return codes[i].ProfitLoss > codes[j].ProfitLoss
	})
	if n > 0 && len(codes) > n {
		codes = codes[:n]
	}
	return codes
}

// FormatMonth 把接口的 YYYYMM 日期格式化为 YYYY-MM。
func FormatMonth(date string) string {
	if len(date) == 6 {
		return date[:4] + "-" + date[4:]
	}
	return date
}

// FormatDay 把接口的 YYYYMMDD 日期格式化为 YYYY-MM-DD。
func FormatDay(date string) string {
	if len(date) == 8 {
		return fmt.Sprintf("%s-%s-%s", date[:4], date[4:6], date[6:])
	}
	return date
}
