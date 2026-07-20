package cmd

import (
	"fmt"
	"os"
	"strings"

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

func getRenderService() *service.RenderService {
	return service.NewRenderService(getPositionService())
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
		renderSvc := getRenderService()
		if outputJSON {
			if flatList {
				renderSvc.PrintJSON(renderSvc.BuildFlatJSON(rows, ctx))
			} else {
				renderSvc.PrintJSON(renderSvc.BuildByAccountJSON(rows, ctx))
			}
			return
		}
		if flatList {
			renderSvc.RenderFlat(rows, ctx)
			return
		}
		renderSvc.RenderByAccount(rows, ctx)
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
		renderSvc := getRenderService()
		if outputJSON {
			renderSvc.PrintJSON(renderSvc.BuildTotalJSON(merged, ctx, sortBy, sortAsc))
			return
		}
		renderSvc.RenderTotal(merged, ctx, sortBy, sortAsc)
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
		renderSvc := getRenderService()
		if outputJSON {
			renderSvc.PrintJSON(renderSvc.BuildTotalJSON(merged, ctx, sortBy, sortAsc))
			return
		}
		renderSvc.RenderTotal(merged, ctx, sortBy, sortAsc)
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
		renderSvc := getRenderService()
		if outputJSON {
			renderSvc.PrintJSON(renderSvc.BuildTotalJSON(merged, nil, sortBy, sortAsc))
			return
		}
		renderSvc.RenderTotal(merged, nil, sortBy, sortAsc)
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

// 已废弃：使用 renderSvc.RenderByAccount() 替代
func printByAccount(rows []posRow, ctx *liveContext) {
	renderSvc := getRenderService()
	renderSvc.RenderByAccount(rows, ctx)
}

// 已废弃：使用 renderSvc.RenderFlat() 替代
func printFlat(rows []posRow, ctx *liveContext) {
	renderSvc := getRenderService()
	renderSvc.RenderFlat(rows, ctx)
}

// 已废弃：使用 renderSvc.RenderTotal() 替代
func printTotal(merged []mergedPos, ctx *liveContext) {
	renderSvc := getRenderService()
	renderSvc.RenderTotal(merged, ctx, sortBy, sortAsc)
}

func recomputePos(r posRow, ctx *liveContext) api.LiveView {
	svc := getPositionService()
	return svc.RecomputePosition(r, ctx)
}

func recomputeMerged(m mergedPos, ctx *liveContext) api.LiveView {
	svc := getPositionService()
	return svc.RecomputeMerged(m, ctx)
}

// totalRowView 已废弃：使用 service.TotalRowView 替代
type totalRowView = service.TotalRowView


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


