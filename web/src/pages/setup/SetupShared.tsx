import type { ReactNode } from 'react'

interface SetupSectionProps {
  icon: ReactNode
  title: string
  description: string
  badge?: string
  highlight?: boolean
  children: ReactNode
}

interface SetupCodeBlockProps {
  label: string
  content: string
  copied?: string | null
  copyKey?: string
  onCopy?: (text: string, key: string) => void
  copyLabel?: string
  copiedLabel?: string
  action?: ReactNode
}

interface SetupScreenshotPlaceholderProps {
  title: string
  caption: string
}

export function SetupSection({
  icon,
  title,
  description,
  badge,
  highlight = false,
  children,
}: SetupSectionProps) {
  return (
    <div className={`setup-section ${highlight ? 'setup-section-highlight' : ''}`}>
      <div className="setup-section-header">
        <span className="setup-section-icon">{icon}</span>
        <div>
          <h3>
            {title}
            {badge && <span className="setup-section-badge">{badge}</span>}
          </h3>
          <p className="setup-section-desc">{description}</p>
        </div>
      </div>
      <div className="setup-section-body">{children}</div>
    </div>
  )
}

export function SetupCodeBlock({
  label,
  content,
  copied,
  copyKey,
  onCopy,
  copyLabel = '复制',
  copiedLabel = '已复制',
  action,
}: SetupCodeBlockProps) {
  return (
    <div className="code-block">
      <div className="code-block-label">{label}</div>
      <pre>{content}</pre>
      {action ?? (
        copyKey && onCopy ? (
          <button
            className="copy-btn"
            onClick={() => onCopy(content, copyKey)}
          >
            {copied === copyKey ? copiedLabel : copyLabel}
          </button>
        ) : null
      )}
    </div>
  )
}

export function SetupScreenshotPlaceholder({ title, caption }: SetupScreenshotPlaceholderProps) {
  return (
    <div className="setup-screenshot-placeholder" aria-label={`截图占位：${title}`}>
      <div className="setup-screenshot-label">截图占位</div>
      <div className="setup-screenshot-title">{title}</div>
      <p className="setup-screenshot-caption">{caption}</p>
    </div>
  )
}
