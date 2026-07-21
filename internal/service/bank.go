package service

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/liyanran/tzzb-cli/internal/formatter"
	"github.com/liyanran/tzzb-cli/pkg/api"
	"github.com/olekukonko/tablewriter"
)

// BankService 银证转账服务
type BankService struct {
	client *api.Client
}

// NewBankService 创建银证转账服务
func NewBankService(client *api.Client) *BankService {
	return &BankService{client: client}
}

// ExecuteListCommand 执行查询银证转账列表命令
func (s *BankService) ExecuteListCommand(accountFilter string, days int, jsonOutput bool) {
	accounts, err := s.client.GetAccountList()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取账户失败: %v\n", err)
		os.Exit(1)
	}
	if len(accounts) == 0 {
		fmt.Println("未找到账户，请先登录")
		return
	}

	// 过滤账户
	var filteredAccounts []api.Account
	if accountFilter != "" {
		for _, acc := range accounts {
			if strings.Contains(strings.ToLower(acc.Name()), strings.ToLower(accountFilter)) {
				filteredAccounts = append(filteredAccounts, acc)
			}
		}
		if len(filteredAccounts) == 0 {
			fmt.Printf("未找到匹配的账户: %s\n", accountFilter)
			return
		}
	} else {
		filteredAccounts = accounts
	}

	// 计算日期范围
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days)

	// 获取所有账户的银证转账记录
	var allTransfers []api.BankTransfer
	for _, acc := range filteredAccounts {
		transfers, err := s.client.GetBankHistory(acc, startDate, endDate)
		if err != nil {
			fmt.Printf("获取 %s 银证转账记录失败: %v\n", acc.Name(), err)
			continue
		}
		allTransfers = append(allTransfers, transfers...)
	}

	if len(allTransfers) == 0 {
		fmt.Println("未找到银证转账记录")
		return
	}

	if jsonOutput {
		s.outputJSON(allTransfers, days)
	} else {
		s.renderTable(allTransfers, days)
	}
}

// renderTable 渲染银证转账表格
func (s *BankService) renderTable(transfers []api.BankTransfer, days int) {
	fmt.Printf("近 %d 天银证转账记录（共 %d 条）\n\n", days, len(transfers))

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"日期", "账户", "方向", "金额"})
	table.SetBorder(true)
	table.SetAutoWrapText(false)

	var totalIn, totalOut float64
	for _, t := range transfers {
		direction := t.Direction
		amount := t.Amount

		// 统计
		if t.OpCode == 86 || t.Direction == "银行转证券" || t.Direction == "收入" {
			totalIn += amount
		} else if t.OpCode == 87 || t.Direction == "证券转银行" || t.Direction == "支出" {
			totalOut += amount
		}

		table.Append([]string{
			t.Date,
			t.Account,
			direction,
			formatter.ColorNum(amount),
		})
	}

	table.Render()

	// 汇总统计
	fmt.Printf("\n转入合计: %s\n", formatter.ColorNum(totalIn))
	fmt.Printf("转出合计: %s\n", formatter.ColorNum(totalOut))
	fmt.Printf("净转入: %s\n", formatter.ColorNum(totalIn-totalOut))
}

// outputJSON 输出 JSON 格式
func (s *BankService) outputJSON(transfers []api.BankTransfer, days int) {
	var totalIn, totalOut float64
	var items []map[string]any

	for _, t := range transfers {
		// 统计
		if t.OpCode == 86 || t.Direction == "银行转证券" || t.Direction == "收入" {
			totalIn += t.Amount
		} else if t.OpCode == 87 || t.Direction == "证券转银行" || t.Direction == "支出" {
			totalOut += t.Amount
		}

		items = append(items, map[string]any{
			"date":      t.Date,
			"account":   t.Account,
			"direction": t.Direction,
			"amount":    t.Amount,
			"op_code":   t.OpCode,
		})
	}

	data := map[string]any{
		"days":       days,
		"count":      len(transfers),
		"total_in":   totalIn,
		"total_out":  totalOut,
		"net_in":     totalIn - totalOut,
		"transfers":  items,
	}

	formatter.PrintJSON(data)
}
