package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// MapFloat 从 map 中提取浮点数值，支持多种类型转换
func MapFloat(r map[string]any, key string) float64 {
	v, ok := r[key]
	if !ok || v == nil {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	default:
		s := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(fmt.Sprintf("%v", v), ",", ""), " ", ""))
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0
		}
		return f
	}
}

// PickField 从 map 中按优先级选择第一个有效字段值
func PickField(r map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := r[k]; ok && v != nil {
			s := strings.TrimSpace(fmt.Sprintf("%v", v))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}
