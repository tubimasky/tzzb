package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

var (
	filterAccount string
	filterType    string // all / stock / fund
	flatList      bool
)

var positionCmd = &cobra.Command{
	Use:   "position",
	Short: "持仓管理",
	Long: `查看股票/基金持仓。

子命令:
  list   按账户分块明细
  total  跨账户按代码汇总（同标的合并）
  stock  仅股票/ETF（普通/手动/两融账户）
  fund   仅场外基金账户`,
}

var positionListCmd = &cobra.Command{
	Use:   "list",
	Short: "按账户查看持仓明细",
	Long:  `默认按账户分块展示；加 --flat 打成一张大表（带账户列）。股票行会附带实时今日盈亏。`,
	Run: func(cmd *cobra.Command, args []string) {
		accounts := mustLoadAccounts()
		rows := collectRows(accounts, filterAccount, filterType)
		if len(rows) == 0 {
			fmt.Println("暂无持仓")
			return
		}
		quotes := fetchQuotesForRows(rows)
		if flatList {
			printFlat(rows, quotes)
			return
		}
		printByAccount(rows, quotes)
	},
}

var positionTotalCmd = &cobra.Command{
	Use:   "total",
	Short: "跨账户按代码汇总",
	Long:  `把所有账户里相同代码的持仓合并成一行。股票/ETF 附实时今日盈亏。`,
	Run: func(cmd *cobra.Command, args []string) {
		accounts := mustLoadAccounts()
		rows := collectRows(accounts, filterAccount, filterType)
		if len(rows) == 0 {
			fmt.Println("暂无持仓")
			return
		}
		quotes := fetchQuotesForRows(rows)
		printTotal(mergeByCode(rows), quotes)
	},
}

var positionStockCmd = &cobra.Command{
	Use:   "stock",
	Short: "股票/ETF 持仓（跨账户汇总）",
	Long:  `只看普通/手动/两融账户，按代码跨账户合并，并显示实时今日盈亏。`,
	Run: func(cmd *cobra.Command, args []string) {
		accounts := mustLoadAccounts()
		rows := collectRows(accounts, filterAccount, "stock")
		if len(rows) == 0 {
			fmt.Println("暂无股票/ETF 持仓")
			return
		}
		quotes := fetchQuotesForRows(rows)
		printTotal(mergeByCode(rows), quotes)
	},
}

var positionFundCmd = &cobra.Command{
	Use:   "fund",
	Short: "场外基金持仓（跨账户汇总）",
	Long:  `只看基金账户，按代码跨账户合并。场外基金无实时今日盈亏（盘后净值）。`,
	Run: func(cmd *cobra.Command, args []string) {
		accounts := mustLoadAccounts()
		rows := collectRows(accounts, filterAccount, "fund")
		if len(rows) == 0 {
			fmt.Println("暂无基金持仓")
			return
		}
		printTotal(mergeByCode(rows), nil)
	},
}

// posRow 是一条带账户信息的持仓行。
type posRow struct {
	Account  string
	Category string // manual/fund/rzrq/common
	api.Position
}

type mergedPos struct {
	Code       string
	Name       string
	Count      float64
	Cost       float64 // 数量加权成本
	Price      float64
	Value      float64
	HoldProfit float64
	Market     string   // 用于行情
	Accounts   []string // 出现在哪些账户
}

func mustLoadAccounts() []api.Account {
	accounts, err := apiClient.GetAccountList()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取持仓失败: %v\n", err)
		os.Exit(1)
	}
	return accounts
}

func collectRows(accounts []api.Account, accountFilter, typeFilter string) []posRow {
	typeFilter = strings.ToLower(strings.TrimSpace(typeFilter))
	if typeFilter == "" {
		typeFilter = "all"
	}
	var out []posRow
	for _, acc := range accounts {
		if accountFilter != "" && !strings.Contains(acc.Name(), accountFilter) {
			continue
		}
		if !matchType(acc.Category, typeFilter) {
			continue
		}
		for _, p := range acc.Positions {
			out = append(out, posRow{
				Account:  acc.Name(),
				Category: acc.Category,
				Position: p,
			})
		}
	}
	return out
}

// matchType: stock = 非基金账户；fund = 基金账户；all = 全部。
func matchType(category, typeFilter string) bool {
	switch typeFilter {
	case "all", "":
		return true
	case "stock", "stocks", "etf":
		return category != "fund"
	case "fund", "funds":
		return category == "fund"
	default:
		fmt.Fprintf(os.Stderr, "未知类型 %q，支持 all/stock/fund\n", typeFilter)
		os.Exit(1)
		return false
	}
}

func fetchQuotesForRows(rows []posRow) map[string]api.Quote {
	// 只对股票/ETF（有 market 或非基金账户）拉行情；基金账户行通常无实时价
	var symbols []string
	for _, r := range rows {
		if r.Category == "fund" {
			continue
		}
		code := normalizeCode(r.Code)
		if code == "" {
			continue
		}
		symbols = append(symbols, api.SinaSymbol(code, r.Market))
	}
	if len(symbols) == 0 {
		return nil
	}
	quotes, err := api.FetchQuotes(symbols)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取实时行情失败（今日盈亏将显示 -）: %v\n", err)
		return nil
	}
	return quotes
}

func dayProfitOf(code string, count float64, quotes map[string]api.Quote) (float64, bool) {
	if quotes == nil {
		return 0, false
	}
	q, ok := quotes[normalizeCode(code)]
	if !ok || q.PrevClose == 0 {
		return 0, false
	}
	return q.DayProfit(count), true
}

func dayRateOf(code string, quotes map[string]api.Quote) (float64, bool) {
	if quotes == nil {
		return 0, false
	}
	q, ok := quotes[normalizeCode(code)]
	if !ok || q.PrevClose == 0 {
		return 0, false
	}
	return q.DayRate() * 100, true
}

func livePriceOf(code string, fallback float64, quotes map[string]api.Quote) float64 {
	if quotes != nil {
		if q, ok := quotes[normalizeCode(code)]; ok && q.Price > 0 {
			return q.Price
		}
	}
	return fallback
}

func printByAccount(rows []posRow, quotes map[string]api.Quote) {
	var order []string
	groups := map[string][]posRow{}
	catOf := map[string]string{}
	for _, r := range rows {
		if _, ok := groups[r.Account]; !ok {
			order = append(order, r.Account)
			catOf[r.Account] = r.Category
		}
		groups[r.Account] = append(groups[r.Account], r)
	}

	showDay := quotes != nil
	var grandValue, grandProfit, grandDay float64
	var hasDay bool
	for _, name := range order {
		list := groups[name]
		fmt.Printf("\n【%s · %s】\n", name, api.CategoryLabel(catOf[name]))
		table := tablewriter.NewWriter(os.Stdout)
		if showDay && catOf[name] != "fund" {
			table.SetHeader([]string{"代码", "名称", "持仓量", "成本价", "现价", "市值", "持仓盈亏", "今日盈亏", "今日涨幅", "收益率"})
		} else {
			table.SetHeader([]string{"代码", "名称", "持仓量", "成本价", "现价", "市值", "持仓盈亏", "本周盈亏", "收益率"})
		}
		table.SetBorder(true)

		var subValue, subProfit, subDay float64
		var subHasDay bool
		for _, r := range list {
			subValue += r.Value.F()
			subProfit += r.HoldProfit.F()
			if showDay && catOf[name] != "fund" {
				day, ok := dayProfitOf(r.Code, r.Count.F(), quotes)
				rate, _ := dayRateOf(r.Code, quotes)
				if ok {
					subDay += day
					subHasDay = true
				}
				price := livePriceOf(r.Code, r.Price.F(), quotes)
				table.Append([]string{
					r.Code, r.Name,
					fmt.Sprintf("%.0f", r.Count.F()),
					priceStr(r.Cost.F()),
					priceStr(price),
					fmt.Sprintf("%.2f", r.Value.F()),
					colorNum(r.HoldProfit.F()),
					dayStr(day, ok),
					rateStr(rate, ok),
					colorPct(r.HoldRate.F() * 100),
				})
			} else {
				table.Append(posCells(r.Position, quotes))
			}
		}
		table.Render()
		grandValue += subValue
		grandProfit += subProfit
		if subHasDay {
			grandDay += subDay
			hasDay = true
		}
		if subHasDay {
			fmt.Printf("小计  市值: %.2f | 持仓盈亏: %s | 今日盈亏: %s | 标的: %d\n",
				subValue, colorNum(subProfit), colorNum(subDay), len(list))
		} else {
			fmt.Printf("小计  市值: %.2f | 持仓盈亏: %s | 标的: %d\n",
				subValue, colorNum(subProfit), len(list))
		}
	}

	fmt.Println()
	if hasDay {
		color.New(color.Bold).Printf("合计  市值: %.2f | 持仓盈亏: %s | 今日盈亏: %s | 行数: %d | 账户: %d\n",
			grandValue, colorNum(grandProfit), colorNum(grandDay), len(rows), len(order))
	} else {
		color.New(color.Bold).Printf("合计  市值: %.2f | 持仓盈亏: %s | 行数: %d | 账户: %d\n",
			grandValue, colorNum(grandProfit), len(rows), len(order))
	}
}

func printFlat(rows []posRow, quotes map[string]api.Quote) {
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Value.F() > rows[j].Value.F()
	})

	showDay := quotes != nil
	table := tablewriter.NewWriter(os.Stdout)
	if showDay {
		table.SetHeader([]string{"账户", "类型", "代码", "名称", "持仓量", "成本价", "现价", "市值", "持仓盈亏", "今日盈亏", "今日涨幅", "收益率"})
	} else {
		table.SetHeader([]string{"账户", "类型", "代码", "名称", "持仓量", "成本价", "现价", "市值", "持仓盈亏", "本周盈亏", "收益率"})
	}
	table.SetBorder(true)

	var totalValue, totalProfit, totalDay float64
	var hasDay bool
	for _, r := range rows {
		totalValue += r.Value.F()
		totalProfit += r.HoldProfit.F()
		if showDay && r.Category != "fund" {
			day, ok := dayProfitOf(r.Code, r.Count.F(), quotes)
			rate, _ := dayRateOf(r.Code, quotes)
			if ok {
				totalDay += day
				hasDay = true
			}
			price := livePriceOf(r.Code, r.Price.F(), quotes)
			table.Append([]string{
				r.Account, api.CategoryLabel(r.Category),
				r.Code, r.Name,
				fmt.Sprintf("%.0f", r.Count.F()),
				priceStr(r.Cost.F()),
				priceStr(price),
				fmt.Sprintf("%.2f", r.Value.F()),
				colorNum(r.HoldProfit.F()),
				dayStr(day, ok),
				rateStr(rate, ok),
				colorPct(r.HoldRate.F() * 100),
			})
		} else {
			cells := []string{r.Account, api.CategoryLabel(r.Category)}
			cells = append(cells, posCells(r.Position, quotes)...)
			table.Append(cells)
		}
	}
	table.Render()

	fmt.Println()
	if hasDay {
		color.New(color.Bold).Printf("合计  市值: %.2f | 持仓盈亏: %s | 今日盈亏: %s | 行数: %d\n",
			totalValue, colorNum(totalProfit), colorNum(totalDay), len(rows))
	} else {
		color.New(color.Bold).Printf("合计  市值: %.2f | 持仓盈亏: %s | 行数: %d\n",
			totalValue, colorNum(totalProfit), len(rows))
	}
}

func printTotal(merged []mergedPos, quotes map[string]api.Quote) {
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Value > merged[j].Value
	})

	showDay := quotes != nil
	table := tablewriter.NewWriter(os.Stdout)
	if showDay {
		table.SetHeader([]string{"代码", "名称", "总数量", "均成本", "现价", "总市值", "持仓盈亏", "今日盈亏", "今日涨幅", "收益率", "账户"})
	} else {
		table.SetHeader([]string{"代码", "名称", "总数量", "均成本", "现价", "总市值", "持仓盈亏", "收益率", "账户"})
	}
	table.SetBorder(true)

	var totalValue, totalProfit, totalDay float64
	var hasDay bool
	for _, m := range merged {
		totalValue += m.Value
		totalProfit += m.HoldProfit
		rate := 0.0
		costBasis := m.Value - m.HoldProfit
		if costBasis > 0 {
			rate = m.HoldProfit / costBasis * 100
		}
		price := livePriceOf(m.Code, m.Price, quotes)

		if showDay {
			day, ok := dayProfitOf(m.Code, m.Count, quotes)
			dayRate, rateOK := dayRateOf(m.Code, quotes)
			if ok {
				totalDay += day
				hasDay = true
			}
			table.Append([]string{
				m.Code,
				m.Name,
				fmt.Sprintf("%.0f", m.Count),
				priceStr(m.Cost),
				priceStr(price),
				fmt.Sprintf("%.2f", m.Value),
				colorNum(m.HoldProfit),
				dayStr(day, ok),
				rateStr(dayRate, rateOK),
				colorPct(rate),
				strings.Join(m.Accounts, ","),
			})
		} else {
			table.Append([]string{
				m.Code,
				m.Name,
				fmt.Sprintf("%.0f", m.Count),
				priceStr(m.Cost),
				priceStr(price),
				fmt.Sprintf("%.2f", m.Value),
				colorNum(m.HoldProfit),
				colorPct(rate),
				strings.Join(m.Accounts, ","),
			})
		}
	}
	table.Render()

	fmt.Println()
	if hasDay {
		color.New(color.Bold).Printf("合计  市值: %.2f | 持仓盈亏: %s | 今日盈亏: %s | 标的: %d\n",
			totalValue, colorNum(totalProfit), colorNum(totalDay), len(merged))
	} else {
		color.New(color.Bold).Printf("合计  市值: %.2f | 持仓盈亏: %s | 标的: %d\n",
			totalValue, colorNum(totalProfit), len(merged))
	}
}

func mergeByCode(rows []posRow) []mergedPos {
	type acc struct {
		m        mergedPos
		costSum  float64
		countSum float64
	}
	byCode := map[string]*acc{}
	var order []string

	for _, r := range rows {
		code := normalizeCode(r.Code)
		a, ok := byCode[code]
		if !ok {
			a = &acc{
				m: mergedPos{
					Code:   code,
					Name:   r.Name,
					Market: r.Market,
				},
			}
			byCode[code] = a
			order = append(order, code)
		}
		if a.m.Name == "" && r.Name != "" {
			a.m.Name = r.Name
		}
		if a.m.Market == "" && r.Market != "" {
			a.m.Market = r.Market
		}
		if r.Price.F() > 0 {
			a.m.Price = r.Price.F()
		}
		a.m.Count += r.Count.F()
		a.m.Value += r.Value.F()
		a.m.HoldProfit += r.HoldProfit.F()
		a.costSum += r.Cost.F() * r.Count.F()
		a.countSum += r.Count.F()
	}

	out := make([]mergedPos, 0, len(order))
	for _, code := range order {
		a := byCode[code]
		if a.countSum > 0 {
			a.m.Cost = a.costSum / a.countSum
		}
		var accs []string
		seen := map[string]bool{}
		for _, r := range rows {
			if normalizeCode(r.Code) != code {
				continue
			}
			if !seen[r.Account] {
				seen[r.Account] = true
				accs = append(accs, r.Account)
			}
		}
		a.m.Accounts = accs
		out = append(out, a.m)
	}
	return out
}

// normalizeCode 去掉市场后缀，便于跨账户合并（515790.SZ → 515790）。
func normalizeCode(code string) string {
	code = strings.TrimSpace(code)
	if i := strings.IndexAny(code, "."); i > 0 {
		return code[:i]
	}
	return code
}

func posCells(p api.Position, quotes map[string]api.Quote) []string {
	price := livePriceOf(p.Code, p.Price.F(), quotes)
	return []string{
		p.Code,
		p.Name,
		fmt.Sprintf("%.0f", p.Count.F()),
		priceStr(p.Cost.F()),
		priceStr(price),
		fmt.Sprintf("%.2f", p.Value.F()),
		colorNum(p.HoldProfit.F()),
		colorNum(p.WProfit.F()),
		colorPct(p.HoldRate.F() * 100),
	}
}

// priceStr 成本/现价统一三位小数；0 显示为 "-"。
func priceStr(v float64) string {
	if v <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.3f", v)
}

func dayStr(v float64, ok bool) string {
	if !ok {
		return "-"
	}
	return colorNum(v)
}

func rateStr(v float64, ok bool) string {
	if !ok {
		return "-"
	}
	return colorPct(v)
}

func colorPct(v float64) string {
	if v < 0 {
		return color.New(color.FgGreen).Sprintf("%.2f%%", v)
	}
	return color.New(color.FgRed).Sprintf("%.2f%%", v)
}

func init() {
	positionCmd.AddCommand(positionListCmd)
	positionCmd.AddCommand(positionTotalCmd)
	positionCmd.AddCommand(positionStockCmd)
	positionCmd.AddCommand(positionFundCmd)

	positionListCmd.Flags().StringVarP(&filterAccount, "account", "a", "", "按账户名过滤（模糊匹配）")
	positionListCmd.Flags().StringVarP(&filterType, "type", "t", "all", "类型: all / stock / fund")
	positionListCmd.Flags().BoolVar(&flatList, "flat", false, "扁平大表（带账户列，不按账户分块）")

	for _, c := range []*cobra.Command{positionTotalCmd, positionStockCmd, positionFundCmd} {
		c.Flags().StringVarP(&filterAccount, "account", "a", "", "按账户名过滤（模糊匹配）")
	}
	positionTotalCmd.Flags().StringVarP(&filterType, "type", "t", "all", "类型: all / stock / fund")
}
