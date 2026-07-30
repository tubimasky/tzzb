import { useEffect, useMemo, useState } from 'react';
import { api, apiErrorMessage } from '../api/client';
import EChart from '../components/EChart';
import type { EChartsOption } from '../charts/echarts';
import {
  axisRatioPct,
  axisWan,
  categoryAxis,
  categoryColors,
  numLabel,
  palette,
  plColor,
  tipDot,
  tipMoney,
  tipRatio,
  tipWan,
  valueAxis,
  zeroMarkLine,
} from '../charts/theme';
import type { AccountItem, AccountSummaryDTO, TrendDTO, TrendPoint } from '../types/api';
import { formatMoney, formatRatio, formatWan, profitClass } from '../utils/format';
import './AccountsPage.css';

function toneClass(n: number): string {
  if (!Number.isFinite(n) || n === 0) return 'tone-neutral';
  return n > 0 ? 'tone-up' : 'tone-down';
}

function IconWallet() {
  return (
    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <rect x="3" y="6" width="18" height="13" rx="2.5" stroke="currentColor" strokeWidth="1.8" />
      <path d="M3 10h18" stroke="currentColor" strokeWidth="1.8" />
      <circle cx="16.5" cy="14.5" r="1.2" fill="currentColor" />
    </svg>
  );
}

function IconTrend() {
  return (
    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M4 16l5-5 3.5 3.5L20 7"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function IconAccounts() {
  return (
    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <circle cx="9" cy="8" r="3" stroke="currentColor" strokeWidth="1.8" />
      <circle cx="17" cy="9" r="2.2" stroke="currentColor" strokeWidth="1.8" />
      <path d="M3.5 19c.5-3.4 2.3-5.2 5.5-5.2s5 1.8 5.5 5.2" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
      <path d="M15 14.3c2.9-.4 4.7 1 5.3 3.7" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
    </svg>
  );
}

function IconBolt() {
  return (
    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M13 3L5.5 13.5H11L10 21l8-11.5H13.5L13 3z"
        stroke="currentColor"
        strokeWidth="1.6"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function Skeleton() {
  return (
    <div className="skeleton-page" aria-busy="true" aria-label="加载中">
      <div className="skeleton-row">
        <div className="skeleton-block" />
        <div className="skeleton-block" />
        <div className="skeleton-block" />
        <div className="skeleton-block" />
      </div>
      <div className="skeleton-block tall" />
    </div>
  );
}

export default function AccountsPage() {
  const [data, setData] = useState<AccountSummaryDTO | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    api
      .getAccounts()
      .then((res) => {
        if (!cancelled) {
          setData(res.data);
          setLoading(false);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setError(apiErrorMessage(err));
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, []);

  if (loading) return <Skeleton />;
  if (error) return <div className="error">错误: {error}</div>;
  if (!data) return null;

  const { summary, accounts } = data;

  return (
    <div className="page accounts-page">
      <header className="page-header">
        <div className="page-header-text">
          <div className="page-eyebrow">Portfolio</div>
          <h1 className="page-title">
            账户<em>总览</em>
          </h1>
          <p className="page-subtitle">跨账户资产、收益与仓位一览，数据来自本地服务实时聚合</p>
        </div>
      </header>

      {data.complete === false && (
        <div className="data-warning" role="status">
          <strong>账户数据不完整</strong>
          <span>
            {(data.warnings ?? [])
              .map((warning) => `${warning.account} ${warning.component ?? ''}`)
              .join('；')}
          </span>
        </div>
      )}

      <section className="summary-cards" aria-label="汇总指标">
        <article className="card tone-neutral">
          <div className="card-top">
            <div className="card-label">总资产</div>
            <div className="card-icon">
              <IconWallet />
            </div>
          </div>
          <div className="card-value">
            {formatWan(summary.total_assets)}
            <span className="card-unit">万</span>
          </div>
          <div className="card-hint">含现金与持仓市值</div>
        </article>

        <article className={`card ${toneClass(summary.hold_profit)}`}>
          <div className="card-top">
            <div className="card-label">持仓盈亏</div>
            <div className="card-icon">
              <IconTrend />
            </div>
          </div>
          <div className={`card-value ${profitClass(summary.hold_profit)}`}>
            {formatWan(summary.hold_profit)}
            <span className="card-unit">万</span>
          </div>
          <div className="card-hint">当前未清仓持仓</div>
        </article>

        <article className="card tone-neutral">
          <div className="card-top">
            <div className="card-label">账户数量</div>
            <div className="card-icon">
              <IconAccounts />
            </div>
          </div>
          <div className="card-value">{accounts.length}</div>
          <div className="card-hint">已记录投资账户</div>
        </article>

        <article className={`card ${toneClass(summary.today_profit)}`}>
          <div className="card-top">
            <div className="card-label">今日盈亏</div>
            <div className="card-icon">
              <IconBolt />
            </div>
          </div>
          <div className={`card-value ${profitClass(summary.today_profit)}`}>
            {formatWan(summary.today_profit)}
            <span className="card-unit">万</span>
          </div>
          <div className="card-hint">当日浮动 + 已实现</div>
        </article>
      </section>

      <TrendChart />

      <div className="chart-grid">
        <AllocationChart accounts={accounts} />
        <AccountProfitChart accounts={accounts} />
      </div>

      <section className="accounts-table-wrapper" aria-label="账户明细">
        <div className="panel-header">
          <h2>账户明细</h2>
          <span className="panel-meta">{accounts.length} 个账户</span>
        </div>
        <div style={{ overflowX: 'auto' }}>
          <table className="accounts-table">
            <thead>
              <tr>
                <th>账户名称</th>
                <th>类别</th>
                <th className="num">总资产（万）</th>
                <th className="num">持仓盈亏（万）</th>
                <th className="num">今日盈亏（万）</th>
                <th className="num">仓位率</th>
              </tr>
            </thead>
            <tbody>
              {accounts.map((acc) => {
                const posPct = Math.min(100, Math.max(0, acc.position_rate * 100));
                return (
                  <tr key={`${acc.category}-${acc.name}`}>
                    <td className="acc-name">{acc.name}</td>
                    <td>
                      <span className="acc-cat">{acc.category}</span>
                    </td>
                    <td className="num">{formatWan(acc.total_assets)}</td>
                    <td className={`num ${profitClass(acc.hold_profit)}`}>
                      {formatWan(acc.hold_profit)}
                    </td>
                    <td className={`num ${profitClass(acc.today_profit)}`}>
                      {formatWan(acc.today_profit)}
                    </td>
                    <td className="num">
                      <div className="rate-cell">
                        <span>{formatRatio(acc.position_rate)}</span>
                        <div className="rate-bar" title={`仓位 ${formatRatio(acc.position_rate)}`}>
                          <span style={{ width: `${posPct}%` }} />
                        </div>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}

/**
 * 资产趋势折线：单独加载（该接口逐账户 fan-out，比其它接口慢数秒）。
 *
 * 两种视图刻意分开，因为量纲不能混：
 * - 资产：总资产金额（万元），含出入金，看的是账户体量。
 * - 收益率对比：剔除出入金的复利累计收益 vs 上证同期累计涨跌，同一百分比轴，
 *   两线的落差就是超额收益。转账不会被误算成收益，这才是能和指数比的口径。
 */
const TREND_RANGE_OPTIONS = [
  { days: 30, label: '30 天' },
  { days: 60, label: '60 天' },
  { days: 90, label: '90 天' },
  { days: 365, label: '今年' },
] as const;

function TrendChart() {
  const [days, setDays] = useState(30);
  const [mode, setMode] = useState<'asset' | 'return'>('asset');
  const [trend, setTrend] = useState<TrendDTO | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const wantBenchmark = mode === 'return';

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    api
      .getTrend({ days, benchmark: wantBenchmark })
      .then((res) => {
        if (!cancelled) {
          setTrend(res.data);
          setLoading(false);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setError(apiErrorMessage(err));
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [days, wantBenchmark]);

  const points = trend?.points;

  const option = useMemo<EChartsOption>(() => {
    const list = points ?? [];
    const dates = list.map((p) => p.date.slice(5)); // MM-DD

    if (mode === 'return') {
      return returnOption(list, dates);
    }
    return assetOption(list, dates);
  }, [points, mode]);

  const stats = trend?.stats;

  return (
    <section className="chart-card span-2">
      <div className="chart-card-head">
        <h2 className="chart-card-title">资产趋势</h2>
        <div className="trend-controls">
          <div className="segmented tiny" role="tablist" aria-label="时间范围">
            {TREND_RANGE_OPTIONS.map((option) => (
              <button
                key={option.days}
                type="button"
                role="tab"
                aria-selected={days === option.days}
                className={days === option.days ? 'active' : undefined}
                onClick={() => setDays(option.days)}
              >
                {option.label}
              </button>
            ))}
          </div>
          <div className="segmented tiny" role="tablist" aria-label="视图">
            {(
              [
                ['asset', '资产'],
                ['return', '对比上证'],
              ] as const
            ).map(([key, label]) => (
              <button
                key={key}
                type="button"
                role="tab"
                aria-selected={mode === key}
                className={mode === key ? 'active' : undefined}
                onClick={() => setMode(key)}
              >
                {label}
              </button>
            ))}
          </div>
        </div>
      </div>

      {error ? (
        <div className="chart-inline-error">趋势加载失败: {error}</div>
      ) : (
        <>
          {!loading && trend?.complete === false && (
            <div className="chart-inline-warning" role="status">
              趋势数据不完整：
              {(trend.warnings ?? [])
                .map((warning) => `${warning.account} ${warning.source ?? ''}`)
                .join('；')}
            </div>
          )}
          <EChart
            option={option}
            height={300}
            loading={loading}
            empty={!loading && (points?.length ?? 0) === 0 ? '暂无趋势数据' : null}
            ariaLabel={mode === 'return' ? '累计收益率与上证对比曲线' : '资产趋势曲线'}
          />
          {stats?.last_date && (
            <div className="trend-stats">
              <span>
                {stats.first_date} → {stats.last_date}
              </span>
              {mode === 'return' ? (
                <>
                  <span>
                    累计收益{' '}
                    <strong className={profitClass(stats.cum_return)}>
                      {tipRatio(stats.cum_return)}
                    </strong>
                  </span>
                  {stats.has_benchmark && (
                    <>
                      <span>
                        上证同期{' '}
                        <strong className={profitClass(stats.benchmark_cum)}>
                          {tipRatio(stats.benchmark_cum)}
                        </strong>
                      </span>
                      <span>
                        超额{' '}
                        <strong className={profitClass(stats.excess)}>
                          {tipRatio(stats.excess)}
                        </strong>
                      </span>
                    </>
                  )}
                  {stats.has_return_drawdown && (
                    <span>
                      最大回撤{' '}
                      <strong className="profit-negative">
                        {tipRatio(-stats.return_drawdown, false)}
                      </strong>
                    </span>
                  )}
                </>
              ) : (
                <>
                  <span>
                    资产变化{' '}
                    <strong className={profitClass(stats.asset_change)}>
                      {formatMoney(stats.asset_change)}
                    </strong>
                  </span>
                  <span>
                    盈亏变化{' '}
                    <strong className={profitClass(stats.profit_change)}>
                      {formatMoney(stats.profit_change)}
                    </strong>
                  </span>
                  {stats.has_cum_return && (
                    <span>
                      累计收益{' '}
                      <strong className={profitClass(stats.cum_return)}>
                        {tipRatio(stats.cum_return)}
                      </strong>
                    </span>
                  )}
                </>
              )}
            </div>
          )}
        </>
      )}
    </section>
  );
}

/** 资产金额视图：单轴万元面积图 */
function assetOption(list: TrendPoint[], dates: string[]): EChartsOption {
  return {
    grid: { left: 8, right: 16, top: 20, bottom: 4, containLabel: true },
    tooltip: {
      trigger: 'axis',
      formatter: (params: unknown) => {
        const arr = params as { dataIndex: number }[];
        if (!arr.length) return '';
        const p = list[arr[0].dataIndex];
        if (!p) return '';
        const rows = [
          `<strong>${p.date}</strong>`,
          `${tipDot(palette.accent)}总资产 ${tipWan(p.total_asset)}`,
          `${tipDot(plColor(p.asset_change))}日变化 ${tipMoney(p.asset_change, true)}`,
          `${tipDot(palette.neutral)}仓位 ${formatRatio(p.position_rate)}`,
        ];
        // 出入金会让"日变化"看起来像盈亏，标出来免得误读
        const netIn = p.fund_in - p.fund_out;
        if (netIn !== 0) {
          rows.push(`${tipDot(palette.textTertiary)}银证转账 ${tipMoney(netIn, true)}`);
        }
        return rows.join('<br/>');
      },
    },
    xAxis: categoryAxis(dates),
    yAxis: [valueAxis({ formatter: axisWan })],
    series: [
      {
        name: '总资产',
        type: 'line',
        data: list.map((p) => p.total_asset),
        smooth: true,
        showSymbol: false,
        lineStyle: { color: palette.accent, width: 2 },
        itemStyle: { color: palette.accent },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(37, 99, 235, 0.22)' },
              { offset: 1, color: 'rgba(37, 99, 235, 0.02)' },
            ],
          },
        },
      },
    ],
  };
}

/** 收益率对比视图：我的累计收益 vs 上证累计，同一百分比轴 */
function returnOption(list: TrendPoint[], dates: string[]): EChartsOption {
  // 没有基准数据的日期留空（null），让 ECharts 断线而不是把线拉回 0
  const bench = list.map((p) => (p.has_benchmark ? p.benchmark_cum : null));
  const hasBench = bench.some((v) => v !== null);

  return {
    legend: {
      data: hasBench ? ['我的收益', '上证'] : ['我的收益'],
      top: 0,
      right: 0,
      icon: 'roundRect',
      itemWidth: 10,
      itemHeight: 10,
      textStyle: { fontSize: 11, color: palette.textSecondary },
    },
    grid: { left: 8, right: 16, top: 30, bottom: 4, containLabel: true },
    tooltip: {
      trigger: 'axis',
      formatter: (params: unknown) => {
        const arr = params as { dataIndex: number }[];
        if (!arr.length) return '';
        const p = list[arr[0].dataIndex];
        if (!p) return '';
        const rows = [
          `<strong>${p.date}</strong>`,
          `${tipDot(plColor(p.cum_return))}我的累计 ${tipRatio(p.cum_return)}`,
        ];
        if (p.has_benchmark) {
          rows.push(`${tipDot(palette.neutral)}上证累计 ${tipRatio(p.benchmark_cum)}`);
          rows.push(
            `${tipDot(plColor(p.cum_return - p.benchmark_cum))}超额 ${tipRatio(
              p.cum_return - p.benchmark_cum,
            )}`,
          );
        }
        if (p.has_return) {
          rows.push(`${tipDot(palette.textTertiary)}当日 ${tipRatio(p.daily_return)}`);
        }
        rows.push(`${tipDot(palette.accent)}总资产 ${tipWan(p.total_asset)}`);
        return rows.join('<br/>');
      },
    },
    xAxis: categoryAxis(dates),
    yAxis: [valueAxis({ formatter: axisRatioPct })],
    series: [
      {
        name: '我的收益',
        type: 'line',
        data: list.map((p) => p.cum_return),
        smooth: true,
        showSymbol: false,
        lineStyle: { color: palette.accent, width: 2 },
        itemStyle: { color: palette.accent },
        markLine: zeroMarkLine,
      },
      {
        name: '上证',
        type: 'line',
        data: bench,
        connectNulls: true,
        smooth: true,
        showSymbol: false,
        lineStyle: { color: palette.neutral, width: 1.5, type: 'dashed' },
        itemStyle: { color: palette.neutral },
      },
    ],
  };
}

/** 账户资产占比环形图 */
function AllocationChart({ accounts }: { accounts: AccountItem[] }) {
  const option = useMemo<EChartsOption>(() => {
    const data = accounts
      .filter((a) => a.total_assets > 0)
      .map((a, i) => ({
        name: a.name,
        value: a.total_assets,
        itemStyle: { color: categoryColors[i % categoryColors.length] },
      }));

    return {
      tooltip: {
        trigger: 'item',
        formatter: (params: unknown) => {
          const p = params as { name: string; value: number; percent: number };
          return `${p.name}<br/>${tipWan(p.value)}（${p.percent.toFixed(1)}%）`;
        },
      },
      legend: {
        type: 'scroll',
        orient: 'vertical',
        right: 0,
        top: 'middle',
        icon: 'circle',
        itemWidth: 8,
        itemHeight: 8,
        textStyle: { fontSize: 11, color: palette.textSecondary },
      },
      series: [
        {
          type: 'pie',
          radius: ['46%', '70%'],
          center: ['34%', '50%'],
          itemStyle: { borderColor: '#fff', borderWidth: 2 },
          label: { show: false },
          emphasis: { scaleSize: 6, label: { show: false } },
          data,
        },
      ],
    };
  }, [accounts]);

  return (
    <section className="chart-card">
      <div className="chart-card-head">
        <h2 className="chart-card-title">资产分布</h2>
        <span className="chart-card-meta">{accounts.length} 个账户</span>
      </div>
      <EChart
        option={option}
        height={260}
        empty={accounts.length === 0 ? '暂无账户' : null}
        ariaLabel="账户资产占比环形图"
      />
    </section>
  );
}

/** 各账户持仓盈亏双向柱状 */
function AccountProfitChart({ accounts }: { accounts: AccountItem[] }) {
  const option = useMemo<EChartsOption>(() => {
    // 升序让盈利在右/上方，读起来和表格一致
    const sorted = [...accounts].sort((a, b) => a.hold_profit - b.hold_profit);
    const names = sorted.map((a) => a.name);
    const values = sorted.map((a) => a.hold_profit);

    return {
      grid: { left: 8, right: 56, top: 8, bottom: 8, containLabel: true },
      tooltip: {
        trigger: 'item',
        formatter: (params: unknown) => {
          const p = params as { dataIndex: number };
          const a = sorted[p.dataIndex];
          return [
            `${a.name} <span style="opacity:.6">${a.category}</span>`,
            `${tipDot(plColor(a.hold_profit))}持仓盈亏 ${tipMoney(a.hold_profit, true)}`,
            `${tipDot(palette.accent)}总资产 ${tipWan(a.total_assets)}`,
          ].join('<br/>');
        },
      },
      xAxis: { ...valueAxis({ formatter: axisWan }), splitLine: { show: false } },
      yAxis: {
        ...categoryAxis(names),
        axisLabel: { color: palette.textSecondary, fontSize: 11 },
      },
      series: [
        {
          type: 'bar',
          data: values.map((v) => ({ value: v, itemStyle: { color: plColor(v) } })),
          barMaxWidth: 18,
          markLine: { ...zeroMarkLine, data: [{ xAxis: 0 }] },
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
  }, [accounts]);

  return (
    <section className="chart-card">
      <div className="chart-card-head">
        <h2 className="chart-card-title">各账户持仓盈亏</h2>
        <span className="chart-card-meta">红盈绿亏</span>
      </div>
      <EChart
        option={option}
        height={260}
        empty={accounts.length === 0 ? '暂无账户' : null}
        ariaLabel="各账户持仓盈亏柱状图"
      />
    </section>
  );
}
