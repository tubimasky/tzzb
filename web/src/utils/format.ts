/** 金额：两位小数，千分位可选 */
export function formatMoney(n: number, digits = 2): string {
  if (!Number.isFinite(n)) return '-';
  return n.toLocaleString('zh-CN', {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  });
}

/** 金额（万元）：两位小数 */
export function formatWan(n: number): string {
  if (!Number.isFinite(n)) return '-';
  return (n / 10000).toFixed(2);
}

/**
 * 已是百分数的值（后端 rate / hold_rate / day_rate 等）
 * 例：15.2 → "15.20%"；带符号时 → "+15.20%"
 */
export function formatPct(n: number, opts?: { signed?: boolean }): string {
  if (!Number.isFinite(n)) return '-';
  const signed = opts?.signed ?? false;
  const sign = signed && n > 0 ? '+' : '';
  return `${sign}${n.toFixed(2)}%`;
}

/**
 * 小数比率（后端 position_rate 等，0.788 → 78.8%）
 */
export function formatRatio(n: number): string {
  if (!Number.isFinite(n)) return '-';
  return `${(n * 100).toFixed(2)}%`;
}

/** A 股习惯：红涨绿跌 */
export function profitClass(n: number): string {
  if (!Number.isFinite(n) || n === 0) return '';
  return n > 0 ? 'profit-positive' : 'profit-negative';
}

/** YYYYMMDD → YYYY-MM-DD；非 8 位原样返回 */
export function formatDateYMD(s: string | undefined | null): string {
  if (!s) return '—';
  if (s.length !== 8) return s;
  return `${s.slice(0, 4)}-${s.slice(4, 6)}-${s.slice(6, 8)}`;
}

/** 成交方向 */
export function tradeOpLabel(op: string): string {
  if (op === '1') return '买入';
  if (op === '2') return '卖出';
  return op || '—';
}
