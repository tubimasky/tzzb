package formatter

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
)

// CalendarCell 是月历渲染需要的单元格数据（与 service.DayCell 解耦）。
type CalendarCell struct {
	Day     int
	TotalPL float64
	HasData bool
}

// weekdayHeaders 月历表头（周一到周日）。
var weekdayHeaders = []string{"一", "二", "三", "四", "五", "六", "日"}

// cellWidth 是每个日历格子的显示宽度。
const cellWidth = 12

// RenderMonthCalendar 渲染月历网格：每个格子上面是日期，下面是当日盈亏（红涨绿跌）。
// weeks 每个元素是一周 7 个 cell。
func RenderMonthCalendar(year, month int, weeks [][]CalendarCell) {
	color.New(color.Bold).Printf("\n%d 年 %d 月 盈亏日历\n\n", year, month)

	// 表头
	var head []string
	for _, w := range weekdayHeaders {
		head = append(head, centerText(w, cellWidth))
	}
	fmt.Println(strings.Join(head, " "))

	for _, week := range weeks {
		var dayLine []string
		var plLine []string
		for _, cell := range week {
			if cell.Day == 0 {
				dayLine = append(dayLine, centerText("", cellWidth))
				plLine = append(plLine, centerText("", cellWidth))
				continue
			}
			dayLine = append(dayLine, centerText(fmt.Sprintf("%d", cell.Day), cellWidth))

			if !cell.HasData {
				plLine = append(plLine, centerText("·", cellWidth))
			} else {
				plStr := fmt.Sprintf("%+.0f", cell.TotalPL)
				plLine = append(plLine, centerColorText(plStr, cellWidth, cell.TotalPL))
			}
		}
		fmt.Println(strings.Join(dayLine, " "))
		fmt.Println(strings.Join(plLine, " "))
		fmt.Println()
	}
}

// centerText 把文本居中到指定显示宽度（按中文占 2 宽度估算）。
func centerText(s string, width int) string {
	w := displayWidth(s)
	if w >= width {
		return s
	}
	left := (width - w) / 2
	right := width - w - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

// centerColorText 居中后再着色（着色会加不可见字符，所以先算宽度再上色）。
func centerColorText(s string, width int, v float64) string {
	padded := centerText(s, width)
	if NoColor {
		return padded
	}
	if v < 0 {
		return color.New(color.FgGreen).Sprint(padded)
	}
	return color.New(color.FgRed).Sprint(padded)
}

// displayWidth 估算字符串显示宽度：ASCII 占 1，其他（中文/全角）占 2。
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if r < 128 {
			w++
		} else {
			w += 2
		}
	}
	return w
}

// CodePLRow 是个股盈亏渲染行（与 api.CalendarCodePL 解耦）。
type CodePLRow struct {
	Code       string
	Name       string
	ProfitLoss float64
	Type       string
}

// RenderCodePL 渲染区间内个股盈亏板块。
func RenderCodePL(title string, codes []CodePLRow) {
	if len(codes) == 0 {
		return
	}
	color.New(color.Bold).Printf("\n%s\n", title)

	headers := []string{"代码", "名称", "盈亏", "类型"}
	var rows [][]string
	for _, c := range codes {
		typeLabel := "股票"
		if c.Type == "fund" {
			typeLabel = "基金"
		}
		rows = append(rows, []string{
			c.Code,
			c.Name,
			ColorNum(c.ProfitLoss),
			typeLabel,
		})
	}
	RenderTable(headers, rows, WithBorder(true), WithAutoWrap(false))
}
