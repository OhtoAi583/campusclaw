import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, describeError } from '../api'
import type { Identity, Material } from '../types'
import { ThemeToggle } from '../components/ThemeToggle'
import { UploadDialog } from '../components/UploadDialog'
import { CommandPalette, type Command } from '../components/CommandPalette'
import { useToast } from '../components/Toast'

interface Props {
  identity: Identity
  onExpired: () => void
}

export default function MaterialsPage({ identity, onExpired }: Props) {
  const toast = useToast()
  const navigate = useNavigate()
  const [materials, setMaterials] = useState<Material[]>([])
  const [loading, setLoading] = useState(true)
  const [query, setQuery] = useState('')
  const [view, setView] = useState<'list' | 'grid'>('list')
  const [uploadOpen, setUploadOpen] = useState(false)
  const [paletteOpen, setPaletteOpen] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const data = await api.listMaterials()
      setMaterials(data.items)
    } catch (error) {
      if ((error as { status?: number }).status === 401) {
        onExpired()
        return
      }
      toast(describeError(error), 'error')
    } finally {
      setLoading(false)
    }
  }, [onExpired, toast])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        setPaletteOpen((open) => !open)
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  // 搜索只在本班已加载的数据范围内过滤，不是跨班全文检索。
  const filtered = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    if (!keyword) return materials
    return materials.filter(
      (material) =>
        material.title.toLowerCase().includes(keyword) ||
        material.original_name.toLowerCase().includes(keyword) ||
        (material.content ?? '').toLowerCase().includes(keyword),
    )
  }, [materials, query])

  async function logout() {
    try {
      await api.logout()
    } finally {
      onExpired()
    }
  }

  async function download(material: Material) {
    try {
      await api.download(material)
      toast('已开始下载', 'success')
    } catch (error) {
      if ((error as { status?: number }).status === 401) {
        onExpired()
        return
      }
      toast(describeError(error), 'error')
    }
  }

  const commands: Command[] = [
    { label: '检索知识库', hint: '在本班材料里按内容检索', run: () => navigate('/search') },
    { label: '登出', hint: '清除服务端会话', run: () => void logout() },
    ...(identity.role === 'teacher'
      ? [{ label: '上传材料', hint: '教师可用', run: () => setUploadOpen(true) }]
      : []),
  ]

  return (
    <div className="app-shell">
      <header className="topbar">
        <div className="brand">
          <span className="brand-mark">CampusClaw</span>
          <span className="brand-sub">{identity.class_name} 班 · 教研材料</span>
        </div>
        <div className="topbar-actions">
          <span className="identity">
            {identity.username}（{identity.role === 'teacher' ? '教师' : '学生'}）
          </span>
          <ThemeToggle />
          <Link to="/search" className="ghost">
            检索
          </Link>
          {identity.role === 'teacher' ? (
            <button type="button" className="primary" onClick={() => setUploadOpen(true)}>
              上传材料
            </button>
          ) : null}
          <button type="button" className="ghost" onClick={() => void logout()}>
            登出
          </button>
        </div>
      </header>

      <main className="content">
        <div className="toolbar">
          <input
            className="search"
            placeholder="搜索本班材料标题或正文…"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
          <div className="view-toggle">
            <button type="button" className={view === 'list' ? 'active' : ''} onClick={() => setView('list')}>
              列表
            </button>
            <button type="button" className={view === 'grid' ? 'active' : ''} onClick={() => setView('grid')}>
              网格
            </button>
          </div>
        </div>

        {loading ? (
          <p className="muted">正在加载本班材料…</p>
        ) : filtered.length === 0 ? (
          <p className="muted">没有匹配的材料。</p>
        ) : (
          <ul className={`materials materials-${view}`}>
            {filtered.map((material) => (
              <li key={material.id} className="material-card">
                <div>
                  <Link to={`/materials/${material.id}`} className="material-title">
                    {material.title}
                  </Link>
                  <p className="muted small">
                    {material.original_name} · {(material.size_bytes / 1024).toFixed(1)} KB · {material.uploader_name}
                  </p>
                </div>
                <div className="material-actions">
                  <button type="button" className="ghost" onClick={() => void download(material)}>
                    下载
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </main>

      <UploadDialog open={uploadOpen} onClose={() => setUploadOpen(false)} onUploaded={() => void load()} />
      <CommandPalette
        open={paletteOpen}
        onClose={() => setPaletteOpen(false)}
        materials={materials}
        commands={commands}
        onOpenMaterial={(material) => navigate(`/materials/${material.id}`)}
      />
    </div>
  )
}
