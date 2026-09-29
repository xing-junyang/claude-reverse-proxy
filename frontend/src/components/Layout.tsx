import {
  Activity,
  ChevronLeft,
  ChevronRight,
  CircleGauge,
  KeyRound,
  LogOut,
  Menu,
  Moon,
  Radio,
  Settings,
  Sun,
  UsersRound,
  X,
} from 'lucide-react'
import { type ReactNode, useEffect, useState } from 'react'
import type { AdminUser } from '../types'
import { IconButton } from './UI'

export type Tab = 'overview' | 'accounts' | 'usage' | 'keys' | 'settings'

const navigation: { id: Tab; label: string; icon: typeof CircleGauge; eyebrow: string }[] = [
  { id: 'overview', label: '运行概览', icon: CircleGauge, eyebrow: 'Overview' },
  { id: 'accounts', label: '上游账号', icon: UsersRound, eyebrow: 'Accounts' },
  { id: 'usage', label: '调用与用量', icon: Activity, eyebrow: 'Observability' },
  { id: 'keys', label: 'API Keys', icon: KeyRound, eyebrow: 'Access' },
  { id: 'settings', label: '系统设置', icon: Settings, eyebrow: 'Configuration' },
]

export default function Layout({ tab, onTab, user, appName, onLogout, children }: {
  tab: Tab
  onTab: (tab: Tab) => void
  user: AdminUser
  appName: string
  onLogout: () => void
  children: ReactNode
}) {
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem('relay:sidebar') === 'collapsed')
  const [mobileOpen, setMobileOpen] = useState(false)
  const [theme, setTheme] = useState<'light' | 'dark'>(() => {
    const saved = localStorage.getItem('relay:theme')
    if (saved === 'light' || saved === 'dark') return saved
    return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
  })

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    localStorage.setItem('relay:theme', theme)
  }, [theme])

  const navigate = (next: Tab) => {
    onTab(next)
    setMobileOpen(false)
  }

  const current = navigation.find((item) => item.id === tab)!

  return (
    <div className={`app-shell ${collapsed ? 'sidebar-collapsed' : ''}`}>
      {mobileOpen && <button className="sidebar-scrim" aria-label="关闭菜单" onClick={() => setMobileOpen(false)} />}
      <aside className={`sidebar ${mobileOpen ? 'mobile-open' : ''}`}>
        <div className="brand">
          <div className="brand-mark"><Radio size={20} /></div>
          <div className="brand-copy">
            <strong>{appName}</strong>
            <span>ANTHROPIC GATEWAY</span>
          </div>
          <IconButton label="关闭菜单" className="mobile-close" onClick={() => setMobileOpen(false)}><X size={18} /></IconButton>
        </div>
        <nav className="side-nav" aria-label="主导航">
          <span className="nav-section-label">Workspace</span>
          {navigation.map((item) => {
            const Icon = item.icon
            return (
              <button key={item.id} className={`nav-item ${tab === item.id ? 'active' : ''}`} onClick={() => navigate(item.id)} title={collapsed ? item.label : undefined}>
                <Icon size={18} strokeWidth={1.8} />
                <span className="nav-copy"><strong>{item.label}</strong><small>{item.eyebrow}</small></span>
                {tab === item.id && <i className="nav-indicator" />}
              </button>
            )
          })}
        </nav>
        <div className="sidebar-footer">
          <div className="gateway-status">
            <span className="pulse-dot" />
            <div><strong>Gateway online</strong><small>服务运行正常</small></div>
          </div>
          <button
            className="collapse-button"
            onClick={() => {
              setCollapsed((value) => {
                localStorage.setItem('relay:sidebar', value ? 'expanded' : 'collapsed')
                return !value
              })
            }}
            aria-label={collapsed ? '展开侧边栏' : '收起侧边栏'}
          >
            {collapsed ? <ChevronRight size={16} /> : <ChevronLeft size={16} />}
            <span>收起侧栏</span>
          </button>
        </div>
      </aside>
      <div className="main-column">
        <header className="topbar">
          <div className="topbar-title">
            <IconButton label="打开菜单" className="menu-button" onClick={() => setMobileOpen(true)}><Menu size={20} /></IconButton>
            <div><span>{current.eyebrow}</span><h1>{current.label}</h1></div>
          </div>
          <div className="topbar-actions">
            <div className="live-chip"><span className="pulse-dot" />Live</div>
            <IconButton label={theme === 'dark' ? '切换浅色主题' : '切换深色主题'} onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}>
              {theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
            </IconButton>
            <div className="user-menu">
              <span className="avatar">{user.username.slice(0, 1).toUpperCase()}</span>
              <div><strong>{user.username}</strong><small>Administrator</small></div>
              <IconButton label="退出登录" onClick={onLogout}><LogOut size={17} /></IconButton>
            </div>
          </div>
        </header>
        <main className="content">{children}</main>
      </div>
    </div>
  )
}
