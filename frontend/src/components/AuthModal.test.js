import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { apiMock, tokenMock } = vi.hoisted(() => ({
  apiMock: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  tokenMock: { value: '', set: vi.fn() },
}))

vi.mock('../api', () => ({
  api: apiMock,
  getAuthToken: () => tokenMock.value,
  setAuthToken: (value) => {
    tokenMock.value = value
    tokenMock.set(value)
  },
  itemsOf: (payload) => (Array.isArray(payload?.items) ? payload.items : []),
}))

import AuthModal from './AuthModal.vue'
import { store } from '../store'

describe('AuthModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    tokenMock.value = ''
    store.authRequired = true
    store.loginUsername = ''
    store.loginPassword = ''
    store.submitting = false
    store.toastMessage = ''
  })

  it('logs in with credentials and closes the modal on success', async () => {
    apiMock.post.mockResolvedValue({ token: 'jwt-1', username: 'admin' })
    const wrapper = mount(AuthModal)
    await wrapper.find('input[autocomplete="username"]').setValue('admin')
    await wrapper.find('input[autocomplete="current-password"]').setValue('secret')
    await wrapper.find('button.primary-button').trigger('click')
    expect(apiMock.post).toHaveBeenCalledWith('/api/auth/login', { username: 'admin', password: 'secret' })
    expect(tokenMock.value).toBe('jwt-1')
    expect(store.authRequired).toBe(false)
  })

  it('rejects empty credentials without calling the API', async () => {
    const wrapper = mount(AuthModal)
    await wrapper.find('button.primary-button').trigger('click')
    expect(apiMock.post).not.toHaveBeenCalled()
    expect(store.toastMessage).toBeTruthy()
  })
})
