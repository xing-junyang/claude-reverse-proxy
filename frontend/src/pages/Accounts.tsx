import {
  Activity,
  CirclePlus,
  CloudCog,
  Edit3,
  ExternalLink,
  MoreHorizontal,
  Power,
  RefreshCw,
  Server,
  ShieldCheck,
  Trash2,
  Wifi,
  WifiOff,
} from 'lucide-react'
import { type FormEvent, useEffect, useMemo, useState } from 'react'
import { api } from '../api'
import {
  Badge,
  Button,
  ConfirmDialog,
  EmptyState,
  Field,
  IconButton,
  Input,
  LoadingBlock,
  Modal,
  Toggle,
  formatDate,
  formatNumber,
  formatUSD,
  statusTone,
  useNotify,
} from '../components/UI'
import type { Account, AccountInput } from '../types'

const blankAccount: AccountInput = {
  name: '', base_url: 'https://api.anthropic.com', api_key: '', enabled: true,
  priority: 0, weight: 1, allowed_models: [], monthly_limit_usd: 0, notes: '',
}

const statusLabel: Record<string, string> = {
  healthy: '健康', unknown: '待检测', degraded: '异常', cooldown: '冷却中', disabled: '已停用',
}

export default function Accounts() {
  const [items, setItems] = useState<Account[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [editing, setEditing] = useState<Account | null | 'new'>(null)
  const [form, setForm] = useState<AccountInput>(blankAccount)
  const [modelsText, setModelsText] = useState('')
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState<number | null>(null)
  const [deleting, setDeleting] = useState<Account | null>(null)
  const [deletingBusy, setDeletingBusy] = useState(false)
  const notify = useNotify()

  const load = async (quiet = false) => {
    quiet ? setRefreshing(true) : setLoading(true)
    try {
      const response = await api<{ items: Account[] }>('/api/accounts')
      setItems(response.items)
    } catch (error) {
      notify({ type: 'error', title: '账号列表加载失败', description: error instanceof Error ? error.message : undefined })
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }

  useEffect(() => { void load() }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const stats = useMemo(() => ({
    healthy: items.filter((item) => item.enabled && item.status !== 'disabled').length,
    requests: items.reduce((sum, item) => sum + item.request_count, 0),
    cost: items.reduce((sum, item) => sum + item.spent_usd, 0),
  }), [items])

  const openCreate = () => {
    setForm({ ...blankAccount })
    setModelsText('')
    setEditing('new')
  }

  const openEdit = (account: Account) => {
    setForm({
      name: account.name, base_url: account.base_url, api_key: '', enabled: account.enabled,
      priority: account.priority, weight: account.weight, allowed_models: account.allowed_models,
      monthly_limit_usd: account.monthly_limit_usd, notes: account.notes,
    })
    setModelsText(account.allowed_models.join('\n'))
    setEditing(account)
  }

  const save = async (event: FormEvent) => {
    event.preventDefault()
    setSaving(true)
    const payload = {
      ...form,
      allowed_models: modelsText.split(/[\n,]/).map((value) => value.trim()).filter(Boolean),
    }
    try {
      if (editing === 'new') {
        await api<Account>('/api/accounts', { method: 'POST', body: payload })
        notify({ type: 'success', title: '上游账号已添加', description: '建议立即执行一次连接检测。' })
      } else if (editing) {
        await api<Account>(`/api/accounts/${editing.id}`, { method: 'PUT', body: payload })
        notify({ type: 'success', title: '账号配置已保存' })
      }
      setEditing(null)
      await load(true)
    } catch (error) {
      notify({ type: 'error', title: '保存失败', description: error instanceof Error ? error.message : undefined })
    } finally {
      setSaving(false)
    }
  }

  const toggle = async (account: Account) => {
    try {
      const updated = await api<Account>(`/api/accounts/${account.id}/toggle`, { method: 'POST', body: { enabled: !account.enabled } })
      setItems((current) => current.map((item) => item.id === account.id ? updated : item))
      notify({ type: 'success', title: updated.enabled ? '账号已启用' : '账号已停用' })
    } catch (error) {
      notify({ type: 'error', title: '状态更新失败', description: error instanceof Error ? error.message : undefined })
    }
  }

  const test = async (account: Account) => {
    setTesting(account.id)
    try {
      const result = await api<{ ok: boolean; latency_ms: number; models?: number; message: string }>(`/api/accounts/${account.id}/test`, { method: 'POST' })
      notify({
        type: result.ok ? 'success' : 'error',
        title: result.ok ? `连接正常 · ${result.latency_ms} ms` : '连接检测失败',
        description: result.ok ? `发现 ${result.models || 0} 个可用模型` : result.message,
      })
      await load(true)
    } catch (error) {
      notify({ type: 'error', title: '检测失败', description: error instanceof Error ? error.message : undefined })
    } finally {
      setTesting(null)
    }
  }

  const remove = async () => {
    if (!deleting) return
    setDeletingBusy(true)
    try {
      await api(`/api/accounts/${deleting.id}`, { method: 'DELETE' })
      notify({ type: 'success', title: '账号已删除' })
      setDeleting(null)
      await load(true)
    } catch (error) {
      notify({ type: 'error', title: '删除失败', description: error instanceof Error ? error.message : undefined })
    } finally {
      setDeletingBusy(false)
    }
  }

  return (
    <div className="page-stack">
      <div className="page-intro compact-intro">
        <div><span className="eyebrow">UPSTREAM POOL</span><h2>上游账号</h2><p>集中维护 Anthropic 凭证，按健康度与策略自动承接流量。</p></div>
        <div className="intro-actions"><Button icon={<RefreshCw className={refreshing ? 'spin' : ''} size={16} />} onClick={() => load(true)} disabled={refreshing}>刷新</Button><Button variant="primary" icon={<CirclePlus size={16} />} onClick={openCreate}>添加账号</Button></div>
      </div>

      <div className="summary-strip">
        <div><span className="summary-icon green"><ShieldCheck size={18} /></span><p><small>可用账号</small><strong>{stats.healthy} <em>/ {items.length}</em></strong></p></div>
        <div><span className="summary-icon blue"><Activity size={18} /></span><p><small>累计请求</small><strong>{formatNumber(stats.requests)}</strong></p></div>
        <div><span className="summary-icon violet"><CloudCog size={18} /></span><p><small>累计估算费用</small><strong>{formatUSD(stats.cost)}</strong></p></div>
        <div className="summary-note"><Wifi size={17} /><span><strong>自动故障转移已启用</strong><small>异常账号会进入冷却，流量自动切换</small></span></div>
      </div>

      <section className="card table-card">
        {loading ? <LoadingBlock rows={7} /> : items.length === 0 ? (
          <EmptyState icon={<Server size={24} />} title="还没有上游账号" description="添加至少一个 Anthropic API 账号后，网关才能转发请求。" action={<Button variant="primary" icon={<CirclePlus size={16} />} onClick={openCreate}>添加第一个账号</Button>} />
        ) : (
          <div className="table-scroll"><table className="data-table accounts-table">
            <thead><tr><th>账号</th><th>状态</th><th>模型范围</th><th>调用 / Token</th><th>费用 / 限额</th><th>最近使用</th><th className="align-right">操作</th></tr></thead>
            <tbody>{items.map((account) => (
              <tr key={account.id} className={!account.enabled ? 'muted-row' : ''}>
                <td><div className="account-cell"><span className={`account-avatar ${account.status}`}><Server size={17} /></span><div><strong>{account.name}</strong><small><span className="mono">{account.key_hint}</span> · {new URL(account.base_url).host}</small></div></div></td>
                <td><Badge tone={statusTone(account.enabled ? account.status : 'disabled')} dot>{account.enabled ? statusLabel[account.status] || account.status : '已停用'}</Badge>{account.cooldown_until && <span className="cell-note">至 {formatDate(account.cooldown_until)}</span>}</td>
                <td>{account.allowed_models.length ? <div className="tag-stack">{account.allowed_models.slice(0, 2).map((model) => <span key={model}>{model}</span>)}{account.allowed_models.length > 2 && <span>+{account.allowed_models.length - 2}</span>}</div> : <span className="subtle">全部模型</span>}</td>
                <td><div className="cell-main"><strong>{formatNumber(account.request_count)} 次</strong><small>{formatNumber(account.input_tokens + account.output_tokens)} tokens</small></div></td>
                <td><div className="cell-main"><strong>{formatUSD(account.spent_usd)}</strong><small>{account.monthly_limit_usd ? `月限额 ${formatUSD(account.monthly_limit_usd)}` : '不限额'}</small></div></td>
                <td><span>{formatDate(account.last_used_at)}</span>{account.last_error && <span className="cell-error" title={account.last_error}>最近有错误</span>}</td>
                <td><div className="row-actions">
                  <Button size="sm" loading={testing === account.id} icon={testing === account.id ? undefined : account.status === 'healthy' ? <Wifi size={14} /> : <WifiOff size={14} />} onClick={() => test(account)}>检测</Button>
                  <IconButton label="编辑账号" onClick={() => openEdit(account)}><Edit3 size={16} /></IconButton>
                  <IconButton label={account.enabled ? '停用账号' : '启用账号'} onClick={() => toggle(account)}><Power size={16} /></IconButton>
                  <IconButton label="删除账号" className="danger-hover" onClick={() => setDeleting(account)}><Trash2 size={16} /></IconButton>
                </div></td>
              </tr>
            ))}</tbody>
          </table></div>
        )}
      </section>

      <Modal open={editing !== null} onClose={() => setEditing(null)} title={editing === 'new' ? '添加上游账号' : '编辑上游账号'} description="凭证会在写入数据库前使用 AES-GCM 加密。" size="lg" footer={<><Button onClick={() => setEditing(null)}>取消</Button><Button variant="primary" loading={saving} onClick={() => document.getElementById('account-form-submit')?.click()}>{editing === 'new' ? '添加账号' : '保存更改'}</Button></>}>
        <form className="form-grid" onSubmit={save}>
          <Field label="账号名称"><Input value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="例如：生产账号 A" maxLength={80} required /></Field>
          <Field label="上游 Base URL" hint="需要填写服务根地址，不要包含 /v1/messages"><Input value={form.base_url} onChange={(event) => setForm({ ...form, base_url: event.target.value })} placeholder="https://api.anthropic.com" type="url" required /></Field>
          <Field label={editing === 'new' ? 'Anthropic API Key' : '替换 API Key（可选）'} hint={editing === 'new' ? '只用于服务端请求，保存后不再明文展示' : '留空则保持现有密钥不变'} className="span-2"><Input value={form.api_key} onChange={(event) => setForm({ ...form, api_key: event.target.value })} placeholder={editing === 'new' ? 'sk-ant-api03-…' : '留空保持不变'} type="password" required={editing === 'new'} autoComplete="off" /></Field>
          <Field label="月度费用上限 (USD)" hint="0 表示不限额"><Input value={form.monthly_limit_usd} onChange={(event) => setForm({ ...form, monthly_limit_usd: Number(event.target.value) || 0 })} type="number" min="0" step="0.01" /></Field>
          <Field label="账号权重" hint="加权路由时使用，最小为 1"><Input value={form.weight} onChange={(event) => setForm({ ...form, weight: Number(event.target.value) || 1 })} type="number" min="1" max="1000" /></Field>
          <Field label="优先级" hint="数值越高越优先"><Input value={form.priority} onChange={(event) => setForm({ ...form, priority: Number(event.target.value) || 0 })} type="number" min="-100" max="100" /></Field>
          <Field label="启用状态"><div className="toggle-field"><Toggle checked={form.enabled} onChange={(enabled) => setForm({ ...form, enabled })} /><span>{form.enabled ? '加入路由池' : '暂不接收流量'}</span></div></Field>
          <Field label="允许模型" hint="每行一个模型，支持 claude-sonnet-*；留空允许全部" className="span-2"><textarea className="textarea" rows={4} value={modelsText} onChange={(event) => setModelsText(event.target.value)} placeholder={'claude-sonnet-*\nclaude-haiku-*'} /></Field>
          <Field label="备注" className="span-2"><textarea className="textarea" rows={3} value={form.notes} onChange={(event) => setForm({ ...form, notes: event.target.value })} placeholder="账号用途、负责人或到期信息…" /></Field>
          <button id="account-form-submit" type="submit" hidden />
        </form>
      </Modal>

      <ConfirmDialog open={!!deleting} onClose={() => setDeleting(null)} onConfirm={remove} loading={deletingBusy} danger title="删除上游账号？" confirmLabel="删除账号" description={`将删除「${deleting?.name || ''}」及其加密凭证，历史调用记录会保留但不再关联此账号。此操作不可撤销。`} />
    </div>
  )
}
