import { useEffect, type ReactNode } from 'react'

type ResourceConfirmDialogProps = {
  open: boolean
  title: ReactNode
  description: ReactNode
  confirmLabel: ReactNode
  cancelLabel: ReactNode
  kicker?: ReactNode
  tone?: 'danger' | 'primary'
  submitting?: boolean
  onConfirm: () => void
  onCancel: () => void
}

export default function ResourceConfirmDialog({
  open,
  title,
  description,
  confirmLabel,
  cancelLabel,
  kicker,
  tone = 'danger',
  submitting = false,
  onConfirm,
  onCancel,
}: ResourceConfirmDialogProps) {
  useEffect(() => {
    if (!open || submitting) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      onCancel()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onCancel, open, submitting])

  if (!open) return null

  return (
    <div className="materials-modal-backdrop" onClick={submitting ? undefined : onCancel}>
      <div
        className="materials-modal resource-confirm"
        role="dialog"
        aria-modal="true"
        aria-labelledby="resource-confirm-title"
        onClick={(event) => event.stopPropagation()}
      >
        {kicker ? <div className="resource-confirm-kicker">{kicker}</div> : null}
        <h3 id="resource-confirm-title" className="resource-confirm-title">{title}</h3>
        <p className="resource-confirm-copy">{description}</p>
        <div className="resource-confirm-actions">
          <button className="btn" disabled={submitting} onClick={onCancel}>
            {cancelLabel}
          </button>
          <button
            className={`btn ${tone === 'danger' ? 'btn-danger' : 'btn-primary'}`}
            disabled={submitting}
            onClick={onConfirm}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
