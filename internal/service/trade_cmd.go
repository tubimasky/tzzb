package service

import (
	"fmt"
	"os"
)

// TradeCommandService 交易命令服务（封装重复的加载+渲染流程）
type TradeCommandService struct {
	tradeService  *TradeService
	renderService *TradeRenderService
	jsonService   *TradeJSONService
}

// NewTradeCommandService 创建交易命令服务
func NewTradeCommandService(tradeService *TradeService) *TradeCommandService {
	return &TradeCommandService{
		tradeService:  tradeService,
		renderService: NewTradeRenderService(tradeService),
		jsonService:   NewTradeJSONService(tradeService),
	}
}

// ExecuteListCommand 执行 list 命令逻辑
func (s *TradeCommandService) ExecuteListCommand(accountFilter string, outputJSON bool) {
	matched, err := s.tradeService.MatchTradeAccounts(accountFilter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if len(matched) == 0 {
		fmt.Println("没有匹配的账户")
		os.Exit(0)
	}

	packs := s.tradeService.LoadAllTradeRecords(matched)

	if outputJSON {
		results := s.jsonService.BuildListJSON(packs)
		if len(results) == 0 {
			s.jsonService.PrintJSON([]any{})
		} else {
			s.jsonService.PrintJSON(results)
		}
		return
	}

	total := s.renderService.RenderRecordsByAccount(packs)
	if total == 0 {
		fmt.Println("暂无交易记录")
	}
}

// ExecuteSummaryCommand 执行 summary 命令逻辑
func (s *TradeCommandService) ExecuteSummaryCommand(accountFilter string, outputJSON bool) {
	matched, err := s.tradeService.MatchTradeAccounts(accountFilter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if len(matched) == 0 {
		fmt.Println("没有匹配的账户")
		os.Exit(0)
	}

	packs := s.tradeService.LoadAllTradeRecords(matched)
	if len(packs) == 0 {
		if outputJSON {
			s.jsonService.PrintJSON(map[string]any{"error": "no trade records"})
		} else {
			fmt.Println("暂无交易记录")
		}
		return
	}

	all := s.tradeService.FlattenRecords(packs)

	if outputJSON {
		result := s.jsonService.BuildSummaryJSON(packs, all)
		s.jsonService.PrintJSON(result)
		return
	}

	s.renderService.RenderAccountSummary(packs)
	fmt.Println()
	s.renderService.RenderCodeSummary(all, 40)
	fmt.Println()
	s.renderService.RenderGrandSummary(all)
}
