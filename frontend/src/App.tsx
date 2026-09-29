import { useCallback, useEffect, useState } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { api } from './api'
import type { Identity } from './types'
import { ToastProvider } from './components/Toast'
import LoginPage from './pages/LoginPage'
import MaterialsPage from './pages/MaterialsPage'
import MaterialDetailPage from './pages/MaterialDetailPage'
import SearchPage from './pages/SearchPage'

export default function App() {
  const [identity, setIdentity] = useState<Identity | null>(null)
  const [loading, setLoading] = useState(true)

  // 刷新或重开标签页时，身份一律以服务端 /api/me 为准，
  // 不读 localStorage 里的角色——那只是客户端声明，不能作为授权依据。
  const refresh = useCallback(async () => {
    try {
      setIdentity(await api.me())
    } catch {
      setIdentity(null)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  if (loading) {
    return <div className="boot">正在确认登录状态…</div>
  }

  return (
    <ToastProvider>
      <Routes>
        <Route
          path="/login"
          element={identity ? <Navigate to="/" replace /> : <LoginPage onSuccess={setIdentity} />}
        />
        <Route
          path="/"
          element={identity ? <MaterialsPage identity={identity} onExpired={() => setIdentity(null)} /> : <Navigate to="/login" replace />}
        />
        <Route
          path="/search"
          element={identity ? <SearchPage identity={identity} onExpired={() => setIdentity(null)} /> : <Navigate to="/login" replace />}
        />
        <Route
          path="/materials/:id"
          element={identity ? <MaterialDetailPage onExpired={() => setIdentity(null)} /> : <Navigate to="/login" replace />}
        />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </ToastProvider>
  )
}
