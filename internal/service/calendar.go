package service

import (
	"fmt"
	"sort"
	"time"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

// CalendarService 盈亏日历业务逻辑。
type CalendarService struct {
	client *api.Client
}

// NewCalendarService 创建盈亏日历服务。
func NewCalendarService(client *api.Client) *CalendarService {
	return &CalendarService{client: client}
}

// LoadMonth 加载某月每日盈亏。month 格式 YYYYMM，空则用当前月。
func (s *CalendarService) LoadMonth(accountFilter, month string) (*api.CalendarResult, string, error) {
	if month == "" {
		month = time.Now().Format("200601")
	}
	params, err := buildCalendarParams(s.client, accountFilter)
	if err != nil {
		return nil, month, err
	}
	res, err := s.client.GetCalendarDaily(params, month)
	return res, month, err
}

// LoadYear 加载某年每月盈亏。year 格式 YYYY，空则用当前年。
func (s *CalendarService) LoadYear(accountFilter, year string) (*api.CalendarResult, string, error) {
	if year == "" {
		year = time.Now().Format("2006")
	}
	params, err := buildCalendarParams(s.client, accountFilter)
	if err != nil {
		return nil, year, err
	}
	res, err := s.client.GetCalendarMonthly(params, year)
	return res, year, err
}

// DayCell 是月历中的一天。
type DayCell struct {
	Day     int     // 1..31；0 表示占位空格
	Date    string  // 20260720
	TotalPL float64 // 当日合计盈亏
	StockPL float64
	FundPL  float64
	HasData bool // 是否有盈亏数据（区分 0 盈亏和无数据）
}

// MonthGrid 是按周排布的月历（每行 7 天，周一到周日）。
type MonthGrid struct {
	Year  int
	Month int
	Weeks [][]DayCell // 每个元素是一周 7 个 cell
}

// BuildMonthGrid 把每日盈亏点排成月历网格。month 格式 YYYYMM。
func (s *CalendarService) BuildMonthGrid(res *api.CalendarResult, month string) (*MonthGrid, error) {
	t, err := time.Parse("200601", month)
	if err != nil {
		return nil, fmt.Errorf("month 格式应为 YYYYMM，得到: %s", month)
	}
	year, mon := t.Year(), int(t.Month())

	// 按日期索引盈亏
	plByDate := map[string]api.CalendarPoint{}
	for _, p := range res.Points {
		plByDate[p.Date] = p
	}

	first := time.Date(year, time.Month(mon), 1, 0, 0, 0, 0, time.UTC)
	daysInMonth := first.AddDate(0, 1, -1).Day()

	// 计算 1 号是周几，周一=0 ... 周日=6
	weekdayOffset := (int(first.Weekday()) + 6) % 7

	grid := &MonthGrid{Year: year, Month: mon}
	var week []DayCell

	// 月初填充空格
	for i := 0; i < weekdayOffset; i++ {
		week = append(week, DayCell{})
	}

	for day := 1; day <= daysInMonth; day++ {
		date := fmt.Sprintf("%04d%02d%02d", year, mon, day)
		cell := DayCell{Day: day, Date: date}
		if p, ok := plByDate[date]; ok {
			cell.TotalPL = p.TotalPL
			cell.StockPL = p.StockPL
			cell.FundPL = p.FundPL
			// total、stock、fund 任一非零即视为有数据
			cell.HasData = p.TotalPL != 0 || p.StockPL != 0 || p.FundPL != 0
		}
		week = append(week, cell)

		if len(week) == 7 {
			grid.Weeks = append(grid.Weeks, week)
			week = nil
		}
	}

	// 月末补齐最后一周
	if len(week) > 0 {
		for len(week) < 7 {
			week = append(week, DayCell{})
		}
		grid.Weeks = append(grid.Weeks, week)
	}

	return grid, nil
}

// TopCodes 返回区间内盈亏绝对值最大的前 n 只标的（按盈亏降序）。
// cat 为 stock 只留股票、fund 只留基金、其它（total）不过滤。
func (s *CalendarService) TopCodes(res *api.CalendarResult, n int, cat string) []api.CalendarCodePL {
	var codes []api.CalendarCodePL
	for _, c := range res.Codes {
		switch cat {
		case "stock":
			if c.Type == "fund" {
				continue
			}
		case "fund":
			if c.Type != "fund" {
				continue
			}
		}
		codes = append(codes, c)
	}
	sort.Slice(codes, func(i, j int) bool {
		return codes[i].ProfitLoss > codes[j].ProfitLoss
	})
	if n > 0 && len(codes) > n {
		codes = codes[:n]
	}
	return codes
}
