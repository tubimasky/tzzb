package service

import (
	"sort"

	"github.com/liyanran/tzzb-cli/internal/formatter"
	"github.com/liyanran/tzzb-cli/pkg/api"
)

// TradeJSONService 交易 JSON 输出服务
type TradeJSONService struct {
	tradeService *TradeService
}

// NewTradeJSONService 创建交易 JSON 服务
func NewTradeJSONService(tradeService *TradeService) *TradeJSONService {
	return &TradeJSONService{tradeService: tradeService}
}

// BuildListJSON 构建 list 命令的 JSON 输出
func (s *TradeJSONService) BuildListJSON(packs []AccountPack) []map[string]any {
	var results []map[string]any
	for _, pack := range packs {
		if len(pack.Records) == 0 {
			continue
		}
		results = append(results, map[string]any{
			"account":  pack.Account.Name(),
			"category": api.CategoryLabel(pack.Account.Category),
			"count":    len(pack.Records),
			"records":  pack.Records,
		})
	}
	return results
}

// BuildSummaryJSON 构建 summary 命令的 JSON 输出
func (s *TradeJSONService) BuildSummaryJSON(packs []AccountPack, all []map[string]any) map[string]any {
	result := map[string]any{}

	// 按账户汇总
	var accountSummary []map[string]any
	aggList, grand := s.tradeService.AggregateByAccount(packs)
	for i, p := range packs {
		a := aggList[i]
		accountSummary = append(accountSummary, map[string]any{
			"account":     p.Account.Name(),
			"category":    api.CategoryLabel(p.Account.Category),
			"buy_amount":  a.BuyAmt,
			"sell_amount": a.SellAmt,
			"net_buy":     a.NetBuy(),
			"buy_count":   a.BuyN,
			"sell_count":  a.SellN,
			"other_count": a.OtherN,
			"fee":         a.Fee,
			"total_count": a.Trades(),
		})
	}
	result["by_account"] = accountSummary

	// 按代码汇总
	byCode := s.tradeService.AggregateByCode(all)
	var codeSummary []map[string]any
	for code, a := range byCode {
		if a.BuyN+a.SellN == 0 {
			continue
		}
		codeSummary = append(codeSummary, map[string]any{
			"code":        code,
			"name":        a.Name,
			"buy_amount":  a.BuyAmt,
			"sell_amount": a.SellAmt,
			"net_buy":     a.NetBuy(),
			"buy_volume":  a.BuyCnt,
			"sell_volume": a.SellCnt,
			"buy_count":   a.BuyN,
			"sell_count":  a.SellN,
		})
	}
	// 按买卖金额合计降序
	sort.Slice(codeSummary, func(i, j int) bool {
		ti := codeSummary[i]["buy_amount"].(float64) + codeSummary[i]["sell_amount"].(float64)
		tj := codeSummary[j]["buy_amount"].(float64) + codeSummary[j]["sell_amount"].(float64)
		return ti > tj
	})
	result["by_code"] = codeSummary

	// 总汇总
	result["summary"] = map[string]any{
		"total_records": len(all),
		"buy_amount":    grand.BuyAmt,
		"buy_count":     grand.BuyN,
		"sell_amount":   grand.SellAmt,
		"sell_count":    grand.SellN,
		"net_buy":       grand.NetBuy(),
		"fee":           grand.Fee,
		"other_count":   grand.OtherN,
	}

	return result
}

// PrintJSON 输出 JSON（统一入口）
func (s *TradeJSONService) PrintJSON(data any) {
	formatter.PrintJSON(data)
}
