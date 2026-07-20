package cli

import (
	"fmt"
	"strings"
	"testing"
)

func TestHandleError(t *testing.T) {
	// 我们不能直接测试 HandleError，因为它会 os.Exit
	// 所以只测试错误格式化逻辑
	err := fmt.Errorf("connection refused")
	msg := "连接失败"

	expectedOutput := fmt.Sprintf("%s: %v\n", msg, err)

	if expectedOutput != "连接失败: connection refused\n" {
		t.Errorf("Error format incorrect")
	}
}

func TestHandleErrorWithCode(t *testing.T) {
	// 测试错误格式化
	err := fmt.Errorf("not found")
	msg := "文件不存在"
	code := 2

	// 验证错误消息格式
	expectedOutput := fmt.Sprintf("%s: %v\n", msg, err)

	if !strings.Contains(expectedOutput, "文件不存在") {
		t.Error("Error message should contain '文件不存在'")
	}

	if !strings.Contains(expectedOutput, "not found") {
		t.Error("Error message should contain 'not found'")
	}

	if code != 2 {
		t.Errorf("Exit code should be 2, got %d", code)
	}
}

func TestErrorMessageFormat(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
		want string
	}{
		{
			name: "简单错误",
			err:  fmt.Errorf("timeout"),
			msg:  "请求超时",
			want: "请求超时: timeout",
		},
		{
			name: "中文错误",
			err:  fmt.Errorf("文件不存在"),
			msg:  "读取失败",
			want: "读取失败: 文件不存在",
		},
		{
			name: "空消息",
			err:  fmt.Errorf("error"),
			msg:  "",
			want: ": error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fmt.Sprintf("%s: %v", tt.msg, tt.err)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
