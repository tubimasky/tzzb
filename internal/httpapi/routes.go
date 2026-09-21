package httpapi

// routes 注册所有 /api 路由。静态资源路由由 SetStaticFS 单独注册。
func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/accounts", s.handleAccounts)
	s.mux.HandleFunc("GET /api/positions", s.handlePositions)
	s.mux.HandleFunc("GET /api/positions/merged", s.handleMergedPositions)
	s.mux.HandleFunc("GET /api/calendar/month", s.handleCalendarMonth)
	s.mux.HandleFunc("GET /api/calendar/year", s.handleCalendarYear)
	s.mux.HandleFunc("GET /api/stockprofit/monthly", s.handleStockProfitMonthly)
	s.mux.HandleFunc("GET /api/stockprofit/range", s.handleStockProfitRange)
	s.mux.HandleFunc("GET /api/analysis/distribution", s.handleAnalysisDistribution)
	s.mux.HandleFunc("GET /api/analysis/posrate", s.handleAnalysisPosRate)
	s.mux.HandleFunc("GET /api/detail/{code}", s.handleDetail)
	s.mux.HandleFunc("GET /api/trend", s.handleTrend)
}
