package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/liyanran/tzzb-cli/internal/service"
)

// 类型筛选：与 position 命令 stock/fund 口径一致（fund 账户 vs 非 fund）。
const (
	typeStock = iota
	typeFund
	typeAll
)

var typeLabels = []string{"股票", "基金", "全部"}

// app 页面：同一 tea.Program 内切换，避免退出/再进 AltScreen 闪屏。
const (
	pagePicker = iota
	pageLoading
	pageDetail
)

// 表头 + 边框等固定占用（不含数据行）。
const (
	pickerChromeLines = 9 // 标题2 + 类型1 + 搜索2 + 帮助2 + 边距2
	pickerMinTableH   = 6
)

// —— 异步加载明细 ——

type detailLoadedMsg struct {
	seq     int
	code    string
	details []service.StockDetail
}

type detailLoadErrMsg struct {
	seq int
	err error
}

func loadDetailCmd(svc *service.DetailService, seq int, code, accountFilter, from, to string) tea.Cmd {
	return func() tea.Msg {
		details, err := svc.LoadDetail(code, accountFilter, from, to)
		if err != nil {
			return detailLoadErrMsg{seq: seq, err: err}
		}
		return detailLoadedMsg{seq: seq, code: code, details: details}
	}
}

// appModel 持仓选择器 + 明细的统一根 model（单 Program）。
type appModel struct {
	page       int
	picker     pickerModel
	detail     detailModel
	detailSvc  *service.DetailService
	accountFlt string
	fromDate   string
	toDate     string
	loadCode   string // 正在加载的代码，用于 loading 文案
	loadSeq    int    // 递增序号，丢弃过期的异步加载结果
	width      int
	height     int
}

// pickerModel 持仓列表子状态（不单独 Run）。
type pickerModel struct {
	table       table.Model
	searchInput textinput.Model
	positions   []service.PositionRow
	filtered    []service.PositionRow
	typeIdx     int // typeStock / typeFund / typeAll
	width       int
	height      int
	searching   bool
	status      string // 临时提示（如基金不可钻取、加载失败）
}

// RunPositionPicker 启动「列表 ↔ 明细」统一 TUI，阻塞到用户退出。
func RunPositionPicker(posSvc *service.PositionService, detailSvc *service.DetailService, accountFilter, fromDate, toDate string) error {
	accounts, err := posSvc.LoadAccounts()
	if err != nil {
		return fmt.Errorf("加载账户失败: %w", err)
	}

	rows := posSvc.CollectRows(accounts, accountFilter, "all")
	if len(rows) == 0 {
		return fmt.Errorf("无持仓数据")
	}

	ti := textinput.New()
	ti.Placeholder = "搜索代码/名称..."
	ti.CharLimit = 20
	ti.Width = 30

	pk := pickerModel{
		positions:   rows,
		searchInput: ti,
		searching:   false,
		typeIdx:     typeStock, // 默认股票（detail 仅支持股票账户）
	}
	pk.applyFilter()

	m := appModel{
		page:       pagePicker,
		picker:     pk,
		detailSvc:  detailSvc,
		accountFlt: accountFilter,
		fromDate:   fromDate,
		toDate:     toDate,
	}

	p := tea.NewProgram(&m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func (m *appModel) Init() tea.Cmd { return nil }

func (m *appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 全局尺寸
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
		m.height = ws.Height
		m.picker.width = ws.Width
		m.picker.height = ws.Height
		m.picker.applyTableSize()
		m.detail.setSize(ws.Width, ws.Height)
		return m, nil
	}

	switch m.page {
	case pageLoading:
		return m.updateLoading(msg)
	case pageDetail:
		return m.updateDetail(msg)
	default:
		return m.updatePicker(msg)
	}
}

func (m *appModel) updateLoading(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case detailLoadedMsg:
		if msg.seq != m.loadSeq {
			return m, nil // 过期请求
		}
		m.detail = newDetailModel(msg.details, true)
		m.detail.setSize(m.width, m.height)
		m.page = pageDetail
		return m, nil
	case detailLoadErrMsg:
		if msg.seq != m.loadSeq {
			return m, nil
		}
		m.picker.status = fmt.Sprintf("加载失败: %v", msg.err)
		m.page = pagePicker
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			// 取消：递增 seq，使在途请求结果失效
			m.loadSeq++
			m.page = pagePicker
			m.picker.status = "已取消加载"
			m.loadCode = ""
			return m, nil
		}
	}
	return m, nil
}

func (m *appModel) updateDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 返回列表
	if _, ok := msg.(detailBackMsg); ok {
		m.page = pagePicker
		m.picker.status = ""
		return m, nil
	}
	// 过期/迟到的加载结果一律忽略
	if _, ok := msg.(detailLoadedMsg); ok {
		return m, nil
	}
	if _, ok := msg.(detailLoadErrMsg); ok {
		return m, nil
	}

	var cmd tea.Cmd
	m.detail, cmd = m.detail.update(msg)
	// detail 独立退出（ctrl+c）
	if m.detail.quitting {
		return m, tea.Quit
	}
	return m, cmd
}

func (m *appModel) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 不在 loading 页时丢弃迟到结果（取消后回列表等）
	if _, ok := msg.(detailLoadedMsg); ok {
		return m, nil
	}
	if _, ok := msg.(detailLoadErrMsg); ok {
		return m, nil
	}

	pk := &m.picker
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if pk.searching {
				pk.searching = false
				pk.searchInput.Blur()
				return m, nil
			}
			return m, tea.Quit

		case "esc":
			if pk.searching {
				pk.searching = false
				pk.searchInput.Blur()
				pk.searchInput.SetValue("")
				pk.applyFilter()
				return m, nil
			}
			return m, tea.Quit

		case "/":
			if !pk.searching {
				pk.searching = true
				pk.searchInput.Focus()
				pk.status = ""
				return m, textinput.Blink
			}

		case "tab", "t":
			if !pk.searching {
				pk.typeIdx = (pk.typeIdx + 1) % len(typeLabels)
				pk.status = ""
				pk.applyFilter()
				return m, nil
			}

		case "1":
			if !pk.searching {
				pk.typeIdx = typeStock
				pk.status = ""
				pk.applyFilter()
				return m, nil
			}
		case "2":
			if !pk.searching {
				pk.typeIdx = typeFund
				pk.status = ""
				pk.applyFilter()
				return m, nil
			}
		case "3":
			if !pk.searching {
				pk.typeIdx = typeAll
				pk.status = ""
				pk.applyFilter()
				return m, nil
			}

		case "enter":
			if pk.searching {
				pk.searching = false
				pk.searchInput.Blur()
				return m, nil
			}
			if len(pk.filtered) == 0 {
				return m, nil
			}
			idx := pk.table.Cursor()
			if idx < 0 || idx >= len(pk.filtered) {
				return m, nil
			}
			row := pk.filtered[idx]
			if row.Category == "fund" {
				pk.status = "基金暂不支持钻取明细（请切到「股票」）"
				return m, nil
			}
			code := row.Position.Code
			m.loadSeq++
			m.loadCode = code
			pk.status = ""
			m.page = pageLoading
			return m, loadDetailCmd(m.detailSvc, m.loadSeq, code, m.accountFlt, m.fromDate, m.toDate)
		}

		if pk.searching {
			pk.searchInput, cmd = pk.searchInput.Update(msg)
			pk.applyFilter()
			return m, cmd
		}
	}

	if !pk.searching {
		pk.table, cmd = pk.table.Update(msg)
	}
	return m, cmd
}

func (m *appModel) View() string {
	switch m.page {
	case pageLoading:
		return m.viewLoading()
	case pageDetail:
		return m.detail.View()
	default:
		return m.picker.View()
	}
}

func (m *appModel) viewLoading() string {
	code := m.loadCode
	if code == "" {
		code = "..."
	}
	var b strings.Builder
	b.WriteString(styleTitle.Render("加载明细") + "\n\n")
	b.WriteString(fmt.Sprintf("正在加载 %s …\n\n", styleTitle.Render(code)))
	b.WriteString(styleHelp.Render("esc/q 取消并返回列表"))
	return b.String()
}

// —— picker 视图与过滤 ——

func pickerColumns() []table.Column {
	return []table.Column{
		{Title: "代码", Width: 8},
		{Title: "名称", Width: 14},
		{Title: "账户", Width: 8},
		{Title: "持仓", Width: 8},
		{Title: "成本价", Width: 8},
		{Title: "现价", Width: 8},
		{Title: "市值", Width: 10},
		{Title: "盈亏", Width: 10},
	}
}

func (m *pickerModel) View() string {
	var b strings.Builder

	title := styleTitle.Render("持仓列表 - 选择标的下钻明细")
	b.WriteString(title + "\n")

	var tabs []string
	for i, name := range typeLabels {
		n := m.countByType(i)
		label := fmt.Sprintf("%s %d", name, n)
		if i == m.typeIdx {
			tabs = append(tabs, styleTabOn.Render(label))
		} else {
			tabs = append(tabs, styleTabOff.Render(label))
		}
	}
	b.WriteString(strings.Join(tabs, " ") + "\n\n")

	searchLabel := "搜索: "
	if m.searching {
		searchLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("#C4612F")).Render("搜索: ")
	}
	b.WriteString(searchLabel + m.searchInput.View() + "\n\n")

	if len(m.filtered) == 0 {
		b.WriteString(styleLabel.Render("（当前筛选无持仓）") + "\n")
	} else {
		b.WriteString(m.table.View() + "\n")
	}

	if m.status != "" {
		b.WriteString(styleWarn.Render(m.status) + "\n")
	}

	help := styleHelp.Render(fmt.Sprintf(
		"tab/t: 股票/基金/全部 · 1/2/3 直切 · ↑/↓ · / 搜索 · Enter 下钻 · ESC: %s · q 退出 · 显示 %d/%d",
		func() string {
			if m.searching {
				return "取消搜索"
			}
			return "退出"
		}(),
		len(m.filtered),
		len(m.positions),
	))
	b.WriteString("\n" + help)

	return b.String()
}

func (m *pickerModel) countByType(typeIdx int) int {
	n := 0
	for _, row := range m.positions {
		if matchPickerType(row.Category, typeIdx) {
			n++
		}
	}
	return n
}

func matchPickerType(category string, typeIdx int) bool {
	switch typeIdx {
	case typeFund:
		return category == "fund"
	case typeStock:
		return category != "fund"
	default:
		return true
	}
}

func (m *pickerModel) applyFilter() {
	query := strings.TrimSpace(strings.ToLower(m.searchInput.Value()))
	m.filtered = nil
	for _, row := range m.positions {
		if !matchPickerType(row.Category, m.typeIdx) {
			continue
		}
		if query != "" {
			code := strings.ToLower(row.Position.Code)
			name := strings.ToLower(row.Position.Name)
			if !strings.Contains(code, query) && !strings.Contains(name, query) {
				continue
			}
		}
		m.filtered = append(m.filtered, row)
	}
	m.rebuildTable()
}

func (m *pickerModel) applyTableSize() {
	if m.width > 0 {
		m.table.SetWidth(m.width)
	}
	if m.height > 0 {
		h := m.height - pickerChromeLines
		if m.status != "" {
			h--
		}
		if h < pickerMinTableH {
			h = pickerMinTableH
		}
		m.table.SetHeight(h)
	}
}

func (m *pickerModel) rebuildTable() {
	columns := pickerColumns()
	var rows []table.Row
	for _, pr := range m.filtered {
		p := pr.Position
		// 单元格必须纯文本：bubbles/table 的 Truncate 会截断 ANSI 导致花屏。
		rows = append(rows, table.Row{
			p.Code,
			p.Name,
			pr.Account,
			fmt.Sprintf("%.0f", p.Count.F()),
			fmt.Sprintf("%.3f", p.Cost.F()),
			fmt.Sprintf("%.3f", p.Price.F()),
			fmt.Sprintf("%.2f", p.Value.F()),
			plainProfit(p.HoldProfit.F()),
		})
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(pickerMinTableH),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		Bold(true).
		Foreground(lipgloss.Color("6")).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("8")).
		BorderBottom(true).
		BorderTop(false).
		BorderLeft(false).
		BorderRight(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("#1F2421")).
		Background(lipgloss.Color("#C4612F")).
		Bold(true)
	t.SetStyles(s)

	m.table = t
	m.applyTableSize()
}

func plainProfit(v float64) string {
	if v > 0 {
		return fmt.Sprintf("+%.2f", v)
	}
	return fmt.Sprintf("%.2f", v)
}
