package service

import (
	"fmt"
	"strings"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

// buildCalendarParams 拉取账户 + 股票账户引用，按账户名过滤后构造聚合参数。
// combineCalendar、stock/*ProfitLoss、stockAnalysisSummary/* 三系接口共用同一套
// 「全账户合算」口径参数，故集中在这里构造。
// accountFilter 非空时只纳入名称匹配的账户（单账户口径）。
func buildCalendarParams(client *api.Client, accountFilter string) (api.CalendarParams, error) {
	accounts, err := client.GetAccountList()
	if err != nil {
		return api.CalendarParams{}, fmt.Errorf("获取账户失败: %v", err)
	}

	if accountFilter != "" {
		var matched []api.Account
		for _, a := range accounts {
			if strings.Contains(a.Name(), accountFilter) {
				matched = append(matched, a)
			}
		}
		if len(matched) == 0 {
			return api.CalendarParams{}, fmt.Errorf("没有匹配的账户: %s", accountFilter)
		}
		accounts = matched
	}

	// stock 账户的 zjzh 只能从 getAccounts 拿。若没有 stock 账户则跳过（省一次请求）。
	var stockRefs []api.StockAccountRef
	if hasStockAccount(accounts) {
		stockRefs, err = client.GetStockAccounts()
		if err != nil {
			return api.CalendarParams{}, fmt.Errorf("获取股票账户失败: %v", err)
		}
	}

	return api.BuildCalendarParams(accounts, stockRefs), nil
}

// hasStockAccount 判断账户里是否含 common/rzrq（需要 zjzh）。
func hasStockAccount(accounts []api.Account) bool {
	for _, a := range accounts {
		if a.Category == "common" || a.Category == "rzrq" {
			return true
		}
	}
	return false
}
