import { useCallback, useEffect, useRef } from 'react';
import { echarts } from '../charts/echarts';
import type { EChartsOption } from '../charts/echarts';
import { baseOption } from '../charts/theme';
import './EChart.css';

type Props = {
  option: EChartsOption;
  /** 图表高度，默认 280 */
  height?: number | string;
  /** 加载中显示骨架 */
  loading?: boolean;
  /** 无数据时显示的文案；给了就渲染空态而不是空白画布 */
  empty?: string | null;
  className?: string;
  /** 无障碍描述，图表是 canvas，屏读需要文字说明 */
  ariaLabel?: string;
};

/**
 * echarts 容器组件：负责实例生命周期与自适应宽度。
 *
 * 画布 div 常驻 DOM，loading / empty 用绝对定位的遮罩盖在上面。
 * 不做「画布 div ↔ 状态 div」的条件切换：那样 echarts 注入的子节点会和
 * React 的节点移除撞车，切换视图时抛 removeChild NotFoundError。
 * 常驻也让实例不必反复建销，切档位时不闪。
 */
export default function EChart({
  option,
  height = 280,
  loading = false,
  empty = null,
  className,
  ariaLabel,
}: Props) {
  const chartRef = useRef<echarts.ECharts | null>(null);
  const roRef = useRef<ResizeObserver | null>(null);
  // 保存最新 option，节点挂载晚于数据到达时也能立刻画上
  const optionRef = useRef(option);
  optionRef.current = option;

  const attach = useCallback((el: HTMLDivElement | null) => {
    if (!el) {
      // 组件销毁
      roRef.current?.disconnect();
      roRef.current = null;
      if (chartRef.current) {
        try {
          chartRef.current.dispose();
        } catch {
          // dispose 可能在某些边界情况下抛异常，忽略
        }
        chartRef.current = null;
      }
      return;
    }

    // 确保旧实例已清理
    if (chartRef.current) {
      try {
        chartRef.current.dispose();
      } catch {
        // ignore
      }
      chartRef.current = null;
    }

    const chart = echarts.init(el, undefined, { renderer: 'canvas' });
    chartRef.current = chart;
    chart.setOption({ ...baseOption, ...optionRef.current }, true);

    // 容器宽度随布局变化（窗口缩放、响应式断点）
    const ro = new ResizeObserver(() => chart.resize());
    ro.observe(el);
    roRef.current = ro;
  }, []);

  // option 变化时全量替换，避免上一次的 series 残留
  useEffect(() => {
    const chart = chartRef.current;
    if (!chart) return;
    chart.setOption({ ...baseOption, ...option }, true);
  }, [option]);

  const style = { height: typeof height === 'number' ? `${height}px` : height };
  const overlay = loading ? (
    <div className="chart-skeleton" aria-busy="true" aria-label="图表加载中" />
  ) : empty ? (
    <span className="chart-empty">{empty}</span>
  ) : null;

  return (
    <div className={`chart-shell${className ? ` ${className}` : ''}`} style={style}>
      <div
        ref={attach}
        className="chart-canvas"
        role="img"
        aria-label={ariaLabel}
        aria-busy={loading || undefined}
      />
      {overlay && <div className="chart-overlay">{overlay}</div>}
    </div>
  );
}
