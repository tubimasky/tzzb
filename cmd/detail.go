package cmd

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/internal/formatter"
	"github.com/liyanran/tzzb-cli/internal/service"
	"github.com/liyanran/tzzb-cli/internal/tui"
	"github.com/liyanran/tzzb-cli/pkg/api"
)

var (
	detailAccount string
	detailFrom    string
	detailTo      string
	detailTUI     bool
)

func getDetailService() *service.DetailService {
	return service.NewDetailService(apiClient)
}

var detailCmd = &cobra.Command{
	Use:   "detail [股票代码]",
	Short: "单只股票钻取明细",
	Long: `钻取单只股票的全维度明细：合并持仓、建仓记录、历史成交、清仓汇总。
自动定位持有该代码的账户，多个账户命中则分块展示。

日期区间（历史成交/清仓汇总）默认从建仓日到今天，可用 --from/--to 覆盖（YYYYMMDD）。

加 -i/--tui 进入交互式界面（Tab 切换四块、多账户切换、成交表可滚动）。
不带代码 + -i：进入持仓选择器（tab 切股票/基金/全部），选中股票后下钻明细。

示例:
  tzzb detail 159227
  tzzb detail -i                 # 持仓选择器 → 下钻
  tzzb detail 159227 -i          # 直接进单股 TUI
  tzzb detail 600519 -a 国泰
  tzzb detail 300033 --from 20250101 --to 20251231`,
	Args: cobra.MaximumNArgs(1), // 允许 0 或 1 个参数
	RunE: func(cmd *cobra.Command, args []string) error {
		// 无代码 + -i → 进持仓选择器 TUI
		if len(args) == 0 && detailTUI {
			if !isatty.IsTerminal(os.Stdout.Fd()) {
				fmt.Fprintln(os.Stderr, "提示：非交互式终端，无法启动 TUI")
				return fmt.Errorf("TUI 需要交互式终端")
			}
			// 启动持仓选择器，返回选中的代码后下钻
			return tui.RunPositionPicker(getPositionService(), getDetailService(), detailAccount, detailFrom, detailTo)
		}

		// 无代码 + 无 -i → 显示 help
		if len(args) == 0 {
			return cmd.Help()
		}

		code := args[0]

		// --tui 与 --json 互斥。
		if detailTUI && outputJSON {
			return fmt.Errorf("--tui 与 --json 不能同时使用")
		}

		svc := getDetailService()
		details, err := svc.LoadDetail(code, detailAccount, detailFrom, detailTo)
		if err != nil {
			return fmt.Errorf("获取股票明细失败: %w", err)
		}

		if outputJSON {
			renderDetailJSON(details)
			return nil
		}

		// TUI：仅在 stdout 为真实终端时进入，否则降级直出（避免管道/重定向卡死）。
		if detailTUI {
			if isatty.IsTerminal(os.Stdout.Fd()) {
				if err := tui.RunDetail(details); err != nil {
					return fmt.Errorf("TUI 运行失败: %w", err)
				}
				return nil
			}
			fmt.Fprintln(os.Stderr, "提示：非交互式终端，已降级为直接输出")
		}
		renderDetailText(details)
		return nil
	},
}

func renderDetailJSON(details []service.StockDetail) {
	var arr []map[string]any
	for _, d := range details {
		arr = append(arr, map[string]any{
			"account":  d.Coord.AccountName,
			"merge":    d.Merge,
			"records":  d.Records,
			"trades":   d.Trades,
			"cleared":  d.Clear,
		})
	}
	formatter.PrintJSON(map[string]any{"details": arr})
}

func renderDetailText(details []service.StockDetail) {
	for i, d := range details {
		if i > 0 {
			fmt.Println()
		}
		renderOneDetail(d)
	}
}

func renderOneDetail(d service.StockDetail) {
	m := d.Merge
	title := d.Coord.StockCode
	if m != nil && m.StockName != "" {
		title = fmt.Sprintf("%s %s", m.StockName, m.StockCode)
	}
	color.New(color.Bold).Printf("== %s @ %s ==\n", title, d.Coord.AccountName)

	// 合并明细
	if m != nil {
		headers := []string{"持有天数", "数量", "买入均价", "现价", "市值", "总成本", "浮动盈亏", "收益率", "费用"}
		rows := [][]string{{
			fmt.Sprintf("%d", m.HoldDays),
			fmt.Sprintf("%.0f", m.Count),
			formatter.PriceStr(m.PriceBuyAv),
			formatter.PriceStr(m.PriceCurr),
			fmt.Sprintf("%.2f", m.MarketValue),
			fmt.Sprintf("%.2f", m.Zcb),
			formatter.ColorNum(m.Yke),
			formatter.ColorPctRaw(m.ProfitRate),
			fmt.Sprintf("%.2f", m.Fee),
		}}
		formatter.RenderTable(headers, rows, formatter.WithBorder(true))
		if m.ClearFlag == "1" {
			color.New(color.FgYellow).Println("（该持仓已清仓）")
		}
	}

	// 建仓记录
	if len(d.Records) > 0 {
		fmt.Println()
		color.New(color.Bold).Println("建仓记录:")
		headers := []string{"建仓日", "结束日", "分段收益", "收益率"}
		var rows [][]string
		for _, r := range d.Records {
			end := r.EndDate
			if end == "" {
				end = "持有中"
			}
			rows = append(rows, []string{
				fmtDate(r.StartDate),
				end,
				formatter.ColorNum(r.Profit),
				formatter.ColorPctRaw(r.ProfitRate),
			})
		}
		formatter.RenderTable(headers, rows, formatter.WithBorder(true))
	}

	// 历史成交
	if len(d.Trades) > 0 {
		fmt.Println()
		color.New(color.Bold).Println("历史成交:")
		headers := []string{"成交日", "方向", "价格", "数量", "金额", "费用"}
		var rows [][]string
		for _, t := range d.Trades {
			rows = append(rows, []string{
				fmtDate(t.TransDate),
				opLabel(t.Op),
				formatter.PriceStr(t.Price),
				fmt.Sprintf("%.0f", t.Count),
				fmt.Sprintf("%.2f", t.Amount),
				fmt.Sprintf("%.2f", t.Fee),
			})
		}
		formatter.RenderTable(headers, rows, formatter.WithBorder(true))
	}

	// 清仓汇总（未清仓跳过）
	if d.Clear != nil && d.Clear.HasData() {
		fmt.Println()
		color.New(color.Bold).Println("清仓汇总:")
		c := d.Clear
		color.New(color.Bold).Printf(
			"持有天数: %d | 总买入: %.2f | 总卖出: %.2f | 费用: %.2f | 清仓盈亏: %s (%s)\n",
			c.HoldDays, c.TotalBuy, c.TotalSell, c.Fee,
			formatter.ColorNum(c.TotalProfit), formatter.ColorPctRaw(c.TotalProfitRate),
		)
	}
}

// opLabel 把成交方向码转成中文（1 买入 / 2 卖出）。
func opLabel(op string) string {
	switch op {
	case api.OpBuy:
		return "买入"
	case api.OpSell:
		return "卖出"
	default:
		return op
	}
}

// fmtDate 把 YYYYMMDD 转成 YYYY-MM-DD，非 8 位原样返回。
func fmtDate(s string) string {
	if len(s) != 8 {
		return s
	}
	return fmt.Sprintf("%s-%s-%s", s[:4], s[4:6], s[6:8])
}

func init() {
	detailCmd.Flags().StringVarP(&detailAccount, "account", "a", "", "按账户名过滤（模糊匹配），默认全部持有该代码的账户")
	detailCmd.Flags().StringVar(&detailFrom, "from", "", "历史成交/清仓查询起始日 YYYYMMDD，默认建仓日")
	detailCmd.Flags().StringVar(&detailTo, "to", "", "历史成交/清仓查询结束日 YYYYMMDD，默认今天")
	detailCmd.Flags().BoolVarP(&detailTUI, "tui", "i", false, "进入交互式界面浏览（与 --json 互斥）")
}
