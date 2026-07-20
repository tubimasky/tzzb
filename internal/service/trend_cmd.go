package service

import (
	"fmt"
	"os"

	"github.com/liyanran/tzzb-cli/pkg/api"
)

// TrendCommandService 趋势命令服务（封装重复的加载+渲染流程）
type TrendCommandService struct {
	trendService  *TrendService
	renderService *TrendRenderService
	jsonService   *TrendJSONService
	client        *api.Client
}

// NewTrendCommandService 创建趋势命令服务
func NewTrendCommandService(client *api.Client) *TrendCommandService {
	trendSvc := NewTrendService(client)
	return &TrendCommandService{
		trendService:  trendSvc,
		renderService: NewTrendRenderService(trendSvc),
		jsonService:   NewTrendJSONService(trendSvc),
		client:        client,
	}
}

// ExecuteShowCommand 执行 show 命令逻辑
func (s *TrendCommandService) ExecuteShowCommand(accountFilter string, days int, limit int, outputJSON bool) {
	accounts, err := s.client.GetAccountList()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取账户失败: %v\n", err)
		os.Exit(1)
	}

	matched := s.trendService.FilterAccounts(accounts, accountFilter)
	if len(matched) == 0 {
		fmt.Println("没有匹配的账户")
		os.Exit(0)
	}

	allData := s.trendService.LoadTrendForAccounts(matched, days)

	if outputJSON {
		var results []map[string]any
		for _, data := range allData {
			results = append(results, s.jsonService.BuildSingleAccountJSON(data, limit))
		}
		s.jsonService.PrintJSON(map[string]any{"accounts": results})
		return
	}

	for _, data := range allData {
		s.renderService.RenderSingleAccountTrend(data, limit)
	}
}

// ExecuteSummaryCommand 执行 summary 命令逻辑
func (s *TrendCommandService) ExecuteSummaryCommand(category string, days int, limit int, outputJSON bool) {
	accounts, err := s.client.GetAccountList()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取账户失败: %v\n", err)
		os.Exit(1)
	}

	groups := s.trendService.GroupAccountsByCategory(accounts)

	if outputJSON {
		result := map[string]any{}
		switch category {
		case "all", "":
			result["all"] = s.jsonService.BuildAggregatedJSON("全部账户", s.trendService.AggregateTrendData(groups.All, days), days, limit)
			if len(groups.Stock) > 0 {
				result["stock"] = s.jsonService.BuildAggregatedJSON("股票账户", s.trendService.AggregateTrendData(groups.Stock, days), days, limit)
			}
			if len(groups.Fund) > 0 {
				result["fund"] = s.jsonService.BuildAggregatedJSON("基金账户", s.trendService.AggregateTrendData(groups.Fund, days), days, limit)
			}
		case "stock":
			if len(groups.Stock) > 0 {
				result["stock"] = s.jsonService.BuildAggregatedJSON("股票账户", s.trendService.AggregateTrendData(groups.Stock, days), days, limit)
			} else {
				result["error"] = "没有股票账户"
			}
		case "fund":
			if len(groups.Fund) > 0 {
				result["fund"] = s.jsonService.BuildAggregatedJSON("基金账户", s.trendService.AggregateTrendData(groups.Fund, days), days, limit)
			} else {
				result["error"] = "没有基金账户"
			}
		default:
			fmt.Fprintf(os.Stderr, "未知的账户类型: %s（支持 all/stock/fund）\n", category)
			os.Exit(1)
		}
		s.jsonService.PrintJSON(result)
		return
	}

	// 文本渲染
	switch category {
	case "all", "":
		s.renderService.RenderAggregatedTrend("全部账户", s.trendService.AggregateTrendData(groups.All, days), days, limit)
		if len(groups.Stock) > 0 {
			s.renderService.RenderAggregatedTrend("股票账户", s.trendService.AggregateTrendData(groups.Stock, days), days, limit)
		}
		if len(groups.Fund) > 0 {
			s.renderService.RenderAggregatedTrend("基金账户", s.trendService.AggregateTrendData(groups.Fund, days), days, limit)
		}
	case "stock":
		if len(groups.Stock) > 0 {
			s.renderService.RenderAggregatedTrend("股票账户", s.trendService.AggregateTrendData(groups.Stock, days), days, limit)
		} else {
			fmt.Println("没有股票账户")
		}
	case "fund":
		if len(groups.Fund) > 0 {
			s.renderService.RenderAggregatedTrend("基金账户", s.trendService.AggregateTrendData(groups.Fund, days), days, limit)
		} else {
			fmt.Println("没有基金账户")
		}
	default:
		fmt.Fprintf(os.Stderr, "未知的账户类型: %s（支持 all/stock/fund）\n", category)
		os.Exit(1)
	}
}
