package service

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/liyanran/tzzb-cli/internal/cli"
	"github.com/liyanran/tzzb-cli/internal/formatter"
	"github.com/liyanran/tzzb-cli/pkg/api"
)

// PositionService 持仓业务逻辑层
type PositionService struct {
	client     *api.Client
	calculator *Calculator
}

// NewPositionService 创建持仓服务
func NewPositionService(client *api.Client, calculator *Calculator) *PositionService {
	return &PositionService{
		client:     client,
		calculator: calculator,
	}
}

// PositionRow 持仓行（带账户信息）
type PositionRow struct {
	Account  string
	Category string
	Position api.Position
}

// MergedPosition 合并后的持仓（跨账户按代码汇总）
type MergedPosition struct {
	Code       string
	Name       string
	Count      float64
	Cost       float64
	Price      float64
	Value      float64
	HoldProfit float64
	Accounts   []string // 来源账户列表
}

// LiveContext 实时行情上下文
type LiveContext struct {
	Quotes map[string]api.Quote            // code → quote
	Trades map[string][]api.DayTrade       // accountName → trades
	ByCode map[string][]api.DayTrade       // code → 跨账户合并成交
}

// LoadAccounts 加载所有账户
func (s *PositionService) LoadAccounts() ([]api.Account, error) {
	return s.client.GetAccountList()
}

// CollectRows 收集持仓行（带过滤）
func (s *PositionService) CollectRows(accounts []api.Account, accountFilter, typeFilter string) []PositionRow {
	var out []PositionRow
	for _, acc := range accounts {
		if accountFilter != "" && acc.Name() != accountFilter {
			continue
		}
		if !matchType(acc.Category, typeFilter) {
			continue
		}
		for _, p := range acc.Positions {
			if p.Count.F() <= 0 {
				continue
			}
			out = append(out, PositionRow{
				Account:  acc.Name(),
				Category: acc.Category,
				Position: p,
			})
		}
	}
	return out
}

// matchType: stock = 非基金账户；fund = 基金账户；all = 全部
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

// LoadLiveContext 加载实时行情和当日成交上下文
func (s *PositionService) LoadLiveContext(accounts []api.Account, rows []PositionRow) *LiveContext {
	ctx := &LiveContext{
		Quotes: map[string]api.Quote{},
		Trades: map[string][]api.DayTrade{},
		ByCode: map[string][]api.DayTrade{},
	}

	// 1. 收集需要的行情
	var refs []api.QuoteRef
	seenRef := map[string]bool{}
	for _, r := range rows {
		if r.Category == "fund" {
			continue
		}
		code := normalizeCode(r.Position.Code)
		if code == "" {
			continue
		}
		mkt := api.GuessMarket(code, r.Position.Market)
		key := mkt + ":" + code
		if seenRef[key] {
			continue
		}
		seenRef[key] = true
		refs = append(refs, api.QuoteRef{Code: code, Market: mkt})
	}

	// 2. 获取行情
	if len(refs) > 0 {
		q, err := s.client.GetQuotes(refs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取实时行情失败（今日盈亏将显示 -）: %v\n", err)
		} else {
			ctx.Quotes = q
		}
	}

	// 3. 获取当日成交（只拉非基金账户）
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
		ts, err := s.client.GetDayTrades(acc)
		if err != nil {
			// 单个账户失败不阻断
			continue
		}
		ctx.Trades[name] = ts
		for _, t := range ts {
			ctx.ByCode[t.Code] = append(ctx.ByCode[t.Code], t)
		}
	}

	return ctx
}

// QuoteOf 从上下文获取行情
func (s *PositionService) QuoteOf(ctx *LiveContext, code string) *api.Quote {
	if ctx == nil || ctx.Quotes == nil {
		return nil
	}
	q, ok := ctx.Quotes[normalizeCode(code)]
	if !ok {
		return nil
	}
	return &q
}

// RecomputePosition 重新计算单个持仓的实时数据
func (s *PositionService) RecomputePosition(r PositionRow, ctx *LiveContext) api.LiveView {
	var trades []api.DayTrade
	if ctx != nil && r.Category != "fund" {
		for _, t := range ctx.Trades[r.Account] {
			if t.Code == normalizeCode(r.Position.Code) {
				trades = append(trades, t)
			}
		}
	}
	q := s.QuoteOf(ctx, r.Position.Code)
	isFund := r.Category == "fund"
	return s.calculator.RecomputePosition(
		r.Position.Count.F(), r.Position.Cost.F(), r.Position.Price.F(), r.Position.Value.F(),
		r.Position.HoldProfit.F(), r.Position.HoldRate.F(), q, trades, isFund,
	)
}

// RecomputeMerged 重新计算合并持仓的实时数据
func (s *PositionService) RecomputeMerged(m MergedPosition, ctx *LiveContext) api.LiveView {
	var trades []api.DayTrade
	if ctx != nil {
		trades = ctx.ByCode[normalizeCode(m.Code)]
	}
	return s.calculator.RecomputeMerged(
		m.Count, m.Cost, m.Price, m.Value, m.HoldProfit,
		s.QuoteOf(ctx, m.Code), trades,
	)
}

// MergeByCode 跨账户按代码合并持仓
func (s *PositionService) MergeByCode(rows []PositionRow) []MergedPosition {
	type key struct{ code, name string }
	grouped := map[key]*MergedPosition{}
	var order []key

	for _, r := range rows {
		code := normalizeCode(r.Position.Code)
		if code == "" {
			code = r.Position.Code
		}
		name := r.Position.Name
		k := key{code, name}
		if grouped[k] == nil {
			order = append(order, k)
			grouped[k] = &MergedPosition{
				Code:     code,
				Name:     name,
				Accounts: []string{},
			}
		}
		m := grouped[k]
		m.Count += r.Position.Count.F()
		m.Cost += r.Position.Cost.F() * r.Position.Count.F()
		m.Value += r.Position.Value.F()
		m.HoldProfit += r.Position.HoldProfit.F()

		// 记录账户来源（去重）
		found := false
		for _, a := range m.Accounts {
			if a == r.Account {
				found = true
				break
			}
		}
		if !found {
			m.Accounts = append(m.Accounts, r.Account)
		}
	}

	var out []MergedPosition
	for _, k := range order {
		m := grouped[k]
		if m.Count > 0 {
			m.Cost /= m.Count
		}
		if m.Value > 0 {
			m.Price = m.Value / m.Count
		}
		out = append(out, *m)
	}
	return out
}

// TotalRowView 用于排序的汇总行视图
type TotalRowView struct {
	Code       string
	Name       string
	Count      float64
	Cost       float64
	Price      float64
	Value      float64
	HoldProfit float64
	HoldRate   float64
	DayProfit  float64
	DayRate    float64
	Accounts   []string
}

// SortTotalViews 按指定字段排序汇总视图
func (s *PositionService) SortTotalViews(views []TotalRowView, sortBy string, asc bool) {
	key := normalizeSortKey(sortBy)
	sort.Slice(views, func(i, j int) bool {
		cmp := cmpTotalView(views[i], views[j], key)
		if asc {
			return cmp < 0
		}
		return cmp > 0
	})
}

func normalizeSortKey(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case "value", "v", "市值":
		return "value"
	case "day", "d", "今日", "今日盈亏":
		return "day"
	case "hold", "h", "持仓", "持仓盈亏":
		return "hold"
	case "rate", "r", "收益率", "持仓收益率":
		return "rate"
	case "dayrate", "dr", "今日收益率":
		return "dayrate"
	case "code", "c", "代码":
		return "code"
	case "name", "n", "名称":
		return "name"
	case "count", "cnt", "持仓数":
		return "count"
	default:
		return "value"
	}
}

func cmpTotalView(a, b TotalRowView, key string) int {
	switch key {
	case "value":
		return cmpFloat(a.Value, b.Value)
	case "day":
		return cmpFloat(a.DayProfit, b.DayProfit)
	case "hold":
		return cmpFloat(a.HoldProfit, b.HoldProfit)
	case "rate":
		return cmpFloat(a.HoldRate, b.HoldRate)
	case "dayrate":
		return cmpFloat(a.DayRate, b.DayRate)
	case "code":
		if a.Code < b.Code {
			return -1
		}
		if a.Code > b.Code {
			return 1
		}
		return 0
	case "name":
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	case "count":
		return cmpFloat(a.Count, b.Count)
	default:
		return 0
	}
}

func cmpFloat(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// normalizeCode 代码归一化：去除市场前缀和 .OF 后缀
func normalizeCode(code string) string {
	// 去除市场前缀 (SH/SZ/BJ)
	code = strings.TrimPrefix(code, "SH")
	code = strings.TrimPrefix(code, "SZ")
	code = strings.TrimPrefix(code, "BJ")
	// 去除 .OF 后缀（场外基金）
	code = strings.TrimSuffix(code, ".OF")
	return code
}


// PositionCommandService 持仓命令业务逻辑（封装重复的加载+渲染流程）
type PositionCommandService struct {
	positionSvc *PositionService
	renderSvc   *RenderService
}

// NewPositionCommandService 创建持仓命令服务
func NewPositionCommandService(positionSvc *PositionService, renderSvc *RenderService) *PositionCommandService {
	return &PositionCommandService{
		positionSvc: positionSvc,
		renderSvc:   renderSvc,
	}
}

// ExecuteListCommand 执行 list 命令逻辑
func (s *PositionCommandService) ExecuteListCommand(accountFilter, typeFilter string, flatList, outputJSON bool) {
	accounts, err := s.positionSvc.LoadAccounts()
	if err != nil {
		cli.HandleError(err, "加载账户失败")
	}
	rows := s.positionSvc.CollectRows(accounts, accountFilter, typeFilter)
	if len(rows) == 0 {
		if outputJSON {
			formatter.PrintJSON([]any{})
		} else {
			fmt.Println("暂无持仓")
		}
		return
	}
	ctx := s.positionSvc.LoadLiveContext(accounts, rows)
	if outputJSON {
		if flatList {
			s.renderSvc.PrintJSON(s.renderSvc.BuildFlatJSON(rows, ctx))
		} else {
			s.renderSvc.PrintJSON(s.renderSvc.BuildByAccountJSON(rows, ctx))
		}
		return
	}
	if flatList {
		s.renderSvc.RenderFlat(rows, ctx)
		return
	}
	s.renderSvc.RenderByAccount(rows, ctx)
}

// ExecuteTotalCommand 执行 total/stock/fund 命令逻辑
func (s *PositionCommandService) ExecuteTotalCommand(accountFilter, typeFilter, sortBy string, sortAsc, outputJSON bool, withLiveContext bool) {
	accounts, err := s.positionSvc.LoadAccounts()
	if err != nil {
		cli.HandleError(err, "加载账户失败")
	}
	rows := s.positionSvc.CollectRows(accounts, accountFilter, typeFilter)
	if len(rows) == 0 {
		if outputJSON {
			formatter.PrintJSON([]any{})
		} else {
			msg := "暂无持仓"
			if typeFilter == "stock" {
				msg = "暂无股票/ETF 持仓"
			} else if typeFilter == "fund" {
				msg = "暂无基金持仓"
			}
			fmt.Println(msg)
		}
		return
	}

	var ctx *LiveContext
	if withLiveContext {
		ctx = s.positionSvc.LoadLiveContext(accounts, rows)
	}

	merged := s.positionSvc.MergeByCode(rows)
	if outputJSON {
		s.renderSvc.PrintJSON(s.renderSvc.BuildTotalJSON(merged, ctx, sortBy, sortAsc))
		return
	}
	s.renderSvc.RenderTotal(merged, ctx, sortBy, sortAsc)
}


// PositionJSON JSON 输出的持仓结构
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

// AccountGroupJSON 按账户分组的 JSON 结构
type AccountGroupJSON struct {
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

// BuildByAccountJSON 构建按账户分组的 JSON 数据
func (s *RenderService) BuildByAccountJSON(rows []PositionRow, ctx *LiveContext) map[string]any {
	groups := s.GroupByAccount(rows)
	showDay := ctx != nil && len(ctx.Quotes) > 0

	var result []AccountGroupJSON
	var grandValue, grandProfit, grandDay float64
	var hasDay bool

	for _, group := range groups {
		useLive := showDay && group.Category != "fund"

		jsonGroup := AccountGroupJSON{
			Account:  group.Account,
			Category: api.CategoryLabel(group.Category),
		}

		var subValue, subProfit, subDay float64
		var subHasDay bool

		for _, r := range group.Rows {
			var lv api.LiveView
			if useLive {
				lv = s.positionService.RecomputePosition(r, ctx)
			} else {
				lv = s.positionService.RecomputePosition(r, nil)
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
			jsonGroup.Positions = append(jsonGroup.Positions, pos)

			subValue += lv.Value
			subProfit += lv.HoldProfit
			if lv.DayOK && useLive {
				subDay += lv.DayProfit
				subHasDay = true
			}
		}

		jsonGroup.Summary.Value = subValue
		jsonGroup.Summary.HoldProfit = subProfit
		jsonGroup.Summary.Count = len(group.Rows)
		if subHasDay {
			jsonGroup.Summary.DayProfit = &subDay
		}

		result = append(result, jsonGroup)
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
		"accounts":    len(groups),
	}
	if hasDay {
		summary["day_profit"] = grandDay
	}

	return map[string]any{
		"accounts": result,
		"summary":  summary,
	}
}

// FlatPositionJSON 扁平视图 JSON 结构
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

// BuildFlatJSON 构建扁平表格 JSON 数据
func (s *RenderService) BuildFlatJSON(rows []PositionRow, ctx *LiveContext) map[string]any {
	views := make([]FlatView, 0, len(rows))
	for _, r := range rows {
		c := ctx
		if r.Category == "fund" {
			c = nil
		}
		views = append(views, FlatView{
			Row:      r,
			LiveView: s.positionService.RecomputePosition(r, c),
		})
	}

	showDay := ctx != nil && len(ctx.Quotes) > 0
	var result []FlatPositionJSON
	var totalValue, totalProfit, totalDay float64
	var hasDay bool

	for _, v := range views {
		r, lv := v.Row, v.LiveView
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

	return map[string]any{
		"positions": result,
		"summary":   summary,
	}
}

// TotalPositionJSON 汇总持仓 JSON 结构
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

// BuildTotalJSON 构建汇总 JSON 数据
func (s *RenderService) BuildTotalJSON(merged []MergedPosition, ctx *LiveContext, sortBy string, sortAsc bool) map[string]any {
	showDay := ctx != nil && len(ctx.Quotes) > 0
	var result []TotalPositionJSON
	var totalValue, totalProfit, totalDay float64
	var hasDay bool

	// 转换为 TotalRowView 并排序
	views := make([]TotalRowView, 0, len(merged))
	for _, m := range merged {
		lv := s.positionService.RecomputeMerged(m, ctx)
		views = append(views, TotalRowView{
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
	s.positionService.SortTotalViews(views, sortBy, sortAsc)

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

	return map[string]any{
		"positions": result,
		"summary":   summary,
	}
}

// PrintJSON 输出 JSON（统一入口）
func (s *RenderService) PrintJSON(data map[string]any) {
	formatter.PrintJSON(data)
}


// RenderService 持仓渲染服务
type RenderService struct {
	positionService *PositionService
}

// NewRenderService 创建渲染服务
func NewRenderService(positionService *PositionService) *RenderService {
	return &RenderService{positionService: positionService}
}

// AccountGroup 按账户分组的持仓数据
type AccountGroup struct {
	Account  string
	Category string
	Rows     []PositionRow
}

// GroupByAccount 按账户分组持仓行
func (s *RenderService) GroupByAccount(rows []PositionRow) []AccountGroup {
	var order []string
	groups := map[string][]PositionRow{}
	catOf := map[string]string{}

	for _, r := range rows {
		if _, ok := groups[r.Account]; !ok {
			order = append(order, r.Account)
			catOf[r.Account] = r.Category
		}
		groups[r.Account] = append(groups[r.Account], r)
	}

	var result []AccountGroup
	for _, name := range order {
		result = append(result, AccountGroup{
			Account:  name,
			Category: catOf[name],
			Rows:     groups[name],
		})
	}
	return result
}

// PositionSummary 持仓汇总数据
type PositionSummary struct {
	Value      float64
	HoldProfit float64
	DayProfit  float64
	HasDay     bool
	Count      int
}

// ComputeAccountSummary 计算账户汇总
func (s *RenderService) ComputeAccountSummary(rows []PositionRow, ctx *LiveContext, isFund bool) PositionSummary {
	var summary PositionSummary
	summary.Count = len(rows)

	for _, r := range rows {
		var lv api.LiveView
		if isFund {
			lv = s.positionService.RecomputePosition(r, nil)
		} else {
			lv = s.positionService.RecomputePosition(r, ctx)
		}
		summary.Value += lv.Value
		summary.HoldProfit += lv.HoldProfit
		if lv.DayOK {
			summary.DayProfit += lv.DayProfit
			summary.HasDay = true
		}
	}
	return summary
}

// RenderByAccount 按账户渲染持仓
func (s *RenderService) RenderByAccount(rows []PositionRow, ctx *LiveContext) {
	groups := s.GroupByAccount(rows)
	showDay := ctx != nil && len(ctx.Quotes) > 0

	var grandValue, grandProfit, grandDay float64
	var hasDay bool

	for _, group := range groups {
		fmt.Printf("\n【%s · %s】\n", group.Account, api.CategoryLabel(group.Category))
		useLive := showDay && group.Category != "fund"

		var headers []string
		if useLive {
			headers = []string{"代码", "名称", "持仓量", "成本价", "现价", "市值", "持仓盈亏", "今日盈亏", "今日涨幅", "收益率"}
		} else {
			headers = []string{"代码", "名称", "持仓量", "成本价", "现价", "市值", "持仓盈亏", "本周盈亏", "收益率"}
		}

		summary := s.ComputeAccountSummary(group.Rows, ctx, group.Category == "fund")

		var rows [][]string
		for _, r := range group.Rows {
			if useLive {
				lv := s.positionService.RecomputePosition(r, ctx)
				rows = append(rows, []string{
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
				rows = append(rows, s.renderFundCells(r))
			}
		}

		formatter.RenderTable(headers, rows, formatter.WithBorder(true))

		grandValue += summary.Value
		grandProfit += summary.HoldProfit
		if summary.HasDay {
			grandDay += summary.DayProfit
			hasDay = true
		}

		if summary.HasDay {
			fmt.Printf("小计  市值: %.2f | 持仓盈亏: %s | 今日盈亏: %s | 标的: %d\n",
				summary.Value, formatter.ColorNum(summary.HoldProfit), formatter.ColorNum(summary.DayProfit), summary.Count)
		} else {
			fmt.Printf("小计  市值: %.2f | 持仓盈亏: %s | 标的: %d\n",
				summary.Value, formatter.ColorNum(summary.HoldProfit), summary.Count)
		}
	}

	fmt.Println()
	if hasDay {
		color.New(color.Bold).Printf("合计  市值: %.2f | 持仓盈亏: %s | 今日盈亏: %s | 行数: %d | 账户: %d\n",
			grandValue, formatter.ColorNum(grandProfit), formatter.ColorNum(grandDay), len(rows), len(groups))
	} else {
		color.New(color.Bold).Printf("合计  市值: %.2f | 持仓盈亏: %s | 行数: %d | 账户: %d\n",
			grandValue, formatter.ColorNum(grandProfit), len(rows), len(groups))
	}
}

// renderFundCells 基金/无行情行：不展示今日盈亏列，用接口值（本周盈亏列）
func (s *RenderService) renderFundCells(r PositionRow) []string {
	lv := s.positionService.RecomputePosition(r, nil)
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

// FlatView 扁平视图（单行 + LiveView）
type FlatView struct {
	Row      PositionRow
	LiveView api.LiveView
}

// RenderFlat 扁平表格渲染
func (s *RenderService) RenderFlat(rows []PositionRow, ctx *LiveContext) {
	views := make([]FlatView, 0, len(rows))
	for _, r := range rows {
		c := ctx
		if r.Category == "fund" {
			c = nil
		}
		views = append(views, FlatView{
			Row:      r,
			LiveView: s.positionService.RecomputePosition(r, c),
		})
	}
	sort.Slice(views, func(i, j int) bool {
		return views[i].LiveView.Value > views[j].LiveView.Value
	})

	showDay := ctx != nil && len(ctx.Quotes) > 0
	var headers []string
	if showDay {
		headers = []string{"账户", "类型", "代码", "名称", "持仓量", "成本价", "现价", "市值", "持仓盈亏", "今日盈亏", "今日涨幅", "收益率"}
	} else {
		headers = []string{"账户", "类型", "代码", "名称", "持仓量", "成本价", "现价", "市值", "持仓盈亏", "本周盈亏", "收益率"}
	}

	var tableRows [][]string
	var totalValue, totalProfit, totalDay float64
	var hasDay bool
	for _, v := range views {
		r, lv := v.Row, v.LiveView
		totalValue += lv.Value
		totalProfit += lv.HoldProfit
		if showDay && r.Category != "fund" {
			if lv.DayOK {
				totalDay += lv.DayProfit
				hasDay = true
			}
			tableRows = append(tableRows, []string{
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
			cells = append(cells, s.renderFundCells(r)...)
			tableRows = append(tableRows, cells)
		}
	}

	formatter.RenderTable(headers, tableRows, formatter.WithBorder(true))

	fmt.Println()
	if hasDay {
		color.New(color.Bold).Printf("合计  市值: %.2f | 持仓盈亏: %s | 今日盈亏: %s | 行数: %d\n",
			totalValue, formatter.ColorNum(totalProfit), formatter.ColorNum(totalDay), len(rows))
	} else {
		color.New(color.Bold).Printf("合计  市值: %.2f | 持仓盈亏: %s | 行数: %d\n",
			totalValue, formatter.ColorNum(totalProfit), len(rows))
	}
}


// RenderTotal 汇总表格渲染
func (s *RenderService) RenderTotal(merged []MergedPosition, ctx *LiveContext, sortBy string, sortAsc bool) {
	views := make([]TotalRowView, 0, len(merged))
	var totalValue, totalProfit, totalDay, totalCount, totalCostBasis, totalSell float64
	var hasDay bool

	for _, m := range merged {
		lv := s.positionService.RecomputeMerged(m, ctx)
		totalValue += lv.Value
		totalProfit += lv.HoldProfit
		totalCount += m.Count
		totalCostBasis += m.Cost * m.Count
		totalSell += lv.SellAmount
		if lv.DayOK {
			totalDay += lv.DayProfit
			hasDay = true
		}
		views = append(views, TotalRowView{
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

	s.positionService.SortTotalViews(views, sortBy, sortAsc)

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
	var headers []string
	if showDay {
		headers = []string{"代码", "名称", "总数量", "均成本", "现价", "总市值", "占比", "持仓盈亏", "今日盈亏", "今日涨幅", "收益率", "账户"}
	} else {
		headers = []string{"代码", "名称", "总数量", "均成本", "现价", "总市值", "占比", "持仓盈亏", "收益率", "账户"}
	}

	var tableRows [][]string
	for _, v := range views {
		weight := formatter.WeightStr(v.Value, totalValue)
		dayOK := v.DayProfit != 0 || hasDay
		rateOK := false
		if q := s.positionService.QuoteOf(ctx, v.Code); q != nil && q.PrevClose > 0 && v.Price > 0 {
			rateOK = true
		}
		if showDay {
			tableRows = append(tableRows, []string{
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
			tableRows = append(tableRows, []string{
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
		tableRows = append(tableRows, []string{
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
		tableRows = append(tableRows, []string{
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

	formatter.RenderTable(headers, tableRows, formatter.WithBorder(true), formatter.WithRowLine(false))
}

