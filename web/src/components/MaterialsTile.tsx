import type { KeyboardEvent, MouseEvent, ReactNode } from 'react'

export type MaterialsTileSelectOptions = {
  multi: boolean
}

type MaterialsTileProps = {
  iconClassName: string
  title: ReactNode
  subtitle?: ReactNode
  description?: ReactNode
  path?: ReactNode
  pills?: ReactNode
  actions?: ReactNode
  footerStart?: ReactNode
  footerEnd?: ReactNode
  selected?: boolean
  emphasized?: boolean
  className?: string
  menu?: ReactNode
  titleActionAriaLabel?: string
  onSelect?: (options: MaterialsTileSelectOptions) => void
  onOpen?: () => void
  children?: ReactNode
}

function stopTileEvent(event: MouseEvent<HTMLElement>) {
  event.stopPropagation()
}

export default function MaterialsTile({
  iconClassName,
  title,
  subtitle,
  description,
  path,
  pills,
  actions,
  footerStart,
  footerEnd,
  selected = false,
  emphasized = false,
  className = '',
  menu = '⋮',
  titleActionAriaLabel,
  onSelect,
  onOpen,
  children,
}: MaterialsTileProps) {
  const interactive = Boolean(onSelect || onOpen)
  const titleAction = onOpen
    ? onOpen
    : onSelect
      ? () => onSelect({ multi: false })
      : undefined

  const tileClassName = [
    'materials-tile',
    interactive ? 'is-interactive' : '',
    selected ? 'is-selected' : '',
    emphasized ? 'is-emphasis' : '',
    className,
  ].filter(Boolean).join(' ')

  const handleSelect = (multi: boolean) => {
    onSelect?.({ multi })
  }

  const handleClick = (event: MouseEvent<HTMLDivElement>) => {
    if (!onSelect) return
    handleSelect(Boolean(event.metaKey || event.ctrlKey || event.shiftKey))
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.target !== event.currentTarget) return
    if (event.key === 'Enter') {
      event.preventDefault()
      if (onOpen) {
        onOpen()
        return
      }
      handleSelect(false)
      return
    }
    if (event.key === ' ') {
      event.preventDefault()
      if (onSelect) {
        handleSelect(false)
        return
      }
      onOpen?.()
    }
  }

  return (
    <div
      className={tileClassName}
      role={interactive ? 'button' : undefined}
      tabIndex={interactive ? 0 : undefined}
      onClick={interactive ? handleClick : undefined}
      onDoubleClick={onOpen}
      onKeyDown={interactive ? handleKeyDown : undefined}
    >
      <div className="materials-tile-top">
        <span className={`materials-tile-icon ${iconClassName}`} />
        <span className="materials-tile-menu" aria-hidden="true">{menu}</span>
      </div>
      <div className="materials-tile-body">
        {titleAction ? (
          <button
            type="button"
            className="materials-tile-title materials-tile-title-button"
            aria-label={titleActionAriaLabel}
            onClick={(event) => {
              event.stopPropagation()
              titleAction()
            }}
            onDoubleClick={stopTileEvent}
          >
            {title}
          </button>
        ) : (
          <div className="materials-tile-title">{title}</div>
        )}
        {subtitle ? <div className="materials-tile-subtitle">{subtitle}</div> : null}
        {(description || path || children) ? (
          <div className="materials-tile-content">
            {description ? <div className="materials-tile-desc">{description}</div> : null}
            {path ? <div className="materials-tile-path">{path}</div> : null}
            {children}
          </div>
        ) : null}
        {pills ? <div className="materials-tile-pills">{pills}</div> : null}
        {actions ? (
          <div className="materials-tile-actions" onClick={stopTileEvent} onDoubleClick={stopTileEvent}>
            {actions}
          </div>
        ) : null}
      </div>
      {(footerStart || footerEnd) ? (
        <div className="materials-tile-footer">
          <span>{footerStart}</span>
          <span>{footerEnd}</span>
        </div>
      ) : null}
    </div>
  )
}
