import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { echartsMock, apiMock } = vi.hoisted(() => ({
  echartsMock: {
    use: vi.fn(),
    init: vi.fn(() => ({ setOption: vi.fn(), resize: vi.fn(), dispose: vi.fn() })),
  },
  apiMock: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}))

vi.mock('echarts/core', () => echartsMock)
vi.mock('echarts/charts', () => ({ LineChart: {} }))
vi.mock('echarts/components', () => ({ GridComponent: {}, TooltipComponent: {}, GraphicComponent: {} }))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: {} }))
vi.mock('../api', () => ({
  api: apiMock,
  getAuthToken: () => '',
  setAuthToken: () => {},
  itemsOf: (payload) => (Array.isArray(payload?.items) ? payload.items : []),
}))

import OverviewView from './OverviewView.vue'
import { store } from '../store'

describe('OverviewView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    store.devices = [
      { device_id: 'd1', name: '温湿度一', product_key: 'pk1', status: 'online', last_online: '2026-01-01T00:00:00Z' },
      { device_id: 'd2', name: '烟感一', product_key: 'pk2', status: 'offline', last_online: null },
    ]
    store.alarms = [{ id: 'a1', status: 'active', rule_id: 'r1', device_id: 'd1', triggered_at: '2026-01-01T00:00:00Z' }]
    store.rules = [{ id: 'r1', name: '高温' }]
    store.telemetry = [{ timestamp: '2026-01-01T00:00:00Z', values: { temperature: 20 } }]
    store.selectedDeviceId = 'd1'
  })

  it('renders the overview metric cards', () => {
    const wrapper = mount(OverviewView)
    expect(wrapper.text()).toContain('在线设备')
    expect(wrapper.text()).toContain('活跃告警')
    expect(wrapper.text()).toContain('遥测记录')
    expect(wrapper.text()).toContain('生效规则')
  })

  it('initializes the chart with the mocked echarts subset', async () => {
    const wrapper = mount(OverviewView)
    await vi.waitFor(() => expect(echartsMock.init).toHaveBeenCalled())
    expect(echartsMock.use).toHaveBeenCalledWith(expect.arrayContaining([expect.anything()]))
    expect(wrapper.find('.chart').exists()).toBe(true)
  })

  it('opens the register device modal from the overview action', async () => {
    const wrapper = mount(OverviewView)
    await wrapper.find('button.primary-button').trigger('click')
    expect(store.showDeviceModal).toBe(true)
  })
})
