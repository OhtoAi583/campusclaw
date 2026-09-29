import { useCallback, useEffect, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { api, describeError } from '../api'
import type { Material } from '../types'
import { ThemeToggle } from '../components/ThemeToggle'
import { useToast } from '../components/Toast'

// 与后端一致地按"码点"切分，避免把表情等多字节字符算错位置。
function sliceByCodePoints(text: string, start: number, end: number) {
  const points = Array.from(text)
  return points.slice(start, end).join('')
}

export default function MaterialDetailPage({ onExpired }: { onExpired: () => void }) {
  const { id } = useParams()
  const [params] = useSearchParams()
  const toast = useToast()
  const [material, setMaterial] = useState<Material | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      setMaterial(await api.getMaterial(Number(id)))
    } catch (err) {
      const status = (err as { status?: number }).status
      if (status === 401) {
        onExpired()
        return
      }
      setError(describeError(err))
    } finally {
      setLoading(false)
    }
  }, [id, onExpired])

  useEffect(() => {
    void load()
  }, [load])

  // 从检索结果跳进来时，用字符区间把命中的片段标出来（spec R7.2）。
  const startParam = Number(params.get('start'))
  const endParam = Number(params.get('end'))
  const content = material?.content
  const hasHighlight =
    Number.isInteger(startParam) && Number.isInteger(endParam) && endParam > startParam && !!content
  const highlight = hasHighlight && content ? sliceByCodePoints(content, startParam, endParam) : ''

  async function download() {
    if (!material) return
    try {
      await api.download(material)
      toast('已开始下载', 'success')
    } catch (err) {
      if ((err as { status?: number }).status === 401) {
        onExpired()
        return
      }
      toast(describeError(err), 'error')
    }
  }

  return (
    <div className="app-shell">
      <header className="topbar">
        <div className="brand">
          <Link to="/" className="brand-mark">
            CampusClaw
          </Link>
          <span className="brand-sub">材料详情</span>
        </div>
        <div className="topbar-actions">
          <ThemeToggle />
          <Link to="/" className="ghost">
            返回列表
          </Link>
        </div>
      </header>
      <main className="content">
        {loading ? <p className="muted">正在加载…</p> : null}
        {error ? <p className="form-error">{error}</p> : null}
        {material ? (
          <article className="detail">
            <h1>{material.title}</h1>
            <p className="muted small">
              {material.original_name} · {(material.size_bytes / 1024).toFixed(1)} KB · 上传者 {material.uploader_name}
            </p>
            <button type="button" className="primary" onClick={() => void download()}>
              下载原文件
            </button>
            {highlight ? (
              <div className="highlight-box">
                <p className="muted small">来自检索结果的命中片段（字符 {startParam}–{endParam}）</p>
                <pre className="highlight-text">{highlight}</pre>
              </div>
            ) : null}
            <div className="markdown">
              {/* react-markdown 默认不解析内联 HTML，材料里的 <script> 只会以文本呈现。 */}
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{material.content ?? ''}</ReactMarkdown>
            </div>
          </article>
        ) : null}
      </main>
    </div>
  )
}
