package formatter

import (
	"encoding/json"
	"fmt"
	"os"
)

// PrintJSON 输出 JSON 格式（美化缩进）
func PrintJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "JSON 编码失败: %v\n", err)
	}
}
