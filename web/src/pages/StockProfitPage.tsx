import { useEffect, useMemo, useState } from 'react';
import { api, apiErrorMessage } from '../api/client';
import EChart from '../components/EChart';
import type { EChartsOption } from '../charts/echarts';
import {
  axisRatioPct,
  axisWan,
  categoryAxis,
  numLabel,
  palette,
  plColor,
  tipDot,
  tipMoney,
  tipRatio,
  valueAxis,
  zeroMarkLine,
} from '../charts/theme';
import type { CodePL, StockProfitDTO, StockProfitPoint } from '../types/api';
import { formatMoney, profitClass } from '../utils/format';
import './StockProfitPage.css';

type ViewMode = 'monthly' | 'range';

/** YYYYMMDD（后端 stock/*ProfitLoss 要求无横杠，带横杠会报服务器繁忙） */
function toCompact(d: string): string {
  return d.replaceAll('-', '');
}

/** <input type="date"> 用的 YYYY-MM-DD */
function isoDate(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function monthStart(): string {
  const now = new Date();
  return isoDate(new Date(now.getFullYear(), now.getMonth(), 1));
}

function today(): string {
  return isoDate(new Date());
}

function currentYear(): string {
  return String(new Date().getFullYear());
}

/** 近若干年供选择 */
function yearOptions(): string[] {
  const y = new Date().getFullYear();
  return [y, y - 1, y - 2, y - 3, y - 4].map(String);
}

export default function StockProfitPage() {
  const [view, setView] = useState<ViewMode>('monthly');
  const [year, setYear] = useState(currentYear());
  const [start, setStart] = useState(monthStart());
  const [end, setEnd] = useState(today());
  const [account, setAccount] = useState('');

  const [data, setData] = useState<StockProfitDTO | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // 账户下拉复用 /accounts（只需名字）
  const [accountNames, setAccountNames] = useState<string[]>([]);
  useEffect(() => {
    let cancelled = false;
    api
      .getAccounts()
      .then((res) => {
        if (!cancelled) setAccountNames(res.data.accounts.map((a) => a.name));
      })
      .catch(() => {
        // 下拉是可选增强，失败不影响主图
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    const req =
      view === 'monthly'
        ? api.getStockProfitMonthly(year, account)
        : api.getStockProfitRange(toCompact(start), toCompact(end), account);

    req
      .then((res) => {
        if (!cancelled) {
          setData(res.data);
          setLoading(false);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setError(apiErrorMessage(err));
          setData(null);
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [view, year, start, end, account]);

  const points = data?.points ?? [];

  return (
    <div className="page stockprofit-page">
      <header className="page-header">
        <div className="page-header-text">
          <div className="page-eyebrow">Stock P&amp;L</div>
          <h1 className="page-title">
            股票<em>盈亏</em>
          </h1>
          <p className="page-subtitle">
            按月/按日的股票盈亏走势，叠加账户收益率与同期上证涨跌对比
          </p>
        </div>
      </header>

      <div className="filters">
        <div className="filter-block">
          <span className="field-label">维度</span>
          <div className="segmented" role="tablist" aria-label="时间维度">
            <button
              type="button"
              role="tab"
              aria-selected={view === 'monthly'}
              className={view === 'monthly' ? 'active' : undefined}
              onClick={() => setView('monthly')}
            >
              按月
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={view === 'range'}
              className={view === 'range' ? 'active' : undefined}
              onClick={() => setView('range')}
            >
              按日区间
            </button>
          </div>
        </div>

        {view === 'monthly' ? (
          <label className="filter-field">
            <span className="field-label">年份</span>
            <select value={year} onChange={(e) => setYear(e.target.value)}>
              {yearOptions().map((y) => (
                <option key={y} value={y}>
                  {y} 年
                </option>
              ))}
            </select>
          </label>
        ) : (
          <>
            <label className="filter-field">
              <span className="field-label">起始日</span>
              <input type="date" value={start} max={end} onChange={(e) => setStart(e.target.value)} />
            </label>
            <label className="filter-field">
              <span className="field-label">结束日</span>
              <input
                type="date"
                value={end}
                min={start}
                max={today()}
                onChange={(e) => setEnd(e.target.value)}
              />
            </label>
          </>
        )}

        <label className="filter-field">
          <span className="field-label">账户</span>
          <select value={account} onChange={(e) => setAccount(e.target.value)}>
            <option value="">全部账户</option>
            {accountNames.map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </select>
        </label>
      </div>

      {error && <div className="error inline-error">加载失败: {error}</div>}

      <SummaryStrip data={data} loading={loading} />

      <div className="chart-grid single">
        <section className="chart-card">
          <div className="chart-card-head">
            <h2 className="chart-card-title">
              {view === 'monthly' ? '各月盈亏与收益率' : '每日盈亏与收益率'}
            </h2>
            <span className="chart-card-meta">柱=盈亏 · 线=账户收益率 vs 上证</span>
          </div>
          <EChart
            option={usePLOption(points, view)}
            height={340}
            loading={loading}
            empty={!loading && points.length === 0 ? '该区间暂无盈亏数据' : null}
            ariaLabel="盈亏与收益率走势图"
          />
        </section>

        <section className="chart-card">
          <div className="chart-card-head">
            <h2 className="chart-card-title">累计盈亏曲线</h2>
            <span className="chart-card-meta">区间内逐点累加</span>
          </div>
          <EChart
            option={useCumulativeOption(points)}
            height={260}
            loading={loading}
            empty={!loading && points.length === 0 ? '暂无数据' : null}
            ariaLabel="累计盈亏曲线"
          />
        </section>
      </div>

      <div className="chart-grid">
        <section className="chart-card">
          <div className="chart-card-head">
            <h2 className="chart-card-title">个股盈亏 Top</h2>
            <span className="chart-card-meta">{data?.top_codes.length ?? 0} 只</span>
          </div>
          <EChart
            option={useTopCodesOption(data?.top_codes ?? [])}
            height={320}
            loading={loading}
            empty={!loading && (data?.top_codes.length ?? 0) === 0 ? '暂无个股盈亏' : null}
            ariaLabel="个股盈亏排行"
          />
        </section>

        <section className="chart-card">
          <div className="chart-card-head">
            <h2 className="chart-card-title">超额收益（账户 − 上证）</h2>
            <span className="chart-card-meta">正=跑赢大盘</span>
          </div>
          <EChart
            option={useExcessOption(points)}
            height={320}
            loading={loading}
            empty={!loading && points.length === 0 ? '暂无数据' : null}
            ariaLabel="超额收益图"
          />
        </section>
      </div>

      <PointsTable points={points} view={view} />
    </div>
  );
}

function SummaryStrip({ data, loading }: { data: StockProfitDTO | null; loading: boolean }) {
  if (loading || !data) return null;
  const excess = data.total_rate - data.total_sz;
  return (
    <div className="summary-card">
      <div className="summary-grid">
        <div className="summary-item">
          <div className="summary-label">区间盈亏</div>
          <div className={`summary-value ${profitClass(data.total_pl)}`}>
            {formatMoney(data.total_pl)}
          </div>
        </div>
        <div className="summary-item">
          <div className="summary-label">账户收益率</div>
          <div className={`summary-value ${profitClass(data.total_rate)}`}>
            {tipRatio(data.total_rate)}
          </div>
        </div>
        <div className="summary-item">
          <div className="summary-label">同期上证</div>
          <div className={`summary-value ${profitClass(data.total_sz)}`}>
            {tipRatio(data.total_sz)}
          </div>
        </div>
        <div className="summary-item">
          <div className="summary-label">超额收益</div>
          <div className={`summary-value ${profitClass(excess)}`}>{tipRatio(excess)}</div>
        </div>
      </div>
    </div>
  );
}

/** 柱=盈亏，双轴折线=账户收益率/大盘涨跌 */
function usePLOption(points: StockProfitPoint[], view: ViewMode): EChartsOption {
  return useMemo<EChartsOption>(() => {
    const labels = points.map((p) => p.date);
    return {
      grid: { left: 8, right: 8, top: 36, bottom: points.length > 14 ? 44 : 8, containLabel: true },
      legend: {
        data: ['盈亏', '账户收益率', '上证'],
        top: 0,
        right: 0,
        icon: 'roundRect',
        itemWidth: 10,
        itemHeight: 10,
        textStyle: { color: palette.textSecondary, fontSize: 11 },
      },
      xAxis: categoryAxis(labels, { rotate: view === 'range' && points.length > 14 ? 45 : 0 }),
      yAxis: [
        valueAxis({ formatter: axisWan }),
        valueAxis({ formatter: axisRatioPct, position: 'right', splitLine: false }),
      ],
      dataZoom:
        points.length > 40
          ? [
              { type: 'inside', throttle: 60 },
              { type: 'slider', height: 16, bottom: 6, borderColor: palette.border },
            ]
          : undefined,
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        formatter: (params: unknown) => {
          const arr = params as { dataIndex: number }[];
          if (!arr?.length) return '';
          const p = points[arr[0].dataIndex];
          if (!p) return '';
          return [
            `<strong>${p.date}</strong>`,
            `${tipDot(plColor(p.profit_loss))}盈亏 ${tipMoney(p.profit_loss, true)}`,
            `${tipDot(palette.accent)}账户 ${tipRatio(p.rate)}`,
            `${tipDot(palette.neutral)}上证 ${tipRatio(p.sz_rate)}`,
          ].join('<br/>');
        },
      },
      series: [
        {
          name: '盈亏',
          type: 'bar',
          data: points.map((p) => ({
            value: p.profit_loss,
            itemStyle: { color: plColor(p.profit_loss) },
          })),
          barMaxWidth: 28,
          markLine: zeroMarkLine,
        },
        {
          name: '账户收益率',
          type: 'line',
          yAxisIndex: 1,
          data: points.map((p) => p.rate),
          smooth: true,
          symbol: 'circle',
          symbolSize: 5,
          lineStyle: { width: 2, color: palette.accent },
          itemStyle: { color: palette.accent },
        },
        {
          name: '上证',
          type: 'line',
          yAxisIndex: 1,
          data: points.map((p) => p.sz_rate),
          smooth: true,
          symbol: 'none',
          lineStyle: { width: 1.5, color: palette.neutral, type: 'dashed' },
          itemStyle: { color: palette.neutral },
        },
      ],
    };
  }, [points, view]);
}

/** 累计盈亏面积图 */
function useCumulativeOption(points: StockProfitPoint[]): EChartsOption {
  return useMemo<EChartsOption>(() => {
    let acc = 0;
    const cum = points.map((p) => {
      acc += p.profit_loss;
      return acc;
    });
    const last = cum.at(-1) ?? 0;
    const color = plColor(last);
    return {
      xAxis: categoryAxis(points.map((p) => p.date)),
      yAxis: valueAxis({ formatter: axisWan }),
      tooltip: {
        trigger: 'axis',
        formatter: (params: unknown) => {
          const arr = params as { dataIndex: number }[];
          if (!arr?.length) return '';
          const i = arr[0].dataIndex;
          return `<strong>${points[i].date}</strong><br/>${tipDot(color)}累计 ${tipMoney(cum[i], true)}`;
        },
      },
      series: [
        {
          type: 'line',
          data: cum,
          smooth: true,
          symbol: 'none',
          lineStyle: { width: 2, color },
          areaStyle: {
            color:
              last >= 0
                ? 'rgba(225, 29, 72, 0.12)'
                : 'rgba(5, 150, 105, 0.12)',
          },
          markLine: zeroMarkLine,
        },
      ],
    };
  }, [points]);
}

/**
 * 个股盈亏横向柱状。
 * 注意：stockprofit 的 top_codes[].type 是空串（与 calendar 不同），故这里不做类型区分。
 */
function useTopCodesOption(codes: CodePL[]): EChartsOption {
  return useMemo<EChartsOption>(() => {
    const sorted = [...codes].sort((a, b) => a.profit_loss - b.profit_loss);
    return {
      grid: { left: 8, right: 64, top: 8, bottom: 8, containLabel: true },
      xAxis: { ...valueAxis({ formatter: axisWan }), splitLine: { show: false } },
      yAxis: {
        ...categoryAxis(sorted.map((c) => c.name || c.code)),
        axisLabel: { color: palette.textSecondary, fontSize: 11 },
      },
      tooltip: {
        trigger: 'item',
        formatter: (params: unknown) => {
          const p = params as { dataIndex: number };
          const c = sorted[p.dataIndex];
          return `${c.name || c.code} <span style="opacity:.6">${c.code}</span><br/>${tipDot(plColor(c.profit_loss))}盈亏 ${tipMoney(c.profit_loss, true)}`;
        },
      },
      series: [
        {
          type: 'bar',
          data: sorted.map((c) => ({
            value: c.profit_loss,
            itemStyle: { color: plColor(c.profit_loss) },
          })),
          barMaxWidth: 14,
          label: {
            show: true,
            position: 'right',
            formatter: numLabel((n) => formatMoney(n, 0)),
            fontSize: 10,
            color: palette.textTertiary,
          },
        },
      ],
    };
  }, [codes]);
}

/** 超额收益 = 账户收益率 − 大盘涨跌 */
function useExcessOption(points: StockProfitPoint[]): EChartsOption {
  return useMemo<EChartsOption>(() => {
    const excess = points.map((p) => p.rate - p.sz_rate);
    return {
      grid: { left: 8, right: 8, top: 8, bottom: 8, containLabel: true },
      xAxis: categoryAxis(points.map((p) => p.date), {
        rotate: points.length > 14 ? 45 : 0,
      }),
      yAxis: valueAxis({ formatter: axisRatioPct }),
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        formatter: (params: unknown) => {
          const arr = params as { dataIndex: number }[];
          if (!arr?.length) return '';
          const i = arr[0].dataIndex;
          const p = points[i];
          return [
            `<strong>${p.date}</strong>`,
            `${tipDot(plColor(excess[i]))}超额 ${tipRatio(excess[i])}`,
            `${tipDot(palette.accent)}账户 ${tipRatio(p.rate)}`,
            `${tipDot(palette.neutral)}上证 ${tipRatio(p.sz_rate)}`,
          ].join('<br/>');
        },
      },
      series: [
        {
          type: 'bar',
          data: excess.map((v) => ({ value: v, itemStyle: { color: plColor(v) } })),
          barMaxWidth: 24,
          markLine: zeroMarkLine,
        },
      ],
    };
  }, [points]);
}

function PointsTable({ points, view }: { points: StockProfitPoint[]; view: ViewMode }) {
  if (points.length === 0) return null;
  return (
    <section className="account-section">
      <div className="account-head">
        <h2 className="account-name">
          明细
          <span className="chip muted">{view === 'monthly' ? '按月' : '按日'}</span>
          <span className="chip">{points.length} 条</span>
        </h2>
      </div>
      <div className="table-container">
        <table className="data-table">
          <thead>
            <tr>
              <th>{view === 'monthly' ? '月份' : '日期'}</th>
              <th className="num">盈亏</th>
              <th className="num">账户收益率</th>
              <th className="num">上证涨跌</th>
              <th className="num">超额</th>
            </tr>
          </thead>
          <tbody>
            {[...points].reverse().map((p) => {
              const excess = p.rate - p.sz_rate;
              return (
                <tr key={p.date}>
                  <td>{p.date}</td>
                  <td className={`num ${profitClass(p.profit_loss)}`}>
                    {formatMoney(p.profit_loss)}
                  </td>
                  <td className={`num ${profitClass(p.rate)}`}>{tipRatio(p.rate)}</td>
                  <td className={`num ${profitClass(p.sz_rate)}`}>{tipRatio(p.sz_rate)}</td>
                  <td className={`num ${profitClass(excess)}`}>{tipRatio(excess)}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </section>
  );
}
