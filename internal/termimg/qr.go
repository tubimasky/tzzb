package termimg

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // 注册 JPEG 解码器
	_ "image/png"  // 注册 PNG 解码器
	"io"
	"strings"
)

// ErrQRTooLarge 表示终端尺寸放不下能扫描的二维码：缩放会丢模块，只能改用图片窗口。
var ErrQRTooLarge = errors.New("终端尺寸放不下可扫描的二维码")

// RenderQR 把二维码图片画到终端，供手机直接扫描。
//
// 二维码不能像照片那样平滑缩放，否则模块边界会糊、手机扫不出来。这里不按像素
// 缩放，而是识别出二维码的模块网格（先由左上角定位图案的 7 模块宽度估出模块
// 像素尺寸，再按每个模块的中心取样），做到 1 个模块 = 1 个字符：
//
//	▀ 半方块 = 上半像素（前景色，上模块） + 下半像素（背景色，下模块）
//
// 终端字符高约为宽的两倍，所以每个模块在屏幕上仍是正方形。颜色显式写成黑/白，
// 深色主题下也能扫。
//
// 返回实际字符宽度、行数。若二维码需要的宽度/高度超过 maxCols/maxRows，
// 会尽量缩小（必要时丢模块）并在 cols 较小时由调用方提示改开图片窗口。
func RenderQR(w io.Writer, data []byte, maxCols, maxRows int) (int, int, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, 0, fmt.Errorf("解码二维码失败: %w", err)
	}
	black := binarize(img)
	grid, ok := sampleQRGrid(black)
	if !ok {
		return 0, 0, fmt.Errorf("识别二维码模块失败（图片可能不是清晰的二维码）")
	}
	modules := len(grid)

	// 保持模块正方形：1 字符宽 × 2 模块高。另加 2 模块静区。
	const quiet = 2
	cols := modules + quiet*2
	rows := (cols + 1) / 2
	if cols > maxCols || rows > maxRows {
		// 二维码不能缩放：丢模块必扫不出来。这里直接报错，让调用方改用图片窗口/最大化终端。
		return cols, rows, ErrQRTooLarge
	}

	const (
		fgBlack = "\x1b[38;2;0;0;0m"
		bgBlack = "\x1b[48;2;0;0;0m"
		fgWhite = "\x1b[38;2;255;255;255m"
		bgWhite = "\x1b[48;2;255;255;255m"
		reset   = "\x1b[0m"
	)
	pick := func(isBlack bool) (string, string) {
		if isBlack {
			return fgBlack, bgBlack
		}
		return fgWhite, bgWhite
	}
	cell := func(topBlack, bottomBlack bool) string {
		ft, _ := pick(topBlack)
		_, bt := pick(bottomBlack)
		return ft + bt + "▀"
	}

	var sb strings.Builder
	sb.Grow(rows * cols * 26)
	totalRows := modules + quiet*2
	for row := 0; row < totalRows; row += 2 {
		for col := 0; col < totalRows; col++ {
			// 左上角多留 2 个模块静区；把 (row,col) 映射回模块坐标（含静区）。
			topBlack := moduleAt(grid, row-quiet, col-quiet)
			bottomBlack := false
			if row+1 < totalRows {
				bottomBlack = moduleAt(grid, row+1-quiet, col-quiet)
			}
			sb.WriteString(cell(topBlack, bottomBlack))
		}
		sb.WriteString(reset)
		sb.WriteByte('\n')
	}
	if _, err := io.WriteString(w, sb.String()); err != nil {
		return cols, rows, err
	}
	return cols, rows, nil
}

// moduleAt 取模块网格坐标（越界视为白色静区）。
func moduleAt(grid [][]bool, row, col int) bool {
	if row < 0 || row >= len(grid) || col < 0 || col >= len(grid[row]) {
		return false
	}
	return grid[row][col]
}

// sampleQRGrid 识别二维码模块网格：估出模块像素尺寸，按模块中心取样。
//
// 估尺寸的办法：二维码左上角定位图案是 7×7 模块的实心方块，其最上面一行是
// 一段 7 模块宽的连续黑色，所以「第一段黑色 run 长度 / 7」就是模块像素宽。
func sampleQRGrid(black [][]bool) ([][]bool, bool) {
	minX, minY, maxX, maxY := blackBounds(black)
	if maxX <= minX || maxY <= minY {
		return nil, false
	}
	w, h := maxX-minX+1, maxY-minY+1

	// 在二维码本体的上部找 7 模块宽的黑色 run（避开抗锯齿，多试几行取中位数）。
	var runs []int
	for _, y := range []int{minY, minY + 1, minY + h/20} {
		if y < 0 || y >= len(black) {
			continue
		}
		run := 0
		for x := minX; x <= maxX && x < len(black[y]); x++ {
			if black[y][x] {
				run++
			} else if run > 0 {
				break
			}
		}
		if run > 0 {
			runs = append(runs, run)
		}
	}
	if len(runs) == 0 {
		return nil, false
	}
	sortInts(runs)
	modulePx := float64(runs[len(runs)/2]) / 7.0
	if modulePx < 0.9 {
		return nil, false
	}
	modules := int(float64(w)/modulePx + 0.5)
	if modules < 10 || modules > 200 {
		return nil, false
	}
	// 高度必须也吻合，否则说明定位图案判断错了。
	if mh := int(float64(h)/modulePx + 0.5); absInt(mh-modules) > 2 {
		return nil, false
	}

	grid := make([][]bool, modules)
	for r := 0; r < modules; r++ {
		grid[r] = make([]bool, modules)
		cy := minY + int((float64(r)+0.5)*modulePx)
		for c := 0; c < modules; c++ {
			cx := minX + int((float64(c)+0.5)*modulePx)
			if cy >= 0 && cy < len(black) && cx >= 0 && cx < len(black[cy]) {
				grid[r][c] = black[cy][cx]
			}
		}
	}
	return grid, true
}

// binarize 把图片按亮度阈值转成黑/白二维布尔表。
func binarize(img image.Image) [][]bool {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([][]bool, h)
	for y := 0; y < h; y++ {
		out[y] = make([]bool, w)
		for x := 0; x < w; x++ {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			lum := 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(bl>>8)
			out[y][x] = lum < 128
		}
	}
	return out
}

// blackBounds 返回黑色像素的外接矩形（右下为闭区间）。没有黑像素时返回全 0。
func blackBounds(black [][]bool) (minX, minY, maxX, maxY int) {
	h := len(black)
	if h == 0 {
		return 0, 0, 0, 0
	}
	w := len(black[0])
	minX, minY, maxX, maxY = w, h, -1, -1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !black[y][x] {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if maxX < minX || maxY < minY {
		return 0, 0, 0, 0
	}
	return minX, minY, maxX, maxY
}

func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
