package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

// DetailService 单只股票钻取明细业务逻辑（对齐插件 handstock/itemDetail 四接口）。
type DetailService struct {
	client *api.Client
}

// NewDetailService 创建明细服务。
func NewDetailService(client *api.Client) *DetailService {
	return &DetailService{client: client}
}

// StockDetail 是单个账户下某只股票的全维度钻取结果。
type StockDetail struct {
	Coord   api.DetailCoord
	Merge   *api.StockMergeDetail
	Records []api.HoldRecord
	Trades  []api.StockTrade
	Clear   *api.ClearSummary
}

// LoadDetail 定位持有该代码的账户并逐个钻取明细。
// accountFilter 非空时只保留名称匹配的账户；from/end 为空时用建仓日到今天。
func (s *DetailService) LoadDetail(code, accountFilter, from, to string) ([]StockDetail, error) {
	accounts, err := s.client.GetAccountList()
	if err != nil {
		return nil, fmt.Errorf("获取账户失败: %v", err)
	}

	coords := locateCoords(accounts, code, accountFilter)
	if len(coords) == 0 {
		return nil, fmt.Errorf("未在持仓中找到代码 %s（清仓股请用 --from/--to 指定区间查询）", code)
	}

	var out []StockDetail
	for _, coord := range coords {
		d, err := s.loadOne(coord, from, to)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// loadOne 钻取单个账户坐标下的四块明细。
func (s *DetailService) loadOne(coord api.DetailCoord, from, to string) (StockDetail, error) {
	detail := StockDetail{Coord: coord}

	merge, err := s.client.GetStockMergeDetail(coord)
	if err != nil {
		return detail, fmt.Errorf("获取合并明细失败: %v", err)
	}
	detail.Merge = merge

	records, err := s.client.GetStockHoldRecords(coord)
	if err != nil {
		return detail, fmt.Errorf("获取建仓记录失败: %v", err)
	}
	detail.Records = records

	// 日期区间：优先用参数，其次用建仓记录最早日，最后回落近一年。
	start, end := resolveRange(from, to, records)

	trades, err := s.client.GetStockTradeHistory(coord, start, end)
	if err != nil {
		return detail, fmt.Errorf("获取历史成交失败: %v", err)
	}
	detail.Trades = trades

	clear, err := s.client.GetStockClearSummary(coord, start, end)
	if err != nil {
		return detail, fmt.Errorf("获取清仓汇总失败: %v", err)
	}
	detail.Clear = clear

	return detail, nil
}

// locateCoords 扫描账户持仓，找出持有该代码的股票账户坐标。
// 只看股票类账户（common/rzrq/manual）；fund 账户不参与钻取（接口按 stock_code 匹配）。
func locateCoords(accounts []api.Account, code, accountFilter string) []api.DetailCoord {
	var coords []api.DetailCoord
	for _, a := range accounts {
		if a.Category == "fund" {
			continue
		}
		if accountFilter != "" && !strings.Contains(a.Name(), accountFilter) {
			continue
		}
		for _, p := range a.Positions {
			if p.Code != code {
				continue
			}
			coords = append(coords, api.DetailCoord{
				Fundkey:     a.FundKey,
				RzrqFundkey: a.RzrqFundKey,
				Manualid:    a.ManualID,
				StockCode:   code,
				Market:      p.Market,
				AccountName: a.Name(),
			})
			break // 同账户同代码只取一次
		}
	}
	return coords
}

// resolveRange 决定历史成交/清仓查询的日期区间（YYYYMMDD）。
func resolveRange(from, to string, records []api.HoldRecord) (start, end string) {
	end = to
	if end == "" {
		end = api.FormatCompactDate(time.Now())
	}
	if from != "" {
		return from, end
	}
	// 用建仓记录里最早的 start_date。
	earliest := ""
	for _, r := range records {
		if r.StartDate == "" {
			continue
		}
		if earliest == "" || r.StartDate < earliest {
			earliest = r.StartDate
		}
	}
	if earliest != "" {
		return earliest, end
	}
	// 回落近一年。
	return api.FormatCompactDate(time.Now().AddDate(-1, 0, 0)), end
}
