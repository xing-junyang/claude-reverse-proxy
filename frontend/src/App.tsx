import { Radio } from 'lucide-react'
import { useEffect, useState } from 'react'
import { api } from './api'
import Layout, { type Tab } from './components/Layout'
import { ToastProvider } from './components/UI'
import Accounts from './pages/Accounts'
import Dashboard from './pages/Dashboard'
import Keys from './pages/Keys'
import Login from './pages/Login'
import Settings from './pages/Settings'
import Usage from './pages/Usage'
import type { AdminUser } from './types'

const tabs: Tab[] = ['overview', 'accounts', 'usage', 'keys', 'settings']

function tabFromURL(): Tab {
  const value = new URLSearchParams(window.location.search).get('tab') as Tab | null
  return value && tabs.includes(value) ? value : 'overview'
}

function AppContent() {
  const [loading, setLoading] = useState(true)
  const [user, setUser] = useState<AdminUser | null>(null)
  const [appName, setAppName] = useState('Claude Relay')
  const [version, setVersion] = useState('dev')
  const [tab, setTab] = useState<Tab>(tabFromURL)

  useEffect(() => {
    const savedTheme = localStorage.getItem('relay:theme')
    document.documentElement.dataset.theme = savedTheme === 'light' ? 'light' : 'dark'
    Promise.allSettled([
      api<{ app_name: string; version: string }>('/api/public/config'),
      api<{ user: AdminUser; app_name: string; version: string }>('/api/auth/me'),
    ]).then(([publicResult, authResult]) => {
      if (publicResult.status === 'fulfilled') {
        setAppName(publicResult.value.app_name)
        setVersion(publicResult.value.version)
      }
      if (authResult.status === 'fulfilled') {
        setUser(authResult.value.user)
        setAppName(authResult.value.app_name)
        setVersion(authResult.value.version)
      }
      setLoading(false)
    })
  }, [])

  useEffect(() => {
    const onExpired = () => setUser(null)
    const onName = (event: Event) => setAppName((event as CustomEvent<string>).detail)
    const onPopState = () => setTab(tabFromURL())
    window.addEventListener('relay:session-expired', onExpired)
    window.addEventListener('relay:app-name', onName)
    window.addEventListener('popstate', onPopState)
    return () => {
      window.removeEventListener('relay:session-expired', onExpired)
      window.removeEventListener('relay:app-name', onName)
      window.removeEventListener('popstate', onPopState)
    }
  }, [])

  const navigate = (next: Tab) => {
    setTab(next)
    const url = new URL(window.location.href)
    if (next === 'overview') url.searchParams.delete('tab')
    else url.searchParams.set('tab', next)
    window.history.replaceState({}, '', url)
  }

  const logout = async () => {
    try { await api('/api/auth/logout', { method: 'POST' }) } catch { /* cookie still cleared by expiry flow */ }
    setUser(null)
  }

  if (loading) {
    return <div className="app-loading"><span className="brand-mark"><Radio size={22} /></span><div className="loading-wordmark">Claude Relay</div><div className="loading-line"><i /></div></div>
  }

  if (!user) return <Login appName={appName} version={version} onLogin={setUser} />

  return (
    <Layout tab={tab} onTab={navigate} user={user} appName={appName} onLogout={logout}>
      {tab === 'overview' && <Dashboard onNavigate={navigate} />}
      {tab === 'accounts' && <Accounts />}
      {tab === 'usage' && <Usage />}
      {tab === 'keys' && <Keys />}
      {tab === 'settings' && <Settings onLogout={logout} />}
    </Layout>
  )
}

export default function App() {
  return <ToastProvider><AppContent /></ToastProvider>
}
