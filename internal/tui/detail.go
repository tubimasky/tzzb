package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/liyanran/tzzb-cli/internal/service"
	"github.com/liyanran/tzzb-cli/pkg/api"
)

// 单只股票钻取明细的交互式界面（bubbletea）。数据源与直出一致（service.StockDetail），
// 这里只换展示层：按账户 + 四块 tab 浏览，历史成交用可滚动表格。
//
// 可单独 RunDetail 启动，也可作为 appModel 子页嵌入（canBack=true 时 esc 返回列表）。

// 红涨绿跌（对齐 formatter 口径）。
var (
	styleUp     = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // 红
	styleDown   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // 绿
	styleTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	styleTabOn  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6")).Padding(0, 1)
	styleTabOff = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Padding(0, 1)
	styleHelp   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleLabel  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleWarn   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
)

var tabNames = []string{"合并明细", "建仓记录", "历史成交", "清仓汇总"}

const (
	tabMerge = iota
	tabRecords
	tabTrades
	tabClear
)

// chromeLines 标题 + tab + 帮助等固定占位行数，用于计算成交表可用高度。
const chromeLines = 8

// detailBackMsg 嵌入模式下用户按 esc 请求返回列表。
type detailBackMsg struct{}

type detailModel struct {
	details    []service.StockDetail
	accountIdx int
	tabIdx     int
	tradeTbl   table.Model
	width      int
	height     int
	canBack    bool // true：esc 返回列表；false：esc/q 退出程序
	quitting   bool
}

// RunDetail 启动明细 TUI（独立入口，阻塞到用户退出）。
func RunDetail(details []service.StockDetail) error {
	m := newDetailModel(details, false)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func newDetailModel(details []service.StockDetail, canBack bool) detailModel {
	m := detailModel{details: details, canBack: canBack}
	m.tradeTbl = newTradeTable(m.cur())
	return m
}

// cur 返回当前账户的明细。
func (m detailModel) cur() service.StockDetail {
	if m.accountIdx < 0 || m.accountIdx >= len(m.details) {
		return service.StockDetail{}
	}
	return m.details[m.accountIdx]
}

// newTradeTable 为某账户的历史成交构造滚动表格。
func newTradeTable(d service.StockDetail) table.Model {
	cols := []table.Column{
		{Title: "成交日", Width: 12},
		{Title: "方向", Width: 6},
		{Title: "价格", Width: 10},
		{Title: "数量", Width: 8},
		{Title: "金额", Width: 12},
		{Title: "费用", Width: 8},
	}
	var rows []table.Row
	for _, t := range d.Trades {
		rows = append(rows, table.Row{
			fmtDate(t.TransDate),
			opLabel(t.Op),
			priceStr(t.Price),
			fmt.Sprintf("%.0f", t.Count),
			fmt.Sprintf("%.2f", t.Amount),
			fmt.Sprintf("%.2f", t.Fee),
		})
	}
	tbl := table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(12),
	)
	st := table.DefaultStyles()
	st.Header = st.Header.Bold(true).Foreground(lipgloss.Color("6"))
	st.Selected = st.Selected.Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6"))
	tbl.SetStyles(st)
	return tbl
}

// applyTradeTableSize 按终端尺寸调整成交表 viewport。
func (m *detailModel) applyTradeTableSize() {
	if m.height > 0 {
		h := m.height - chromeLines
		if h < 5 {
			h = 5
		}
		m.tradeTbl.SetHeight(h)
	}
	if m.width > 0 {
		m.tradeTbl.SetWidth(m.width)
	}
}

// rebuildTradeTable 切换账户后重建成交表并恢复尺寸。
func (m *detailModel) rebuildTradeTable() {
	m.tradeTbl = newTradeTable(m.cur())
	m.applyTradeTableSize()
}

// setSize 由父 model 在 WindowSizeMsg 时调用。
func (m *detailModel) setSize(w, h int) {
	m.width = w
	m.height = h
	m.applyTradeTableSize()
}

func (m detailModel) Init() tea.Cmd { return nil }

func (m detailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m2, cmd := m.update(msg)
	return m2, cmd
}

// update 返回具体类型，便于 appModel 嵌入。
func (m detailModel) update(msg tea.Msg) (detailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.setSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "esc", "backspace":
			if m.canBack {
				return m, func() tea.Msg { return detailBackMsg{} }
			}
			m.quitting = true
			return m, tea.Quit
		case "q":
			// 嵌入模式：q 也回列表（与 esc 一致，避免误退出整个程序）；
			// 独立模式：q 退出。
			if m.canBack {
				return m, func() tea.Msg { return detailBackMsg{} }
			}
			m.quitting = true
			return m, tea.Quit
		case "tab", "right", "l":
			m.tabIdx = (m.tabIdx + 1) % len(tabNames)
			return m, nil
		case "shift+tab", "left", "h":
			m.tabIdx = (m.tabIdx - 1 + len(tabNames)) % len(tabNames)
			return m, nil
		case "]", "n":
			if len(m.details) > 1 {
				m.accountIdx = (m.accountIdx + 1) % len(m.details)
				m.rebuildTradeTable()
			}
			return m, nil
		case "[", "p":
			if len(m.details) > 1 {
				m.accountIdx = (m.accountIdx - 1 + len(m.details)) % len(m.details)
				m.rebuildTradeTable()
			}
			return m, nil
		}
	}

	// 历史成交 tab：↑↓/翻页交给表格。
	if m.tabIdx == tabTrades {
		var cmd tea.Cmd
		m.tradeTbl, cmd = m.tradeTbl.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m detailModel) View() string {
	if m.quitting {
		return ""
	}
	d := m.cur()
	var b strings.Builder

	// 标题
	title := d.Coord.StockCode
	if d.Merge != nil && d.Merge.StockName != "" {
		title = fmt.Sprintf("%s %s", d.Merge.StockName, d.Merge.StockCode)
	}
	head := styleTitle.Render("═ " + title + " ═")
	if len(m.details) > 1 {
		head += styleLabel.Render(fmt.Sprintf("  账户 %d/%d · %s", m.accountIdx+1, len(m.details), d.Coord.AccountName))
	} else {
		head += styleLabel.Render("  @ " + d.Coord.AccountName)
	}
	b.WriteString(head + "\n\n")

	// tab 栏
	var tabs []string
	for i, name := range tabNames {
		label := name
		if i == tabClear && (d.Clear == nil || !d.Clear.HasData()) {
			label = name + "(无)"
		}
		if i == m.tabIdx {
			tabs = append(tabs, styleTabOn.Render(label))
		} else {
			tabs = append(tabs, styleTabOff.Render(label))
		}
	}
	b.WriteString(strings.Join(tabs, " ") + "\n\n")

	// 主区
	switch m.tabIdx {
	case tabMerge:
		b.WriteString(viewMerge(d))
	case tabRecords:
		b.WriteString(viewRecords(d))
	case tabTrades:
		if len(d.Trades) == 0 {
			b.WriteString(styleLabel.Render("无历史成交"))
		} else {
			b.WriteString(m.tradeTbl.View())
		}
	case tabClear:
		b.WriteString(viewClear(d))
	}

	// help
	b.WriteString("\n\n")
	var keys string
	if m.canBack {
		keys = "tab/←→ 切换 · ↑↓ 滚动 · esc/q 返回列表"
		if len(m.details) > 1 {
			keys = "tab/←→ 切换 · [ ] 切账户 · ↑↓ 滚动 · esc/q 返回列表"
		}
	} else {
		keys = "tab/←→ 切换 · ↑↓ 滚动 · q 退出"
		if len(m.details) > 1 {
			keys = "tab/←→ 切换 · [ ] 切账户 · ↑↓ 滚动 · q 退出"
		}
	}
	b.WriteString(styleHelp.Render(keys))
	return b.String()
}

func viewMerge(d service.StockDetail) string {
	m := d.Merge
	if m == nil {
		return styleLabel.Render("无合并明细")
	}
	var b strings.Builder
	row := func(label, val string) {
		b.WriteString(styleLabel.Render(padDisplay(label, 10)) + val + "\n")
	}
	row("持有天数", fmt.Sprintf("%d 天", m.HoldDays))
	row("数量", fmt.Sprintf("%.0f", m.Count))
	row("买入均价", priceStr(m.PriceBuyAv))
	row("现价", priceStr(m.PriceCurr))
	row("市值", fmt.Sprintf("%.2f", m.MarketValue))
	row("总成本", fmt.Sprintf("%.2f", m.Zcb))
	row("浮动盈亏", colorNum(m.Yke))
	row("收益率", colorPctRaw(m.ProfitRate))
	row("费用", fmt.Sprintf("%.2f", m.Fee))
	if m.ClearFlag == "1" {
		b.WriteString(styleWarn.Render("（该持仓已清仓）") + "\n")
	}
	return b.String()
}

func viewRecords(d service.StockDetail) string {
	if len(d.Records) == 0 {
		return styleLabel.Render("无建仓记录")
	}
	const (
		wDate   = 12
		wEnd    = 10
		wProfit = 12
		wRate   = 10
	)
	var b strings.Builder
	header := padDisplay("建仓日", wDate) + " " +
		padDisplay("结束日", wEnd) + " " +
		padDisplay("分段收益", wProfit) + " " +
		padDisplay("收益率", wRate)
	b.WriteString(styleLabel.Render(header) + "\n")
	for _, r := range d.Records {
		end := r.EndDate
		if end == "" {
			end = "持有中"
		} else {
			end = fmtDate(end)
		}
		// 先 pad 再着色，避免 ANSI 打乱多列对齐。
		line := padDisplay(fmtDate(r.StartDate), wDate) + " " +
			padDisplay(end, wEnd) + " " +
			colorPad(fmt.Sprintf("%.2f", r.Profit), r.Profit, wProfit) + " " +
			colorPad(fmt.Sprintf("%.2f%%", r.ProfitRate*100), r.ProfitRate, wRate)
		b.WriteString(line + "\n")
	}
	return b.String()
}

func viewClear(d service.StockDetail) string {
	c := d.Clear
	if c == nil || !c.HasData() {
		return styleLabel.Render("未清仓，无清仓汇总")
	}
	var b strings.Builder
	row := func(label, val string) {
		b.WriteString(styleLabel.Render(padDisplay(label, 10)) + val + "\n")
	}
	row("持有天数", fmt.Sprintf("%d 天", c.HoldDays))
	row("总买入", fmt.Sprintf("%.2f", c.TotalBuy))
	row("总卖出", fmt.Sprintf("%.2f", c.TotalSell))
	row("费用", fmt.Sprintf("%.2f", c.Fee))
	row("清仓盈亏", colorNum(c.TotalProfit))
	row("清仓收益率", colorPctRaw(c.TotalProfitRate))
	return b.String()
}

// —— 展示辅助（与 cmd 层口径一致，TUI 用 lipgloss 着色）——

// padDisplay 按终端显示列宽右补空格（中文按 2 列，ANSI 不计宽）。
func padDisplay(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// colorPad 先按显示宽度 pad 再着色，保证多列对齐。
func colorPad(plain string, v float64, width int) string {
	padded := padDisplay(plain, width)
	if v < 0 {
		return styleDown.Render(padded)
	}
	return styleUp.Render(padded)
}

func colorNum(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	if v < 0 {
		return styleDown.Render(s)
	}
	return styleUp.Render(s)
}

func colorPctRaw(v float64) string {
	s := fmt.Sprintf("%.2f%%", v*100)
	if v < 0 {
		return styleDown.Render(s)
	}
	return styleUp.Render(s)
}

func priceStr(v float64) string {
	if v <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.3f", v)
}

func opLabel(op string) string {
	switch op {
	case api.OpBuy:
		return "买入"
	case api.OpSell:
		return "卖出"
	default:
		return op
	}
}

func fmtDate(s string) string {
	if len(s) != 8 {
		return s
	}
	return fmt.Sprintf("%s-%s-%s", s[:4], s[4:6], s[6:8])
}
