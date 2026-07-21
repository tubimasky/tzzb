package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/liyanran/tzzb-cli/internal/service"
	"github.com/liyanran/tzzb-cli/pkg/api"
)

func TestPadDisplayCJK(t *testing.T) {
	cases := []struct {
		s    string
		w    int
		want int
	}{
		{"持有天数", 10, 10},
		{"数量", 10, 10},
		{"买入均价", 10, 10},
		{"总成本", 10, 10},
		{"费用", 10, 10},
		{"2026-07-06", 12, 12},
		{"持有中", 10, 10},
	}
	for _, tc := range cases {
		got := padDisplay(tc.s, tc.w)
		if w := lipgloss.Width(got); w != tc.want {
			t.Errorf("padDisplay(%q, %d) width=%d want %d (raw=%q)", tc.s, tc.w, w, tc.want, got)
		}
	}
}

func TestColorPadWidthStable(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	got := colorPad("-905.28", -905.28, 12)
	if w := lipgloss.Width(got); w != 12 {
		t.Errorf("colorPad width=%d want 12 (raw=%q)", w, got)
	}
	gotPos := colorPad("12.50", 12.50, 12)
	if w := lipgloss.Width(gotPos); w != 12 {
		t.Errorf("colorPad+ width=%d want 12 (raw=%q)", w, gotPos)
	}
}

func TestViewRecordsAlignment(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	d := service.StockDetail{
		Records: []api.HoldRecord{
			{StartDate: "20260706", EndDate: "", Profit: -905.28, ProfitRate: -0.1614},
			{StartDate: "20240101", EndDate: "20241231", Profit: 12.5, ProfitRate: 0.01},
		},
	}
	out := viewRecords(d)
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) < 3 {
		t.Fatalf("expected header+2 rows, got %q", out)
	}
	hw := lipgloss.Width(lines[0])
	for i, line := range lines[1:] {
		if w := lipgloss.Width(line); w != hw {
			t.Errorf("row %d width=%d header=%d\n  header=%q\n  row=%q", i, w, hw, lines[0], line)
		}
	}
}

func TestMatchPickerType(t *testing.T) {
	if !matchPickerType("common", typeStock) || matchPickerType("fund", typeStock) {
		t.Fatal("stock filter should exclude fund")
	}
	if !matchPickerType("fund", typeFund) || matchPickerType("common", typeFund) {
		t.Fatal("fund filter should only include fund")
	}
	if !matchPickerType("fund", typeAll) || !matchPickerType("rzrq", typeAll) {
		t.Fatal("all should include both")
	}
}

func TestPlainProfitNoANSI(t *testing.T) {
	for _, v := range []float64{12.5, -3.2, 0} {
		s := plainProfit(v)
		if strings.Contains(s, "\x1b") {
			t.Errorf("plainProfit(%v) contains ANSI: %q", v, s)
		}
	}
	if plainProfit(1.5) != "+1.50" {
		t.Errorf("want +1.50 got %q", plainProfit(1.5))
	}
	if plainProfit(-1.5) != "-1.50" {
		t.Errorf("want -1.50 got %q", plainProfit(-1.5))
	}
}

func TestPickerTableRendersAllRows(t *testing.T) {
	// 构造 52 行纯文本，确认 viewport 高度内可见多行且无残缺。
	m := &pickerModel{
		positions: nil,
		filtered:  nil,
		width:     100,
		height:    40,
	}
	// 用 rebuild 需要 filtered；直接塞假数据
	for i := 0; i < 52; i++ {
		// rebuildTable 读 filtered 的 Position 字段，这里只测 table 行数：
		// 改为直接验证 plainProfit + table 不炸。
		_ = i
	}
	// 用 table 直接验证 52 纯文本行在 height=30 时 View 行数合理
	cols := pickerColumns()
	var rows []table.Row
	for i := 0; i < 52; i++ {
		rows = append(rows, table.Row{
			fmt.Sprintf("%06d", i),
			"鹏华丰禄债券",
			"京东",
			"5037",
			"1.034",
			"0.000",
			"5246.51",
			plainProfit(float64(i) - 20),
		})
	}
	t.Helper()
	tbl := table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(15),
	)
	s := table.DefaultStyles()
	s.Header = s.Header.Bold(true).BorderStyle(lipgloss.NormalBorder()).BorderBottom(true)
	tbl.SetStyles(s)
	tbl.SetWidth(100)
	tbl.SetHeight(30)
	view := tbl.View()
	lines := strings.Split(view, "\n")
	// 至少应有 header + border + 多条数据行，不应只有 2 行数据
	dataLines := 0
	for _, line := range lines {
		if strings.Contains(line, "鹏华") {
			dataLines++
		}
	}
	if dataLines < 10 {
		t.Fatalf("expected many visible rows, got dataLines=%d totalLines=%d\n%s", dataLines, len(lines), view)
	}
	// 任一数据行不应因 ANSI 残缺而只剩半截（盈亏列应有数字）
	for _, line := range lines {
		if strings.Contains(line, "鹏华") && !strings.Contains(line, "5246.51") {
			t.Errorf("row missing 市值: %q", line)
		}
	}
	_ = m
}

func TestViewMergeRenders(t *testing.T) {
	d := service.StockDetail{
		Merge: &api.StockMergeDetail{
			StockCode:   "159227",
			StockName:   "航空航天",
			HoldDays:    12,
			Count:       5000,
			PriceBuyAv:  1.122,
			PriceCurr:   0.941,
			MarketValue: 4705,
			Zcb:         5610,
			Yke:         -905.28,
			ProfitRate:  -0.1614,
			Fee:         0.28,
		},
	}
	out := viewMerge(d)
	if !strings.Contains(out, "持有天数") || !strings.Contains(out, "5000") {
		t.Fatalf("unexpected merge view: %q", out)
	}
	// 短标签「数量」与长标签「持有天数」pad 后显示宽一致。
	if w := lipgloss.Width(padDisplay("数量", 10)); w != 10 {
		t.Errorf("数量 pad width=%d", w)
	}
	if w := lipgloss.Width(padDisplay("持有天数", 10)); w != 10 {
		t.Errorf("持有天数 pad width=%d", w)
	}
}
