package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// 网页版（PC 页面）在加载/刷新数据时并不是"直接读取"服务端汇总值，而是：
//
//	account_list（拿账户 ID）→ init（触发服务端重新计算）→ 每 500ms 轮询 status
//	→ 直至全部完成 → 再读取持仓/资产接口
//
// 见 pc 首页 JS 的 triggerStockServerCal / pollStockServerCal / _pollStockServerCalStatus，
// 以及页面提示"当您完成新的交易后，请在投资账本APP上重新上传数据到网页版"。
//
// CLI 之前只做最后一步"读取"，从不调 init —— 这就是 2026-10-22 卖出后
// 服务端汇总值（持仓/现金）滞后 20 分钟以上、而打开一次网页后立刻正确的根因。
const (
	// ServerCalcFailed / ServerCalcDone / ServerCalcRunning 与网页 JS 里的状态码一致。
	ServerCalcFailed  = -1
	ServerCalcDone    = 0
	ServerCalcRunning = 1

	// serverCalcPollInterval 与网页轮询间隔一致。
	serverCalcPollInterval = 500 * time.Millisecond
	// serverCalcDefaultTimeout 是等待服务端算完的默认上限。
	serverCalcDefaultTimeout = 30 * time.Second
)

// ServerCalcStatus 描述服务端计算任务中单个账户的状态。
type ServerCalcStatus struct {
	// AccountKey 是账户标识（manual_id / fund_key / rzrq_fund_key 三者中非空的那个）。
	AccountKey  string `json:"account_key"`
	ManualID    string `json:"manual_id,omitempty"`
	FundKey     string `json:"fund_key,omitempty"`
	RzrqFundKey string `json:"rzrq_fund_key,omitempty"`
	Status      int    `json:"status"` // -1 失败 / 0 完成 / 1 计算中
}

// ServerCalcScope 指定触发服务端重新计算的范围，与网页 init 的请求参数一致。
type ServerCalcScope struct {
	ManualIDs    []string
	FundKeys     []string
	RzrqFundKeys []string
}

// accountCalcExData 只解析"触发服务端计算"所需的账户标识。
//
// 单独定义而不复用 Account 结构体，是因为 manual 账户的 fund_key 在真实响应里是
// JSON 数字（如 2179680），而 Account 的字段是 string；本文件用 flexStr 兼容两种形态，
// 避免把 fund_key 的兼容性改动扩散到其它命令的解析路径。
type accountCalcExData struct {
	Manual []struct {
		ManualID flexStr `json:"manual_id"`
	} `json:"manual"`
	Common []struct {
		FundKey flexStr `json:"fund_key"`
	} `json:"common"`
	Rzrq []struct {
		RzrqFundKey flexStr `json:"rzrq_fund_key"`
	} `json:"rzrq"`
	Fund []struct {
		FundKey flexStr `json:"fund_key"`
	} `json:"fund"`
	Monitor []struct {
		FundKey flexStr `json:"fund_key"`
	} `json:"monitor"`
}

// GetServerCalcScope 读取账户列表并构造服务端计算范围（与网页 init 的请求参数一致）：
//   - 手工账户传 manual_id（它的 fund_key 就是自己的数字 id，不混入 fund_key 列表）
//   - 普通账户传 fund_key
//   - 两融账户传 rzrq_fund_key
//   - 基金账户走 init_fund/status_fund，另一套接口，不在本范围
//   - 模拟账户（monitor）不参与计算
func (c *Client) GetServerCalcScope() (ServerCalcScope, error) {
	ex, err := c.call(epAccountList, nil)
	if err != nil {
		return ServerCalcScope{}, err
	}
	var d accountCalcExData
	if err := json.Unmarshal(ex, &d); err != nil {
		return ServerCalcScope{}, fmt.Errorf("解析账户数据失败: %v", err)
	}
	var s ServerCalcScope
	for _, m := range d.Manual {
		if k := string(m.ManualID); k != "" {
			s.ManualIDs = append(s.ManualIDs, k)
		}
	}
	for _, f := range d.Common {
		if k := string(f.FundKey); k != "" {
			s.FundKeys = append(s.FundKeys, k)
		}
	}
	for _, r := range d.Rzrq {
		if k := string(r.RzrqFundKey); k != "" {
			s.RzrqFundKeys = append(s.RzrqFundKeys, k)
		}
	}
	return s, nil
}

// Empty 报告是否有可计算的股票账户。
func (s ServerCalcScope) Empty() bool {
	return len(s.ManualIDs) == 0 && len(s.FundKeys) == 0 && len(s.RzrqFundKeys) == 0
}

func (s ServerCalcScope) params() map[string]string {
	return map[string]string{
		"manual_id":     strings.Join(s.ManualIDs, ","),
		"fund_key":      strings.Join(s.FundKeys, ","),
		"rzrq_fund_key": strings.Join(s.RzrqFundKeys, ","),
	}
}

// ServerCalcResult 是一次触发的结果。
type ServerCalcResult struct {
	Triggered bool               `json:"triggered"`
	Polls     int                `json:"polls"`
	Elapsed   time.Duration      `json:"elapsed"`
	Statuses  []ServerCalcStatus `json:"statuses,omitempty"`
}

// triggerServerCalc 调 init 接口触发服务端重新计算。网页侧该请求返回 {}，是纯动作。
func (c *Client) triggerServerCalc(scope ServerCalcScope) error {
	_, err := c.call(epAccountInit, scope.params())
	return err
}

// pollServerCalc 调 status 接口读取各账户计算状态。
func (c *Client) pollServerCalc(scope ServerCalcScope) ([]ServerCalcStatus, error) {
	ex, err := c.call(epAccountStatus, scope.params())
	if err != nil {
		return nil, err
	}
	var d struct {
		Data []struct {
			ManualID    flexStr `json:"manual_id"`
			FundKey     flexStr `json:"fund_key"`
			RzrqFundKey flexStr `json:"rzrq_fund_key"`
			Status      int     `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(ex, &d); err != nil {
		return nil, fmt.Errorf("解析计算状态失败: %v", err)
	}
	out := make([]ServerCalcStatus, 0, len(d.Data))
	for _, s := range d.Data {
		st := ServerCalcStatus{
			ManualID:    string(s.ManualID),
			FundKey:     string(s.FundKey),
			RzrqFundKey: string(s.RzrqFundKey),
			Status:      s.Status,
		}
		switch {
		case st.ManualID != "":
			st.AccountKey = "manual:" + st.ManualID
		case st.RzrqFundKey != "":
			st.AccountKey = "rzrq:" + st.RzrqFundKey
		default:
			st.AccountKey = "fund:" + st.FundKey
		}
		out = append(out, st)
	}
	return out, nil
}

// TriggerServerCalc 复刻网页加载时的触发动作：init → 轮询 status 到全部完成。
//
// timeout 为 0 时用默认 30 秒；ctx 只用于两次轮询之间的取消（请求本身受
// Client 的上下文/超时控制）。返回的 result 在超时/失败时仍会带上已轮询信息，
// 便于调用方判断卡在哪一步。
func (c *Client) TriggerServerCalc(ctx context.Context, scope ServerCalcScope, timeout time.Duration) (*ServerCalcResult, error) {
	if scope.Empty() {
		return &ServerCalcResult{}, nil
	}
	if timeout <= 0 {
		timeout = serverCalcDefaultTimeout
	}
	if ctx == nil {
		ctx = context.Background()
	}
	res := &ServerCalcResult{}
	if err := c.triggerServerCalc(scope); err != nil {
		return res, fmt.Errorf("触发服务端计算失败: %w", err)
	}
	res.Triggered = true
	start := time.Now()
	for {
		sts, err := c.pollServerCalc(scope)
		if err != nil {
			return res, fmt.Errorf("轮询服务端计算状态失败: %w", err)
		}
		res.Polls++
		res.Statuses = sts
		running := 0
		var failed []string
		for _, s := range sts {
			switch s.Status {
			case ServerCalcRunning:
				running++
			case ServerCalcFailed:
				failed = append(failed, s.AccountKey)
			}
		}
		res.Elapsed = time.Since(start)
		if len(failed) > 0 {
			return res, fmt.Errorf("服务端计算失败: %s", strings.Join(failed, ", "))
		}
		if running == 0 {
			return res, nil
		}
		if time.Since(start) > timeout {
			return res, fmt.Errorf("等待服务端计算超时（%s，已轮询 %d 次，仍有 %d 个账户计算中）",
				timeout, res.Polls, running)
		}
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		case <-time.After(serverCalcPollInterval):
		}
	}
}
