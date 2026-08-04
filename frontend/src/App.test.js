import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { apiMock } = vi.hoisted(() => ({
  apiMock: { get: vi.fn(() => Promise.resolve({ items: [] })), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}))

vi.mock('./api', () => ({
  api: apiMock,
  getAuthToken: () => '',
  setAuthToken: () => {},
  itemsOf: (payload) => (Array.isArray(payload?.items) ? payload.items : []),
}))

for (const name of ['OverviewView', 'DevicesView', 'AlarmsView', 'RulesView', 'ProductsView', 'OTAView']) {
  vi.mock(`./views/${name}.vue`, () => ({ default: { name, template: `<div>${name}</div>` } }))
}

import App from './App.vue'
import { store } from './store'

describe('App shell', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    store.toastMessage = ''
    store.errorMessage = ''
    store.loading = false
    store.connected = true
    store.activeView = 'overview'
  })

  it('keeps the sidebar bottom (platform status + settings) across every view', async () => {
    const wrapper = mount(App)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.sidebar-bottom').exists()).toBe(true)
    expect(wrapper.find('.broker-status').exists()).toBe(true)
    expect(wrapper.text()).toContain('数据服务')
    expect(wrapper.text()).toContain('系统设置')
    for (const view of ['devices', 'alarms', 'rules', 'products', 'ota']) {
      store.activeView = view
      await wrapper.vm.$nextTick()
      expect(wrapper.find('.sidebar-bottom').exists()).toBe(true)
    }
  })

  it('system settings is clickable and gives feedback', async () => {
    const wrapper = mount(App)
    await wrapper.vm.$nextTick()
    await wrapper.find('button[title="系统设置"]').trigger('click')
    expect(store.toastMessage).toContain('系统设置')
  })
})
