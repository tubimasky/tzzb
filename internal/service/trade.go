package service

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/liyanran/tzzb-cli/internal/formatter"
	"github.com/liyanran/tzzb-cli/pkg/api"
	"github.com/olekukonko/tablewriter"
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
	accounts, err := s.client.GetAccountMetadata()
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

// TradeCommandService 交易命令服务（封装重复的加载+渲染流程）
type TradeCommandService struct {
	tradeService  *TradeService
	renderService *TradeRenderService
	jsonService   *TradeJSONService
}

// NewTradeCommandService 创建交易命令服务
func NewTradeCommandService(tradeService *TradeService) *TradeCommandService {
	return &TradeCommandService{
		tradeService:  tradeService,
		renderService: NewTradeRenderService(tradeService),
		jsonService:   NewTradeJSONService(tradeService),
	}
}

// ExecuteListCommand 执行 list 命令逻辑
func (s *TradeCommandService) ExecuteListCommand(accountFilter string, outputJSON bool) {
	matched, err := s.tradeService.MatchTradeAccounts(accountFilter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if len(matched) == 0 {
		fmt.Println("没有匹配的账户")
		os.Exit(0)
	}

	packs := s.tradeService.LoadAllTradeRecords(matched)

	if outputJSON {
		results := s.jsonService.BuildListJSON(packs)
		if len(results) == 0 {
			s.jsonService.PrintJSON([]any{})
		} else {
			s.jsonService.PrintJSON(results)
		}
		return
	}

	total := s.renderService.RenderRecordsByAccount(packs)
	if total == 0 {
		fmt.Println("暂无交易记录")
	}
}

// ExecuteSummaryCommand 执行 summary 命令逻辑
func (s *TradeCommandService) ExecuteSummaryCommand(accountFilter string, outputJSON bool) {
	matched, err := s.tradeService.MatchTradeAccounts(accountFilter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if len(matched) == 0 {
		fmt.Println("没有匹配的账户")
		os.Exit(0)
	}

	packs := s.tradeService.LoadAllTradeRecords(matched)
	if len(packs) == 0 {
		if outputJSON {
			s.jsonService.PrintJSON(map[string]any{"error": "no trade records"})
		} else {
			fmt.Println("暂无交易记录")
		}
		return
	}

	all := s.tradeService.FlattenRecords(packs)

	if outputJSON {
		result := s.jsonService.BuildSummaryJSON(packs, all)
		s.jsonService.PrintJSON(result)
		return
	}

	s.renderService.RenderAccountSummary(packs)
	fmt.Println()
	s.renderService.RenderCodeSummary(all, 40)
	fmt.Println()
	s.renderService.RenderGrandSummary(all)
}

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

// TradeRenderService 交易渲染服务
type TradeRenderService struct {
	tradeService *TradeService
}

// NewTradeRenderService 创建交易渲染服务
func NewTradeRenderService(tradeService *TradeService) *TradeRenderService {
	return &TradeRenderService{tradeService: tradeService}
}

// RenderRecordsByAccount 按账户渲染交易记录
func (s *TradeRenderService) RenderRecordsByAccount(packs []AccountPack) int {
	total := 0
	for _, pack := range packs {
		if len(pack.Records) == 0 {
			continue
		}
		total += len(pack.Records)
		fmt.Printf("\n【%s · %s】共 %d 条\n", pack.Account.Name(), api.CategoryLabel(pack.Account.Category), len(pack.Records))
		s.RenderRecords(pack.Records)
	}
	return total
}

// RenderRecords 渲染交易记录表格（固定中文列）
func (s *TradeRenderService) RenderRecords(records []map[string]any) {
	// 候选字段 → 中文表头
	type col struct {
		keys  []string
		title string
	}
	cols := []col{
		{[]string{"entry_date", "date"}, "日期"},
		{[]string{"entry_time", "time"}, "时间"},
		{[]string{"op_name", "type", "op"}, "类型"},
		{[]string{"code"}, "代码"},
		{[]string{"name"}, "名称"},
		{[]string{"entry_count", "count"}, "数量"},
		{[]string{"entry_price", "price"}, "价格"},
		{[]string{"entry_money", "amount", "money"}, "金额"},
		{[]string{"fee", "commission"}, "费用"},
	}

	// 检测哪些列有值
	var used []col
	for _, c := range cols {
		ok := false
		for _, r := range records {
			for _, k := range c.keys {
				if v, exists := r[k]; exists && v != nil && fmt.Sprintf("%v", v) != "" {
					ok = true
					break
				}
			}
			if ok {
				break
			}
		}
		if ok {
			used = append(used, c)
		}
	}
	if len(used) == 0 {
		// 回退：动态字段
		s.renderRecordsDynamic(records)
		return
	}

	headers := make([]string, len(used))
	for i, c := range used {
		headers[i] = c.title
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(headers)
	table.SetBorder(true)
	table.SetAutoWrapText(false)
	for _, r := range records {
		row := make([]string, len(used))
		for i, c := range used {
			row[i] = PickField(r, c.keys...)
		}
		table.Append(row)
	}
	table.Render()
}

func (s *TradeRenderService) renderRecordsDynamic(records []map[string]any) {
	keySet := map[string]struct{}{}
	for _, r := range records {
		for k := range r {
			if strings.HasPrefix(k, "_") {
				continue
			}
			keySet[k] = struct{}{}
		}
	}
	var keys []string
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(keys)
	table.SetBorder(true)
	table.SetAutoWrapText(false)
	for _, r := range records {
		row := make([]string, len(keys))
		for i, k := range keys {
			row[i] = fmt.Sprintf("%v", r[k])
		}
		table.Append(row)
	}
	table.Render()
}

// RenderAccountSummary 渲染按账户汇总表格
func (s *TradeRenderService) RenderAccountSummary(packs []AccountPack) {
	aggList, grand := s.tradeService.AggregateByAccount(packs)

	color.New(color.Bold).Println("【按账户汇总】")
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"账户", "类型", "买入金额", "卖出金额", "净买入", "买笔", "卖笔", "其它", "费用", "总笔数"})
	table.SetBorder(true)
	table.SetRowLine(true)
	table.SetAutoWrapText(false)

	for i, p := range packs {
		a := aggList[i]
		table.Append([]string{
			p.Account.Name(),
			api.CategoryLabel(p.Account.Category),
			fmt.Sprintf("%.2f", a.BuyAmt),
			fmt.Sprintf("%.2f", a.SellAmt),
			formatter.ColorNum(a.NetBuy()),
			strconv.Itoa(a.BuyN),
			strconv.Itoa(a.SellN),
			strconv.Itoa(a.OtherN),
			fmt.Sprintf("%.2f", a.Fee),
			strconv.Itoa(a.Trades()),
		})
	}
	table.Append([]string{
		"合计",
		"",
		fmt.Sprintf("%.2f", grand.BuyAmt),
		fmt.Sprintf("%.2f", grand.SellAmt),
		formatter.ColorNum(grand.NetBuy()),
		strconv.Itoa(grand.BuyN),
		strconv.Itoa(grand.SellN),
		strconv.Itoa(grand.OtherN),
		fmt.Sprintf("%.2f", grand.Fee),
		strconv.Itoa(grand.Trades()),
	})
	table.Render()
}

// RenderCodeSummary 渲染按代码汇总表格
func (s *TradeRenderService) RenderCodeSummary(all []map[string]any, limit int) {
	byCode := s.tradeService.AggregateByCode(all)

	type row struct {
		code string
		agg  *TradeAgg
	}
	var rows []row
	for c, a := range byCode {
		// 只展示有买卖的代码，纯其它笔可跳过
		if a.BuyN+a.SellN == 0 {
			continue
		}
		rows = append(rows, row{code: c, agg: a})
	}
	// 按买入+卖出金额降序
	sort.Slice(rows, func(i, j int) bool {
		ti := rows[i].agg.BuyAmt + rows[i].agg.SellAmt
		tj := rows[j].agg.BuyAmt + rows[j].agg.SellAmt
		if ti != tj {
			return ti > tj
		}
		return rows[i].code < rows[j].code
	})

	color.New(color.Bold).Println("【按代码汇总】（按买卖金额合计降序）")
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"代码", "名称", "买入金额", "卖出金额", "净买入", "买量", "卖量", "买笔", "卖笔"})
	table.SetBorder(true)
	table.SetRowLine(true)
	table.SetAutoWrapText(false)

	for i, r := range rows {
		if i >= limit {
			break
		}
		table.Append([]string{
			r.code,
			r.agg.Name,
			fmt.Sprintf("%.2f", r.agg.BuyAmt),
			fmt.Sprintf("%.2f", r.agg.SellAmt),
			formatter.ColorNum(r.agg.NetBuy()),
			fmt.Sprintf("%.0f", r.agg.BuyCnt),
			fmt.Sprintf("%.0f", r.agg.SellCnt),
			strconv.Itoa(r.agg.BuyN),
			strconv.Itoa(r.agg.SellN),
		})
	}
	table.Render()
	if len(rows) > limit {
		fmt.Printf("… 另有 %d 个代码未展示（共 %d 个）\n", len(rows)-limit, len(rows))
	}
}

// RenderGrandSummary 渲染总汇总
func (s *TradeRenderService) RenderGrandSummary(all []map[string]any) {
	var g TradeAgg
	for _, r := range all {
		g.AddRecord(r)
	}
	color.New(color.Bold).Printf(
		"汇总  流水: %d 笔 | 买入: %.2f（%d 笔）| 卖出: %.2f（%d 笔）| 净买入: %s | 费用: %.2f | 其它: %d 笔\n",
		len(all),
		g.BuyAmt, g.BuyN,
		g.SellAmt, g.SellN,
		formatter.ColorNum(g.NetBuy()),
		g.Fee,
		g.OtherN,
	)
	fmt.Println("说明: 净买入 = 买入金额 − 卖出金额；正数=净加仓/净投入，负数=净减仓。含逆回购滚动。")
}
