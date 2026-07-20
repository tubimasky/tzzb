package formatter

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/olekukonko/tablewriter"
)

func TestRenderTable_Basic(t *testing.T) {
	// 捕获标准输出
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	headers := []string{"姓名", "年龄", "城市"}
	rows := [][]string{
		{"张三", "25", "北京"},
		{"李四", "30", "上海"},
	}

	RenderTable(headers, rows)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// 验证表头存在
	if !strings.Contains(output, "姓名") {
		t.Error("Output should contain header '姓名'")
	}
	if !strings.Contains(output, "年龄") {
		t.Error("Output should contain header '年龄'")
	}

	// 验证数据行存在
	if !strings.Contains(output, "张三") {
		t.Error("Output should contain '张三'")
	}
	if !strings.Contains(output, "李四") {
		t.Error("Output should contain '李四'")
	}
}

func TestRenderTable_WithBorder(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	headers := []string{"A", "B"}
	rows := [][]string{{"1", "2"}}

	RenderTable(headers, rows, WithBorder(true))

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// 有边框时应该包含分隔符
	if !strings.Contains(output, "|") {
		t.Error("Output with border should contain '|'")
	}
}

func TestRenderTable_WithoutBorder(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	headers := []string{"A", "B"}
	rows := [][]string{{"1", "2"}}

	RenderTable(headers, rows, WithBorder(false))

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// 验证数据存在
	if !strings.Contains(output, "A") || !strings.Contains(output, "B") {
		t.Error("Output should contain headers")
	}
}

func TestRenderTable_EmptyRows(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	headers := []string{"姓名", "年龄"}
	rows := [][]string{}

	RenderTable(headers, rows)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// 空表格也应该渲染表头
	if !strings.Contains(output, "姓名") {
		t.Error("Empty table should still render headers")
	}
}

func TestTableConfig_DefaultValues(t *testing.T) {
	headers := []string{"A", "B"}
	config := DefaultTableConfig(headers)

	if len(config.Headers) != 2 {
		t.Errorf("Headers length = %d, want 2", len(config.Headers))
	}

	if !config.Border {
		t.Error("Default Border should be true")
	}

	if config.RowLine {
		t.Error("Default RowLine should be false")
	}

	if !config.AutoWrap {
		t.Error("Default AutoWrap should be true")
	}
}

func TestTableConfig_Options(t *testing.T) {
	headers := []string{"A", "B"}

	config := DefaultTableConfig(headers)

	// 测试选项应用
	WithBorder(false)(config)
	if config.Border {
		t.Error("WithBorder(false) should set Border to false")
	}

	WithRowLine(true)(config)
	if !config.RowLine {
		t.Error("WithRowLine(true) should set RowLine to true")
	}

	WithAutoWrap(false)(config)
	if config.AutoWrap {
		t.Error("WithAutoWrap(false) should set AutoWrap to false")
	}

	alignments := []int{tablewriter.ALIGN_LEFT, tablewriter.ALIGN_RIGHT}
	WithAlignment(alignments)(config)
	if len(config.Alignment) != 2 {
		t.Error("WithAlignment should set Alignment")
	}
}

func TestNewTable_AppendAndRender(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	config := DefaultTableConfig([]string{"代码", "名称"})
	table := NewTable(config)
	table.Append([]string{"600519", "贵州茅台"})
	table.Append([]string{"000858", "五粮液"})
	table.Render()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "600519") {
		t.Error("Output should contain '600519'")
	}
	if !strings.Contains(output, "贵州茅台") {
		t.Error("Output should contain '贵州茅台'")
	}
}

func TestNewTable_AppendBulk(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	config := DefaultTableConfig([]string{"A", "B"})
	table := NewTable(config)
	rows := [][]string{
		{"1", "2"},
		{"3", "4"},
	}
	table.AppendBulk(rows)
	table.Render()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "1") || !strings.Contains(output, "2") {
		t.Error("Output should contain bulk rows")
	}
}
