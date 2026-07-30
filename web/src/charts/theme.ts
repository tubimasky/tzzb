// 图表共享样式与格式化。
// 配色取自 index.css 的设计 token，保持图表与页面一致（A 股红涨绿跌）。
import type { EChartsOption } from './echarts';
import { formatMoney, formatWan } from '../utils/format';

/** 与 CSS token 对齐的调色板 */
export const palette = {
  up: '#e11d48',
  upSoft: 'rgba(225, 29, 72, 0.12)',
  down: '#059669',
  downSoft: 'rgba(5, 150, 105, 0.12)',
  brand: '#1e3a5f',
  accent: '#2563eb',
  accentSoft: 'rgba(37, 99, 235, 0.14)',
  neutral: '#94a3b8',
  text: '#0f172a',
  textSecondary: '#64748b',
  textTertiary: '#94a3b8',
  border: '#e2e8f0',
  grid: '#eef1f5',
};

/** 饼图/多标的分类色（蓝为主，避免与红绿盈亏语义冲突） */
export const categoryColors = [
  '#2563eb',
  '#f59e0b',
  '#0ea5e9',
  '#64748b',
  '#1e3a5f',
  '#38bdf8',
  '#94a3b8',
  '#0f766e',
  '#4f46e5',
  '#b45309',
  '#0284c7',
  '#475569',
];

const fontFamily =
  "-apple-system, BlinkMacSystemFont, 'SF Pro Text', 'PingFang SC', 'Microsoft YaHei', sans-serif";
const monoFamily = "'SF Mono', 'Roboto Mono', Menlo, Consolas, ui-monospace, monospace";

/** 盈亏取色：正红负绿 */
export function plColor(n: number): string {
  if (!Number.isFinite(n) || n === 0) return palette.neutral;
  return n > 0 ? palette.up : palette.down;
}

/** 万元轴标签（图表纵轴用，避免长数字挤压） */
export function axisWan(n: number): string {
  if (!Number.isFinite(n)) return '';
  if (n === 0) return '0';
  return `${(n / 10000).toFixed(1)}万`;
}

/** 小数比率 → 百分数轴标签（0.0327 → 3.27%） */
export function axisRatioPct(n: number): string {
  if (!Number.isFinite(n)) return '';
  return `${(n * 100).toFixed(1)}%`;
}

/** tooltip 里的金额（带符号） */
export function tipMoney(n: number, signed = false): string {
  const sign = signed && n > 0 ? '+' : '';
  return `${sign}${formatMoney(n)}`;
}

/** tooltip 里的万元 */
export function tipWan(n: number): string {
  return `${formatWan(n)} 万`;
}

/** 小数比率 → 带符号百分数（0.0327 → +3.27%） */
export function tipRatio(n: number, signed = true): string {
  if (!Number.isFinite(n)) return '-';
  const sign = signed && n > 0 ? '+' : '';
  return `${sign}${(n * 100).toFixed(2)}%`;
}

/** 彩色圆点，用于自定义 tooltip 的图例标记 */
export function tipDot(color: string): string {
  return `<span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${color};margin-right:6px;"></span>`;
}

/** 所有图表共享的基础配置（字体、tooltip 外观、网格留白） */
export const baseOption: EChartsOption = {
  textStyle: { fontFamily, color: palette.textSecondary },
  animationDuration: 420,
  grid: {
    left: 8,
    right: 16,
    top: 28,
    bottom: 4,
    containLabel: true,
  },
  tooltip: {
    backgroundColor: 'rgba(15, 23, 42, 0.92)',
    borderWidth: 0,
    padding: [8, 12],
    textStyle: { color: '#e2e8f0', fontSize: 12, fontFamily },
    extraCssText: 'border-radius:8px;box-shadow:0 8px 24px rgba(15,23,42,0.18);',
  },
};

/** 类目轴默认样式 */
export function categoryAxis(data: string[], opts?: { rotate?: number }) {
  return {
    type: 'category' as const,
    data,
    axisLine: { lineStyle: { color: palette.border } },
    axisTick: { show: false },
    axisLabel: {
      color: palette.textTertiary,
      fontSize: 11,
      rotate: opts?.rotate ?? 0,
      fontFamily: monoFamily,
    },
  };
}

/** 数值轴默认样式；formatter 传轴标签格式化函数 */
export function valueAxis(opts?: {
  name?: string;
  formatter?: (n: number) => string;
  position?: 'left' | 'right';
  splitLine?: boolean;
  min?: number;
  max?: number;
}) {
  return {
    type: 'value' as const,
    name: opts?.name,
    nameTextStyle: { color: palette.textTertiary, fontSize: 11, padding: [0, 0, 0, 4] },
    position: opts?.position,
    min: opts?.min,
    max: opts?.max,
    axisLine: { show: false },
    axisTick: { show: false },
    axisLabel: {
      color: palette.textTertiary,
      fontSize: 11,
      fontFamily: monoFamily,
      formatter: opts?.formatter,
    },
    splitLine: {
      show: opts?.splitLine ?? true,
      lineStyle: { color: palette.grid, type: 'solid' as const },
    },
  };
}

/**
 * series.label.formatter 适配器。
 * echarts 回调参数是 CallbackDataParams（value 可能是数组/对象/undefined），
 * 这里收敛成「拿到数字再格式化」，各页不用重复 cast。
 */
export function numLabel(fn: (n: number) => string) {
  return (params: { value?: unknown }) => {
    const v = typeof params.value === 'number' ? params.value : Number(params.value);
    return Number.isFinite(v) ? fn(v) : '';
  };
}

/** 零轴参考线（盈亏类图表用） */
export const zeroMarkLine = {
  silent: true,
  symbol: 'none' as const,
  lineStyle: { color: palette.border, width: 1 },
  label: { show: false },
  data: [{ yAxis: 0 }],
};
