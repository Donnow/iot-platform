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

import ProductModal from './ProductModal.vue'
import { store } from '../store'

describe('ProductModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    store.showProductModal = true
    store.products = [{ product_key: 'pk', name: '温湿度传感器' }]
    store.productForm = { product_key: '', name: '', device_type: 'sensor', description: '', properties: [] }
    store.toastMessage = ''
  })

  it('adds and removes property rows', async () => {
    const wrapper = mount(ProductModal)
    expect(wrapper.findAll('.property-row')).toHaveLength(0)
    await wrapper.find('button.compact-action').trigger('click')
    expect(store.productForm.properties).toHaveLength(1)
    await wrapper.find('button.danger-icon').trigger('click')
    expect(store.productForm.properties).toHaveLength(0)
  })

  it('requires a product name before creating', async () => {
    const wrapper = mount(ProductModal)
    store.productForm.name = ''
    await wrapper.find('button.primary-button').trigger('click')
    expect(apiMock.post).not.toHaveBeenCalled()
    expect(store.toastMessage).toBeTruthy()
  })

  it('creates a product with normalized numeric properties', async () => {
    const wrapper = mount(ProductModal)
    store.productForm.name = '烟感'
    store.productForm.product_key = 'smoke'
    store.productForm.properties = [{ name: 'level', data_type: 'float', unit: '', min_value: '0', max_value: '100' }]
    apiMock.post.mockResolvedValue({})
    await wrapper.find('button.primary-button').trigger('click')
    expect(apiMock.post).toHaveBeenCalledWith('/api/products', expect.objectContaining({
      name: '烟感',
      product_key: 'smoke',
      properties: [{ name: 'level', data_type: 'float', unit: '', min_value: 0, max_value: 100 }],
    }))
    expect(store.showProductModal).toBe(false)
  })
})
