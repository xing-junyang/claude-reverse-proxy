import { useMemo, useState } from 'react'
import type { SeriesPoint } from '../types'
import { formatNumber, formatUSD } from './UI'

type Metric = 'requests' | 'tokens' | 'cost'

function metricValue(point: SeriesPoint, metric: Metric) {
  if (metric === 'tokens') return point.input_tokens + point.output_tokens
  if (metric === 'cost') return point.cost_usd
  return point.requests
}

function compactLabel(value: string, longRange: boolean) {
  const date = new Date(value)
  return new Intl.DateTimeFormat('zh-CN', longRange ? { month: 'numeric', day: 'numeric' } : { hour: '2-digit', minute: '2-digit' }).format(date)
}

export default function UsageChart({ data, metric = 'requests' }: { data: SeriesPoint[]; metric?: Metric }) {
  const [active, setActive] = useState<number | null>(null)
  const width = 860
  const height = 250
  const pad = { top: 18, right: 12, bottom: 34, left: 10 }
  const chartWidth = width - pad.left - pad.right
  const chartHeight = height - pad.top - pad.bottom
  const values = data.map((point) => metricValue(point, metric))
  const maximum = Math.max(...values, 1)
  const longRange = data.length > 48
  const coordinates = useMemo(() => data.map((point, index) => ({
    x: pad.left + (data.length === 1 ? chartWidth / 2 : (index / Math.max(data.length - 1, 1)) * chartWidth),
    y: pad.top + chartHeight - (metricValue(point, metric) / maximum) * chartHeight,
  })), [chartWidth, chartHeight, data, maximum, metric, pad.left, pad.top])
  const line = coordinates.map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x.toFixed(2)} ${point.y.toFixed(2)}`).join(' ')
  const area = coordinates.length ? `${line} L ${coordinates.at(-1)!.x} ${pad.top + chartHeight} L ${coordinates[0].x} ${pad.top + chartHeight} Z` : ''
  const shownTicks = [0, Math.floor((data.length - 1) / 2), data.length - 1].filter((item, index, all) => item >= 0 && all.indexOf(item) === index)

  if (!data.length) return <div className="chart-empty">暂无用量数据</div>

  return (
    <div className="usage-chart-wrap">
      <svg
        className="usage-chart"
        viewBox={`0 0 ${width} ${height}`}
        role="img"
        aria-label="用量趋势图"
        onMouseLeave={() => setActive(null)}
        onMouseMove={(event) => {
          const bounds = event.currentTarget.getBoundingClientRect()
          const x = ((event.clientX - bounds.left) / bounds.width) * width
          const index = Math.round(((x - pad.left) / chartWidth) * Math.max(data.length - 1, 1))
          setActive(Math.min(Math.max(index, 0), data.length - 1))
        }}
      >
        <defs>
          <linearGradient id="chartFill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--accent)" stopOpacity="0.24" />
            <stop offset="100%" stopColor="var(--accent)" stopOpacity="0" />
          </linearGradient>
        </defs>
        {[0, 0.5, 1].map((position) => (
          <line key={position} className="chart-grid" x1={pad.left} x2={width - pad.right} y1={pad.top + position * chartHeight} y2={pad.top + position * chartHeight} />
        ))}
        <path d={area} fill="url(#chartFill)" />
        <path d={line} className="chart-line" fill="none" />
        {shownTicks.map((index) => (
          <text key={index} className="chart-label" x={coordinates[index].x} y={height - 7} textAnchor={index === 0 ? 'start' : index === data.length - 1 ? 'end' : 'middle'}>
            {compactLabel(data[index].bucket, longRange)}
          </text>
        ))}
        {active !== null && (
          <>
            <line className="chart-crosshair" x1={coordinates[active].x} x2={coordinates[active].x} y1={pad.top} y2={pad.top + chartHeight} />
            <circle className="chart-dot" cx={coordinates[active].x} cy={coordinates[active].y} r="5" />
          </>
        )}
      </svg>
      {active !== null && (
        <div className="chart-tooltip" style={{ left: `${(coordinates[active].x / width) * 100}%` }}>
          <span>{new Intl.DateTimeFormat('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(data[active].bucket))}</span>
          <strong>{metric === 'cost' ? formatUSD(values[active]) : formatNumber(values[active])}</strong>
          {data[active].errors > 0 && <em>{data[active].errors} 次错误</em>}
        </div>
      )}
    </div>
  )
}
