export interface Bucket {
  id: string
  name: string
  has_pin: boolean
  pinned: boolean
  private: boolean
  owner?: string
  created_at: number
}

export interface Message {
  id: string
  bucket_id: string
  kind: 'text' | 'file'
  content?: string
  filename?: string
  mime?: string
  size?: number
  sender?: string
  pinned: boolean
  created_at: number
}

export interface AuthState {
  auth_required: boolean
  authenticated: boolean
  trusted: boolean
  oidc_enabled: boolean
  user_auth: boolean
  user: string
}

export class ApiError extends Error {
  status: number
  data: Record<string, unknown>

  constructor(status: number, data: Record<string, unknown>) {
    super(typeof data.error === 'string' ? data.error : `HTTP ${status}`)
    this.status = status
    this.data = data
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(path, init)
  if (resp.status === 204) return undefined as T
  let data: Record<string, unknown> = {}
  const text = await resp.text()
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = {}
    }
  }
  if (!resp.ok) throw new ApiError(resp.status, data)
  return data as T
}

function jsonBody(payload: unknown): RequestInit {
  return {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  }
}

export const api = {
  me: () => request<AuthState>('/api/auth/me'),
  login: (password: string) => request<void>('/api/auth/login', jsonBody({ password })),
  logout: () => request<void>('/api/auth/logout', { method: 'POST' }),

  buckets: () => request<{ buckets: Bucket[] }>('/api/buckets'),
  createBucket: (name: string, pin?: string, priv?: boolean) =>
    request<Bucket>('/api/buckets', jsonBody({ name, pin: pin || '', private: priv ?? false })),
  deleteBucket: (id: string) => request<void>(`/api/buckets/${id}`, { method: 'DELETE' }),
  unlock: (id: string, pin: string) =>
    request<void>(`/api/buckets/${id}/unlock`, jsonBody({ pin })),
  lock: (id: string) => request<void>(`/api/buckets/${id}/lock`, { method: 'POST' }),
  pinBucket: (id: string, pinned: boolean) =>
    request<void>(`/api/buckets/${id}/pin`, jsonBody({ pinned })),

  messages: (bucketId: string, afterId = '') =>
    request<{ messages: Message[]; pinned_messages: Message[] }>(
      `/api/buckets/${bucketId}/messages${afterId ? `?after_id=${encodeURIComponent(afterId)}` : ''}`,
    ),
  sendMessage: (bucketId: string, content: string, sender: string) =>
    request<Message>(`/api/buckets/${bucketId}/messages`, jsonBody({ content, sender })),
  pinMessage: (id: string, pinned: boolean) =>
    request<void>(`/api/messages/${id}/pin`, jsonBody({ pinned })),
  deleteMessage: (id: string) => request<void>(`/api/messages/${id}`, { method: 'DELETE' }),
}

export function uploadFile(
  bucketId: string,
  file: File,
  sender: string,
  onProgress: (fraction: number) => void,
): Promise<Message> {
  return new Promise((resolve, reject) => {
    const form = new FormData()
    form.append('sender', sender)
    form.append('file', file)

    const xhr = new XMLHttpRequest()
    xhr.open('POST', `/api/buckets/${bucketId}/files`)
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress(e.loaded / e.total)
    }
    xhr.onload = () => {
      let data: Record<string, unknown> = {}
      try {
        data = JSON.parse(xhr.responseText || '{}')
      } catch {
        /* keep empty */
      }
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(data as unknown as Message)
      } else {
        reject(new ApiError(xhr.status, data))
      }
    }
    xhr.onerror = () => reject(new ApiError(0, { error: 'network error' }))
    xhr.send(form)
  })
}

export function fileUrl(id: string, inline = false): string {
  return `/api/files/${id}${inline ? '?inline=1' : ''}`
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unit = 'B'
  for (const u of units) {
    if (value < 1024) break
    value /= 1024
    unit = u
  }
  return `${value.toFixed(value >= 100 ? 0 : 1)} ${unit}`
}

export function formatTime(ts: number): string {
  const d = new Date(ts * 1000)
  const sameDay = d.toDateString() === new Date().toDateString()
  const time = d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
  if (sameDay) return time
  const date = d.toLocaleDateString(undefined, { day: 'numeric', month: 'short' })
  return `${date} ${time}`
}
