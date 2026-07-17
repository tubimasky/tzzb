package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

var (
	outputFile   string
	exportFormat string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "导出数据",
	Long:  `导出账户汇总或持仓明细（支持 csv / json）`,
}

var exportAccountCmd = &cobra.Command{
	Use:   "account",
	Short: "导出账户汇总",
	Run: func(cmd *cobra.Command, args []string) {
		accounts, err := apiClient.GetAccountList()
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取账户失败: %v\n", err)
			os.Exit(1)
		}
		if len(accounts) == 0 {
			fmt.Println("暂无账户")
			return
		}

		out := defaultOut("accounts")
		switch exportFormat {
		case "json":
			if err := writeJSON(accounts, out); err != nil {
				fmt.Fprintf(os.Stderr, "导出失败: %v\n", err)
				os.Exit(1)
			}
		case "csv":
			rows := [][]string{{"账户", "类型", "本金", "总资产", "持仓盈亏", "当日盈亏", "总盈亏", "仓位"}}
			for _, a := range accounts {
				rows = append(rows, []string{
					a.Name(), api.CategoryLabel(a.Category),
					f2(a.Principal.F()), f2(a.TotalAssets.F()),
					f2(a.HoldProfit.F()), f2(a.TodayProfit.F()), f2(a.TotalProfit.F()),
					fmt.Sprintf("%.1f%%", a.PositionRate.F()*100),
				})
			}
			if err := writeCSV(rows, out); err != nil {
				fmt.Fprintf(os.Stderr, "导出失败: %v\n", err)
				os.Exit(1)
			}
		default:
			fmt.Fprintf(os.Stderr, "不支持的格式: %s（支持 csv/json）\n", exportFormat)
			os.Exit(1)
		}
		fmt.Printf("✓ 已导出 %d 个账户到: %s\n", len(accounts), out)
	},
}

var exportPositionCmd = &cobra.Command{
	Use:   "position",
	Short: "导出持仓明细",
	Run: func(cmd *cobra.Command, args []string) {
		accounts, err := apiClient.GetAccountList()
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取持仓失败: %v\n", err)
			os.Exit(1)
		}

		// 扁平化成带账户信息的持仓行
		type row struct {
			Account  string      `json:"account"`
			Category string      `json:"category"`
			api.Position
		}
		var flat []row
		for _, a := range accounts {
			for _, p := range a.Positions {
				flat = append(flat, row{Account: a.Name(), Category: api.CategoryLabel(a.Category), Position: p})
			}
		}
		if len(flat) == 0 {
			fmt.Println("暂无持仓")
			return
		}

		out := defaultOut("positions")
		switch exportFormat {
		case "json":
			if err := writeJSON(flat, out); err != nil {
				fmt.Fprintf(os.Stderr, "导出失败: %v\n", err)
				os.Exit(1)
			}
		case "csv":
			rows := [][]string{{"账户", "类型", "代码", "名称", "持仓量", "成本价", "现价", "市值", "持仓盈亏", "本周盈亏", "收益率"}}
			for _, r := range flat {
				rows = append(rows, []string{
					r.Account, r.Category, r.Code, r.Name,
					fmt.Sprintf("%.0f", r.Count.F()),
					fmt.Sprintf("%.4f", r.Cost.F()),
					fmt.Sprintf("%.4f", r.Price.F()),
					f2(r.Value.F()), f2(r.HoldProfit.F()), f2(r.WProfit.F()),
					fmt.Sprintf("%.2f%%", r.HoldRate.F()*100),
				})
			}
			if err := writeCSV(rows, out); err != nil {
				fmt.Fprintf(os.Stderr, "导出失败: %v\n", err)
				os.Exit(1)
			}
		default:
			fmt.Fprintf(os.Stderr, "不支持的格式: %s（支持 csv/json）\n", exportFormat)
			os.Exit(1)
		}
		fmt.Printf("✓ 已导出 %d 条持仓到: %s\n", len(flat), out)
	},
}

func f2(v float64) string { return fmt.Sprintf("%.2f", v) }

func defaultOut(prefix string) string {
	if outputFile != "" {
		return outputFile
	}
	return fmt.Sprintf("%s_%s.%s", prefix, time.Now().Format("20060102"), exportFormat)
}

func writeJSON(v any, filename string) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

func writeCSV(rows [][]string, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// 写 UTF-8 BOM，Excel 打开中文才不乱码
	if _, err := file.WriteString("\xEF\xBB\xBF"); err != nil {
		return err
	}
	w := csv.NewWriter(file)
	defer w.Flush()
	for _, r := range rows {
		if err := w.Write(r); err != nil {
			return err
		}
	}
	return nil
}

func init() {
	exportCmd.AddCommand(exportAccountCmd)
	exportCmd.AddCommand(exportPositionCmd)

	for _, c := range []*cobra.Command{exportAccountCmd, exportPositionCmd} {
		c.Flags().StringVarP(&outputFile, "output", "o", "", "输出文件名")
		c.Flags().StringVarP(&exportFormat, "format", "f", "csv", "导出格式 (csv/json)")
	}
}
