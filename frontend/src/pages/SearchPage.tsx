import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, describeError } from '../api'
import type { Identity, SearchResult } from '../types'
import { ThemeToggle } from '../components/ThemeToggle'
import { useToast } from '../components/Toast'

interface Props {
  identity: Identity
  onExpired: () => void
}

export default function SearchPage({ identity, onExpired }: Props) {
  const toast = useToast()
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<SearchResult[] | null>(null)
  const [busy, setBusy] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  const run = useCallback(
    async (text: string) => {
      const trimmed = text.trim()
      if (!trimmed) return
      setBusy(true)
      try {
        const data = await api.search(trimmed, 5)
        setResults(data.items)
      } catch (error) {
        if ((error as { status?: number }).status === 401) {
          onExpired()
          return
        }
        toast(describeError(error), 'error')
      } finally {
        setBusy(false)
      }
    },
    [onExpired, toast],
  )

  async function logout() {
    try {
      await api.logout()
    } finally {
      onExpired()
    }
  }

  return (
    <div className="app-shell">
      <header className="topbar">
        <div className="brand">
          <Link to="/" className="brand-mark">
            CampusClaw
          </Link>
          <span className="brand-sub">{identity.class_name} 班 · 知识库检索</span>
        </div>
        <div className="topbar-actions">
          <span className="identity">
            {identity.username}（{identity.role === 'teacher' ? '教师' : '学生'}）
          </span>
          <ThemeToggle />
          <Link to="/" className="ghost">
            返回材料
          </Link>
          <button type="button" className="ghost" onClick={() => void logout()}>
            登出
          </button>
        </div>
      </header>

      <main className="content">
        <form
          className="search-form"
          onSubmit={(event) => {
            event.preventDefault()
            void run(query)
          }}
        >
          <input
            ref={inputRef}
            value={query}
            placeholder="在本班材料里检索，例如：阅读课的导入环节"
            onChange={(event) => setQuery(event.target.value)}
          />
          <button type="submit" className="primary" disabled={busy}>
            {busy ? '检索中…' : '检索'}
          </button>
        </form>
        <p className="muted small">
          检索范围固定为 {identity.class_name} 班；结果可点进材料查看出处。
        </p>

        {results === null ? null : results.length === 0 ? (
          <p className="muted">没有找到相关内容。</p>
        ) : (
          <ul className="results">
            {results.map((item) => (
              <li key={`${item.material_id}-${item.chunk_index}`} className="result-card">
                <div className="result-head">
                  <span className="result-title">{item.material_title}</span>
                  <span className="result-score">相关度 {(item.score * 100).toFixed(1)}%</span>
                </div>
                <p className="result-snippet">{item.content}</p>
                <p className="muted small">
                  出处：{item.original_name} · 第 {item.chunk_index + 1} 块 · 字符 {item.start_offset}–
                  {item.end_offset}
                </p>
                <button
                  type="button"
                  className="ghost"
                  onClick={() =>
                    navigate(
                      `/materials/${item.material_id}?start=${item.start_offset}&end=${item.end_offset}`,
                    )
                  }
                >
                  查看出处
                </button>
              </li>
            ))}
          </ul>
        )}
      </main>
    </div>
  )
}
