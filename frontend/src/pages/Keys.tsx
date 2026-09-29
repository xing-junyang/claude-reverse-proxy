import {
  Braces,
  CirclePlus,
  Copy,
  Edit3,
  KeyRound,
  LockKeyhole,
  Power,
  ShieldCheck,
  Trash2,
} from 'lucide-react'
import { type FormEvent, useEffect, useMemo, useState } from 'react'
import { api } from '../api'
import {
  Badge,
  Button,
  ConfirmDialog,
  CopyButton,
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
  useNotify,
} from '../components/UI'
import type { APIKey, APIKeyCreated, APIKeyInput } from '../types'

const emptyKey: APIKeyInput = { name: '', enabled: true, monthly_limit_usd: 0, allowed_models: [], expires_at: null }

function toLocalDateTime(value?: string) {
  if (!value) return ''
  const date = new Date(value)
  const offset = date.getTimezoneOffset() * 60000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}

export default function Keys() {
  const [items, setItems] = useState<APIKey[]>([])
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState<APIKey | 'new' | null>(null)
  const [form, setForm] = useState<APIKeyInput>(emptyKey)
  const [modelsText, setModelsText] = useState('')
  const [expiresText, setExpiresText] = useState('')
  const [saving, setSaving] = useState(false)
  const [created, setCreated] = useState<APIKeyCreated | null>(null)
  const [deleting, setDeleting] = useState<APIKey | null>(null)
  const [deleteBusy, setDeleteBusy] = useState(false)
  const notify = useNotify()

  const load = async () => {
    setLoading(true)
    try {
      const response = await api<{ items: APIKey[] }>('/api/keys')
      setItems(response.items)
    } catch (error) {
      notify({ type: 'error', title: 'API Key 加载失败', description: error instanceof Error ? error.message : undefined })
    } finally {
      setLoading(false)
    }
  }
  useEffect(() => { void load() }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const summary = useMemo(() => ({
    active: items.filter((key) => key.enabled && (!key.expires_at || new Date(key.expires_at) > new Date())).length,
    requests: items.reduce((sum, key) => sum + key.request_count, 0),
    spent: items.reduce((sum, key) => sum + key.spent_usd, 0),
  }), [items])

  const openCreate = () => {
    setForm({ ...emptyKey })
    setModelsText('')
    setExpiresText('')
    setEditing('new')
  }
  const openEdit = (key: APIKey) => {
    setForm({ name: key.name, enabled: key.enabled, monthly_limit_usd: key.monthly_limit_usd, allowed_models: key.allowed_models, expires_at: key.expires_at || null })
    setModelsText(key.allowed_models.join('\n'))
    setExpiresText(toLocalDateTime(key.expires_at))
    setEditing(key)
  }

  const save = async (event: FormEvent) => {
    event.preventDefault()
    setSaving(true)
    const payload: APIKeyInput = {
      ...form,
      allowed_models: modelsText.split(/[\n,]/).map((value) => value.trim()).filter(Boolean),
      expires_at: expiresText ? new Date(expiresText).toISOString() : null,
    }
    try {
      if (editing === 'new') {
        const result = await api<APIKeyCreated>('/api/keys', { method: 'POST', body: payload })
        setCreated(result)
        notify({ type: 'success', title: 'API Key 已创建' })
      } else if (editing) {
        await api<APIKey>(`/api/keys/${editing.id}`, { method: 'PUT', body: payload })
        notify({ type: 'success', title: 'API Key 配置已保存' })
      }
      setEditing(null)
      await load()
    } catch (error) {
      notify({ type: 'error', title: '保存失败', description: error instanceof Error ? error.message : undefined })
    } finally {
      setSaving(false)
    }
  }

  const toggle = async (key: APIKey) => {
    try {
      const updated = await api<APIKey>(`/api/keys/${key.id}`, { method: 'PUT', body: {
        name: key.name, enabled: !key.enabled, monthly_limit_usd: key.monthly_limit_usd,
        allowed_models: key.allowed_models, expires_at: key.expires_at || null,
      } })
      setItems((current) => current.map((item) => item.id === key.id ? updated : item))
      notify({ type: 'success', title: updated.enabled ? 'API Key 已启用' : 'API Key 已停用' })
    } catch (error) {
      notify({ type: 'error', title: '状态更新失败', description: error instanceof Error ? error.message : undefined })
    }
  }

  const remove = async () => {
    if (!deleting) return
    setDeleteBusy(true)
    try {
      await api(`/api/keys/${deleting.id}`, { method: 'DELETE' })
      setDeleting(null)
      notify({ type: 'success', title: 'API Key 已撤销' })
      await load()
    } catch (error) {
      notify({ type: 'error', title: '撤销失败', description: error instanceof Error ? error.message : undefined })
    } finally {
      setDeleteBusy(false)
    }
  }

  return (
    <div className="page-stack">
      <div className="page-intro compact-intro">
        <div><span className="eyebrow">ACCESS CONTROL</span><h2>API Key 管理</h2><p>为不同应用签发独立凭证，并限制模型范围与月度预算。</p></div>
        <Button variant="primary" icon={<CirclePlus size={16} />} onClick={openCreate}>创建 API Key</Button>
      </div>

      <div className="key-summary-grid">
        <article className="card mini-stat"><span className="metric-icon green"><ShieldCheck size={18} /></span><div><small>有效 Key</small><strong>{summary.active}</strong></div></article>
        <article className="card mini-stat"><span className="metric-icon blue"><Braces size={18} /></span><div><small>累计调用</small><strong>{formatNumber(summary.requests)}</strong></div></article>
        <article className="card mini-stat"><span className="metric-icon violet"><KeyRound size={18} /></span><div><small>累计估算费用</small><strong>{formatUSD(summary.spent)}</strong></div></article>
        <div className="security-callout"><LockKeyhole size={20} /><div><strong>密钥只显示一次</strong><span>服务端仅保存 SHA-256 哈希，无法找回明文。</span></div></div>
      </div>

      <section className="card table-card">
        {loading ? <LoadingBlock rows={6} /> : items.length === 0 ? (
          <EmptyState icon={<KeyRound size={24} />} title="还没有客户端 API Key" description="创建一个 Key，然后把它配置到 Claude SDK 或 Claude Code。" action={<Button variant="primary" icon={<CirclePlus size={16} />} onClick={openCreate}>创建第一个 Key</Button>} />
        ) : <div className="table-scroll"><table className="data-table keys-table">
          <thead><tr><th>名称 / Key</th><th>状态</th><th>模型权限</th><th>请求 / Token</th><th>费用 / 限额</th><th>最近使用</th><th className="align-right">操作</th></tr></thead>
          <tbody>{items.map((key) => {
            const expired = !!key.expires_at && new Date(key.expires_at) < new Date()
            return <tr key={key.id} className={!key.enabled || expired ? 'muted-row' : ''}>
              <td><div className="key-cell"><span className="key-avatar"><KeyRound size={17} /></span><div><strong>{key.name}</strong><code>{key.prefix}••••{key.last_four}</code></div></div></td>
              <td><Badge tone={expired || !key.enabled ? 'danger' : 'success'} dot>{expired ? '已过期' : key.enabled ? '有效' : '已停用'}</Badge>{key.expires_at && <span className="cell-note">到期 {formatDate(key.expires_at)}</span>}</td>
              <td>{key.allowed_models.length ? <div className="tag-stack">{key.allowed_models.slice(0, 2).map((model) => <span key={model}>{model}</span>)}{key.allowed_models.length > 2 && <span>+{key.allowed_models.length - 2}</span>}</div> : <span className="subtle">全部模型</span>}</td>
              <td><div className="cell-main"><strong>{formatNumber(key.request_count)} 次</strong><small>{formatNumber(key.input_tokens + key.output_tokens)} tokens</small></div></td>
              <td><div className="cell-main"><strong>{formatUSD(key.spent_usd)}</strong><small>{key.monthly_limit_usd ? `月限额 ${formatUSD(key.monthly_limit_usd)}` : '不限额'}</small></div></td>
              <td>{formatDate(key.last_used_at)}</td>
              <td><div className="row-actions"><IconButton label="编辑 Key" onClick={() => openEdit(key)}><Edit3 size={16} /></IconButton><IconButton label={key.enabled ? '停用 Key' : '启用 Key'} onClick={() => toggle(key)}><Power size={16} /></IconButton><IconButton label="撤销 Key" className="danger-hover" onClick={() => setDeleting(key)}><Trash2 size={16} /></IconButton></div></td>
            </tr>
          })}</tbody>
        </table></div>}
      </section>

      <Modal open={editing !== null} onClose={() => setEditing(null)} title={editing === 'new' ? '创建 API Key' : '编辑 API Key'} description="模型规则支持完整模型名或以 * 结尾的前缀。" size="md" footer={<><Button onClick={() => setEditing(null)}>取消</Button><Button variant="primary" loading={saving} onClick={() => document.getElementById('key-form-submit')?.click()}>{editing === 'new' ? '创建 Key' : '保存更改'}</Button></>}>
        <form className="form-grid" onSubmit={save}>
          <Field label="名称" className="span-2"><Input value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="例如：研发团队 / Cursor" maxLength={80} required /></Field>
          <Field label="月度费用上限 (USD)" hint="0 表示不限制"><Input type="number" min="0" step="0.01" value={form.monthly_limit_usd} onChange={(event) => setForm({ ...form, monthly_limit_usd: Number(event.target.value) || 0 })} /></Field>
          <Field label="过期时间" hint="留空表示永不过期"><Input type="datetime-local" value={expiresText} onChange={(event) => setExpiresText(event.target.value)} /></Field>
          <Field label="允许模型" hint="每行一个；留空允许全部" className="span-2"><textarea className="textarea" rows={5} value={modelsText} onChange={(event) => setModelsText(event.target.value)} placeholder={'claude-sonnet-*\nclaude-haiku-*'} /></Field>
          <Field label="启用状态" className="span-2"><div className="toggle-field"><Toggle checked={form.enabled} onChange={(enabled) => setForm({ ...form, enabled })} /><span>{form.enabled ? '签发后立即可用' : '暂不启用'}</span></div></Field>
          <button id="key-form-submit" type="submit" hidden />
        </form>
      </Modal>

      <Modal open={!!created} onClose={() => setCreated(null)} title="保存你的 API Key" description="这是唯一一次显示完整密钥。关闭后无法再次查看。" size="md" footer={<Button variant="primary" onClick={() => setCreated(null)}>我已妥善保存</Button>}>
        <div className="secret-reveal">
          <div className="secret-warning"><LockKeyhole size={18} /><span><strong>请立即复制</strong><small>不要通过聊天或邮件发送此密钥。</small></span></div>
          <div className="secret-value"><code>{created?.secret}</code><CopyButton value={created?.secret || ''} /></div>
          <div className="code-sample"><header><span>环境变量示例</span><CopyButton value={`export ANTHROPIC_BASE_URL=${window.location.origin}\nexport ANTHROPIC_API_KEY=${created?.secret || ''}`} label="复制配置" /></header><pre>{`export ANTHROPIC_BASE_URL=${window.location.origin}\nexport ANTHROPIC_API_KEY=${created?.secret || ''}`}</pre></div>
        </div>
      </Modal>

      <ConfirmDialog open={!!deleting} onClose={() => setDeleting(null)} onConfirm={remove} loading={deleteBusy} danger title="撤销 API Key？" confirmLabel="立即撤销" description={`撤销「${deleting?.name || ''}」后，使用该 Key 的所有客户端会立即无法访问。历史用量记录仍会保留。`} />
    </div>
  )
}
