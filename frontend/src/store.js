import { reactive } from 'vue'
import { api, getAuthToken, setAuthToken, itemsOf } from './api'

export const store = reactive({
  activeView: 'overview',
  connected: false,
  loading: false,
  refreshing: false,
  errorMessage: '',
  toastMessage: '',
  authRequired: false,
  tokenInput: getAuthToken(),
  loginUsername: '',
  loginPassword: '',
  submitting: false,

  products: [],
  devices: [],
  alarms: [],
  rules: [],
  firmwares: [],
  otaTasks: [],
  telemetry: [],
  deviceShadow: null,
  desiredInput: '{}',

  commandStatus: null,
  commandPolling: false,
  selectedDeviceId: '',
  selectedMetric: '',

  showDeviceModal: false,
  showProductModal: false,
  showRuleModal: false,
  showCommandModal: false,
  showFirmwareModal: false,
  showOTAModal: false,
  revealedSecret: '',
  commandDevice: null,

  deviceForm: { device_id: '', product_key: '', name: '', description: '' },
  productForm: { product_key: '', name: '', device_type: 'sensor', description: '', properties: [] },
  ruleForm: { product_key: '', name: '', property_name: '', operator: '>', threshold: 40, duration_seconds: 0, action_type: 'alarm' },
  commandForm: { method: 'open', params: '{}' },
  firmwareForm: { product_key: '', version: '', md5: '', file_url: '', changelog: '' },
  otaForm: { product_key: '', firmware_id: '', target: 'all', target_device_ids: '' },
})

let toastTimer = null
let commandPollTimer = null
let mqttClient = null

export function navigate(view) {
  store.activeView = view
}

export function showToast(message) {
  store.toastMessage = message
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => {
    store.toastMessage = ''
  }, 3400)
}

export async function loadData({ silent = false } = {}) {
  if (!silent) store.loading = true
  else store.refreshing = true
  store.errorMessage = ''
  try {
    const [productPayload, devicePayload, alarmPayload, firmwarePayload, otaPayload] = await Promise.all([
      api.get('/api/products?page=1&page_size=100'),
      api.get('/api/devices?page=1&page_size=100'),
      api.get('/api/alarms?page=1&page_size=100'),
      api.get('/api/firmwares'),
      api.get('/api/ota/tasks'),
    ])
    store.products = itemsOf(productPayload)
    store.devices = itemsOf(devicePayload)
    store.alarms = itemsOf(alarmPayload)
    store.firmwares = itemsOf(firmwarePayload)
    store.otaTasks = itemsOf(otaPayload)
    if (!store.selectedDeviceId || !store.devices.some((device) => device.device_id === store.selectedDeviceId)) {
      store.selectedDeviceId = store.devices[0]?.device_id || ''
    }
    const rulePayloads = await Promise.all(store.products.map((product) => api.get(`/api/rules?product_key=${encodeURIComponent(product.product_key)}`)))
    store.rules = rulePayloads.flatMap(itemsOf)
    if (store.selectedDeviceId) {
      await Promise.all([loadTelemetry(store.selectedDeviceId), loadShadow(store.selectedDeviceId)])
    }
    store.connected = true
  } catch (error) {
    store.connected = false
    if (error.status === 401) {
      store.authRequired = true
      store.errorMessage = '请输入平台访问令牌'
    } else {
      store.errorMessage = error.message || '平台暂时不可用'
    }
  } finally {
    store.loading = false
    store.refreshing = false
  }
}

export async function loadTelemetry(deviceId) {
  if (!deviceId) {
    store.telemetry = []
    return
  }
  try {
    const payload = await api.get(`/api/devices/${encodeURIComponent(deviceId)}/telemetry?limit=200`)
    store.telemetry = itemsOf(payload)
  } catch (error) {
    if (error.status === 401) store.authRequired = true
    store.telemetry = []
  }
}

export async function loadShadow(deviceId) {
  if (!deviceId) {
    store.deviceShadow = null
    store.desiredInput = '{}'
    return
  }
  try {
    const shadow = await api.get(`/api/devices/${encodeURIComponent(deviceId)}/shadow`)
    store.deviceShadow = shadow
    store.desiredInput = JSON.stringify(shadow?.desired || {}, null, 2)
  } catch (error) {
    if (error.status === 401) store.authRequired = true
    store.deviceShadow = null
  }
}

export function selectDevice(deviceId) {
  store.selectedDeviceId = deviceId
  Promise.all([loadTelemetry(deviceId), loadShadow(deviceId)])
}

export function resetDeviceForm() {
  Object.assign(store.deviceForm, { device_id: '', product_key: store.products[0]?.product_key || '', name: '', description: '' })
  store.revealedSecret = ''
}

export function resetProductForm() {
  Object.assign(store.productForm, { product_key: '', name: '', device_type: 'sensor', description: '', properties: [] })
}

export function openProductModal() {
  resetProductForm()
  store.showProductModal = true
}

export function addProperty() {
  store.productForm.properties.push({ name: '', data_type: 'float', unit: '', min_value: '', max_value: '' })
}

export function removeProperty(index) {
  store.productForm.properties.splice(index, 1)
}

export function openDeviceModal() {
  resetDeviceForm()
  store.showDeviceModal = true
}

export async function createDevice() {
  if (!store.deviceForm.product_key || !store.deviceForm.name) return showToast('请填写设备名称和产品')
  try {
    const response = await api.post('/api/devices', store.deviceForm)
    store.revealedSecret = response.device_secret || ''
    showToast('设备已注册')
    await loadData({ silent: true })
  } catch (error) {
    showToast(error.message)
  }
}

export async function createProduct() {
  if (!store.productForm.name) return showToast('请填写产品名称')
  try {
    const properties = store.productForm.properties.filter((property) => property.name.trim()).map((property) => ({
      ...property,
      min_value: property.min_value === '' ? undefined : Number(property.min_value),
      max_value: property.max_value === '' ? undefined : Number(property.max_value),
    }))
    await api.post('/api/products', { ...store.productForm, properties })
    store.showProductModal = false
    showToast('产品已创建')
    await loadData({ silent: true })
  } catch (error) {
    showToast(error.message)
  }
}

export async function createRule() {
  if (!store.ruleForm.product_key || !store.ruleForm.name || !store.ruleForm.property_name) return showToast('请补全规则字段')
  try {
    await api.post('/api/rules', { ...store.ruleForm, threshold: Number(store.ruleForm.threshold), duration_seconds: Number(store.ruleForm.duration_seconds) })
    store.showRuleModal = false
    showToast('规则已启用')
    await loadData({ silent: true })
  } catch (error) {
    showToast(error.message)
  }
}

export function openCommand(device) {
  store.commandDevice = device
  Object.assign(store.commandForm, { method: device.product_key.includes('air') ? 'setTemp' : 'open', params: '{}' })
  store.showCommandModal = true
  store.commandStatus = null
}

export async function sendCommand() {
  if (!store.commandDevice) return
  let params
  try {
    params = JSON.parse(store.commandForm.params || '{}')
  } catch {
    return showToast('Params 必须是合法 JSON')
  }
  try {
    const command = await api.post(`/api/devices/${encodeURIComponent(store.commandDevice.device_id)}/commands`, { method: store.commandForm.method, params })
    store.showCommandModal = false
    store.commandStatus = command
    startCommandPolling(store.commandDevice.device_id, command.command_id)
    showToast('指令已下发')
    await loadData({ silent: true })
  } catch (error) {
    showToast(error.message)
  }
}

export function startCommandPolling(deviceId, commandId) {
  clearInterval(commandPollTimer)
  store.commandPolling = true
  const poll = async () => {
    try {
      const status = await api.get(`/api/devices/${encodeURIComponent(deviceId)}/commands/${encodeURIComponent(commandId)}`)
      store.commandStatus = status
      if (status.status !== 'pending') {
        store.commandPolling = false
        clearInterval(commandPollTimer)
      }
    } catch {
      store.commandPolling = false
      clearInterval(commandPollTimer)
    }
  }
  poll()
  commandPollTimer = setInterval(poll, 1000)
}

export async function deleteDevice(device) {
  if (!window.confirm(`确认删除设备 ${device.name || device.device_id}？`)) return
  try {
    await api.delete(`/api/devices/${encodeURIComponent(device.device_id)}`)
    if (store.selectedDeviceId === device.device_id) store.selectedDeviceId = ''
    showToast('设备已删除')
    await loadData({ silent: true })
  } catch (error) {
    showToast(error.message)
  }
}

export async function saveDesired(deviceId) {
  if (!deviceId) return
  let desired
  try {
    desired = JSON.parse(store.desiredInput || '{}')
  } catch {
    return showToast('desired 必须是合法 JSON')
  }
  if (!desired || Array.isArray(desired)) return showToast('desired 必须是 JSON 对象')
  try {
    store.deviceShadow = await api.put(`/api/devices/${encodeURIComponent(deviceId)}/shadow/desired`, desired)
    store.desiredInput = JSON.stringify(store.deviceShadow.desired || {}, null, 2)
    showToast('期望状态已保存')
  } catch (error) {
    showToast(error.message)
  }
}

export function openFirmwareModal() {
  Object.assign(store.firmwareForm, { product_key: store.products[0]?.product_key || '', version: '', md5: '', file_url: '', changelog: '' })
  store.showFirmwareModal = true
}

export async function createFirmware() {
  if (!store.firmwareForm.product_key || !store.firmwareForm.version || !store.firmwareForm.md5 || !store.firmwareForm.file_url) return showToast('请补全固件字段')
  try {
    await api.post('/api/firmwares', { ...store.firmwareForm })
    store.showFirmwareModal = false
    showToast('固件元数据已登记')
    await loadData({ silent: true })
  } catch (error) {
    showToast(error.message)
  }
}

export function openOTAModal() {
  Object.assign(store.otaForm, { product_key: store.products[0]?.product_key || '', firmware_id: '', target: 'all', target_device_ids: '' })
  store.showOTAModal = true
}

export async function createOTATask() {
  if (!store.otaForm.product_key || !store.otaForm.firmware_id) return showToast('请选择产品和固件')
  const targetDeviceIds = store.otaForm.target === 'all' ? [] : store.otaForm.target_device_ids.split(/[\s,]+/).map((value) => value.trim()).filter(Boolean)
  if (store.otaForm.target === 'devices' && !targetDeviceIds.length) return showToast('请填写目标设备 ID')
  try {
    await api.post('/api/ota/tasks', { product_key: store.otaForm.product_key, firmware_id: store.otaForm.firmware_id, target: store.otaForm.target, target_device_ids: targetDeviceIds })
    store.showOTAModal = false
    showToast('OTA 任务已创建')
    await loadData({ silent: true })
  } catch (error) {
    showToast(error.message)
  }
}

export function onRealtimeMessage(topic, payload) {
  const parts = topic.split('/')
  if (parts.length < 4 || parts[0] !== 'devices') return
  const device = store.devices.find((item) => item.device_id === parts[2])
  if (!device) return
  try {
    const event = JSON.parse(payload.toString())
    if (parts[3] === 'status' && event.status) device.status = event.status
    if (parts[3] === 'event' && event.status) device.status = event.status
  } catch {
    // Ignore malformed realtime messages; HTTP refresh remains authoritative.
  }
}

export function connectRealtime() {
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
      store.connected = true
      mqttClient.subscribe(import.meta.env.VITE_MQTT_STATUS_TOPIC || 'devices/+/+/status')
      mqttClient.subscribe(import.meta.env.VITE_MQTT_EVENT_TOPIC || 'devices/+/+/event')
    })
    mqttClient.on('message', onRealtimeMessage)
    mqttClient.on('error', () => {
      mqttClient?.end(true)
    })
  }).catch(() => {})
}

export function disconnectRealtime() {
  mqttClient?.end(true)
  mqttClient = null
}

export async function resolveAlarm(alarm) {
  try {
    await api.put(`/api/alarms/${encodeURIComponent(alarm.id)}/resolve`, { note: '已由运维台确认处理' })
    showToast('告警已解除')
    await loadData({ silent: true })
  } catch (error) {
    showToast(error.message)
  }
}

export async function submitLogin() {
  if (!store.loginUsername || !store.loginPassword) {
    showToast('请输入用户名和密码')
    return
  }
  store.submitting = true
  try {
    const result = await api.post('/api/auth/login', {
      username: store.loginUsername,
      password: store.loginPassword,
    })
    setAuthToken(result.token)
    store.authRequired = false
    store.loginPassword = ''
    showToast(`已登录：${result.username || store.loginUsername}`)
    loadData()
  } catch (error) {
    showToast(error.message || '登录失败')
  } finally {
    store.submitting = false
  }
}

export function clearToken() {
  store.tokenInput = ''
  store.loginUsername = ''
  store.loginPassword = ''
  setAuthToken('')
  store.authRequired = true
}
