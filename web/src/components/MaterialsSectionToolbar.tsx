import type { ReactNode } from 'react'
import { useI18n } from '../i18n'
import type { MaterialsSortDir } from '../pages/data/DataShared'

type SortOption = {
  value: string
  label: string
}

type MaterialsSectionToolbarProps = {
  count?: number
  sortKey?: string
  sortOptions?: SortOption[]
  sortDir?: MaterialsSortDir
  onSortKeyChange?: (value: string) => void
  onSortDirToggle?: () => void
  children?: ReactNode
}

export default function MaterialsSectionToolbar({
  count,
  sortKey,
  sortOptions,
  sortDir,
  onSortKeyChange,
  onSortDirToggle,
  children,
}: MaterialsSectionToolbarProps) {
  const { tx } = useI18n()
  const showSort = Boolean(sortOptions && sortOptions.length > 0 && sortKey && onSortKeyChange)

  return (
    <div className="materials-compact-toolbar">
      {typeof count === 'number' ? <span className="materials-tile-pill">{tx(`${count} 项`, `${count} items`)}</span> : null}
      {showSort ? (
        <select
          className="materials-toolbar-control"
          aria-label={tx('排序字段', 'Sort field')}
          value={sortKey}
          onChange={(event) => onSortKeyChange?.(event.target.value)}
        >
          {sortOptions?.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      ) : null}
      {showSort && onSortDirToggle ? (
        <button type="button" className="btn btn-sm materials-toolbar-control" onClick={onSortDirToggle}>
          {sortDir === 'desc' ? tx('倒序', 'Descending') : tx('正序', 'Ascending')}
        </button>
      ) : null}
      {children}
    </div>
  )
}
