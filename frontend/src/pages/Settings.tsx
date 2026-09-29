import {
  Box,
  CheckCircle2,
  CirclePlus,
  Clock3,
  Coins,
  Database,
  Edit3,
  ExternalLink,
  FileClock,
  Gauge,
  LockKeyhole,
  Network,
  RefreshCw,
  Save,
  Settings2,
  ShieldCheck,
  Trash2,
} from 'lucide-react'
import { type FormEvent, useEffect, useState } from 'react'
import { api } from '../api'
import {
  Badge,
  Button,
  ConfirmDialog,
  Field,
  IconButton,
  Input,
  LoadingBlock,
  Modal,
  formatFullDate,
  formatNumber,
  formatUSD,
  useNotify,
} from '../components/UI'
import type { ModelPrice, Settings as SettingsType, SystemInfo } from '../types'

type SettingsTab = 'gateway' | 'pricing' | 'security' | 'system'

const emptyPrice: ModelPrice = {
  match: '', input_per_million_usd: 0, output_per_million_usd: 0,
  cache_read_per_million_usd: 0, cache_write_per_million_usd: 0,
}

export default function Settings({ onLogout }: { onLogout: () => void }) {
  const [tab, setTab] = useState<SettingsTab>('gateway')
  const [settings, setSettings] = useState<SettingsType | null>(null)
  const [prices, setPrices] = useState<ModelPrice[]>([])
  const [system, setSystem] = useState<SystemInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [priceEditing, setPriceEditing] = useState<ModelPrice | 'new' | null>(null)
  const [priceForm, setPriceForm] = useState<ModelPrice>(emptyPrice)
  const [priceSaving, setPriceSaving] = useState(false)
  const [priceDeleting, setPriceDeleting] = useState<ModelPrice | null>(null)
  const [password, setPassword] = useState({ current: '', next: '', confirm: '' })
  const [passwordBusy, setPasswordBusy] = useState(false)
  const [cleanupOpen, setCleanupOpen] = useState(false)
  const [cleanupBusy, setCleanupBusy] = useState(false)
  const notify = useNotify()

  const load = async () => {
    setLoading(true)
    try {
      const [settingsData, priceData, systemData] = await Promise.all([
        api<SettingsType>('/api/settings'),
        api<{ items: ModelPrice[] }>('/api/prices'),
        api<SystemInfo>('/api/system'),
      ])
      setSettings(settingsData)
      setPrices(priceData.items)
      setSystem(systemData)
    } catch (error) {
      notify({ type: 'error', title: '设置加载失败', description: error instanceof Error ? error.message : undefined })
    } finally {
      setLoading(false)
    }
  }
  useEffect(() => { void load() }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const saveSettings = async (event: FormEvent) => {
    event.preventDefault()
    if (!settings) return
    setSaving(true)
    try {
      const updated = await api<SettingsType>('/api/settings', { method: 'PUT', body: settings })
      setSettings(updated)
      window.dispatchEvent(new CustomEvent('relay:app-name', { detail: updated.app_name }))
      notify({ type: 'success', title: '网关设置已保存', description: '新请求会立即使用更新后的配置。' })
    } catch (error) {
      notify({ type: 'error', title: '保存失败', description: error instanceof Error ? error.message : undefined })
    } finally {
      setSaving(false)
    }
  }

  const editPrice = (price?: ModelPrice) => {
    setPriceForm(price ? { ...price } : { ...emptyPrice })
    setPriceEditing(price || 'new')
  }

  const savePrice = async (event: FormEvent) => {
    event.preventDefault()
    setPriceSaving(true)
    try {
      await api<ModelPrice>(`/api/prices/${encodeURIComponent(priceForm.match)}`, { method: 'PUT', body: priceForm })
      notify({ type: 'success', title: '计价规则已保存' })
      setPriceEditing(null)
      const response = await api<{ items: ModelPrice[] }>('/api/prices')
      setPrices(response.items)
    } catch (error) {
      notify({ type: 'error', title: '保存失败', description: error instanceof Error ? error.message : undefined })
    } finally {
      setPriceSaving(false)
    }
  }

  const deletePrice = async () => {
    if (!priceDeleting) return
    try {
      await api(`/api/prices/${encodeURIComponent(priceDeleting.match)}`, { method: 'DELETE' })
      setPrices((current) => current.filter((item) => item.match !== priceDeleting.match))
      setPriceDeleting(null)
      notify({ type: 'success', title: '计价规则已删除' })
    } catch (error) {
      notify({ type: 'error', title: '删除失败', description: error instanceof Error ? error.message : undefined })
    }
  }

  const changePassword = async (event: FormEvent) => {
    event.preventDefault()
    if (password.next !== password.confirm) {
      notify({ type: 'error', title: '两次输入的新密码不一致' })
      return
    }
    setPasswordBusy(true)
    try {
      await api('/api/auth/password', { method: 'PUT', body: { current_password: password.current, new_password: password.next } })
      notify({ type: 'success', title: '密码已修改，请重新登录' })
      window.setTimeout(onLogout, 800)
    } catch (error) {
      notify({ type: 'error', title: '密码修改失败', description: error instanceof Error ? error.message : undefined })
    } finally {
      setPasswordBusy(false)
    }
  }

  const cleanup = async () => {
    if (!settings) return
    setCleanupBusy(true)
    try {
      const result = await api<{ deleted: number }>(`/api/usage?older_than_days=${settings.log_retention_days}`, { method: 'DELETE' })
      notify({ type: 'success', title: `已清理 ${formatNumber(result.deleted)} 条过期记录` })
      setCleanupOpen(false)
      await load()
    } catch (error) {
      notify({ type: 'error', title: '清理失败', description: error instanceof Error ? error.message : undefined })
    } finally {
      setCleanupBusy(false)
    }
  }

  const navItems: { id: SettingsTab; label: string; icon: typeof Settings2 }[] = [
    { id: 'gateway', label: '网关策略', icon: Network },
    { id: 'pricing', label: '模型计价', icon: Coins },
    { id: 'security', label: '安全设置', icon: ShieldCheck },
    { id: 'system', label: '系统信息', icon: Database },
  ]

  return (
    <div className="page-stack">
      <div className="page-intro compact-intro"><div><span className="eyebrow">CONFIGURATION</span><h2>系统设置</h2><p>调整路由、容错、计价与管理端安全配置。</p></div><Button icon={<RefreshCw size={16} />} onClick={load}>重新载入</Button></div>
      <div className="settings-layout">
        <aside className="settings-nav">{navItems.map((item) => { const Icon = item.icon; return <button key={item.id} className={tab === item.id ? 'active' : ''} onClick={() => setTab(item.id)}><Icon size={17} />{item.label}</button> })}</aside>
        <section className="settings-content">
          {loading || !settings ? <div className="card settings-card"><LoadingBlock rows={8} /></div> : <>
            {tab === 'gateway' && <form className="card settings-card" onSubmit={saveSettings}>
              <header className="settings-section-header"><span className="settings-section-icon"><Gauge size={20} /></span><div><h3>网关与路由策略</h3><p>这些设置会热更新，无需重启容器。</p></div></header>
              <div className="settings-form-grid">
                <Field label="控制台名称" hint="显示在左上角和登录页"><Input value={settings.app_name} onChange={(event) => setSettings({ ...settings, app_name: event.target.value })} /></Field>
                <Field label="Anthropic API 版本"><Input value={settings.default_anthropic_version} onChange={(event) => setSettings({ ...settings, default_anthropic_version: event.target.value })} placeholder="2023-06-01" /></Field>
                <Field label="账号选择策略"><select className="select full" value={settings.routing_strategy} onChange={(event) => setSettings({ ...settings, routing_strategy: event.target.value as SettingsType['routing_strategy'] })}><option value="least_recently_used">最久未使用（推荐）</option><option value="priority">优先级优先</option><option value="weighted">加权随机</option></select></Field>
                <Field label="单次请求超时（秒）" hint="流式请求也受此上限约束"><Input type="number" min="10" max="3600" value={settings.request_timeout_seconds} onChange={(event) => setSettings({ ...settings, request_timeout_seconds: Number(event.target.value) })} /></Field>
                <Field label="失败重试次数" hint="每次重试会切换到另一个上游账号"><Input type="number" min="0" max="5" value={settings.max_retries} onChange={(event) => setSettings({ ...settings, max_retries: Number(event.target.value) })} /></Field>
                <Field label="连续失败阈值"><Input type="number" min="1" max="20" value={settings.failure_threshold} onChange={(event) => setSettings({ ...settings, failure_threshold: Number(event.target.value) })} /></Field>
                <Field label="基础冷却时间（秒）" hint="连续失败后按指数增加"><Input type="number" min="5" max="86400" value={settings.cooldown_seconds} onChange={(event) => setSettings({ ...settings, cooldown_seconds: Number(event.target.value) })} /></Field>
                <Field label="调用日志保留（天）"><Input type="number" min="1" max="3650" value={settings.log_retention_days} onChange={(event) => setSettings({ ...settings, log_retention_days: Number(event.target.value) })} /></Field>
                <Field label="管理会话时长（小时）"><Input type="number" min="1" max="720" value={settings.session_ttl_hours} onChange={(event) => setSettings({ ...settings, session_ttl_hours: Number(event.target.value) })} /></Field>
              </div>
              <footer className="settings-actions"><span><CheckCircle2 size={15} />配置保存在 SQLite 中</span><Button type="submit" variant="primary" icon={<Save size={16} />} loading={saving}>保存设置</Button></footer>
            </form>}

            {tab === 'pricing' && <div className="card settings-card">
              <header className="settings-section-header with-action"><span className="settings-section-icon"><Coins size={20} /></span><div><h3>模型计价规则</h3><p>用于本地估算费用；以模型 ID 前缀做最长匹配。</p></div><Button variant="primary" size="sm" icon={<CirclePlus size={15} />} onClick={() => editPrice()}>添加规则</Button></header>
              <div className="info-banner"><Coins size={17} /><span>价格仅用于网关内的成本估算，不会影响 Anthropic 实际账单。请在官方价格变化后及时更新。</span></div>
              <div className="table-scroll"><table className="data-table pricing-table"><thead><tr><th>模型前缀</th><th>输入 / MTok</th><th>输出 / MTok</th><th>缓存读取 / MTok</th><th>缓存写入 / MTok</th><th></th></tr></thead><tbody>{prices.map((price) => <tr key={price.match}><td><code>{price.match}*</code></td><td>{formatUSD(price.input_per_million_usd)}</td><td>{formatUSD(price.output_per_million_usd)}</td><td>{formatUSD(price.cache_read_per_million_usd)}</td><td>{formatUSD(price.cache_write_per_million_usd)}</td><td><div className="row-actions"><IconButton label="编辑规则" onClick={() => editPrice(price)}><Edit3 size={16} /></IconButton><IconButton label="删除规则" className="danger-hover" onClick={() => setPriceDeleting(price)}><Trash2 size={16} /></IconButton></div></td></tr>)}</tbody></table></div>
            </div>}

            {tab === 'security' && <div className="settings-section-stack">
              <form className="card settings-card" onSubmit={changePassword}>
                <header className="settings-section-header"><span className="settings-section-icon"><LockKeyhole size={20} /></span><div><h3>修改管理员密码</h3><p>修改后所有现有管理会话都会立即退出。</p></div></header>
                <div className="password-form">
                  <Field label="当前密码"><Input type="password" autoComplete="current-password" value={password.current} onChange={(event) => setPassword({ ...password, current: event.target.value })} required /></Field>
                  <Field label="新密码" hint="至少 12 个字符"><Input type="password" minLength={12} autoComplete="new-password" value={password.next} onChange={(event) => setPassword({ ...password, next: event.target.value })} required /></Field>
                  <Field label="确认新密码"><Input type="password" minLength={12} autoComplete="new-password" value={password.confirm} onChange={(event) => setPassword({ ...password, confirm: event.target.value })} required /></Field>
                </div>
                <footer className="settings-actions"><span><ShieldCheck size={15} />密码使用 PBKDF2-SHA256 存储</span><Button type="submit" variant="primary" loading={passwordBusy}>修改密码</Button></footer>
              </form>
              <div className="card settings-card"><header className="settings-section-header"><span className="settings-section-icon"><ShieldCheck size={20} /></span><div><h3>部署安全检查</h3><p>上线前建议逐项确认。</p></div></header><div className="security-checklist"><div><CheckCircle2 size={17} /><span><strong>MASTER_KEY</strong><small>生产环境使用固定的高强度随机值</small></span></div><div><CheckCircle2 size={17} /><span><strong>HTTPS</strong><small>在 Caddy / Nginx 后部署并设置 COOKIE_SECURE=true</small></span></div><div><CheckCircle2 size={17} /><span><strong>网络边界</strong><small>管理控制台限制为内网或可信 IP 访问</small></span></div></div></div>
            </div>}

            {tab === 'system' && <div className="settings-section-stack">
              <div className="card settings-card"><header className="settings-section-header"><span className="settings-section-icon"><Box size={20} /></span><div><h3>运行信息</h3><p>当前容器和数据存储状态。</p></div></header>{system && <div className="system-grid"><div><span>Claude Relay</span><strong>{system.version}</strong></div><div><span>Go Runtime</span><strong>{system.go_version}</strong></div><div><span>启动时间</span><strong>{formatFullDate(system.started_at)}</strong></div><div><span>数据库</span><strong className="mono truncate">{system.database_path}</strong></div><div><span>上游账号</span><strong>{formatNumber(system.stats.accounts || 0)}</strong></div><div><span>API Keys</span><strong>{formatNumber(system.stats.api_keys || 0)}</strong></div><div><span>调用记录</span><strong>{formatNumber(system.stats.usage_logs || 0)}</strong></div></div>}</div>
              <div className="card settings-card danger-zone"><header className="settings-section-header"><span className="settings-section-icon danger"><FileClock size={20} /></span><div><h3>数据维护</h3><p>系统每 6 小时自动清理超过保留期的调用记录。</p></div></header><div className="maintenance-row"><div><strong>立即清理过期调用记录</strong><span>删除早于 {settings.log_retention_days} 天的调用明细，累计计数不会重置。</span></div><Button variant="danger" icon={<Trash2 size={15} />} onClick={() => setCleanupOpen(true)}>立即清理</Button></div></div>
            </div>}
          </>}
        </section>
      </div>

      <Modal open={priceEditing !== null} onClose={() => setPriceEditing(null)} title={priceEditing === 'new' ? '添加计价规则' : '编辑计价规则'} size="md" footer={<><Button onClick={() => setPriceEditing(null)}>取消</Button><Button variant="primary" loading={priceSaving} onClick={() => document.getElementById('price-form-submit')?.click()}>保存规则</Button></>}>
        <form className="form-grid" onSubmit={savePrice}>
          <Field label="模型 ID 前缀" hint="例如 claude-sonnet，系统会匹配其后的所有版本" className="span-2"><Input value={priceForm.match} disabled={priceEditing !== 'new'} onChange={(event) => setPriceForm({ ...priceForm, match: event.target.value })} placeholder="claude-sonnet" required /></Field>
          <Field label="输入 / 百万 Token (USD)"><Input type="number" min="0" step="0.0001" value={priceForm.input_per_million_usd} onChange={(event) => setPriceForm({ ...priceForm, input_per_million_usd: Number(event.target.value) })} /></Field>
          <Field label="输出 / 百万 Token (USD)"><Input type="number" min="0" step="0.0001" value={priceForm.output_per_million_usd} onChange={(event) => setPriceForm({ ...priceForm, output_per_million_usd: Number(event.target.value) })} /></Field>
          <Field label="缓存读取 / 百万 Token"><Input type="number" min="0" step="0.0001" value={priceForm.cache_read_per_million_usd} onChange={(event) => setPriceForm({ ...priceForm, cache_read_per_million_usd: Number(event.target.value) })} /></Field>
          <Field label="缓存写入 / 百万 Token"><Input type="number" min="0" step="0.0001" value={priceForm.cache_write_per_million_usd} onChange={(event) => setPriceForm({ ...priceForm, cache_write_per_million_usd: Number(event.target.value) })} /></Field>
          <button id="price-form-submit" type="submit" hidden />
        </form>
      </Modal>

      <ConfirmDialog open={!!priceDeleting} onClose={() => setPriceDeleting(null)} onConfirm={deletePrice} danger title="删除计价规则？" confirmLabel="删除规则" description={`删除「${priceDeleting?.match || ''}」后，匹配这些模型的调用可能无法估算费用。`} />
      <ConfirmDialog open={cleanupOpen} onClose={() => setCleanupOpen(false)} onConfirm={cleanup} loading={cleanupBusy} danger title="清理过期记录？" confirmLabel="开始清理" description={`将永久删除早于 ${settings?.log_retention_days || 0} 天的调用明细。账号和 API Key 的累计统计不会被重置。`} />
    </div>
  )
}
