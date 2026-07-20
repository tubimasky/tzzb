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
	"github.com/liyanran/tzzb-cli/internal/service"
	"github.com/liyanran/tzzb-cli/pkg/api"
)

var (
	calculator = service.NewCalculator()

	filterAccount string
	filterType    string // all / stock / fund
	flatList      bool
	sortBy        string // value/day/hold/rate/dayrate/code/name/count
	sortAsc       bool
)

func getPositionService() *service.PositionService {
	return service.NewPositionService(apiClient, calculator)
}

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
		svc := getPositionService()
		accounts, err := svc.LoadAccounts()
		if err != nil {
			fmt.Fprintf(os.Stderr, "加载账户失败: %v\n", err)
			os.Exit(1)
		}
		rows := svc.CollectRows(accounts, filterAccount, filterType)
		if len(rows) == 0 {
			if outputJSON {
				formatter.PrintJSON([]any{})
			} else {
				fmt.Println("暂无持仓")
			}
			return
		}
		ctx := svc.LoadLiveContext(accounts, rows)
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
		svc := getPositionService()
		accounts, err := svc.LoadAccounts()
		if err != nil {
			fmt.Fprintf(os.Stderr, "加载账户失败: %v\n", err)
			os.Exit(1)
		}
		rows := svc.CollectRows(accounts, filterAccount, filterType)
		if len(rows) == 0 {
			if outputJSON {
				formatter.PrintJSON([]any{})
			} else {
				fmt.Println("暂无持仓")
			}
			return
		}
		ctx := svc.LoadLiveContext(accounts, rows)
		merged := svc.MergeByCode(rows)
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
		svc := getPositionService()
		accounts, err := svc.LoadAccounts()
		if err != nil {
			fmt.Fprintf(os.Stderr, "加载账户失败: %v\n", err)
			os.Exit(1)
		}
		rows := svc.CollectRows(accounts, filterAccount, "stock")
		if len(rows) == 0 {
			if outputJSON {
				formatter.PrintJSON([]any{})
			} else {
				fmt.Println("暂无股票/ETF 持仓")
			}
			return
		}
		ctx := svc.LoadLiveContext(accounts, rows)
		merged := svc.MergeByCode(rows)
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
		svc := getPositionService()
		accounts, err := svc.LoadAccounts()
		if err != nil {
			fmt.Fprintf(os.Stderr, "加载账户失败: %v\n", err)
			os.Exit(1)
		}
		rows := svc.CollectRows(accounts, filterAccount, "fund")
		if len(rows) == 0 {
			if outputJSON {
				formatter.PrintJSON([]any{})
			} else {
				fmt.Println("暂无基金持仓")
			}
			return
		}
		merged := svc.MergeByCode(rows)
		if outputJSON {
			printPositionTotalJSON(merged, nil)
			return
		}
		printTotal(merged, nil)
	},
}

// 类型别名：将 service 层的类型映射到 cmd 层，保持兼容性
type posRow = service.PositionRow
type mergedPos = service.MergedPosition
type liveContext = service.LiveContext


// 已废弃：使用 svc.LoadAccounts() 替代
func mustLoadAccounts() []api.Account {
	svc := getPositionService()
	accounts, err := svc.LoadAccounts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取持仓失败: %v\n", err)
		os.Exit(1)
	}
	return accounts
}

// 已废弃：使用 svc.CollectRows() 替代
func collectRows(accounts []api.Account, accountFilter, typeFilter string) []posRow {
	svc := getPositionService()
	return svc.CollectRows(accounts, accountFilter, typeFilter)
}

// 已废弃：使用 svc.LoadLiveContext() 替代
func loadLiveContext(accounts []api.Account, rows []posRow) *liveContext {
	svc := getPositionService()
	return svc.LoadLiveContext(accounts, rows)
}

// 已废弃：使用 svc.MergeByCode() 替代
func mergeByCode(rows []posRow) []mergedPos {
	svc := getPositionService()
	return svc.MergeByCode(rows)
}

func quoteOf(ctx *liveContext, code string) *api.Quote {
	svc := getPositionService()
	return svc.QuoteOf(ctx, code)
}

func recomputePos(r posRow, ctx *liveContext) api.LiveView {
	svc := getPositionService()
	return svc.RecomputePosition(r, ctx)
}

func recomputeMerged(m mergedPos, ctx *liveContext) api.LiveView {
	svc := getPositionService()
	return svc.RecomputeMerged(m, ctx)
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

	showDay := ctx != nil && len(ctx.Quotes) > 0
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
					r.Position.Code, r.Position.Name,
					fmt.Sprintf("%.0f", r.Position.Count.F()),
					formatter.PriceStr(r.Position.Cost.F()),
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

	showDay := ctx != nil && len(ctx.Quotes) > 0
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
				r.Position.Code, r.Position.Name,
				fmt.Sprintf("%.0f", r.Position.Count.F()),
				formatter.PriceStr(r.Position.Cost.F()),
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

// totalRowView 已废弃：使用 service.TotalRowView 替代
type totalRowView = service.TotalRowView

func printTotal(merged []mergedPos, ctx *liveContext) {
	views := make([]service.TotalRowView, 0, len(merged))
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
		views = append(views, service.TotalRowView{
			Code:       m.Code,
			Name:       m.Name,
			Count:      m.Count,
			Cost:       m.Cost,
			Price:      lv.Price,
			Value:      lv.Value,
			HoldProfit: lv.HoldProfit,
			HoldRate:   lv.HoldRate,
			DayProfit:  lv.DayProfit,
			DayRate:    lv.DayRate,
			Accounts:   m.Accounts,
		})
	}

	svc := getPositionService()
	svc.SortTotalViews(views, sortBy, sortAsc)

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

	showDay := ctx != nil && len(ctx.Quotes) > 0
	table := tablewriter.NewWriter(os.Stdout)
	if showDay {
		table.SetHeader([]string{"代码", "名称", "总数量", "均成本", "现价", "总市值", "占比", "持仓盈亏", "今日盈亏", "今日涨幅", "收益率", "账户"})
	} else {
		table.SetHeader([]string{"代码", "名称", "总数量", "均成本", "现价", "总市值", "占比", "持仓盈亏", "收益率", "账户"})
	}
	table.SetBorder(true)
	table.SetRowLine(false)

	for _, v := range views {
		weight := formatter.WeightStr(v.Value, totalValue)
		dayOK := v.DayProfit != 0 || hasDay
		rateOK := false
		if q := quoteOf(ctx, v.Code); q != nil && q.PrevClose > 0 && v.Price > 0 {
			rateOK = true
		}
		if showDay {
			table.Append([]string{
				v.Code,
				v.Name,
				fmt.Sprintf("%.0f", v.Count),
				formatter.PriceStr(v.Cost),
				formatter.PriceStr(v.Price),
				fmt.Sprintf("%.2f", v.Value),
				weight,
				formatter.ColorNum(v.HoldProfit),
				formatter.DayStr(v.DayProfit, dayOK),
				formatter.RateStr(v.DayRate, rateOK),
				formatter.ColorPct(v.HoldRate),
				strings.Join(v.Accounts, ","),
			})
		} else {
			table.Append([]string{
				v.Code,
				v.Name,
				fmt.Sprintf("%.0f", v.Count),
				formatter.PriceStr(v.Cost),
				formatter.PriceStr(v.Price),
				fmt.Sprintf("%.2f", v.Value),
				weight,
				formatter.ColorNum(v.HoldProfit),
				formatter.ColorPct(v.HoldRate),
				strings.Join(v.Accounts, ","),
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


// normalizeCode 已废弃：使用 service.normalizeCode 替代
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
		r.Position.Code,
		r.Position.Name,
		fmt.Sprintf("%.0f", r.Position.Count.F()),
		formatter.PriceStr(r.Position.Cost.F()),
		formatter.PriceStr(lv.Price),
		fmt.Sprintf("%.2f", lv.Value),
		formatter.ColorNum(lv.HoldProfit),
		formatter.ColorNum(r.Position.WProfit.F()),
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
	type PositionJSON struct {
		Code       string   `json:"code"`
		Name       string   `json:"name"`
		Count      float64  `json:"count"`
		Cost       float64  `json:"cost"`
		Price      float64  `json:"price"`
		Value      float64  `json:"value"`
		HoldProfit float64  `json:"hold_profit"`
		HoldRate   float64  `json:"hold_rate"`
		DayProfit  *float64 `json:"day_profit,omitempty"`
		DayRate    *float64 `json:"day_rate,omitempty"`
		WeekProfit float64  `json:"week_profit"`
	}
	type AccountGroup struct {
		Account   string         `json:"account"`
		Category  string         `json:"category"`
		Positions []PositionJSON `json:"positions"`
		Summary   struct {
			Value      float64  `json:"value"`
			HoldProfit float64  `json:"hold_profit"`
			DayProfit  *float64 `json:"day_profit,omitempty"`
			Count      int      `json:"count"`
		} `json:"summary"`
	}

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

	showDay := ctx != nil && len(ctx.Quotes) > 0
	var result []AccountGroup
	var grandValue, grandProfit, grandDay float64
	var hasDay bool

	for _, name := range order {
		list := groups[name]
		useLive := showDay && catOf[name] != "fund"

		group := AccountGroup{
			Account:  name,
			Category: api.CategoryLabel(catOf[name]),
		}

		var subValue, subProfit, subDay float64
		var subHasDay bool
		for _, r := range list {
			lv := recomputePos(r, ctx)
			if useLive {
				lv = recomputePos(r, ctx)
			} else {
				lv = recomputePos(r, nil)
			}

			pos := PositionJSON{
				Code:       r.Position.Code,
				Name:       r.Position.Name,
				Count:      r.Position.Count.F(),
				Cost:       r.Position.Cost.F(),
				Price:      lv.Price,
				Value:      lv.Value,
				HoldProfit: lv.HoldProfit,
				HoldRate:   lv.HoldRate,
				WeekProfit: r.Position.WProfit.F(),
			}
			if useLive && lv.DayOK {
				pos.DayProfit = &lv.DayProfit
				pos.DayRate = &lv.DayRate
			}
			group.Positions = append(group.Positions, pos)

			subValue += lv.Value
			subProfit += lv.HoldProfit
			if lv.DayOK && useLive {
				subDay += lv.DayProfit
				subHasDay = true
			}
		}

		group.Summary.Value = subValue
		group.Summary.HoldProfit = subProfit
		group.Summary.Count = len(list)
		if subHasDay {
			group.Summary.DayProfit = &subDay
		}

		result = append(result, group)
		grandValue += subValue
		grandProfit += subProfit
		if subHasDay {
			grandDay += subDay
			hasDay = true
		}
	}

	summary := map[string]any{
		"value":       grandValue,
		"hold_profit": grandProfit,
		"rows":        len(rows),
		"accounts":    len(order),
	}
	if hasDay {
		summary["day_profit"] = grandDay
	}

	formatter.PrintJSON(map[string]any{
		"accounts": result,
		"summary":  summary,
	})
}

func printFlatJSON(rows []posRow, ctx *liveContext) {
	type FlatPositionJSON struct {
		Account    string   `json:"account"`
		Category   string   `json:"category"`
		Code       string   `json:"code"`
		Name       string   `json:"name"`
		Count      float64  `json:"count"`
		Cost       float64  `json:"cost"`
		Price      float64  `json:"price"`
		Value      float64  `json:"value"`
		HoldProfit float64  `json:"hold_profit"`
		HoldRate   float64  `json:"hold_rate"`
		DayProfit  *float64 `json:"day_profit,omitempty"`
		DayRate    *float64 `json:"day_rate,omitempty"`
		WeekProfit float64  `json:"week_profit"`
	}

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

	showDay := ctx != nil && len(ctx.Quotes) > 0
	var result []FlatPositionJSON
	var totalValue, totalProfit, totalDay float64
	var hasDay bool

	for _, v := range views {
		r, lv := v.r, v.lv
		totalValue += lv.Value
		totalProfit += lv.HoldProfit

		pos := FlatPositionJSON{
			Account:    r.Account,
			Category:   api.CategoryLabel(r.Category),
			Code:       r.Position.Code,
			Name:       r.Position.Name,
			Count:      r.Position.Count.F(),
			Cost:       r.Position.Cost.F(),
			Price:      lv.Price,
			Value:      lv.Value,
			HoldProfit: lv.HoldProfit,
			HoldRate:   lv.HoldRate,
			WeekProfit: r.Position.WProfit.F(),
		}

		if showDay && r.Category != "fund" && lv.DayOK {
			totalDay += lv.DayProfit
			hasDay = true
			pos.DayProfit = &lv.DayProfit
			pos.DayRate = &lv.DayRate
		}
		result = append(result, pos)
	}

	summary := map[string]any{
		"value":       totalValue,
		"hold_profit": totalProfit,
		"rows":        len(rows),
	}
	if hasDay {
		summary["day_profit"] = totalDay
	}

	formatter.PrintJSON(map[string]any{
		"positions": result,
		"summary":   summary,
	})
}

func printTotalJSON(merged []mergedPos, ctx *liveContext) {
	type TotalPositionJSON struct {
		Code       string   `json:"code"`
		Name       string   `json:"name"`
		Count      float64  `json:"count"`
		Cost       float64  `json:"cost"`
		Price      float64  `json:"price"`
		Value      float64  `json:"value"`
		HoldProfit float64  `json:"hold_profit"`
		HoldRate   float64  `json:"hold_rate"`
		DayProfit  *float64 `json:"day_profit,omitempty"`
		DayRate    *float64 `json:"day_rate,omitempty"`
		Accounts   []string `json:"accounts"`
	}

	showDay := ctx != nil && len(ctx.Quotes) > 0
	var result []TotalPositionJSON
	var totalValue, totalProfit, totalDay float64
	var hasDay bool

	// 转换为 service.TotalRowView 并排序
	views := make([]service.TotalRowView, 0, len(merged))
	for _, m := range merged {
		lv := recomputeMerged(m, ctx)
		views = append(views, service.TotalRowView{
			Code:       m.Code,
			Name:       m.Name,
			Count:      m.Count,
			Cost:       m.Cost,
			Price:      lv.Price,
			Value:      lv.Value,
			HoldProfit: lv.HoldProfit,
			HoldRate:   lv.HoldRate,
			DayProfit:  lv.DayProfit,
			DayRate:    lv.DayRate,
			Accounts:   m.Accounts,
		})
	}
	svc := getPositionService()
	svc.SortTotalViews(views, sortBy, sortAsc)

	for _, v := range views {
		pos := TotalPositionJSON{
			Code:       v.Code,
			Name:       v.Name,
			Count:      v.Count,
			Cost:       v.Cost,
			Price:      v.Price,
			Value:      v.Value,
			HoldProfit: v.HoldProfit,
			HoldRate:   v.HoldRate,
			Accounts:   v.Accounts,
		}

		if showDay && v.DayProfit != 0 {
			pos.DayProfit = &v.DayProfit
			pos.DayRate = &v.DayRate
			totalDay += v.DayProfit
			hasDay = true
		}

		result = append(result, pos)
		totalValue += v.Value
		totalProfit += v.HoldProfit
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

