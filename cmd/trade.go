package cmd

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/internal/formatter"
	"github.com/liyanran/tzzb-cli/internal/service"
	"github.com/liyanran/tzzb-cli/pkg/api"
)

var tradeAccount string

func getTradeService() *service.TradeService {
	return service.NewTradeService(apiClient)
}

var tradeCmd = &cobra.Command{
	Use:   "trade",
	Short: "交易记录",
	Long:  `查看账户的资金/交易流水（近 3 个月）`,
}

var tradeListCmd = &cobra.Command{
	Use:   "list",
	Short: "查看交易明细",
	Long:  `按账户列出近 3 个月流水明细。可用 -a 过滤账户。`,
	Run: func(cmd *cobra.Command, args []string) {
		svc := getTradeService()
		matched, err := svc.MatchTradeAccounts(tradeAccount)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		if len(matched) == 0 {
			fmt.Println("没有匹配的账户")
			os.Exit(0)
		}

		if outputJSON {
			var allResults []map[string]any
			for _, acc := range matched {
				records, err := svc.LoadTradeRecords(acc)
				if err != nil {
					fmt.Fprintf(os.Stderr, "【%s】%v\n", acc.Name(), err)
					continue
				}
				if len(records) == 0 {
					continue
				}
				allResults = append(allResults, map[string]any{
					"account":  acc.Name(),
					"category": api.CategoryLabel(acc.Category),
					"count":    len(records),
					"records":  records,
				})
			}
			if len(allResults) == 0 {
				formatter.PrintJSON([]any{})
			} else {
				formatter.PrintJSON(allResults)
			}
			return
		}

		var total int
		for _, acc := range matched {
			records, err := svc.LoadTradeRecords(acc)
			if err != nil {
				fmt.Fprintf(os.Stderr, "【%s】%v\n", acc.Name(), err)
				continue
			}
			if len(records) == 0 {
				continue
			}
			total += len(records)
			fmt.Printf("\n【%s · %s】共 %d 条\n", acc.Name(), api.CategoryLabel(acc.Category), len(records))
			renderRecords(records)
		}
		if total == 0 {
			fmt.Println("暂无交易记录")
		}
	},
}

var tradeSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "交易汇总",
	Long: `汇总近 3 个月流水：
  1) 按账户：买入/卖出金额、净买入、笔数、费用
  2) 按代码：买入/卖出金额与数量、净买入、笔数

金额口径：净买入 = 买入金额 − 卖出金额（正数=净加仓/净投入，负数=净减仓）。
逆回购/货基等也会计入对应买卖（与流水 op_name 一致）。`,
	Run: func(cmd *cobra.Command, args []string) {
		svc := getTradeService()
		matched, err := svc.MatchTradeAccounts(tradeAccount)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		if len(matched) == 0 {
			fmt.Println("没有匹配的账户")
			os.Exit(0)
		}

		packs := svc.LoadAllTradeRecords(matched)
		if len(packs) == 0 {
			if outputJSON {
				formatter.PrintJSON(map[string]any{"error": "no trade records"})
			} else {
				fmt.Println("暂无交易记录")
			}
			return
		}

		all := svc.FlattenRecords(packs)
		if outputJSON {
			printTradeSummaryJSON(svc, packs, all)
			return
		}

		printTradeAccountSummary(svc, packs)
		fmt.Println()
		printTradeCodeSummary(svc, all)
		fmt.Println()
		printTradeGrandSummary(all)
	},
}

// 已废弃：使用 svc.MatchTradeAccounts() 替代
func mustMatchTradeAccounts() []api.Account {
	svc := getTradeService()
	matched, err := svc.MatchTradeAccounts(tradeAccount)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if len(matched) == 0 {
		fmt.Println("没有匹配的账户")
		os.Exit(0)
	}
	return matched
}

func printTradeAccountSummary(svc *service.TradeService, packs []service.AccountPack) {
	color.New(color.Bold).Println("【按账户汇总】")
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"账户", "类型", "买入金额", "卖出金额", "净买入", "买笔", "卖笔", "其它", "费用", "总笔数"})
	table.SetBorder(true)
	table.SetRowLine(true)
	table.SetAutoWrapText(false)

	aggList, grand := svc.AggregateByAccount(packs)
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

func printTradeCodeSummary(svc *service.TradeService, all []map[string]any) {
	byCode := svc.AggregateByCode(all)

	type row struct {
		code string
		agg  *service.TradeAgg
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

	limit := 40
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

func printTradeGrandSummary(all []map[string]any) {
	var g service.TradeAgg
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

// renderRecords 用固定中文列展示流水（字段名随接口映射）。
func renderRecords(records []map[string]any) {
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
		renderRecordsDynamic(records)
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
			row[i] = pickField(r, c.keys...)
		}
		table.Append(row)
	}
	table.Render()
}

func renderRecordsDynamic(records []map[string]any) {
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

func pickField(r map[string]any, keys ...string) string {
	return service.PickField(r, keys...)
}

func mapFloat(r map[string]any, key string) float64 {
	return service.MapFloat(r, key)
}

func init() {
	tradeCmd.AddCommand(tradeListCmd)
	tradeCmd.AddCommand(tradeSummaryCmd)
	tradeListCmd.Flags().StringVarP(&tradeAccount, "account", "a", "", "按账户名过滤（模糊匹配），默认全部")
	tradeSummaryCmd.Flags().StringVarP(&tradeAccount, "account", "a", "", "按账户名过滤（模糊匹配），默认全部")
}

// JSON 输出函数

func printTradeSummaryJSON(svc *service.TradeService, packs []service.AccountPack, all []map[string]any) {
	result := map[string]any{}

	// 按账户汇总
	var accountSummary []map[string]any
	aggList, grand := svc.AggregateByAccount(packs)
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
	byCode := svc.AggregateByCode(all)
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

	formatter.PrintJSON(result)
}
