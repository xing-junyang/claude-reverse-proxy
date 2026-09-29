import {
  Activity,
  ArrowLeft,
  ArrowRight,
  CheckCircle2,
  ChevronRight,
  Clock3,
  Coins,
  Download,
  RefreshCw,
  Search,
  Sparkles,
  XCircle,
} from 'lucide-react'
import { type FormEvent, useEffect, useMemo, useState } from 'react'
import { api, queryString } from '../api'
import {
  Badge,
  Button,
  EmptyState,
  Field,
  IconButton,
  Input,
  LoadingBlock,
  Modal,
  formatDuration,
  formatFullDate,
  formatNumber,
  formatUSD,
  useNotify,
} from '../components/UI'
import type { APIKey, Account, UsageLog } from '../types'

type Summary = {
  requests: number
  input_tokens: number
  output_tokens: number
  cost_usd: number
  errors: number
  average_latency_ms: number
}

type Filters = {
  range: number
  keyID: string
  accountID: string
  model: string
  status: string
  search: string
}

const pageSize = 30
const initialFilters: Filters = { range: 24, keyID: '', accountID: '', model: '', status: '', search: '' }

export default function Usage() {
  const [items, setItems] = useState<UsageLog[]>([])
  const [total, setTotal] = useState(0)
  const [summary, setSummary] = useState<Summary | null>(null)
  const [accounts, setAccounts] = useState<Account[]>([])
  const [keys, setKeys] = useState<APIKey[]>([])
  const [models, setModels] = useState<string[]>([])
  const [filters, setFilters] = useState<Filters>(initialFilters)
  const [searchDraft, setSearchDraft] = useState('')
  const [page, setPage] = useState(0)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [selected, setSelected] = useState<UsageLog | null>(null)
  const notify = useNotify()

  const buildParams = (withPage: boolean) => {
    const from = new Date(Date.now() - filters.range * 60 * 60 * 1000).toISOString()
    return {
      key_id: filters.keyID, account_id: filters.accountID, model: filters.model,
      status: filters.status, search: filters.search, from,
      limit: withPage ? pageSize : 200, offset: withPage ? page * pageSize : 0,
    }
  }

  const load = async (quiet = false) => {
    quiet ? setRefreshing(true) : setLoading(true)
    try {
      const [logs, aggregate] = await Promise.all([
        api<{ items: UsageLog[]; total: number }>(`/api/usage${queryString(buildParams(true))}`),
        api<Summary>(`/api/usage/summary${queryString(buildParams(false))}`),
      ])
      setItems(logs.items)
      setTotal(logs.total)
      setSummary(aggregate)
    } catch (error) {
      notify({ type: 'error', title: '调用记录加载失败', description: error instanceof Error ? error.message : undefined })
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }

  useEffect(() => {
    Promise.all([
      api<{ items: Account[] }>('/api/accounts'),
      api<{ items: APIKey[] }>('/api/keys'),
      api<{ items: string[] }>('/api/usage/models'),
    ]).then(([accountData, keyData, modelData]) => {
      setAccounts(accountData.items)
      setKeys(keyData.items)
      setModels(modelData.items)
    }).catch(() => undefined)
  }, [])

  useEffect(() => { void load() }, [filters, page]) // eslint-disable-line react-hooks/exhaustive-deps

  const pageCount = Math.max(Math.ceil(total / pageSize), 1)
  const successRate = summary?.requests ? (summary.requests - summary.errors) / summary.requests * 100 : 0
  const hasFilters = !!(filters.keyID || filters.accountID || filters.model || filters.status || filters.search)

  const updateFilter = <K extends keyof Filters>(key: K, value: Filters[K]) => {
    setPage(0)
    setFilters((current) => ({ ...current, [key]: value }))
  }

  const submitSearch = (event: FormEvent) => {
    event.preventDefault()
    updateFilter('search', searchDraft.trim())
  }

  const exportCurrent = () => {
    const header = ['request_id', 'time', 'status', 'model', 'api_key', 'account', 'input_tokens', 'output_tokens', 'cost_usd', 'duration_ms', 'streamed', 'error']
    const escape = (value: unknown) => `"${String(value ?? '').replaceAll('"', '""')}"`
    const rows = items.map((item) => [item.request_id, item.created_at, item.status_code, item.model, item.api_key_name, item.account_name, item.input_tokens, item.output_tokens, item.cost_usd, item.duration_ms, item.streamed, item.error].map(escape).join(','))
    const blob = new Blob([[header.join(','), ...rows].join('\n')], { type: 'text/csv;charset=utf-8' })
    const link = document.createElement('a')
    link.href = URL.createObjectURL(blob)
    link.download = `claude-relay-usage-${new Date().toISOString().slice(0, 10)}.csv`
    link.click()
    URL.revokeObjectURL(link.href)
  }

  return (
    <div className="page-stack">
      <div className="page-intro compact-intro">
        <div><span className="eyebrow">OBSERVABILITY</span><h2>调用与用量</h2><p>逐次追踪请求路径、Token、费用、延迟与上游错误。</p></div>
        <div className="intro-actions"><Button icon={<Download size={16} />} onClick={exportCurrent} disabled={!items.length}>导出当前页</Button><Button icon={<RefreshCw className={refreshing ? 'spin' : ''} size={16} />} onClick={() => load(true)} disabled={refreshing}>刷新</Button></div>
      </div>

      <div className="usage-summary-grid">
        <article className="card mini-stat"><span className="metric-icon accent"><Activity size={18} /></span><div><small>请求总数</small><strong>{formatNumber(summary?.requests || 0)}</strong></div></article>
        <article className="card mini-stat"><span className="metric-icon violet"><Sparkles size={18} /></span><div><small>总 Token</small><strong>{formatNumber((summary?.input_tokens || 0) + (summary?.output_tokens || 0))}</strong></div></article>
        <article className="card mini-stat"><span className="metric-icon blue"><Coins size={18} /></span><div><small>估算费用</small><strong>{formatUSD(summary?.cost_usd || 0)}</strong></div></article>
        <article className="card mini-stat"><span className="metric-icon green"><CheckCircle2 size={18} /></span><div><small>成功率</small><strong>{successRate.toFixed(1)}%</strong></div></article>
        <article className="card mini-stat"><span className="metric-icon amber"><Clock3 size={18} /></span><div><small>平均延迟</small><strong>{formatDuration(summary?.average_latency_ms || 0)}</strong></div></article>
      </div>

      <section className="card filter-card">
        <div className="filter-row">
          <form className="search-box" onSubmit={submitSearch}><Search size={16} /><input value={searchDraft} onChange={(event) => setSearchDraft(event.target.value)} placeholder="搜索请求 ID、模型、IP 或错误…" /><button type="submit">搜索</button></form>
          <select className="select" value={filters.range} onChange={(event) => updateFilter('range', Number(event.target.value))}><option value={24}>最近 24 小时</option><option value={168}>最近 7 天</option><option value={720}>最近 30 天</option></select>
          <select className="select" value={filters.keyID} onChange={(event) => updateFilter('keyID', event.target.value)}><option value="">全部 API Key</option>{keys.map((key) => <option key={key.id} value={key.id}>{key.name}</option>)}</select>
          <select className="select" value={filters.accountID} onChange={(event) => updateFilter('accountID', event.target.value)}><option value="">全部账号</option>{accounts.map((account) => <option key={account.id} value={account.id}>{account.name}</option>)}</select>
          <select className="select" value={filters.model} onChange={(event) => updateFilter('model', event.target.value)}><option value="">全部模型</option>{models.map((model) => <option key={model}>{model}</option>)}</select>
          <select className="select" value={filters.status} onChange={(event) => updateFilter('status', event.target.value)}><option value="">全部状态</option><option value="success">成功</option><option value="error">失败</option></select>
          {hasFilters && <button className="clear-filter" onClick={() => { setSearchDraft(''); setPage(0); setFilters({ ...initialFilters, range: filters.range }) }}>清除筛选</button>}
        </div>
      </section>

      <section className="card table-card usage-table-card">
        {loading ? <LoadingBlock rows={9} /> : items.length === 0 ? (
          <EmptyState icon={<Activity size={24} />} title="没有匹配的调用记录" description={hasFilters ? '试试放宽筛选条件，或查看更长的时间范围。' : '客户端首次调用 /v1/messages 后，记录会出现在这里。'} />
        ) : <>
          <div className="table-scroll"><table className="data-table usage-table">
            <thead><tr><th>状态</th><th>时间</th><th>模型 / 端点</th><th>API Key → 账号</th><th>Token</th><th>费用</th><th>延迟</th><th></th></tr></thead>
            <tbody>{items.map((item) => {
              const ok = item.status_code >= 200 && item.status_code < 400
              return <tr key={item.id} className="clickable-row" onClick={() => setSelected(item)}>
                <td><Badge tone={ok ? 'success' : 'danger'}>{ok ? <CheckCircle2 size={13} /> : <XCircle size={13} />}{item.status_code}</Badge></td>
                <td><div className="cell-main"><strong>{new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(item.created_at))}</strong><small>{new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit' }).format(new Date(item.created_at))}</small></div></td>
                <td><div className="cell-main"><strong>{item.model || '—'}</strong><small>{item.endpoint}{item.streamed ? ' · 流式' : ''}</small></div></td>
                <td><div className="route-cell"><span>{item.api_key_name || '已删除 Key'}</span><ChevronRight size={13} /><span>{item.account_name || '已删除账号'}</span></div></td>
                <td><div className="token-cell"><span>IN {formatNumber(item.input_tokens)}</span><span>OUT {formatNumber(item.output_tokens)}</span></div></td>
                <td><span className="mono">{formatUSD(item.cost_usd)}</span></td>
                <td><span>{formatDuration(item.duration_ms)}</span><small className="cell-note">TTFB {formatDuration(item.time_to_first_byte_ms)}</small></td>
                <td><ChevronRight size={16} className="subtle" /></td>
              </tr>
            })}</tbody>
          </table></div>
          <footer className="pagination"><span>共 {formatNumber(total)} 条 · 第 {page + 1} / {pageCount} 页</span><div><Button size="sm" icon={<ArrowLeft size={14} />} disabled={page === 0} onClick={() => setPage(page - 1)}>上一页</Button><Button size="sm" disabled={page + 1 >= pageCount} onClick={() => setPage(page + 1)}>下一页 <ArrowRight size={14} /></Button></div></footer>
        </>}
      </section>

      <Modal open={!!selected} onClose={() => setSelected(null)} title="调用详情" description={selected?.request_id} size="lg">
        {selected && <div className="request-detail">
          <div className="detail-status"><Badge tone={selected.status_code < 400 ? 'success' : 'danger'} dot>HTTP {selected.status_code}</Badge><span>{selected.streamed ? 'Streaming SSE' : 'JSON response'}</span><span>{formatFullDate(selected.created_at)}</span></div>
          <div className="detail-grid">
            <div><span>模型</span><strong>{selected.model || '—'}</strong></div><div><span>端点</span><strong>/v1/{selected.endpoint === 'count_tokens' ? 'messages/count_tokens' : selected.endpoint}</strong></div>
            <div><span>客户端 Key</span><strong>{selected.api_key_name || '已删除'}</strong></div><div><span>上游账号</span><strong>{selected.account_name || '已删除'}</strong></div>
            <div><span>输入 Token</span><strong>{formatNumber(selected.input_tokens)}</strong></div><div><span>输出 Token</span><strong>{formatNumber(selected.output_tokens)}</strong></div>
            <div><span>缓存读取</span><strong>{formatNumber(selected.cache_read_tokens)}</strong></div><div><span>缓存写入</span><strong>{formatNumber(selected.cache_creation_tokens)}</strong></div>
            <div><span>总耗时</span><strong>{formatDuration(selected.duration_ms)}</strong></div><div><span>首字节</span><strong>{formatDuration(selected.time_to_first_byte_ms)}</strong></div>
            <div><span>估算费用</span><strong>{formatUSD(selected.cost_usd)}</strong></div><div><span>客户端 IP</span><strong className="mono">{selected.client_ip || '—'}</strong></div>
          </div>
          {selected.error && <div className="error-detail"><strong>错误信息</strong><pre>{selected.error}</pre></div>}
        </div>}
      </Modal>
    </div>
  )
}
