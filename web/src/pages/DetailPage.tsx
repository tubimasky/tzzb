import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { api, apiErrorMessage } from '../api/client';
import EChart from '../components/EChart';
import type { EChartsOption } from '../charts/echarts';
import {
  axisWan,
  categoryAxis,
  palette,
  plColor,
  tipDot,
  tipMoney,
  tipRatio,
  valueAxis,
  zeroMarkLine,
} from '../charts/theme';
import type { DetailDTO, HoldRecord, StockDetail, StockTrade } from '../types/api';
import {
  formatDateYMD,
  formatMoney,
  formatRatio,
  profitClass,
  tradeOpLabel,
} from '../utils/format';
import './DetailPage.css';

type TabKey = 'overview' | 'records' | 'trades' | 'clear';

const TABS: { key: TabKey; label: string }[] = [
  { key: 'overview', label: '持仓概览' },
  { key: 'records', label: '建仓记录' },
  { key: 'trades', label: '历史成交' },
  { key: 'clear', label: '清仓汇总' },
];

function Skeleton() {
  return (
    <div className="skeleton-page" aria-busy="true" aria-label="加载中">
      <div className="skeleton-block" style={{ height: 72 }} />
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

export default function DetailPage() {
  const { code = '' } = useParams<{ code: string }>();
  const [searchParams] = useSearchParams();
  const accountParam = searchParams.get('account') || '';
  const navigate = useNavigate();

  const [data, setData] = useState<DetailDTO | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [accountIdx, setAccountIdx] = useState(0);
  const [tab, setTab] = useState<TabKey>('overview');

  useEffect(() => {
    if (!code) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    setAccountIdx(0);
    setTab('overview');

    api
      .getDetail(code, { account: accountParam || undefined })
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
  }, [code, accountParam]);

  const details = data?.details ?? [];
  const current: StockDetail | null = details[accountIdx] ?? null;

  const title = useMemo(() => {
    const m = current?.merge;
    if (m?.stock_name) return { name: m.stock_name, code: m.stock_code || code };
    return { name: code, code };
  }, [current, code]);

  if (!code) {
    return <div className="error">缺少股票代码</div>;
  }
  if (loading) return <Skeleton />;
  if (error) {
    return (
      <div className="page detail-page">
        <BackBar />
        <div className="error">错误: {error}</div>
      </div>
    );
  }
  if (!data || details.length === 0) {
    return (
      <div className="page detail-page">
        <BackBar />
        <div className="empty-state">
          未找到 {code} 的持仓明细（基金账户不参与钻取；清仓股需指定日期区间）
        </div>
      </div>
    );
  }

  const merge = current?.merge;
  const cleared = merge?.clear_flag === '1';

  return (
    <div className="page detail-page">
      <BackBar onBack={() => navigate(-1)} />

      <header className="page-header detail-header">
        <div className="page-header-text">
          <div className="page-eyebrow">Stock Detail</div>
          <h1 className="page-title">
            {title.name} <em className="detail-code">{title.code}</em>
          </h1>
          <p className="page-subtitle">
            合并持仓 · 建仓记录 · 历史成交 · 清仓汇总
            {cleared && <span className="chip warn">已清仓</span>}
          </p>
        </div>
      </header>

      {details.length > 1 && (
        <div className="account-switch" role="tablist" aria-label="账户切换">
          {details.map((d, i) => (
            <button
              key={`${d.account_name}-${i}`}
              type="button"
              role="tab"
              aria-selected={accountIdx === i}
              className={accountIdx === i ? 'active' : undefined}
              onClick={() => {
                setAccountIdx(i);
                setTab('overview');
              }}
            >
              {d.account_name || `账户 ${i + 1}`}
            </button>
          ))}
        </div>
      )}

      {details.length === 1 && current && (
        <div className="single-account-chip">
          <span className="chip muted">{current.account_name}</span>
        </div>
      )}

      <div className="detail-tabs segmented" role="tablist" aria-label="明细分块">
        {TABS.map((t) => {
          const count =
            t.key === 'records'
              ? current?.records.length ?? 0
              : t.key === 'trades'
                ? current?.trades.length ?? 0
                : null;
          return (
            <button
              key={t.key}
              type="button"
              role="tab"
              aria-selected={tab === t.key}
              className={tab === t.key ? 'active' : undefined}
              onClick={() => setTab(t.key)}
            >
              {t.label}
              {count != null && count > 0 ? <span className="tab-count">{count}</span> : null}
            </button>
          );
        })}
      </div>

      {tab === 'overview' && <OverviewPanel detail={current} />}
      {tab === 'records' && <RecordsPanel detail={current} />}
      {tab === 'trades' && <TradesPanel detail={current} />}
      {tab === 'clear' && <ClearPanel detail={current} />}
    </div>
  );
}

function BackBar({ onBack }: { onBack?: () => void }) {
  return (
    <div className="back-bar">
      {onBack ? (
        <button type="button" className="back-link" onClick={onBack}>
          ← 返回
        </button>
      ) : (
        <Link to="/positions" className="back-link">
          ← 返回持仓
        </Link>
      )}
      <Link to="/positions" className="back-link secondary">
        持仓列表
      </Link>
    </div>
  );
}

function OverviewPanel({ detail }: { detail: StockDetail | null }) {
  const m = detail?.merge;
  if (!m) {
    return <div className="empty-state">暂无合并持仓数据</div>;
  }

  const metrics = [
    { label: '持有天数', value: `${m.hold_days} 天`, cls: '' },
    { label: '持仓数量', value: formatCount(m.count), cls: '' },
    { label: '买入均价', value: m.price_buy_av.toLocaleString('zh-CN', { minimumFractionDigits: 2 }), cls: '' },
    { label: '现价', value: m.price_curr.toLocaleString('zh-CN', { minimumFractionDigits: 2 }), cls: '' },
    { label: '市值', value: formatMoney(m.market_value), cls: '' },
    { label: '总成本', value: formatMoney(m.zcb), cls: '' },
    {
      label: '浮动盈亏',
      value: formatMoney(m.yke),
      cls: profitClass(m.yke),
    },
    {
      label: '收益率',
      value: formatRatio(m.profit_rate),
      cls: profitClass(m.profit_rate),
    },
    { label: '费用', value: formatMoney(m.fee), cls: '' },
  ];

  return (
    <section className="detail-panel">
      <div className="metric-grid">
        {metrics.map((item) => (
          <div key={item.label} className="metric-card">
            <div className="metric-label">{item.label}</div>
            <div className={`metric-value ${item.cls}`}>{item.value}</div>
          </div>
        ))}
      </div>
      {m.clear_flag === '1' && (
        <div className="inline-warn">该账户下此持仓标记为已清仓</div>
      )}
    </section>
  );
}

function RecordsPanel({ detail }: { detail: StockDetail | null }) {
  const records = detail?.records ?? [];
  if (records.length === 0) {
    return <div className="empty-state">暂无建仓记录</div>;
  }
  return (
    <section className="detail-panel">
      <RecordsChart records={records} />
      <div className="table-container">
        <table className="data-table">
          <thead>
            <tr>
              <th>建仓日</th>
              <th>结束日</th>
              <th className="num">分段收益</th>
              <th className="num">收益率</th>
            </tr>
          </thead>
          <tbody>
            {records.map((r, i) => (
              <tr key={`${r.start_date}-${i}`}>
                <td>{formatDateYMD(r.start_date)}</td>
                <td>{r.end_date ? formatDateYMD(r.end_date) : '持有中'}</td>
                <td className={`num ${profitClass(r.profit)}`}>{formatMoney(r.profit)}</td>
                <td className={`num ${profitClass(r.profit_rate)}`}>
                  {formatRatio(r.profit_rate)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function TradesPanel({ detail }: { detail: StockDetail | null }) {
  const trades = detail?.trades;
  // hooks 必须在 early return 之前；依赖 detail.trades 引用，避免 ?? [] 每渲染新建数组
  const sorted = useMemo(() => {
    const list = trades ?? [];
    return [...list].sort((a, b) => {
      const da = `${a.trans_date}${a.trans_time || ''}`;
      const db = `${b.trans_date}${b.trans_time || ''}`;
      return db.localeCompare(da);
    });
  }, [trades]);

  if (!trades || trades.length === 0) {
    return <div className="empty-state">暂无历史成交</div>;
  }

  return (
    <section className="detail-panel">
      <TradesChart trades={sorted} />
      <div className="table-container">
        <table className="data-table">
          <thead>
            <tr>
              <th>成交日</th>
              <th>方向</th>
              <th className="num">价格</th>
              <th className="num">数量</th>
              <th className="num">金额</th>
              <th className="num">费用</th>
              <th className="num">盈亏</th>
            </tr>
          </thead>
          <tbody>
            {sorted.map((t, i) => (
              <tr key={`${t.trans_date}-${t.trans_time}-${i}`}>
                <td>
                  <span className="trade-date">{formatDateYMD(t.trans_date)}</span>
                  {t.trans_time ? <span className="trade-time">{t.trans_time}</span> : null}
                </td>
                <td>
                  <span className={`op-pill op-${t.op === '1' ? 'buy' : t.op === '2' ? 'sell' : 'other'}`}>
                    {tradeOpLabel(t.op)}
                  </span>
                </td>
                <td className="num">{t.price.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}</td>
                <td className="num">{formatCount(t.count)}</td>
                <td className="num">{formatMoney(t.amount)}</td>
                <td className="num">{formatMoney(t.fee)}</td>
                <td className={`num ${profitClass(t.profit)}`}>
                  {formatMoney(t.profit)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function ClearPanel({ detail }: { detail: StockDetail | null }) {
  const c = detail?.clear;
  if (!c || !c.has_data) {
    return <div className="empty-state">暂无清仓汇总（未清仓或区间内无数据）</div>;
  }

  return (
    <section className="detail-panel">
      <div className="metric-grid">
        <div className="metric-card">
          <div className="metric-label">区间</div>
          <div className="metric-value metric-value-sm">
            {formatDateYMD(c.start_date)} → {formatDateYMD(c.end_date)}
          </div>
        </div>
        <div className="metric-card">
          <div className="metric-label">持有天数</div>
          <div className="metric-value">{c.hold_days} 天</div>
        </div>
        <div className="metric-card">
          <div className="metric-label">总买入</div>
          <div className="metric-value">{formatMoney(c.total_buy)}</div>
        </div>
        <div className="metric-card">
          <div className="metric-label">总卖出</div>
          <div className="metric-value">{formatMoney(c.total_sell)}</div>
        </div>
        <div className="metric-card">
          <div className="metric-label">费用</div>
          <div className="metric-value">{formatMoney(c.fee)}</div>
        </div>
        <div className="metric-card">
          <div className="metric-label">清仓盈亏</div>
          <div className={`metric-value ${profitClass(c.total_profit)}`}>
            {formatMoney(c.total_profit)}
          </div>
        </div>
        <div className="metric-card">
          <div className="metric-label">清仓收益率</div>
          <div className={`metric-value ${profitClass(c.total_profit_rate)}`}>
            {formatRatio(c.total_profit_rate)}
          </div>
        </div>
      </div>
    </section>
  );
}

function formatCount(n: number): string {
  if (!Number.isFinite(n)) return '—';
  if (Math.abs(n - Math.round(n)) < 1e-6) return Math.round(n).toLocaleString('zh-CN');
  return n.toLocaleString('zh-CN', { maximumFractionDigits: 2 });
}

/** 建仓记录分段收益柱状：每段持仓的盈亏，红盈绿亏 */
function RecordsChart({ records }: { records: HoldRecord[] }) {
  const option = useMemo<EChartsOption>(() => {
    // 建仓时间升序，读起来是一条时间线
    const sorted = [...records].sort((a, b) => a.start_date.localeCompare(b.start_date));
    const labels = sorted.map((r) => formatDateYMD(r.start_date));

    return {
      grid: { left: 8, right: 16, top: 24, bottom: 8, containLabel: true },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        formatter: (params: unknown) => {
          const arr = params as { dataIndex: number }[];
          if (!arr?.length) return '';
          const r = sorted[arr[0].dataIndex];
          const end = r.end_date ? formatDateYMD(r.end_date) : '持有中';
          return [
            `${formatDateYMD(r.start_date)} → ${end}`,
            `${tipDot(plColor(r.profit))}分段收益 ${tipMoney(r.profit, true)}`,
            `${tipDot(plColor(r.profit_rate))}收益率 ${tipRatio(r.profit_rate)}`,
          ].join('<br/>');
        },
      },
      xAxis: categoryAxis(labels, { rotate: labels.length > 6 ? 30 : 0 }),
      yAxis: valueAxis({ formatter: axisWan }),
      series: [
        {
          name: '分段收益',
          type: 'bar',
          data: sorted.map((r) => ({ value: r.profit, itemStyle: { color: plColor(r.profit) } })),
          barMaxWidth: 28,
          markLine: zeroMarkLine,
        },
      ],
    };
  }, [records]);

  return (
    <section className="chart-card detail-chart">
      <div className="chart-card-head">
        <h2 className="chart-card-title">分段收益</h2>
        <span className="chart-card-meta">{records.length} 段建仓</span>
      </div>
      <EChart option={option} height={240} ariaLabel="建仓分段收益柱状图" />
    </section>
  );
}

/**
 * 历史成交买卖点散点：x=成交日，y=成交价，点大小随金额，红买绿卖。
 * 直观看出加仓/减仓的价格位置。
 */
function TradesChart({ trades }: { trades: StockTrade[] }) {
  const option = useMemo<EChartsOption>(() => {
    // 时间升序（表格是倒序的，图上要正序）
    const sorted = [...trades].sort((a, b) =>
      `${a.trans_date}${a.trans_time || ''}`.localeCompare(`${b.trans_date}${b.trans_time || ''}`),
    );

    const amounts = sorted.map((t) => Math.abs(t.amount));
    const maxAmount = Math.max(...amounts, 1);
    // 金额映射到 8~26px，避免大单把小单压成看不见的点
    const sizeOf = (amount: number) => 8 + (Math.abs(amount) / maxAmount) * 18;

    const buys: [string, number, number][] = [];
    const sells: [string, number, number][] = [];
    for (const t of sorted) {
      const point: [string, number, number] = [formatDateYMD(t.trans_date), t.price, t.amount];
      if (t.op === '2') sells.push(point);
      else buys.push(point);
    }

    const dates = Array.from(new Set(sorted.map((t) => formatDateYMD(t.trans_date))));

    return {
      grid: { left: 8, right: 16, top: 32, bottom: 8, containLabel: true },
      legend: {
        data: ['买入', '卖出'],
        top: 0,
        right: 0,
        itemWidth: 8,
        itemHeight: 8,
        textStyle: { color: palette.textSecondary, fontSize: 11 },
      },
      tooltip: {
        trigger: 'item',
        formatter: (params: unknown) => {
          const p = params as { seriesName: string; value: [string, number, number] };
          const [date, price, amount] = p.value;
          const color = p.seriesName === '买入' ? palette.up : palette.down;
          return [
            date,
            `${tipDot(color)}${p.seriesName} @ ${formatMoney(price)}`,
            `金额 ${formatMoney(amount)}`,
          ].join('<br/>');
        },
      },
      xAxis: categoryAxis(dates, { rotate: dates.length > 8 ? 30 : 0 }),
      yAxis: valueAxis({ name: '成交价', formatter: (n) => formatMoney(n, 2) }),
      series: [
        {
          name: '买入',
          type: 'scatter',
          data: buys,
          symbolSize: (val: unknown) => sizeOf((val as [string, number, number])[2]),
          itemStyle: { color: palette.up, opacity: 0.75 },
        },
        {
          name: '卖出',
          type: 'scatter',
          data: sells,
          symbolSize: (val: unknown) => sizeOf((val as [string, number, number])[2]),
          itemStyle: { color: palette.down, opacity: 0.75 },
        },
      ],
    };
  }, [trades]);

  return (
    <section className="chart-card detail-chart">
      <div className="chart-card-head">
        <h2 className="chart-card-title">买卖点分布</h2>
        <span className="chart-card-meta">点大小 = 成交金额</span>
      </div>
      <EChart option={option} height={260} ariaLabel="历史成交买卖点散点图" />
    </section>
  );
}
