import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { api, describeError } from '../api'
import type { Material } from '../types'
import { ThemeToggle } from '../components/ThemeToggle'
import { useToast } from '../components/Toast'

export default function MaterialDetailPage({ onExpired }: { onExpired: () => void }) {
  const { id } = useParams()
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
