import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, getAuthToken, itemsOf, request, setAuthToken } from './api'

describe('frontend api client', () => {
  beforeEach(() => {
    setAuthToken('')
    vi.restoreAllMocks()
  })

  it('stores and sends the bearer token', async () => {
    setAuthToken('token-123')
    expect(getAuthToken()).toBe('token-123')

    globalThis.fetch = vi.fn().mockResolvedValue(new Response('{"items":[]}', { status: 200 }))
    await api.get('/api/devices?page=1')

    expect(fetch).toHaveBeenCalledWith('/api/devices?page=1', expect.objectContaining({
      headers: expect.any(Headers),
    }))
    const headers = fetch.mock.calls[0][1].headers
    expect(headers.get('Authorization')).toBe('Bearer token-123')
  })

  it('normalizes paged responses and propagates server errors', async () => {
    expect(itemsOf({ items: [{ id: 'd1' }] })).toEqual([{ id: 'd1' }])
    expect(itemsOf({})).toEqual([])

    globalThis.fetch = vi.fn().mockResolvedValue(new Response('{"message":"unauthorized"}', { status: 401 }))
    await expect(request('/api/products')).rejects.toMatchObject({ status: 401, message: 'unauthorized' })
  })
})
