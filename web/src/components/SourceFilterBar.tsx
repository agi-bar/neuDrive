import { useI18n } from '../i18n'
import type { SourceFilterOption } from '../pages/data/DataShared'
import { sourceFilterLabel } from '../pages/data/DataShared'

type SourceFilterBarProps = {
  options: SourceFilterOption[]
  value: string
  onChange: (value: string) => void
}

export default function SourceFilterBar({ options, value, onChange }: SourceFilterBarProps) {
  const { locale } = useI18n()

  if (options.length === 0) return null

  return (
    <div className="source-filter-row">
      <button
        type="button"
        className={`source-filter-chip${value === 'all' ? ' is-active' : ''}`}
        onClick={() => onChange('all')}
      >
        {sourceFilterLabel('all', locale)}
      </button>
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          className={`source-filter-chip${value === option.value ? ' is-active' : ''}`}
          onClick={() => onChange(option.value)}
        >
          {option.label}
          <span className="source-filter-count">{option.count}</span>
        </button>
      ))}
    </div>
  )
}
