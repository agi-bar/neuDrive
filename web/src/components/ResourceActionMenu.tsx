import type { ReactNode } from 'react'

export type ResourceActionMenuItem = {
  key: string
  label: ReactNode
  tone?: 'default' | 'danger'
  disabled?: boolean
  onSelect: () => void
}

type ResourceActionMenuProps = {
  items: ResourceActionMenuItem[]
}

export default function ResourceActionMenu({ items }: ResourceActionMenuProps) {
  return (
    <div className="resource-card-menu-list">
      {items.map((item) => (
        <button
          key={item.key}
          type="button"
          className={`resource-card-menu-item${item.tone === 'danger' ? ' is-danger' : ''}`}
          role="menuitem"
          disabled={item.disabled}
          onClick={item.onSelect}
        >
          {item.label}
        </button>
      ))}
    </div>
  )
}
