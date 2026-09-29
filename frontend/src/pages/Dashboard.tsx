import {
  Activity,
  ArrowDownRight,
  ArrowUpRight,
  CheckCircle2,
  Clock3,
  Coins,
  Copy,
  KeyRound,
  Radio,
  RefreshCw,
  Server,
  Sparkles,
  Zap,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { api } from '../api'
import UsageChart from '../components/UsageChart'
import { Badge, Button, EmptyState, LoadingBlock, formatDate, formatDuration, formatNumber, formatUSD, useNotify } from '../components/UI'
import type { DashboardData, MetricDelta, UsageLog } from '../types'

function Delta({ metric, invert = false }: { metric: MetricDelta; invert?: boolean }) {
  if (!metric.has_previous) return <span className="metric-context">当前周期</span>
  const positive = invert ? metric.change_pct <= 0 : metric.change_pct >= 0
  return (
    <span className={`metric-delta ${positive ? 'positive' : 'negative'}`}>
      {metric.change_pct >= 0 ? <ArrowUpRight size={14} /> : <ArrowDownRight size={14} />}
      {Math.abs(metric.change_pct).toFixed(1)}% <small>较上期</small>
    </span>
  )
}

function RequestRow({ item }: { item: UsageLog }) {
  const ok = item.status_code >= 200 && item.status_code < 400
  return (
    <tr>
      <td><span className={`request-status ${ok ? 'ok' : 'bad'}`}>{ok ? <CheckCircle2 size={14} /> : <Activity size={14} />}</span></td>
      <td><div className="cell-main"><strong>{item.model || '—'}</strong><small>{item.api_key_name || '已删除的 Key'}</small></div></td>
      <td><span className="mono subtle">{formatNumber(item.input_tokens + item.output_tokens)}</span></td>
      <td><span className="mono">{formatUSD(item.cost_usd)}</span></td>
      <td><span>{formatDuration(item.duration_ms)}</span></td>
      <td className="table-time">{formatDate(item.created_at)}</td>
    </tr>
  )
}

export default function Dashboard({ onNavigate }: { onNavigate: (tab: 'accounts' | 'keys' | 'usage') => void }) {
  const [hours, setHours] = useState(24)
  const [data, setData] = useState<DashboardData | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [chartMetric, setChartMetric] = useState<'requests' | 'tokens' | 'cost'>('requests')
  const notify = useNotify()

  const load = async (quiet = false) => {
    if (quiet) setRefreshing(true); else setLoading(true)
    try {
      setData(await api<DashboardData>(`/api/dashboard?hours=${hours}`))
    } catch (error) {
      notify({ type: 'error', title: '概览加载失败', description: error instanceof Error ? error.message : undefined })
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }

  useEffect(() => {
    void load()
    const timer = window.setInterval(() => void load(true), 30000)
    return () => window.clearInterval(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hours])

  const totalModelRequests = useMemo(() => data?.models.reduce((sum, item) => sum + item.requests, 0) || 0, [data])

  return (
    <div className="page-stack">
      <div className="page-intro">
        <div><span className="eyebrow">REAL-TIME OPERATIONS</span><h2>早上好，网关一切正常。</h2><p>掌握请求流量、Token 消耗与账号池健康状况。</p></div>
        <div className="intro-actions">
          <div className="segmented">
            {[[24, '24 小时'], [168, '7 天'], [720, '30 天']].map(([value, label]) => <button key={value} className={hours === value ? 'active' : ''} onClick={() => setHours(value as number)}>{label}</button>)}
          </div>
          <Button icon={<RefreshCw className={refreshing ? 'spin' : ''} size={16} />} onClick={() => load(true)} disabled={refreshing}>刷新</Button>
        </div>
      </div>

      {loading || !data ? <div className="metric-grid">{Array.from({ length: 4 }, (_, index) => <div className="card metric-card" key={index}><LoadingBlock rows={3} /></div>)}</div> : (
        <>
          <div className="metric-grid">
            <article className="card metric-card"><div className="metric-top"><span className="metric-icon accent"><Zap size={18} /></span><span>请求总量</span></div><strong>{formatNumber(data.requests.value)}</strong><Delta metric={data.requests} /></article>
            <article className="card metric-card"><div className="metric-top"><span className="metric-icon violet"><Sparkles size={18} /></span><span>Token 消耗</span></div><strong>{formatNumber(data.tokens.value)}</strong><Delta metric={data.tokens} /></article>
            <article className="card metric-card"><div className="metric-top"><span className="metric-icon blue"><Coins size={18} /></span><span>估算费用</span></div><strong>{formatUSD(data.cost.value)}</strong><Delta metric={data.cost} invert /></article>
            <article className="card metric-card"><div className="metric-top"><span className="metric-icon green"><CheckCircle2 size={18} /></span><span>成功率</span></div><strong>{data.success_rate.value.toFixed(1)}%</strong><Delta metric={data.success_rate} /></article>
          </div>

          <div className="dashboard-grid">
            <section className="card chart-card">
              <header className="card-header">
                <div><span className="card-kicker">TRAFFIC</span><h3>调用趋势</h3></div>
                <div className="mini-tabs">
                  {(['requests', 'tokens', 'cost'] as const).map((metric) => <button className={chartMetric === metric ? 'active' : ''} key={metric} onClick={() => setChartMetric(metric)}>{metric === 'requests' ? '请求' : metric === 'tokens' ? 'Token' : '费用'}</button>)}
                </div>
              </header>
              <UsageChart data={data.series} metric={chartMetric} />
              <div className="chart-footnotes"><span><Clock3 size={14} />平均延迟 {formatDuration(data.average_latency_ms)}</span><span><Radio size={14} />每 30 秒更新</span></div>
            </section>
            <section className="card health-card">
              <header className="card-header"><div><span className="card-kicker">CAPACITY</span><h3>网关容量</h3></div><Badge tone="success" dot>运行中</Badge></header>
              <div className="health-score"><div className="score-ring" style={{ '--score': `${data.total_accounts ? (data.active_accounts / data.total_accounts) * 100 : 0}%` } as React.CSSProperties}><span>{data.active_accounts}</span><small>/ {data.total_accounts}</small></div><div><strong>可用上游</strong><span>{data.total_accounts ? `${Math.round(data.active_accounts / data.total_accounts * 100)}% 容量在线` : '等待添加账号'}</span></div></div>
              <div className="health-list">
                <button onClick={() => onNavigate('accounts')}><span><Server size={16} />上游账号</span><strong>{data.active_accounts} / {data.total_accounts}</strong></button>
                <button onClick={() => onNavigate('keys')}><span><KeyRound size={16} />活跃 API Key</span><strong>{data.active_keys}</strong></button>
                <button onClick={() => onNavigate('usage')}><span><Activity size={16} />平均响应时间</span><strong>{formatDuration(data.average_latency_ms)}</strong></button>
              </div>
            </section>
          </div>

          <div className="dashboard-bottom-grid">
            <section className="card model-card">
              <header className="card-header"><div><span className="card-kicker">MODELS</span><h3>模型分布</h3></div></header>
              {data.models.length === 0 ? <EmptyState title="暂无调用" description="首次调用后，这里会展示模型流量分布。" /> : <div className="model-list">{data.models.map((model, index) => (
                <div className="model-row" key={model.model}>
                  <div className="model-rank">{String(index + 1).padStart(2, '0')}</div>
                  <div className="model-detail"><div><strong>{model.model}</strong><span>{formatNumber(model.input_tokens + model.output_tokens)} tokens</span></div><div className="progress"><i style={{ width: `${Math.max((model.requests / totalModelRequests) * 100, 2)}%` }} /></div></div>
                  <span>{Math.round(model.requests / totalModelRequests * 100)}%</span>
                </div>
              ))}</div>}
            </section>
            <section className="card recent-card">
              <header className="card-header"><div><span className="card-kicker">LATEST</span><h3>最近调用</h3></div><button className="text-button" onClick={() => onNavigate('usage')}>查看全部</button></header>
              {data.recent.length === 0 ? <EmptyState title="还没有调用记录" description="创建 API Key 并发起一次 /v1/messages 请求即可看到记录。" /> : <div className="table-scroll"><table className="data-table compact"><thead><tr><th></th><th>模型 / Key</th><th>Token</th><th>费用</th><th>耗时</th><th>时间</th></tr></thead><tbody>{data.recent.map((item) => <RequestRow item={item} key={item.id} />)}</tbody></table></div>}
            </section>
          </div>

          <section className="endpoint-strip">
            <div className="endpoint-icon"><Copy size={18} /></div>
            <div><span>ANTHROPIC BASE URL</span><code>{window.location.origin}</code></div>
            <Button size="sm" icon={<Copy size={14} />} onClick={() => { navigator.clipboard.writeText(window.location.origin); notify({ type: 'success', title: 'Base URL 已复制' }) }}>复制地址</Button>
          </section>
        </>
      )}
    </div>
  )
}
