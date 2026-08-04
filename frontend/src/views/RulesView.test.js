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

import RulesView from './RulesView.vue'
import { store } from '../store'

describe('RulesView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    store.rules = [
      { id: 'r1', name: '高温告警', product_key: 'pk1', property_name: 'temperature', operator: '>', threshold: 40, duration_seconds: 0, action_type: 'alarm', enabled: true },
      { id: 'r2', name: '停用规则', product_key: 'pk2', property_name: 'smoke', operator: '>=', threshold: 80, duration_seconds: 5, action_type: 'alarm', enabled: false },
    ]
  })

  it('renders rules with their condition and enabled state', () => {
    const wrapper = mount(RulesView)
    expect(wrapper.text()).toContain('高温告警')
    expect(wrapper.text()).toContain('temperature')
    expect(wrapper.text()).toContain('已启用')
    expect(wrapper.text()).toContain('已停用')
  })

  it('opens the new rule modal', async () => {
    const wrapper = mount(RulesView)
    await wrapper.find('button.primary-button').trigger('click')
    expect(store.showRuleModal).toBe(true)
  })
})
