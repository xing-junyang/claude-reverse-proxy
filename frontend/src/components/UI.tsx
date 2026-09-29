import {
  AlertTriangle,
  Check,
  CheckCircle2,
  Clipboard,
  Info,
  LoaderCircle,
  X,
  XCircle,
} from 'lucide-react'
import {
  createContext,
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useId,
  useState,
} from 'react'
import type { ToastMessage } from '../types'

type ToastContextValue = {
  notify: (message: Omit<ToastMessage, 'id'>) => void
}

const ToastContext = createContext<ToastContextValue>({ notify: () => undefined })

export function ToastProvider({ children }: { children: ReactNode }) {
  const [messages, setMessages] = useState<ToastMessage[]>([])

  const notify = useCallback((message: Omit<ToastMessage, 'id'>) => {
    const id = Date.now() + Math.floor(Math.random() * 1000)
    setMessages((current) => [...current, { ...message, id }])
    window.setTimeout(() => {
      setMessages((current) => current.filter((item) => item.id !== id))
    }, 4500)
  }, [])

  return (
    <ToastContext.Provider value={{ notify }}>
      {children}
      <div className="toast-viewport" aria-live="polite">
        {messages.map((message) => (
          <div className={`toast toast-${message.type}`} key={message.id}>
            <span className="toast-icon">
              {message.type === 'success' ? <CheckCircle2 size={18} /> : message.type === 'error' ? <XCircle size={18} /> : <Info size={18} />}
            </span>
            <div className="toast-copy">
              <strong>{message.title}</strong>
              {message.description && <span>{message.description}</span>}
            </div>
            <button aria-label="关闭提示" onClick={() => setMessages((current) => current.filter((item) => item.id !== message.id))}>
              <X size={16} />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}

export const useNotify = () => useContext(ToastContext).notify

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger'
  size?: 'sm' | 'md'
  loading?: boolean
  icon?: ReactNode
}

export function Button({ variant = 'secondary', size = 'md', loading, icon, children, className = '', disabled, ...props }: ButtonProps) {
  return (
    <button className={`button button-${variant} button-${size} ${className}`} disabled={disabled || loading} {...props}>
      {loading ? <LoaderCircle className="spin" size={16} /> : icon}
      {children}
    </button>
  )
}

export function IconButton({ label, children, className = '', ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { label: string; children: ReactNode }) {
  return (
    <button className={`icon-button ${className}`} aria-label={label} title={label} {...props}>
      {children}
    </button>
  )
}

type ModalProps = {
  open: boolean
  onClose: () => void
  title: string
  description?: string
  children: ReactNode
  footer?: ReactNode
  size?: 'sm' | 'md' | 'lg'
}

export function Modal({ open, onClose, title, description, children, footer, size = 'md' }: ModalProps) {
  const titleID = useId()
  useEffect(() => {
    if (!open) return
    const onKey = (event: KeyboardEvent) => event.key === 'Escape' && onClose()
    document.addEventListener('keydown', onKey)
    document.body.classList.add('modal-open')
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.classList.remove('modal-open')
    }
  }, [open, onClose])
  if (!open) return null
  return (
    <div className="modal-layer" role="presentation" onMouseDown={(event) => event.currentTarget === event.target && onClose()}>
      <section className={`modal modal-${size}`} role="dialog" aria-modal="true" aria-labelledby={titleID}>
        <header className="modal-header">
          <div>
            <h2 id={titleID}>{title}</h2>
            {description && <p>{description}</p>}
          </div>
          <IconButton label="关闭" onClick={onClose}><X size={19} /></IconButton>
        </header>
        <div className="modal-body">{children}</div>
        {footer && <footer className="modal-footer">{footer}</footer>}
      </section>
    </div>
  )
}

type FieldProps = {
  label: string
  hint?: string
  error?: string
  children: ReactNode
  className?: string
}

export function Field({ label, hint, error, children, className = '' }: FieldProps) {
  return (
    <label className={`field ${className}`}>
      <span className="field-label">{label}</span>
      {children}
      {(error || hint) && <span className={error ? 'field-error' : 'field-hint'}>{error || hint}</span>}
    </label>
  )
}

export function Input({ className = '', ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return <input className={`input ${className}`} {...props} />
}

export function Toggle({ checked, onChange, label }: { checked: boolean; onChange: (checked: boolean) => void; label?: string }) {
  return (
    <button type="button" role="switch" aria-checked={checked} className={`toggle ${checked ? 'is-on' : ''}`} onClick={() => onChange(!checked)}>
      <span className="toggle-thumb" />
      {label && <span className="sr-only">{label}</span>}
    </button>
  )
}

export function Badge({ tone = 'neutral', children, dot }: { tone?: 'success' | 'warning' | 'danger' | 'neutral' | 'accent' | 'info'; children: ReactNode; dot?: boolean }) {
  return <span className={`badge badge-${tone}`}>{dot && <i />}{children}</span>
}

export function EmptyState({ icon, title, description, action }: { icon?: ReactNode; title: string; description: string; action?: ReactNode }) {
  return (
    <div className="empty-state">
      {icon && <div className="empty-icon">{icon}</div>}
      <h3>{title}</h3>
      <p>{description}</p>
      {action && <div className="empty-action">{action}</div>}
    </div>
  )
}

export function LoadingBlock({ rows = 3 }: { rows?: number }) {
  return (
    <div className="loading-block">
      {Array.from({ length: rows }, (_, index) => <div key={index} className="skeleton" style={{ width: `${92 - index * 7}%` }} />)}
    </div>
  )
}

export function ConfirmDialog({ open, title, description, confirmLabel = '确认', danger = false, loading, onConfirm, onClose }: {
  open: boolean
  title: string
  description: string
  confirmLabel?: string
  danger?: boolean
  loading?: boolean
  onConfirm: () => void
  onClose: () => void
}) {
  return (
    <Modal open={open} onClose={onClose} title={title} size="sm" footer={
      <>
        <Button onClick={onClose}>取消</Button>
        <Button variant={danger ? 'danger' : 'primary'} loading={loading} onClick={onConfirm}>{confirmLabel}</Button>
      </>
    }>
      <div className="confirm-message">
        <AlertTriangle size={22} />
        <p>{description}</p>
      </div>
    </Modal>
  )
}

export function CopyButton({ value, label = '复制' }: { value: string; label?: string }) {
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    await navigator.clipboard.writeText(value)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1800)
  }
  return (
    <Button size="sm" variant="secondary" icon={copied ? <Check size={15} /> : <Clipboard size={15} />} onClick={copy}>
      {copied ? '已复制' : label}
    </Button>
  )
}

export const formatNumber = (value: number) => new Intl.NumberFormat('zh-CN', { notation: value >= 10000 ? 'compact' : 'standard', maximumFractionDigits: 1 }).format(value)
export const formatUSD = (value: number) => `$${value < 0.01 && value > 0 ? value.toFixed(5) : value.toFixed(2)}`
export const formatDate = (value?: string) => value ? new Intl.DateTimeFormat('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value)) : '从未'
export const formatFullDate = (value?: string) => value ? new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(value)) : '—'
export const formatDuration = (ms: number) => ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(ms < 10000 ? 2 : 1)} s`

export function statusTone(status: string): 'success' | 'warning' | 'danger' | 'neutral' | 'info' {
  if (status === 'healthy' || status === 'success') return 'success'
  if (status === 'cooldown' || status === 'degraded') return 'warning'
  if (status === 'disabled' || status === 'error') return 'danger'
  if (status === 'unknown') return 'neutral'
  return 'info'
}
