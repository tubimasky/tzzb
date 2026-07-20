package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Quote 是一条实时行情（同源插件 getQuotes：xianjia/zuoshou）。
type Quote struct {
	Code      string  // 归一化代码，如 512890
	Market    string  // 1=深 2=沪
	Price     float64 // 现价 xianjia
	PrevClose float64 // 昨收 zuoshou
}

// QuoteRef 拉行情时的代码+市场。
type QuoteRef struct {
	Code   string
	Market string
}

// NormalizeCode 去掉市场后缀（515790.SZ → 515790）。
func NormalizeCode(code string) string {
	code = strings.TrimSpace(code)
	if i := strings.IndexAny(code, "."); i > 0 {
		return code[:i]
	}
	return code
}

// GuessMarket 无 market 时按 A 股代码启发式：5/6/9→沪(2)，其余深(1)。
func GuessMarket(code, market string) string {
	market = strings.TrimSpace(market)
	if market != "" {
		return market
	}
	code = NormalizeCode(code)
	if code == "" {
		return "1"
	}
	switch code[0] {
	case '5', '6', '9':
		return "2"
	default:
		return "1"
	}
}

// GetQuotes 批量拉同花顺实时行情（插件 requestPriceInfo → /invest/getQuotes）。
// 参数 code 形如 "2:512890,1:159992"；返回 map[归一化代码]Quote。
func (c *Client) GetQuotes(refs []QuoteRef) (map[string]Quote, error) {
	seen := map[string]struct{}{}
	var parts []string
	for _, r := range refs {
		code := NormalizeCode(r.Code)
		if code == "" {
			continue
		}
		mkt := GuessMarket(code, r.Market)
		key := mkt + ":" + code
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		parts = append(parts, key)
	}
	out := map[string]Quote{}
	if len(parts) == 0 {
		return out, nil
	}

	// 单次不宜过长，按 80 一批
	const batch = 80
	for i := 0; i < len(parts); i += batch {
		end := i + batch
		if end > len(parts) {
			end = len(parts)
		}
		ex, err := c.call(epGetQuotes, map[string]string{
			"code":   strings.Join(parts[i:end], ","),
			"userid": c.uid,
			"date":   "",
		})
		if err != nil {
			return out, err
		}
		// ex_data 是数组
		var arr []struct {
			Xianjia string `json:"xianjia"`
			Zuoshou string `json:"zuoshou"`
			Zqdm    string `json:"zqdm"`
			Scdm    string `json:"scdm"`
		}
		if err := json.Unmarshal(ex, &arr); err != nil {
			return out, fmt.Errorf("解析行情失败: %v", err)
		}
		for _, it := range arr {
			code := NormalizeCode(it.Zqdm)
			if code == "" {
				continue
			}
			price, _ := strconv.ParseFloat(strings.TrimPrefix(strings.TrimSpace(it.Xianjia), "HK$"), 64)
			prev, _ := strconv.ParseFloat(strings.TrimPrefix(strings.TrimSpace(it.Zuoshou), "HK$"), 64)
			// 去掉港币前缀后的纯数字；空串归零
			if price == 0 && prev == 0 {
				continue
			}
			out[code] = Quote{
				Code:      code,
				Market:    strings.TrimSpace(it.Scdm),
				Price:     price,
				PrevClose: prev,
			}
		}
	}
	return out, nil
}
