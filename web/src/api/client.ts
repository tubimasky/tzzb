import axios, { AxiosError } from 'axios';
import type {
  AccountSummaryDTO,
  PositionsDTO,
  CalendarMonthDTO,
  CalendarYearDTO,
  DetailDTO,
  StockProfitDTO,
  AnalysisDistDTO,
  AnalysisPosRateDTO,
  TrendDTO,
} from '../types/api';

const client = axios.create({
  baseURL: '/api',
  // 趋势接口按账户串行拉取上游，耗时可达数秒，留足超时
  timeout: 60000,
});

/** 把 axios / 业务错误整理成可读字符串 */
export function apiErrorMessage(err: unknown, fallback = '加载失败'): string {
  if (axios.isAxiosError(err)) {
    const ax = err as AxiosError<{ error?: string; message?: string }>;
    const body = ax.response?.data;
    if (body && typeof body === 'object') {
      if (typeof body.error === 'string' && body.error) return body.error;
      if (typeof body.message === 'string' && body.message) return body.message;
    }
    if (ax.response?.status) {
      return `HTTP ${ax.response.status}${ax.message ? `: ${ax.message}` : ''}`;
    }
    return ax.message || fallback;
  }
  if (err instanceof Error) return err.message || fallback;
  return fallback;
}

export const api = {
  getAccounts: () => client.get<AccountSummaryDTO>('/accounts'),

  getPositions: (params?: { account?: string; type?: string }) =>
    client.get<PositionsDTO>('/positions', {
      params: {
        account: params?.account || undefined,
        type: params?.type && params.type !== 'all' ? params.type : undefined,
      },
    }),

  // 类别参数后端读的是 cat（total/stock/fund），别写成 category
  getCalendarMonth: (month: string, account?: string, cat?: string) =>
    client.get<CalendarMonthDTO>('/calendar/month', {
      params: { month, account: account || undefined, cat: cat || undefined },
    }),

  getCalendarYear: (year: string | number, account?: string, cat?: string) =>
    client.get<CalendarYearDTO>('/calendar/year', {
      params: { year, account: account || undefined, cat: cat || undefined },
    }),

  /** 股票盈亏：某年各月 */
  getStockProfitMonthly: (year?: string, account?: string) =>
    client.get<StockProfitDTO>('/stockprofit/monthly', {
      params: { year: year || undefined, account: account || undefined },
    }),

  /** 股票盈亏：区间每日（start/end 为 YYYYMMDD） */
  getStockProfitRange: (start?: string, end?: string, account?: string) =>
    client.get<StockProfitDTO>('/stockprofit/range', {
      params: {
        start: start || undefined,
        end: end || undefined,
        account: account || undefined,
      },
    }),

  /** 持仓分布（市值占比） */
  getAnalysisDistribution: (account?: string) =>
    client.get<AnalysisDistDTO>('/analysis/distribution', {
      params: { account: account || undefined },
    }),

  /** 各股票账户仓位率 */
  getAnalysisPosRate: (account?: string) =>
    client.get<AnalysisPosRateDTO>('/analysis/posrate', {
      params: { account: account || undefined },
    }),

  /**
   * 资产趋势曲线。上游按天数分档返回，
   * 且需逐账户拉取，耗时数秒 —— 调用方应非阻塞加载。
   */
  getTrend: (params?: {
    days?: number;
    category?: string;
    account?: string;
    benchmark?: boolean;
  }) =>
    client.get<TrendDTO>('/trend', {
      params: {
        days: params?.days || undefined,
        category: params?.category && params.category !== 'all' ? params.category : undefined,
        account: params?.account || undefined,
        benchmark: params?.benchmark ? 1 : undefined,
      },
      timeout: 60000, // 逐账户 fan-out，比其它接口慢得多
    }),

  /** 单股钻取：合并持仓 / 建仓记录 / 历史成交 / 清仓汇总 */
  getDetail: (code: string, params?: { account?: string; from?: string; to?: string }) =>
    client.get<DetailDTO>(`/detail/${encodeURIComponent(code)}`, {
      params: {
        account: params?.account || undefined,
        from: params?.from || undefined,
        to: params?.to || undefined,
      },
    }),
};
