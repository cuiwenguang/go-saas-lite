<script setup lang="ts">
import * as echarts from "echarts/core";
import { LineChart } from "echarts/charts";
import { GridComponent, LegendComponent, TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import type { EChartsCoreOption } from "echarts/core";

echarts.use([LineChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer]);

const props = defineProps<{ option: EChartsCoreOption; height?: string }>();

const el = ref<HTMLDivElement>();
let chart: echarts.ECharts | null = null;
let ro: ResizeObserver | null = null;

onMounted(() => {
  if (!el.value) return;
  chart = echarts.init(el.value);
  chart.setOption(props.option);
  ro = new ResizeObserver(() => chart?.resize());
  ro.observe(el.value);
});

onBeforeUnmount(() => {
  ro?.disconnect();
  chart?.dispose();
  chart = null;
});

watch(() => props.option, (o) => chart?.setOption(o, true), { deep: true });
</script>

<template>
  <div ref="el" :style="{ height: height || '260px', width: '100%' }" />
</template>
