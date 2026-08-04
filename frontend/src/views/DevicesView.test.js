import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { apiMock } = vi.hoisted(() => ({
  apiMock: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}))

vi.mock('../api', () => ({
  api: apiMock,
  getAuthToken: () => '',
  setAuthToken: () => {},
  itemsOf: (payload) => (Array.isArray(payload?.items) ? payload.items : []),
}))

import DevicesView from './DevicesView.vue'
import { store } from '../store'

describe('DevicesView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    store.devices = [
      { device_id: 'd1', name: '温湿度一', product_key: 'pk1', status: 'online', last_online: '2026-01-01T00:00:00Z' },
      { device_id: 'd2', name: '烟感一', product_key: 'pk2', status: 'offline', last_online: null },
    ]
    store.selectedDeviceId = 'd1'
    store.deviceShadow = { desired: {}, reported: {} }
    store.desiredInput = '{}'
    store.commandStatus = null
    store.commandPolling = false
  })

  it('renders the device table and the selected device detail', () => {
    const wrapper = mount(DevicesView)
    expect(wrapper.text()).toContain('温湿度一')
    expect(wrapper.text()).toContain('烟感一')
    expect(wrapper.text()).toContain('在线')
  })

  it('filters the table by search term', async () => {
    const wrapper = mount(DevicesView)
    await wrapper.find('input[placeholder="搜索设备名称、ID 或产品"]').setValue('烟感')
    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(1)
    expect(rows[0].text()).toContain('烟感一')
  })

  it('selecting a row switches the selected device', async () => {
    const wrapper = mount(DevicesView)
    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    await rows[1].trigger('click')
    expect(store.selectedDeviceId).toBe('d2')
  })

  it('opening the register device modal is exposed through the page action', async () => {
    const wrapper = mount(DevicesView)
    await wrapper.findAll('button').find((button) => button.text().includes('注册设备')).trigger('click')
    expect(store.showDeviceModal).toBe(true)
  })
})
