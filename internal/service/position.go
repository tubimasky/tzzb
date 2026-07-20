package service

import (
	"fmt"
	"os"
	"sort"
	"strings"

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
