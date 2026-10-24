package service

import (
	"context"
	"fmt"
	"time"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

// TriggerServerCalcResult 是一次「直连触发服务端重算」的结果。
type TriggerServerCalcResult struct {
	Accounts int                   `json:"accounts"`
	Calc     *api.ServerCalcResult `json:"calc"`
}

// TriggerServerCalc 用纯 API 复刻"打开 PC 网页"的触发动作。
//
// 页面加载时的真实流程（见 pkg/api/servercalc.go 的说明）是：
// account_list 取账户 ID → init 触发服务端重新计算 → 每 500ms 轮询 status 直到完成
// → 之后再读持仓/资产接口。tzzb 之前只做"读"，从不调 init，所以服务端汇总值
// 可能在当日成交后长时间滞后（2026-10-22 BUG-stale-position-intraday）。
//
// 本函数只负责触发 + 等完成，不读取任何数据；调用方随后正常查询即可。
//
// timeout <= 0 时用默认 30 秒。
func TriggerServerCalc(client *api.Client, timeout time.Duration) (*TriggerServerCalcResult, error) {
	if client == nil {
		return nil, fmt.Errorf("未初始化 API 客户端")
	}
	scope, err := client.GetServerCalcScope()
	if err != nil {
		return nil, fmt.Errorf("读取账户列表失败: %w", err)
	}
	if scope.Empty() {
		return nil, fmt.Errorf("没有可触发的股票账户（manual_id / fund_key 均为空）")
	}
	res, err := client.TriggerServerCalc(context.Background(), scope, timeout)
	if res == nil {
		res = &api.ServerCalcResult{}
	}
	out := &TriggerServerCalcResult{
		Accounts: len(scope.ManualIDs) + len(scope.FundKeys) + len(scope.RzrqFundKeys),
		Calc:     res,
	}
	if err != nil {
		return out, err
	}
	return out, nil
}
