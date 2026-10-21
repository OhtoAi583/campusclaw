import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { Identity } from '../types'
import { ThemeToggle } from '../components/ThemeToggle'
import { useToast } from '../components/Toast'

interface Skill {
  id: string
  name: string
  description: string
  builtin: boolean
  size_chars: number
  source_filename: string
}

interface Template {
  id: string
  name: string
  system_template: string
  user_template: string
  variables: string
}

interface MCPServer {
  id: string
  name: string
  command: string
  args: string
}

interface App {
  id: string
  name: string
  description: string
  icon: string
  model: string
  temperature: number
  max_tokens: number
  kb_enabled: boolean
  kb_top_k: number
  template: Template
  skills: Skill[] | null
  mcp_servers: MCPServer[] | null
}

interface ChatResult {
  answer: string
  citations: Array<{ index: number; material_title: string; chunk_index: number; start_offset: number; end_offset: number; snippet: string }>
  conversation_id: string
  meta: Record<string, unknown>
}

interface LLMCall {
  id: number
  request: Record<string, unknown>
  response: Record<string, unknown>
  tools: unknown
  [k: string]: unknown
}

export default function PlatformPage({ identity, onExpired }: { identity: Identity; onExpired: () => void }) {
  const toast = useToast()
  const [apps, setApps] = useState<App[]>([])
  const [activeId, setActiveId] = useState('')
  const [question, setQuestion] = useState('导入环节为什么要用情境图？')
  const [result, setResult] = useState<ChatResult | null>(null)
  const [busy, setBusy] = useState(false)
  const [calls, setCalls] = useState<LLMCall[]>([])
  const [tab, setTab] = useState<'templates' | 'skills' | 'mcp'>('templates')
  const [templates, setTemplates] = useState<Template[]>([])
  const [skills, setSkills] = useState<Skill[]>([])
  const [mcpServers, setMcpServers] = useState<MCPServer[]>([])
  const [mcpTools, setMcpTools] = useState<Record<string, unknown[]>>({})

  const load = useCallback(async () => {
    try {
      const [a, t, s, m] = await Promise.all([
        api.get<{ items: App[] }>('/api/apps'),
        api.get<{ items: Template[] }>('/api/templates'),
        api.get<{ items: Skill[] }>('/api/skills'),
        api.get<{ items: MCPServer[] }>('/api/mcp/servers'),
      ])
      setApps(a.items)
      setTemplates(t.items)
      setSkills(s.items)
      setMcpServers(m.items)
      if (a.items[0]) setActiveId((cur) => cur || a.items[0].id)
    } catch (e) {
      if ((e as { status?: number }).status === 401) onExpired()
      else toast('平台配置加载失败', 'error')
    }
  }, [onExpired, toast])

  useEffect(() => { void load() }, [load])

  const active = apps.find((a) => a.id === activeId)

  async function send() {
    if (!active || !question.trim()) return
    setBusy(true)
    setResult(null)
    setCalls([])
    try {
      const r = await api.post<ChatResult>('/api/chat', { app_id: active.id, question })
      setResult(r)
      const ids = (r.meta.llm_call_ids as number[]) || []
      const details = await Promise.all(ids.map((id) => api.get<LLMCall>(`/api/llm-calls/${id}`)))
      setCalls(details)
    } catch (e) {
      if ((e as { status?: number }).status === 401) onExpired()
      else toast('对话失败：' + ((e as Error).message || ''), 'error')
    } finally {
      setBusy(false)
    }
  }

  async function loadTools(id: string) {
    try {
      const d = await api.get<{ tools: unknown[] }>(`/api/mcp/servers/${id}/tools`)
      setMcpTools((cur) => ({ ...cur, [id]: d.tools }))
    } catch {
      toast('MCP 工具读取失败', 'error')
    }
  }

  async function uploadSkill(file: File) {
    const form = new FormData()
    form.append('file', file)
    try {
      await api.postForm('/api/skills', form)
      toast('技能已上传', 'success')
      void load()
    } catch {
      toast('技能上传失败（仅支持 .md）', 'error')
    }
  }

  return (
    <div className="app-shell">
      <header className="topbar">
        <div className="brand">
          <Link to="/" className="brand-mark">CampusClaw</Link>
          <span className="brand-sub">{identity.class_name} 班 · AI 应用平台</span>
        </div>
        <div className="topbar-actions">
          <span className="identity">{identity.username}（{identity.role === 'teacher' ? '教师' : '学生'}）</span>
          <ThemeToggle />
          <Link to="/" className="ghost">返回材料</Link>
        </div>
      </header>

      <main className="content platform">
        <section className="apps">
          {apps.map((a) => (
            <button key={a.id} type="button" className={`app-card ${a.id === activeId ? 'active' : ''}`} onClick={() => { setActiveId(a.id); setResult(null); setCalls([]) }}>
              <span className="app-icon">{a.icon}</span>
              <span className="app-name">{a.name}</span>
              <span className="muted small">{a.description}</span>
            </button>
          ))}
        </section>

        {active && (
          <>
            <section className="panel">
              <div className="panel-head">
                <h2>{active.icon} {active.name}</h2>
                <div className="chips">
                  <span className="chip">模型 {active.model}</span>
                  <span className="chip">T={active.temperature}</span>
                  <span className="chip">max_tokens={active.max_tokens}</span>
                  <span className="chip">知识库 {active.kb_enabled ? `top-${active.kb_top_k}` : '关闭'}</span>
                  <span className="chip">技能 {(active.skills || []).map((s) => s.name).join('、') || '无'}</span>
                  <span className="chip">MCP {(active.mcp_servers || []).map((m) => m.name).join('、') || '无'}</span>
                </div>
              </div>
              <div className="ask-row">
                <input value={question} onChange={(e) => setQuestion(e.target.value)} placeholder="向这个应用提问…" />
                <button type="button" className="primary" onClick={() => void send()} disabled={busy}>{busy ? '生成中…' : '发送'}</button>
              </div>

              {result && (
                <div className="answer-block">
                  <div className="answer-text">{result.answer}</div>
                  <div className="chips">
                    {Object.entries(result.meta).filter(([k]) => ['kb_chunks', 'kb_total_chunks', 'kb_chars', 'skill_chars', 'tool_rounds', 'latency_ms'].includes(k)).map(([k, v]) => (
                      <span className="chip chip-meta" key={k}>{k}={String(v)}</span>
                    ))}
                  </div>
                  {result.citations.length > 0 && (
                    <ol className="citations">
                      {result.citations.map((c) => (
                        <li key={c.index}>
                          <b>[{c.index}]</b> {c.material_title} · 第 {c.chunk_index + 1} 块 · 字符 {c.start_offset}–{c.end_offset}
                          <div className="muted small">{c.snippet.slice(0, 80)}…</div>
                        </li>
                      ))}
                    </ol>
                  )}
                </div>
              )}
            </section>

            {calls.length > 0 && (
              <section className="panel">
                <h2>本次对话发给 LLM 的原始报文（{calls.length} 次调用）</h2>
                {calls.map((c) => (
                  <details key={c.id} className="call" open={calls.length === 1}>
                    <summary>
                      调用 #{c.id} · round={String(c.round_index)} · tools={Array.isArray(c.tools) ? c.tools.length : 0} · {String(c.latency_ms)}ms
                    </summary>
                    <div className="call-grid">
                      <div>
                        <h3>request（发给 /chat/completions 的完整 body）</h3>
                        <pre>{JSON.stringify(c.request, null, 2)}</pre>
                      </div>
                      <div>
                        <h3>response（网关返回）</h3>
                        <pre>{JSON.stringify(c.response, null, 2)}</pre>
                      </div>
                    </div>
                  </details>
                ))}
              </section>
            )}
          </>
        )}

        <section className="panel">
          <div className="tabs">
            <button type="button" className={tab === 'templates' ? 'active' : ''} onClick={() => setTab('templates')}>提示词模板 {templates.length}</button>
            <button type="button" className={tab === 'skills' ? 'active' : ''} onClick={() => setTab('skills')}>技能 {skills.length}</button>
            <button type="button" className={tab === 'mcp' ? 'active' : ''} onClick={() => setTab('mcp')}>MCP 服务器 {mcpServers.length}</button>
          </div>

          {tab === 'templates' && (
            <div className="list">
              {templates.map((t) => (
                <details key={t.id} className="item">
                  <summary><b>{t.name}</b> <code>{t.id}</code> <span className="muted small">变量：{t.variables}</span></summary>
                  <h4>system</h4><pre>{t.system_template}</pre>
                  <h4>user</h4><pre>{t.user_template}</pre>
                </details>
              ))}
            </div>
          )}

          {tab === 'skills' && (
            <div className="list">
              {identity.role === 'teacher' && (
                <label className="upload-skill">
                  上传技能（.md）：
                  <input type="file" accept=".md" onChange={(e) => { const f = e.target.files?.[0]; if (f) void uploadSkill(f) }} />
                </label>
              )}
              {skills.map((s) => (
                <div key={s.id} className="item">
                  <b>{s.name}</b> <code>{s.id}</code>{' '}
                  <span className="chip">{s.builtin ? '内置' : '上传'}</span>{' '}
                  <span className="muted small">{s.size_chars} 字 · {s.description}</span>
                </div>
              ))}
            </div>
          )}

          {tab === 'mcp' && (
            <div className="list">
              {mcpServers.map((m) => (
                <div key={m.id} className="item">
                  <b>{m.name}</b> <code>{m.id}</code> <span className="muted small">{m.command} {m.args}</span>
                  <button type="button" className="ghost" onClick={() => void loadTools(m.id)}>查看工具</button>
                  {mcpTools[m.id] && (
                    <pre>{JSON.stringify(mcpTools[m.id], null, 2)}</pre>
                  )}
                </div>
              ))}
            </div>
          )}
        </section>
      </main>
    </div>
  )
}
