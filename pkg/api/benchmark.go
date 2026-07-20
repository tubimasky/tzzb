package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// BenchmarkPoint 基准指数的单日数据点
type BenchmarkPoint struct {
	Date  string
	Close float64 // 收盘价
}

// GetShanghaiIndexHistory 获取上证指数的历史数据
// 使用腾讯财经的公开接口
func GetShanghaiIndexHistory(days int) ([]BenchmarkPoint, error) {
	if days <= 0 {
		days = 30
	}

	// 腾讯财经上证指数接口
	// sh000001 表示上证指数
	url := fmt.Sprintf("https://web.ifzq.gtimg.cn/appstock/app/fqkline/get?param=sh000001,day,,,%d,qfq", days+10)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 添加必要的请求头
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://gu.qq.com/")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Code int `json:"code"`
		Data map[string]struct {
			Day [][]string `json:"day"` // 格式: ["2026-07-17", "3228.06", "3252.35", "3258.86", "3216.15", "276912035"]
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析失败: %v", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("API 返回错误码: %d", result.Code)
	}

	indexData, ok := result.Data["sh000001"]
	if !ok || len(indexData.Day) == 0 {
		return nil, fmt.Errorf("未获取到数据")
	}

	var points []BenchmarkPoint
	for _, line := range indexData.Day {
		if len(line) < 3 {
			continue
		}

		date := line[0]    // 日期
		closeStr := line[2] // 收盘价（第3列）

		var close float64
		fmt.Sscanf(closeStr, "%f", &close)

		points = append(points, BenchmarkPoint{
			Date:  date,
			Close: close,
		})
	}

	return points, nil
}

// CalculateBenchmarkReturns 计算基准指数的每日收益率
// 返回 map[日期]收益率（相对前一日的涨跌幅）
func CalculateBenchmarkReturns(points []BenchmarkPoint) map[string]float64 {
	returns := make(map[string]float64)

	for i := 1; i < len(points); i++ {
		prev := points[i-1]
		curr := points[i]

		if prev.Close > 0 {
			ret := (curr.Close - prev.Close) / prev.Close
			returns[curr.Date] = ret
		}
	}

	return returns
}
