package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/internal/formatter"
	"github.com/liyanran/tzzb-cli/pkg/api"
)

var (
	filterAccount string
	filterType    string // all / stock / fund
	flatList      bool
	sortBy        string // value/day/hold/rate/dayrate/code/name/count
	sortAsc       bool
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
	Long:  `默认按账户分块展示；加 --flat 打成一张大表（带账户列）。股票行按插件口径重算实时今日盈亏。`,
	Run: func(cmd *cobra.Command, args []string) {
		accounts := mustLoadAccounts()
		rows := collectRows(accounts, filterAccount, filterType)
		if len(rows) == 0 {
			if outputJSON {
				formatter.PrintJSON([]any{})
			} else {
				fmt.Println("暂无持仓")
			}
			return
		}
		ctx := loadLiveContext(accounts, rows)
		if outputJSON {
			printPositionListJSON(rows, ctx, flatList)
			return
		}
		if flatList {
			printFlat(rows, ctx)
			return
		}
		printByAccount(rows, ctx)
	},
}

var positionTotalCmd = &cobra.Command{
	Use:   "total",
	Short: "跨账户按代码汇总",
	Long: `把所有账户里相同代码的持仓合并成一行。股票/ETF 按插件口径重算实时今日盈亏。

排序（-s/--sort）: value(默认) day hold rate dayrate code name count
升序加 --asc，默认降序。`,
	Run: func(cmd *cobra.Command, args []string) {
		accounts := mustLoadAccounts()
		rows := collectRows(accounts, filterAccount, filterType)
		if len(rows) == 0 {
			if outputJSON {
				formatter.PrintJSON([]any{})
			} else {
				fmt.Println("暂无持仓")
			}
			return
		}
		ctx := loadLiveContext(accounts, rows)
		merged := mergeByCode(rows)
		if outputJSON {
			printPositionTotalJSON(merged, ctx)
			return
		}
		printTotal(merged, ctx)
	},
}

var positionStockCmd = &cobra.Command{
	Use:   "stock",
	Short: "股票/ETF 持仓（跨账户汇总）",
	Long: `只看普通/手动/两融账户，按代码跨账户合并。
现价/市值/持仓盈亏/今日盈亏/收益率按同花顺 getQuotes + 当日成交重算（对齐插件 stockCalc）。

排序（-s/--sort）: value(默认) day hold rate dayrate code name count
升序加 --asc，默认降序。

示例:
  tzzb position stock -s day
  tzzb position stock -s hold --asc
  tzzb position stock -s dayrate`,
	Run: func(cmd *cobra.Command, args []string) {
		accounts := mustLoadAccounts()
		rows := collectRows(accounts, filterAccount, "stock")
		if len(rows) == 0 {
			if outputJSON {
				formatter.PrintJSON([]any{})
			} else {
				fmt.Println("暂无股票/ETF 持仓")
			}
			return
		}
		ctx := loadLiveContext(accounts, rows)
		merged := mergeByCode(rows)
		if outputJSON {
			printPositionTotalJSON(merged, ctx)
			return
		}
		printTotal(merged, ctx)
	},
}

var positionFundCmd = &cobra.Command{
	Use:   "fund",
	Short: "场外基金持仓（跨账户汇总）",
	Long: `只看基金账户，按代码跨账户合并。场外基金无实时今日盈亏（盘后净值）。

排序（-s/--sort）: value(默认) hold rate code name count
升序加 --asc。`,
	Run: func(cmd *cobra.Command, args []string) {
		accounts := mustLoadAccounts()
		rows := collectRows(accounts, filterAccount, "fund")
		if len(rows) == 0 {
			if outputJSON {
				formatter.PrintJSON([]any{})
			} else {
				fmt.Println("暂无基金持仓")
			}
			return
		}
		merged := mergeByCode(rows)
		if outputJSON {
			printPositionTotalJSON(merged, nil)
			return
		}
		printTotal(merged, nil)
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

// liveContext 插件同源实时上下文：getQuotes + 各账户当日成交。
type liveContext struct {
	quotes map[string]api.Quote            // code → quote
	trades map[string][]api.DayTrade       // accountName → trades
	byCode map[string][]api.DayTrade       // code → 跨账户合并成交（total/stock 用）
}

func loadLiveContext(accounts []api.Account, rows []posRow) *liveContext {
	ctx := &liveContext{
		quotes: map[string]api.Quote{},
		trades: map[string][]api.DayTrade{},
		byCode: map[string][]api.DayTrade{},
	}
	// 行情
	var refs []api.QuoteRef
	seenRef := map[string]bool{}
	for _, r := range rows {
		if r.Category == "fund" {
			continue
		}
		code := normalizeCode(r.Code)
		if code == "" {
			continue
		}
		mkt := api.GuessMarket(code, r.Market)
		key := mkt + ":" + code
		if seenRef[key] {
			continue
		}
		seenRef[key] = true
		refs = append(refs, api.QuoteRef{Code: code, Market: mkt})
	}
	if len(refs) > 0 {
		q, err := apiClient.GetQuotes(refs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取实时行情失败（今日盈亏将显示 -）: %v\n", err)
		} else {
			ctx.quotes = q
		}
	}
	// 当日成交：只拉 rows 里出现过的非基金账户
	needAcc := map[string]bool{}
	for _, r := range rows {
		if r.Category != "fund" {
			needAcc[r.Account] = true
		}
	}
	for _, acc := range accounts {
		name := acc.Name()
		if !needAcc[name] || acc.Category == "fund" {
			continue
		}
		ts, err := apiClient.GetDayTrades(acc)
		if err != nil {
			// 单个账户失败不阻断
			continue
		}
		ctx.trades[name] = ts
		for _, t := range ts {
			ctx.byCode[t.Code] = append(ctx.byCode[t.Code], t)
		}
	}
	return ctx
}

func quoteOf(ctx *liveContext, code string) *api.Quote {
	if ctx == nil || ctx.quotes == nil {
		return nil
	}
	q, ok := ctx.quotes[normalizeCode(code)]
	if !ok {
		return nil
	}
	return &q
}

func recomputePos(r posRow, ctx *liveContext) api.LiveView {
	var trades []api.DayTrade
	if ctx != nil && r.Category != "fund" {
		for _, t := range ctx.trades[r.Account] {
			if t.Code == normalizeCode(r.Code) {
				trades = append(trades, t)
			}
		}
	}
	q := quoteOf(ctx, r.Code)
	if r.Category == "fund" {
		q = nil
		trades = nil
	}
	return api.RecomputeLive(
		r.Count.F(), r.Cost.F(), r.Price.F(), r.Value.F(),
		r.HoldProfit.F(), r.HoldRate.F(), q, trades,
	)
}

func recomputeMerged(m mergedPos, ctx *liveContext) api.LiveView {
	apiHoldRate := 0.0
	costBasis := m.Cost * m.Count
	if costBasis > 0 {
		apiHoldRate = m.HoldProfit / costBasis
	}
	var trades []api.DayTrade
	if ctx != nil {
		trades = ctx.byCode[normalizeCode(m.Code)]
	}
	return api.RecomputeLive(
		m.Count, m.Cost, m.Price, m.Value, m.HoldProfit, apiHoldRate,
		quoteOf(ctx, m.Code), trades,
	)
}

func printByAccount(rows []posRow, ctx *liveContext) {
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

	showDay := ctx != nil && len(ctx.quotes) > 0
	var grandValue, grandProfit, grandDay float64
	var hasDay bool
	for _, name := range order {
		list := groups[name]
		fmt.Printf("\n【%s · %s】\n", name, api.CategoryLabel(catOf[name]))
		table := tablewriter.NewWriter(os.Stdout)
		useLive := showDay && catOf[name] != "fund"
		if useLive {
			table.SetHeader([]string{"代码", "名称", "持仓量", "成本价", "现价", "市值", "持仓盈亏", "今日盈亏", "今日涨幅", "收益率"})
		} else {
			table.SetHeader([]string{"代码", "名称", "持仓量", "成本价", "现价", "市值", "持仓盈亏", "本周盈亏", "收益率"})
		}
		table.SetBorder(true)

		var subValue, subProfit, subDay float64
		var subHasDay bool
		for _, r := range list {
			if useLive {
				lv := recomputePos(r, ctx)
				subValue += lv.Value
				subProfit += lv.HoldProfit
				if lv.DayOK {
					subDay += lv.DayProfit
					subHasDay = true
				}
				table.Append([]string{
					r.Code, r.Name,
					fmt.Sprintf("%.0f", r.Count.F()),
					formatter.PriceStr(r.Cost.F()),
					formatter.PriceStr(lv.Price),
					fmt.Sprintf("%.2f", lv.Value),
					formatter.ColorNum(lv.HoldProfit),
					formatter.DayStr(lv.DayProfit, lv.DayOK),
					formatter.RateStr(lv.DayRate, lv.DayOK),
					formatter.ColorPct(lv.HoldRate),
				})
			} else {
				lv := recomputePos(r, nil)
				subValue += lv.Value
				subProfit += lv.HoldProfit
				table.Append(posCells(r, nil))
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
				subValue, formatter.ColorNum(subProfit), formatter.ColorNum(subDay), len(list))
		} else {
			fmt.Printf("小计  市值: %.2f | 持仓盈亏: %s | 标的: %d\n",
				subValue, formatter.ColorNum(subProfit), len(list))
		}
	}

	fmt.Println()
	if hasDay {
		color.New(color.Bold).Printf("合计  市值: %.2f | 持仓盈亏: %s | 今日盈亏: %s | 行数: %d | 账户: %d\n",
			grandValue, formatter.ColorNum(grandProfit), formatter.ColorNum(grandDay), len(rows), len(order))
	} else {
		color.New(color.Bold).Printf("合计  市值: %.2f | 持仓盈亏: %s | 行数: %d | 账户: %d\n",
			grandValue, formatter.ColorNum(grandProfit), len(rows), len(order))
	}
}

func printFlat(rows []posRow, ctx *liveContext) {
	type flatView struct {
		r  posRow
		lv api.LiveView
	}
	views := make([]flatView, 0, len(rows))
	for _, r := range rows {
		c := ctx
		if r.Category == "fund" {
			c = nil
		}
		views = append(views, flatView{r: r, lv: recomputePos(r, c)})
	}
	sort.Slice(views, func(i, j int) bool {
		return views[i].lv.Value > views[j].lv.Value
	})

	showDay := ctx != nil && len(ctx.quotes) > 0
	table := tablewriter.NewWriter(os.Stdout)
	if showDay {
		table.SetHeader([]string{"账户", "类型", "代码", "名称", "持仓量", "成本价", "现价", "市值", "持仓盈亏", "今日盈亏", "今日涨幅", "收益率"})
	} else {
		table.SetHeader([]string{"账户", "类型", "代码", "名称", "持仓量", "成本价", "现价", "市值", "持仓盈亏", "本周盈亏", "收益率"})
	}
	table.SetBorder(true)

	var totalValue, totalProfit, totalDay float64
	var hasDay bool
	for _, v := range views {
		r, lv := v.r, v.lv
		totalValue += lv.Value
		totalProfit += lv.HoldProfit
		if showDay && r.Category != "fund" {
			if lv.DayOK {
				totalDay += lv.DayProfit
				hasDay = true
			}
			table.Append([]string{
				r.Account, api.CategoryLabel(r.Category),
				r.Code, r.Name,
				fmt.Sprintf("%.0f", r.Count.F()),
				formatter.PriceStr(r.Cost.F()),
				formatter.PriceStr(lv.Price),
				fmt.Sprintf("%.2f", lv.Value),
				formatter.ColorNum(lv.HoldProfit),
				formatter.DayStr(lv.DayProfit, lv.DayOK),
				formatter.RateStr(lv.DayRate, lv.DayOK),
				formatter.ColorPct(lv.HoldRate),
			})
		} else {
			cells := []string{r.Account, api.CategoryLabel(r.Category)}
			cells = append(cells, posCells(r, nil)...)
			table.Append(cells)
		}
	}
	table.Render()

	fmt.Println()
	if hasDay {
		color.New(color.Bold).Printf("合计  市值: %.2f | 持仓盈亏: %s | 今日盈亏: %s | 行数: %d\n",
			totalValue, formatter.ColorNum(totalProfit), formatter.ColorNum(totalDay), len(rows))
	} else {
		color.New(color.Bold).Printf("合计  市值: %.2f | 持仓盈亏: %s | 行数: %d\n",
			totalValue, formatter.ColorNum(totalProfit), len(rows))
	}
}

// totalRowView 汇总表一行（含排序用的派生字段，已按插件口径重算）。
type totalRowView struct {
	m       mergedPos
	lv      api.LiveView
	price   float64
	day     float64
	dayOK   bool
	dayRate float64
	rateOK  bool
	rate    float64
	value   float64
	hold    float64
}

func printTotal(merged []mergedPos, ctx *liveContext) {
	views := make([]totalRowView, 0, len(merged))
	var totalValue, totalProfit, totalDay, totalCount, totalCostBasis, totalSell float64
	var hasDay bool
	for _, m := range merged {
		lv := recomputeMerged(m, ctx)
		totalValue += lv.Value
		totalProfit += lv.HoldProfit
		totalCount += m.Count
		totalCostBasis += m.Cost * m.Count
		totalSell += lv.SellAmount
		if lv.DayOK {
			totalDay += lv.DayProfit
			hasDay = true
		}
		// 行内「今日涨幅」= 价格涨跌幅；有昨收即可显示（含 0.00%）
		rateOK := false
		if q := quoteOf(ctx, m.Code); q != nil && q.PrevClose > 0 && lv.Price > 0 {
			rateOK = true
		}
		views = append(views, totalRowView{
			m: m, lv: lv,
			price: lv.Price, day: lv.DayProfit, dayOK: lv.DayOK,
			dayRate: lv.DayRate, rateOK: rateOK,
			rate: lv.HoldRate, value: lv.Value, hold: lv.HoldProfit,
		})
	}

	sortTotalViews(views)

	// 合计「收益率」= 总持仓盈亏 / 总成本
	totalRate := 0.0
	if totalCostBasis != 0 {
		totalRate = totalProfit / totalCostBasis * 100
	}
	// 合计行「今日涨幅」= 组合持仓当日盈亏比（相对总市值，非账户总资产）：
	//   总今日盈亏 / (总市值 − 总今日盈亏 + 总卖出额)
	// 与单票 drykb 同一套分母；现金不摊薄。账本 now_rate 用总资产，口径不同。
	totalDayRate := 0.0
	totalDayRateOK := false
	if hasDay {
		den := totalValue - totalDay + totalSell
		if den != 0 {
			totalDayRate = totalDay / den * 100
			totalDayRateOK = true
		}
	}

	showDay := ctx != nil && len(ctx.quotes) > 0
	table := tablewriter.NewWriter(os.Stdout)
	if showDay {
		table.SetHeader([]string{"代码", "名称", "总数量", "均成本", "现价", "总市值", "占比", "持仓盈亏", "今日盈亏", "今日涨幅", "收益率", "账户"})
	} else {
		table.SetHeader([]string{"代码", "名称", "总数量", "均成本", "现价", "总市值", "占比", "持仓盈亏", "收益率", "账户"})
	}
	table.SetBorder(true)
	table.SetRowLine(false)

	for _, v := range views {
		m := v.m
		weight := formatter.WeightStr(v.value, totalValue)
		if showDay {
			table.Append([]string{
				m.Code,
				m.Name,
				fmt.Sprintf("%.0f", m.Count),
				formatter.PriceStr(m.Cost),
				formatter.PriceStr(v.price),
				fmt.Sprintf("%.2f", v.value),
				weight,
				formatter.ColorNum(v.hold),
				formatter.DayStr(v.day, v.dayOK),
				formatter.RateStr(v.dayRate, v.rateOK),
				formatter.ColorPct(v.rate),
				strings.Join(m.Accounts, ","),
			})
		} else {
			table.Append([]string{
				m.Code,
				m.Name,
				fmt.Sprintf("%.0f", m.Count),
				formatter.PriceStr(m.Cost),
				formatter.PriceStr(v.price),
				fmt.Sprintf("%.2f", v.value),
				weight,
				formatter.ColorNum(v.hold),
				formatter.ColorPct(v.rate),
				strings.Join(m.Accounts, ","),
			})
		}
	}

	if showDay {
		table.Append([]string{
			"合计",
			fmt.Sprintf("%d只", len(merged)),
			fmt.Sprintf("%.0f", totalCount),
			"-",
			"-",
			fmt.Sprintf("%.2f", totalValue),
			formatter.WeightStr(totalValue, totalValue),
			formatter.ColorNum(totalProfit),
			formatter.DayStr(totalDay, hasDay),
			formatter.RateStr(totalDayRate, totalDayRateOK),
			formatter.ColorPct(totalRate),
			"",
		})
	} else {
		table.Append([]string{
			"合计",
			fmt.Sprintf("%d只", len(merged)),
			fmt.Sprintf("%.0f", totalCount),
			"-",
			"-",
			fmt.Sprintf("%.2f", totalValue),
			formatter.WeightStr(totalValue, totalValue),
			formatter.ColorNum(totalProfit),
			formatter.ColorPct(totalRate),
			"",
		})
	}
	table.Render()
}

// sortTotalViews 按 -s/--sort 与 --asc 排序。
// 数值字段默认降序；code/name 默认升序。--asc 强制升序。
func sortTotalViews(views []totalRowView) {
	key := normalizeSortKey(sortBy)
	desc := !sortAsc
	if !sortAsc && (key == "code" || key == "name") {
		desc = false // 代码/名称默认升序
	}

	sort.SliceStable(views, func(i, j int) bool {
		a, b := views[i], views[j]
		// 无行情的 day/dayrate 始终沉底
		if key == "day" && a.dayOK != b.dayOK {
			return a.dayOK
		}
		if key == "dayrate" && a.rateOK != b.rateOK {
			return a.rateOK
		}
		cmp := cmpTotalView(a, b, key)
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
}

func normalizeSortKey(raw string) string {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		return "value"
	}
	switch key {
	case "market", "sz", "市值", "总市值":
		return "value"
	case "today", "dayprofit", "今日", "今日盈亏", "dryk":
		return "day"
	case "profit", "holdprofit", "持仓", "持仓盈亏", "ccyk":
		return "hold"
	case "return", "holdrate", "收益率":
		return "rate"
	case "zdf", "pct", "今日涨幅", "涨幅":
		return "dayrate"
	case "qty", "数量", "总数量":
		return "count"
	case "代码":
		return "code"
	case "名称":
		return "name"
	case "value", "day", "hold", "rate", "dayrate", "code", "name", "count":
		return key
	default:
		fmt.Fprintf(os.Stderr, "未知排序字段 %q，支持: value day hold rate dayrate code name count\n", raw)
		os.Exit(1)
		return "value"
	}
}

// cmpTotalView: -1 if a<b, 1 if a>b, 0 if equal (on the sort key).
// 市值/持仓盈亏用实时重算后的 value/hold，与展示一致。
func cmpTotalView(a, b totalRowView, key string) int {
	var av, bv float64
	switch key {
	case "value":
		av, bv = a.value, b.value
	case "day":
		av, bv = a.day, b.day
	case "hold":
		av, bv = a.hold, b.hold
	case "rate":
		av, bv = a.rate, b.rate
	case "dayrate":
		av, bv = a.dayRate, b.dayRate
	case "count":
		av, bv = a.m.Count, b.m.Count
	case "code":
		return strings.Compare(a.m.Code, b.m.Code)
	case "name":
		return strings.Compare(a.m.Name, b.m.Name)
	}
	switch {
	case av < bv:
		return -1
	case av > bv:
		return 1
	default:
		return 0
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

// posCells 基金/无行情行：不展示今日盈亏列，用接口值（本周盈亏列）。
func posCells(r posRow, ctx *liveContext) []string {
	lv := recomputePos(r, ctx)
	return []string{
		r.Code,
		r.Name,
		fmt.Sprintf("%.0f", r.Count.F()),
		formatter.PriceStr(r.Cost.F()),
		formatter.PriceStr(lv.Price),
		fmt.Sprintf("%.2f", lv.Value),
		formatter.ColorNum(lv.HoldProfit),
		formatter.ColorNum(r.WProfit.F()),
		formatter.ColorPct(lv.HoldRate),
	}
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
		c.Flags().StringVarP(&sortBy, "sort", "s", "value", "排序: value day hold rate dayrate code name count")
		c.Flags().BoolVar(&sortAsc, "asc", false, "升序（默认降序）")
		c.Flags().StringVarP(&filterAccount, "account", "a", "", "按账户名过滤（模糊匹配）")
	}
}

// JSON 输出函数

func printByAccountJSON(rows []posRow, ctx *liveContext) {
			Code:       m.Code,
			Name:       m.Name,
			Count:      m.Count,
			Cost:       m.Cost,
			Price:      v.price,
			Value:      v.value,
			HoldProfit: v.hold,
			HoldRate:   v.rate,
			Accounts:   m.Accounts,
		}

		if showDay && v.dayOK {
			pos.DayProfit = &v.day
			pos.DayRate = &v.dayRate
			totalDay += v.day
			hasDay = true
		}

		result = append(result, pos)
		totalValue += v.value
		totalProfit += v.hold
	}

	summary := map[string]any{
		"value":       totalValue,
		"hold_profit": totalProfit,
		"count":       len(merged),
	}
	if hasDay {
		summary["day_profit"] = totalDay
	}

	formatter.PrintJSON(map[string]any{
		"positions": result,
		"summary":   summary,
	})
}

// printPositionListJSON 统一的 list JSON 输出
func printPositionListJSON(rows []posRow, ctx *liveContext, flat bool) {
	if flat {
		printFlatJSON(rows, ctx)
	} else {
		printByAccountJSON(rows, ctx)
	}
}

// printPositionTotalJSON 统一的 total/stock/fund JSON 输出
func printPositionTotalJSON(merged []mergedPos, ctx *liveContext) {
	printTotalJSON(merged, ctx)
}

