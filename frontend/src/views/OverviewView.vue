<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, toRefs, watch } from 'vue'
import {
  Activity,
  AlertTriangle,
  BellRing,
  Check,
  ChevronRight,
  Cpu,
  Database,
  Gauge,
  Plus,
  ShieldCheck,
  SlidersHorizontal,
  Thermometer,
  Wifi,
} from 'lucide-vue-next'
import { store, navigate, openDeviceModal, resolveAlarm, selectDevice } from '../store'
import { formatNumber, formatTime, statusClass, statusLabel } from '../helpers'

const { devices, alarms, rules, telemetry, selectedDeviceId, selectedMetric } = toRefs(store)

const onlineDevices = computed(() => devices.value.filter((device) => device.status === 'online').length)
const activeAlarms = computed(() => alarms.value.filter((alarm) => alarm.status === 'active').length)
const selectedDevice = computed(() => devices.value.find((device) => device.device_id === selectedDeviceId.value) || null)
const selectedMetricName = computed(() => {
  if (selectedMetric.value) return selectedMetric.value
  const first = telemetry.value[0]
  return first ? Object.keys(first.values || {})[0] || '' : ''
})

const chartRef = ref(null)
let chartInstance = null
let resizeObserver = null
let echartsLoaded = null

async function loadEcharts() {
  if (!echartsLoaded) {
    echartsLoaded = Promise.all([
      import('echarts/core'),
      import('echarts/charts'),
      import('echarts/components'),
      import('echarts/renderers'),
    ]).then(([core, charts, components, renderers]) => {
      core.use([charts.LineChart, components.GridComponent, components.TooltipComponent, components.GraphicComponent, renderers.CanvasRenderer])
      return core
    })
  }
  return echartsLoaded
}

async function renderChart() {
  if (!chartRef.value) return
  const echarts = await loadEcharts()
  if (!chartInstance) chartInstance = echarts.init(chartRef.value)
  const metric = selectedMetricName.value || 'telemetry'
  const points = telemetry.value.map((sample) => [new Date(sample.timestamp).getTime(), Number(sample.values?.[metric])]).filter((point) => Number.isFinite(point[1]))
  chartInstance.setOption({
    animation: false,
    grid: { top: 18, right: 20, bottom: 32, left: 42 },
    tooltip: { trigger: 'axis', backgroundColor: '#102a2c', borderWidth: 0, textStyle: { color: '#f6f8f5' } },
    xAxis: { type: 'time', axisLine: { lineStyle: { color: '#d8e1df' } }, axisLabel: { color: '#7a8c8d', fontSize: 11 } },
    yAxis: { type: 'value', splitLine: { lineStyle: { color: '#edf1ee' } }, axisLabel: { color: '#7a8c8d', fontSize: 11 } },
    series: [{ type: 'line', smooth: true, showSymbol: false, data: points, lineStyle: { color: '#0d8b83', width: 3 }, areaStyle: { color: 'rgba(13,139,131,0.10)' } }],
    graphic: points.length ? [] : [{ type: 'text', left: 'center', top: 'middle', style: { text: '等待遥测数据', fill: '#9aacaa', fontSize: 14 } }],
  }, true)
}

function resizeChart() {
  chartInstance?.resize()
}

watch([telemetry, selectedMetric], () => nextTick(renderChart), { deep: true })
watch(selectedDeviceId, () => nextTick(renderChart))

onMounted(() => {
  nextTick(renderChart)
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(resizeChart)
    if (chartRef.value) resizeObserver.observe(chartRef.value)
  }
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  chartInstance?.dispose()
})
</script>

<template>
  <section class="content-area">
    <div class="page-intro">
      <div><span class="eyebrow">OPERATIONS / LIVE</span><h1>运行总览</h1><p>园区设备、遥测与告警的实时工作面。</p></div>
      <button class="primary-button" @click="openDeviceModal"><Plus :size="17" />注册设备</button>
    </div>

    <div class="metric-grid">
      <article class="metric-card accent-teal"><div class="metric-label"><span>在线设备</span><Wifi :size="17" /></div><strong>{{ onlineDevices }}<small>/ {{ devices.length }}</small></strong><span class="metric-foot">当前连接状态</span></article>
      <article class="metric-card accent-coral"><div class="metric-label"><span>活跃告警</span><BellRing :size="17" /></div><strong>{{ activeAlarms }}</strong><span class="metric-foot">需要运维关注</span></article>
      <article class="metric-card accent-blue"><div class="metric-label"><span>遥测记录</span><Database :size="17" /></div><strong>{{ formatNumber(telemetry.length) }}</strong><span class="metric-foot">当前设备样本</span></article>
      <article class="metric-card accent-gold"><div class="metric-label"><span>生效规则</span><SlidersHorizontal :size="17" /></div><strong>{{ rules.length }}</strong><span class="metric-foot">产品级策略</span></article>
    </div>

    <div class="overview-grid">
      <section class="panel chart-panel"><div class="panel-heading"><div><span class="panel-kicker">TELEMETRY</span><h2>遥测趋势</h2></div><select v-model="selectedDeviceId" class="compact-select"><option value="">选择设备</option><option v-for="device in devices" :key="device.device_id" :value="device.device_id">{{ device.name || device.device_id }}</option></select></div><div ref="chartRef" class="chart"></div><div class="chart-footer"><span class="legend-line"></span><span>{{ selectedDevice?.name || '未选择设备' }}</span><span class="chart-metric">{{ selectedMetricName || '暂无属性' }}</span></div></section>
      <section class="panel pulse-panel"><div class="panel-heading"><div><span class="panel-kicker">DEVICE PULSE</span><h2>设备脉搏</h2></div><Gauge :size="20" class="heading-icon" /></div><div v-if="devices.length" class="pulse-list"><button v-for="device in devices.slice(0, 5)" :key="device.device_id" class="pulse-row" @click="navigate('devices'); selectDevice(device.device_id)"><span class="pulse-symbol"><Thermometer :size="16" /></span><span class="pulse-copy"><strong>{{ device.name || device.device_id }}</strong><small>{{ device.product_key }}</small></span><span class="status-pill" :class="statusClass(device.status)"><i></i>{{ statusLabel(device.status) }}</span><ChevronRight :size="15" /></button></div><div v-else class="empty-state compact"><Cpu :size="25" /><strong>还没有设备</strong><span>注册首台设备后，状态会显示在这里</span><button class="text-button" @click="openDeviceModal">注册设备 <ChevronRight :size="14" /></button></div></section>
    </div>

    <div class="lower-grid">
      <section class="panel table-panel"><div class="panel-heading"><div><span class="panel-kicker">RECENT DEVICES</span><h2>设备清单</h2></div><button class="ghost-button" @click="navigate('devices')">查看全部 <ChevronRight :size="14" /></button></div><div v-if="devices.length" class="table-wrap"><table><thead><tr><th>设备</th><th>产品</th><th>状态</th><th>最后在线</th></tr></thead><tbody><tr v-for="device in devices.slice(0, 6)" :key="device.device_id" @click="navigate('devices'); selectDevice(device.device_id)"><td><strong>{{ device.name || device.device_id }}</strong><small>{{ device.device_id }}</small></td><td>{{ device.product_key }}</td><td><span class="status-pill" :class="statusClass(device.status)"><i></i>{{ statusLabel(device.status) }}</span></td><td>{{ formatTime(device.last_online) }}</td></tr></tbody></table></div><div v-else class="empty-state"><Cpu :size="28" /><strong>设备清单为空</strong><span>先注册一个产品和设备开始接入</span></div></section>
      <section class="panel alarm-panel"><div class="panel-heading"><div><span class="panel-kicker">ATTENTION</span><h2>待处理告警</h2></div><button class="ghost-button" @click="navigate('alarms')">告警中心 <ChevronRight :size="14" /></button></div><div v-if="activeAlarms" class="alarm-list"><div v-for="alarm in alarms.filter((item) => item.status === 'active').slice(0, 4)" :key="alarm.id" class="alarm-row"><span class="alarm-icon"><AlertTriangle :size="16" /></span><div><strong>{{ alarm.rule_id || '规则告警' }}</strong><small>{{ alarm.device_id }} · {{ formatTime(alarm.triggered_at) }}</small></div><button class="icon-button small" title="解除告警" @click="resolveAlarm(alarm)"><Check :size="15" /></button></div></div><div v-else class="empty-state compact"><ShieldCheck :size="25" /><strong>当前没有活跃告警</strong><span>系统会在规则命中后显示异常</span></div></section>
    </div>
  </section>
</template>
