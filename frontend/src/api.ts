import type { Identity, Material, SearchResponse } from './types'

// 统一错误：把服务端状态码与错误码带给界面，
// 但界面只按状态码决定"回登录页"还是"提示失败"，不改写服务端语义。
export class ApiError extends Error {
  constructor(readonly status: number, readonly code: string) {
    super(`${status} ${code}`)
    this.name = 'ApiError'
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: 'same-origin',
    headers: {
      ...(init.body && !(init.body instanceof FormData) ? { 'Content-Type': 'application/json' } : {}),
      ...(init.headers ?? {}),
    },
  })

  if (response.status === 204) {
    return undefined as T
  }

  const text = await response.text()
  const payload = text ? JSON.parse(text) : null

  if (!response.ok) {
    const code = payload && typeof payload.error === 'string' ? payload.error : 'unknown_error'
    throw new ApiError(response.status, code)
  }
  return payload as T
}

export const api = {
  me: () => request<Identity>('/api/me'),

  login: (username: string, password: string) =>
    request<Identity>('/api/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),

  logout: () => request<void>('/api/logout', { method: 'POST' }),

  listMaterials: () => request<{ items: Material[]; class_id: number }>('/api/materials'),

  getMaterial: (id: number) => request<Material>(`/api/materials/${id}`),

  // 下载必须先带会话 Cookie 请求鉴权接口，拿到内容后再由前端触发保存。
  // 页面上不出现任何指向上传目录的静态 URL。
  download: async (material: Material) => {
    const response = await fetch(`/api/materials/${material.id}/file`, { credentials: 'same-origin' })
    if (!response.ok) {
      const text = await response.text()
      let code = 'unknown_error'
      try {
        code = JSON.parse(text).error ?? code
      } catch {
        /* 空响应体时保留默认错误码 */
      }
      throw new ApiError(response.status, code)
    }
    const blob = await response.blob()
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = material.original_name
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
    URL.revokeObjectURL(url)
  },

  // 本班范围内的知识库检索。范围由服务端会话决定，前端不传班级。
  search: (query: string, topK = 5) =>
    request<SearchResponse>('/api/search', {
      method: 'POST',
      body: JSON.stringify({ query, top_k: topK }),
    }),

  // 上传走 XHR，为了拿到进度事件（R10.5）。
  upload: (file: File, onProgress: (percent: number) => void) =>
    new Promise<Material>((resolve, reject) => {
      const form = new FormData()
      form.append('file', file)
      const xhr = new XMLHttpRequest()
      xhr.open('POST', '/api/materials')
      xhr.withCredentials = true
      xhr.upload.onprogress = (event) => {
        if (event.lengthComputable) {
          onProgress(Math.round((event.loaded / event.total) * 100))
        }
      }
      xhr.onload = () => {
        let payload: unknown = null
        try {
          payload = JSON.parse(xhr.responseText)
        } catch {
          payload = null
        }
        if (xhr.status >= 200 && xhr.status < 300) {
          resolve(payload as Material)
          return
        }
        const code =
          payload && typeof (payload as { error?: string }).error === 'string'
            ? (payload as { error: string }).error
            : 'unknown_error'
        reject(new ApiError(xhr.status, code))
      }
      xhr.onerror = () => reject(new ApiError(0, 'network_error'))
      xhr.send(form)
    }),
}

// 把错误码翻成给用户看的中文提示；401 单独处理为"回登录页"。
export function describeError(error: unknown): string {
  if (error instanceof ApiError) {
    switch (error.status) {
      case 400:
        return '请求不符合要求（检索词不能为空或过长；上传只支持 .txt / .md 的非空 UTF-8 文本）'
      case 401:
        return '登录状态已失效，请重新登录'
      case 403:
        return '当前角色没有这个操作权限'
      case 404:
        return '材料不存在或不属于你所在的班级'
      case 413:
        return '文件超过大小上限'
      default:
        return '操作失败，请稍后再试'
    }
  }
  return '操作失败，请稍后再试'
}
