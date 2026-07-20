package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

var tradeAccount string

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
		matched := mustMatchTradeAccounts()

		if outputJSON {
			var allResults []map[string]any
			for _, acc := range matched {
				records, err := apiClient.GetMoneyHistory(acc)
				if err != nil {
					fmt.Fprintf(os.Stderr, "【%s】获取流水失败: %v\n", acc.Name(), err)
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
				printJSON([]any{})
			} else {
				printJSON(allResults)
			}
			return
		}

		var total int
		for _, acc := range matched {
			records, err := apiClient.GetMoneyHistory(acc)
			if err != nil {
				fmt.Fprintf(os.Stderr, "【%s】获取流水失败: %v\n", acc.Name(), err)
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
		matched := mustMatchTradeAccounts()
		var packs []accPack
		var all []map[string]any
		for _, acc := range matched {
			records, err := apiClient.GetMoneyHistory(acc)
			if err != nil {
				fmt.Fprintf(os.Stderr, "【%s】获取流水失败: %v\n", acc.Name(), err)
				continue
			}
			if len(records) == 0 {
				continue
			}
			for i := range records {
				if records[i] == nil {
					records[i] = map[string]any{}
				}
				records[i]["_account"] = acc.Name()
			}
			packs = append(packs, accPack{acc: acc, records: records})
			all = append(all, records...)
		}
		if len(all) == 0 {
			if outputJSON {
				printJSON(map[string]any{"error": "no trade records"})
			} else {
				fmt.Println("暂无交易记录")
			}
			return
		}

		if outputJSON {
			printTradeSummaryJSON(packs, all)
			return
		}

		printTradeAccountSummary(packs)
		fmt.Println()
		printTradeCodeSummary(all)
		fmt.Println()
		printTradeGrandSummary(all)
	},
}

func mustMatchTradeAccounts() []api.Account {
	accounts, err := apiClient.GetAccountList()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取账户失败: %v\n", err)
		os.Exit(1)
	}
	var matched []api.Account
	for _, acc := range accounts {
		if tradeAccount == "" || strings.Contains(acc.Name(), tradeAccount) {
			matched = append(matched, acc)
		}
	}
	if len(matched) == 0 {
		fmt.Println("没有匹配的账户")
		os.Exit(0)
	}
	return matched
}

// tradeAgg 买卖汇总。
type tradeAgg struct {
	buyAmt, sellAmt, fee float64
	buyCnt, sellCnt      float64
	buyN, sellN, otherN  int
	name                 string
}

func (a *tradeAgg) add(r map[string]any) {
	op := strings.TrimSpace(fmt.Sprintf("%v", r["op_name"]))
	money := mapFloat(r, "entry_money")
	cnt := mapFloat(r, "entry_count")
	fee := mapFloat(r, "fee")
	if fee == 0 {
		fee = mapFloat(r, "commission")
	}
	a.fee += fee

	// 银证转账单独计入 other，不进买卖金额
	if strings.Contains(op, "银行") {
		a.otherN++
		return
	}
	switch {
	case strings.Contains(op, "买") || op == "转入" || op == "担保品转入":
		a.buyAmt += money
		a.buyCnt += cnt
		a.buyN++
	case strings.Contains(op, "卖") || op == "转出" || op == "担保品转出":
		a.sellAmt += money
		a.sellCnt += cnt
		a.sellN++
	default:
		a.otherN++
	}
	if a.name == "" {
		if n := strings.TrimSpace(fmt.Sprintf("%v", r["name"])); n != "" && n != "<nil>" {
			a.name = n
		}
	}
}

// netBuy 净买入金额 = 买入 − 卖出。正=净加仓/净投入，负=净减仓。
func (a tradeAgg) netBuy() float64 { return a.buyAmt - a.sellAmt }
func (a tradeAgg) trades() int     { return a.buyN + a.sellN + a.otherN }

type accPack struct {
	acc     api.Account
	records []map[string]any
}

func printTradeAccountSummary(packs []accPack) {
	color.New(color.Bold).Println("【按账户汇总】")
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"账户", "类型", "买入金额", "卖出金额", "净买入", "买笔", "卖笔", "其它", "费用", "总笔数"})
	table.SetBorder(true)
	table.SetRowLine(true)
	table.SetAutoWrapText(false)

	var g tradeAgg
	for _, p := range packs {
		var a tradeAgg
		for _, r := range p.records {
			a.add(r)
		}
		g.buyAmt += a.buyAmt
		g.sellAmt += a.sellAmt
		g.fee += a.fee
		g.buyN += a.buyN
		g.sellN += a.sellN
		g.otherN += a.otherN
		table.Append([]string{
			p.acc.Name(),
			api.CategoryLabel(p.acc.Category),
			fmt.Sprintf("%.2f", a.buyAmt),
			fmt.Sprintf("%.2f", a.sellAmt),
			colorNum(a.netBuy()),
			strconv.Itoa(a.buyN),
			strconv.Itoa(a.sellN),
			strconv.Itoa(a.otherN),
			fmt.Sprintf("%.2f", a.fee),
			strconv.Itoa(a.trades()),
		})
	}
	table.Append([]string{
		"合计",
		"",
		fmt.Sprintf("%.2f", g.buyAmt),
		fmt.Sprintf("%.2f", g.sellAmt),
		colorNum(g.netBuy()),
		strconv.Itoa(g.buyN),
		strconv.Itoa(g.sellN),
		strconv.Itoa(g.otherN),
		fmt.Sprintf("%.2f", g.fee),
		strconv.Itoa(g.trades()),
	})
	table.Render()
}

func printTradeCodeSummary(all []map[string]any) {
	byCode := map[string]*tradeAgg{}
	for _, r := range all {
		code := strings.TrimSpace(fmt.Sprintf("%v", r["code"]))
		if code == "" || code == "<nil>" {
			code = "(无代码)"
		}
		a, ok := byCode[code]
		if !ok {
			a = &tradeAgg{}
			byCode[code] = a
		}
		a.add(r)
	}

	type row struct {
		code string
		agg  *tradeAgg
	}
	var rows []row
	for c, a := range byCode {
		// 只展示有买卖的代码，纯其它笔可跳过
		if a.buyN+a.sellN == 0 {
			continue
		}
		rows = append(rows, row{code: c, agg: a})
	}
	// 按买入+卖出金额降序
	sort.Slice(rows, func(i, j int) bool {
		ti := rows[i].agg.buyAmt + rows[i].agg.sellAmt
		tj := rows[j].agg.buyAmt + rows[j].agg.sellAmt
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
			r.agg.name,
			fmt.Sprintf("%.2f", r.agg.buyAmt),
			fmt.Sprintf("%.2f", r.agg.sellAmt),
			colorNum(r.agg.netBuy()),
			fmt.Sprintf("%.0f", r.agg.buyCnt),
			fmt.Sprintf("%.0f", r.agg.sellCnt),
			strconv.Itoa(r.agg.buyN),
			strconv.Itoa(r.agg.sellN),
		})
	}
	table.Render()
	if len(rows) > limit {
		fmt.Printf("… 另有 %d 个代码未展示（共 %d 个）\n", len(rows)-limit, len(rows))
	}
}

func printTradeGrandSummary(all []map[string]any) {
	var g tradeAgg
	for _, r := range all {
		g.add(r)
	}
	color.New(color.Bold).Printf(
		"汇总  流水: %d 笔 | 买入: %.2f（%d 笔）| 卖出: %.2f（%d 笔）| 净买入: %s | 费用: %.2f | 其它: %d 笔\n",
		len(all),
		g.buyAmt, g.buyN,
		g.sellAmt, g.sellN,
		colorNum(g.netBuy()),
		g.fee,
		g.otherN,
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

func mapFloat(r map[string]any, key string) float64 {
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
		s := strings.TrimSpace(fmt.Sprintf("%v", v))
		s = strings.ReplaceAll(s, ",", "")
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0
		}
		return f
	}
}

func init() {
	tradeCmd.AddCommand(tradeListCmd)
	tradeCmd.AddCommand(tradeSummaryCmd)
	tradeListCmd.Flags().StringVarP(&tradeAccount, "account", "a", "", "按账户名过滤（模糊匹配），默认全部")
	tradeSummaryCmd.Flags().StringVarP(&tradeAccount, "account", "a", "", "按账户名过滤（模糊匹配），默认全部")
}

// JSON 输出函数

func printTradeSummaryJSON(packs []accPack, all []map[string]any) {
	result := map[string]any{}

	// 按账户汇总
	var accountSummary []map[string]any
	var g tradeAgg
	for _, p := range packs {
		var a tradeAgg
		for _, r := range p.records {
			a.add(r)
		}
		g.buyAmt += a.buyAmt
		g.sellAmt += a.sellAmt
		g.fee += a.fee
		g.buyN += a.buyN
		g.sellN += a.sellN
		g.otherN += a.otherN

		accountSummary = append(accountSummary, map[string]any{
			"account":     p.acc.Name(),
			"category":    api.CategoryLabel(p.acc.Category),
			"buy_amount":  a.buyAmt,
			"sell_amount": a.sellAmt,
			"net_buy":     a.netBuy(),
			"buy_count":   a.buyN,
			"sell_count":  a.sellN,
			"other_count": a.otherN,
			"fee":         a.fee,
			"total_count": a.trades(),
		})
	}
	result["by_account"] = accountSummary

	// 按代码汇总
	byCode := map[string]*tradeAgg{}
	for _, r := range all {
		code := strings.TrimSpace(fmt.Sprintf("%v", r["code"]))
		if code == "" || code == "<nil>" {
			code = "(无代码)"
		}
		a, ok := byCode[code]
		if !ok {
			a = &tradeAgg{}
			byCode[code] = a
		}
		a.add(r)
	}

	var codeSummary []map[string]any
	for code, a := range byCode {
		if a.buyN+a.sellN == 0 {
			continue
		}
		codeSummary = append(codeSummary, map[string]any{
			"code":        code,
			"name":        a.name,
			"buy_amount":  a.buyAmt,
			"sell_amount": a.sellAmt,
			"net_buy":     a.netBuy(),
			"buy_volume":  a.buyCnt,
			"sell_volume": a.sellCnt,
			"buy_count":   a.buyN,
			"sell_count":  a.sellN,
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
		"buy_amount":    g.buyAmt,
		"buy_count":     g.buyN,
		"sell_amount":   g.sellAmt,
		"sell_count":    g.sellN,
		"net_buy":       g.netBuy(),
		"fee":           g.fee,
		"other_count":   g.otherN,
	}

	printJSON(result)
}
