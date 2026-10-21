// Package termimg 把图片直接渲染进终端，避免为看一眼图形验证码而弹外部看图窗口。
//
// 实现是标准的"半方块"技巧：每个字符位画一个 ▀，前景色为上半像素、背景色为
// 下半像素，因此一个字符能承载 1x2 个像素。终端字符通常高约为宽的两倍，
// 这样渲染出来的图片比例不会变形。
package termimg

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg" // 注册 JPEG 解码器
	_ "image/png"  // 注册 PNG 解码器
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

// Cols 返回终端可用列数：先 ioctl 探测真实宽度，再退回 $COLUMNS，最后默认 80。
func Cols() int {
	if n, _ := terminalSize(); n >= 20 {
		return n
	}
	if s := strings.TrimSpace(os.Getenv("COLUMNS")); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 20 {
			return n
		}
	}
	return 80
}

// Rows 返回终端可用行数：先 ioctl 探测，再退回 $LINES，最后默认 24。
func Rows() int {
	if _, n := terminalSize(); n >= 8 {
		return n
	}
	if s := strings.TrimSpace(os.Getenv("LINES")); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 8 {
			return n
		}
	}
	return 24
}

// Truecolor 报告终端是否支持 24 位色（COLORTERM=truecolor/24bit）。
func Truecolor() bool {
	c := strings.ToLower(os.Getenv("COLORTERM"))
	return strings.Contains(c, "truecolor") || strings.Contains(c, "24bit")
}

// NoColor 报告终端是否明确不支持彩色（TERM=dumb 或设置了 NO_COLOR）。
func NoColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(os.Getenv("TERM")), "dumb")
}

// Render 把 data 里的图片渲染到 w。cols 为期望的字符宽度（会被钳到合理范围）。
// 返回实际渲染的字符宽度与行数（行数按终端行计，不是像素行）。
func Render(w io.Writer, data []byte, cols int) (int, int, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, 0, fmt.Errorf("解码图片失败: %w", err)
	}
	b := img.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	if srcW <= 0 || srcH <= 0 {
		return 0, 0, fmt.Errorf("图片尺寸非法: %dx%d", srcW, srcH)
	}
	if cols <= 0 {
		cols = Cols() - 2
	}
	cols = clamp(cols, 20, 200)

	if NoColor() {
		return RenderASCII(w, img, cols)
	}
	return renderHalfBlocks(w, img, cols, Truecolor())
}

// RenderASCII 用亮度字符渲染（不支持彩色的终端兜底）。
func RenderASCII(w io.Writer, img image.Image, cols int) (int, int, error) {
	b := img.Bounds()
	rows := int(math.Round(float64(b.Dy()) / float64(b.Dx()) * float64(cols) * 0.5))
	rows = clamp(rows, 1, 100)
	const ramp = " .:-=+*#%@"
	var sb strings.Builder
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			r, g, bl := averageRGB(img, x, y, cols, rows)
			// 人眼亮度权重，映射到字符梯度。
			lum := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(bl)
			idx := int(lum / 255 * float64(len(ramp)-1))
			sb.WriteByte(ramp[clamp(idx, 0, len(ramp)-1)])
		}
		sb.WriteByte('\n')
	}
	_, err := io.WriteString(w, sb.String())
	return cols, rows, err
}

// renderHalfBlocks 用 ▀ + 前景/背景色渲染，一个字符承载 1x2 像素。
func renderHalfBlocks(w io.Writer, img image.Image, cols int, truecolor bool) (int, int, error) {
	b := img.Bounds()
	// 每个字符承载 2 行像素，所以像素行数 = 图片比例换算后再乘 2。
	rows := int(math.Round(float64(b.Dy()) / float64(b.Dx()) * float64(cols) / 2))
	rows = clamp(rows, 1, 60)
	pixRows := rows * 2

	grid := make([][][3]uint8, pixRows)
	for py := 0; py < pixRows; py++ {
		grid[py] = make([][3]uint8, cols)
		for px := 0; px < cols; px++ {
			r, g, bl := averageRGB(img, px, py, cols, pixRows)
			grid[py][px] = [3]uint8{r, g, bl}
		}
	}

	var sb strings.Builder
	for row := 0; row < rows; row++ {
		for x := 0; x < cols; x++ {
			top := grid[row*2][x]
			bottom := grid[row*2+1][x]
			if truecolor {
				fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀",
					top[0], top[1], top[2], bottom[0], bottom[1], bottom[2])
			} else {
				fmt.Fprintf(&sb, "\x1b[38;5;%dm\x1b[48;5;%dm▀",
					xterm256(top[0], top[1], top[2]), xterm256(bottom[0], bottom[1], bottom[2]))
			}
		}
		sb.WriteString("\x1b[0m\n")
	}
	_, err := io.WriteString(w, sb.String())
	return cols, rows, err
}

// averageRGB 计算第 (px, py) 个输出像素对应的源图区域的平均颜色。
func averageRGB(img image.Image, px, py, cols, rows int) (uint8, uint8, uint8) {
	b := img.Bounds()
	x0 := b.Min.X + px*b.Dx()/cols
	x1 := b.Min.X + (px+1)*b.Dx()/cols
	y0 := b.Min.Y + py*b.Dy()/rows
	y1 := b.Min.Y + (py+1)*b.Dy()/rows
	if x1 <= x0 {
		x1 = x0 + 1
	}
	if y1 <= y0 {
		y1 = y0 + 1
	}
	var sr, sg, sb, n uint64
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			sr += uint64(r)
			sg += uint64(g)
			sb += uint64(bl)
			n++
		}
	}
	if n == 0 {
		return 0, 0, 0
	}
	return uint8(sr / n >> 8), uint8(sg / n >> 8), uint8(sb / n >> 8)
}

// xterm256 把 RGB 量化到 xterm-256 调色板的 6x6x6 色立方。
func xterm256(r, g, b uint8) int {
	q := func(v uint8) int {
		if v < 48 {
			return 0
		}
		if v < 114 {
			return 1
		}
		return int((int(v)-35)/40) + 1
	}
	return 16 + 36*q(r) + 6*q(g) + q(b)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
