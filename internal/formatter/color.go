package formatter

import (
	"fmt"

	"github.com/fatih/color"
)

var (
	// NoColor 全局禁用颜色标志（由 cmd 层设置）
	NoColor = false
)

// ColorNum 金额着色：中国股市习惯，红涨绿跌
func ColorNum(v float64) string {
	if NoColor {
		return fmt.Sprintf("%.2f", v)
	}
	if v < 0 {
		return color.New(color.FgGreen).Sprintf("%.2f", v)
	}
	return color.New(color.FgRed).Sprintf("%.2f", v)
}

// ColorPct 百分比着色（值已经是百分比，如 45.05 表示 45.05%）
// 中国股市：红涨绿跌
func ColorPct(v float64) string {
	if NoColor {
		return fmt.Sprintf("%.2f%%", v)
	}
	if v < 0 {
		return color.New(color.FgGreen).Sprintf("%.2f%%", v)
	}
	return color.New(color.FgRed).Sprintf("%.2f%%", v)
}

// ColorPctRaw 百分比着色（值是原始小数，如 -0.0223 表示 -2.23%）
func ColorPctRaw(v float64) string {
	return ColorPct(v * 100)
}

// PriceStr 成本/现价统一三位小数；0 显示为 "-"
func PriceStr(v float64) string {
	if v <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.3f", v)
}

// DayStr 今日盈亏字符串（有行情时着色，无行情时显示 "-"）
func DayStr(v float64, ok bool) string {
	if !ok {
		return "-"
	}
	return ColorNum(v)
}

// RateStr 涨跌幅字符串（有数据时着色，无数据时显示 "-"）
func RateStr(v float64, ok bool) string {
	if !ok {
		return "-"
	}
	return ColorPct(v)
}

// WeightStr 市值占比（百分比）
func WeightStr(value, total float64) string {
	if total == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", value/total*100)
}
