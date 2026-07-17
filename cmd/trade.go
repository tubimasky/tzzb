package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

var tradeAccount string

var tradeCmd = &cobra.Command{
	Use:   "trade",
	Short: "交易记录",
	Long:  `查看账户的资金/交易流水`,
}

var tradeListCmd = &cobra.Command{
	Use:   "list",
	Short: "查看交易记录",
	Run: func(cmd *cobra.Command, args []string) {
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

// renderRecords 动态列出流水记录的字段。后端字段不固定，取并集按字母序展示。
func renderRecords(records []map[string]any) {
	keySet := map[string]struct{}{}
	for _, r := range records {
		for k := range r {
			keySet[k] = struct{}{}
		}
	}
	var keys []string
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 优先展示常见字段，其余附后。
	preferred := []string{"date", "time", "type", "code", "name", "count", "price", "amount", "fee"}
	var ordered []string
	seen := map[string]bool{}
	for _, p := range preferred {
		if _, ok := keySet[p]; ok {
			ordered = append(ordered, p)
			seen[p] = true
		}
	}
	for _, k := range keys {
		if !seen[k] {
			ordered = append(ordered, k)
		}
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(ordered)
	table.SetBorder(true)
	table.SetAutoWrapText(false)
	for _, r := range records {
		row := make([]string, len(ordered))
		for i, k := range ordered {
			row[i] = fmt.Sprintf("%v", r[k])
		}
		table.Append(row)
	}
	table.Render()
}

func init() {
	tradeCmd.AddCommand(tradeListCmd)
	tradeListCmd.Flags().StringVarP(&tradeAccount, "account", "a", "", "按账户名过滤（模糊匹配），默认全部")
}
