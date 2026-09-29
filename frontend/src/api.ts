export class ApiError extends Error {
  status: number
  code: string

  constructor(message: string, status: number, code = 'request_failed') {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

type RequestOptions = Omit<RequestInit, 'body'> & { body?: unknown }

export async function api<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const headers = new Headers(options.headers)
  let body: BodyInit | undefined
  if (options.body !== undefined) {
    headers.set('Content-Type', 'application/json')
    body = JSON.stringify(options.body)
  }
  const response = await fetch(path, {
    ...options,
    body,
    headers,
    credentials: 'same-origin',
  })
  if (response.status === 204) return undefined as T

  const contentType = response.headers.get('content-type') || ''
  const payload = contentType.includes('application/json')
    ? await response.json().catch(() => ({}))
    : await response.text()
  if (!response.ok) {
    const message =
      typeof payload === 'object' && payload
        ? payload.error?.message || payload.message || `请求失败 (${response.status})`
        : String(payload || `请求失败 (${response.status})`)
    const code = typeof payload === 'object' && payload ? payload.error?.code : undefined
    if (response.status === 401 && !path.startsWith('/api/auth/login')) {
      window.dispatchEvent(new CustomEvent('relay:session-expired'))
    }
    throw new ApiError(message, response.status, code)
  }
  return payload as T
}

export function queryString(params: Record<string, string | number | undefined | null>) {
  const search = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') search.set(key, String(value))
  })
  const encoded = search.toString()
  return encoded ? `?${encoded}` : ''
}
