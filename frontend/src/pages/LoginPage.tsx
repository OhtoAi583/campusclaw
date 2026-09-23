import { useState, type FormEvent } from 'react'
import { api, describeError } from '../api'
import type { Identity } from '../types'
import { ThemeToggle } from '../components/ThemeToggle'

export default function LoginPage({ onSuccess }: { onSuccess: (identity: Identity) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      onSuccess(await api.login(username, password))
    } catch (err) {
      // 登录失败只显示统一文案：不区分"用户不存在"与"口令错误"，避免账号枚举。
      setError(describeError(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="login-shell">
      <div className="login-card">
        <header>
          <span className="brand-mark">CampusClaw</span>
          <ThemeToggle />
        </header>
        <h1>登录教研材料库</h1>
        <p className="muted">教师可上传、查看、下载本班材料；学生可查看与下载。</p>
        <form onSubmit={submit}>
          <label>
            账号
            <input value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="username" required />
          </label>
          <label>
            口令
            <input
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete="current-password"
              required
            />
          </label>
          {error ? <p className="form-error">{error}</p> : null}
          <button type="submit" className="primary" disabled={busy}>
            {busy ? '登录中…' : '登录'}
          </button>
        </form>
      </div>
    </div>
  )
}
