// echarts 按需注册入口。
// 只引 core + 实际用到的图表/组件，避免整包引入（echarts 全量 ~1MB）。
// 页面统一从这里 import echarts，不要直接 import 'echarts'。
import * as echarts from 'echarts/core';
import { BarChart, LineChart, PieChart, ScatterChart } from 'echarts/charts';
import {
  DataZoomComponent,
  GridComponent,
  LegendComponent,
  MarkLineComponent,
  TitleComponent,
  TooltipComponent,
} from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';

echarts.use([
  BarChart,
  LineChart,
  PieChart,
  ScatterChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  TitleComponent,
  DataZoomComponent,
  MarkLineComponent,
  CanvasRenderer,
]);

export { echarts };
export type { EChartsOption } from 'echarts';
