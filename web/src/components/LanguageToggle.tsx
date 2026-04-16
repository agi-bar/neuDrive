import { useI18n, type AppLocale } from '../i18n'

type LanguageToggleProps = {
  compact?: boolean
}

const LOCALES: Array<{ value: AppLocale; label: string }> = [
  { value: 'en', label: 'EN' },
  { value: 'zh-CN', label: '中文' },
]

export default function LanguageToggle({ compact = false }: LanguageToggleProps) {
  const { locale, setLocale, tx } = useI18n()

  return (
    <div
      className={`language-toggle ${compact ? 'language-toggle-compact' : ''}`}
      role="group"
      aria-label={tx('切换语言', 'Switch language')}
    >
      {LOCALES.map((entry) => (
        <button
          key={entry.value}
          type="button"
          className={`language-toggle-btn ${locale === entry.value ? 'language-toggle-btn-active' : ''}`}
          onClick={() => setLocale(entry.value)}
        >
          {entry.label}
        </button>
      ))}
    </div>
  )
}
