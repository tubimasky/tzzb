import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api, apiErrorMessage } from '../api/client';
import EChart from '../components/EChart';
import type { EChartsOption } from '../charts/echarts';
import {
  axisWan,
  categoryAxis,
  numLabel,
  palette,
  plColor,
  tipDot,
  tipMoney,
  valueAxis,
  zeroMarkLine,
} from '../charts/theme';
import type { PositionItem, PositionsDTO } from '../types/api';
import { formatMoney, formatPct, profitClass } from '../utils/format';
import './Positions.css';

type ViewMode = 'account' | 'flat' | 'total';
type SortKey = 'value' | 'day' | 'hold' | 'rate' | 'dayrate' | 'code' | 'name' | 'count';

type Group = {
  category: string;
  positions: PositionItem[];
  subtotal: { value: number; hold_profit: number; day_profit: number };
};

type TotalRow = {
  code: string;
  name: string;
  count: number;
  cost: number;
  price: number;
  value: number;
  hold_profit: number;
  hold_rate: number;
  day_profit: number;
  day_rate: number;
  accounts: string[];
};

const TYPE_OPTIONS = [
  { value: 'all', label: '全部' },
  { value: 'stock', label: '股票/ETF' },
  { value: 'fund', label: '基金' },
] as const;

const VIEW_OPTIONS: { value: ViewMode; label: string; hint: string }[] = [
  { value: 'account', label: '按账户', hint: '分账户明细' },
  { value: 'flat', label: '扁平表', hint: '同代码合并，一张表' },
  { value: 'total', label: '跨账户汇总', hint: '同代码合并' },
];

const SORT_OPTIONS: { value: SortKey; label: string }[] = [
  { value: 'value', label: '市值' },
  { value: 'day', label: '今日盈亏' },
  { value: 'hold', label: '持仓盈亏' },
  { value: 'rate', label: '收益率' },
  { value: 'dayrate', label: '今日涨幅' },
  { value: 'count', label: '持仓量' },
  { value: 'code', label: '代码' },
  { value: 'name', label: '名称' },
];

function Skeleton() {
  return (
    <div className="skeleton-page" aria-busy="true" aria-label="加载中">
      <div className="skeleton-block" style={{ height: 88 }} />
      <div className="skeleton-block tall" />
    </div>
  );
}

function cmp(a: number | string, b: number | string): number {
  if (typeof a === 'string' && typeof b === 'string') {
    return a.localeCompare(b, 'zh-CN', { numeric: true });
  }
  return (a as number) - (b as number);
}

function sortPositions(list: PositionItem[], key: SortKey, asc: boolean): PositionItem[] {
  const out = [...list];
  out.sort((x, y) => {
    let c = 0;
    switch (key) {
      case 'value':
        c = cmp(x.value, y.value);
        break;
      case 'day':
        c = cmp(x.day_profit, y.day_profit);
        break;
      case 'hold':
        c = cmp(x.hold_profit, y.hold_profit);
        break;
      case 'rate':
        c = cmp(x.hold_rate, y.hold_rate);
        break;
      case 'dayrate':
        c = cmp(x.day_rate, y.day_rate);
        break;
      case 'count':
        c = cmp(x.count, y.count);
        break;
      case 'code':
        c = cmp(x.code, y.code);
        break;
      case 'name':
        c = cmp(x.name, y.name);
        break;
    }
    return asc ? c : -c;
  });
  return out;
}

function sortTotals(list: TotalRow[], key: SortKey, asc: boolean): TotalRow[] {
  const out = [...list];
  out.sort((x, y) => {
    let c = 0;
    switch (key) {
      case 'value':
        c = cmp(x.value, y.value);
        break;
      case 'day':
        c = cmp(x.day_profit, y.day_profit);
        break;
      case 'hold':
        c = cmp(x.hold_profit, y.hold_profit);
        break;
      case 'rate':
        c = cmp(x.hold_rate, y.hold_rate);
        break;
      case 'dayrate':
        c = cmp(x.day_rate, y.day_rate);
        break;
      case 'count':
        c = cmp(x.count, y.count);
        break;
      case 'code':
        c = cmp(x.code, y.code);
        break;
      case 'name':
        c = cmp(x.name, y.name);
        break;
    }
    return asc ? c : -c;
  });
  return out;
}

/** 跨账户按代码合并（对齐 CLI position total） */
function mergeByCode(rows: PositionItem[]): TotalRow[] {
  const map = new Map<string, TotalRow & { costBasis: number; valueWeightDay: number }>();

  for (const r of rows) {
    const code = (r.code || '').trim();
    if (!code) continue;
    let m = map.get(code);
    if (!m) {
      m = {
        code,
        name: r.name,
        count: 0,
        cost: 0,
        price: 0,
        value: 0,
        hold_profit: 0,
        hold_rate: 0,
        day_profit: 0,
        day_rate: 0,
        accounts: [],
        costBasis: 0,
        valueWeightDay: 0,
      };
      map.set(code, m);
    }
    m.count += r.count;
    m.costBasis += r.cost * r.count;
    m.value += r.value;
    m.hold_profit += r.hold_profit;
    m.day_profit += r.day_profit;
    m.valueWeightDay += r.day_rate * r.value;
    if (r.name && (!m.name || m.name.length < r.name.length)) m.name = r.name;
    if (r.account && !m.accounts.includes(r.account)) m.accounts.push(r.account);
  }

  return Array.from(map.values()).map((m) => {
    const cost = m.count > 0 ? m.costBasis / m.count : 0;
    const price = m.count > 0 ? m.value / m.count : 0;
    const hold_rate = m.costBasis !== 0 ? (m.hold_profit / m.costBasis) * 100 : 0;
    const day_rate = m.value !== 0 ? m.valueWeightDay / m.value : 0;
    return {
      code: m.code,
      name: m.name,
      count: m.count,
      cost,
      price,
      value: m.value,
      hold_profit: m.hold_profit,
      hold_rate,
      day_profit: m.day_profit,
      day_rate,
      accounts: m.accounts,
    };
  });
}

function SortHeader({
  label,
  col,
  sortKey,
  sortAsc,
  onSort,
  className,
}: {
  label: string;
  col: SortKey;
  sortKey: SortKey;
  sortAsc: boolean;
  onSort: (k: SortKey) => void;
  className?: string;
}) {
  const active = sortKey === col;
  return (
    <th className={className}>
      <button
        type="button"
        className={`th-sort${active ? ' active' : ''}`}
        onClick={() => onSort(col)}
        title="点击排序"
      >
        {label}
        <span className="th-arrow" aria-hidden>
          {active ? (sortAsc ? '↑' : '↓') : '↕'}
        </span>
      </button>
    </th>
  );
}

/** 基金账户无法走 stock detail 接口 */
function isFundCategory(category: string): boolean {
  const c = (category || '').toLowerCase();
  return c === 'fund' || c.includes('基金');
}

function openDetailPath(code: string, account?: string): string {
  const base = `/detail/${encodeURIComponent(code)}`;
  if (account) return `${base}?account=${encodeURIComponent(account)}`;
  return base;
}

export default function Positions() {
  const navigate = useNavigate();
  const [data, setData] = useState<PositionsDTO | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [viewMode, setViewMode] = useState<ViewMode>('account');
  const [typeFilter, setTypeFilter] = useState('all');
  const [accountFilter, setAccountFilter] = useState('');
  const [keyword, setKeyword] = useState('');
  const [sortKey, setSortKey] = useState<SortKey>('value');
  const [sortAsc, setSortAsc] = useState(false);

  function goDetail(code: string, account?: string, category?: string) {
    if (!code || isFundCategory(category || '')) return;
    navigate(openDetailPath(code, account));
  }

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    api
      .getPositions({ type: typeFilter })
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
  }, [typeFilter]);

  const accountOptions = useMemo(() => {
    if (!data) return [] as string[];
    const set = new Set<string>();
    for (const p of data.positions) {
      if (p.account) set.add(p.account);
    }
    return Array.from(set).sort((a, b) => a.localeCompare(b, 'zh-CN'));
  }, [data]);

  const filtered = useMemo(() => {
    if (!data) return [] as PositionItem[];
    const acc = accountFilter.trim().toLowerCase();
    const kw = keyword.trim().toLowerCase();
    return data.positions.filter((p) => {
      if (acc && !p.account.toLowerCase().includes(acc)) return false;
      if (kw) {
        const hay = `${p.code} ${p.name}`.toLowerCase();
        if (!hay.includes(kw)) return false;
      }
      return true;
    });
  }, [data, accountFilter, keyword]);

  // 按账户视图仍用未合并明细排序
  const sortedByAccount = useMemo(
    () => sortPositions(filtered, sortKey, sortAsc),
    [filtered, sortKey, sortAsc],
  );

  const grouped = useMemo(() => {
    const map: Record<string, Group> = {};
    for (const pos of sortedByAccount) {
      if (!map[pos.account]) {
        map[pos.account] = {
          category: pos.category,
          positions: [],
          subtotal: { value: 0, hold_profit: 0, day_profit: 0 },
        };
      }
      map[pos.account].positions.push(pos);
      map[pos.account].subtotal.value += pos.value;
      map[pos.account].subtotal.hold_profit += pos.hold_profit;
      map[pos.account].subtotal.day_profit += pos.day_profit;
    }
    // 账户块按小计市值降序
    return Object.entries(map).sort((a, b) => b[1].subtotal.value - a[1].subtotal.value);
  }, [sortedByAccount]);

  // 扁平表 / 跨账户汇总：同代码合并（对齐 CLI position total / stock）
  const totals = useMemo(
    () => sortTotals(mergeByCode(filtered), sortKey, sortAsc),
    [filtered, sortKey, sortAsc],
  );

  const viewSummary = useMemo(() => {
    const merged = viewMode === 'flat' || viewMode === 'total';
    let value = 0;
    let hold = 0;
    let day = 0;
    let costBasis = 0;
    if (merged) {
      for (const r of totals) {
        value += r.value;
        hold += r.hold_profit;
        day += r.day_profit;
        costBasis += r.cost * r.count;
      }
    } else {
      for (const r of filtered) {
        value += r.value;
        hold += r.hold_profit;
        day += r.day_profit;
        costBasis += r.cost * r.count;
      }
    }
    // 对齐 CLI 合计行：收益率 = 持仓盈亏/总成本；今日涨幅 ≈ 今日盈亏/(市值−今日盈亏)
    const hold_rate = costBasis !== 0 ? (hold / costBasis) * 100 : 0;
    const dayDen = value - day;
    const day_rate = dayDen !== 0 ? (day / dayDen) * 100 : 0;
    return {
      value,
      hold,
      day,
      hold_rate,
      day_rate,
      count: merged ? totals.length : filtered.length,
      lines: filtered.length,
    };
  }, [viewMode, filtered, totals]);

  function toggleSort(col: SortKey) {
    if (sortKey === col) {
      setSortAsc((v) => !v);
    } else {
      setSortKey(col);
      // 金额类默认降序，名称/代码默认升序
      setSortAsc(col === 'code' || col === 'name');
    }
  }

  if (loading && !data) return <Skeleton />;
  if (error && !data) return <div className="error">错误: {error}</div>;
  if (!data) return null;

  const empty = filtered.length === 0;

  return (
    <div className="page positions-page">
      <header className="page-header">
        <div className="page-header-text">
          <div className="page-eyebrow">Holdings</div>
          <h1 className="page-title">
            持仓<em>明细</em>
          </h1>
          <p className="page-subtitle">
            对齐 CLI：按账户分块 / 扁平合并表（同 total）/ 跨账户汇总；点击股票行可下钻明细
          </p>
        </div>
      </header>

      <div className="filters positions-filters">
        <div className="filter-block">
          <span className="field-label">视图</span>
          <div className="segmented" role="tablist" aria-label="视图模式">
            {VIEW_OPTIONS.map((opt) => (
              <button
                key={opt.value}
                type="button"
                role="tab"
                title={opt.hint}
                aria-selected={viewMode === opt.value}
                className={viewMode === opt.value ? 'active' : undefined}
                onClick={() => setViewMode(opt.value)}
              >
                {opt.label}
              </button>
            ))}
          </div>
        </div>

        <div className="filter-block">
          <span className="field-label">类型</span>
          <div className="segmented" role="tablist" aria-label="持仓类型">
            {TYPE_OPTIONS.map((opt) => (
              <button
                key={opt.value}
                type="button"
                role="tab"
                aria-selected={typeFilter === opt.value}
                className={typeFilter === opt.value ? 'active' : undefined}
                onClick={() => setTypeFilter(opt.value)}
              >
                {opt.label}
              </button>
            ))}
          </div>
        </div>

        <label className="filter-field">
          <span className="field-label">账户</span>
          <select value={accountFilter} onChange={(e) => setAccountFilter(e.target.value)}>
            <option value="">全部账户</option>
            {accountOptions.map((name) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))}
          </select>
        </label>

        <label className="filter-field">
          <span className="field-label">搜索</span>
          <input
            type="search"
            placeholder="代码 / 名称"
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            autoComplete="off"
          />
        </label>

        <label className="filter-field filter-field-sm">
          <span className="field-label">排序</span>
          <div className="sort-row">
            <select
              value={sortKey}
              onChange={(e) => setSortKey(e.target.value as SortKey)}
            >
              {SORT_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
            <button
              type="button"
              className="sort-dir-btn"
              onClick={() => setSortAsc((v) => !v)}
              title={sortAsc ? '当前升序，点击改降序' : '当前降序，点击改升序'}
            >
              {sortAsc ? '升序 ↑' : '降序 ↓'}
            </button>
          </div>
        </label>
      </div>

      {error && <div className="error inline-error">刷新失败: {error}</div>}

      {empty && <div className="empty-state">暂无持仓数据，试试调整筛选条件</div>}

      {!empty && viewMode === 'account' &&
        grouped.map(([account, group]) => (
          <section key={account} className="account-section">
            <div className="account-head">
              <h2 className="account-name">
                {account}
                <span className="chip muted">{group.category}</span>
                <span className="chip">{group.positions.length} 标的</span>
              </h2>
            </div>
            <PositionTable
              rows={group.positions}
              sortKey={sortKey}
              sortAsc={sortAsc}
              onSort={toggleSort}
              showAccount={false}
              onOpen={goDetail}
            />
            <div className="subtotal">
              <span>
                市值 <strong>{formatMoney(group.subtotal.value)}</strong>
              </span>
              <span>
                持仓盈亏{' '}
                <strong className={profitClass(group.subtotal.hold_profit)}>
                  {formatMoney(group.subtotal.hold_profit)}
                </strong>
              </span>
              <span>
                今日盈亏{' '}
                <strong className={profitClass(group.subtotal.day_profit)}>
                  {formatMoney(group.subtotal.day_profit)}
                </strong>
              </span>
            </div>
          </section>
        ))}

      {!empty && (viewMode === 'flat' || viewMode === 'total') && (
        <section className="account-section">
          <div className="account-head">
            <h2 className="account-name">
              {viewMode === 'flat' ? '扁平表' : '跨账户汇总'}
              <span className="chip muted">同代码合并</span>
              <span className="chip">{totals.length} 标的</span>
              {viewSummary.lines !== totals.length && (
                <span className="chip muted" title="合并前明细行数">
                  来自 {viewSummary.lines} 行
                </span>
              )}
            </h2>
          </div>
          <TotalTable
            rows={totals}
            sortKey={sortKey}
            sortAsc={sortAsc}
            onSort={toggleSort}
            onOpen={(code) => goDetail(code)}
          />
        </section>
      )}

      {!empty && <PositionCharts rows={totals} />}

      {!empty && (
        <div className="summary-card">
          <div className="summary-grid">
            <div className="summary-item">
              <div className="summary-label">总市值</div>
              <div className="summary-value">{formatMoney(viewSummary.value)}</div>
            </div>
            <div className="summary-item">
              <div className="summary-label">持仓盈亏</div>
              <div className={`summary-value ${profitClass(viewSummary.hold)}`}>
                {formatMoney(viewSummary.hold)}
              </div>
              <div className={`summary-sub ${profitClass(viewSummary.hold_rate)}`}>
                收益率 {formatPct(viewSummary.hold_rate, { signed: true })}
              </div>
            </div>
            <div className="summary-item">
              <div className="summary-label">今日盈亏</div>
              <div className={`summary-value ${profitClass(viewSummary.day)}`}>
                {formatMoney(viewSummary.day)}
              </div>
              <div className={`summary-sub ${profitClass(viewSummary.day_rate)}`}>
                今日涨幅 {formatPct(viewSummary.day_rate, { signed: true })}
              </div>
            </div>
            <div className="summary-item">
              <div className="summary-label">
                {viewMode === 'account' ? '明细行数' : '合并标的'}
              </div>
              <div className="summary-value">{viewSummary.count}</div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function PositionTable({
  rows,
  sortKey,
  sortAsc,
  onSort,
  showAccount,
  onOpen,
}: {
  rows: PositionItem[];
  sortKey: SortKey;
  sortAsc: boolean;
  onSort: (k: SortKey) => void;
  showAccount: boolean;
  onOpen: (code: string, account?: string, category?: string) => void;
}) {
  return (
    <div className="table-container">
      <table className="data-table">
        <thead>
          <tr>
            {showAccount && <th>账户</th>}
            <SortHeader label="代码" col="code" sortKey={sortKey} sortAsc={sortAsc} onSort={onSort} />
            <SortHeader label="名称" col="name" sortKey={sortKey} sortAsc={sortAsc} onSort={onSort} />
            <SortHeader
              label="持仓量"
              col="count"
              sortKey={sortKey}
              sortAsc={sortAsc}
              onSort={onSort}
              className="num"
            />
            <th className="num">成本价</th>
            <th className="num">现价</th>
            <SortHeader
              label="市值"
              col="value"
              sortKey={sortKey}
              sortAsc={sortAsc}
              onSort={onSort}
              className="num"
            />
            <SortHeader
              label="持仓盈亏"
              col="hold"
              sortKey={sortKey}
              sortAsc={sortAsc}
              onSort={onSort}
              className="num"
            />
            <SortHeader
              label="今日盈亏"
              col="day"
              sortKey={sortKey}
              sortAsc={sortAsc}
              onSort={onSort}
              className="num"
            />
            <SortHeader
              label="今日涨幅"
              col="dayrate"
              sortKey={sortKey}
              sortAsc={sortAsc}
              onSort={onSort}
              className="num"
            />
            <SortHeader
              label="收益率"
              col="rate"
              sortKey={sortKey}
              sortAsc={sortAsc}
              onSort={onSort}
              className="num"
            />
          </tr>
        </thead>
        <tbody>
          {rows.map((pos, idx) => {
            const clickable = !isFundCategory(pos.category);
            // 同账户同 code 可能多行（上游拆单），key 必须含 idx，否则排序时 DOM 幽灵行
            return (
              <tr
                key={`${pos.account}-${pos.code}-${idx}`}
                className={clickable ? 'row-clickable' : undefined}
                title={clickable ? '点击查看明细' : '基金暂不支持钻取'}
                onClick={() => clickable && onOpen(pos.code, pos.account, pos.category)}
                onKeyDown={(e) => {
                  if (!clickable) return;
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    onOpen(pos.code, pos.account, pos.category);
                  }
                }}
                tabIndex={clickable ? 0 : undefined}
              >
                {showAccount && <td className="acc-cell">{pos.account}</td>}
                <td className="code">
                  {clickable ? <span className="code-link">{pos.code}</span> : pos.code}
                </td>
                <td className="name">{pos.name}</td>
                <td className="num">{formatCount(pos.count)}</td>
                <td className="num">{pos.cost.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}</td>
                <td className="num">{pos.price.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}</td>
                <td className="num">{formatMoney(pos.value)}</td>
                <td className={`num ${profitClass(pos.hold_profit)}`}>
                  {formatMoney(pos.hold_profit)}
                </td>
                <td className={`num ${profitClass(pos.day_profit)}`}>
                  {formatMoney(pos.day_profit)}
                </td>
                <td className={`num ${profitClass(pos.day_rate)}`}>
                  {formatPct(pos.day_rate, { signed: true })}
                </td>
                <td className={`num ${profitClass(pos.hold_profit)}`}>
                  {formatPct(pos.hold_rate, { signed: true })}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function TotalTable({
  rows,
  sortKey,
  sortAsc,
  onSort,
  onOpen,
}: {
  rows: TotalRow[];
  sortKey: SortKey;
  sortAsc: boolean;
  onSort: (k: SortKey) => void;
  onOpen: (code: string) => void;
}) {
  return (
    <div className="table-container">
      <table className="data-table">
        <thead>
          <tr>
            <SortHeader label="代码" col="code" sortKey={sortKey} sortAsc={sortAsc} onSort={onSort} />
            <SortHeader label="名称" col="name" sortKey={sortKey} sortAsc={sortAsc} onSort={onSort} />
            <th>分布账户</th>
            <SortHeader
              label="持仓量"
              col="count"
              sortKey={sortKey}
              sortAsc={sortAsc}
              onSort={onSort}
              className="num"
            />
            <th className="num">成本价</th>
            <th className="num">现价</th>
            <SortHeader
              label="市值"
              col="value"
              sortKey={sortKey}
              sortAsc={sortAsc}
              onSort={onSort}
              className="num"
            />
            <SortHeader
              label="持仓盈亏"
              col="hold"
              sortKey={sortKey}
              sortAsc={sortAsc}
              onSort={onSort}
              className="num"
            />
            <SortHeader
              label="今日盈亏"
              col="day"
              sortKey={sortKey}
              sortAsc={sortAsc}
              onSort={onSort}
              className="num"
            />
            <SortHeader
              label="今日涨幅"
              col="dayrate"
              sortKey={sortKey}
              sortAsc={sortAsc}
              onSort={onSort}
              className="num"
            />
            <SortHeader
              label="收益率"
              col="rate"
              sortKey={sortKey}
              sortAsc={sortAsc}
              onSort={onSort}
              className="num"
            />
          </tr>
        </thead>
        <tbody>
          {rows.map((pos) => (
            <tr
              key={pos.code}
              className="row-clickable"
              title="点击查看明细"
              onClick={() => onOpen(pos.code)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  onOpen(pos.code);
                }
              }}
              tabIndex={0}
            >
              <td className="code">
                <span className="code-link">{pos.code}</span>
              </td>
              <td className="name">{pos.name}</td>
              <td className="accounts-cell">
                {pos.accounts.length <= 1 ? (
                  pos.accounts[0] || '—'
                ) : (
                  <span className="account-tags" title={pos.accounts.join('、')}>
                    {pos.accounts.map((a) => (
                      <span key={a} className="chip muted tiny">
                        {a}
                      </span>
                    ))}
                  </span>
                )}
              </td>
              <td className="num">{formatCount(pos.count)}</td>
              <td className="num">{pos.cost.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}</td>
              <td className="num">{pos.price.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}</td>
              <td className="num">{formatMoney(pos.value)}</td>
              <td className={`num ${profitClass(pos.hold_profit)}`}>
                {formatMoney(pos.hold_profit)}
              </td>
              <td className={`num ${profitClass(pos.day_profit)}`}>
                {formatMoney(pos.day_profit)}
              </td>
              <td className={`num ${profitClass(pos.day_rate)}`}>
                {formatPct(pos.day_rate, { signed: true })}
              </td>
              <td className={`num ${profitClass(pos.hold_profit)}`}>
                {formatPct(pos.hold_rate, { signed: true })}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function formatCount(n: number): string {
  if (!Number.isFinite(n)) return '—';
  // 基金可能有小数份额
  if (Math.abs(n - Math.round(n)) < 1e-6) return Math.round(n).toLocaleString('zh-CN');
  return n.toLocaleString('zh-CN', { maximumFractionDigits: 2 });
}

/**
 * 持仓图表：市值 Top10 + 持仓盈亏。
 * 数据由已加载的合并行（同代码合并）派生，不额外请求接口。
 */
function PositionCharts({ rows }: { rows: TotalRow[] }) {
  // 市值 Top10：横向柱状，倒序让最大值在顶部（echarts y 轴自下而上）
  const valueOption = useMemo<EChartsOption>(() => {
    const top = [...rows].sort((a, b) => b.value - a.value).slice(0, 10).reverse();
    const names = top.map((r) => r.name || r.code);
    const total = rows.reduce((s, r) => s + r.value, 0);
    return {
      grid: { left: 8, right: 64, top: 8, bottom: 8, containLabel: true },
      xAxis: { ...valueAxis({ formatter: axisWan }), splitLine: { show: false } },
      yAxis: {
        ...categoryAxis(names),
        axisLabel: { color: palette.textSecondary, fontSize: 11 },
      },
      tooltip: {
        trigger: 'item',
        formatter: (params: unknown) => {
          const p = params as { dataIndex: number };
          const r = top[p.dataIndex];
          const weight = total > 0 ? (r.value / total) * 100 : 0;
          return [
            `${r.name || r.code} <span style="opacity:.6">${r.code}</span>`,
            `${tipDot(palette.accent)}市值 ${tipMoney(r.value)}（${weight.toFixed(2)}%）`,
            `${tipDot(plColor(r.hold_profit))}持仓盈亏 ${tipMoney(r.hold_profit, true)}`,
          ].join('<br/>');
        },
      },
      series: [
        {
          type: 'bar',
          data: top.map((r) => r.value),
          barMaxWidth: 14,
          itemStyle: { color: palette.accent, borderRadius: [0, 4, 4, 0] },
          label: {
            show: true,
            position: 'right',
            formatter: numLabel((n) => `${(n / 10000).toFixed(1)}万`),
            fontSize: 10,
            color: palette.textTertiary,
          },
        },
      ],
    };
  }, [rows]);

  // 持仓盈亏：按盈亏排序的双向柱状，红涨绿跌
  const profitOption = useMemo<EChartsOption>(() => {
    const sorted = [...rows]
      .filter((r) => r.hold_profit !== 0)
      .sort((a, b) => b.hold_profit - a.hold_profit);
    const names = sorted.map((r) => r.name || r.code);
    return {
      grid: { left: 8, right: 16, top: 20, bottom: 8, containLabel: true },
      xAxis: categoryAxis(names, { rotate: sorted.length > 12 ? 45 : 0 }),
      yAxis: valueAxis({ formatter: axisWan }),
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        formatter: (params: unknown) => {
          const arr = params as { dataIndex: number }[];
          if (!arr?.length) return '';
          const r = sorted[arr[0].dataIndex];
          return [
            `${r.name || r.code} <span style="opacity:.6">${r.code}</span>`,
            `${tipDot(plColor(r.hold_profit))}持仓盈亏 ${tipMoney(r.hold_profit, true)}`,
            `收益率 ${formatPct(r.hold_rate, { signed: true })}`,
            `市值 ${tipMoney(r.value)}`,
          ].join('<br/>');
        },
      },
      series: [
        {
          type: 'bar',
          data: sorted.map((r) => ({
            value: r.hold_profit,
            itemStyle: { color: plColor(r.hold_profit) },
          })),
          barMaxWidth: 22,
          markLine: zeroMarkLine,
        },
      ],
    };
  }, [rows]);

  const hasProfit = rows.some((r) => r.hold_profit !== 0);

  return (
    <div className="chart-grid">
      <section className="chart-card">
        <div className="chart-card-head">
          <h2 className="chart-card-title">市值 Top 10</h2>
          <span className="chart-card-meta">{rows.length} 只标的</span>
        </div>
        <EChart
          option={valueOption}
          height={300}
          empty={rows.length === 0 ? '暂无持仓' : null}
          ariaLabel="持仓市值 Top 10 柱状图"
        />
      </section>

      <section className="chart-card">
        <div className="chart-card-head">
          <h2 className="chart-card-title">持仓盈亏</h2>
          <span className="chart-card-meta">红涨绿跌</span>
        </div>
        <EChart
          option={profitOption}
          height={300}
          empty={hasProfit ? null : '暂无盈亏数据'}
          ariaLabel="各标的持仓盈亏柱状图"
        />
      </section>
    </div>
  );
}
