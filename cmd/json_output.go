package cmd

import (
	"encoding/json"
	"fmt"
	"os"
)

// printJSON 以 JSON 格式输出数据
func printJSON(v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON 序列化失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}
