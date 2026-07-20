package formatter

import (
	"os"

	"github.com/olekukonko/tablewriter"
)

// TableConfig 表格配置
type TableConfig struct {
	Headers     []string
	Border      bool
	RowLine     bool
	AutoWrap    bool
	Alignment   []int // tablewriter.ALIGN_* 常量
}

// DefaultTableConfig 默认表格配置
func DefaultTableConfig(headers []string) *TableConfig {
	return &TableConfig{
		Headers:  headers,
		Border:   true,
		RowLine:  false,
		AutoWrap: true,
	}
}

// Table 表格渲染器
type Table struct {
	config *TableConfig
	writer *tablewriter.Table
}

// NewTable 创建新表格
func NewTable(config *TableConfig) *Table {
	if config == nil {
		config = DefaultTableConfig([]string{})
	}

	writer := tablewriter.NewWriter(os.Stdout)
	writer.SetHeader(config.Headers)
	writer.SetBorder(config.Border)
	writer.SetRowLine(config.RowLine)
	writer.SetAutoWrapText(config.AutoWrap)

	// 设置列对齐
	if len(config.Alignment) > 0 {
		writer.SetColumnAlignment(config.Alignment)
	}

	return &Table{
		config: config,
		writer: writer,
	}
}

// Append 添加一行数据
func (t *Table) Append(row []string) {
	t.writer.Append(row)
}

// AppendBulk 批量添加行
func (t *Table) AppendBulk(rows [][]string) {
	t.writer.AppendBulk(rows)
}

// Render 渲染表格
func (t *Table) Render() {
	t.writer.Render()
}

// RenderTable 快速渲染表格（一次性）
func RenderTable(headers []string, rows [][]string, opts ...func(*TableConfig)) {
	config := DefaultTableConfig(headers)
	for _, opt := range opts {
		opt(config)
	}

	table := NewTable(config)
	table.AppendBulk(rows)
	table.Render()
}

// WithBorder 设置是否显示边框
func WithBorder(border bool) func(*TableConfig) {
	return func(c *TableConfig) {
		c.Border = border
	}
}

// WithRowLine 设置是否显示行分隔线
func WithRowLine(rowLine bool) func(*TableConfig) {
	return func(c *TableConfig) {
		c.RowLine = rowLine
	}
}

// WithAutoWrap 设置是否自动换行
func WithAutoWrap(autoWrap bool) func(*TableConfig) {
	return func(c *TableConfig) {
		c.AutoWrap = autoWrap
	}
}

// WithAlignment 设置列对齐方式
func WithAlignment(alignment []int) func(*TableConfig) {
	return func(c *TableConfig) {
		c.Alignment = alignment
	}
}
