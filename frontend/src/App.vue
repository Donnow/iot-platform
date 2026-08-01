<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import * as echarts from 'echarts'
import {
  Activity,
  AlertTriangle,
  BellRing,
  Check,
  ChevronRight,
  CircleHelp,
  Cpu,
  Database,
  Gauge,
  KeyRound,
  LayoutDashboard,
  Menu,
  Package,
  Plus,
  RefreshCw,
  Search,
  Send,
  Settings2,
  ShieldCheck,
  SlidersHorizontal,
  Thermometer,
  Trash2,
  Upload,
  Wifi,
  Workflow,
  X,
} from 'lucide-vue-next'
import { api, getAuthToken, itemsOf, setAuthToken } from './api'

const navItems = [
  { id: 'overview', label: '运行总览', icon: LayoutDashboard },
  { id: 'devices', label: '设备管理', icon: Cpu },
  { id: 'alarms', label: '告警中心', icon: BellRing },
  { id: 'rules', label: '规则配置', icon: Workflow },
  { id: 'products', label: '产品与物模型', icon: Package },
  { id: 'ota', label: '固件与 OTA', icon: Upload },
]

const activeView = ref('overview')
const sidebarOpen = ref(false)
const loading = ref(false)
const refreshing = ref(false)
const connected = ref(false)
const errorMessage = ref('')
const toastMessage = ref('')
const authRequired = ref(false)
const tokenInput = ref(getAuthToken())
const products = ref([])
const devices = ref([])
const alarms = ref([])
const rules = ref([])
const firmwares = ref([])
const otaTasks = ref([])
const telemetry = ref([])
const deviceShadow = ref(null)
const desiredInput = ref('{}')
const commandStatus = ref(null)
const commandPolling = ref(false)
const selectedDeviceId = ref('')
const selectedMetric = ref('')
const deviceSearch = ref('')
const deviceStatusFilter = ref('')
const alarmStatusFilter = ref('')
const chartRef = ref(null)
let chartInstance = null
let resizeObserver = null
let toastTimer = null
let commandPollTimer = null
let mqttClient = null

const showDeviceModal = ref(false)
const showProductModal = ref(false)
const showRuleModal = ref(false)
const showCommandModal = ref(false)
const showFirmwareModal = ref(false)
const showOTAModal = ref(false)
const revealedSecret = ref('')
const commandDevice = ref(null)

const deviceForm = reactive({ device_id: '', product_key: '', name: '', description: '' })
const productForm = reactive({ product_key: '', name: '', device_type: 'sensor', description: '', properties: [] })
const ruleForm = reactive({ product_key: '', name: '', property_name: '', operator: '>', threshold: 40, duration_seconds: 0, action_type: 'alarm' })
const commandForm = reactive({ method: 'open', params: '{}' })
const firmwareForm = reactive({ product_key: '', version: '', md5: '', file_url: '', changelog: '' })
const otaForm = reactive({ product_key: '', firmware_id: '', target: 'all', target_device_ids: '' })

const selectedDevice = computed(() => devices.value.find((device) => device.device_id === selectedDeviceId.value) || null)
const onlineDevices = computed(() => devices.value.filter((device) => device.status === 'online').length)
const activeAlarms = computed(() => alarms.value.filter((alarm) => alarm.status === 'active').length)
const filteredDevices = computed(() => devices.value.filter((device) => {
  const matchesSearch = !deviceSearch.value || `${device.name} ${device.device_id} ${device.product_key}`.toLowerCase().includes(deviceSearch.value.toLowerCase())
  const matchesStatus = !deviceStatusFilter.value || device.status === deviceStatusFilter.value
  return matchesSearch && matchesStatus
}))
const filteredAlarms = computed(() => alarms.value.filter((alarm) => !alarmStatusFilter.value || alarm.status === alarmStatusFilter.value))
const selectedMetricName = computed(() => {
  if (selectedMetric.value) return selectedMetric.value
  const first = telemetry.value[0]
  return first ? Object.keys(first.values || {})[0] || '' : ''
})
const healthLabel = computed(() => {
  if (loading.value) return '正在同步'
  if (connected.value) return '平台在线'
  return '等待连接'
})
const mqttRealtimeEnabled = computed(() => Boolean(import.meta.env.VITE_MQTT_WS_URL))
const selectedProduct = computed(() => products.value.find((product) => product.product_key === productForm.product_key) || null)

function showToast(message) {
  toastMessage.value = message
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toastMessage.value = '' }, 3400)
}

function statusLabel(status) {
  return ({ online: '在线', offline: '离线', inactive: '未激活', deleted: '已删除', active: '处理中', resolved: '已解除', pending: '等待中', success: '成功', failed: '失败', timeout: '超时' })[status] || status || '未知'
}

function statusClass(status) {
  return `status-${status || 'unknown'}`
}

function formatTime(value) {
  if (!value) return '暂无记录'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '暂无记录'
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(date)
}

function formatNumber(value) {
  return new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 1 }).format(value || 0)
}

function payloadItems(payload) {
  return itemsOf(payload)
}

async function loadData({ silent = false } = {}) {
  if (!silent) loading.value = true
  else refreshing.value = true
  errorMessage.value = ''
  try {
    const [productPayload, devicePayload, alarmPayload, firmwarePayload, otaPayload] = await Promise.all([
      api.get('/api/products?page=1&page_size=100'),
      api.get('/api/devices?page=1&page_size=100'),
      api.get('/api/alarms?page=1&page_size=100'),
      api.get('/api/firmwares'),
      api.get('/api/ota/tasks'),
    ])
    products.value = payloadItems(productPayload)
    devices.value = payloadItems(devicePayload)
    alarms.value = payloadItems(alarmPayload)
    firmwares.value = payloadItems(firmwarePayload)
    otaTasks.value = payloadItems(otaPayload)
    if (!selectedDeviceId.value || !devices.value.some((device) => device.device_id === selectedDeviceId.value)) {
      selectedDeviceId.value = devices.value[0]?.device_id || ''
    }
    const rulePayloads = await Promise.all(products.value.map((product) => api.get(`/api/rules?product_key=${encodeURIComponent(product.product_key)}`)))
    rules.value = rulePayloads.flatMap(payloadItems)
    if (selectedDeviceId.value) {
      await Promise.all([loadTelemetry(selectedDeviceId.value), loadShadow(selectedDeviceId.value)])
    }
    connected.value = true
  } catch (error) {
    connected.value = false
    if (error.status === 401) {
      authRequired.value = true
      errorMessage.value = '请输入平台访问令牌'
    } else {
      errorMessage.value = error.message || '平台暂时不可用'
    }
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

async function loadTelemetry(deviceId) {
  if (!deviceId) {
    telemetry.value = []
    return
  }
  try {
    const payload = await api.get(`/api/devices/${encodeURIComponent(deviceId)}/telemetry?limit=200`)
    telemetry.value = payloadItems(payload)
  } catch (error) {
    if (error.status === 401) authRequired.value = true
    telemetry.value = []
  }
}

async function loadShadow(deviceId) {
  if (!deviceId) {
    deviceShadow.value = null
    desiredInput.value = '{}'
    return
  }
  try {
    const shadow = await api.get(`/api/devices/${encodeURIComponent(deviceId)}/shadow`)
    deviceShadow.value = shadow
    desiredInput.value = JSON.stringify(shadow?.desired || {}, null, 2)
  } catch (error) {
    if (error.status === 401) authRequired.value = true
    deviceShadow.value = null
  }
}

function selectDevice(deviceId) {
  selectedDeviceId.value = deviceId
  Promise.all([loadTelemetry(deviceId), loadShadow(deviceId)])
}

function navigate(view) {
  activeView.value = view
  sidebarOpen.value = false
}

function resetDeviceForm() {
  Object.assign(deviceForm, { device_id: '', product_key: products.value[0]?.product_key || '', name: '', description: '' })
  revealedSecret.value = ''
}

function resetProductForm() {
  Object.assign(productForm, { product_key: '', name: '', device_type: 'sensor', description: '', properties: [] })
}

function openProductModal() {
  resetProductForm()
  showProductModal.value = true
}

function addProperty() {
  productForm.properties.push({ name: '', data_type: 'float', unit: '', min_value: '', max_value: '' })
}

function removeProperty(index) {
  productForm.properties.splice(index, 1)
}

function openDeviceModal() {
  resetDeviceForm()
  showDeviceModal.value = true
}

async function createDevice() {
  if (!deviceForm.product_key || !deviceForm.name) return showToast('请填写设备名称和产品')
  try {
    const response = await api.post('/api/devices', deviceForm)
    revealedSecret.value = response.device_secret || ''
    showToast('设备已注册')
    await loadData({ silent: true })
  } catch (error) {
    showToast(error.message)
  }
}

async function createProduct() {
  if (!productForm.name) return showToast('请填写产品名称')
  try {
    const properties = productForm.properties.filter((property) => property.name.trim()).map((property) => ({
      ...property,
      min_value: property.min_value === '' ? undefined : Number(property.min_value),
      max_value: property.max_value === '' ? undefined : Number(property.max_value),
    }))
    await api.post('/api/products', { ...productForm, properties })
    showProductModal.value = false
    showToast('产品已创建')
    await loadData({ silent: true })
  } catch (error) {
    showToast(error.message)
  }
}

async function createRule() {
  if (!ruleForm.product_key || !ruleForm.name || !ruleForm.property_name) return showToast('请补全规则字段')
  try {
    await api.post('/api/rules', { ...ruleForm, threshold: Number(ruleForm.threshold), duration_seconds: Number(ruleForm.duration_seconds) })
    showRuleModal.value = false
    showToast('规则已启用')
    await loadData({ silent: true })
  } catch (error) {
    showToast(error.message)
  }
}

function openCommand(device) {
  commandDevice.value = device
  Object.assign(commandForm, { method: device.product_key.includes('air') ? 'setTemp' : 'open', params: '{}' })
  showCommandModal.value = true
  commandStatus.value = null
}

async function sendCommand() {
  if (!commandDevice.value) return
  let params
  try {
    params = JSON.parse(commandForm.params || '{}')
  } catch {
    return showToast('Params 必须是合法 JSON')
  }
  try {
    const command = await api.post(`/api/devices/${encodeURIComponent(commandDevice.value.device_id)}/commands`, { method: commandForm.method, params })
    showCommandModal.value = false
    commandStatus.value = command
    startCommandPolling(commandDevice.value.device_id, command.command_id)
    showToast('指令已下发')
    await loadData({ silent: true })
  } catch (error) {
    showToast(error.message)
  }
}

function startCommandPolling(deviceId, commandId) {
  clearInterval(commandPollTimer)
  commandPolling.value = true
  const poll = async () => {
    try {
      const status = await api.get(`/api/devices/${encodeURIComponent(deviceId)}/commands/${encodeURIComponent(commandId)}`)
      commandStatus.value = status
      if (status.status !== 'pending') {
        commandPolling.value = false
        clearInterval(commandPollTimer)
      }
    } catch {
      commandPolling.value = false
      clearInterval(commandPollTimer)
    }
  }
  poll()
  commandPollTimer = setInterval(poll, 1000)
}

async function deleteDevice(device) {
  if (!window.confirm(`确认删除设备 ${device.name || device.device_id}？`)) return
  try {
    await api.delete(`/api/devices/${encodeURIComponent(device.device_id)}`)
    if (selectedDeviceId.value === device.device_id) selectedDeviceId.value = ''
    showToast('设备已删除')
    await loadData({ silent: true })
  } catch (error) {
    showToast(error.message)
  }
}

async function saveDesired() {
  if (!selectedDevice.value) return
  let desired
  try {
    desired = JSON.parse(desiredInput.value || '{}')
  } catch {
    return showToast('desired 必须是合法 JSON')
  }
  if (!desired || Array.isArray(desired)) return showToast('desired 必须是 JSON 对象')
  try {
    deviceShadow.value = await api.put(`/api/devices/${encodeURIComponent(selectedDevice.value.device_id)}/shadow/desired`, desired)
    desiredInput.value = JSON.stringify(deviceShadow.value.desired || {}, null, 2)
    showToast('期望状态已保存')
  } catch (error) {
    showToast(error.message)
  }
}

function openFirmwareModal() {
  Object.assign(firmwareForm, { product_key: products.value[0]?.product_key || '', version: '', md5: '', file_url: '', changelog: '' })
  showFirmwareModal.value = true
}

async function createFirmware() {
  if (!firmwareForm.product_key || !firmwareForm.version || !firmwareForm.md5 || !firmwareForm.file_url) return showToast('请补全固件字段')
  try {
    await api.post('/api/firmwares', { ...firmwareForm })
    showFirmwareModal.value = false
    showToast('固件元数据已登记')
    await loadData({ silent: true })
  } catch (error) {
    showToast(error.message)
  }
}

function openOTAModal() {
  Object.assign(otaForm, { product_key: products.value[0]?.product_key || '', firmware_id: '', target: 'all', target_device_ids: '' })
  showOTAModal.value = true
}

async function createOTATask() {
  if (!otaForm.product_key || !otaForm.firmware_id) return showToast('请选择产品和固件')
  const targetDeviceIds = otaForm.target === 'all' ? [] : otaForm.target_device_ids.split(/[\s,]+/).map((value) => value.trim()).filter(Boolean)
  if (otaForm.target === 'devices' && !targetDeviceIds.length) return showToast('请填写目标设备 ID')
  try {
    await api.post('/api/ota/tasks', { product_key: otaForm.product_key, firmware_id: otaForm.firmware_id, target: otaForm.target, target_device_ids: targetDeviceIds })
    showOTAModal.value = false
    showToast('OTA 任务已创建')
    await loadData({ silent: true })
  } catch (error) {
    showToast(error.message)
  }
}

function onRealtimeMessage(topic, payload) {
  const parts = topic.split('/')
  if (parts.length < 4 || parts[0] !== 'devices') return
  const device = devices.value.find((item) => item.device_id === parts[2])
  if (!device) return
  try {
    const event = JSON.parse(payload.toString())
    if (parts[3] === 'status' && event.status) device.status = event.status
    if (parts[3] === 'event' && event.status) device.status = event.status
  } catch {
    // Ignore malformed realtime messages; HTTP refresh remains authoritative.
  }
}

function connectRealtime() {
  const url = import.meta.env.VITE_MQTT_WS_URL
  if (!url) return
  import('mqtt').then(({ connect }) => {
    mqttClient = connect(url, {
      username: import.meta.env.VITE_MQTT_USERNAME,
      password: import.meta.env.VITE_MQTT_PASSWORD,
      clientId: `console-${Math.random().toString(16).slice(2)}`,
      clean: true,
    })
    mqttClient.on('connect', () => {
      connected.value = true
      mqttClient.subscribe(import.meta.env.VITE_MQTT_STATUS_TOPIC || 'devices/+/+/status')
      mqttClient.subscribe(import.meta.env.VITE_MQTT_EVENT_TOPIC || 'devices/+/+/event')
    })
    mqttClient.on('message', onRealtimeMessage)
    mqttClient.on('error', () => { mqttClient?.end(true) })
  }).catch(() => {})
}

async function resolveAlarm(alarm) {
  try {
    await api.put(`/api/alarms/${encodeURIComponent(alarm.id)}/resolve`, { note: '已由运维台确认处理' })
    showToast('告警已解除')
    await loadData({ silent: true })
  } catch (error) {
    showToast(error.message)
  }
}

function submitToken() {
  setAuthToken(tokenInput.value)
  authRequired.value = false
  loadData()
}

function clearToken() {
  tokenInput.value = ''
  setAuthToken('')
  authRequired.value = true
}

function renderChart() {
  if (!chartRef.value) return
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
  loadData()
  connectRealtime()
  nextTick(renderChart)
  resizeObserver = new ResizeObserver(resizeChart)
  if (chartRef.value) resizeObserver.observe(chartRef.value)
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  chartInstance?.dispose()
  clearTimeout(toastTimer)
  clearInterval(commandPollTimer)
  mqttClient?.end(true)
})
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar" :class="{ 'sidebar-open': sidebarOpen }">
      <div class="brand-lockup">
        <div class="brand-mark"><Activity :size="20" /></div>
        <div>
          <strong>PERFORM</strong>
          <span>园区设备运维台</span>
        </div>
      </div>
      <nav class="main-nav" aria-label="主导航">
        <button v-for="item in navItems" :key="item.id" class="nav-item" :class="{ active: activeView === item.id }" @click="navigate(item.id)">
          <component :is="item.icon" :size="17" />
          <span>{{ item.label }}</span>
          <span v-if="item.id === 'alarms' && activeAlarms" class="nav-count">{{ activeAlarms }}</span>
        </button>
      </nav>
      <div class="sidebar-bottom">
        <div class="broker-status">
          <span class="status-dot" :class="connected ? 'dot-online' : 'dot-idle'"></span>
          <div><strong>{{ healthLabel }}</strong><span>数据服务</span></div>
        </div>
        <button class="nav-item muted" title="系统设置"><Settings2 :size="17" /><span>系统设置</span></button>
      </div>
    </aside>

    <main class="workspace">
      <header class="topbar">
        <button class="icon-button mobile-menu" title="打开导航" @click="sidebarOpen = !sidebarOpen"><Menu :size="19" /></button>
        <div class="breadcrumb"><span>园区运维</span><ChevronRight :size="15" /><strong>{{ navItems.find((item) => item.id === activeView)?.label }}</strong></div>
        <div class="topbar-actions">
          <span class="sync-time">{{ refreshing ? '正在刷新' : '刚刚同步' }}</span>
          <button class="icon-button" title="刷新数据" :disabled="refreshing" @click="loadData({ silent: true })"><RefreshCw :size="17" :class="{ spin: refreshing }" /></button>
          <button class="icon-button" title="访问令牌" @click="authRequired = true"><KeyRound :size="17" /></button>
          <div class="user-chip"><span class="avatar">OP</span><span>运维账户</span></div>
        </div>
      </header>

      <div v-if="errorMessage" class="connection-banner"><CircleHelp :size="17" /><span>{{ errorMessage }}</span><button class="text-button" @click="authRequired = true">配置连接</button></div>

      <section v-if="activeView === 'overview'" class="content-area">
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

      <section v-else-if="activeView === 'devices'" class="content-area">
        <div class="page-intro"><div><span class="eyebrow">ASSET / DEVICES</span><h1>设备管理</h1><p>管理设备身份、连接状态、影子和远程操作。</p></div><button class="primary-button" @click="openDeviceModal"><Plus :size="17" />注册设备</button></div>
        <div class="toolbar"><div class="search-field"><Search :size="16" /><input v-model="deviceSearch" placeholder="搜索设备名称、ID 或产品" /></div><select v-model="deviceStatusFilter" class="filter-select"><option value="">全部状态</option><option value="online">在线</option><option value="offline">离线</option><option value="inactive">未激活</option></select><span class="toolbar-count">{{ filteredDevices.length }} 台设备</span></div>
        <div class="split-layout"><section class="panel table-panel"><div class="table-wrap"><table><thead><tr><th>设备</th><th>产品</th><th>状态</th><th>最后在线</th><th class="align-right">操作</th></tr></thead><tbody><tr v-for="device in filteredDevices" :key="device.device_id" :class="{ selected: selectedDeviceId === device.device_id }" @click="selectDevice(device.device_id)"><td><strong>{{ device.name || device.device_id }}</strong><small>{{ device.device_id }}</small></td><td>{{ device.product_key }}</td><td><span class="status-pill" :class="statusClass(device.status)"><i></i>{{ statusLabel(device.status) }}</span></td><td>{{ formatTime(device.last_online) }}</td><td class="align-right"><button class="icon-button small" title="发送指令" @click.stop="openCommand(device)"><Send :size="15" /></button><button class="icon-button small danger-icon" title="删除设备" @click.stop="deleteDevice(device)"><Trash2 :size="15" /></button></td></tr></tbody></table></div><div v-if="!filteredDevices.length" class="empty-state"><Search :size="28" /><strong>没有匹配设备</strong><span>调整筛选条件后再试</span></div></section><aside class="panel device-detail"><div class="panel-heading"><div><span class="panel-kicker">SELECTED DEVICE</span><h2>{{ selectedDevice?.name || '设备详情' }}</h2></div><span v-if="selectedDevice" class="status-pill" :class="statusClass(selectedDevice.status)"><i></i>{{ statusLabel(selectedDevice.status) }}</span></div><div v-if="selectedDevice" class="detail-content"><dl><div><dt>设备 ID</dt><dd>{{ selectedDevice.device_id }}</dd></div><div><dt>产品</dt><dd>{{ selectedDevice.product_key }}</dd></div><div><dt>创建时间</dt><dd>{{ formatTime(selectedDevice.created_at) }}</dd></div></dl><div class="detail-actions detail-actions-grid"><button class="primary-button" @click="openCommand(selectedDevice)"><Send :size="16" />发送指令</button><button class="ghost-button danger-button" @click="deleteDevice(selectedDevice)"><Trash2 :size="15" />删除设备</button></div><div class="state-block"><div class="state-heading"><div><span class="panel-kicker">DEVICE SHADOW</span><strong>影子状态</strong></div><button class="icon-button small" title="刷新影子" @click="loadShadow(selectedDevice.device_id)"><RefreshCw :size="14" /></button></div><textarea v-model="desiredInput" rows="4" spellcheck="false" aria-label="期望状态 JSON"></textarea><button class="ghost-button full" @click="saveDesired"><Check :size="14" />保存 desired</button><div v-if="deviceShadow" class="shadow-values"><span><b>reported</b><code>{{ JSON.stringify(deviceShadow.reported || {}) }}</code></span><span><b>delta</b><code>{{ JSON.stringify(deviceShadow.delta || {}) }}</code></span></div></div><div v-if="commandStatus" class="command-status"><span class="panel-kicker">LAST COMMAND</span><strong>{{ commandStatus.method }} · {{ statusLabel(commandStatus.status) }}</strong><small>{{ commandStatus.message || (commandPolling ? '等待设备回复' : '暂无回复') }}</small></div></div><div v-else class="empty-state compact"><Cpu :size="25" /><strong>选择一台设备</strong><span>详情、影子和操作会在这里展开</span></div></aside></div>
      </section>

      <section v-else-if="activeView === 'alarms'" class="content-area">
        <div class="page-intro"><div><span class="eyebrow">ATTENTION / ALERTS</span><h1>告警中心</h1><p>跟踪异常状态并留下处理结果。</p></div><div class="page-stat"><strong>{{ activeAlarms }}</strong><span>活跃告警</span></div></div>
        <div class="toolbar"><div class="filter-tabs"><button :class="{ active: alarmStatusFilter === '' }" @click="alarmStatusFilter = ''">全部</button><button :class="{ active: alarmStatusFilter === 'active' }" @click="alarmStatusFilter = 'active'">处理中</button><button :class="{ active: alarmStatusFilter === 'resolved' }" @click="alarmStatusFilter = 'resolved'">已解除</button></div><span class="toolbar-count">{{ filteredAlarms.length }} 条记录</span></div>
        <section class="panel table-panel"><div class="table-wrap"><table><thead><tr><th>状态</th><th>设备</th><th>规则</th><th>触发值</th><th>触发时间</th><th class="align-right">处理</th></tr></thead><tbody><tr v-for="alarm in filteredAlarms" :key="alarm.id"><td><span class="status-pill" :class="statusClass(alarm.status)"><i></i>{{ statusLabel(alarm.status) }}</span></td><td><strong>{{ alarm.device_id }}</strong></td><td>{{ alarm.rule_id || '规则' }}</td><td class="value-cell">{{ formatNumber(alarm.trigger_value) }}</td><td>{{ formatTime(alarm.triggered_at) }}</td><td class="align-right"><button v-if="alarm.status === 'active'" class="ghost-button compact-action" @click="resolveAlarm(alarm)"><Check :size="14" />解除</button><span v-else class="muted-text">{{ formatTime(alarm.resolved_at) }}</span></td></tr></tbody></table></div><div v-if="!filteredAlarms.length" class="empty-state"><ShieldCheck :size="30" /><strong>没有告警记录</strong><span>规则命中后会在这里留下记录</span></div></section>
      </section>

      <section v-else-if="activeView === 'rules'" class="content-area">
        <div class="page-intro"><div><span class="eyebrow">AUTOMATION / RULES</span><h1>规则配置</h1><p>为产品定义实时阈值和告警策略。</p></div><button class="primary-button" @click="showRuleModal = true"><Plus :size="17" />新建规则</button></div>
        <section class="panel table-panel"><div class="table-wrap"><table><thead><tr><th>规则</th><th>产品</th><th>条件</th><th>持续时间</th><th>动作</th><th>状态</th></tr></thead><tbody><tr v-for="rule in rules" :key="rule.id"><td><strong>{{ rule.name }}</strong><small>{{ rule.id }}</small></td><td>{{ rule.product_key }}</td><td class="condition-cell"><span>{{ rule.property_name }}</span><b>{{ rule.operator }}</b><span>{{ rule.threshold }}</span></td><td>{{ rule.duration_seconds || 0 }} 秒</td><td>{{ rule.action_type === 'alarm' ? '生成告警' : '下发指令' }}</td><td><span class="status-pill" :class="rule.enabled ? 'status-online' : 'status-inactive'"><i></i>{{ rule.enabled ? '已启用' : '已停用' }}</span></td></tr></tbody></table></div><div v-if="!rules.length" class="empty-state"><SlidersHorizontal :size="30" /><strong>还没有规则</strong><span>创建第一条阈值策略</span><button class="text-button" @click="showRuleModal = true">新建规则 <ChevronRight :size="14" /></button></div></section>
      </section>

      <section v-else-if="activeView === 'products'" class="content-area">
        <div class="page-intro"><div><span class="eyebrow">CATALOG / MODELS</span><h1>产品与物模型</h1><p>产品定义设备类型和遥测属性边界。</p></div><button class="primary-button" @click="openProductModal"><Plus :size="17" />新建产品</button></div>
        <div class="product-grid"><article v-for="product in products" :key="product.product_key" class="product-card"><div class="product-card-top"><span class="product-icon"><Package :size="18" /></span><span class="status-pill status-online"><i></i>已接入</span></div><h2>{{ product.name }}</h2><code>{{ product.product_key }}</code><p>{{ product.description || '暂无产品描述' }}</p><div class="property-list"><span v-for="property in (product.properties || []).slice(0, 4)" :key="property.name">{{ property.name }} <small>{{ property.data_type }}</small></span><span v-if="!product.properties?.length" class="muted-text">未定义属性</span></div><div class="product-card-foot"><span>{{ devices.filter((device) => device.product_key === product.product_key).length }} 台设备</span><span>{{ rules.filter((rule) => rule.product_key === product.product_key).length }} 条规则</span></div></article></div><div v-if="!products.length" class="empty-state panel"><Package :size="30" /><strong>还没有产品</strong><span>先创建产品，再注册设备并定义数据边界</span><button class="primary-button" @click="openProductModal"><Plus :size="16" />新建产品</button></div>
      </section>

      <section v-else class="content-area">
        <div class="page-intro"><div><span class="eyebrow">RELEASE / OTA</span><h1>固件与 OTA</h1><p>登记固件、创建升级任务并追踪设备阶段。</p></div><div class="page-actions"><button class="ghost-button" @click="openFirmwareModal"><Upload :size="16" />登记固件</button><button class="primary-button" @click="openOTAModal"><Plus :size="17" />创建任务</button></div></div>
        <div class="ota-summary"><article class="metric-card accent-blue"><div class="metric-label"><span>固件版本</span><Upload :size="17" /></div><strong>{{ firmwares.length }}</strong><span class="metric-foot">已登记元数据</span></article><article class="metric-card accent-teal"><div class="metric-label"><span>升级任务</span><Workflow :size="17" /></div><strong>{{ otaTasks.length }}</strong><span class="metric-foot">全部产品任务</span></article><article class="metric-card accent-gold"><div class="metric-label"><span>进行中设备</span><Activity :size="17" /></div><strong>{{ otaTasks.reduce((total, task) => total + (task.summary?.pending || 0) + (task.summary?.downloading || 0) + (task.summary?.installing || 0), 0) }}</strong><span class="metric-foot">等待或执行中</span></article></div>
        <section class="panel table-panel"><div class="panel-heading"><div><span class="panel-kicker">FIRMWARE CATALOG</span><h2>固件元数据</h2></div><span class="toolbar-count">{{ firmwares.length }} 个版本</span></div><div class="table-wrap"><table><thead><tr><th>产品</th><th>版本</th><th>MD5</th><th>文件地址</th><th>登记时间</th></tr></thead><tbody><tr v-for="firmware in firmwares" :key="firmware.id"><td><strong>{{ firmware.product_key }}</strong></td><td>{{ firmware.version }}</td><td><code>{{ firmware.md5 }}</code></td><td class="truncate-cell">{{ firmware.file_url }}</td><td>{{ formatTime(firmware.created_at) }}</td></tr></tbody></table></div><div v-if="!firmwares.length" class="empty-state"><Upload :size="28" /><strong>还没有固件</strong><span>先登记一份固件元数据</span></div></section>
        <section class="panel table-panel ota-table"><div class="panel-heading"><div><span class="panel-kicker">DEPLOYMENT TASKS</span><h2>升级任务</h2></div><span class="toolbar-count">{{ otaTasks.length }} 个任务</span></div><div class="table-wrap"><table><thead><tr><th>任务</th><th>产品 / 版本</th><th>目标设备</th><th>阶段统计</th><th>更新时间</th></tr></thead><tbody><tr v-for="task in otaTasks" :key="task.task_id"><td><strong>{{ task.task_id }}</strong><small>{{ formatTime(task.created_at) }}</small></td><td>{{ task.product_key }} / {{ task.version }}</td><td>{{ task.target_device_ids?.length || 0 }} 台</td><td><span class="stage-summary"><span v-for="stage in ['pending', 'downloading', 'installing', 'success', 'failed']" :key="stage" v-if="task.summary?.[stage]"><i :class="statusClass(stage)"></i>{{ statusLabel(stage) }} {{ task.summary[stage] }}</span></span></td><td>{{ formatTime(task.updated_at) }}</td></tr></tbody></table></div><div v-if="!otaTasks.length" class="empty-state"><Workflow :size="28" /><strong>还没有升级任务</strong><span>创建任务后会在这里追踪进度</span></div></section>
      </section>
    </main>

    <div v-if="showDeviceModal" class="modal-backdrop" @click.self="showDeviceModal = false"><section class="modal"><div class="modal-heading"><div><span class="panel-kicker">NEW DEVICE</span><h2>注册设备</h2></div><button class="icon-button" title="关闭" @click="showDeviceModal = false"><X :size="18" /></button></div><div v-if="revealedSecret" class="secret-callout"><KeyRound :size="18" /><div><strong>设备密钥只显示这一次</strong><code>{{ revealedSecret }}</code></div><button class="icon-button small" title="关闭密钥提示" @click="showDeviceModal = false"><Check :size="16" /></button></div><div class="form-grid"><label>设备名称<input v-model="deviceForm.name" placeholder="例如：一号楼温湿度 01" /></label><label>所属产品<select v-model="deviceForm.product_key"><option value="" disabled>选择产品</option><option v-for="product in products" :key="product.product_key" :value="product.product_key">{{ product.name }}</option></select></label><label>设备 ID（可选）<input v-model="deviceForm.device_id" placeholder="系统自动生成" /></label><label>描述（可选）<input v-model="deviceForm.description" placeholder="位置或用途" /></label></div><div class="modal-actions"><button class="ghost-button" @click="showDeviceModal = false">取消</button><button class="primary-button" @click="createDevice"><Plus :size="16" />创建设备</button></div></section></div>

    <div v-if="showProductModal" class="modal-backdrop" @click.self="showProductModal = false"><section class="modal modal-wide"><div class="modal-heading"><div><span class="panel-kicker">NEW PRODUCT</span><h2>新建产品</h2></div><button class="icon-button" title="关闭" @click="showProductModal = false"><X :size="18" /></button></div><div class="form-grid"><label>产品名称<input v-model="productForm.name" placeholder="例如：温湿度传感器" /></label><label>设备类型<select v-model="productForm.device_type"><option value="sensor">传感器</option><option value="actuator">执行器</option><option value="composite">复合设备</option></select></label><label>产品 Key（可选）<input v-model="productForm.product_key" placeholder="系统自动生成" /></label><label>描述（可选）<input v-model="productForm.description" placeholder="产品用途" /></label></div><div class="property-editor"><div class="state-heading"><div><span class="panel-kicker">THING MODEL</span><strong>属性定义</strong></div><button class="ghost-button compact-action" @click="addProperty"><Plus :size="14" />添加属性</button></div><div v-for="(property, index) in productForm.properties" :key="index" class="property-row"><input v-model="property.name" placeholder="属性名" /><select v-model="property.data_type"><option value="float">float</option><option value="int">int</option><option value="bool">bool</option><option value="string">string</option></select><input v-model="property.unit" placeholder="单位" /><input v-model="property.min_value" type="number" placeholder="最小值" /><input v-model="property.max_value" type="number" placeholder="最大值" /><button class="icon-button small danger-icon" title="移除属性" @click="removeProperty(index)"><Trash2 :size="14" /></button></div><div v-if="!productForm.properties.length" class="editor-empty">可选。添加属性后，平台会校验设备遥测类型和范围。</div></div><div class="modal-actions"><button class="ghost-button" @click="showProductModal = false">取消</button><button class="primary-button" @click="createProduct"><Plus :size="16" />创建产品</button></div></section></div>

    <div v-if="showRuleModal" class="modal-backdrop" @click.self="showRuleModal = false"><section class="modal"><div class="modal-heading"><div><span class="panel-kicker">NEW RULE</span><h2>新建规则</h2></div><button class="icon-button" title="关闭" @click="showRuleModal = false"><X :size="18" /></button></div><div class="form-grid"><label>绑定产品<select v-model="ruleForm.product_key"><option value="" disabled>选择产品</option><option v-for="product in products" :key="product.product_key" :value="product.product_key">{{ product.name }}</option></select></label><label>规则名称<input v-model="ruleForm.name" placeholder="例如：高温告警" /></label><label>属性名称<input v-model="ruleForm.property_name" placeholder="temperature" /></label><label>比较运算<select v-model="ruleForm.operator"><option v-for="operator in ['>', '<', '>=', '<=', '==', '!=']" :key="operator" :value="operator">{{ operator }}</option></select></label><label>阈值<input v-model="ruleForm.threshold" type="number" /></label><label>持续时间（秒）<input v-model="ruleForm.duration_seconds" type="number" min="0" /></label></div><div class="modal-actions"><button class="ghost-button" @click="showRuleModal = false">取消</button><button class="primary-button" @click="createRule"><Check :size="16" />启用规则</button></div></section></div>

    <div v-if="showCommandModal" class="modal-backdrop" @click.self="showCommandModal = false"><section class="modal"><div class="modal-heading"><div><span class="panel-kicker">REMOTE COMMAND</span><h2>发送指令</h2></div><button class="icon-button" title="关闭" @click="showCommandModal = false"><X :size="18" /></button></div><div class="command-target"><span class="product-icon"><Send :size="17" /></span><div><strong>{{ commandDevice?.name || commandDevice?.device_id }}</strong><small>{{ commandDevice?.device_id }}</small></div></div><div class="form-grid"><label>指令方法<input v-model="commandForm.method" placeholder="open" /></label><label class="span-two">参数 JSON<textarea v-model="commandForm.params" rows="5" spellcheck="false"></textarea></label></div><div class="modal-actions"><button class="ghost-button" @click="showCommandModal = false">取消</button><button class="primary-button" @click="sendCommand"><Send :size="16" />立即下发</button></div></section></div>

    <div v-if="showFirmwareModal" class="modal-backdrop" @click.self="showFirmwareModal = false"><section class="modal"><div class="modal-heading"><div><span class="panel-kicker">FIRMWARE CATALOG</span><h2>登记固件</h2></div><button class="icon-button" title="关闭" @click="showFirmwareModal = false"><X :size="18" /></button></div><div class="form-grid"><label>所属产品<select v-model="firmwareForm.product_key"><option value="" disabled>选择产品</option><option v-for="product in products" :key="product.product_key" :value="product.product_key">{{ product.name }}</option></select></label><label>版本号<input v-model="firmwareForm.version" placeholder="1.2.3" /></label><label class="span-two">MD5<input v-model="firmwareForm.md5" placeholder="32 位十六进制摘要" /></label><label class="span-two">文件地址<input v-model="firmwareForm.file_url" placeholder="https://.../firmware.bin" /></label><label class="span-two">变更说明<textarea v-model="firmwareForm.changelog" rows="3"></textarea></label></div><div class="modal-actions"><button class="ghost-button" @click="showFirmwareModal = false">取消</button><button class="primary-button" @click="createFirmware"><Upload :size="16" />登记固件</button></div></section></div>

    <div v-if="showOTAModal" class="modal-backdrop" @click.self="showOTAModal = false"><section class="modal"><div class="modal-heading"><div><span class="panel-kicker">OTA DEPLOYMENT</span><h2>创建升级任务</h2></div><button class="icon-button" title="关闭" @click="showOTAModal = false"><X :size="18" /></button></div><div class="form-grid"><label>所属产品<select v-model="otaForm.product_key"><option value="" disabled>选择产品</option><option v-for="product in products" :key="product.product_key" :value="product.product_key">{{ product.name }}</option></select></label><label>固件版本<select v-model="otaForm.firmware_id"><option value="" disabled>选择固件</option><option v-for="firmware in firmwares.filter((item) => item.product_key === otaForm.product_key)" :key="firmware.id" :value="firmware.id">{{ firmware.version }}</option></select></label><label>目标范围<select v-model="otaForm.target"><option value="all">产品下全部设备</option><option value="devices">指定设备</option></select></label><label v-if="otaForm.target === 'devices'" class="span-two">设备 ID（可用空格或逗号分隔）<textarea v-model="otaForm.target_device_ids" rows="3" placeholder="temp-001 temp-002"></textarea></label></div><div class="modal-actions"><button class="ghost-button" @click="showOTAModal = false">取消</button><button class="primary-button" @click="createOTATask"><Upload :size="16" />创建任务</button></div></section></div>

    <div v-if="authRequired" class="modal-backdrop"><section class="modal auth-modal"><div class="auth-mark"><ShieldCheck :size="23" /></div><span class="eyebrow">SECURE ACCESS</span><h2>连接平台</h2><p>输入平台签发的 JWT 访问令牌，继续查看实时数据。</p><label>Bearer Token<input v-model="tokenInput" type="password" placeholder="eyJhbGciOiJIUzI1NiIs..." @keyup.enter="submitToken" /></label><div class="modal-actions"><button v-if="getAuthToken()" class="ghost-button" @click="clearToken">清除令牌</button><button class="primary-button" @click="submitToken"><KeyRound :size="16" />连接平台</button></div></section></div>

    <div v-if="toastMessage" class="toast"><Check :size="16" />{{ toastMessage }}</div>
  </div>
</template>
