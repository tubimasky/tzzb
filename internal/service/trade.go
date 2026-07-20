package service

import (
	"fmt"
	"strings"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

// TradeService 交易流水业务服务
type TradeService struct {
	client *api.Client
}

// NewTradeService 创建交易服务实例
func NewTradeService(client *api.Client) *TradeService {
	return &TradeService{client: client}
}

// TradeAgg 买卖汇总数据
type TradeAgg struct {
	BuyAmt, SellAmt, Fee float64
	BuyCnt, SellCnt      float64
	BuyN, SellN, OtherN  int
	Name                 string
}

// NetBuy 净买入金额 = 买入 − 卖出。正=净加仓/净投入，负=净减仓。
func (a TradeAgg) NetBuy() float64 { return a.BuyAmt - a.SellAmt }

// Trades 总笔数
func (a TradeAgg) Trades() int { return a.BuyN + a.SellN + a.OtherN }

// AddRecord 将交易记录加入汇总
func (a *TradeAgg) AddRecord(r map[string]any) {
	op := strings.TrimSpace(fmt.Sprintf("%v", r["op_name"]))
	money := MapFloat(r, "entry_money")
	cnt := MapFloat(r, "entry_count")
	fee := MapFloat(r, "fee")
	if fee == 0 {
		fee = MapFloat(r, "commission")
	}
	a.Fee += fee

	// 银证转账单独计入 other，不进买卖金额
	if strings.Contains(op, "银行") {
		a.OtherN++
		return
	}
	switch {
	case strings.Contains(op, "买") || op == "转入" || op == "担保品转入":
		a.BuyAmt += money
		a.BuyCnt += cnt
		a.BuyN++
	case strings.Contains(op, "卖") || op == "转出" || op == "担保品转出":
		a.SellAmt += money
		a.SellCnt += cnt
		a.SellN++
	default:
		a.OtherN++
	}
	if a.Name == "" {
		if n := strings.TrimSpace(fmt.Sprintf("%v", r["name"])); n != "" && n != "<nil>" {
			a.Name = n
		}
	}
}

// AccountPack 账户及其流水记录
type AccountPack struct {
	Account api.Account
	Records []map[string]any
}

// MatchTradeAccounts 根据账户名过滤匹配账户
func (s *TradeService) MatchTradeAccounts(accountFilter string) ([]api.Account, error) {
	accounts, err := s.client.GetAccountList()
	if err != nil {
		return nil, fmt.Errorf("获取账户失败: %w", err)
	}
	var matched []api.Account
	for _, acc := range accounts {
		if accountFilter == "" || strings.Contains(acc.Name(), accountFilter) {
			matched = append(matched, acc)
		}
	}
	return matched, nil
}

// LoadTradeRecords 加载账户的交易流水
func (s *TradeService) LoadTradeRecords(account api.Account) ([]map[string]any, error) {
	records, err := s.client.GetMoneyHistory(account)
	if err != nil {
		return nil, fmt.Errorf("获取流水失败: %w", err)
	}
	// 为每条记录添加账户名标记
	for i := range records {
		if records[i] == nil {
			records[i] = map[string]any{}
		}
		records[i]["_account"] = account.Name()
	}
	return records, nil
}

// LoadAllTradeRecords 批量加载多个账户的流水并返回 AccountPack 列表
func (s *TradeService) LoadAllTradeRecords(accounts []api.Account) []AccountPack {
	var packs []AccountPack
	for _, acc := range accounts {
		records, err := s.LoadTradeRecords(acc)
		if err != nil || len(records) == 0 {
			continue
		}
		packs = append(packs, AccountPack{
			Account: acc,
			Records: records,
		})
	}
	return packs
}

// AggregateByAccount 按账户汇总交易数据
func (s *TradeService) AggregateByAccount(packs []AccountPack) ([]TradeAgg, TradeAgg) {
	var result []TradeAgg
	var grand TradeAgg
	for _, p := range packs {
		var agg TradeAgg
		for _, r := range p.Records {
			agg.AddRecord(r)
		}
		grand.BuyAmt += agg.BuyAmt
		grand.SellAmt += agg.SellAmt
		grand.Fee += agg.Fee
		grand.BuyN += agg.BuyN
		grand.SellN += agg.SellN
		grand.OtherN += agg.OtherN
		result = append(result, agg)
	}
	return result, grand
}

// AggregateByCode 按代码汇总交易数据
func (s *TradeService) AggregateByCode(records []map[string]any) map[string]*TradeAgg {
	byCode := map[string]*TradeAgg{}
	for _, r := range records {
		code := strings.TrimSpace(fmt.Sprintf("%v", r["code"]))
		if code == "" || code == "<nil>" {
			code = "(无代码)"
		}
		agg, ok := byCode[code]
		if !ok {
			agg = &TradeAgg{}
			byCode[code] = agg
		}
		agg.AddRecord(r)
	}
	return byCode
}

// FlattenRecords 将 AccountPack 列表展平为单一记录列表
func (s *TradeService) FlattenRecords(packs []AccountPack) []map[string]any {
	var all []map[string]any
	for _, p := range packs {
		all = append(all, p.Records...)
	}
	return all
}
