// API 响应类型，与 internal/httpapi handlers 的 DTO 对齐。
// 比率字段约定：
//   - rate / total_rate / hold_rate / day_rate：已是百分数（15.2 表示 15.2%）
//   - position_rate：小数比率（0.788 表示 78.8%）

export interface AccountItem {
  name: string;
  category: string;
  total_assets: number;
  hold_profit: number;
  today_profit: number;
  total_profit: number;
  position_rate: number; // 小数
  rate: number | null; // 本金缺失时不可计算
}

export interface DataWarning {
  account: string;
  component?: string;
  source?: string;
  message: string;
}

export interface AccountSummary {
  total_principal: number | null;
  total_assets: number;
  hold_profit: number;
  today_profit: number;
  total_profit: number;
  total_rate: number | null;
  profit_scope: 'holding';
}

export interface AccountSummaryDTO {
  accounts: AccountItem[];
  complete: boolean;
  warnings: DataWarning[];
  summary: AccountSummary;
}

export interface PositionItem {
  account: string;
  category: string;
  code: string;
  name: string;
  count: number;
  cost: number;
  price: number;
  value: number;
  hold_profit: number;
  hold_rate: number; // 百分数
  day_profit: number;
  day_rate: number; // 百分数
}

export interface PositionsSummary {
  value: number;
  hold_profit: number;
  day_profit: number;
  count: number;
}

export interface PositionsDTO {
  positions: PositionItem[];
  summary: PositionsSummary;
}

export interface DayCell {
  day: number;
  date: string;
  total_pl: number;
  stock_pl: number;
  fund_pl: number;
  has_data: boolean;
}

export interface CodePL {
  code: string;
  name: string;
  profit_loss: number;
  type: string;
}

export interface CalendarMonthDTO {
  year: number;
  month: number;
  weeks: DayCell[][];
  top_codes: CodePL[];
  total_pl: number;
  stock_pl: number;
  fund_pl: number;
}

export interface MonthPL {
  date: string;
  total_pl: number;
  stock_pl: number;
  fund_pl: number;
}

export interface CalendarYearDTO {
  year: string;
  months: MonthPL[];
  top_codes: CodePL[];
  total_pl: number;
  stock_pl: number;
  fund_pl: number;
}

// --- 单股钻取 /api/detail/{code} ---
// 注意：merge/records/clear 的 profit_rate 是小数（-0.0223 = -2.23%），与持仓 hold_rate 不同。

export interface MergeDetail {
  stock_code: string;
  stock_name: string;
  count: number;
  hold_days: number;
  price_buy_av: number;
  price_curr: number;
  market_value: number;
  zcb: number; // 总成本
  yke: number; // 浮动盈亏
  profit_rate: number; // 小数
  fee: number;
  come_profit: number;
  clear_flag: string; // "1" = 已清仓
}

export interface HoldRecord {
  start_date: string; // YYYYMMDD
  end_date: string;
  profit: number;
  profit_rate: number; // 小数
}

export interface StockTrade {
  trans_date: string;
  trans_time: string;
  op: string; // 1=买入 2=卖出
  price: number;
  count: number;
  amount: number;
  fee: number;
  profit: number;
  account_name: string;
}

export interface ClearSummary {
  start_date: string;
  end_date: string;
  hold_days: number;
  fee: number;
  total_buy: number;
  total_sell: number;
  total_profit: number;
  total_profit_rate: number; // 小数
  has_data: boolean;
}

export interface StockDetail {
  account_name: string;
  merge: MergeDetail | null;
  records: HoldRecord[];
  trades: StockTrade[];
  clear: ClearSummary | null;
}

export interface DetailDTO {
  code: string;
  details: StockDetail[];
}

// --- 股票盈亏 /api/stockprofit/{monthly,range} ---
// 注意：rate / sz_rate 是小数（0.0327 = 3.27%），用 formatRatio 而非 formatPct。
// top_codes[].type 在该接口恒为空串（与 calendar 不同），故无法按类型过滤。

export interface StockProfitPoint {
  date: string; // monthly: YYYY-MM；range: YYYY-MM-DD
  profit_loss: number;
  rate: number; // 小数
  sz_rate: number; // 小数
}

export interface StockProfitDTO {
  points: StockProfitPoint[];
  top_codes: CodePL[];
  total_pl: number;
  total_rate: number; // 小数
  total_sz: number; // 小数
}

// --- 仓位分析 /api/analysis/* ---

export interface DistItem {
  code: string;
  name: string;
  value: number;
  weight: number; // 百分数
  market: string; // 1=深 2=沪
}

export interface AnalysisDistDTO {
  items: DistItem[];
  total: number;
}

export interface AccountPosRate {
  name: string;
  broker: string;
  stock_value: number;
  money_remain: number;
  total: number;
  rate: number; // 百分数
  zjzh: string;
}

export interface AnalysisPosRateDTO {
  accounts: AccountPosRate[];
}

// --- 资产趋势 /api/trend ---
// position_rate / profit_rate / *_return / benchmark* / max_drawdown 均为小数。
// 上游按天数分档：days=7 与 30 返回同一批点，90 与 365 同理，故只暴露 30/60/90。

export interface TrendPoint {
  date: string; // YYYY-MM-DD
  total_asset: number;
  profit: number;
  profit_rate: number;
  cash: number;
  market_value: number;
  position_rate: number;
  fund_in: number;
  fund_out: number;
  asset_change: number; // 首日为 0
  daily_return: number; // 剔除银证转账后的当日收益率
  cum_return: number; // 相对区间首日的复利累计收益率
  has_return: boolean;
  benchmark: number; // 上证当日涨跌
  benchmark_cum: number; // 上证相对区间首日的累计涨跌，与 cum_return 同轴可比
  has_benchmark: boolean;
}

export interface TrendStats {
  first_date: string;
  first_asset: number;
  last_date: string;
  last_asset: number;
  asset_change: number;
  profit_change: number;
  max_drawdown: number; // 基于总资产曲线（含出入金）
  has_max_drawdown: boolean;
  cum_return: number; // 区间累计收益率（已剔除出入金）
  has_cum_return: boolean;
  return_drawdown: number; // 收益曲线上的最大回撤，不受出入金干扰
  has_return_drawdown: boolean;
  benchmark_cum: number;
  excess: number; // cum_return - benchmark_cum
  has_benchmark: boolean;
}

export interface TrendDTO {
  category: string;
  days: number;
  complete: boolean;
  warnings: DataWarning[];
  points: TrendPoint[];
  stats: TrendStats;
}
