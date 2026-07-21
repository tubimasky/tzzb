package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/internal/formatter"
	"github.com/liyanran/tzzb-cli/internal/service"
	"github.com/liyanran/tzzb-cli/pkg/api"
)

var (
	calendarAccount  string
	calendarMonth    string
	calendarYear     string
	calendarTop      int
	calendarCategory string
)

// validateCalendarCategory 校验 --category 取值。
func validateCalendarCategory(cat string) error {
	switch cat {
	case "total", "stock", "fund":
		return nil
	default:
		return fmt.Errorf("category 只能是 total/stock/fund，得到: %s", cat)
	}
}

// pickPL 按类别取当日/当月盈亏值。
func pickPL(total, stock, fund float64, cat string) float64 {
	switch cat {
	case "stock":
		return stock
	case "fund":
		return fund
	default:
		return total
	}
}

// categoryLabel 返回类别的中文标签。
func categoryLabel(cat string) string {
	switch cat {
	case "stock":
		return "股票"
	case "fund":
		return "基金"
	default:
		return "总盈亏"
	}
}

func getCalendarService() *service.CalendarService {
	return service.NewCalendarService(apiClient)
}

var calendarCmd = &cobra.Command{
	Use:   "calendar",
	Short: "盈亏日历",
	Long:  `按月历视图展示每日盈亏，或按年展示每月盈亏（红涨绿跌）`,
}

var calendarMonthCmd = &cobra.Command{
	Use:   "month",
	Short: "月历视图（某月每日盈亏）",
	Run: func(cmd *cobra.Command, args []string) {
		if err := validateCalendarCategory(calendarCategory); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}

		svc := getCalendarService()
		res, month, err := svc.LoadMonth(calendarAccount, calendarMonth)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}

		if outputJSON {
			printCalendarJSON(res)
			return
		}

		grid, err := svc.BuildMonthGrid(res, month)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}

		var weeks [][]formatter.CalendarCell
		for _, week := range grid.Weeks {
			var row []formatter.CalendarCell
			for _, c := range week {
				row = append(row, formatter.CalendarCell{
					Day:     c.Day,
					Value:   pickPL(c.TotalPL, c.StockPL, c.FundPL, calendarCategory),
					HasData: c.HasData,
				})
			}
			weeks = append(weeks, row)
		}
		formatter.RenderMonthCalendar(grid.Year, grid.Month, weeks, categoryLabel(calendarCategory))

		printCalendarSummary(res, calendarCategory)

		if calendarTop != 0 {
			renderTopCodes(svc, res, calendarCategory)
		}
	},
}

var calendarYearCmd = &cobra.Command{
	Use:   "year",
	Short: "年视图（某年每月盈亏）",
	Run: func(cmd *cobra.Command, args []string) {
		if err := validateCalendarCategory(calendarCategory); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}

		svc := getCalendarService()
		res, _, err := svc.LoadYear(calendarAccount, calendarYear)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}

		if outputJSON {
			printCalendarJSON(res)
			return
		}

		renderYearTable(res, calendarCategory)
		printCalendarSummary(res, calendarCategory)

		if calendarTop != 0 {
			renderTopCodes(svc, res, calendarCategory)
		}
	},
}

func printCalendarSummary(res *api.CalendarResult, cat string) {
	if cat == "stock" || cat == "fund" {
		fmt.Printf("\n区间合计  %s: %s\n",
			categoryLabel(cat),
			formatter.ColorNum(pickPL(res.TotalPL, res.StockPL, res.FundPL, cat)),
		)
		return
	}
	fmt.Printf("\n区间合计  总盈亏: %s | 股票: %s | 基金: %s",
		formatter.ColorNum(res.TotalPL),
		formatter.ColorNum(res.StockPL),
		formatter.ColorNum(res.FundPL),
	)
	if res.OtherPL != 0 {
		fmt.Printf(" | 其他: %s", formatter.ColorNum(res.OtherPL))
	}
	fmt.Println()
}

func renderTopCodes(svc *service.CalendarService, res *api.CalendarResult, cat string) {
	codes := svc.TopCodes(res, calendarTop, cat)
	var rows []formatter.CodePLRow
	for _, c := range codes {
		rows = append(rows, formatter.CodePLRow{
			Code:       c.Code,
			Name:       c.Name,
			ProfitLoss: c.ProfitLoss,
			Type:       c.Type,
		})
	}
	formatter.RenderCodePL(fmt.Sprintf("区间个股盈亏 Top %d", calendarTop), rows)
}

func renderYearTable(res *api.CalendarResult, cat string) {
	col := formatter.ColorNum

	points := make([]api.CalendarPoint, len(res.Points))
	copy(points, res.Points)
	sort.Slice(points, func(i, j int) bool {
		return points[i].Date < points[j].Date
	})

	monthLabel := func(date string) string {
		if len(date) == 6 {
			return date[:4] + "-" + date[4:]
		}
		return date
	}

	// stock/fund：只展示月份 + 选中类别两列。
	if cat == "stock" || cat == "fund" {
		headers := []string{"月份", categoryLabel(cat)}
		var rows [][]string
		for _, p := range points {
			rows = append(rows, []string{
				monthLabel(p.Date),
				col(pickPL(p.TotalPL, p.StockPL, p.FundPL, cat)),
			})
		}
		formatter.RenderTable(headers, rows, formatter.WithBorder(true))
		return
	}

	headers := []string{"月份", "总盈亏", "股票", "基金"}
	var rows [][]string
	for _, p := range points {
		rows = append(rows, []string{
			monthLabel(p.Date),
			col(p.TotalPL),
			col(p.StockPL),
			col(p.FundPL),
		})
	}
	formatter.RenderTable(headers, rows, formatter.WithBorder(true))
}

func printCalendarJSON(res *api.CalendarResult) {
	formatter.PrintJSON(map[string]any{
		"total_pl": res.TotalPL,
		"stock_pl": res.StockPL,
		"fund_pl":  res.FundPL,
		"other_pl": res.OtherPL,
		"points":   res.Points,
		"codes":    res.Codes,
	})
}

func init() {
	calendarCmd.AddCommand(calendarMonthCmd)
	calendarCmd.AddCommand(calendarYearCmd)

	calendarMonthCmd.Flags().StringVarP(&calendarAccount, "account", "a", "", "按账户名过滤（模糊匹配），默认全部账户汇总")
	calendarMonthCmd.Flags().StringVarP(&calendarMonth, "month", "m", "", "月份 YYYYMM，默认当前月")
	calendarMonthCmd.Flags().IntVarP(&calendarTop, "top", "t", 10, "显示区间个股盈亏 Top N，0 表示不显示")
	calendarMonthCmd.Flags().StringVarP(&calendarCategory, "category", "c", "total", "展示类别: total(总)/stock(股票)/fund(基金)")

	calendarYearCmd.Flags().StringVarP(&calendarAccount, "account", "a", "", "按账户名过滤（模糊匹配），默认全部账户汇总")
	calendarYearCmd.Flags().StringVarP(&calendarYear, "year", "y", "", "年份 YYYY，默认当前年")
	calendarYearCmd.Flags().IntVarP(&calendarTop, "top", "t", 10, "显示区间个股盈亏 Top N，0 表示不显示")
	calendarYearCmd.Flags().StringVarP(&calendarCategory, "category", "c", "total", "展示类别: total(总)/stock(股票)/fund(基金)")
}
