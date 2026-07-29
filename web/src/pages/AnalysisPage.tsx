import { useEffect, useMemo, useState } from 'react';
import { api, apiErrorMessage } from '../api/client';
import EChart from '../components/EChart';
import type { EChartsOption } from '../charts/echarts';
import {
  axisWan,
  categoryAxis,
  categoryColors,
  numLabel,
  palette,
  tipDot,
  tipMoney,
  tipWan,
  valueAxis,
} from '../charts/theme';
import type { AccountPosRate, AnalysisDistDTO, AnalysisPosRateDTO, DistItem } from '../types/api';
import { formatMoney, formatPct, formatWan } from '../utils/format';
import './AnalysisPage.css';

/** 市场码 → 交易所标签 */
function marketLabel(m: string): string {
  if (m === '1') return '深';
  if (m === '2') return '沪';
  return '—';
}

function Skeleton() {
  return (
    <div className="skeleton-page" aria-busy="true" aria-label="加载中">
      <div className="skeleton-block" style={{ height: 72 }} />
      <div className="skeleton-block tall" />
    </div>
  );
}

export default function AnalysisPage() {
  const [dist, setDist] = useState<AnalysisDistDTO | null>(null);
  const [posRate, setPosRate] = useState<AnalysisPosRateDTO | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [account, setAccount] = useState('');
  const [accountInput, setAccountInput] = useState('');

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    Promise.all([api.getAnalysisDistribution(account), api.getAnalysisPosRate(account)])
      .then(([d, p]) => {
        if (cancelled) return;
        setDist(d.data);
        setPosRate(p.data);
        setLoading(false);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(apiErrorMessage(err));
        setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [account]);

  const items = dist?.items ?? [];
  const accounts = posRate?.accounts ?? [];

  // 汇总：总市值 / 总现金 / 整体仓位率
  // 依赖 posRate?.accounts 引用而非 accounts，后者每次渲染都是新数组（?? []）
  const totals = useMemo(() => {
    const rows = posRate?.accounts ?? [];
    let stockValue = 0;
    let cash = 0;
    for (const a of rows) {
      stockValue += a.stock_value;
      cash += a.money_remain;
    }
    const total = stockValue + cash;
    return {
      stockValue,
      cash,
      total,
      rate: total > 0 ? (stockValue / total) * 100 : 0,
    };
  }, [posRate?.accounts]);

  function submitAccount(e: React.FormEvent) {
    e.preventDefault();
    setAccount(accountInput.trim());
  }

  if (loading && !dist) return <Skeleton />;

  return (
    <div className="page analysis-page">
      <header className="page-header">
        <div className="page-header-text">
          <div className="page-eyebrow">Analysis</div>
          <h1 className="page-title">
            仓位<em>分析</em>
          </h1>
          <p className="page-subtitle">持仓市值分布与各股票账户仓位率（仅统计股票侧账户）</p>
        </div>
      </header>

      <form className="filters analysis-filters" onSubmit={submitAccount}>
        <label className="filter-field">
          <span className="field-label">账户</span>
          <input
            type="search"
            placeholder="账户名模糊匹配，留空为全部"
            value={accountInput}
            onChange={(e) => setAccountInput(e.target.value)}
            autoComplete="off"
          />
        </label>
        <div className="filter-block">
          <span className="field-label">&nbsp;</span>
          <button type="submit" className="apply-btn">
            应用
          </button>
        </div>
        {account && (
          <div className="filter-block">
            <span className="field-label">&nbsp;</span>
            <button
              type="button"
              className="apply-btn ghost"
              onClick={() => {
                setAccountInput('');
                setAccount('');
              }}
            >
              清除
            </button>
          </div>
        )}
      </form>

      {error && <div className="error inline-error">加载失败: {error}</div>}

      <section className="analysis-stats" aria-label="仓位汇总">
        <div className="analysis-stat">
          <div className="analysis-stat-label">持仓市值</div>
          <div className="analysis-stat-value">
            {formatWan(totals.stockValue)}
            <span className="analysis-stat-unit">万</span>
          </div>
        </div>
        <div className="analysis-stat">
          <div className="analysis-stat-label">可用现金</div>
          <div className="analysis-stat-value">
            {formatWan(totals.cash)}
            <span className="analysis-stat-unit">万</span>
          </div>
        </div>
        <div className="analysis-stat">
          <div className="analysis-stat-label">账户合计</div>
          <div className="analysis-stat-value">
            {formatWan(totals.total)}
            <span className="analysis-stat-unit">万</span>
          </div>
        </div>
        <div className="analysis-stat">
          <div className="analysis-stat-label">整体仓位率</div>
          <div className="analysis-stat-value">{formatPct(totals.rate)}</div>
          <div className="analysis-stat-bar" title={`仓位 ${formatPct(totals.rate)}`}>
            <span style={{ width: `${Math.min(100, Math.max(0, totals.rate))}%` }} />
          </div>
        </div>
      </section>

      <div className="chart-grid">
        <DistributionChart items={items} total={dist?.total ?? 0} loading={loading} />
        <PosRateChart accounts={accounts} loading={loading} />
      </div>

      <DistributionTable items={items} total={dist?.total ?? 0} />
      <PosRateTable accounts={accounts} />
    </div>
  );
}

/** 持仓分布环形图。weight 后端已是百分数。 */
function DistributionChart({
  items,
  total,
  loading,
}: {
  items: DistItem[];
  total: number;
  loading: boolean;
}) {
  const option = useMemo<EChartsOption>(() => {
    // 标的多时尾部会挤成一团，Top 10 之外归为「其他」
    const TOP = 10;
    const sorted = [...items].sort((a, b) => b.value - a.value);
    const head = sorted.slice(0, TOP);
    const tail = sorted.slice(TOP);
    const data = head.map((it, i) => ({
      name: it.name || it.code,
      value: it.value,
      itemStyle: { color: categoryColors[i % categoryColors.length] },
    }));
    if (tail.length > 0) {
      data.push({
        name: `其他 ${tail.length} 只`,
        value: tail.reduce((s, it) => s + it.value, 0),
        itemStyle: { color: palette.neutral },
      });
    }

    return {
      tooltip: {
        trigger: 'item',
        formatter: (params: unknown) => {
          const p = params as { name: string; value: number; percent: number; color: string };
          return `${tipDot(p.color)}${p.name}<br/>市值 ${tipWan(p.value)}（${p.percent.toFixed(2)}%）`;
        },
      },
      legend: {
        type: 'scroll',
        orient: 'vertical',
        right: 0,
        top: 'middle',
        itemWidth: 8,
        itemHeight: 8,
        textStyle: { color: palette.textSecondary, fontSize: 11 },
      },
      series: [
        {
          type: 'pie',
          radius: ['48%', '72%'],
          center: ['34%', '50%'],
          avoidLabelOverlap: true,
          itemStyle: { borderColor: '#fff', borderWidth: 2 },
          label: { show: false },
          emphasis: {
            scaleSize: 6,
            label: { show: false },
          },
          data,
        },
      ],
    };
  }, [items]);

  return (
    <section className="chart-card">
      <div className="chart-card-head">
        <h2 className="chart-card-title">持仓分布</h2>
        <span className="chart-card-meta">
          {items.length} 只 · 合计 {formatWan(total)} 万
        </span>
      </div>
      <EChart
        option={option}
        height={320}
        loading={loading}
        empty={items.length === 0 ? '暂无持仓分布数据' : null}
        ariaLabel="持仓市值分布环形图"
      />
    </section>
  );
}

/** 账户仓位率：市值/现金堆叠柱 + 仓位率折线（双轴） */
function PosRateChart({ accounts, loading }: { accounts: AccountPosRate[]; loading: boolean }) {
  const option = useMemo<EChartsOption>(() => {
    const names = accounts.map((a) => a.name || a.broker || '—');
    return {
      grid: { left: 8, right: 8, top: 36, bottom: 4, containLabel: true },
      legend: {
        top: 0,
        itemWidth: 10,
        itemHeight: 10,
        textStyle: { color: palette.textSecondary, fontSize: 11 },
        data: ['持仓市值', '可用现金', '仓位率'],
      },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        formatter: (params: unknown) => {
          const arr = params as { dataIndex: number }[];
          if (!arr?.length) return '';
          const a = accounts[arr[0].dataIndex];
          if (!a) return '';
          return [
            `<strong>${a.name || '—'}</strong>${a.broker ? ` <span style="opacity:.6">${a.broker}</span>` : ''}`,
            `${tipDot(palette.accent)}持仓市值 ${tipMoney(a.stock_value)}`,
            `${tipDot(palette.neutral)}可用现金 ${tipMoney(a.money_remain)}`,
            `${tipDot(palette.up)}仓位率 ${formatPct(a.rate)}`,
            `合计 ${tipMoney(a.total)}`,
          ].join('<br/>');
        },
      },
      xAxis: categoryAxis(names),
      yAxis: [
        valueAxis({ formatter: axisWan }),
        // 仓位率是百分数（78.17），固定 0~100 便于跨账户比较
        valueAxis({
          formatter: (n: number) => `${n.toFixed(0)}%`,
          position: 'right',
          splitLine: false,
          max: 100,
          min: 0,
        }),
      ],
      series: [
        {
          name: '持仓市值',
          type: 'bar',
          stack: 'asset',
          data: accounts.map((a) => a.stock_value),
          itemStyle: { color: palette.accent, borderRadius: [0, 0, 0, 0] },
          barMaxWidth: 42,
        },
        {
          name: '可用现金',
          type: 'bar',
          stack: 'asset',
          data: accounts.map((a) => a.money_remain),
          itemStyle: { color: palette.grid, borderRadius: [4, 4, 0, 0] },
          barMaxWidth: 42,
        },
        {
          name: '仓位率',
          type: 'line',
          yAxisIndex: 1,
          data: accounts.map((a) => a.rate),
          symbolSize: 7,
          lineStyle: { color: palette.up, width: 2 },
          itemStyle: { color: palette.up },
          label: {
            show: true,
            position: 'top',
            formatter: numLabel((n) => `${n.toFixed(1)}%`),
            fontSize: 10,
            color: palette.up,
          },
        },
      ],
    };
  }, [accounts]);

  return (
    <section className="chart-card">
      <div className="chart-card-head">
        <h2 className="chart-card-title">账户仓位率</h2>
        <span className="chart-card-meta">{accounts.length} 个股票账户</span>
      </div>
      <EChart
        option={option}
        height={320}
        loading={loading}
        empty={accounts.length === 0 ? '暂无股票账户仓位数据' : null}
        ariaLabel="各账户持仓市值与仓位率柱状图"
      />
    </section>
  );
}

function DistributionTable({ items, total }: { items: DistItem[]; total: number }) {
  if (items.length === 0) return null;
  return (
    <section className="account-section">
      <div className="account-head">
        <h2 className="account-name">
          分布明细
          <span className="chip muted">按市值降序</span>
          <span className="chip">{items.length} 只</span>
        </h2>
      </div>
      <div className="table-container">
        <table className="data-table">
          <thead>
            <tr>
              <th>代码</th>
              <th>名称</th>
              <th>市场</th>
              <th className="num">市值</th>
              <th className="num">占比</th>
              <th className="weight-col">占比条</th>
            </tr>
          </thead>
          <tbody>
            {items.map((it) => (
              <tr key={it.code}>
                <td className="code">{it.code}</td>
                <td className="name">{it.name}</td>
                <td>
                  <span className="market-tag">{marketLabel(it.market)}</span>
                </td>
                <td className="num">{formatMoney(it.value)}</td>
                <td className="num">{formatPct(it.weight)}</td>
                <td className="weight-col">
                  <div className="weight-cell">
                    <div className="weight-bar" title={formatPct(it.weight)}>
                      <span style={{ width: `${Math.min(100, Math.max(0, it.weight))}%` }} />
                    </div>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="subtotal">
        <span>
          合计市值 <strong>{formatMoney(total)}</strong>
        </span>
      </div>
    </section>
  );
}

function PosRateTable({ accounts }: { accounts: AccountPosRate[] }) {
  if (accounts.length === 0) return null;
  return (
    <section className="account-section">
      <div className="account-head">
        <h2 className="account-name">
          账户仓位明细
          <span className="chip">{accounts.length} 个账户</span>
        </h2>
      </div>
      <div className="table-container">
        <table className="data-table">
          <thead>
            <tr>
              <th>账户</th>
              <th>券商</th>
              <th className="num">持仓市值</th>
              <th className="num">可用现金</th>
              <th className="num">合计</th>
              <th className="num">仓位率</th>
            </tr>
          </thead>
          <tbody>
            {accounts.map((a) => (
              <tr key={a.zjzh || a.name}>
                <td className="name">{a.name || '—'}</td>
                <td>{a.broker || '—'}</td>
                <td className="num">{a.stock_value.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}</td>
                <td className="num">{a.money_remain.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}</td>
                <td className="num">{a.total.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}</td>
                <td className="num">
                  <div className="posrate-cell">
                    <span>{formatPct(a.rate)}</span>
                    <div className="posrate-bar" title={formatPct(a.rate)}>
                      <span style={{ width: `${Math.min(100, Math.max(0, a.rate))}%` }} />
                    </div>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
