package service

import (
	"fmt"
	"os"

	"github.com/liyanran/tzzb-cli/internal/formatter"
)

// PositionCommandService 持仓命令业务逻辑（封装重复的加载+渲染流程）
type PositionCommandService struct {
	positionSvc *PositionService
	renderSvc   *RenderService
}

// NewPositionCommandService 创建持仓命令服务
func NewPositionCommandService(positionSvc *PositionService, renderSvc *RenderService) *PositionCommandService {
	return &PositionCommandService{
		positionSvc: positionSvc,
		renderSvc:   renderSvc,
	}
}

// ExecuteListCommand 执行 list 命令逻辑
func (s *PositionCommandService) ExecuteListCommand(accountFilter, typeFilter string, flatList, outputJSON bool) {
	accounts, err := s.positionSvc.LoadAccounts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载账户失败: %v\n", err)
		os.Exit(1)
	}
	rows := s.positionSvc.CollectRows(accounts, accountFilter, typeFilter)
	if len(rows) == 0 {
		if outputJSON {
			formatter.PrintJSON([]any{})
		} else {
			fmt.Println("暂无持仓")
		}
		return
	}
	ctx := s.positionSvc.LoadLiveContext(accounts, rows)
	if outputJSON {
		if flatList {
			s.renderSvc.PrintJSON(s.renderSvc.BuildFlatJSON(rows, ctx))
		} else {
			s.renderSvc.PrintJSON(s.renderSvc.BuildByAccountJSON(rows, ctx))
		}
		return
	}
	if flatList {
		s.renderSvc.RenderFlat(rows, ctx)
		return
	}
	s.renderSvc.RenderByAccount(rows, ctx)
}

// ExecuteTotalCommand 执行 total/stock/fund 命令逻辑
func (s *PositionCommandService) ExecuteTotalCommand(accountFilter, typeFilter, sortBy string, sortAsc, outputJSON bool, withLiveContext bool) {
	accounts, err := s.positionSvc.LoadAccounts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载账户失败: %v\n", err)
		os.Exit(1)
	}
	rows := s.positionSvc.CollectRows(accounts, accountFilter, typeFilter)
	if len(rows) == 0 {
		if outputJSON {
			formatter.PrintJSON([]any{})
		} else {
			msg := "暂无持仓"
			if typeFilter == "stock" {
				msg = "暂无股票/ETF 持仓"
			} else if typeFilter == "fund" {
				msg = "暂无基金持仓"
			}
			fmt.Println(msg)
		}
		return
	}

	var ctx *LiveContext
	if withLiveContext {
		ctx = s.positionSvc.LoadLiveContext(accounts, rows)
	}

	merged := s.positionSvc.MergeByCode(rows)
	if outputJSON {
		s.renderSvc.PrintJSON(s.renderSvc.BuildTotalJSON(merged, ctx, sortBy, sortAsc))
		return
	}
	s.renderSvc.RenderTotal(merged, ctx, sortBy, sortAsc)
}
