const apiBase = import.meta.env.VITE_API_BASE || ''
const storage = typeof globalThis.localStorage === 'undefined' ? null : globalThis.localStorage
let authToken = storage?.getItem('iot-perform-token') || ''

export function getAuthToken() {
  return authToken
}

export function setAuthToken(value) {
  authToken = value.trim()
  if (authToken) {
    storage?.setItem('iot-perform-token', authToken)
  } else {
    storage?.removeItem('iot-perform-token')
  }
}

export async function request(path, options = {}) {
  const headers = new Headers(options.headers || {})
  if (options.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  if (authToken) {
    headers.set('Authorization', `Bearer ${authToken}`)
  }
  const response = await fetch(`${apiBase}${path}`, { ...options, headers })
  const text = await response.text()
  let data = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = text
    }
  }
  if (!response.ok) {
    const error = new Error(data?.message || data?.error || `请求失败 (${response.status})`)
    error.status = response.status
    error.payload = data
    throw error
  }
  return data
}

export const api = {
  get(path) {
    return request(path)
  },
  post(path, body) {
    return request(path, { method: 'POST', body: JSON.stringify(body) })
  },
  put(path, body) {
    return request(path, { method: 'PUT', body: JSON.stringify(body) })
  },
  delete(path) {
    return request(path, { method: 'DELETE' })
  },
}

export function itemsOf(payload) {
  return Array.isArray(payload?.items) ? payload.items : []
}
