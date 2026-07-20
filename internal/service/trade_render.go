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
