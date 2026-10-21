package termimg

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// fakeQR 生成一张带 7x7 定位图案的假二维码图：41 个模块，每模块 6 像素。
func fakeQR(t *testing.T) ([]byte, int) {
	t.Helper()
	const modules, px = 41, 6
	img := image.NewRGBA(image.Rect(0, 0, modules*px, modules*px))
	// 全白底
	for y := 0; y < modules*px; y++ {
		for x := 0; x < modules*px; x++ {
			img.Set(x, y, color.White)
		}
	}
	black := func(col, row int) {
		for y := row * px; y < (row+1)*px; y++ {
			for x := col * px; x < (col+1)*px; x++ {
				img.Set(x, y, color.Black)
			}
		}
	}
	// 左上/右上/左下三个定位图案（7x7 实心环）
	finder := func(c0, r0 int) {
		for r := 0; r < 7; r++ {
			for c := 0; c < 7; c++ {
				if r == 0 || r == 6 || c == 0 || c == 6 || (r >= 2 && r <= 4 && c >= 2 && c <= 4) {
					black(c0+c, r0+r)
				}
			}
		}
	}
	finder(0, 0)
	finder(modules-7, 0)
	finder(0, modules-7)
	// 随便加一些内部黑块，模拟数据区
	for r := 10; r < 30; r += 3 {
		for c := 10; c < 30; c += 2 {
			black(c, r)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("生成假二维码失败: %v", err)
	}
	return buf.Bytes(), modules
}

func TestRenderQR(t *testing.T) {
	data, modules := fakeQR(t)
	var sb strings.Builder
	cols, rows, err := RenderQR(&sb, data, 120, 40)
	if err != nil {
		t.Fatalf("渲染二维码失败: %v", err)
	}
	if cols != modules+4 { // 2 模块静区 × 2 边
		t.Errorf("宽度 = %d, 期望 %d", cols, modules+4)
	}
	if want := (cols + 1) / 2; rows != want {
		t.Errorf("行数 = %d, 期望 %d", rows, want)
	}
	out := sb.String()
	if got := strings.Count(out, "▀"); got != cols*rows {
		t.Errorf("半方块数量 = %d, 期望 %d", got, cols*rows)
	}
	if !strings.Contains(out, "\x1b[38;2;0;0;0m") || !strings.Contains(out, "\x1b[48;2;255;255;255m") {
		t.Errorf("缺少显式黑白配色（深色主题也需要能扫）")
	}
}

func TestRenderQRTooLarge(t *testing.T) {
	data, modules := fakeQR(t)
	var sb strings.Builder
	cols, _, err := RenderQR(&sb, data, 30, 10)
	if !errors.Is(err, ErrQRTooLarge) {
		t.Fatalf("期望 ErrQRTooLarge，实际 %v", err)
	}
	if cols != modules+4 {
		t.Errorf("返回的所需宽度 = %d, 期望 %d", cols, modules+4)
	}
	if sb.Len() != 0 {
		t.Errorf("放不下时不应输出半截二维码")
	}
}

func TestRenderQRInvalidImage(t *testing.T) {
	var sb strings.Builder
	if _, _, err := RenderQR(&sb, []byte("not an image"), 100, 40); err == nil {
		t.Fatalf("非法图片应返回错误")
	}
}
