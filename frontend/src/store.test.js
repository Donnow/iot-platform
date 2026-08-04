import { beforeEach, describe, expect, it, vi } from 'vitest'

const { apiMock, tokenMock } = vi.hoisted(() => ({
  apiMock: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  tokenMock: { value: '', set: vi.fn() },
}))

vi.mock('./api', () => ({
  api: apiMock,
  getAuthToken: () => tokenMock.value,
  setAuthToken: (value) => {
    tokenMock.value = value
    tokenMock.set(value)
  },
  itemsOf: (payload) => (Array.isArray(payload?.items) ? payload.items : []),
}))

import {
  clearToken,
  createDevice,
  deleteDevice,
  loadData,
  navigate,
  onRealtimeMessage,
  saveDesired,
  selectDevice,
  startCommandPolling,
  store,
  submitLogin,
} from './store'

function resetStore() {
  Object.assign(store, {
    activeView: 'overview',
    connected: false,
    loading: false,
    refreshing: false,
    errorMessage: '',
    toastMessage: '',
    authRequired: false,
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
}

function mockGet(handler) {
  apiMock.get.mockImplementation(handler)
}

describe('platform store', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetStore()
    tokenMock.value = ''
  })

  it('navigates between views', () => {
    navigate('devices')
    expect(store.activeView).toBe('devices')
  })

  it('loadData hydrates lists, rules and selected device', async () => {
    mockGet((path) => {
      if (path === '/api/products?page=1&page_size=100') return Promise.resolve({ items: [{ product_key: 'pk', name: 'P' }] })
      if (path === '/api/devices?page=1&page_size=100') return Promise.resolve({ items: [{ device_id: 'd1', name: 'D' }] })
      if (path === '/api/alarms?page=1&page_size=100') return Promise.resolve({ items: [{ id: 'a1', status: 'active' }] })
      if (path === '/api/firmwares') return Promise.resolve({ items: [] })
      if (path === '/api/ota/tasks') return Promise.resolve({ items: [] })
      if (path.startsWith('/api/rules?product_key=')) return Promise.resolve({ items: [{ id: 'r1' }] })
      if (path.includes('/telemetry')) return Promise.resolve({ items: [{ timestamp: '2026-01-01T00:00:00Z', values: { temperature: 20 } }] })
      if (path.includes('/shadow')) return Promise.resolve({ desired: {}, reported: {} })
      return Promise.resolve({ items: [] })
    })
    await loadData()
    expect(store.products).toHaveLength(1)
    expect(store.devices).toHaveLength(1)
    expect(store.alarms[0].status).toBe('active')
    expect(store.rules).toHaveLength(1)
    expect(store.selectedDeviceId).toBe('d1')
    expect(store.connected).toBe(true)
  })

  it('loadData marks auth required on 401', async () => {
    apiMock.get.mockRejectedValue({ status: 401, message: 'unauthorized' })
    await loadData()
    expect(store.authRequired).toBe(true)
    expect(store.connected).toBe(false)
  })

  it('selectDevice loads telemetry and shadow for the device', async () => {
    store.devices = [{ device_id: 'd1' }]
    mockGet((path) => {
      if (path.includes('/telemetry')) return Promise.resolve({ items: [{ timestamp: '2026-01-01T00:00:00Z', values: { temperature: 21 } }] })
      if (path.includes('/shadow')) return Promise.resolve({ desired: { target: 26 }, reported: {} })
      return Promise.resolve({})
    })
    await selectDevice('d1')
    expect(store.selectedDeviceId).toBe('d1')
    expect(store.telemetry).toHaveLength(1)
    expect(store.deviceShadow.desired.target).toBe(26)
  })

  it('onRealtimeMessage updates device status from status and event topics', () => {
    store.devices = [{ device_id: 'd1', status: 'offline' }]
    onRealtimeMessage('devices/pk/d1/status', JSON.stringify({ status: 'online' }))
    expect(store.devices[0].status).toBe('online')
    onRealtimeMessage('devices/pk/d1/event', JSON.stringify({ status: 'offline' }))
    expect(store.devices[0].status).toBe('offline')
  })

  it('onRealtimeMessage ignores unknown devices and malformed payloads', () => {
    store.devices = [{ device_id: 'd1', status: 'online' }]
    onRealtimeMessage('devices/pk/unknown/status', JSON.stringify({ status: 'offline' }))
    onRealtimeMessage('devices/pk/d1/status', '{not json')
    expect(store.devices[0].status).toBe('online')
  })

  it('startCommandPolling stops once the command leaves pending', async () => {
    vi.useFakeTimers()
    try {
      apiMock.get
        .mockResolvedValueOnce({ status: 'pending' })
        .mockResolvedValueOnce({ status: 'success', message: 'ok' })
      startCommandPolling('d1', 'c1')
      await vi.advanceTimersByTimeAsync(0)
      expect(store.commandPolling).toBe(true)
      expect(store.commandStatus.status).toBe('pending')
      await vi.advanceTimersByTimeAsync(1000)
      expect(store.commandPolling).toBe(false)
      expect(store.commandStatus.status).toBe('success')
    } finally {
      vi.useRealTimers()
    }
  })

  it('submitLogin posts credentials and clears the auth modal on success', async () => {
    apiMock.post.mockResolvedValue({ token: 'jwt-1', username: 'admin' })
    store.loginUsername = 'admin'
    store.loginPassword = 'secret'
    await submitLogin()
    expect(apiMock.post).toHaveBeenCalledWith('/api/auth/login', { username: 'admin', password: 'secret' })
    expect(tokenMock.value).toBe('jwt-1')
    expect(store.authRequired).toBe(false)
  })

  it('submitLogin rejects empty credentials without calling the API', async () => {
    store.loginUsername = ''
    await submitLogin()
    expect(apiMock.post).not.toHaveBeenCalled()
    expect(store.toastMessage).toBeTruthy()
  })

  it('clearToken resets credentials and reopens auth', () => {
    clearToken()
    expect(tokenMock.value).toBe('')
    expect(store.authRequired).toBe(true)
  })

  it('createDevice requires a name and product, then reveals the secret', async () => {
    store.deviceForm.name = ''
    await createDevice()
    expect(apiMock.post).not.toHaveBeenCalled()
    store.deviceForm.name = '温湿度'
    store.deviceForm.product_key = 'pk'
    apiMock.post.mockResolvedValue({ device_secret: 'secret-1' })
    await createDevice()
    expect(apiMock.post).toHaveBeenCalledWith('/api/devices', store.deviceForm)
    expect(store.revealedSecret).toBe('secret-1')
  })

  it('saveDesired validates JSON and saves via the shadow endpoint', async () => {
    store.desiredInput = '{not json'
    await saveDesired('d1')
    expect(apiMock.put).not.toHaveBeenCalled()
    store.desiredInput = '{"target":26}'
    apiMock.put.mockResolvedValue({ desired: { target: 26 }, reported: {} })
    await saveDesired('d1')
    expect(apiMock.put).toHaveBeenCalledWith('/api/devices/d1/shadow/desired', { target: 26 })
  })

  it('deleteDevice confirms, calls the API and clears selection', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    store.selectedDeviceId = 'd1'
    apiMock.delete.mockResolvedValue({})
    mockGet((path) => {
      if (path === '/api/products?page=1&page_size=100') return Promise.resolve({ items: [] })
      return Promise.resolve({ items: [] })
    })
    await deleteDevice({ device_id: 'd1', name: 'D' })
    expect(apiMock.delete).toHaveBeenCalledWith('/api/devices/d1')
    expect(store.selectedDeviceId).toBe('')
  })
})
