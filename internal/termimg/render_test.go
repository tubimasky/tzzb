package termimg

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// testPNG 生成一张 cols x rows 的横向渐变图，用于渲染测试。
func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 255 / w), G: uint8(y * 255 / h), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("生成测试图失败: %v", err)
	}
	return buf.Bytes()
}

func TestRenderTruecolor(t *testing.T) {
	data := testPNG(t, 60, 20)
	var sb strings.Builder
	cols, rows, err := renderHalfBlocks(&sb, decode(t, data), 40, true)
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}
	if cols != 40 || rows < 1 {
		t.Errorf("尺寸异常: cols=%d rows=%d", cols, rows)
	}
	out := sb.String()
	if !strings.Contains(out, "\x1b[38;2;") || !strings.Contains(out, "\x1b[48;2;") {
		t.Errorf("缺少 24 位色转义序列")
	}
	if got := strings.Count(out, "▀"); got != cols*rows {
		t.Errorf("半方块数量 = %d, 期望 %d", got, cols*rows)
	}
	if !strings.HasSuffix(out, "\x1b[0m\n") {
		t.Errorf("每行应以重置序列结尾")
	}
}

func TestRender256(t *testing.T) {
	data := testPNG(t, 60, 20)
	var sb strings.Builder
	cols, rows, err := renderHalfBlocks(&sb, decode(t, data), 30, false)
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}
	out := sb.String()
	if !strings.Contains(out, "\x1b[38;5;") || !strings.Contains(out, "\x1b[48;5;") {
		t.Errorf("缺少 256 色转义序列")
	}
	if got := strings.Count(out, "▀"); got != cols*rows {
		t.Errorf("半方块数量 = %d, 期望 %d", got, cols*rows)
	}
}

func TestRenderASCII(t *testing.T) {
	data := testPNG(t, 60, 20)
	var sb strings.Builder
	cols, rows, err := RenderASCII(&sb, decode(t, data), 50)
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}
	lines := strings.Split(strings.TrimRight(sb.String(), "\n"), "\n")
	if len(lines) != rows {
		t.Errorf("行数 = %d, 期望 %d", len(lines), rows)
	}
	for i, ln := range lines {
		if len([]rune(ln)) != cols {
			t.Errorf("第 %d 行字符数 = %d, 期望 %d", i, len([]rune(ln)), cols)
		}
	}
}

func TestRenderInvalidImage(t *testing.T) {
	var sb strings.Builder
	if _, _, err := Render(&sb, []byte("not an image"), 40); err == nil {
		t.Fatalf("非法图片应返回错误")
	}
}

func TestColsClamp(t *testing.T) {
	// 极小/极大的列数都应被钳到安全范围，不能 panic。
	data := testPNG(t, 100, 30)
	for _, cols := range []int{-5, 0, 1, 500} {
		var sb strings.Builder
		if _, _, err := Render(&sb, data, cols); err != nil {
			t.Errorf("cols=%d 渲染失败: %v", cols, err)
		}
	}
}

func decode(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("解码测试图失败: %v", err)
	}
	return img
}
