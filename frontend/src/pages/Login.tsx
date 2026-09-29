import { Eye, EyeOff, KeyRound, LockKeyhole, Radio, ShieldCheck, Waypoints } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { api, ApiError } from '../api'
import { Button, Field, Input } from '../components/UI'
import type { AdminUser } from '../types'

export default function Login({ appName, version, onLogin }: { appName: string; version: string; onLogin: (user: AdminUser) => void }) {
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setLoading(true)
    setError('')
    try {
      const result = await api<{ user: AdminUser }>('/api/auth/login', {
        method: 'POST', body: { username, password },
      })
      onLogin(result.user)
    } catch (reason) {
      setError(reason instanceof ApiError ? reason.message : '无法连接到服务器')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-page">
      <div className="login-atmosphere atmosphere-one" />
      <div className="login-atmosphere atmosphere-two" />
      <section className="login-story">
        <div className="login-brand">
          <span className="brand-mark"><Radio size={22} /></span>
          <span><strong>{appName}</strong><small>ANTHROPIC GATEWAY</small></span>
        </div>
        <div className="story-copy">
          <span className="story-kicker"><i /> PRIVATE AI INFRASTRUCTURE</span>
          <h1>一个入口，<br />掌控每次 Claude 调用。</h1>
          <p>集中管理上游账号、访问密钥与成本，为团队提供稳定、透明且可审计的 Anthropic 兼容入口。</p>
        </div>
        <div className="story-features">
          <div><Waypoints size={19} /><span><strong>弹性路由</strong><small>多账号自动轮转与故障切换</small></span></div>
          <div><ShieldCheck size={19} /><span><strong>密钥隔离</strong><small>上游凭证加密存储</small></span></div>
          <div><KeyRound size={19} /><span><strong>精细权限</strong><small>模型白名单与月度配额</small></span></div>
        </div>
      </section>
      <section className="login-panel">
        <form className="login-card" onSubmit={submit}>
          <div className="login-icon"><LockKeyhole size={23} /></div>
          <header>
            <span>WELCOME BACK</span>
            <h2>登录管理控制台</h2>
            <p>输入管理员凭据以继续</p>
          </header>
          {error && <div className="form-alert" role="alert">{error}</div>}
          <Field label="用户名">
            <Input value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="username" required autoFocus />
          </Field>
          <Field label="密码">
            <div className="password-input">
              <Input type={showPassword ? 'text' : 'password'} value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" required />
              <button type="button" aria-label={showPassword ? '隐藏密码' : '显示密码'} onClick={() => setShowPassword(!showPassword)}>
                {showPassword ? <EyeOff size={17} /> : <Eye size={17} />}
              </button>
            </div>
          </Field>
          <Button variant="primary" loading={loading} type="submit" className="login-submit">进入控制台</Button>
          <div className="login-security"><ShieldCheck size={15} />会话使用 HttpOnly Cookie 保护</div>
        </form>
        <footer>Claude Relay · {version}</footer>
      </section>
    </div>
  )
}
