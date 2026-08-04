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

import AlarmsView from './AlarmsView.vue'
import { store } from '../store'

describe('AlarmsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    store.alarms = [
      { id: 'a1', device_id: 'd1', rule_id: 'r1', status: 'active', trigger_value: 50, triggered_at: '2026-01-01T00:00:00Z' },
      { id: 'a2', device_id: 'd2', rule_id: 'r2', status: 'resolved', trigger_value: 10, triggered_at: '2026-01-01T00:00:00Z', resolved_at: '2026-01-01T01:00:00Z' },
    ]
  })

  it('shows the active alarm count and both records', () => {
    const wrapper = mount(AlarmsView)
    expect(wrapper.text()).toContain('1')
    expect(wrapper.text()).toContain('处理中')
    expect(wrapper.text()).toContain('已解除')
  })

  it('filters by status tab', async () => {
    const wrapper = mount(AlarmsView)
    const tabs = wrapper.findAll('.filter-tabs button')
    await tabs[1].trigger('click') // 处理中
    let rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(1)
    expect(rows[0].text()).toContain('处理中')
    await tabs[2].trigger('click') // 已解除
    rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(1)
    expect(rows[0].text()).toContain('已解除')
  })

  it('resolves an active alarm through the resolve action', async () => {
    const wrapper = mount(AlarmsView)
    await wrapper.find('button.compact-action').trigger('click')
    expect(apiMock.put).toHaveBeenCalledWith('/api/alarms/a1/resolve', expect.anything())
  })
})
