import { useEffect, useMemo, useState } from 'react';
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
import type { CalendarMonthDTO, CalendarYearDTO, CodePL, DayCell } from '../types/api';
import { formatMoney, profitClass } from '../utils/format';
import './CalendarPage.css';

type ViewMode = 'month' | 'year';
type Cat = 'total' | 'stock' | 'fund';

const CAT_OPTIONS: { value: Cat; label: string }[] = [
  { value: 'total', label: '总盈亏' },
  { value: 'stock', label: '股票' },
  { value: 'fund', label: '基金' },
];

const WEEKDAYS = ['一', '二', '三', '四', '五', '六', '日'];

/** 当前月 YYYY-MM（<input type="month"> 的取值格式） */
function currentMonthInput(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;
}

/** YYYY-MM → YYYYMM（后端要求） */
function toApiMonth(v: string): string {
  return v.replace('-', '');
}

/** 取单元格里选中类别的盈亏 */
function cellValue(cell: DayCell, cat: Cat): number {
  if (cat === 'stock') return cell.stock_pl;
  if (cat === 'fund') return cell.fund_pl;
  return cell.total_pl;
}

/** 盈亏热力等级：按当月绝对值最大值分 3 档，用于日历格背景深浅 */
function heatLevel(value: number, max: number): string {
  if (value === 0 || max <= 0) return 'lv0';
  const ratio = Math.abs(value) / max;
  const dir = value > 0 ? 'up' : 'down';
  if (ratio > 0.66) return `${dir}3`;
  if (ratio > 0.33) return `${dir}2`;
  return `${dir}1`;
}

export default function CalendarPage() {
  const [view, setView] = useState<ViewMode>('month');
  const [cat, setCat] = useState<Cat>('total');
  const [month, setMonth] = useState(currentMonthInput());
  const [year, setYear] = useState(String(new Date().getFullYear()));
  const [account, setAccount] = useState('');
  const [accountOptions, setAccountOptions] = useState<string[]>([]);

  const [monthData, setMonthData] = useState<CalendarMonthDTO | null>(null);
  const [yearData, setYearData] = useState<CalendarYearDTO | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // 账户下拉
  useEffect(() => {
    let cancelled = false;
    api
      .getAccounts()
      .then((res) => {
        if (!cancelled) setAccountOptions(res.data.accounts.map((a) => a.name));
      })
      .catch(() => {
        /* 账户列表失败不影响主数据 */
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
      view === 'month'
        ? api.getCalendarMonth(toApiMonth(month), account, cat)
        : api.getCalendarYear(year, account, cat);

    req
      .then((res) => {
        if (cancelled) return;
        if (view === 'month') setMonthData(res.data as CalendarMonthDTO);
        else setYearData(res.data as CalendarYearDTO);
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
  }, [view, month, year, account, cat]);

  const active = view === 'month' ? monthData : yearData;
  const catTotal = useMemo(() => {
    if (!active) return 0;
    if (cat === 'stock') return active.stock_pl;
    if (cat === 'fund') return active.fund_pl;
    return active.total_pl;
  }, [active, cat]);

  // 月视图：每日柱状
  const dailyOption = useMemo<EChartsOption>(() => {
    if (!monthData) return {};
    const cells = monthData.weeks.flat().filter((c) => c.day > 0 && c.has_data);
    const dates = cells.map((c) => String(c.day));
    const values = cells.map((c) => cellValue(c, cat));
    return {
      xAxis: categoryAxis(dates),
      yAxis: valueAxis({ formatter: axisWan }),
      tooltip: {
        trigger: 'axis',
        formatter: (params: unknown) => {
          const arr = params as { dataIndex: number }[];
          const idx = arr[0]?.dataIndex ?? 0;
          const c = cells[idx];
          const v = cellValue(c, cat);
          return `${c.date.slice(4, 6)}-${c.date.slice(6, 8)}<br/>${tipDot(plColor(v))}盈亏 ${tipMoney(v, true)}`;
        },
      },
      series: [
        {
          type: 'bar',
          data: values.map((v) => ({ value: v, itemStyle: { color: plColor(v) } })),
          barMaxWidth: 22,
          markLine: zeroMarkLine,
        },
      ],
    };
  }, [monthData, cat]);

  // 年视图：12 个月柱状
  const monthlyOption = useMemo<EChartsOption>(() => {
    if (!yearData) return {};
    const pts = yearData.months;
    const labels = pts.map((p) => `${Number(p.date.slice(4, 6))}月`);
    const values = pts.map((p) =>
      cat === 'stock' ? p.stock_pl : cat === 'fund' ? p.fund_pl : p.total_pl,
    );
    return {
      xAxis: categoryAxis(labels),
      yAxis: valueAxis({ formatter: axisWan }),
      tooltip: {
        trigger: 'axis',
        formatter: (params: unknown) => {
          const arr = params as { dataIndex: number }[];
          const idx = arr[0]?.dataIndex ?? 0;
          const v = values[idx];
          return `${labels[idx]}<br/>${tipDot(plColor(v))}盈亏 ${tipMoney(v, true)}`;
        },
      },
      series: [
        {
          type: 'bar',
          data: values.map((v) => ({ value: v, itemStyle: { color: plColor(v) } })),
          barMaxWidth: 34,
          markLine: zeroMarkLine,
        },
      ],
    };
  }, [yearData, cat]);

  // 累计盈亏曲线（月/年通用）
  const cumulativeOption = useMemo<EChartsOption>(() => {
    let labels: string[] = [];
    let values: number[] = [];

    if (view === 'month' && monthData) {
      const cells = monthData.weeks.flat().filter((c) => c.day > 0 && c.has_data);
      labels = cells.map((c) => String(c.day));
      let acc = 0;
      values = cells.map((c) => {
        acc += cellValue(c, cat);
        return Number(acc.toFixed(2));
      });
    } else if (view === 'year' && yearData) {
      labels = yearData.months.map((p) => `${Number(p.date.slice(4, 6))}月`);
      let acc = 0;
      values = yearData.months.map((p) => {
        acc += cat === 'stock' ? p.stock_pl : cat === 'fund' ? p.fund_pl : p.total_pl;
        return Number(acc.toFixed(2));
      });
    }

    const last = values[values.length - 1] ?? 0;
    const lineColor = plColor(last);

    return {
      xAxis: categoryAxis(labels),
      yAxis: valueAxis({ formatter: axisWan }),
      tooltip: {
        trigger: 'axis',
        formatter: (params: unknown) => {
          const arr = params as { dataIndex: number; value: number }[];
          const p = arr[0];
          if (!p) return '';
          return `${labels[p.dataIndex]}<br/>${tipDot(lineColor)}累计 ${tipMoney(p.value, true)}`;
        },
      },
      series: [
        {
          type: 'line',
          data: values,
          smooth: true,
          symbol: 'none',
          lineStyle: { width: 2, color: lineColor },
          areaStyle: {
            color: last >= 0 ? 'rgba(225, 29, 72, 0.10)' : 'rgba(5, 150, 105, 0.10)',
          },
          markLine: zeroMarkLine,
        },
      ],
    };
  }, [view, monthData, yearData, cat]);

  const topCodes = active?.top_codes ?? [];

  return (
    <div className="page calendar-page">
      <header className="page-header">
        <div className="page-header-text">
          <div className="page-eyebrow">Calendar</div>
          <h1 className="page-title">
            盈亏<em>日历</em>
          </h1>
          <p className="page-subtitle">按日/按月查看盈亏节奏，支持总盈亏与股票、基金分类口径</p>
        </div>
      </header>

      <div className="filters">
        <div className="filter-block">
          <span className="field-label">视图</span>
          <div className="segmented" role="tablist" aria-label="视图模式">
            <button
              type="button"
              role="tab"
              aria-selected={view === 'month'}
              className={view === 'month' ? 'active' : undefined}
              onClick={() => setView('month')}
            >
              月历
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={view === 'year'}
              className={view === 'year' ? 'active' : undefined}
              onClick={() => setView('year')}
            >
              年视图
            </button>
          </div>
        </div>

        <div className="filter-block">
          <span className="field-label">类别</span>
          <div className="segmented" role="tablist" aria-label="盈亏类别">
            {CAT_OPTIONS.map((o) => (
              <button
                key={o.value}
                type="button"
                role="tab"
                aria-selected={cat === o.value}
                className={cat === o.value ? 'active' : undefined}
                onClick={() => setCat(o.value)}
              >
                {o.label}
              </button>
            ))}
          </div>
        </div>

        {view === 'month' ? (
          <label className="filter-field filter-field-sm">
            <span className="field-label">月份</span>
            <input type="month" value={month} onChange={(e) => setMonth(e.target.value)} />
          </label>
        ) : (
          <label className="filter-field filter-field-sm">
            <span className="field-label">年份</span>
            <input
              type="number"
              min="2000"
              max="2100"
              step="1"
              value={year}
              onChange={(e) => setYear(e.target.value)}
            />
          </label>
        )}

        <label className="filter-field">
          <span className="field-label">账户</span>
          <select value={account} onChange={(e) => setAccount(e.target.value)}>
            <option value="">全部账户</option>
            {accountOptions.map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </select>
        </label>
      </div>

      {error && <div className="error inline-error">加载失败: {error}</div>}

      {active && (
        <section className="cal-summary" aria-label="区间汇总">
          <div className="cal-summary-item">
            <span className="cal-summary-label">
              {view === 'month' ? `${active.year} 年 ${(active as CalendarMonthDTO).month ?? ''} 月` : `${active.year} 年`}
              合计（{CAT_OPTIONS.find((o) => o.value === cat)?.label}）
            </span>
            <strong className={`cal-summary-value ${profitClass(catTotal)}`}>
              {formatMoney(catTotal)}
            </strong>
          </div>
          <div className="cal-summary-item">
            <span className="cal-summary-label">股票</span>
            <strong className={`cal-summary-value sm ${profitClass(active.stock_pl)}`}>
              {formatMoney(active.stock_pl)}
            </strong>
          </div>
          <div className="cal-summary-item">
            <span className="cal-summary-label">基金</span>
            <strong className={`cal-summary-value sm ${profitClass(active.fund_pl)}`}>
              {formatMoney(active.fund_pl)}
            </strong>
          </div>
        </section>
      )}

      {view === 'month' && (
        <MonthGrid data={monthData} cat={cat} loading={loading} />
      )}

      <div className="chart-grid">
        <div className="chart-card span-2">
          <div className="chart-card-head">
            <h2 className="chart-card-title">
              {view === 'month' ? '每日盈亏' : '各月盈亏'}
            </h2>
            <span className="chart-card-meta">红涨绿跌</span>
          </div>
          <EChart
            option={view === 'month' ? dailyOption : monthlyOption}
            loading={loading}
            empty={
              !loading && (view === 'month' ? !monthData : !yearData) ? '暂无数据' : null
            }
            height={260}
            ariaLabel={view === 'month' ? '每日盈亏柱状图' : '各月盈亏柱状图'}
          />
        </div>

        <div className="chart-card">
          <div className="chart-card-head">
            <h2 className="chart-card-title">累计盈亏</h2>
            <span className={`chart-card-meta ${profitClass(catTotal)}`}>
              {formatMoney(catTotal)}
            </span>
          </div>
          <EChart
            option={cumulativeOption}
            loading={loading}
            empty={!loading && !active ? '暂无数据' : null}
            height={260}
            ariaLabel="累计盈亏曲线"
          />
        </div>

        <div className="chart-card">
          <div className="chart-card-head">
            <h2 className="chart-card-title">标的盈亏 Top</h2>
            <span className="chart-card-meta">{topCodes.length} 只</span>
          </div>
          <TopCodesChart codes={topCodes} loading={loading} />
        </div>
      </div>
    </div>
  );
}

/** 月历热力网格 */
function MonthGrid({
  data,
  cat,
  loading,
}: {
  data: CalendarMonthDTO | null;
  cat: Cat;
  loading: boolean;
}) {
  const max = useMemo(() => {
    if (!data) return 0;
    return data.weeks
      .flat()
      .reduce((m, c) => (c.has_data ? Math.max(m, Math.abs(cellValue(c, cat))) : m), 0);
  }, [data, cat]);

  if (loading) {
    return <div className="skeleton-block" style={{ height: 300, marginBottom: '1.5rem' }} />;
  }
  if (!data) return null;

  return (
    <section className="cal-grid-wrap" aria-label="月历">
      <div className="cal-grid-head">
        {WEEKDAYS.map((w) => (
          <div key={w} className="cal-weekday">
            {w}
          </div>
        ))}
      </div>
      <div className="cal-grid">
        {data.weeks.flat().map((cell, i) => {
          // day=0 是月初/月末补齐的占位格
          if (cell.day === 0) return <div key={`pad-${i}`} className="cal-cell pad" />;
          const v = cellValue(cell, cat);
          return (
            <div
              key={cell.date}
              className={`cal-cell ${cell.has_data ? heatLevel(v, max) : 'nodata'}`}
              title={
                cell.has_data
                  ? `${cell.date.slice(0, 4)}-${cell.date.slice(4, 6)}-${cell.date.slice(6, 8)}  ${formatMoney(v)}`
                  : '无交易数据'
              }
            >
              <span className="cal-day">{cell.day}</span>
              <span className={`cal-pl ${profitClass(v)}`}>
                {cell.has_data ? formatMoney(v, 0) : '·'}
              </span>
            </div>
          );
        })}
      </div>
    </section>
  );
}

/** Top 标的横向柱状 */
function TopCodesChart({ codes, loading }: { codes: CodePL[]; loading: boolean }) {
  const option = useMemo<EChartsOption>(() => {
    // 横向柱状：echarts y 轴自下而上，倒序让最大值在顶部
    const sorted = [...codes].sort((a, b) => a.profit_loss - b.profit_loss);
    const names = sorted.map((c) => c.name || c.code);
    const values = sorted.map((c) => c.profit_loss);
    return {
      grid: { left: 8, right: 56, top: 8, bottom: 8, containLabel: true },
      xAxis: { ...valueAxis({ formatter: axisWan }), splitLine: { show: false } },
      yAxis: {
        ...categoryAxis(names),
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
          data: values.map((v) => ({ value: v, itemStyle: { color: plColor(v) } })),
          barMaxWidth: 14,
          label: {
            show: true,
            position: 'right',
            formatter: numLabel((v) => formatMoney(v, 0)),
            fontSize: 10,
            color: palette.textTertiary,
          },
        },
      ],
    };
  }, [codes]);

  return (
    <EChart
      option={option}
      loading={loading}
      empty={!loading && codes.length === 0 ? '暂无标的盈亏' : null}
      height={Math.max(260, codes.length * 26)}
      ariaLabel="标的盈亏排行"
    />
  );
}
